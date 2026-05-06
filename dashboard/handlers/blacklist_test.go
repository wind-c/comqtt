// dashboard/handlers/blacklist_test.go
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
	"github.com/wind-c/comqtt/v2/dashboard/auth"
)

func newBlacklistRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/blacklist.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "blacklist"}}{{template "layout" .}}{{end}}{{define "content"}}<div>readonly={{.Readonly}}</div>{{range .Items}}<div class="row">{{.}}</div>{{else}}<div class="empty">empty</div>{{end}}{{end}}`)},
	}))
}

func newBlacklistDeps(t *testing.T) BlacklistDeps {
	t.Helper()
	return BlacklistDeps{
		Server:   mqtt.New(nil),
		Renderer: newBlacklistRenderer(t),
	}
}

func adminCtx(r *http.Request) *http.Request {
	return r.WithContext(auth.WithUser(r.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
}

func viewerCtx(r *http.Request) *http.Request {
	return r.WithContext(auth.WithUser(r.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
}

func TestBlacklistGetEmpty(t *testing.T) {
	deps := newBlacklistDeps(t)
	rr := httptest.NewRecorder()
	BlacklistGet(deps)(rr, adminCtx(httptest.NewRequest(http.MethodGet, "/dashboard/blacklist", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "empty") {
		t.Fatalf("expected empty: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "readonly=false") {
		t.Fatalf("expected admin readonly=false: %q", rr.Body.String())
	}
}

func TestBlacklistGetViewerReadonly(t *testing.T) {
	deps := newBlacklistDeps(t)
	rr := httptest.NewRecorder()
	BlacklistGet(deps)(rr, viewerCtx(httptest.NewRequest(http.MethodGet, "/dashboard/blacklist", nil)))
	if !strings.Contains(rr.Body.String(), "readonly=true") {
		t.Fatalf("expected viewer readonly=true: %q", rr.Body.String())
	}
}

func TestBlacklistAddAdmin(t *testing.T) {
	deps := newBlacklistDeps(t)
	form := url.Values{"client_id": {"bad-bot"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/blacklist", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	BlacklistAdd(deps)(rr, adminCtx(req))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if len(deps.Server.Blacklist) != 1 || deps.Server.Blacklist[0] != "bad-bot" {
		t.Fatalf("blacklist: %v", deps.Server.Blacklist)
	}
}

func TestBlacklistAddViewerForbidden(t *testing.T) {
	deps := newBlacklistDeps(t)
	form := url.Values{"client_id": {"bad-bot"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/blacklist", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	BlacklistAdd(deps)(rr, viewerCtx(req))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: %d", rr.Code)
	}
	if len(deps.Server.Blacklist) != 0 {
		t.Fatalf("viewer should not mutate blacklist: %v", deps.Server.Blacklist)
	}
}

func TestBlacklistAddIsIdempotent(t *testing.T) {
	deps := newBlacklistDeps(t)
	deps.Server.Blacklist = []string{"existing"}
	form := url.Values{"client_id": {"existing"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/blacklist", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	BlacklistAdd(deps)(rr, adminCtx(req))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}
	if len(deps.Server.Blacklist) != 1 {
		t.Fatalf("expected idempotent add, got %v", deps.Server.Blacklist)
	}
}

func TestBlacklistRemoveAdmin(t *testing.T) {
	deps := newBlacklistDeps(t)
	deps.Server.Blacklist = []string{"a", "b", "c"}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/blacklist/b/delete", nil)
	req.SetPathValue("id", "b")
	rr := httptest.NewRecorder()
	BlacklistRemove(deps)(rr, adminCtx(req))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}
	if len(deps.Server.Blacklist) != 2 || deps.Server.Blacklist[0] != "a" || deps.Server.Blacklist[1] != "c" {
		t.Fatalf("expected [a c], got %v", deps.Server.Blacklist)
	}
}
