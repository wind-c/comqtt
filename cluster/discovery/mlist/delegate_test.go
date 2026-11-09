// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind

package mlist

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt"
)

func newTestDelegate() (*Delegate, chan []byte) {
	msgCh := make(chan []byte, 16)
	d := NewDelegate(msgCh)
	d.LocalNode = &memberlist.Node{Name: "node0"}
	return d, msgCh
}

// stopWithTimeout runs Stop and fails if it does not return.
func stopWithTimeout(t *testing.T, d *Delegate) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.Stop()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}
}

func TestDelegateStopReturns(t *testing.T) {
	d, _ := newTestDelegate()
	stopWithTimeout(t, d)
}

// A second Stop must neither deadlock nor panic closing msgCh again.
func TestDelegateStopTwice(t *testing.T) {
	d, _ := newTestDelegate()
	stopWithTimeout(t, d)
	stopWithTimeout(t, d)
}

// Stop must not close msgCh: the channel is owned by the agent, and a
// memberlist handler delivering a message during shutdown would panic.
func TestDelegateNotifyMsgAfterStop(t *testing.T) {
	d, msgCh := newTestDelegate()
	stopWithTimeout(t, d)
	require.NotPanics(t, func() {
		d.NotifyMsg([]byte("relay"))
	})
	select {
	case msg := <-msgCh:
		require.Equal(t, []byte("relay"), msg)
	default:
		t.Fatal("NotifyMsg message was not delivered")
	}
}

// memberlist calls LocalState and MergeRemoteState from different goroutines
// during overlapping push/pulls. Run with -race to catch the unlocked map
// access; without it the runtime may still report concurrent map writes.
func TestDelegateStateConcurrentAccess(t *testing.T) {
	d, _ := newTestDelegate()
	d.BindMqttServer(mqtt.New(&mqtt.Options{InlineClient: true}))

	remote, err := json.Marshal(map[string]int64{"node1": 7})
	require.NoError(t, err)

	const rounds = 300
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				_ = d.LocalState(false)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				d.MergeRemoteState(remote, false)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				_ = d.Stats()
			}
		}()
	}
	wg.Wait()

	got := d.Stats()
	require.Equal(t, int64(7), got["node1"])
	require.Contains(t, got, "node0")
}

// Stats must hand out a snapshot: mutating it must not corrupt the delegate.
func TestDelegateStatsReturnsCopy(t *testing.T) {
	d, _ := newTestDelegate()
	d.State["node0"] = 3
	got := d.Stats()
	require.Equal(t, int64(3), got["node0"])
	got["node0"] = 99
	got["evil"] = 1
	require.Equal(t, int64(3), d.State["node0"])
	require.NotContains(t, d.State, "evil")
}
