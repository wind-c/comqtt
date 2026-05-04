// mqtt/rest/retained_test.go
package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func addRetained(t *testing.T, server *mqtt.Server, topic string, payload []byte, qos byte) {
	t.Helper()
	pk := packets.Packet{
		TopicName:   topic,
		Payload:     payload,
		FixedHeader: packets.FixedHeader{Qos: qos, Retain: true},
		Created:     12345,
	}
	server.Topics.Retained.Add(topic, pk)
}

func TestListRetainedEmpty(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)
	rr := httptest.NewRecorder()
	rest.listRetained(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/retained", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	var got Page[retainedSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("expected empty: %+v", got)
	}
}

func TestListRetainedBasic(t *testing.T) {
	server := mqtt.New(nil)
	addRetained(t, server, "sensors/temp/room1", []byte("21.5"), 0)
	addRetained(t, server, "sensors/temp/room2", []byte("22.0"), 1)
	addRetained(t, server, "metrics/heap", []byte("100"), 0)
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listRetained(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/retained", nil))
	var got Page[retainedSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 3 {
		t.Fatalf("total: %d", got.Total)
	}
	for _, item := range got.Items {
		if len(item.Payload) != 0 {
			t.Fatalf("payload should be omitted by default: %s -> %q", item.Topic, item.Payload)
		}
		if item.Size <= 0 {
			t.Fatalf("size should be set: %+v", item)
		}
		if item.StoredAt != 12345 {
			t.Fatalf("stored_at not propagated: %+v", item)
		}
	}
}

func TestListRetainedTopicFilter(t *testing.T) {
	server := mqtt.New(nil)
	addRetained(t, server, "sensors/temp/room1", []byte("21.5"), 0)
	addRetained(t, server, "metrics/heap", []byte("100"), 0)
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listRetained(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/retained?topic=sensors", nil))
	var got Page[retainedSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 1 || got.Items[0].Topic != "sensors/temp/room1" {
		t.Fatalf("filter mismatch: %+v", got)
	}
}

func TestListRetainedPayloadIncludedAndCapped(t *testing.T) {
	server := mqtt.New(nil)
	big := make([]byte, 8192)
	for i := range big {
		big[i] = byte(i)
	}
	addRetained(t, server, "big/topic", big, 0)
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listRetained(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/retained?payload=true", nil))
	var got Page[retainedSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 1 {
		t.Fatalf("total: %d", got.Total)
	}
	if len(got.Items[0].Payload) != 4096 {
		t.Fatalf("payload should be capped at 4096, got %d", len(got.Items[0].Payload))
	}
	if got.Items[0].Size != 8192 {
		t.Fatalf("size should be the original length: %d", got.Items[0].Size)
	}
}

func TestClearRetainedHappy(t *testing.T) {
	server := mqtt.New(nil)
	addRetained(t, server, "sensors/temp", []byte("21.5"), 0)
	rest := New(server)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/x", nil)
	req.SetPathValue("topic", "sensors/temp")
	rest.clearRetained(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if _, ok := server.Topics.Retained.Get("sensors/temp"); ok {
		t.Fatal("retained should be cleared")
	}
}

func TestClearRetainedNotFound(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/x", nil)
	req.SetPathValue("topic", "ghost")
	rest.clearRetained(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: %d", rr.Code)
	}
}
