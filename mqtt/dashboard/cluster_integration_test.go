//go:build integration
// +build integration

// mqtt/dashboard/cluster_integration_test.go
package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/handlers"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/sse"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// node bundles a fake "broker A" or "broker B" - one mqtt.Server, one
// hub, one bridge, one events handler exposed via httptest.
type node struct {
	name   string
	server *mqtt.Server
	hub    *sse.Hub
	bridge *sse.Bridge
	srv    *httptest.Server
}

func newNode(t *testing.T, name string, redisAddr string) *node {
	t.Helper()
	server := mqtt.New(nil)
	hub := sse.NewHub(64)

	if err := server.AddHook(&sse.HubHook{Hub: hub, Node: name}, nil); err != nil {
		t.Fatalf("AddHook: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = client.Close() })

	br := sse.NewBridge(client, hub, name)
	br.Start(context.Background())

	srv := httptest.NewServer(handlers.Events(hub))

	t.Cleanup(func() {
		srv.Close()
		br.Stop()
		hub.Close()
		_ = server.Close()
	})

	return &node{name: name, server: server, hub: hub, bridge: br, srv: srv}
}

// readSSEUntil reads `event:` lines from the SSE stream until one matches
// the wanted type with the wanted node, or the deadline elapses. Returns
// the matched parsed event or an error.
func readSSEUntil(t *testing.T, url string, wantType, wantNode string, timeout time.Duration) (sse.Event, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"?as=json", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sse.Event{}, err
	}
	defer resp.Body.Close()

	buf := make([]byte, 1024)
	var sb strings.Builder
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			body := sb.String()
			records := strings.Split(body, "\n\n")
			sb.Reset()
			sb.WriteString(records[len(records)-1])
			for _, rec := range records[:len(records)-1] {
				rec = strings.TrimSpace(rec)
				if rec == "" || strings.HasPrefix(rec, ":") {
					continue
				}
				var typeLine, dataLine string
				for _, line := range strings.Split(rec, "\n") {
					if strings.HasPrefix(line, "event: ") {
						typeLine = strings.TrimPrefix(line, "event: ")
					} else if strings.HasPrefix(line, "data: ") {
						dataLine = strings.TrimPrefix(line, "data: ")
					}
				}
				if typeLine != wantType {
					continue
				}
				var ev sse.Event
				if err := json.Unmarshal([]byte(dataLine), &ev); err != nil {
					continue
				}
				if ev.Type == wantType && ev.Node == wantNode {
					return ev, nil
				}
			}
		}
		if err != nil {
			return sse.Event{}, err
		}
	}
}

// TestClusterEventAggregation simulates two cluster nodes sharing events
// via miniredis. A connect event triggered on node-A is observed on
// node-B's SSE stream.
func TestClusterEventAggregation(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()

	a := newNode(t, "node-A", addr)
	b := newNode(t, "node-B", addr)

	// Allow both bridges' subscribers to be registered with redis before we
	// publish. miniredis is fast but PSubscribe still needs a moment to round
	// trip. 150ms is plenty.
	time.Sleep(150 * time.Millisecond)

	cl := &mqtt.Client{ID: "alice"}
	cl.Properties.Username = []byte("u")
	cl.Net.Remote = "127.0.0.1:0"

	hubHook := &sse.HubHook{Hub: a.hub, Node: "node-A"}
	if err := hubHook.OnConnect(cl, packets.Packet{}); err != nil {
		t.Fatalf("OnConnect: %v", err)
	}

	ev, err := readSSEUntil(t, b.srv.URL, "client.connected", "node-A", 5*time.Second)
	if err != nil {
		t.Fatalf("readSSEUntil: %v", err)
	}
	if ev.Node != "node-A" {
		t.Fatalf("expected node=node-A, got %q", ev.Node)
	}
	if ev.Type != "client.connected" {
		t.Fatalf("expected type=client.connected, got %q", ev.Type)
	}
}

// TestClusterEventAggregationOwnNodeFiltered verifies that a tab connected
// to node-A doesn't see its own events DUPLICATED via the redis round trip.
func TestClusterEventAggregationOwnNodeFiltered(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	a := newNode(t, "node-A", addr)

	time.Sleep(100 * time.Millisecond)

	hubHook := &sse.HubHook{Hub: a.hub, Node: "node-A"}
	cl := &mqtt.Client{ID: "alice"}
	cl.Properties.Username = []byte("u")
	cl.Net.Remote = "127.0.0.1:0"

	// The hub is fan-out and does not buffer for not-yet-connected subscribers,
	// so we must open the SSE stream before triggering events.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.srv.URL+"?as=json", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	// Allow the SSE handler to subscribe to the hub before publishing.
	time.Sleep(150 * time.Millisecond)

	for i := 0; i < 5; i++ {
		if err := hubHook.OnConnect(cl, packets.Packet{}); err != nil {
			t.Fatalf("OnConnect: %v", err)
		}
	}

	count := 0
	deadline := time.After(2 * time.Second)
	buf := make([]byte, 1024)
	var sb strings.Builder
DrainLoop:
	for {
		select {
		case <-deadline:
			break DrainLoop
		default:
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			count = strings.Count(sb.String(), "event: client.connected")
			if count >= 5 {
				time.Sleep(500 * time.Millisecond)
				n, _ = resp.Body.Read(buf)
				if n > 0 {
					sb.Write(buf[:n])
					count = strings.Count(sb.String(), "event: client.connected")
				}
				break DrainLoop
			}
		}
		if err != nil {
			break
		}
	}
	if count != 5 {
		t.Fatalf("expected exactly 5 client.connected events, got %d", count)
	}
}
