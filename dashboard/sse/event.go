// dashboard/sse/event.go
package sse

import "encoding/json"

// Event is one item dispatched through the SSE hub. Type is the event class
// (client.connected, message.published, ...). Payload is event-specific JSON
// kept opaque so producers don't share a giant union struct. Seq is set by
// the hub on Publish - producers do not populate it.
type Event struct {
	Type    string          `json:"type"`
	Node    string          `json:"node"`
	TS      int64           `json:"ts"`
	Seq     int64           `json:"seq"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
