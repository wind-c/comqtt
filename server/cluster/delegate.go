package cluster

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/memberlist"
	mqtt "github.com/wind-c/comqtt/server"
	"github.com/wind-c/comqtt/server/system"
	"sync"
	"time"
)

// Maximum number of messages to be held in the queue.
const maxQueueSize = 4096

type Broadcast struct {
	msg    []byte
	notify chan<- struct{}
}

func (b *Broadcast) Invalidates(other memberlist.Broadcast) bool {
	return false
}

func (b *Broadcast) Message() []byte {
	return b.msg
}

func (b *Broadcast) Finished() {
	if b.notify != nil {
		close(b.notify)
	}
}

type Delegate struct {
	sync.RWMutex
	stop       chan struct{}
	Mch        chan []byte
	State      map[string]*system.Info
	Broadcasts *memberlist.TransmitLimitedQueue
	LocalNode  *memberlist.Node
	server     *mqtt.Server
}

func NewDelegate() *Delegate {
	 d := &Delegate{
		 Mch: make(chan []byte, 1024),
		State: make(map[string]*system.Info, 2),
	}
	go d.handleQueueDepth()
	return d
}

func (d *Delegate) Stop() {
	d.stop <- struct{}{}
}

func (d *Delegate) NotifyMsg(msg []byte) {
	d.Mch <- msg
}

func (d *Delegate) NodeMeta(limit int) []byte {
	return []byte{}
}

func (d *Delegate) LocalState(join bool) []byte {
	if d.server != nil {
		d.State[d.LocalNode.Name] = d.server.System
	}
	bs, _ := json.Marshal(d.State)
	return bs
}

func (d *Delegate) GetBroadcasts(overhead, limit int) [][]byte {
	return d.Broadcasts.GetBroadcasts(overhead, limit)
}

func (d *Delegate) MergeRemoteState(buf []byte, join bool) {
	var m map[string]*system.Info
	if err := json.Unmarshal(buf, &m); err != nil {
		return
	}
	d.Lock()
	for k, v := range m {
		if k == d.LocalNode.Name {
			continue
		}
		d.State[k] = v
	}
	d.Unlock()
}

func (d *Delegate) InitBroadcasts(list *memberlist.Memberlist) {
	d.LocalNode = list.LocalNode()
	d.Broadcasts = &memberlist.TransmitLimitedQueue{
		NumNodes: func() int {
			return list.NumMembers()
		},
		RetransmitMult: 3,
	}
}

func (d *Delegate) BindMqttServer(server *mqtt.Server) {
	d.server = server
}

//Broadcast broadcast to everyone including yourself
func (d *Delegate) Broadcast(data []byte) {
	d.Broadcasts.QueueBroadcast(&Broadcast{
		msg:    data,
		notify: nil,
	})
}

// handleQueueDepth ensures that the queue doesn't grow unbounded by pruning
// older messages at regular interval.
func (d *Delegate) handleQueueDepth() {
	for {
		select {
		case <-d.stop:
			return
		case <-time.After(15 * time.Minute):
			n := d.Broadcasts.NumQueued()
			if n > maxQueueSize {
				fmt.Printf("dropping messages because too many are queued current=%v limit=%v", n, maxQueueSize)
				d.Broadcasts.Prune(maxQueueSize)
			}
		}
	}
}
