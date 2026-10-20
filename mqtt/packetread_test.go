// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package mqtt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// mqttMaxRemaining is the largest Remaining Length an MQTT packet can declare.
const mqttMaxRemaining = 268435455

// heapLimit separates "memory tracks bytes received" from "memory tracks bytes
// declared": the declared length used in these tests is 256 MiB, so anything in
// this range tells the two apart with a wide margin.
const heapLimit = int64(8 << 20)

// eofWithLastBytes returns the final bytes and io.EOF from the same Read call.
// The io.Reader contract permits it ("It may return the (n, err) either from
// the same call or from the next call"), crypto/tls does exactly that when a
// close_notify arrives behind the last data, and io.ReadFull treats it as
// success. A loop that inspects the error before checking whether the body is
// complete gets this wrong, so it is pinned explicitly here as well as through
// the iotest reader below.
type eofWithLastBytes struct {
	data []byte
	off  int
}

func (r *eofWithLastBytes) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	if r.off >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

// partialThenError delivers part of the data and then fails with something
// other than io.EOF, which is what a reset or a timeout looks like.
type partialThenError struct {
	data []byte
	off  int
	err  error
}

func (r *partialThenError) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	if r.off >= len(r.data) {
		return n, r.err
	}
	return n, nil
}

// TestReadRemainingDoesNotOverRead pins the invariant that the growth path
// consumes exactly the declared body and no more. A regression that narrowed
// the read window to p[len(p):] would swallow the first bytes of the next
// packet on the connection, and nothing else in the suite would notice.
func TestReadRemainingDoesNotOverRead(t *testing.T) {
	const body = maxPacketPreallocBytes + 4096
	const tail = 64

	stream := bytes.NewReader(make([]byte, body+tail))

	p, err := readRemaining(stream, body)
	require.NoError(t, err)
	require.Len(t, p, body)

	require.Equal(t, tail, stream.Len(),
		"readRemaining consumed %d bytes past the declared body", body+tail-stream.Len())
}

// TestReadRemainingMatchesReadFull pins readRemaining to the behaviour of the
// io.ReadFull it replaced — same bytes, same success/failure split, same error
// — across readers that hand back data in every way the contract allows.
func TestReadRemainingMatchesReadFull(t *testing.T) {
	sentinel := errors.New("sentinel")
	reset := errors.New("connection reset")

	readers := []struct {
		name string
		make func([]byte) io.Reader
	}{
		{"fills the whole buffer", func(b []byte) io.Reader { return bytes.NewReader(b) }},
		{"one byte per Read", func(b []byte) io.Reader { return iotest.OneByteReader(bytes.NewReader(b)) }},
		{"half the buffer per Read", func(b []byte) io.Reader { return iotest.HalfReader(bytes.NewReader(b)) }},
		{"final bytes with io.EOF", func(b []byte) io.Reader { return &eofWithLastBytes{data: b} }},
		{"final bytes with io.EOF (stdlib)", func(b []byte) io.Reader { return iotest.DataErrReader(bytes.NewReader(b)) }},
		{"stops half way", func(b []byte) io.Reader { return bytes.NewReader(b[:len(b)/2]) }},
		{"always an error", func(b []byte) io.Reader { return iotest.ErrReader(sentinel) }},
		{"partial data then a reset", func(b []byte) io.Reader {
			return &partialThenError{data: b[:len(b)/2], err: reset}
		}},
	}

	sizes := []int{
		0,
		1,
		maxPacketPreallocBytes - 1, // last size before the growth path
		maxPacketPreallocBytes,     // last size on the single-allocation path
		maxPacketPreallocBytes + 1, // first size on the growth path
		maxPacketPreallocBytes + 7,
		256 * 1024,
		256*1024 + 777,
	}

	for _, size := range sizes {
		body := make([]byte, size)
		for i := range body {
			body[i] = byte(i*31 + 7)
		}

		for _, rd := range readers {
			t.Run(fmt.Sprintf("%s/size=%d", rd.name, size), func(t *testing.T) {
				want := make([]byte, size)
				_, wantErr := io.ReadFull(rd.make(body), want)

				got, gotErr := readRemaining(rd.make(body), size)

				if wantErr != nil {
					require.Error(t, gotErr, "io.ReadFull failed, so readRemaining must too")
					require.ErrorIs(t, gotErr, wantErr, "error must match io.ReadFull")
					require.Nil(t, got)
					return
				}

				require.NoError(t, gotErr, "io.ReadFull succeeded, so readRemaining must too")
				require.Len(t, got, size)
				require.Equal(t, want, got, "bytes must match io.ReadFull")
			})
		}
	}
}

