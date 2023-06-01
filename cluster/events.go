// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package cluster

import (
	"bytes"
	"errors"

	msg "github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// MqttEventHook is a mqtt event hook that callback when events such as connect, publish, subscribe, etc. occur.
type MqttEventHook struct {
	mqtt.HookBase
	agent *Agent
}

// ID returns the id of the hook.
func (h *MqttEventHook) ID() string {
	return "agent-event"
}

// Provides indicates which hook methods this hook provides.
func (h *MqttEventHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnSessionEstablished,
		mqtt.OnSubscribed,
		mqtt.OnUnsubscribed,
		mqtt.OnPublished,
		mqtt.OnWillSent,
	}, []byte{b})
}

// Init initializes
func (h *MqttEventHook) Init(config any) error {
	if _, ok := config.(*Agent); !ok && config != nil {
		return mqtt.ErrInvalidConfigType
	}

	if config == nil {
		return errors.New("MqttEventHook initialization failed")
	}

	h.agent = config.(*Agent)
	return nil
}

// OnSessionEstablished notifies other nodes to perform local subscription cleanup when their session is established.
func (h *MqttEventHook) OnSessionEstablished(cl *mqtt.Client, pk packets.Packet) {
	if cl.InheritWay != mqtt.InheritWayRemote {
		return
	}
	h.agent.SubmitOutTask(&pk)
}

// OnPublished is called when a client has published a message to subscribers.
func (h *MqttEventHook) OnPublished(cl *mqtt.Client, pk packets.Packet) {
	if pk.Connect.ClientIdentifier == "" {
		pk.Connect.ClientIdentifier = cl.ID
	}
	h.agent.SubmitOutTask(&pk)
}

// OnWillSent is called when an LWT message has been issued from a disconnecting client.
func (h *MqttEventHook) OnWillSent(cl *mqtt.Client, pk packets.Packet) {
	if pk.Connect.ClientIdentifier == "" {
		pk.Connect.ClientIdentifier = cl.ID
	}
	h.agent.SubmitOutTask(&pk)
}

// OnSubscribed is called when a client subscribes to one or more filters.
func (h *MqttEventHook) OnSubscribed(cl *mqtt.Client, pk packets.Packet, reasonCodes []byte, counts []int) {
	if len(pk.Filters) == 0 {
		return
	}
	for i, v := range pk.Filters {
		if reasonCodes[i] <= packets.CodeGrantedQos2.Code && counts[i] == 1 { // first subscription
			m := msg.Message{
				Type:            packets.Subscribe,
				ClientID:        cl.ID,
				NodeID:          h.agent.GetLocalName(),
				ProtocolVersion: cl.Properties.ProtocolVersion,
				Payload:         []byte(v.Filter),
			}
			h.agent.SubmitRaftTask(&m)
		}
	}
}

func (h *MqttEventHook) OnUnsubscribed(cl *mqtt.Client, pk packets.Packet, reasonCodes []byte, counts []int) {
	if len(pk.Filters) == 0 {
		return
	}
	for i, v := range pk.Filters {
		if reasonCodes[i] == packets.CodeSuccess.Code && counts[i] == 0 { // first subscription
			m := msg.Message{
				Type:            packets.Unsubscribe,
				ClientID:        cl.ID,
				NodeID:          h.agent.GetLocalName(),
				ProtocolVersion: cl.Properties.ProtocolVersion,
				Payload:         []byte(v.Filter),
			}
			h.agent.SubmitRaftTask(&m)
		}
	}
}
