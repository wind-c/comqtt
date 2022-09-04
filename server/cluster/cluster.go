package cluster

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/hashicorp/memberlist"
	"github.com/panjf2000/ants/v2"
	mqtt "github.com/wind-c/comqtt/server"
	"github.com/wind-c/comqtt/server/cluster/message"
	"github.com/wind-c/comqtt/server/cluster/raft"
	"github.com/wind-c/comqtt/server/cluster/topics"
	"github.com/wind-c/comqtt/server/events"
	"github.com/wind-c/comqtt/server/internal/packets"
	"github.com/wind-c/comqtt/server/log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Cluster struct {
	Config   *memberlist.Config
	List     *memberlist.Memberlist
	server   *mqtt.Server
	delegate *Delegate
	event    *NodeEvents
	raftPool *ants.PoolWithFunc
	outPool  *ants.PoolWithFunc
	inPool   *ants.Pool
	subTree  *topics.Index
	raftNode *raft.Node
}

func LaunchNode(members string, opts ...Option) (*Cluster, error) {
	conf := NewOptions(opts...)
	cluster, err := Create(conf)
	if err != nil {
		return nil, err
	}
	if len(members) > 0 {
		parts := strings.Split(members, ",")
		_, err := cluster.Join(parts)
		if err != nil {
			return nil, err
		}
	}
	node := cluster.LocalNode()
	fmt.Printf("Local member %s:%d\n", node.Addr, node.Port)
	log.Infof("Local member %s:%d", node.Addr, node.Port)

	return cluster, nil
}

func Create(conf *memberlist.Config) (c *Cluster, err error) {
	if conf == nil {
		conf = memberlist.DefaultLocalConfig()
	}
	delegate := NewDelegate()
	event := NewEvents()
	conf.Delegate = delegate
	conf.Events = event
	list, err := memberlist.Create(conf)
	if err != nil {
		return nil, err
	}
	delegate.InitBroadcasts(list)
	c = &Cluster{
		Config:   conf,
		List:     list,
		delegate: delegate,
		event:    event,
		subTree:  topics.New(),
	}

	//create raft app goroutine pool and raft msg
	gps := runtime.GOMAXPROCS(0)
	rp, err := ants.NewPoolWithFunc(gps/2, func(i interface{}) {
		v, ok := i.([]byte)
		if ok {
			c.triggerRaftApply(v)
		}
	})
	if err != nil {
		return nil, err
	}
	c.raftPool = rp

	//create outbound goroutine pool and outbound msg
	op, err := ants.NewPoolWithFunc(gps, func(i interface{}) {
		v, ok := i.(packets.Packet)
		if ok {
			c.processPacket(v)
		}
	})
	if err != nil {
		return nil, err
	}
	c.outPool = op

	//create inbound goroutine pool and process inbound msg
	ip, err := ants.NewPool(gps)
	if err != nil {
		return nil, err
	}
	c.inPool = ip
	for i := 0; i < gps; i++ {
		c.inPool.Submit(c.processInboundMsg)
	}

	//process node event
	go c.processNodeEvent()

	return c, nil
}

func (c *Cluster) BootstrapRaft(raftPort int, raftDir string) error {
	raftAddr := c.LocalNode().Addr.String() + ":" + strconv.Itoa(raftPort)
	raftNode, err := raft.NewRaftNode(raftAddr, c.LocalNode().Name, raftDir)
	if err != nil {
		log.Infof("bootstrap raft error:%s", err)
		return err
	}
	c.raftNode = raftNode
	raftNode.SetListener(c.raftApplyListener)
	joinMsg := message.Message{Type: message.RaftJoin, Data: []byte(c.LocalNode().Name + ":" + raftAddr)}
	c.SendToOthers(joinMsg.Bytes())

	return nil
}

func (c *Cluster) raftApplyListener(op, key, value string) {
	if op == "" || key == "" || value == "" {
		return
	}
	if op == "set" {
		c.subTree.Subscribe(key)
	} else if op == "del" {
		c.subTree.Unsubscribe(key)
	} else {
		return
	}
}

func (c *Cluster) Join(members []string) (int, error) {
	if len(members) > 0 {
		count, err := c.List.Join(members)
		return count, err
	}

	return 0, nil
}

func (c *Cluster) BindMqttServer(server *mqtt.Server) {
	server.Events.OnMessage = c.OnMessage
	server.Events.OnConnect = c.OnConnect
	server.Events.OnSubscribe = c.OnSubscribe
	server.Events.OnUnsubscribe = c.OnUnsubscribe
	server.Events.OnError = c.OnError
	c.server = server
	c.delegate.BindMqttServer(server)
}

func (c *Cluster) LocalNode() *memberlist.Node {
	return c.List.LocalNode()
}

func (c *Cluster) Members() []*memberlist.Node {
	return c.List.Members()
}

func (c *Cluster) NumMembers() int {
	return c.List.NumMembers()
}

func (c *Cluster) GetNodeByIP(ipAddr net.IP) *memberlist.Node {
	members := c.Members()
	for _, node := range members {
		if node.Name == c.Config.Name {
			continue // skip self
		}
		if node.Addr.To4().Equal(ipAddr.To4()) {
			return node
		}
	}
	return nil
}

func (c *Cluster) send(to *memberlist.Node, msg []byte) error {
	//return c.List.SendReliable(to, msg)      //tcp reliable
	return c.List.SendBestEffort(to, msg) //udp unreliable
}

