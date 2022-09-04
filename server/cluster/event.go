package cluster

import (
	"fmt"
	"github.com/hashicorp/memberlist"
)

const(
	EventJoin = iota + 1
	EventLeave
	EventUpdate
)

type Event struct {
	Type int
	Node *memberlist.Node
}

type NodeEvents struct{
	ech  chan Event
}

func NewEvents() *NodeEvents {
	return &NodeEvents{ech: make(chan Event, 2)}
}

func (n *NodeEvents) NotifyJoin(node *memberlist.Node) {
	fmt.Println("A node has joined: " + node.String())
	n.ech <- Event{EventJoin, node}
}

func (n *NodeEvents) NotifyLeave(node *memberlist.Node) {
	fmt.Println("A node has left: " + node.String())
	n.ech <- Event{EventLeave, node}
}

func (n *NodeEvents) NotifyUpdate(node *memberlist.Node) {
	fmt.Println("A node was updated: " + node.String())
	n.ech <- Event{EventUpdate, node}
}
