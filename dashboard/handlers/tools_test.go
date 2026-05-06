// dashboard/handlers/tools_test.go
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

func newToolsRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/tools.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "tools"}}{{template "layout" .}}{{end}}{{define "content"}}<div>readonly={{.Readonly}}</div>{{with .Flash}}<div class="ok">{{.}}</div>{{end}}{{with .Error}}<div class="err">{{.}}</div>{{end}}<form><input value="{{.Form.Topic}}"><input value="{{.Form.QoS}}"></form>{{end}}`)},
	}))
}

func newToolsDeps(t *testing.T) ToolsDeps {
	t.Helper()
	return ToolsDeps{
		Server:   mqtt.New(&mqtt.Options{InlineClient: true}),
		Renderer: newToolsRenderer(t),
	}
}

func TestToolsGetAdminNotReadonly(t *testing.T) {
	deps := newToolsDeps(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard/tools", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	ToolsGet(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "readonly=false") {
		t.Fatalf("admin should not be readonly: %q", rr.Body.String())
	}
}

func TestToolsGetViewerReadonly(t *testing.T) {
	deps := newToolsDeps(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard/tools", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	ToolsGet(deps)(rr, req)
	if !strings.Contains(rr.Body.String(), "readonly=true") {
		t.Fatalf("viewer should be readonly: %q", rr.Body.String())
	}
}

func TestToolsPublishAdminHappy(t *testing.T) {
	deps := newToolsDeps(t)
	form := url.Values{
		"topic":   {"sensors/temp"},
		"payload": {"21.5"},
		"qos":     {"1"},
		"retain":  {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/tools/publish", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ToolsPublish(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Published to sensors/temp") {
		t.Fatalf("expected success flash: %q", rr.Body.String())
	}
	if _, ok := deps.Server.Topics.Retained.Get("sensors/temp"); !ok {
		t.Fatal("expected retained message at sensors/temp")
	}
}

func TestToolsPublishViewerForbidden(t *testing.T) {
	deps := newToolsDeps(t)
	form := url.Values{"topic": {"x"}, "qos": {"0"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/tools/publish", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	rr := httptest.NewRecorder()
	ToolsPublish(deps)(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestToolsPublishMissingTopic(t *testing.T) {
	deps := newToolsDeps(t)
	form := url.Values{"topic": {""}, "qos": {"0"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/tools/publish", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ToolsPublish(deps)(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Topic is required") {
		t.Fatalf("expected required-topic error: %q", rr.Body.String())
	}
}

func TestToolsPublishBadQoS(t *testing.T) {
	deps := newToolsDeps(t)
	form := url.Values{"topic": {"x"}, "qos": {"7"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/tools/publish", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ToolsPublish(deps)(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "QoS must be 0, 1, or 2") {
		t.Fatalf("expected qos error: %q", rr.Body.String())
	}
}
