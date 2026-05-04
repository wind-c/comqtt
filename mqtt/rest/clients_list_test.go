// mqtt/rest/clients_list_test.go
package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wind-c/comqtt/v2/mqtt"
)

// addFakeClient inserts a minimally-populated *mqtt.Client into the broker's
// Clients map. The fields populated are exactly the ones listClients reads.
func addFakeClient(t *testing.T, server *mqtt.Server, id, username string) {
	t.Helper()
	cl := &mqtt.Client{ID: id}
	cl.Properties.Username = []byte(username)
	cl.Net.Remote = "127.0.0.1:0"
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	server.Clients.Add(cl)
}

func TestListClientsEmpty(t *testing.T) {
	server := mqtt.New(nil)
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listClients(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/clients", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	var got Page[clientSummary]
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

func TestListClientsPagination(t *testing.T) {
	server := mqtt.New(nil)
	for i := 0; i < 60; i++ {
		addFakeClient(t, server, fmtID(i), "u")
	}
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listClients(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/clients?page=2&size=20", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	var got Page[clientSummary]
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 60 {
		t.Fatalf("total: got %d want 60", got.Total)
	}
	if len(got.Items) != 20 {
		t.Fatalf("items: got %d want 20", len(got.Items))
	}
}

func TestListClientsPrefixSearch(t *testing.T) {
	server := mqtt.New(nil)
	addFakeClient(t, server, "alpha-1", "u")
	addFakeClient(t, server, "alpha-2", "u")
	addFakeClient(t, server, "bravo-1", "u")
	rest := New(server)

	rr := httptest.NewRecorder()
	rest.listClients(rr, httptest.NewRequest(http.MethodGet, "/api/v1/mqtt/clients?q=alpha", nil))
	var got Page[clientSummary]
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Total != 2 {
		t.Fatalf("expected 2 alpha clients, got %d", got.Total)
	}
}

// fmtID formats a deterministic ID like "client-007".
func fmtID(i int) string {
	return "client-" + leftPad(i)
}

func leftPad(i int) string {
	s := ""
	if i < 10 {
		s = "00"
	} else if i < 100 {
		s = "0"
	}
	return s + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
