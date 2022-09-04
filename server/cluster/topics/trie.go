package topics

import (
	"strings"
	"sync"
)

// Subscriptions is a map of subscriptions keyed on client.
type Subscriptions map[string]byte

// Index is a prefix/trie tree containing topic subscribers and retained messages.
type Index struct {
	mu   sync.RWMutex // a mutex for locking the whole index.
	Root *Leaf        // a leaf containing a message and more leaves.
}

// New returns a pointer to a new instance of Index.
func New() *Index {
	return &Index{
		Root: &Leaf{
			Leaves: make(map[string]*Leaf),
			Count:  0,
		},
	}
}

// Subscribe creates a subscription filter for a client. Returns true if the
// subscription was new.
func (x *Index) Subscribe(filter string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()

	n := x.poperate(filter)
	n.Filter = filter

	return n.Count > 0
}

// Unsubscribe removes a subscription filter for a client. Returns true if an
// unsubscribe action successful and the subscription existed.
func (x *Index) Unsubscribe(filter string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.unpoperate(filter)
}

// unpoperate steps backward through a trie sequence and removes any orphaned
// nodes. If a client id is specified, it will unsubscribe a client. If message
// is true, it will delete a retained message.
func (x *Index) unpoperate(filter string) bool {
	var d int // Walk to end leaf.
	var particle string
	var hasNext = true
	e := x.Root
	for hasNext {
		particle, hasNext = isolateParticle(filter, d)
		d++
		e, _ = e.Leaves[particle]

		// If the topic part doesn't exist in the tree, there's nothing
		// left to do.
		if e == nil {
			return false
		}
	}

	// Step backward removing client and orphaned leaves.
	var key string
	var orphaned bool
	var end = true
	for e.Parent != nil {
		key = e.Key

		// Wipe the client from this leaf if it's the filter end.
		if end {
			if e.Count > 0 {
				e.Count--
			}
			end = false
		}

		// If this leaf is empty, note it as orphaned.
		orphaned = e.Count == 0 && len(e.Leaves) == 0

		// Traverse up the branch.
		e = e.Parent

		// If the leaf we just came from was empty, delete it.
		if orphaned {
			delete(e.Leaves, key)
		}
	}

	return true
}

// poperate iterates and populates through a topic/filter path, instantiating
// leaves as it goes and returning the final leaf in the branch.
// poperate is a more enjoyable word than iterpop.
func (x *Index) poperate(topic string) *Leaf {
	var d int
	var particle string
	var hasNext = true
	n := x.Root
	for hasNext {
		particle, hasNext = isolateParticle(topic, d)
		d++

		child, _ := n.Leaves[particle]
		if child == nil {
			child = &Leaf{
				Key:    particle,
				Parent: n,
				Leaves: make(map[string]*Leaf),
				Count:  0,
			}
			n.Leaves[particle] = child
		}
		n = child
	}
	n.Count++

	return n
}

// Scan returns true if a matching filter exists
func (x *Index) Scan(topic string) []string {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.Root.scan(topic, 0)
}

// Leaf is a child node on the tree.
type Leaf struct {
	Key    string           // the key that was used to create the leaf.
	Parent *Leaf            // a pointer to the parent node for the leaf.
	Leaves map[string]*Leaf // a map of child nodes, keyed on particle id.
	Filter string           // the path of the topic filter being matched.
	Count  int
}

// scanSubscribers recursively steps through a branch of leaves finding clients who
// have subscription filters matching a topic, and their highest QoS byte.
func (l *Leaf) scan(topic string, d int) []string {
	filters := make([]string, 0)
	part, hasNext := isolateParticle(topic, d)
	// For either the topic part, a +, or a #, follow the branch.
	for _, particle := range []string{part, "+", "#"} {

		// Topics beginning with the reserved $ character are restricted from
		// being returned for top level wildcards.
		if d == 0 && len(part) > 0 && part[0] == '$' && (particle == "+" || particle == "#") {
			continue
		}

		if child, ok := l.Leaves[particle]; ok {

			// We're only interested in getting clients from the final
			// element in the topic, or those with wildhashes.
			if !hasNext || particle == "#" {
				// matching the topic.
				filters = append(filters, child.Filter)

				// Make sure we also capture any client who are listening
				// to this topic via path/#
				if !hasNext {
					if extra, ok := child.Leaves["#"]; ok {
						filters = append(filters, extra.Filter)
					}
				}
			}

			// If this branch has hit a wildhash, just return immediately.
			if particle == "#" {
				return filters
			} else if hasNext {
				filters = child.scan(topic, d+1)
			}
		}
	}

	return filters
}

// isolateParticle extracts a particle between d / and d+1 / without allocations.
func isolateParticle(filter string, d int) (particle string, hasNext bool) {
	var next, end int
	for i := 0; end > -1 && i <= d; i++ {
		end = strings.IndexRune(filter, '/')
		if d > -1 && i == d && end > -1 {
			hasNext = true
			particle = filter[next:end]
		} else if end > -1 {
			hasNext = false
			filter = filter[end+1:]
		} else {
			hasNext = false
			particle = filter[next:]
		}
	}

	return
}

// ReLeaf is a dev function for showing the trie leafs.
/*
func ReLeaf(m string, leaf *Leaf, d int) {
	for k, v := range leaf.Leaves {
		fmt.Println(m, d, strings.Repeat("  ", d), k)
		ReLeaf(m, v, d+1)
	}
}
*/
