// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	mqtt "github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func newTestServer(t *testing.T) *mqtt.Server {
	t.Helper()
	server := mqtt.New(nil)
	require.NotNil(t, server)
	return server
}

func newRestServer(t *testing.T) (*Rest, *mqtt.Server) {
	t.Helper()
	server := newTestServer(t)
	return New(server), server
}

func TestGetClientsEmpty(t *testing.T) {
	r, _ := newRestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/clients", nil)
	w := httptest.NewRecorder()

	r.getClients(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp PagedResponse[client]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Total)
	require.Len(t, resp.Items, 0)
}

func TestGetClientsWithData(t *testing.T) {
	_, server := newRestServer(t)

	cl := server.NewClient(nil, "tcp", "client-1", false)
	cl.Net.Remote = "10.0.0.1:1234"
	cl.Properties.Username = []byte("user1")
	server.Clients.Add(cl)

	cl2 := server.NewClient(nil, "tcp", "client-2", false)
	cl2.Net.Remote = "10.0.0.2:1234"
	cl2.Properties.Username = []byte("user2")
	server.Clients.Add(cl2)

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/clients", nil)
	w := httptest.NewRecorder()

	r.getClients(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp PagedResponse[client]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Total)
	require.Len(t, resp.Items, 2)
}

func TestGetClientsWithSearch(t *testing.T) {
	_, server := newRestServer(t)

	cl := server.NewClient(nil, "tcp", "alpha-device", false)
	server.Clients.Add(cl)
	cl2 := server.NewClient(nil, "tcp", "beta-sensor", false)
	server.Clients.Add(cl2)

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/clients?search=alpha", nil)
	w := httptest.NewRecorder()

	r.getClients(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp PagedResponse[client]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Total)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "alpha-device", resp.Items[0].ID)
}

func TestGetSubscriptions(t *testing.T) {
	_, server := newRestServer(t)

	cl := server.NewClient(nil, "tcp", "client-1", false)
	cl.State.Subscriptions.Add("sensors/#", packets.Subscription{Filter: "sensors/#", Qos: 1})
	cl.State.Subscriptions.Add("devices/+/status", packets.Subscription{Filter: "devices/+/status", Qos: 2})
	server.Clients.Add(cl)

	cl2 := server.NewClient(nil, "tcp", "client-2", false)
	cl2.State.Subscriptions.Add("sensors/#", packets.Subscription{Filter: "sensors/#", Qos: 0})
	cl2.State.Subscriptions.Add("alerts", packets.Subscription{Filter: "alerts", Qos: 2})
	server.Clients.Add(cl2)

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/subscriptions", nil)
	w := httptest.NewRecorder()

	r.getSubscriptions(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp []subscriptionInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp, 3)

	counts := make(map[string]int)
	for _, s := range resp {
		counts[s.Filter] = s.Count
	}
	require.Equal(t, 2, counts["sensors/#"])
	require.Equal(t, 1, counts["devices/+/status"])
	require.Equal(t, 1, counts["alerts"])
}

func TestGetTopics(t *testing.T) {
	_, server := newRestServer(t)

	server.Topics.Subscribe("cl1", packets.Subscription{Filter: "a/b/c", Qos: 1})
	server.Topics.Subscribe("cl2", packets.Subscription{Filter: "a/b/c", Qos: 0})

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/topics", nil)
	w := httptest.NewRecorder()

	r.getTopics(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var tree mqtt.TopicTreeNode
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tree))
	require.Equal(t, 2, tree.SubscriberCount)
	require.NotEmpty(t, tree.Children)
}

func TestGetRetained(t *testing.T) {
	_, server := newRestServer(t)

	server.Topics.RetainMessage(packets.Packet{
		TopicName:   "devices/status",
		Payload:     []byte(`{"online":true}`),
		FixedHeader: packets.FixedHeader{Retain: true},
	})
	server.Topics.RetainMessage(packets.Packet{
		TopicName:   "system/version",
		Payload:     []byte("2.4.0"),
		FixedHeader: packets.FixedHeader{Retain: true},
	})

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/retained", nil)
	w := httptest.NewRecorder()

	r.getRetained(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp PagedResponse[RetainedMsg]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Total)
	require.Len(t, resp.Items, 2)
}

func TestDeleteRetained(t *testing.T) {
	_, server := newRestServer(t)

	server.Topics.RetainMessage(packets.Packet{
		TopicName:   "devices/old",
		Payload:     []byte("stale"),
		FixedHeader: packets.FixedHeader{Retain: true},
	})

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/mqtt/retained/devices/old", nil)
	req.SetPathValue("topic", "devices/old")
	w := httptest.NewRecorder()

	r.deleteRetained(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	_, ok := server.Topics.Retained.Get("devices/old")
	require.False(t, ok)
}

func TestGetClientsParsesQueryParams(t *testing.T) {
	_, server := newRestServer(t)

	for i := range 5 {
		cl := server.NewClient(nil, "tcp", "client-"+string(rune('0'+i)), false)
		server.Clients.Add(cl)
	}

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/clients?page=2&page_size=2", nil)
	w := httptest.NewRecorder()

	r.getClients(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp PagedResponse[client]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 5, resp.Total)
	require.Equal(t, 2, resp.Page)
	require.Equal(t, 2, resp.PageSize)
	require.Len(t, resp.Items, 2)
}

func TestDisconnectClient(t *testing.T) {
	_, server := newRestServer(t)

	cl := server.NewClient(nil, "tcp", "client-x", false)
	server.Clients.Add(cl)

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mqtt/clients/client-x/disconnect", nil)
	req.SetPathValue("id", "client-x")
	w := httptest.NewRecorder()

	r.disconnectClient(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestDisconnectClientNotFound(t *testing.T) {
	_, server := newRestServer(t)

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mqtt/clients/nope/disconnect", nil)
	req.SetPathValue("id", "nope")
	w := httptest.NewRecorder()

	r.disconnectClient(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteRetainedNotFound(t *testing.T) {
	_, server := newRestServer(t)

	r := &Rest{server: server}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/mqtt/retained/no/topic", nil)
	req.SetPathValue("topic", "no/topic")
	w := httptest.NewRecorder()

	r.deleteRetained(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
