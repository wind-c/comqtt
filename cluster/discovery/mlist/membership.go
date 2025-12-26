// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package mlist

import (
	"net"
	"time"

	"github.com/hashicorp/memberlist"
	mb "github.com/wind-c/comqtt/v2/cluster/discovery"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/config"
	"github.com/wind-c/comqtt/v2/mqtt"
)

type Membership struct {
	config     *config.Cluster
	list       *memberlist.Memberlist
	delegate   *Delegate
	event      *NodeEvents
	mqttServer *mqtt.Server
	msgCh      chan<- []byte
}

func wrapOptions(conf *config.Cluster) *memberlist.Config {
	opts := make([]Option, 3)
	opts[0] = WithLogOutput(log.Writer(), LogLevelInfo) //Used to filter memberlist logs
	opts[1] = WithBindPort(conf.BindPort)
	opts[2] = WithHandoffQueueDepth(conf.QueueDepth)
	if conf.NodeName != "" {
		opts = append(opts, WithNodeName(conf.NodeName))
	}
	if conf.BindAddr != "" {
		opts = append(opts, WithBindAddr(conf.BindAddr))
	}
	if conf.AdvertiseAddr != "" {
		opts = append(opts, WithAdvertiseAddr(conf.AdvertiseAddr))
	}
	if conf.AdvertisePort != 0 {
		opts = append(opts, WithAdvertisePort(conf.AdvertisePort))
	}

	return NewOptions(opts...)
}

func New(config *config.Cluster, inboundMsgCh chan<- []byte) *Membership {
	return &Membership{
		config: config,
		msgCh:  inboundMsgCh,
	}
}

func (m *Membership) Setup() error {
	// create member list
	if err := m.createMemberList(wrapOptions(m.config)); err != nil {
		return err
	}
	// join cluster
	if len(m.config.Members) > 0 {
		if _, err := m.list.Join(m.config.Members); err != nil {
			return err
		}
	}
	log.Info("local member", "addr", m.LocalAddr(), "port", m.config.BindPort)

	return nil
}

func (m *Membership) createMemberList(conf *memberlist.Config) (err error) {
	if conf == nil {
		conf = memberlist.DefaultLocalConfig()
	}
	m.delegate = NewDelegate(m.msgCh)
	m.event = NewEvents()
	conf.Delegate = m.delegate
	conf.Events = m.event
	//if tn, err := mb.NewCoNetTransport(conf); err != nil {
	//	return err
	//} else {
	//	conf.Transport = tn
	//}

	if m.list, err = memberlist.Create(conf); err != nil {
		return err
	}

	m.delegate.InitBroadcasts(m.list)
	m.delegate.BindMqttServer(m.mqttServer)

	return nil
}

func (m *Membership) BindMqttServer(server *mqtt.Server) {
	m.mqttServer = server
}

func (m *Membership) LocalName() string {
	return m.list.LocalNode().Name
}

func (m *Membership) LocalAddr() string {
	return m.list.LocalNode().Addr.String()
}

func (m *Membership) NumMembers() int {
	return m.list.NumMembers()
}

func (m *Membership) EventChan() <-chan *mb.Event {
	return m.event.ech
}

func (m *Membership) LocalNode() *memberlist.Node {
	return m.list.LocalNode()
}

func (m *Membership) Members() []mb.Member {
	members := m.aliveMembers()
	ms := make([]mb.Member, len(members))
	for i, m := range members {
		ms[i] = mb.Member{
			Name: m.Name,
			Addr: m.Addr.String(),
			Port: int(m.Port),
			Tags: nil,
		}
	}
	return ms
}

func (m *Membership) aliveMembers() []*memberlist.Node {
	return m.list.Members()
}

func (m *Membership) GetNodeByIP(ipAddr net.IP) *memberlist.Node {
	members := m.aliveMembers()
	for _, node := range members {
		if node.Name == m.config.NodeName {
			continue // skip self
		}
		if node.Addr.To4().Equal(ipAddr.To4()) {
			return node
		}
	}
	return nil
}

func (m *Membership) send(to *memberlist.Node, msg []byte) error {
	//return m.list.SendReliable(to, msg) //tcp reliable
	return m.list.SendBestEffort(to, msg) //udp unreliable
}

// SendToOthers send message to all nodes except yourself
func (m *Membership) SendToOthers(msg []byte) {
	for _, node := range m.aliveMembers() {
		if node.Name == m.config.NodeName {
			continue // skip self
		}
		if err := m.send(node, msg); err != nil {
			log.Error("send to others", "error", err, "from", m.config.NodeName, "to", node.Name)
		}
	}
}

// SendToNode send message to a node
func (m *Membership) SendToNode(nodeName string, msg []byte) error {
	for _, node := range m.aliveMembers() {
		if node.Name == nodeName {
			if err := m.send(node, msg); err != nil {
				log.Error("send to others", "error", err, "from", m.config.NodeName, "to", nodeName)
				return err
			}
		}
	}
	return nil
}

func (m *Membership) Broadcast(msg []byte) {
	m.delegate.Broadcast(msg)
}

func (m *Membership) Stat() map[string]int64 {
	return m.delegate.State
}

func (m *Membership) Stop() {
	m.list.Leave(time.Second)
	m.list.Shutdown()
	m.delegate.Stop()
}

// Join joins an existing Serf cluster. Returns the number of nodes
// successfully contacted. The returned error will be non-nil only in the
// case that no nodes could be contacted.
// The format of an existing node is nodename/ip:port or ip:port
func (m *Membership) Join(existing []string) (int, error) {
	return m.list.Join(existing)
}

// Leave gracefully exits the cluster.
func (m *Membership) Leave() error {
	return m.list.Leave(5 * time.Second)
}
