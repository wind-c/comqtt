// dashboard/handlers/sessions_test.go
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
)

func newSessionsRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/sessions.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "sessions"}}{{template "layout" .}}{{end}}{{define "content"}}<div>total={{.Page.Total}} online={{.Online}} readonly={{.Readonly}}</div>{{range .Page.Items}}<div class="row">{{.ClientID}}|on{{.Online}}|subs{{.Subs}}|inflight{{.Inflight}}</div>{{else}}<div class="empty">no sessions</div>{{end}}{{end}}`)},
	}))
}

func TestSessionsListEmpty(t *testing.T) {
	server := mqtt.New(nil)
	deps := SessionsDeps{Server: server, Renderer: newSessionsRenderer(t)}
	rr := httptest.NewRecorder()
	SessionsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/sessions", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "total=0") {
		t.Fatalf("expected total=0: %q", body)
	}
	if !strings.Contains(body, "no sessions") {
		t.Fatalf("expected empty marker: %q", body)
	}
}

func TestSessionsListPopulatedAndFiltered(t *testing.T) {
	server := mqtt.New(nil)
	addClientWithSubs(t, server, "alpha", "sensors/a")
	addClientWithSubs(t, server, "bravo")
	deps := SessionsDeps{Server: server, Renderer: newSessionsRenderer(t)}

	rr := httptest.NewRecorder()
	SessionsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/sessions", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "total=2") {
		t.Fatalf("expected total=2: %q", body)
	}
	if !strings.Contains(body, "alpha|ontrue|subs1") {
		t.Fatalf("expected alpha session row: %q", body)
	}

	rr2 := httptest.NewRecorder()
	SessionsList(deps)(rr2, httptest.NewRequest(http.MethodGet, "/dashboard/sessions?online=false", nil))
	body2 := rr2.Body.String()
	if !strings.Contains(body2, "total=0") {
		t.Fatalf("offline-only with no storage hook should be 0: %q", body2)
	}
	if !strings.Contains(body2, "online=false") {
		t.Fatalf("expected online=false echoed: %q", body2)
	}
}

func TestSessionsListReadonlyForViewer(t *testing.T) {
	server := mqtt.New(nil)
	deps := SessionsDeps{Server: server, Renderer: newSessionsRenderer(t)}
	req := httptest.NewRequest(http.MethodGet, "/dashboard/sessions", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	rr := httptest.NewRecorder()
	SessionsList(deps)(rr, req)
	if !strings.Contains(rr.Body.String(), "readonly=true") {
		t.Fatalf("expected readonly: %q", rr.Body.String())
	}
}

func TestSessionsClearViewerForbidden(t *testing.T) {
	server := mqtt.New(nil)
	addClientWithSubs(t, server, "alpha")
	deps := SessionsDeps{Server: server, Renderer: newSessionsRenderer(t)}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/sessions/alpha/delete", nil)
	req.SetPathValue("id", "alpha")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	rr := httptest.NewRecorder()
	SessionsClear(deps)(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestSessionsClearNotFound(t *testing.T) {
	server := mqtt.New(nil)
	deps := SessionsDeps{Server: server, Renderer: newSessionsRenderer(t)}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/sessions/ghost/delete", nil)
	req.SetPathValue("id", "ghost")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	SessionsClear(deps)(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: %d", rr.Code)
	}
}
