// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package cluster

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// encodePublish encodes a publish the way the cluster does when it relays one
// to another node.
func encodePublish(t *testing.T, topic string, payload []byte) []byte {
	t.Helper()
	pk := packets.Packet{
		FixedHeader: packets.FixedHeader{Type: packets.Publish},
		TopicName:   topic,
		Payload:     payload,
	}
	var buf bytes.Buffer
	require.NoError(t, pk.PublishEncode(&buf))
	return buf.Bytes()
}

// A peer in the cluster chooses the bytes in the payload, including the
// remaining length it declares. A payload that is shorter than the declared
// body must be dropped rather than sliced out of range.
func TestProcessRelayMsgDropsMalformedPublish(t *testing.T) {
	server := mqtt.New(&mqtt.Options{InlineClient: true})

	// A wildcard subscription, so that a payload which was not dropped would
	// reach the handler whatever topic it carried.
	published := make(chan string, 8)
	require.NoError(t, server.Subscribe("#", 1, func(cl *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
		published <- pk.TopicName
	}))

	a := &Agent{}
	a.BindMqttServer(server)

	cases := []struct {
		name    string
		payload []byte
	}{
		{"nil payload", nil},
		{"empty payload", []byte{}},
		{"type byte only", []byte{packets.Publish << 4}},
		{"body longer than the payload", []byte{packets.Publish << 4, 0xE4, 0x01}},
		{"body far longer than the payload", []byte{packets.Publish << 4, 0xFF, 0xFF, 0xFF, 0x7F}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				a.processRelayMsg(&message.Message{
					Type:    packets.Publish,
					Payload: c.payload,
				})
			})
			require.Empty(t, published, "the malformed payload was published")
		})
	}
}

// A well formed publish must still reach the subscribers.
func TestProcessRelayMsgPublishesValidPacket(t *testing.T) {
	server := mqtt.New(&mqtt.Options{InlineClient: true})

	received := make(chan string, 1)
	require.NoError(t, server.Subscribe("a/b/c", 1, func(cl *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
		received <- pk.TopicName
	}))

	a := &Agent{}
	a.BindMqttServer(server)

	payload := encodePublish(t, "a/b/c", []byte("hello mochi"))
	a.processRelayMsg(&message.Message{
		Type:    packets.Publish,
		Payload: payload,
	})

	select {
	case topic := <-received:
		require.Equal(t, "a/b/c", topic)
	case <-time.After(5 * time.Second):
		t.Fatal("the publish did not reach the subscriber")
	}
}
