// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package rest

import (
	"time"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

type RetainedMsg struct {
	Topic    string `json:"topic"`
	Payload  string `json:"payload"`
	Qos      byte   `json:"qos"`
	ClientID string `json:"client_id"`
	Created  int64  `json:"created"`
}

type client struct {
	ID              string    `json:"id"`
	IP              string    `json:"ip"`
	Online          bool      `json:"online"`
	Username        string    `json:"username"`
	TopicFilters    []string  `json:"topic_filters"`
	ProtocolVersion byte      `json:"protocol_version"`
	SessionClean    bool      `json:"session_clean"`
	WillTopicName   string    `json:"will_topic_name"`
	WillPayload     string    `json:"will_payload"`
	WillRetain      bool      `json:"will_retain"`
	InflightCount   int       `json:"inflight_count"`
	ConnectedAt     time.Time `json:"connected_at"`
	BytesReceived   int64     `json:"bytes_received"`
	BytesSent       int64     `json:"bytes_sent"`
	Keepalive       uint16    `json:"keepalive"`
}

func genClient(cl *mqtt.Client) client {
	filters := make([]string, 0, len(cl.State.Subscriptions.GetAll()))
	for k := range cl.State.Subscriptions.GetAll() {
		filters = append(filters, k)
	}

	nc := client{
		ID:              cl.ID,
		IP:              cl.Net.Remote,
		Online:          !cl.Closed(),
		Username:        string(cl.Properties.Username),
		TopicFilters:    filters,
		ProtocolVersion: cl.Properties.ProtocolVersion,
		SessionClean:    cl.Properties.Clean,
		WillTopicName:   cl.Properties.Will.TopicName,
		WillRetain:      cl.Properties.Will.Retain,
		InflightCount:   cl.State.Inflight.Len(),
		ConnectedAt:     cl.ConnectedAt,
		Keepalive:       cl.State.Keepalive,
		BytesReceived:   cl.BytesRecv(),
		BytesSent:       cl.BytesSent(),
	}
	if cl.Properties.Will.Payload != nil {
		nc.WillPayload = string(cl.Properties.Will.Payload)
	}

	return nc
}

type message struct {
	TopicName string `json:"topic_name"`
	Payload   string `json:"payload"`
	Retain    bool   `json:"retain"`
	Qos       byte   `json:"qos"`
}

func retainedMsgFromPacket(pk packets.Packet) RetainedMsg {
	return RetainedMsg{
		Topic:    pk.TopicName,
		Payload:  string(pk.Payload),
		Qos:      pk.FixedHeader.Qos,
		ClientID: pk.Origin,
		Created:  pk.Created,
	}
}
