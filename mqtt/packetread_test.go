// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package mqtt

import (
	"bytes"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// mqttMaxRemaining is the largest Remaining Length an MQTT packet can declare.
const mqttMaxRemaining = 268435455

// A peer declares the packet size in the fixed header, so before the fix a
// peer could announce the 256 MB maximum and then fall silent, pinning that
// much memory in the broker while it waited for bytes that never come.
func TestReadRemainingDoesNotReserveDeclaredLength(t *testing.T) {
	const sent = 16

	pr, pw := io.Pipe()
	defer pw.Close()

	go func() {
		_, _ = pw.Write(bytes.Repeat([]byte{'x'}, sent))
		// then say nothing further, exactly as the attack does
		time.Sleep(30 * time.Second)
	}()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	type result struct {
		p   []byte
		err error
	}
	res := make(chan result, 1)
	go func() {
		p, err := readRemaining(pr, mqttMaxRemaining)
		res <- result{p, err}
	}()

	// Let the reader reach the point where it is blocked waiting for more. A
	// pre-allocating implementation has already committed the full declared
	// length by now; an incremental one has committed only what arrived.
	time.Sleep(750 * time.Millisecond)
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	grew := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	require.Less(t, grew, int64(8<<20),
		"declaring %d bytes and sending %d grew the heap by %d bytes; memory must track bytes received, not bytes declared",
		mqttMaxRemaining, sent, grew)

	select {
	case r := <-res:
		t.Fatalf("readRemaining returned before the peer finished: len=%d err=%v", len(r.p), r.err)
	default:
	}

	pw.Close()
	<-res // reap the reader
}

// The same property, exercised through the real receive path rather than the
// helper, so the guarantee is pinned where it is actually relied upon.
func TestClientReadPacketDoesNotReserveDeclaredLength(t *testing.T) {
	const sent = 16

	cl, _, w := newTestClient()
	defer w.Close()

	type result struct {
		pk  packets.Packet
		err error
	}
	res := make(chan result, 1)
	go func() {
		pk, err := cl.ReadPacket(&packets.FixedHeader{
			Type: packets.Publish, Remaining: mqttMaxRemaining,
		})
		res <- result{pk, err}
	}()

	// net.Pipe is unbuffered, so the write has to race the reader.
	go func() {
		// A PUBLISH fixed header announcing the maximum Remaining Length.
		_, _ = w.Write([]byte{byte(packets.Publish), 0xFF, 0xFF, 0xFF, 0x7F})
		_, _ = w.Write(bytes.Repeat([]byte{'x'}, sent))
	}()

	time.Sleep(750 * time.Millisecond)
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// One connection must not be able to cost hundreds of MiB on its own.
	require.Less(t, m.HeapAlloc, uint64(64<<20),
		"a single connection announcing %d bytes held the heap at %d bytes", mqttMaxRemaining, m.HeapAlloc)

	select {
	case <-res:
		t.Fatal("ReadPacket returned before the peer finished sending")
	default:
	}

	w.Close()
	<-res // reap the reader
}

// The incremental path must still deliver the exact bytes of a genuinely large
// packet; the fix is about when memory is committed, not about truncating.
func TestReadRemainingLargeBodyIsDeliveredIntact(t *testing.T) {
	for _, size := range []int{
		maxPacketPreallocBytes - 1, // fast path
		maxPacketPreallocBytes,     // fast path, boundary
		maxPacketPreallocBytes + 1, // incremental path
		1024 * 1024,                // incremental, several growth steps
	} {
		body := make([]byte, size)
		for i := range body {
			body[i] = byte(i % 251)
		}

		p, err := readRemaining(bytes.NewReader(body), size)
		require.NoError(t, err, "size %d", size)
		require.Len(t, p, size)
		require.True(t, bytes.Equal(body, p), "size %d: body was corrupted", size)
	}
}

// A peer that declares more than it delivers must still surface an error, and
// must not have been charged for the part it never sent.
func TestReadRemainingTruncatedByPeer(t *testing.T) {
	size := maxPacketPreallocBytes + 4096

	p, err := readRemaining(bytes.NewReader(make([]byte, size/2)), size)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, p)
}

func TestReadRemainingEmpty(t *testing.T) {
	p, err := readRemaining(bytes.NewReader(nil), 0)
	require.NoError(t, err)
	require.Empty(t, p)
}