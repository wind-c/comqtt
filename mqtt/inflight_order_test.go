// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package mqtt

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// recordOrderHook captures the order in which ResendInflightMessages hands
// packets back out.
type recordOrderHook struct {
	HookBase
	ids []uint16
}

func (h *recordOrderHook) Provides(b byte) bool {
	return b == OnQosPublish
}

func (h *recordOrderHook) OnQosPublish(cl *Client, pk packets.Packet, sent int64, resends int) {
	h.ids = append(h.ids, pk.PacketID)
}

func packetIDs(t *testing.T, pks []packets.Packet) []uint16 {
	t.Helper()
	ids := make([]uint16, len(pks))
	for i, p := range pks {
		ids[i] = p.PacketID
	}
	return ids
}

// The reported defect: messages created within the same second share a
// one-second-resolution sort key, and because the slice is built by ranging a
// map the resulting order was a fresh random rotation on every call. [MQTT-4.6.0-1]
func TestInflightGetAllSameSecondIsDeterministic(t *testing.T) {
	cl, _, _ := newTestClient()

	// all within one second, exactly as the reporter's PoC
	const now = int64(1700000000)
	for id := uint16(1); id <= 6; id++ {
		cl.State.Inflight.Set(packets.Packet{PacketID: id, Created: now})
	}

	first := packetIDs(t, cl.State.Inflight.GetAll(false))
	require.Equal(t, []uint16{1, 2, 3, 4, 5, 6}, first,
		"tied messages must fall back to their original send order")

	// a single lucky run must not be mistaken for a fix, so hammer it: before
	// the fix every call reshuffled via Go's randomised map iteration order
	for i := 0; i < 500; i++ {
		require.Equal(t, first, packetIDs(t, cl.State.Inflight.GetAll(false)),
			"call %d returned a different order", i)
	}
}

// A larger window makes the pre-fix behaviour more obvious and guards against a
// tiebreaker that only works for small sets.
func TestInflightGetAllSameSecondLargeWindow(t *testing.T) {
	cl, _, _ := newTestClient()

	const now = int64(1700000000)
	const n = 256
	for id := uint16(1); id <= n; id++ {
		cl.State.Inflight.Set(packets.Packet{PacketID: id, Created: now})
	}

	want := make([]uint16, n)
	for i := range want {
		want[i] = uint16(i + 1)
	}

	for i := 0; i < 200; i++ {
		require.Equal(t, want, packetIDs(t, cl.State.Inflight.GetAll(false)), "call %d", i)
	}
}

// Created must still dominate: a tiebreaker must not reorder across seconds.
func TestInflightGetAllOrdersAcrossSeconds(t *testing.T) {
	cl, _, _ := newTestClient()

	// deliberately inserted out of order, and with packet ids that disagree
	// with the timestamps
	for _, p := range []packets.Packet{
		{PacketID: 9, Created: 300},
		{PacketID: 2, Created: 100},
		{PacketID: 7, Created: 200},
		{PacketID: 1, Created: 100},
		{PacketID: 5, Created: 300},
	} {
		cl.State.Inflight.Set(p)
	}

	require.Equal(t, []uint16{1, 2, 7, 5, 9}, packetIDs(t, cl.State.Inflight.GetAll(false)))
}

// NextImmediate reads through the same path and returns element 0, so it
// inherited the same scrambling.
func TestNextImmediateSameSecondIsDeterministic(t *testing.T) {
	cl, _, _ := newTestClient()

	const now = int64(1700000000)
	for id := uint16(1); id <= 6; id++ {
		cl.State.Inflight.Set(packets.Packet{PacketID: id, Created: now, Expiry: -1})
	}

	pk, ok := cl.State.Inflight.NextImmediate()
	require.True(t, ok)
	require.Equal(t, uint16(1), pk.PacketID, "the earliest message must be offered first")

	for i := 0; i < 500; i++ {
		pk, ok := cl.State.Inflight.NextImmediate()
		require.True(t, ok)
		require.Equal(t, uint16(1), pk.PacketID, "call %d", i)
	}
}

// The guarantee has to hold on the path a reconnecting persistent session
// actually takes, not just in the helper.
func TestResendInflightMessagesPreservesOrder(t *testing.T) {
	for attempt := 0; attempt < 25; attempt++ {
		cl, r, _ := newTestClient()

		h := &recordOrderHook{}
		require.NoError(t, cl.ops.hooks.Add(h, nil))

		const now = int64(1700000000)
		for id := uint16(1); id <= 6; id++ {
			cl.State.Inflight.Set(packets.Packet{
				FixedHeader: packets.FixedHeader{Type: packets.Publish, Qos: 1},
				PacketID:    id,
				Created:     now,
			})
		}

		// the broker writes to the other end of an unbuffered pipe, so drain
		// it while the resend runs
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			buf := make([]byte, 8192)
			for {
				if _, err := r.Read(buf); err != nil {
					return
				}
			}
		}()

		require.NoError(t, cl.ResendInflightMessages(true))
		r.Close()
		<-drained

		require.Equal(t, []uint16{1, 2, 3, 4, 5, 6}, h.ids, "resend %d went out out of order", attempt)
	}
}

// Sorting must not disturb the immediate-expiry filter.
func TestInflightGetAllImmediateFilterStillApplies(t *testing.T) {
	cl, _, _ := newTestClient()

	const now = int64(1700000000)
	cl.State.Inflight.Set(packets.Packet{PacketID: 1, Created: now, Expiry: -1})
	cl.State.Inflight.Set(packets.Packet{PacketID: 2, Created: now})
	cl.State.Inflight.Set(packets.Packet{PacketID: 3, Created: now, Expiry: -1})

	require.Equal(t, []uint16{1, 2, 3}, packetIDs(t, cl.State.Inflight.GetAll(false)))
	require.Equal(t, []uint16{1, 3}, packetIDs(t, cl.State.Inflight.GetAll(true)))
}