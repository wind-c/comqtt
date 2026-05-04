// mqtt/dashboard/handlers/subscriptions_test.go
package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func newSubscriptionsRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/subscriptions.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "subscriptions"}}{{template "layout" .}}{{end}}{{define "content"}}<div>total={{.Page.Total}} topic={{.Topic}} clientid={{.ClientID}}</div>{{range .Page.Items}}<div class="row">{{.ClientID}}|{{.Topic}}|qos{{.QoS}}</div>{{else}}<div class="empty">no subs</div>{{end}}{{end}}`)},
	}))
}

func addClientWithSubs(t *testing.T, server *mqtt.Server, id string, filters ...string) {
	t.Helper()
	cl := &mqtt.Client{ID: id}
	cl.Properties.Username = []byte("u")
	cl.Net.Remote = "127.0.0.1:0"
	cl.State.Subscriptions = mqtt.NewSubscriptions()
	cl.State.Inflight = mqtt.NewInflights()
	for _, f := range filters {
		cl.State.Subscriptions.Add(f, packets.Subscription{Filter: f, Qos: 1})
	}
	server.Clients.Add(cl)
}

func TestSubscriptionsListEmpty(t *testing.T) {
	server := mqtt.New(nil)
	deps := SubscriptionsDeps{Server: server, Renderer: newSubscriptionsRenderer(t)}
	rr := httptest.NewRecorder()
	SubscriptionsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/subscriptions", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "total=0") {
		t.Fatalf("expected total=0: %q", body)
	}
	if !strings.Contains(body, "no subs") {
		t.Fatalf("expected empty marker: %q", body)
	}
}

func TestSubscriptionsListPopulated(t *testing.T) {
	server := mqtt.New(nil)
	addClientWithSubs(t, server, "alpha", "sensors/a", "sensors/b")
	addClientWithSubs(t, server, "bravo", "metrics/heap")
	deps := SubscriptionsDeps{Server: server, Renderer: newSubscriptionsRenderer(t)}
	rr := httptest.NewRecorder()
	SubscriptionsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/subscriptions", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "total=3") {
		t.Fatalf("expected total=3: %q", body)
	}
	if !strings.Contains(body, "alpha|sensors/a|qos1") {
		t.Fatalf("expected alpha row: %q", body)
	}
	if !strings.Contains(body, "bravo|metrics/heap|qos1") {
		t.Fatalf("expected bravo row: %q", body)
	}
}

func TestSubscriptionsListFilters(t *testing.T) {
	server := mqtt.New(nil)
	addClientWithSubs(t, server, "alpha", "sensors/a", "metrics/heap")
	addClientWithSubs(t, server, "bravo", "sensors/b")
	deps := SubscriptionsDeps{Server: server, Renderer: newSubscriptionsRenderer(t)}

	rr := httptest.NewRecorder()
	SubscriptionsList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/subscriptions?topic=sensors", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "total=2") {
		t.Fatalf("expected topic filter total=2: %q", body)
	}

	rr2 := httptest.NewRecorder()
	SubscriptionsList(deps)(rr2, httptest.NewRequest(http.MethodGet, "/dashboard/subscriptions?clientid=alpha", nil))
	body2 := rr2.Body.String()
	if !strings.Contains(body2, "total=2") {
		t.Fatalf("expected clientid filter total=2: %q", body2)
	}
	if strings.Contains(body2, "bravo|") {
		t.Fatalf("clientid filter should exclude bravo: %q", body2)
	}
}
