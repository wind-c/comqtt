// dashboard/handlers/cluster_test.go
package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wind-c/comqtt/v2/cluster/discovery"
)

type fakeAgent struct {
	members []discovery.Member
	leader  string
}

func (f *fakeAgent) GetMemberList() []discovery.Member { return f.members }
func (f *fakeAgent) Leader() string                    { return f.leader }

func newClusterRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/cluster.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "cluster"}}{{template "layout" .}}{{end}}{{define "content"}}<div>leader={{.Leader}} count={{len .Members}}</div>{{range .Members}}<div class="row">{{.Name}}@{{.Addr}}:{{.Port}} leader={{.IsLeader}}</div>{{else}}<div class="empty">none</div>{{end}}{{end}}`)},
	}))
}

func TestClusterPageEmpty(t *testing.T) {
	deps := ClusterDeps{Agent: &fakeAgent{}, Renderer: newClusterRenderer(t)}
	rr := httptest.NewRecorder()
	ClusterPage(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/cluster", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "count=0") {
		t.Fatalf("expected empty: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<div class=\"empty\">") {
		t.Fatalf("expected empty row: %q", rr.Body.String())
	}
}

func TestClusterPageWithMembersHighlightsLeader(t *testing.T) {
	deps := ClusterDeps{
		Agent: &fakeAgent{
			members: []discovery.Member{
				{Name: "n2", Addr: "10.0.0.2", Port: 7946},
				{Name: "n1", Addr: "10.0.0.1", Port: 7946},
				{Name: "n3", Addr: "10.0.0.3", Port: 7946},
			},
			leader: "n1",
		},
		Renderer: newClusterRenderer(t),
	}
	rr := httptest.NewRecorder()
	ClusterPage(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/cluster", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "leader=n1 count=3") {
		t.Fatalf("expected leader+count line: %q", body)
	}
	if !strings.Contains(body, "n1@10.0.0.1:7946 leader=true") {
		t.Fatalf("expected n1 marked as leader: %q", body)
	}
	if !strings.Contains(body, "n2@10.0.0.2:7946 leader=false") {
		t.Fatalf("expected n2 not leader: %q", body)
	}
	// Sort check: n1 should appear before n2 in body
	if strings.Index(body, "n1@") > strings.Index(body, "n2@") {
		t.Fatalf("expected sorted order: %q", body)
	}
}

func TestClusterPageHandlesUnknownLeader(t *testing.T) {
	deps := ClusterDeps{
		Agent: &fakeAgent{
			members: []discovery.Member{{Name: "n1", Addr: "10.0.0.1", Port: 7946}},
			leader:  "",
		},
		Renderer: newClusterRenderer(t),
	}
	rr := httptest.NewRecorder()
	ClusterPage(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/cluster", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "leader= count=1") {
		t.Fatalf("expected empty leader: %q", body)
	}
	if !strings.Contains(body, "n1@10.0.0.1:7946 leader=false") {
		t.Fatalf("expected n1 not leader: %q", body)
	}
}
