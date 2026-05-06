// dashboard/handlers/overview_test.go
package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/wind-c/comqtt/v2/mqtt"
)

const overviewMaster = `{{define "layout"}}<html>{{template "content" .}}</html>{{end}}` +
	`{{define "overview"}}{{template "layout" .}}{{end}}` +
	`{{define "content"}}<div id="overview-cards">{{template "overview_cards" .}}</div>{{end}}` +
	`{{define "overview_cards"}}{{range .Cards}}<div class="card">{{.Label}}={{.Value}}{{with .Unit}} {{.}}{{end}}</div>{{end}}{{end}}`

func newOverviewRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/overview.html": &fstest.MapFile{Data: []byte(overviewMaster)},
	}))
}

func TestOverviewGetRendersWithCards(t *testing.T) {
	server := mqtt.New(nil)
	atomic.StoreInt64(&server.Info.ClientsConnected, 7)
	atomic.StoreInt64(&server.Info.Subscriptions, 12)
	deps := OverviewDeps{
		Server:   server,
		Renderer: newOverviewRenderer(t),
	}
	rr := httptest.NewRecorder()
	OverviewGet(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Connections=7") {
		t.Fatalf("missing Connections card: %q", body)
	}
	if !strings.Contains(body, "Subscriptions=12") {
		t.Fatalf("missing Subscriptions card: %q", body)
	}
}

func TestOverviewCardsFragment(t *testing.T) {
	server := mqtt.New(nil)
	atomic.StoreInt64(&server.Info.Retained, 3)
	deps := OverviewDeps{
		Server:   server,
		Renderer: newOverviewRenderer(t),
	}
	rr := httptest.NewRecorder()
	OverviewCards(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/fragments/overview-cards", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Retained=3") {
		t.Fatalf("missing Retained card: %q", body)
	}
}

func TestRateSamplerComputesDelta(t *testing.T) {
	server := mqtt.New(nil)
	s := NewRateSampler(server)
	defer s.Stop()
	atomic.StoreInt64(&server.Info.MessagesReceived, 100)
	atomic.StoreInt64(&server.Info.MessagesSent, 50)
	in, out := s.Rates()
	if in != 0 || out != 0 {
		t.Fatalf("baseline rates should be 0: in=%v out=%v", in, out)
	}
}

func TestFormatIntPositive(t *testing.T) {
	cases := map[int64]string{0: "0", 1: "1", 42: "42", 100: "100", 1234: "1234"}
	for in, want := range cases {
		if got := formatInt(in); got != want {
			t.Errorf("formatInt(%d): %q want %q", in, got, want)
		}
	}
}
