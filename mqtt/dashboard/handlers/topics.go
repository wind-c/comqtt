// mqtt/dashboard/handlers/topics.go
package handlers

import (
	"net/http"
	"sort"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
)

type TopicsDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type topicTreeNode struct {
	Topic       string
	Subscribers int
	Children    []*topicTreeNode
}

type topicsPageData struct {
	Title         string
	User          auth.User
	CSRF          string
	Cluster       bool
	Flash         string
	Root          *topicTreeNode
	UniqueFilters int
}

func TopicsTree(d TopicsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root := &topicTreeNode{}
		seen := map[string]bool{}
		for _, cl := range d.Server.Clients.GetAll() {
			for filter := range cl.State.Subscriptions.GetAll() {
				seen[filter] = true
				insertTopicTree(root, filter)
			}
		}
		sortTopicTree(root)
		d.Renderer.Render(w, "topics", topicsPageData{
			Title:         "Topics",
			User:          auth.UserFromContext(r.Context()),
			CSRF:          auth.NewCSRFToken(),
			Cluster:       d.Cluster,
			Root:          root,
			UniqueFilters: len(seen),
		})
	}
}

func insertTopicTree(root *topicTreeNode, filter string) {
	cur := root
	for _, seg := range strings.Split(filter, "/") {
		var found *topicTreeNode
		for _, c := range cur.Children {
			if c.Topic == seg {
				found = c
				break
			}
		}
		if found == nil {
			found = &topicTreeNode{Topic: seg}
			cur.Children = append(cur.Children, found)
		}
		cur = found
	}
	cur.Subscribers++
}

func sortTopicTree(n *topicTreeNode) {
	sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Topic < n.Children[j].Topic })
	for _, c := range n.Children {
		sortTopicTree(c)
	}
}
