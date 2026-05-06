// dashboard/handlers/client_detail_test.go
package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/dashboard/auth"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func newClientDetailRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/clients/detail.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "client_detail"}}{{template "layout" .}}{{end}}{{define "content"}}<div>id={{.ClientID}} online={{.Online}} tab={{.Tab}} subcount={{.SubCount}} readonly={{.Readonly}}</div>{{if eq .Tab "info"}}<div class="info">user={{.Info.Username}} remote={{.Info.Remote}} kal={{.Info.Keepalive}}</div>{{end}}{{if eq .Tab "subs"}}{{range .Subs}}<div class="sub">{{.Topic}}|{{.TopicEncoded}}|qos{{.QoS}}</div>{{else}}<div class="empty">no subs</div>{{end}}{{end}}{{end}}`)},
	}))
}

func newClientDetailDeps(t *testing.T) ClientDetailDeps {
	t.Helper()
	return ClientDetailDeps{
		Server:   mqtt.New(nil),
		Renderer: newClientDetailRenderer(t),
	}
}

func addClientForDetail(t *testing.T, server *mqtt.Server, id string, filters ...string) {
	t.Helper()
	cl := &mqtt.Client{ID: id}
	cl.Properties.Username = []byte("u")
	cl.Properties.ProtocolVersion = 5
	cl.Properties.Clean = false
	cl.Net.Remote = "127.0.0.1:0"
	cl.Net.Listener = "tcp"
	cl.State.Keepalive = 60
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	for _, f := range filters {
		cl.State.Subscriptions.Add(f, packets.Subscription{Filter: f, Qos: 1})
		server.Topics.Subscribe(id, packets.Subscription{Filter: f, Qos: 1})
	}
	server.Clients.Add(cl)
}

func TestClientDetailNotFound(t *testing.T) {
	deps := newClientDetailDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/clients/ghost", nil)
	req.SetPathValue("id", "ghost")
	rr := httptest.NewRecorder()
	ClientDetail(deps)(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestClientDetailInfoTab(t *testing.T) {
	deps := newClientDetailDeps(t)
	addClientForDetail(t, deps.Server, "alice")
	req := httptest.NewRequest(http.MethodGet, "/dashboard/clients/alice", nil)
	req.SetPathValue("id", "alice")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ClientDetail(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "id=alice") {
		t.Fatalf("missing id: %q", body)
	}
	if !strings.Contains(body, "online=true") {
		t.Fatalf("expected online=true: %q", body)
	}
	if !strings.Contains(body, "tab=info") {
		t.Fatalf("expected default info tab: %q", body)
	}
	if !strings.Contains(body, "user=u") {
		t.Fatalf("expected username: %q", body)
	}
}

func TestClientDetailSubsTab(t *testing.T) {
	deps := newClientDetailDeps(t)
	addClientForDetail(t, deps.Server, "alice", "sensors/temp", "sensors/+/room1")
	req := httptest.NewRequest(http.MethodGet, "/dashboard/clients/alice?tab=subs", nil)
	req.SetPathValue("id", "alice")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ClientDetail(deps)(rr, req)
	body := rr.Body.String()
	if !strings.Contains(body, "subcount=2") {
		t.Fatalf("expected 2 subs: %q", body)
	}
	if !strings.Contains(body, "sensors/temp|sensors%2Ftemp") {
		t.Fatalf("expected escaped topic: %q", body)
	}
	// html/template escapes '+' as &#43; in element content; the encoded
	// path segment is what the browser receives.
	if !strings.Contains(body, "sensors/&#43;/room1|sensors%2F&#43;%2Froom1") {
		t.Fatalf("expected escaped + topic: %q", body)
	}
}

func TestClientDetailReadonlyForViewer(t *testing.T) {
	deps := newClientDetailDeps(t)
	addClientForDetail(t, deps.Server, "alice")
	req := httptest.NewRequest(http.MethodGet, "/dashboard/clients/alice?tab=subs", nil)
	req.SetPathValue("id", "alice")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	rr := httptest.NewRecorder()
	ClientDetail(deps)(rr, req)
	if !strings.Contains(rr.Body.String(), "readonly=true") {
		t.Fatalf("expected readonly: %q", rr.Body.String())
	}
}

func TestClientUnsubscribeAdminHappy(t *testing.T) {
	deps := newClientDetailDeps(t)
	addClientForDetail(t, deps.Server, "alice", "sensors/temp")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/clients/alice/subscriptions/sensors%2Ftemp/delete", nil)
	req.SetPathValue("id", "alice")
	req.SetPathValue("topic", "sensors%2Ftemp")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ClientUnsubscribe(deps)(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	cl, _ := deps.Server.Clients.Get("alice")
	if _, ok := cl.State.Subscriptions.Get("sensors/temp"); ok {
		t.Fatal("subscription should be cleared")
	}
}

func TestClientUnsubscribeViewerForbidden(t *testing.T) {
	deps := newClientDetailDeps(t)
	addClientForDetail(t, deps.Server, "alice", "sensors/temp")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/clients/alice/subscriptions/sensors%2Ftemp/delete", nil)
	req.SetPathValue("id", "alice")
	req.SetPathValue("topic", "sensors%2Ftemp")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	rr := httptest.NewRecorder()
	ClientUnsubscribe(deps)(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: %d", rr.Code)
	}
}
