// mqtt/rest/subscriptions_list_test.go
package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// addFakeClientWithSubs inserts a minimal *mqtt.Client into the broker and
// attaches the given topic filters as QoS 1 subscriptions.
func addFakeClientWithSubs(t *testing.T, server *mqtt.Server, id string, filters ...string) {
	t.Helper()
	cl := &mqtt.Client{ID: id}
	cl.Properties.Username = []byte("u")
	cl.Net.Remote = "127.0.0.1:0"
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	for _, f := range filters {
		cl.State.Subscriptions.Add(f, packets.Subscription{Filter: f, Qos: 1})
	}
	server.Clients.Add(cl)
}

func TestListSubscriptionsEmpty(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listSubscriptions(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/subscriptions", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	var got Page[subscriptionSummary]
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("expected empty: %+v", got)
	}
	if got.Page != 1 || got.Size != 25 {
		t.Fatalf("defaults: %+v", got)
	}
}

func TestListSubscriptionsBasic(t *testing.T) {
	server := mqtt.New(nil)
	addFakeClientWithSubs(t, server, "client-a", "a/1", "a/2")
	addFakeClientWithSubs(t, server, "client-b", "b/1", "b/2")
	addFakeClientWithSubs(t, server, "client-c", "c/1", "c/2")
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listSubscriptions(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/subscriptions", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	var got Page[subscriptionSummary]
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 6 {
		t.Fatalf("total: got %d want 6", got.Total)
	}
	if len(got.Items) != 6 {
		t.Fatalf("items: got %d want 6", len(got.Items))
	}
	for _, it := range got.Items {
		if it.QoS != 1 {
			t.Fatalf("qos: got %d want 1 for %+v", it.QoS, it)
		}
	}
}

func TestListSubscriptionsTopicFilter(t *testing.T) {
	server := mqtt.New(nil)
	addFakeClientWithSubs(t, server, "client-a", "sensors/temp", "sensors/humidity")
	addFakeClientWithSubs(t, server, "client-b", "sensors/temp/room1", "alerts/fire")
	addFakeClientWithSubs(t, server, "client-c", "alerts/flood")
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listSubscriptions(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/subscriptions?topic=temp", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	var got Page[subscriptionSummary]
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 2 {
		t.Fatalf("total: got %d want 2", got.Total)
	}
	for _, it := range got.Items {
		if it.Topic != "sensors/temp" && it.Topic != "sensors/temp/room1" {
			t.Fatalf("unexpected topic in filtered result: %s", it.Topic)
		}
	}
}
