// mqtt/dashboard/sse/hub_test.go
package sse

import (
	"testing"
	"time"
)

func TestHubFanOut(t *testing.T) {
	h := NewHub(8)
	defer h.Close()
	a := h.Subscribe()
	b := h.Subscribe()
	h.Publish(Event{Type: "client.connected", Node: "n1", TS: 1})
	for _, ch := range []<-chan Event{a, b} {
		select {
		case ev := <-ch:
			if ev.Type != "client.connected" {
				t.Fatalf("type: %q", ev.Type)
			}
			if ev.Seq != 1 {
				t.Fatalf("seq should be set to 1: %d", ev.Seq)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	}
}

func TestHubDropsSlowSubscriber(t *testing.T) {
	h := NewHub(2)
	defer h.Close()
	slow := h.Subscribe()
	for i := 0; i < 10; i++ {
		h.Publish(Event{Type: "x", TS: int64(i)})
	}
	if h.Drops() == 0 {
		t.Fatal("expected drops > 0 from a never-drained slow subscriber")
	}
	_ = slow
}

func TestHubSeqIsMonotonic(t *testing.T) {
	h := NewHub(16)
	defer h.Close()
	ch := h.Subscribe()
	for i := 0; i < 5; i++ {
		h.Publish(Event{Type: "x", TS: int64(i)})
	}
	var last int64
	for i := 0; i < 5; i++ {
		select {
		case ev := <-ch:
			if ev.Seq <= last {
				t.Fatalf("seq not monotonic: %d after %d", ev.Seq, last)
			}
			last = ev.Seq
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub(4)
	defer h.Close()
	ch := h.Subscribe()
	h.Unsubscribe(ch)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel closed after Unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout - channel should be closed")
	}
}

func TestHubCloseIsIdempotent(t *testing.T) {
	h := NewHub(4)
	h.Close()
	h.Close()
	h.Publish(Event{Type: "after-close", TS: 1})
}
