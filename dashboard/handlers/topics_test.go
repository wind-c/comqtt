// dashboard/handlers/topics_test.go
package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wind-c/comqtt/v2/mqtt"
)

func newTopicsRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/topics.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "topics"}}{{template "layout" .}}{{end}}{{define "content"}}<div>unique={{.UniqueFilters}}</div>{{template "topic_node" .Root}}{{end}}{{define "topic_node"}}{{if .Topic}}<n topic="{{.Topic}}" subs="{{.Subscribers}}">{{range .Children}}{{template "topic_node" .}}{{end}}</n>{{else}}{{range .Children}}{{template "topic_node" .}}{{end}}{{end}}{{end}}`)},
	}))
}

func TestTopicsTreeEmpty(t *testing.T) {
	server := mqtt.New(nil)
	deps := TopicsDeps{Server: server, Renderer: newTopicsRenderer(t)}
	rr := httptest.NewRecorder()
	TopicsTree(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/topics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unique=0") {
		t.Fatalf("expected unique=0: %q", rr.Body.String())
	}
}

func TestTopicsTreeBuilds(t *testing.T) {
	server := mqtt.New(nil)
	addClientWithSubs(t, server, "alpha", "sensors/temp/room1", "sensors/temp/room2")
	addClientWithSubs(t, server, "bravo", "sensors/temp/room1")
	deps := TopicsDeps{Server: server, Renderer: newTopicsRenderer(t)}
	rr := httptest.NewRecorder()
	TopicsTree(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/topics", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "unique=2") {
		t.Fatalf("expected unique=2: %q", body)
	}
	if !strings.Contains(body, `topic="sensors"`) {
		t.Fatalf("expected sensors node: %q", body)
	}
	if !strings.Contains(body, `topic="room1" subs="2"`) {
		t.Fatalf("expected room1 with 2 subscribers: %q", body)
	}
	if !strings.Contains(body, `topic="room2" subs="1"`) {
		t.Fatalf("expected room2 with 1 subscriber: %q", body)
	}
}

func TestInsertTopicTreeShared(t *testing.T) {
	root := &topicTreeNode{}
	insertTopicTree(root, "a/b/c")
	insertTopicTree(root, "a/b/d")
	if len(root.Children) != 1 || root.Children[0].Topic != "a" {
		t.Fatalf("root children: %+v", root.Children)
	}
	a := root.Children[0]
	if len(a.Children) != 1 || a.Children[0].Topic != "b" {
		t.Fatalf("a children: %+v", a.Children)
	}
	b := a.Children[0]
	if len(b.Children) != 2 {
		t.Fatalf("expected 2 leaves under b: %+v", b.Children)
	}
}
