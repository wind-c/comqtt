// mqtt/rest/client_unsubscribe_test.go
package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func TestUnsubscribeClientHappyPath(t *testing.T) {
	server := mqtt.New(nil)
	cl := &mqtt.Client{ID: "alice"}
	cl.Properties.Username = []byte("u")
	cl.Net.Remote = "127.0.0.1:0"
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	cl.State.Subscriptions.Add("sensors/temp", packets.Subscription{Filter: "sensors/temp", Qos: 1})
	server.Topics.Subscribe(cl.ID, packets.Subscription{Filter: "sensors/temp", Qos: 1})
	server.Clients.Add(cl)
	rest := New(server)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/mqtt/clients/alice/subscriptions/sensors%2Ftemp", nil)
	req.SetPathValue("id", "alice")
	req.SetPathValue("topic", "sensors/temp")
	rest.unsubscribeClient(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if _, has := cl.State.Subscriptions.Get("sensors/temp"); has {
		t.Fatal("expected per-client subscription to be cleared")
	}
}

func TestUnsubscribeClientNotFound(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/x", nil)
	req.SetPathValue("id", "ghost")
	req.SetPathValue("topic", "anything")
	rest.unsubscribeClient(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestUnsubscribeClientUnknownTopic(t *testing.T) {
	server := mqtt.New(nil)
	cl := &mqtt.Client{ID: "alice"}
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	server.Clients.Add(cl)
	rest := New(server)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/x", nil)
	req.SetPathValue("id", "alice")
	req.SetPathValue("topic", "never/subscribed")
	rest.unsubscribeClient(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: %d", rr.Code)
	}
}
