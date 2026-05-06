// dashboard/handlers/retained_test.go
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

func newRetainedRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/retained.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "retained"}}{{template "layout" .}}{{end}}{{define "content"}}<div>total={{.Page.Total}} topic={{.Topic}} readonly={{.Readonly}}</div>{{range .Page.Items}}<div class="row">{{.Topic}}|{{.TopicEncoded}}|qos{{.QoS}}|sz{{.Size}}</div>{{else}}<div class="empty">no retained</div>{{end}}{{end}}`)},
	}))
}

func addRetainedDash(t *testing.T, server *mqtt.Server, topic string, payload []byte, qos byte) {
	t.Helper()
	pk := packets.Packet{
		TopicName:   topic,
		Payload:     payload,
		FixedHeader: packets.FixedHeader{Qos: qos, Retain: true},
		Created:     12345,
	}
	server.Topics.Retained.Add(topic, pk)
}

func TestRetainedListEmpty(t *testing.T) {
	server := mqtt.New(nil)
	deps := RetainedDeps{Server: server, Renderer: newRetainedRenderer(t)}
	rr := httptest.NewRecorder()
	RetainedList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/retained", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "total=0") {
		t.Fatalf("expected total=0: %q", body)
	}
	if !strings.Contains(body, "no retained") {
		t.Fatalf("expected empty marker: %q", body)
	}
}

func TestRetainedListPopulatedAndFiltered(t *testing.T) {
	server := mqtt.New(nil)
	addRetainedDash(t, server, "sensors/temp", []byte("21.5"), 0)
	addRetainedDash(t, server, "metrics/heap", []byte("100"), 1)
	deps := RetainedDeps{Server: server, Renderer: newRetainedRenderer(t)}

	rr := httptest.NewRecorder()
	RetainedList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/retained", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "total=2") {
		t.Fatalf("expected total=2: %q", body)
	}
	if !strings.Contains(body, "sensors/temp|sensors%2Ftemp|qos0|sz4") {
		t.Fatalf("expected sensors row with encoded topic: %q", body)
	}

	rr2 := httptest.NewRecorder()
	RetainedList(deps)(rr2, httptest.NewRequest(http.MethodGet, "/dashboard/retained?topic=metrics", nil))
	body2 := rr2.Body.String()
	if !strings.Contains(body2, "total=1") {
		t.Fatalf("expected filter total=1: %q", body2)
	}
	if !strings.Contains(body2, "metrics/heap") {
		t.Fatalf("expected metrics row: %q", body2)
	}
}

func TestRetainedListReadonlyForViewer(t *testing.T) {
	server := mqtt.New(nil)
	deps := RetainedDeps{Server: server, Renderer: newRetainedRenderer(t)}
	req := httptest.NewRequest(http.MethodGet, "/dashboard/retained", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	rr := httptest.NewRecorder()
	RetainedList(deps)(rr, req)
	if !strings.Contains(rr.Body.String(), "readonly=true") {
		t.Fatalf("expected readonly: %q", rr.Body.String())
	}
}

func TestRetainedClearAdminHappy(t *testing.T) {
	server := mqtt.New(nil)
	addRetainedDash(t, server, "sensors/temp", []byte("21.5"), 0)
	deps := RetainedDeps{Server: server, Renderer: newRetainedRenderer(t)}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/retained/sensors%2Ftemp/delete", nil)
	req.SetPathValue("topic", "sensors%2Ftemp")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	RetainedClear(deps)(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if _, ok := server.Topics.Retained.Get("sensors/temp"); ok {
		t.Fatal("retained should be cleared")
	}
}

func TestRetainedClearViewerForbidden(t *testing.T) {
	server := mqtt.New(nil)
	addRetainedDash(t, server, "sensors/temp", []byte("21.5"), 0)
	deps := RetainedDeps{Server: server, Renderer: newRetainedRenderer(t)}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/retained/sensors%2Ftemp/delete", nil)
	req.SetPathValue("topic", "sensors%2Ftemp")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "v", Role: auth.RoleViewer}))
	rr := httptest.NewRecorder()
	RetainedClear(deps)(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: %d", rr.Code)
	}
}
