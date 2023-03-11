// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package cluster

import (
	"bytes"
	"context"
	"errors"
	"github.com/panjf2000/ants/v2"
	"github.com/wind-c/comqtt/cluster/discovery"
	"github.com/wind-c/comqtt/cluster/discovery/mlist"
	"github.com/wind-c/comqtt/cluster/discovery/serf"
	"github.com/wind-c/comqtt/cluster/log/zero"
	"github.com/wind-c/comqtt/cluster/message"
	"github.com/wind-c/comqtt/cluster/raft"
	"github.com/wind-c/comqtt/cluster/raft/etcd"
	"github.com/wind-c/comqtt/cluster/raft/hashicorp"
	"github.com/wind-c/comqtt/cluster/topics"
	"github.com/wind-c/comqtt/cluster/utils"
	"github.com/wind-c/comqtt/config"
	"github.com/wind-c/comqtt/mqtt"
	"github.com/wind-c/comqtt/mqtt/packets"
	"net"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
)

const (
	DirectionInbound byte = iota
	DirectionOutbound
)

const (
	NodesFile = "nodes.json"
	PeersFIle = "peers.json"
)

type Agent struct {
	membership        discovery.Node
	ctx               context.Context
	cancel            context.CancelFunc
	Config            *config.Cluster
	mqttServer        *mqtt.Server
	grpcService       *RpcService
	grpcClientManager *ClientManager
	raftPool          *ants.PoolWithFunc
	OutPool           *ants.PoolWithFunc
	inPool            *ants.Pool
	subTree           *topics.Index
	raftPeer          raft.IPeer
	raftNotifyCh      chan *message.Message
	inboundMsgCh      chan []byte
	grpcMsgCh         chan *message.Message
}

func NewAgent(conf *config.Cluster) *Agent {
	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{
		ctx:          ctx,
		cancel:       cancel,
		Config:       conf,
		subTree:      topics.New(),
		raftNotifyCh: make(chan *message.Message, 1024),
		inboundMsgCh: make(chan []byte, 10240),
		grpcMsgCh:    make(chan *message.Message, 10240),
	}
}

func (a *Agent) Start() (err error) {
	// setup raft
	if a.Config.RaftPort == 0 {
		a.Config.RaftPort = 8946
	}
	if a.Config.RaftDir == "" {
		a.Config.RaftDir = path.Join("data", a.Config.NodeName)
	}

	// listen for raft apply notifications
	go a.raftApplyListener()

	if a.Config.RaftImpl == config.RaftImplEtcd {
		if a.raftPeer, err = etcd.Setup(a.Config, a.raftNotifyCh); err != nil {
			return
		}
	} else {
		if a.raftPeer, err = hashicorp.Setup(a.Config, a.raftNotifyCh); err != nil {
			return
		}
	}
	raftAddr := net.JoinHostPort(a.Config.BindAddr, strconv.Itoa(a.Config.RaftPort))
	OnJoinLog(a.Config.NodeName, raftAddr, "setup raft", nil)

	// create and join cluster
	if utils.PathExists(a.getNodesFile()) {
		ms := discovery.GenMemberAddrs(discovery.ReadMembers(a.getNodesFile()))
		if ms != nil {
			a.Config.Members = ms
		}
	}

	switch a.Config.DiscoveryWay {
	case config.DiscoveryWayMemberlist:
		a.membership = mlist.New(a.Config, a.inboundMsgCh)
	default:
		a.membership = serf.New(a.Config, a.inboundMsgCh)
	}

	a.membership.BindMqttServer(a.mqttServer)
	if err := a.membership.Setup(); err != nil {
		return err
	}

	// start grpc server
	if a.Config.GrpcEnable {
		a.grpcService = NewRpcService(a)
		a.grpcClientManager = NewClientManager(a)
		if err := a.grpcService.StartRpcServer(); err != nil {
			return err
		}
		zero.Info().Str("addr", net.JoinHostPort(a.Config.BindAddr, strconv.Itoa(a.Config.GrpcPort))).Msg("grpc listen at")
	}

	// init goroutine pool
	a.initPool()

	// notification of a new raft peer
	//a.notifyNewRaftPeer()

	// process node event
	go a.processNodeEvent()

	return nil
}

