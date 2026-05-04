// mqtt/dashboard/sse/hooks_test.go
package sse

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func newHook(t *testing.T) (*HubHook, <-chan Event) {
	t.Helper()
	hub := NewHub(16)
	t.Cleanup(func() { hub.Close() })
	hook := &HubHook{Hub: hub, Node: "n1"}
	return hook, hub.Subscribe()
}

func recv(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
		return Event{}
	}
}

func mkClient(id, username string) *mqtt.Client {
	cl := &mqtt.Client{ID: id}
	cl.Properties.Username = []byte(username)
	cl.Net.Remote = "127.0.0.1:0"
	return cl
}

func TestHubHookProvides(t *testing.T) {
	h := &HubHook{}
	for _, want := range []byte{mqtt.OnConnect, mqtt.OnDisconnect, mqtt.OnPublished, mqtt.OnSubscribed, mqtt.OnUnsubscribed} {
		if !h.Provides(want) {
			t.Fatalf("Provides(%d) = false, want true", want)
		}
	}
	if h.Provides(mqtt.OnSysInfoTick) {
		t.Fatalf("Provides(OnSysInfoTick) should be false")
	}
}

func TestHubHookOnConnect(t *testing.T) {
	hook, ch := newHook(t)
	if err := hook.OnConnect(mkClient("alice", "u"), packets.Packet{}); err != nil {
		t.Fatalf("OnConnect: %v", err)
	}
	ev := recv(t, ch)
	if ev.Type != "client.connected" {
		t.Fatalf("type: %q", ev.Type)
	}
	if ev.Node != "n1" {
		t.Fatalf("node: %q", ev.Node)
	}
	var payload map[string]any
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload["client_id"] != "alice" {
		t.Fatalf("client_id: %v", payload["client_id"])
	}
}

func TestHubHookOnDisconnect(t *testing.T) {
	hook, ch := newHook(t)
	hook.OnDisconnect(mkClient("alice", "u"), errors.New("normal"), false)
	ev := recv(t, ch)
	if ev.Type != "client.disconnected" {
		t.Fatalf("type: %q", ev.Type)
	}
	var payload map[string]any
	_ = json.Unmarshal(ev.Payload, &payload)
	if payload["reason"] != "normal" {
		t.Fatalf("reason: %v", payload["reason"])
	}
}

func TestHubHookOnPublished(t *testing.T) {
	hook, ch := newHook(t)
	pk := packets.Packet{TopicName: "sensors/temp", Payload: []byte("21.5")}
	pk.FixedHeader.Qos = 1
	hook.OnPublished(mkClient("alice", "u"), pk)
	ev := recv(t, ch)
	if ev.Type != "message.published" {
		t.Fatalf("type: %q", ev.Type)
	}
	var payload map[string]any
	_ = json.Unmarshal(ev.Payload, &payload)
	if payload["topic"] != "sensors/temp" {
		t.Fatalf("topic: %v", payload["topic"])
	}
	// JSON unmarshals numbers as float64
	if payload["qos"].(float64) != 1 {
		t.Fatalf("qos: %v", payload["qos"])
	}
	if payload["payload_size"].(float64) != 4 {
		t.Fatalf("payload_size: %v", payload["payload_size"])
	}
}

func TestHubHookOnSubscribedFansOutPerFilter(t *testing.T) {
	hook, ch := newHook(t)
	pk := packets.Packet{
		Filters: []packets.Subscription{
			{Filter: "a/+"},
			{Filter: "b/c"},
		},
	}
	hook.OnSubscribed(mkClient("alice", "u"), pk, nil, nil)
	for i := 0; i < 2; i++ {
		ev := recv(t, ch)
		if ev.Type != "subscription.added" {
			t.Fatalf("type[%d]: %q", i, ev.Type)
		}
	}
}

func TestHubHookOnUnsubscribed(t *testing.T) {
	hook, ch := newHook(t)
	pk := packets.Packet{Filters: []packets.Subscription{{Filter: "a/+"}}}
	hook.OnUnsubscribed(mkClient("alice", "u"), pk, nil, nil)
	ev := recv(t, ch)
	if ev.Type != "subscription.removed" {
		t.Fatalf("type: %q", ev.Type)
	}
}
