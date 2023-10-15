// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package mlist

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/wind-c/comqtt/v2/cluster/log"
	mqtt "github.com/wind-c/comqtt/v2/mqtt"
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
	msgCh      chan<- []byte
	State      map[string]int64
	Broadcasts *memberlist.TransmitLimitedQueue
	LocalNode  *memberlist.Node
	server     *mqtt.Server
}

func NewDelegate(inboundMsgCh chan<- []byte) *Delegate {
	d := &Delegate{
		msgCh: inboundMsgCh,
		State: make(map[string]int64, 2),
	}
	go d.handleQueueDepth()
	return d
}

func (d *Delegate) Stop() {
	d.stop <- struct{}{}
	close(d.msgCh)
}

func (d *Delegate) NotifyMsg(msg []byte) {
	d.msgCh <- msg
}

func (d *Delegate) NodeMeta(limit int) []byte {
	return []byte{}
}

func (d *Delegate) LocalState(join bool) []byte {
	if d.server != nil {
		d.State[d.LocalNode.Name] = d.server.Info.ClientsConnected
	}
	bs, _ := json.Marshal(d.State)
	return bs
}

func (d *Delegate) GetBroadcasts(overhead, limit int) [][]byte {
	return d.Broadcasts.GetBroadcasts(overhead, limit)
}

func (d *Delegate) MergeRemoteState(buf []byte, join bool) {
	var m map[string]int64
	if err := json.Unmarshal(buf, &m); err != nil {
		return
	}
	d.Lock()
	defer d.Unlock()
	for k, v := range m {
		if k == d.LocalNode.Name {
			continue
		}
		d.State[k] = v
	}
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

// Broadcast broadcast to everyone including yourself
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
				log.Info("delete messages", "current", n, "limit", maxQueueSize)
				d.Broadcasts.Prune(maxQueueSize)
			}
		}
	}
}