func (a *Agent) initPool() error {
	gps := runtime.GOMAXPROCS(0)
	// create raft app goroutine pool and raft msg
	rp, err := ants.NewPoolWithFunc(gps/2, func(i interface{}) {
		if v, ok := i.(*message.Message); ok {
			a.raftPropose(v)
		}
	})
	if err != nil {
		return err
	}
	a.raftPool = rp

	// create outbound goroutine pool and outbound msg
	op, err := ants.NewPoolWithFunc(gps, func(i interface{}) {
		if v, ok := i.(*packets.Packet); ok {
			a.processOutboundPacket(v)
		}
	})
	if err != nil {
		return err
	}
	a.OutPool = op

	// create inbound goroutine pool and process inbound msg
	if ip, err := ants.NewPool(gps); err != nil {
		return err
	} else {
		a.inPool = ip
		for i := 0; i < gps; i++ {
			a.inPool.Submit(a.processInboundMsg)
		}
	}

	return nil
}

func (a *Agent) Stop() {
	defer func() {
		if err := recover(); err != nil {
			zero.Error().Msg("not graceful stop")
		}
	}()
	a.cancel()
	a.OutPool.Release()
	a.raftPool.Release()
	if a.inPool != nil {
		a.inPool.Release()
	}

	// stop raft
	zero.Info().Msg("stopping raft...")
	a.raftPeer.Stop()
	zero.Info().Msg("raft stopped")

	// stop node
	zero.Info().Msg("stopping node...")
	a.membership.Stop()
	a.grpcService.StopRpcServer()
	zero.Info().Msg("grpc server stopped")
	zero.Info().Msg("node stopped")
	zero.Close()
}

func (a *Agent) BindMqttServer(server *mqtt.Server) {
	server.AddHook(new(MqttEventHook), a)
	a.mqttServer = server
}

func (a *Agent) GetLocalName() string {
	return a.Config.NodeName
}

func (a *Agent) getNodeMember(nodeId string) *discovery.Member {
	for _, m := range a.membership.Members() {
		if m.Name == nodeId {
			return &m
		}
	}
	return nil
}

func (a *Agent) GetMemberList() []discovery.Member {
	return a.membership.Members()
}

func getRaftPeerAddr(member *discovery.Member) string {
	// using serf
	if raftPort, ok := member.Tags[discovery.TagRaftPort]; ok {
		return net.JoinHostPort(member.Addr, raftPort)
	}

	// using memberlist
	return net.JoinHostPort(member.Addr, strconv.Itoa(member.Port+1000))
}

func getGrpcAddr(member *discovery.Member) string {
	// using serf
	if grpcPort, ok := member.Tags[discovery.TagGrpcPort]; ok {
		return net.JoinHostPort(member.Addr, grpcPort)
	}

	// using memberlist
	return net.JoinHostPort(member.Addr, strconv.Itoa(member.Port+10000))
}

func (a *Agent) Stat() map[string]int64 {
	return a.membership.Stat()
}

func (a *Agent) notifyNewRaftPeer() {
	addr := net.JoinHostPort(a.Config.BindAddr, strconv.Itoa(a.Config.RaftPort))
	joinMsg := message.Message{
		Type:    message.RaftJoin,
		NodeID:  a.Config.NodeName,
		Payload: []byte(addr)}

	if a.Config.GrpcEnable {
		a.grpcClientManager.RaftJoinToOthers()
	} else {
		a.membership.SendToOthers(joinMsg.MsgpackBytes())
	}
}

func (a *Agent) raftApplyListener() {
	for {
		select {
		case msg := <-a.raftNotifyCh:
			if msg.NodeID == "" || msg.NodeID == a.GetLocalName() || len(msg.Payload) == 0 {
				continue
			}
			filter := string(msg.Payload)
			if msg.Type == packets.Subscribe {
				a.subTree.Subscribe(filter)
			} else if msg.Type == packets.Unsubscribe {
				a.subTree.Unsubscribe(filter)
			} else {
				continue
			}
			zero.Info().Str("from", msg.NodeID).Str("filter", filter).Uint8("type", msg.Type).Msg("apply listening")
		case <-a.ctx.Done():
			return
		}
	}
}

