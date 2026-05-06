// dashboard/handlers/clients_test.go
package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wind-c/comqtt/v2/mqtt"
)

func newClientsRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/clients/list.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "clients_list"}}{{template "layout" .}}{{end}}{{define "content"}}<table>{{range .Page.Items}}<tr><td>{{.ClientID}}</td><td>{{.Username}}</td></tr>{{else}}<tr><td>empty</td></tr>{{end}}</table><div class="meta">total={{.Page.Total}} q={{.Q}} page={{.Page.Page}}/{{.TotalPages}}</div>{{end}}`)},
	}))
}

func addFakeClientForDashboard(t *testing.T, server *mqtt.Server, id string) {
	t.Helper()
	cl := &mqtt.Client{ID: id}
	cl.Properties.Username = []byte("u")
	cl.Net.Remote = "127.0.0.1:0"
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	server.Clients.Add(cl)
}

func TestClientsListEmpty(t *testing.T) {
	server := mqtt.New(nil)
	deps := ClientsDeps{Server: server, Renderer: newClientsRenderer(t)}
	rr := httptest.NewRecorder()
	ClientsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/clients", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "empty") {
		t.Fatalf("expected empty row: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "total=0") {
		t.Fatalf("expected total=0: %q", rr.Body.String())
	}
}

func TestClientsListWithRowsAndPagination(t *testing.T) {
	server := mqtt.New(nil)
	for i := 0; i < 60; i++ {
		addFakeClientForDashboard(t, server, "client-"+itoa(int64(i)))
	}
	deps := ClientsDeps{Server: server, Renderer: newClientsRenderer(t)}
	rr := httptest.NewRecorder()
	ClientsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/clients?page=2&size=20", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "total=60") {
		t.Fatalf("expected total=60: %q", body)
	}
	if !strings.Contains(body, "page=2/3") {
		t.Fatalf("expected page=2/3: %q", body)
	}
}

func TestClientsListSearch(t *testing.T) {
	server := mqtt.New(nil)
	addFakeClientForDashboard(t, server, "alpha-1")
	addFakeClientForDashboard(t, server, "alpha-2")
	addFakeClientForDashboard(t, server, "bravo-1")
	deps := ClientsDeps{Server: server, Renderer: newClientsRenderer(t)}
	rr := httptest.NewRecorder()
	ClientsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/clients?q=alpha", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "total=2") {
		t.Fatalf("expected total=2: %q", body)
	}
	if !strings.Contains(body, "q=alpha") {
		t.Fatalf("expected q=alpha echoed: %q", body)
	}
}

func TestPageQueryPreservesOtherParams(t *testing.T) {
	got := pageQuery(url.Values{"q": {"alpha"}, "size": {"10"}, "page": {"1"}}, 3)
	if !strings.Contains(got, "page=3") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "q=alpha") {
		t.Fatalf("expected q to be preserved: %q", got)
	}
}
