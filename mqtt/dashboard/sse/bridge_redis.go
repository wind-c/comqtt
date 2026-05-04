// mqtt/dashboard/sse/bridge_redis.go
package sse

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
)

const defaultEventsChannel = "comqtt:dashboard:events"

// Bridge connects a local *Hub to a Redis pub/sub channel for cluster-wide
// event aggregation. Each node:
//   - Forwards locally-produced events to the Redis channel.
//   - Subscribes to the channel and merges peer events into the local hub.
//
// Events with .Node == self.Node are dropped on receive to prevent loops.
// A small LRU dedupes (node, seq) pairs in case the network round-trips an
// event we already saw locally.
type Bridge struct {
	Client  *redis.Client
	Hub     *Hub
	Node    string
	Channel string
	BufSize int

	dedup *lru
	stopF context.CancelFunc
	wg    sync.WaitGroup
}

// NewBridge constructs a Bridge with sensible defaults. Channel defaults to
// "comqtt:dashboard:events". BufSize defaults to 4096 (LRU capacity).
func NewBridge(client *redis.Client, hub *Hub, node string) *Bridge {
	return &Bridge{
		Client:  client,
		Hub:     hub,
		Node:    node,
		Channel: defaultEventsChannel,
		BufSize: 4096,
	}
}

// Start spins up the publisher and subscriber goroutines. Stop must be
// called to release them.
func (b *Bridge) Start(ctx context.Context) {
	if b.Channel == "" {
		b.Channel = defaultEventsChannel
	}
	if b.BufSize <= 0 {
		b.BufSize = 4096
	}
	b.dedup = newLRU(b.BufSize)
	ctx, cancel := context.WithCancel(ctx)
	b.stopF = cancel

	b.wg.Add(2)
	go b.runPublisher(ctx)
	go b.runSubscriber(ctx)
}

// Stop cancels the publisher and subscriber and waits for them to exit.
// Safe to call multiple times.
func (b *Bridge) Stop() {
	if b.stopF != nil {
		b.stopF()
		b.stopF = nil
	}
	b.wg.Wait()
}

func (b *Bridge) runPublisher(ctx context.Context) {
	defer b.wg.Done()
	ch := b.Hub.Subscribe()
	defer b.Hub.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Node != b.Node {
				continue
			}
			body, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_ = b.Client.Publish(ctx, b.Channel, body).Err()
		}
	}
}

func (b *Bridge) runSubscriber(ctx context.Context) {
	defer b.wg.Done()
	ps := b.Client.Subscribe(ctx, b.Channel)
	defer ps.Close()
	msgs := ps.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-msgs:
			if !ok {
				return
			}
			var ev Event
			if err := json.Unmarshal([]byte(m.Payload), &ev); err != nil {
				continue
			}
			if ev.Node == b.Node {
				continue
			}
			key := ev.Node + ":" + strconv.FormatInt(ev.Seq, 10)
			if !b.dedup.add(key) {
				continue
			}
			b.Hub.Publish(ev)
		}
	}
}

// lru is a tiny FIFO-with-set hybrid - O(1) add, drops the oldest entry on
// overflow. Not thread-safe; only used by runSubscriber.
type lru struct {
	cap   int
	order []string
	set   map[string]struct{}
}

func newLRU(cap int) *lru {
	return &lru{cap: cap, order: make([]string, 0, cap), set: make(map[string]struct{}, cap)}
}

// add returns true if the key was new (and was inserted), false if it was
// already present.
func (l *lru) add(key string) bool {
	if _, ok := l.set[key]; ok {
		return false
	}
	if len(l.order) >= l.cap {
		old := l.order[0]
		l.order = l.order[1:]
		delete(l.set, old)
	}
	l.order = append(l.order, key)
	l.set[key] = struct{}{}
	return true
}