// A peer declares the body size in the fixed header, so before the fix it could
// announce the 256 MB protocol maximum and then fall silent, pinning that much
// memory in the broker while it waited for bytes that never come.
//
// io.Pipe is synchronous — Write returns only once a Read has consumed the
// bytes — so the write completing is proof the reader took them and is now
// blocked wanting more. That makes the measurement deterministic instead of a
// race against a sleep.
func TestReadRemainingDoesNotReserveDeclaredLength(t *testing.T) {
	const sent = 16

	pr, pw := io.Pipe()

	consumed := make(chan struct{})
	go func() {
		_, _ = pw.Write(bytes.Repeat([]byte{'x'}, sent))
		close(consumed)
	}()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	done := make(chan error, 1)
	go func() {
		_, err := readRemaining(pr, mqttMaxRemaining)
		done <- err
	}()

	<-consumed

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	require.Less(t, int64(after.HeapAlloc)-int64(before.HeapAlloc), heapLimit,
		"declaring %d bytes and sending %d must not commit the declared length",
		mqttMaxRemaining, sent)

	select {
	case err := <-done:
		t.Fatalf("readRemaining returned before the peer finished: %v", err)
	default:
	}

	pw.Close()
	require.ErrorIs(t, <-done, io.ErrUnexpectedEOF)
}

// The same guarantee at the call site the broker actually depends on. A
// declared length arrives before any authentication happens —
// readConnectionPacket reads CONNECT through ReadPacket — so a single
// unauthenticated connection must not be able to commit 256 MiB.
func TestClientReadPacketDoesNotReserveDeclaredLength(t *testing.T) {
	const sent = 16

	// newTestClient builds the client on the second end of a net.Pipe, so r is
	// the peer side. Writing to the client's own end would send bytes towards
	// an end nobody reads, which is what the original version of this test did:
	// it asserted on the heap while never actually delivering a body byte.
	cl, r, _ := newTestClient()
	defer r.Close() //nolint:errcheck

	consumed := make(chan struct{})
	go func() {
		_, _ = r.Write(bytes.Repeat([]byte{'x'}, sent))
		close(consumed)
	}()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	done := make(chan error, 1)
	go func() {
		_, err := cl.ReadPacket(&packets.FixedHeader{
			Type: packets.Publish, Remaining: mqttMaxRemaining,
		})
		done <- err
	}()

	<-consumed

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	require.Less(t, int64(after.HeapAlloc)-int64(before.HeapAlloc), heapLimit,
		"a single connection announcing %d bytes must not commit the declared length",
		mqttMaxRemaining)

	select {
	case err := <-done:
		t.Fatalf("ReadPacket returned before the peer finished: %v", err)
	default:
	}

	r.Close()
	require.ErrorIs(t, <-done, io.ErrUnexpectedEOF,
		"a connection cut short mid-body must surface the same error io.ReadFull produced")
}

// The growth path must still deliver the exact bytes of a genuinely large
// packet; the fix is about when memory is committed, not about truncating.
func TestReadRemainingLargeBodyIsDeliveredIntact(t *testing.T) {
	for _, size := range []int{
		maxPacketPreallocBytes - 1, // single-allocation path
		maxPacketPreallocBytes,     // boundary
		maxPacketPreallocBytes + 1, // growth path
		1024 * 1024,                // several growth steps
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
