// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package serf

import (
	"github.com/hashicorp/logutils"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/serf"
	mb "github.com/wind-c/comqtt/v2/cluster/discovery"
	"github.com/wind-c/comqtt/v2/cluster/log/zero"
	"github.com/wind-c/comqtt/v2/config"
	"github.com/wind-c/comqtt/v2/mqtt"
	"io"
	"os"
	"strconv"
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
	var logger io.Writer
	if zero.Logger() != nil {
		logger = zero.Logger()
	} else {
		logger = os.Stderr
	}
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{LogLevelDebug, LogLevelWarn, LogLevelError, LogLevelInfo},
		MinLevel: logutils.LogLevel(LogLevelError),
		Writer:   logger,
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

	go m.eventLoop()

	if m.config.Members != nil {
		if len(m.config.Members) > 0 {
			if _, err = m.serf.Join(m.config.Members, true); err != nil {
				return err
			}
		}
	}
	zero.Info().Str("addr", m.LocalAddr()).Int("port", m.config.BindPort).Msg("local member")

	return
}

func (m *Membership) BindMqttServer(server *mqtt.Server) {
	m.mqttServer = server
}

func (m *Membership) EventChan() <-chan *mb.Event {
	return m.eventCh
}

func (m *Membership) Leave() error {
	return m.serf.Leave()
}

func (m *Membership) NumMembers() int {
	return m.serf.NumNodes()
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
	m.serf.Leave()
	m.serf.Shutdown()
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
				onLog(&member, "notify leave")
			}
		case serf.EventMemberJoin:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.eventCh <- genEvent(mb.EventJoin, &member)
				onLog(&member, "notify join")
			}
		case serf.EventMemberUpdate:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.eventCh <- genEvent(mb.EventUpdate, &member)
				onLog(&member, "notify update")
			}
		case serf.EventMemberReap:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.eventCh <- genEvent(mb.EventReap, &member)
				onLog(&member, "notify reap")
			}
		case serf.EventUser:
			ue := e.(serf.UserEvent)
			if ue.Name == m.config.NodeName {
				continue
			}
			m.msgCh <- ue.Payload
			//zero.Info().Str("name", ue.Name).Msg("serf message")
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

func (m *Membership) send(to memberlist.Address, msg []byte) error {
	return m.serf.Memberlist().SendToAddress(to, msg)
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
		zero.Error().Err(err).Str("from", m.config.NodeName).Str("to", nodeName).Msg("send to node")
		return err
	}

	return nil
}

func (m *Membership) Broadcast(msg []byte) {
	m.serf.UserEvent(m.config.NodeName, msg, false)
}

func onLog(node *serf.Member, prompt string) {
	zero.Info().Str("node", node.Name).Str("addr", node.Addr.String()).Msg(prompt)
}

func (m *Membership) isLocal(member serf.Member) bool {
	return m.serf.LocalMember().Name == member.Name
}

func (m *Membership) logError(err error, msg string, name string) {
	zero.Error().Err(err).Str("node", name).Msg(msg)
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
