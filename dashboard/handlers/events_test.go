// dashboard/handlers/events_test.go
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wind-c/comqtt/v2/dashboard/sse"
)

func TestEventsStreamsJSONWithAsParam(t *testing.T) {
	hub := sse.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(Events(hub))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?as=json", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	time.Sleep(75 * time.Millisecond)
	hub.Publish(sse.Event{Type: "client.connected", Node: "n1", TS: 1, Payload: json.RawMessage(`{"client_id":"alice"}`)})

	got := readUntil(t, resp.Body, "event: client.connected", 2*time.Second)
	if !strings.Contains(got, "data: {") {
		t.Fatalf("expected JSON data: %q", got)
	}
}

func TestEventsStreamsHTMLByDefault(t *testing.T) {
	hub := sse.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(Events(hub))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	time.Sleep(75 * time.Millisecond)
	hub.Publish(sse.Event{Type: "client.connected", Node: "n1", TS: 1, Payload: json.RawMessage(`{"client_id":"alice","remote":"127.0.0.1:0"}`)})

	got := readUntil(t, resp.Body, "<li", 2*time.Second)
	if !strings.Contains(got, "<li") {
		t.Fatalf("expected <li> fragment: %q", got)
	}
	if !strings.Contains(got, "alice connected") {
		t.Fatalf("expected headline: %q", got)
	}
	if strings.Count(got, "\n\n") < 1 {
		t.Fatalf("expected SSE record terminator: %q", got)
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

func TestRenderEventHTMLEscapes(t *testing.T) {
	ev := sse.Event{
		Type:    "client.connected",
		Payload: json.RawMessage(`{"client_id":"<script>","remote":"x"}`),
	}
	got := renderEventHTML(ev)
	if strings.Contains(got, "<script>") {
		t.Fatalf("html.EscapeString should have escaped the angle brackets: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected escaped form: %q", got)
	}
}

// readUntil reads from r until needle appears or timeout fires; returns the
// accumulated body (whether or not it matched).
func readUntil(t *testing.T, r io.Reader, needle string, timeout time.Duration) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
				if strings.Contains(sb.String(), needle) {
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
	case s := <-done:
		return s
	case <-time.After(timeout):
		return ""
	}
}
