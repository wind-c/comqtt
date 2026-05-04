// mqtt/dashboard/sse/hooks.go
package sse

import (
	"encoding/json"
	"time"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// HubHook is a *mqtt.Hook implementation that publishes broker lifecycle
// events into a Hub. It is intentionally read-only: it never blocks the
// broker hot path because Hub.Publish is non-blocking with drop-on-full.
type HubHook struct {
	mqtt.HookBase
	Hub  *Hub
	Node string
}

func (h *HubHook) ID() string { return "comqtt-dashboard-sse" }

// Provides advertises the hook event types we react to. Anything else falls
// through to HookBase's no-op defaults.
func (h *HubHook) Provides(b byte) bool {
	switch b {
	case mqtt.OnConnect, mqtt.OnDisconnect, mqtt.OnPublished, mqtt.OnSubscribed, mqtt.OnUnsubscribed:
		return true
	}
	return false
}

func (h *HubHook) OnConnect(cl *mqtt.Client, _ packets.Packet) error {
	payload, _ := json.Marshal(map[string]any{
		"client_id": cl.ID,
		"username":  string(cl.Properties.Username),
		"remote":    cl.Net.Remote,
	})
	h.Hub.Publish(Event{
		Type:    "client.connected",
		Node:    h.Node,
		TS:      time.Now().UnixMilli(),
		Payload: payload,
	})
	return nil
}

func (h *HubHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	payload, _ := json.Marshal(map[string]any{
		"client_id": cl.ID,
		"username":  string(cl.Properties.Username),
		"reason":    reason,
		"expire":    expire,
	})
	h.Hub.Publish(Event{
		Type:    "client.disconnected",
		Node:    h.Node,
		TS:      time.Now().UnixMilli(),
		Payload: payload,
	})
}

func (h *HubHook) OnPublished(cl *mqtt.Client, pk packets.Packet) {
	payload, _ := json.Marshal(map[string]any{
		"client_id":    cl.ID,
		"topic":        pk.TopicName,
		"qos":          pk.FixedHeader.Qos,
		"payload_size": len(pk.Payload),
	})
	h.Hub.Publish(Event{
		Type:    "message.published",
		Node:    h.Node,
		TS:      time.Now().UnixMilli(),
		Payload: payload,
	})
}

func (h *HubHook) OnSubscribed(cl *mqtt.Client, pk packets.Packet, _ []byte, _ []int) {
	for _, sub := range pk.Filters {
		payload, _ := json.Marshal(map[string]any{
			"client_id": cl.ID,
			"topic":     sub.Filter,
			"qos":       sub.Qos,
		})
		h.Hub.Publish(Event{
			Type:    "subscription.added",
			Node:    h.Node,
			TS:      time.Now().UnixMilli(),
			Payload: payload,
		})
	}
}

func (h *HubHook) OnUnsubscribed(cl *mqtt.Client, pk packets.Packet, _ []byte, _ []int) {
	for _, sub := range pk.Filters {
		payload, _ := json.Marshal(map[string]any{
			"client_id": cl.ID,
			"topic":     sub.Filter,
		})
		h.Hub.Publish(Event{
			Type:    "subscription.removed",
			Node:    h.Node,
			TS:      time.Now().UnixMilli(),
			Payload: payload,
		})
	}
}
