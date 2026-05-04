// mqtt/dashboard/sse/bridge_redis_test.go
package sse

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRedisForBridge(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func TestBridgeForwardsLocalEventsToRedis(t *testing.T) {
	client, _ := newRedisForBridge(t)
	hub := NewHub(8)
	defer hub.Close()
	br := NewBridge(client, hub, "node-A")
	br.Start(context.Background())
	defer br.Stop()

	ps := client.Subscribe(context.Background(), defaultEventsChannel)
	defer ps.Close()
	msgs := ps.Channel()

	time.Sleep(50 * time.Millisecond)

	hub.Publish(Event{Type: "client.connected", Node: "node-A", TS: 1})

	select {
	case m := <-msgs:
		var ev Event
		if err := json.Unmarshal([]byte(m.Payload), &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.Type != "client.connected" {
			t.Fatalf("type: %q", ev.Type)
		}
		if ev.Node != "node-A" {
			t.Fatalf("node: %q", ev.Node)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for forwarded event")
	}
}

func TestBridgeReceivesPeerEventIntoLocalHub(t *testing.T) {
	client, _ := newRedisForBridge(t)
	hub := NewHub(8)
	defer hub.Close()
	br := NewBridge(client, hub, "node-A")
	br.Start(context.Background())
	defer br.Stop()

	sub := hub.Subscribe()

	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(Event{Type: "message.published", Node: "node-B", Seq: 42, TS: 1})
	if err := client.Publish(context.Background(), defaultEventsChannel, body).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case ev := <-sub:
		if ev.Node != "node-B" {
			t.Fatalf("expected node-B event, got %q", ev.Node)
		}
		if ev.Type != "message.published" {
			t.Fatalf("type: %q", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for peer event")
	}
}

func TestBridgeDedupsRepeatedEvents(t *testing.T) {
	client, _ := newRedisForBridge(t)
	hub := NewHub(8)
	defer hub.Close()
	br := NewBridge(client, hub, "node-A")
	br.Start(context.Background())
	defer br.Stop()

	sub := hub.Subscribe()

	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(Event{Type: "message.published", Node: "node-B", Seq: 7, TS: 1})
	_ = client.Publish(context.Background(), defaultEventsChannel, body).Err()
	_ = client.Publish(context.Background(), defaultEventsChannel, body).Err()

	count := 0
	deadline := time.After(1 * time.Second)
DrainLoop:
	for {
		select {
		case ev := <-sub:
			if ev.Node == "node-B" {
				count++
			}
		case <-deadline:
			break DrainLoop
		}
	}

	if count != 1 {
		t.Fatalf("expected 1 deduplicated event, got %d", count)
	}
}

func TestBridgeStopReleasesGoroutines(t *testing.T) {
	client, _ := newRedisForBridge(t)
	hub := NewHub(8)
	defer hub.Close()
	br := NewBridge(client, hub, "node-A")
	br.Start(context.Background())
	br.Stop()
	br.Stop()
}

func TestLRUDropsOldestOnOverflow(t *testing.T) {
	l := newLRU(3)
	for _, k := range []string{"a", "b", "c"} {
		if !l.add(k) {
			t.Fatalf("expected %q to be new", k)
		}
	}
	if l.add("a") {
		t.Fatal("expected a to be deduped")
	}
	if !l.add("d") {
		t.Fatal("expected d to be new")
	}
	if !l.add("a") {
		t.Fatal("after eviction, a should be new again")
	}
}

func TestBridgeIgnoresOwnNodeEchoes(t *testing.T) {
	client, _ := newRedisForBridge(t)
	hub := NewHub(8)
	defer hub.Close()
	br := NewBridge(client, hub, "node-A")
	br.Start(context.Background())
	defer br.Stop()

	sub := hub.Subscribe()

	time.Sleep(50 * time.Millisecond)

	body, _ := json.Marshal(Event{Type: "client.connected", Node: "node-A", Seq: 99, TS: 1})
	_ = client.Publish(context.Background(), defaultEventsChannel, body).Err()

	select {
	case ev := <-sub:
		t.Fatalf("did not expect own-node event back from redis: %+v", ev)
	case <-time.After(500 * time.Millisecond):
	}

	_ = sync.WaitGroup{}
}
