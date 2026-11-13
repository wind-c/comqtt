// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package mqtt

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// willACLHook authenticates every client but can deny WRITE access to the
// protected/ namespace, so will delivery can be checked against topic ACLs.
type willACLHook struct {
	HookBase
	deny atomic.Bool
	sent atomic.Bool
}

func (h *willACLHook) ID() string { return "will-acl-test" }

func (h *willACLHook) Provides(b byte) bool {
	return bytes.Contains([]byte{OnConnectAuthenticate, OnACLCheck, OnWillSent}, []byte{b})
}

func (h *willACLHook) OnConnectAuthenticate(cl *Client, pk packets.Packet) bool { return true }

func (h *willACLHook) OnACLCheck(cl *Client, topic string, write bool) bool {
	if write && h.deny.Load() && strings.HasPrefix(topic, "protected/") {
		return false
	}
	return true
}

func (h *willACLHook) OnWillSent(cl *Client, pk packets.Packet) {
	h.sent.Store(true)
}

func newWillACLServer(t *testing.T) (*Server, *willACLHook) {
	t.Helper()
	cc := *DefaultServerCapabilities
	cc.MaximumMessageExpiryInterval = 0
	cc.ReceiveMaximum = 0
	s := New(&Options{Logger: logger, Capabilities: &cc})
	h := new(willACLHook)
	require.NoError(t, s.AddHook(h, nil))
	return s, h
}

func newWillClient(t *testing.T, s *Server, id, topic string, delay uint32) *Client {
	t.Helper()
	cl, _, _ := newTestClient()
	cl.ID = id
	cl.Properties.Will = Will{
		Flag:              1,
		TopicName:         topic,
		Payload:           []byte("forged"),
		Retain:            true,
		WillDelayInterval: delay,
	}
	s.Clients.Add(cl)
	t.Cleanup(func() { cl.Stop(nil) })
	return cl
}

func TestWillACLRejectsDeniedImmediateWill(t *testing.T) {
	s, h := newWillACLServer(t)
	cl := newWillClient(t, s, "sender", "protected/cmd/evil", 0)
	h.deny.Store(true)

	s.sendLWT(cl)

	require.False(t, h.sent.Load())
	require.Empty(t, s.Topics.Messages("protected/cmd/evil"))
	require.Zero(t, atomic.LoadUint32(&cl.Properties.Will.Flag))
}

func TestWillACLRejectsDeniedDelayedWillAtSchedule(t *testing.T) {
	s, h := newWillACLServer(t)
	cl := newWillClient(t, s, "sender", "protected/cmd/evil", 1)
	h.deny.Store(true)

	s.sendLWT(cl)

	require.Zero(t, s.loop.willDelayed.Len())
	require.False(t, h.sent.Load())
	require.Zero(t, atomic.LoadUint32(&cl.Properties.Will.Flag))
}

func TestWillACLRejectsWillRevokedDuringDelay(t *testing.T) {
	s, h := newWillACLServer(t)
	cl := newWillClient(t, s, "sender", "protected/cmd/evil", 1)

	s.sendLWT(cl) // authorized at scheduling time
	pk, ok := s.loop.willDelayed.Get(cl.ID)
	require.True(t, ok)
	pk.Expiry = time.Now().Unix() - 1
	s.loop.willDelayed.Add(cl.ID, pk)

	h.deny.Store(true)
	s.sendDelayedLWT(time.Now().Unix())

	require.False(t, h.sent.Load())
	require.Empty(t, s.Topics.Messages("protected/cmd/evil"))
	require.Zero(t, s.loop.willDelayed.Len())
	require.Zero(t, atomic.LoadUint32(&cl.Properties.Will.Flag))
}

func TestWillACLAllowsAuthorizedImmediateWill(t *testing.T) {
	s, h := newWillACLServer(t)
	cl := newWillClient(t, s, "sender", "protected/cmd/evil", 0)

	s.sendLWT(cl)

	require.True(t, h.sent.Load())
	require.NotEmpty(t, s.Topics.Messages("protected/cmd/evil"))
	require.Zero(t, atomic.LoadUint32(&cl.Properties.Will.Flag))
}

func TestWillACLAllowsAuthorizedDelayedWill(t *testing.T) {
	s, h := newWillACLServer(t)
	cl := newWillClient(t, s, "sender", "protected/cmd/evil", 1)

	s.sendLWT(cl)
	pk, ok := s.loop.willDelayed.Get(cl.ID)
	require.True(t, ok)
	pk.Expiry = time.Now().Unix() - 1
	s.loop.willDelayed.Add(cl.ID, pk)

	s.sendDelayedLWT(time.Now().Unix())

	require.True(t, h.sent.Load())
	require.NotEmpty(t, s.Topics.Messages("protected/cmd/evil"))
	require.Zero(t, s.loop.willDelayed.Len())
	require.Zero(t, atomic.LoadUint32(&cl.Properties.Will.Flag))
}
