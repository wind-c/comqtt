// dashboard/sse/hub.go
package sse

import (
	"sync"
	"sync/atomic"
)

// Hub is a fan-out, ring-buffered event bus. Subscribers get a buffered
// channel; if their buffer fills, Publish drops events for that subscriber
// rather than blocking other subscribers or the broker hot path.
type Hub struct {
	mu      sync.RWMutex
	subs    []chan Event
	bufSize int
	drops   atomic.Int64
	seq     atomic.Int64
	closed  atomic.Bool
}

func NewHub(bufSize int) *Hub {
	return &Hub{bufSize: bufSize}
}

// Subscribe returns a new buffered channel. Caller MUST eventually call
// Unsubscribe with the same channel value to free resources. The returned
// channel is closed by Unsubscribe or by Close.
func (h *Hub) Subscribe() <-chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan Event, h.bufSize)
	h.subs = append(h.subs, ch)
	return ch
}

// Unsubscribe removes the subscriber and closes its channel. No-op if the
// channel was already closed (idempotent).
func (h *Hub) Unsubscribe(target <-chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.subs[:0]
	for _, s := range h.subs {
		if (<-chan Event)(s) == target {
			close(s)
			continue
		}
		out = append(out, s)
	}
	h.subs = out
}

// Publish stamps Seq on the event and fans it out to all subscribers
// non-blocking. If a subscriber's buffer is full, the event is dropped for
// that subscriber and Drops() is incremented.
func (h *Hub) Publish(e Event) {
	if h.closed.Load() {
		return
	}
	e.Seq = h.seq.Add(1)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subs {
		select {
		case s <- e:
		default:
			h.drops.Add(1)
		}
	}
}

// Drops returns the running count of events dropped due to slow subscribers.
func (h *Hub) Drops() int64 { return h.drops.Load() }

// Close shuts down the hub. Subsequent Publish calls are no-ops; all
// subscribed channels are closed.
func (h *Hub) Close() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.subs {
		close(s)
	}
	h.subs = nil
}
