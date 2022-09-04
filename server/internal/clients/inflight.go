package clients

import (
	"github.com/wind-c/comqtt/server/internal/packets"
)

// InflightMessage contains data about a packet which is currently in-flight.
type InflightMessage struct {
	Packet  packets.Packet // the packet currently in-flight.
	Sent    int64          // the last time the message was sent (for retries) in unixtime.
	Resends int            // the number of times the message was attempted to be sent.
	Expiry  int64          // the message expiration time in unixtime.
}

// Inflight is an interface of for storing and manipulating inflight messages
type Inflight interface {
	Set(key uint16, in *InflightMessage) bool
	Get(key uint16) (*InflightMessage, bool)
	GetAll() map[uint16]*InflightMessage
	Walk(cl *Client, handler func(cl *Client, im *InflightMessage, force bool) error)
	Len() int
	Delete(key uint16) bool
}
