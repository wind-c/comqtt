// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package mlist

import (
	"github.com/hashicorp/memberlist"
	"github.com/wind-c/comqtt/v2/cluster/discovery"
	"github.com/wind-c/comqtt/v2/cluster/log"
)

//type Event struct {
//	Type int
//	Node *memberlist.Node
//}

type NodeEvents struct {
	ech chan *discovery.Event
}

func NewEvents() *NodeEvents {
	return &NodeEvents{ech: make(chan *discovery.Event, 64)}
}

func genEvent(tp int, node *memberlist.Node) *discovery.Event {
	return &discovery.Event{
		Type: tp,
		Member: discovery.Member{
			Name: node.Name,
			Addr: node.Addr.String(),
			Port: int(node.Port),
		},
	}
}

func (n *NodeEvents) NotifyJoin(node *memberlist.Node) {
	n.ech <- genEvent(discovery.EventJoin, node)
	onLog(node, "notify join")
}

func (n *NodeEvents) NotifyLeave(node *memberlist.Node) {
	n.ech <- genEvent(discovery.EventLeave, node)
	onLog(node, "notify leave")
}

func (n *NodeEvents) NotifyUpdate(node *memberlist.Node) {
	n.ech <- genEvent(discovery.EventUpdate, node)
	onLog(node, "notify update")
}

func onLog(node *memberlist.Node, prompt string) {
	log.Info(prompt, "node", node.Name, "addr", node.Addr.String())
}
