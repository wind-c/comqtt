// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-co
// SPDX-FileContributor: mochi-co

package storage

import (
	"encoding/json"
	"errors"

	"github.com/wind-c/comqtt/mqtt/packets"
	"github.com/wind-c/comqtt/mqtt/system"
)

const (
	SubscriptionKey = "sub" // unique key to denote Subscriptions in a store
	SysInfoKey      = "sys" // unique key to denote server system information in a store
	RetainedKey     = "ret" // unique key to denote retained messages in a store
	InflightKey     = "ifm" // unique key to denote inflight messages in a store
	ClientKey       = "cl"  // unique key to denote clients in a store
)

var (
	// ErrDBFileNotOpen indicates that the file database (e.g. bolt/badger) wasn't open for reading.
	ErrDBFileNotOpen = errors.New("db file not open")
)

// Client is a storable representation of an mqtt client.
type Client struct {
	Will            ClientWill       `json:"will,omitempty"`       // will topic and payload data if applicable
	Properties      ClientProperties `json:"properties,omitempty"` // the connect properties for the client
	Username        []byte           `json:"username,omitempty"`   // the username of the client
	ID              string           `json:"id" storm:"id"`        // the client id / storage key
	T               string           `json:"t,omitempty"`          // the data type (client)
	Remote          string           `json:"remote,omitempty"`     // the remote address of the client
	Listener        string           `json:"listener,omitempty"`   // the listener the client connected on
	ProtocolVersion byte             `json:"protocolVersion"`      // mqtt protocol version of the client
	Clean           bool             `json:"clean,omitempty"`      // if the client requested a clean start/session
}

// ClientProperties contains a limited set of the mqtt v5 properties specific to a client connection.
type ClientProperties struct {
	AuthenticationData        []byte                 `json:"authenticationData,omitempty"`
	User                      []packets.UserProperty `json:"user,omitempty"`
	AuthenticationMethod      string                 `json:"authenticationMethod,omitempty"`
	SessionExpiryInterval     uint32                 `json:"sessionExpiryInterval,omitempty"`
	MaximumPacketSize         uint32                 `json:"maximumPacketSize,omitempty"`
	ReceiveMaximum            uint16                 `json:"receiveMaximum,omitempty"`
	TopicAliasMaximum         uint16                 `json:"topicAliasMaximum,omitempty"`
	SessionExpiryIntervalFlag bool                   `json:"sessionExpiryIntervalFlag,omitempty"`
	RequestProblemInfo        byte                   `json:"requestProblemInfo,omitempty"`
	RequestProblemInfoFlag    bool                   `json:"requestProblemInfoFlag,omitempty"`
	RequestResponseInfo       byte                   `json:"requestResponseInfo,omitempty"`
}

// ClientWill contains a will message for a client, and limited mqtt v5 properties.
type ClientWill struct {
	Payload           []byte                 `json:"payload,omitempty"`
	User              []packets.UserProperty `json:"user,omitempty"`
	TopicName         string                 `json:"topicName,omitempty"`
	Flag              uint32                 `json:"flag,omitempty"`
	WillDelayInterval uint32                 `json:"willDelayInterval,omitempty"`
	Qos               byte                   `json:"qos,omitempty"`
	Retain            bool                   `json:"retain,omitempty"`
}

// MarshalBinary encodes the values into a json string.
func (d Client) MarshalBinary() (data []byte, err error) {
	return json.Marshal(d)
}

// UnmarshalBinary decodes a json string into a struct.
func (d *Client) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, d)
}

// Message is a storable representation of an MQTT message (specifically publish).
type Message struct {
	Properties  MessageProperties   `json:"properties,omitempty"`    // -
	Payload     []byte              `json:"payload"`                 // the message payload (if retained)
	T           string              `json:"t,omitempty"`             // the data type
	ID          string              `json:"id,omitempty" storm:"id"` // the storage key
	Origin      string              `json:"origin"`                  // the id of the client who sent the message
	TopicName   string              `json:"topic_name"`              // the topic the message was sent to (if retained)
	FixedHeader packets.FixedHeader `json:"fixedheader"`             // the header properties of the message
	Created     int64               `json:"created"`                 // the time the message was created in unixtime
	Sent        int64               `json:"sent,omitempty"`          // the last time the message was sent (for retries) in unixtime (if inflight)
	PacketID    uint16              `json:"packet_id"`               // the unique id of the packet (if inflight)
}

// MessageProperties contains a limited subset of mqtt v5 properties specific to publish messages.
type MessageProperties struct {
	CorrelationData        []byte                 `json:"correlationData,omitempty"`
	SubscriptionIdentifier []int                  `json:"subscriptionIdentifier,omitempty"`
	User                   []packets.UserProperty `json:"user,omitempty"`
	ContentType            string                 `json:"contentType,omitempty"`
	ResponseTopic          string                 `json:"responseTopic,omitempty"`
	MessageExpiryInterval  uint32                 `json:"messageExpiry,omitempty"`
	TopicAlias             uint16                 `json:"topicAlias,omitempty"`
	PayloadFormat          byte                   `json:"payloadFormat,omitempty"`
	PayloadFormatFlag      bool                   `json:"payloadFormatFlag,omitempty"`
}

// MarshalBinary encodes the values into a json string.
func (d Message) MarshalBinary() (data []byte, err error) {
	return json.Marshal(d)
}

// UnmarshalBinary decodes a json string into a struct.
func (d *Message) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, d)
}

// Subscription is a storable representation of an mqtt subscription.
type Subscription struct {
	T                 string `json:"t,omitempty"`
	ID                string `json:"id,omitempty" storm:"id"`
	Client            string `json:"client,omitempty"`
	Filter            string `json:"filter,omitempty"`
	Identifier        int    `json:"identifier,omitempty"`
	RetainHandling    byte   `json:"retain_handling,omitempty"`
	Qos               byte   `json:"qos"`
	RetainAsPublished bool   `json:"retain_as_pub,omitempty"`
	NoLocal           bool   `json:"no_local,omitempty"`
}

// MarshalBinary encodes the values into a json string.
func (d Subscription) MarshalBinary() (data []byte, err error) {
	return json.Marshal(d)
}

// UnmarshalBinary decodes a json string into a struct.
func (d *Subscription) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, d)
}

// SystemInfo is a storable representation of the system information values.
type SystemInfo struct {
	system.Info        // embed the system info struct
	T           string `json:"t,omitempty"`             // the data type
	ID          string `json:"id,omitempty" storm:"id"` // the storage key
}

// MarshalBinary encodes the values into a json string.
func (d SystemInfo) MarshalBinary() (data []byte, err error) {
	return json.Marshal(d)
}

// UnmarshalBinary decodes a json string into a struct.
func (d *SystemInfo) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, d)
}
