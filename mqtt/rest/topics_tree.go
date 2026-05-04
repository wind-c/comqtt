// mqtt/rest/topics_tree.go
package rest

import (
	"net/http"
	"sort"
	"strings"
)

type topicNode struct {
	Topic       string       `json:"topic"`
	Subscribers int          `json:"subscribers"`
	Children    []*topicNode `json:"children,omitempty"`
}

// topicsTree handles GET /api/v1/mqtt/topics.
// Walks every connected client's subscription set, builds a slash-separated
// trie, and returns the root.
func (s *Rest) topicsTree(w http.ResponseWriter, r *http.Request) {
	root := &topicNode{}
	for _, cl := range s.server.Clients.GetAll() {
		for filter := range cl.State.Subscriptions.GetAll() {
			insertTopic(root, filter)
		}
	}
	sortTree(root)
	Ok(w, root)
}

func insertTopic(root *topicNode, filter string) {
	cur := root
	for _, seg := range strings.Split(filter, "/") {
		var found *topicNode
		for _, c := range cur.Children {
			if c.Topic == seg {
				found = c
				break
			}
		}
		if found == nil {
			found = &topicNode{Topic: seg}
			cur.Children = append(cur.Children, found)
		}
		cur = found
	}
	cur.Subscribers++
}

func sortTree(n *topicNode) {
	sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Topic < n.Children[j].Topic })
	for _, c := range n.Children {
		sortTree(c)
	}
}
