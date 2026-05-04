// mqtt/rest/topics_tree_test.go
package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func TestTopicsTreeShape(t *testing.T) {
	server := mqtt.New(nil)

	subscribe := func(id, filter string) {
		cl := &mqtt.Client{ID: id}
		cl.Properties.Username = []byte("u")
		cl.Net.Remote = "127.0.0.1:0"
		cl.State.Subscriptions = mqtt.NewSubscriptions()
		cl.State.Inflight = mqtt.NewInflights()
		cl.State.Subscriptions.Add(filter, packets.Subscription{Filter: filter, Qos: 0})
		server.Clients.Add(cl)
	}
	subscribe("alice", "sensors/temp/room1")
	subscribe("bob", "sensors/temp/room2")
	subscribe("alice2", "sensors/humidity/+")

	rest := New(server)
	rr := httptest.NewRecorder()
	rest.topicsTree(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/topics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	var root topicNode
	if err := json.NewDecoder(rr.Body).Decode(&root); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if root.Topic != "" {
		t.Fatalf("root.Topic should be empty: %q", root.Topic)
	}
	if len(root.Children) != 1 || root.Children[0].Topic != "sensors" {
		t.Fatalf("expected sensors child, got %+v", root.Children)
	}
	sensors := root.Children[0]
	if len(sensors.Children) != 2 {
		t.Fatalf("expected 2 children under sensors, got %d", len(sensors.Children))
	}
	// children should be sorted: humidity before temp
	if sensors.Children[0].Topic != "humidity" || sensors.Children[1].Topic != "temp" {
		t.Fatalf("not sorted: %v", []string{sensors.Children[0].Topic, sensors.Children[1].Topic})
	}
}

func TestTopicsTreeEmpty(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)
	rr := httptest.NewRecorder()
	rest.topicsTree(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/topics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	var root topicNode
	_ = json.NewDecoder(rr.Body).Decode(&root)
	if len(root.Children) != 0 {
		t.Fatalf("expected empty tree, got %+v", root)
	}
}
