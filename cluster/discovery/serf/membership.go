// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package serf

import (
	"strconv"

	"github.com/hashicorp/logutils"
	"github.com/hashicorp/serf/serf"
	mb "github.com/wind-c/comqtt/v2/cluster/discovery"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/config"
	"github.com/wind-c/comqtt/v2/mqtt"
)

const (
	LogLevelDebug = "DEBUG"
	LogLevelWarn  = "WARN"
	LogLevelError = "ERROR"
	LogLevelInfo  = "INFO"
)

func wrapOptions(conf *config.Cluster, ech chan serf.Event) *serf.Config {
	config := serf.DefaultConfig()
	config.Init()
	config.MemberlistConfig.BindAddr = conf.BindAddr
	config.MemberlistConfig.BindPort = conf.BindPort
	config.NodeName = conf.NodeName
	config.EventCh = ech
	if conf.Tags == nil {
		conf.Tags = make(map[string]string)
	}
	if len(conf.Tags) == 0 {
		conf.Tags[mb.TagRaftPort] = strconv.Itoa(conf.RaftPort)
		conf.Tags[mb.TagGrpcPort] = strconv.Itoa(conf.GrpcPort)
	}
	config.Tags = conf.Tags
	if conf.QueueDepth != 0 {
		config.MaxQueueDepth = conf.QueueDepth
	}

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{LogLevelDebug, LogLevelWarn, LogLevelError, LogLevelInfo},
		MinLevel: logutils.LogLevel(LogLevelError),
		Writer:   log.Writer(),
	}
	config.MemberlistConfig.LogOutput = filter
	config.LogOutput = filter

	return config
}

type Membership struct {
	config     *config.Cluster
	serf       *serf.Serf
	serfCh     chan serf.Event
	eventCh    chan *mb.Event
	msgCh      chan<- []byte
	mqttServer *mqtt.Server
}

func New(conf *config.Cluster, inboundMsgCh chan<- []byte) *Membership {
	return &Membership{
		config:  conf,
		serfCh:  make(chan serf.Event, 256),
		eventCh: make(chan *mb.Event, 64),
		msgCh:   inboundMsgCh,
	}
}

func (m *Membership) Setup() (err error) {
	config := wrapOptions(m.config, m.serfCh)
	m.serf, err = serf.Create(config)
	if err != nil {
		return
	}

	go m.eventLoop() // start the event loop, will stop when serfCh is closed

	if m.config.Members != nil {
		if len(m.config.Members) > 0 {
			if _, err = m.serf.Join(m.config.Members, true); err != nil {
				return err
			}
		}
	}
	log.Info("local member", "addr", m.LocalAddr(), "port", m.config.BindPort)
	return
}

func (m *Membership) BindMqttServer(server *mqtt.Server) {
	m.mqttServer = server
}

func (m *Membership) EventChan() <-chan *mb.Event {
	return m.eventCh
}

func (m *Membership) numMembers() int {
	return len(m.aliveMembers())
}

func (m *Membership) LocalName() string {
	return m.serf.LocalMember().Name
}

func (m *Membership) LocalAddr() string {
	return m.serf.LocalMember().Addr.String()
}

func (m *Membership) Stat() map[string]int64 {
	return nil
}

func (m *Membership) Stop() {
	err := m.serf.Leave()
	if err != nil {
		log.Error("serf leave", "error", err)
	}
	err = m.serf.Shutdown()
	if err != nil {
		log.Error("serf shutdown", "error", err)
	}
	// this shuts down the event loop, note that this can't be called multiple times
	// if we need to do so, we could use a bool, sync.Once or recover from the panic
	close(m.serfCh)
}

func genEvent(tp int, node *serf.Member) *mb.Event {
	return &mb.Event{
		Type: tp,
		Member: mb.Member{
			Name: node.Name,
			Addr: node.Addr.String(),
			Port: int(node.Port),
			Tags: node.Tags,
		},
	}
}

func (m *Membership) eventLoop() {
	for e := range m.serfCh {
		switch e.EventType() {
		case serf.EventMemberLeave, serf.EventMemberFailed:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.eventCh <- genEvent(mb.EventLeave, &member)
				onLog(&member, "member leave")
			}
		case serf.EventMemberJoin:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.eventCh <- genEvent(mb.EventJoin, &member)
				onLog(&member, "member join")
			}
		case serf.EventMemberUpdate:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.eventCh <- genEvent(mb.EventUpdate, &member)
				onLog(&member, "member update")
			}
		case serf.EventMemberReap:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.eventCh <- genEvent(mb.EventReap, &member)
				onLog(&member, "member reap")
			}
		case serf.EventUser:
			ue := e.(serf.UserEvent)
			if ue.Name == m.config.NodeName {
				continue
			}
			m.msgCh <- ue.Payload
		case serf.EventQuery:
			q := e.(*serf.Query)
			if q.SourceNode() == m.config.NodeName {
				continue
			}
			m.msgCh <- q.Payload
			//m.handleQuery(e.(*serf.Query))
		default:
			panic("unknown serf event type")
		}
	}
}

// SendToOthers send message to all nodes except yourself
func (m *Membership) SendToOthers(msg []byte) {
	m.Broadcast(msg)
}

// SendToNode send message to a node
func (m *Membership) SendToNode(nodeName string, msg []byte) error {
	qp := m.serf.DefaultQueryParams()
	qp.FilterNodes = m.otherNames(nodeName)
	if _, err := m.serf.Query(m.config.NodeName, msg, qp); err != nil {
		log.Error("send to node", "error", err, "from", m.config.NodeName, "to", nodeName)
		return err
	}

	return nil
}

func (m *Membership) Broadcast(msg []byte) {
	m.serf.UserEvent(m.config.NodeName, msg, false)
}

func onLog(node *serf.Member, prompt string) {
	log.Info(prompt, "node", node.Name, "addr", node.Addr.String(), "port", node.Port)
}

func (m *Membership) isLocal(member serf.Member) bool {
	return m.serf.LocalMember().Name == member.Name
}

func (m *Membership) otherNames(excluded string) []string {
	names := make([]string, 0)
	for _, node := range m.serf.Members() {
		if node.Status != serf.StatusAlive || node.Name != excluded {
			continue // skip excluded and not alive node
		}
		names = append(names, node.Name)
	}
	return names
}

func (m *Membership) Members() []mb.Member {
	members := m.aliveMembers()
	ms := make([]mb.Member, len(members))
	for i, m := range members {
		ms[i] = mb.Member{m.Name, m.Addr.String(), int(m.Port), m.Tags}
	}
	return ms
}

// AliveMembers return alive serf members
func (m *Membership) aliveMembers() []serf.Member {
	if m.serf == nil {
		return nil
	}
	all := m.serf.Members()
	alive := make([]serf.Member, 0, len(all))
	for _, b := range all {
		// that return only alive nodes
		if b.Status == serf.StatusAlive {
			alive = append(alive, b)
		}
	}
	return alive
}

// Join joins an existing Serf cluster. Returns the number of nodes
// successfully contacted. The returned error will be non-nil only in the
// case that no nodes could be contacted.
// The format of an existing node is nodename/ip:port or ip:port
func (m *Membership) Join(existing []string) (int, error) {
	return m.serf.Join(existing, true)
}

// Leave gracefully exits the cluster.
func (m *Membership) Leave() error {
	return m.serf.Leave()
}
