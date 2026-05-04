// mqtt/dashboard/handlers/events_test.go
package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wind-c/comqtt/v2/mqtt/dashboard/sse"
)

func TestEventsStreamsPublishedEvents(t *testing.T) {
	hub := sse.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(Events(hub))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type: %q", got)
	}

	time.Sleep(75 * time.Millisecond)
	hub.Publish(sse.Event{Type: "client.connected", Node: "n1", TS: 1})
	hub.Publish(sse.Event{Type: "message.published", Node: "n1", TS: 2})

	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
				if strings.Contains(sb.String(), "event: client.connected") &&
					strings.Contains(sb.String(), "event: message.published") {
					done <- sb.String()
					return
				}
			}
			if err != nil {
				done <- sb.String()
				return
			}
		}
	}()

	select {
	case got := <-done:
		if !strings.Contains(got, "event: client.connected") {
			t.Fatalf("missing client.connected: %q", got)
		}
		if !strings.Contains(got, "event: message.published") {
			t.Fatalf("missing message.published: %q", got)
		}
		if !strings.Contains(got, "id: ") {
			t.Fatalf("missing id field: %q", got)
		}
		if !strings.Contains(got, "data: {") {
			t.Fatalf("missing data field with JSON: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SSE events")
	}
}

func TestEventsClosesOnContextCancel(t *testing.T) {
	hub := sse.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(Events(hub))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