// SendToOthers send message to all nodes except yourself
func (c *Cluster) SendToOthers(msg []byte) {
	for _, node := range c.Members() {
		if node.Name == c.Config.Name {
			continue // skip self
		}
		c.send(node, msg)
	}
}

// SendToNode send message to a node
func (c *Cluster) SendToNode(nodename string, msg []byte) {
	for _, node := range c.Members() {
		if node.Name == nodename {
			c.send(node, msg)
			return
		}
	}
}

func (c *Cluster) Broadcast(msg []byte) {
	c.delegate.Broadcast(msg)
}

func (c *Cluster) Stop() {
	c.delegate.Stop()
	c.outPool.Release()
	c.inPool.Release()
	c.raftPool.Release()
	c.raftNode.Shutdown()
	c.List.Shutdown()
}

func (c *Cluster) processNodeEvent() {
	for {
		select {
		case event := <-c.event.ech:
			if event.Type == EventLeave {
				//delete(c.routes, event.Node.Name)
				c.raftNode.DelByNode(event.Node.Name)
			}
		}
	}
}

// ProcessInboundMsg process messages from other nodes in the cluster
func (c *Cluster) processInboundMsg() {
	for {
		select {
		case bs := <-c.delegate.Mch:
			var msg message.Message
			msg.Load(bs)
			switch msg.Type {
			case message.RaftJoin:
				na := strings.Split(string(msg.Data), ":")
				if len(na) == 3 {
					c.raftNode.Join(na[0], na[1]+":"+na[2])
				}
			case message.RaftApply:
				c.raftNode.Apply(msg.Data, 5*time.Second)
			case packets.Publish:
				pk := &packets.Packet{FixedHeader: packets.FixedHeader{Type: packets.Publish}}
				pk.FixedHeader.Decode(msg.Data[0])    // Unpack fixedheader.
				err := pk.PublishDecode(msg.Data[2:]) // Unpack skips fixedheader.
				if err != nil {
					break
				}
				c.server.Publish(pk.TopicName, pk.Payload, false)
			case packets.Connect:
				//If a client is connected to another node, the client's data cached on the node needs to be cleared
				nc := strings.Split(string(msg.Data), ":")
				//nodeName := nc[0]
				cid := nc[1]
				if existing, ok := c.server.Clients.Get(cid); ok {
					existing.Stop(errors.New("connection from other node"))
					// clean local session
					c.server.CleanSession(existing)
				}
			}

			//log.Infof("processed a broadcast message : %s", string(msg.Bytes()))
		}
	}
}

func (c *Cluster) OnMessage(cl events.Client, pk packets.Packet) (pkx packets.Packet, err error) {
	if pk.ClientIdentifier == "" {
		pk.ClientIdentifier = cl.ID
	}
	c.outPool.Invoke(pk)
	return pk, nil
}

func (c *Cluster) OnConnect(cl events.Client, pk packets.Packet) {
	// broadcast only when the client connects to local node for the first time
	if cl.First {
		c.outPool.Invoke(pk)
	}
}

func (c *Cluster) OnSubscribe(filter string, cl events.Client, qos byte, isFirst bool) {
	if !isFirst || filter == "" {
		return
	}
	cmd := "set:" + filter + ":" + c.LocalNode().Name
	//c.triggerRaftApply([]byte(cmd))
	c.raftPool.Invoke([]byte(cmd))
}

func (c *Cluster) OnUnsubscribe(filter string, cl events.Client, isLast bool) {
	if !isLast || filter == "" {
		return
	}
	cmd := "del:" + filter + ":" + c.LocalNode().Name
	//c.triggerRaftApply([]byte(cmd))
	c.raftPool.Invoke([]byte(cmd))
}

func (c *Cluster) triggerRaftApply(cmd []byte) {
	if c.raftNode.IsLeader() {
		err := c.raftNode.Apply([]byte(cmd), 3*time.Second)
		if err != nil {
			log.Error(err)
		}
	} else { //send to leader apply
		msg := message.Message{Type: message.RaftApply, Data: []byte(cmd)}
		_, leaderId := c.raftNode.GetLeader()
		c.SendToNode(leaderId, msg.Bytes())
	}
}

func (c *Cluster) OnError(cl events.Client, err error) {
	log.Error(err, log.String("cid", cl.ID), log.String("rm", cl.Remote), log.String("lt", cl.Listener))
}

func (c *Cluster) processPacket(pk packets.Packet) {
	msg := message.Message{}
	switch pk.FixedHeader.Type {
	case packets.Publish:
		var buf bytes.Buffer
		pk.PublishEncode(&buf)
		msg.Type = packets.Publish
		msg.Data = buf.Bytes()
		filters := c.subTree.Scan(pk.TopicName)
		for _, filter := range filters {
			ns := c.raftNode.Search(filter)
			for _, node := range ns {
				if node != c.LocalNode().Name {
					c.SendToNode(node, msg.Bytes())
				}
			}
		}
	case packets.Connect:
		msg.Type = packets.Connect
		msg.Data = []byte(c.LocalNode().Name + ":" + pk.ClientIdentifier)
		c.SendToOthers(msg.Bytes())
	}
	//log.Info("broadcast package", log.String("cid", pk.ClientIdentifier), log.Uint8("pt", pk.FixedHeader.Type),
	//	log.Uint16("pid", pk.PacketID), log.String("pl", string(pk.Payload)))
}