// send the message to the leader apply
func (a *Agent) raftPropose(msg *message.Message) {
	if a.raftPeer.IsApplyRight() {
		err := a.raftPeer.Propose(msg)
		OnApplyLog(a.GetLocalName(), msg.NodeID, msg.Type, msg.Payload, "apply raft log", err)
	} else { //send to leader apply
		_, leaderId := a.raftPeer.GetLeader()
		if leaderId == "" {
			if a.Config.GrpcEnable {
				a.grpcClientManager.RaftApplyToOthers(msg)
			} else {
				a.membership.SendToOthers(msg.MsgpackBytes())
			}
			OnApplyLog("unknown", msg.NodeID, msg.Type, msg.Payload, "broadcast raft log", nil)
		} else {
			if a.Config.GrpcEnable {
				a.grpcClientManager.RelayRaftApply(leaderId, msg)
			} else {
				a.membership.SendToNode(leaderId, msg.MsgpackBytes())
			}
			OnApplyLog(leaderId, msg.NodeID, msg.Type, msg.Payload, "forward raft log", nil)
		}
	}
}

func (a *Agent) getNodesFile() string {
	return a.Config.NodeName + "-" + NodesFile
}

func (a *Agent) getPeersFile() string {
	return filepath.Join(a.Config.RaftDir, PeersFIle)
}

func (a *Agent) genNodesFile() {
	if err := discovery.GenNodesFile(a.getNodesFile(), a.membership.Members()); err != nil {
		zero.Error().Err(err).Msg("gen nodes file")
	}
	if err := a.raftPeer.GenPeersFile(a.getPeersFile()); err != nil {
		zero.Error().Err(err).Msg("gen peers file")
	}
}

// processNodeEvent handle events of node join and leave
func (a *Agent) processNodeEvent() {
	for {
		select {
		case event := <-a.membership.EventChan():
			var err error
			prompt := "raft joining"
			nodeName := event.Name
			addr := getRaftPeerAddr(&event.Member)
			//addr := event.Addr
			if event.Type == discovery.EventJoin {
				if nodeName != a.GetLocalName() && a.raftPeer.IsApplyRight() {
					err = a.raftPeer.Join(nodeName, addr)
					prompt = "raft joined"
				}
			} else if event.Type == discovery.EventLeave {
				err = a.raftPeer.Leave(nodeName)
				if a.Config.GrpcEnable {
					a.grpcClientManager.RemoveGrpcClient(nodeName)
				}
				prompt = "raft leaved"
			} else {
				prompt = "raft updated"
			}
			OnJoinLog(nodeName, addr, prompt, err)
			go a.genNodesFile()
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *Agent) processRelayMsg(msg *message.Message) {
	switch msg.Type {
	case message.RaftJoin:
		addr := string(msg.Payload)
		err := a.raftPeer.Join(msg.NodeID, addr)
		OnJoinLog(msg.NodeID, addr, "raft join", err)
	case packets.Subscribe, packets.Unsubscribe:
		a.raftPropose(msg)
	case packets.Publish:
		pk := packets.Packet{FixedHeader: packets.FixedHeader{Type: packets.Publish}}
		pk.ProtocolVersion = msg.ProtocolVersion
		pk.Origin = msg.ClientID
		pk.FixedHeader.Decode(msg.Payload[0])                     // Unpack fixedheader.
		if err := pk.PublishDecode(msg.Payload[2:]); err == nil { // Unpack skips fixedheader
			a.mqttServer.PublishToSubscribers(pk)
			OnPublishPacketLog(DirectionInbound, msg.NodeID, msg.ClientID, pk.TopicName, pk.PacketID)
		}
	case packets.Connect:
		//If a client is connected to another node, the client's data cached on the node needs to be cleared
		if existing, ok := a.mqttServer.Clients.Get(msg.ClientID); ok {
			existing.Stop(errors.New("connection from other node"))
			// clean local session and subscriptions
			a.mqttServer.UnsubscribeClient(existing)
			a.mqttServer.Clients.Delete(msg.ClientID)
		}
		OnConnectPacketLog(DirectionInbound, msg.NodeID, msg.ClientID)
	}
}

// ProcessInboundMsg process messages from other nodes in the cluster
func (a *Agent) processInboundMsg() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case bs := <-a.inboundMsgCh:
			var msg message.Message
			if err := msg.MsgpackLoad(bs); err == nil {
				a.processRelayMsg(&msg)
			}
		case msg := <-a.grpcMsgCh:
			a.processRelayMsg(msg)
		}
	}
}

