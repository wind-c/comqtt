// mqtt/rest/sessions_test.go
package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wind-c/comqtt/v2/mqtt"
)

func addOnlineSession(t *testing.T, server *mqtt.Server, id, username string) *mqtt.Client {
	t.Helper()
	cl := &mqtt.Client{ID: id}
	cl.Properties.Username = []byte(username)
	cl.Net.Remote = "127.0.0.1:0"
	cl.Net.Listener = "test-listener"
	cl.Properties.ProtocolVersion = 5
	cl.Properties.Clean = false
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	server.Clients.Add(cl)
	return cl
}

func TestListSessionsEmpty(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)
	rr := httptest.NewRecorder()
	rest.listSessions(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/sessions", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	var got Page[sessionSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("expected empty: %+v", got)
	}
}

func TestListSessionsOnlineOnly(t *testing.T) {
	server := mqtt.New(nil)
	addOnlineSession(t, server, "alice", "u")
	addOnlineSession(t, server, "bob", "u")
	rest := New(server)
	rr := httptest.NewRecorder()
	rest.listSessions(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/sessions", nil))
	var got Page[sessionSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 2 {
		t.Fatalf("total: %d", got.Total)
	}
	for _, item := range got.Items {
		if !item.Online {
			t.Fatalf("expected Online=true: %+v", item)
		}
	}
}

func TestListSessionsOnlineFilter(t *testing.T) {
	server := mqtt.New(nil)
	addOnlineSession(t, server, "alice", "u")
	rest := New(server)
	rr := httptest.NewRecorder()
	rest.listSessions(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/sessions?online=true", nil))
	var got Page[sessionSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 1 {
		t.Fatalf("total: %d", got.Total)
	}
	rr2 := httptest.NewRecorder()
	rest.listSessions(rr2, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/sessions?online=false", nil))
	var got2 Page[sessionSummary]
	_ = json.NewDecoder(rr2.Body).Decode(&got2)
	if got2.Total != 0 {
		t.Fatalf("offline-only with no storage hook should return 0, got %d", got2.Total)
	}
}

func TestClearSessionNotFound(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/x", nil)
	req.SetPathValue("id", "ghost")
	rest.clearSession(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: %d", rr.Code)
	}
}