// processOutboundPacket process outbound msg
func (a *Agent) processOutboundPacket(pk *packets.Packet) {
	msg := message.Message{
		NodeID:          a.Config.NodeName,
		ClientID:        pk.Origin,
		ProtocolVersion: pk.ProtocolVersion,
	}
	switch pk.FixedHeader.Type {
	case packets.Publish:
		var buf bytes.Buffer
		pk.Mods.AllowResponseInfo = true
		if err := pk.PublishEncode(&buf); err != nil {
			return
		}
		msg.Type = packets.Publish
		msg.Payload = buf.Bytes()
		filters := a.subTree.Scan(pk.TopicName, make([]string, 0))
		oldNodes := make([]string, 0)
		for _, filter := range filters {
			ns := a.raftPeer.Lookup(filter)
			for _, node := range ns {
				if node != a.GetLocalName() && !utils.Contains(oldNodes, node) {
					if a.Config.GrpcEnable {
						a.grpcClientManager.RelayPublishPacket(node, &msg)
					} else {
						bs := msg.MsgpackBytes()
						a.membership.SendToNode(node, bs)
					}
					oldNodes = append(oldNodes, node)
					OnPublishPacketLog(DirectionOutbound, node, pk.Origin, pk.TopicName, pk.PacketID)
				}
			}
		}
	case packets.Connect:
		msg.Type = packets.Connect
		if msg.ClientID == "" {
			msg.ClientID = pk.Connect.ClientIdentifier
		}
		if a.Config.GrpcEnable {
			a.grpcClientManager.ConnectNotifyToOthers(&msg)
		} else {
			a.membership.SendToOthers(msg.MsgpackBytes())
		}
		OnConnectPacketLog(DirectionOutbound, a.GetLocalName(), msg.ClientID)
	}
}

func OnJoinLog(nodeId, addr, prompt string, err error) {
	logEvent := zero.Info()
	if err != nil {
		logEvent.Err(err)
	}
	logEvent.Str("node", nodeId).Str("addr", addr).Msg(prompt)
}

func OnApplyLog(leaderId, nodeId string, tp byte, filter []byte, prompt string, err error) {
	logEvent := zero.Info()
	if err != nil {
		logEvent.Err(err)
	}
	logEvent.Str("leader", leaderId).Str("from", nodeId).Uint8("type", tp).Bytes("filter", filter).Msg(prompt)
}

func OnPublishPacketLog(direction byte, nodeId, cid, topic string, pid uint16) {
	logEvent := zero.Info()
	if direction == DirectionInbound {
		logEvent.Str("d", "inbound").Str("from", nodeId)
	} else {
		logEvent.Str("d", "outbound").Str("to", nodeId)
	}
	logEvent.Str("cid", cid).Uint16("pid", pid).Str("topic", topic).Msg("publish message")
}

func OnConnectPacketLog(direction byte, node, clientId string) {
	logEvent := zero.Info()
	if direction == DirectionInbound {
		logEvent.Str("d", "inbound").Str("from", node)
	} else {
		logEvent.Str("d", "outbound").Str("to", node)
	}
	logEvent.Str("cid", clientId).Msg("connection notification")
}

func (a *Agent) AddRaftPeer(id, addr string) {
	a.raftPeer.Join(id, addr)
	zero.Info().Str("nid", id).Str("addr", addr).Msg("add peer")
}

func (a *Agent) RemoveRaftPeer(id string) {
	a.raftPeer.Leave(id)
	zero.Info().Str("nid", id).Msg("remove peer")
}

func (a *Agent) GetValue(key string) []string {
	zero.Info().Str("key", key).Msg("get value")
	return a.raftPeer.Lookup(key)
}
