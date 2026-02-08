// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package hashicorp

import (
	"bytes"
	"encoding/gob"
	"io"
	"strings"

	"github.com/hashicorp/raft"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/message"
	base "github.com/wind-c/comqtt/v2/cluster/raft"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

type Fsm struct {
	*base.KV
	notifyCh chan<- *message.Message
}

func NewFsm(notifyCh chan<- *message.Message) *Fsm {
	fsm := &Fsm{
		KV:       base.NewKV(),
		notifyCh: notifyCh,
	}
	return fsm
}

func (f *Fsm) Apply(l *raft.Log) interface{} {
	var msg message.Message
	if err := msg.MsgpackLoad(l.Data); err != nil {
		return nil
	}
	filter := string(msg.Payload)
	deliverable := false
	if msg.Type == packets.Subscribe {
		deliverable = f.Add(filter, msg.NodeID)
	} else if msg.Type == packets.Unsubscribe {
		deliverable = f.Del(filter, msg.NodeID)
	} else {
		return nil
	}
	log.Info("raft apply", "from", msg.NodeID, "filter", filter, "type", msg.Type)
	if f.notifyCh != nil && deliverable {
		select {
		case f.notifyCh <- &msg:
		default: // channel is full, drop notification but do not block Raft
			log.Warn("notify channel full, dropping notification")
		}
	}

	return nil
}

func (f *Fsm) Lookup(key string) []string {
	return f.Get(key)
}

func (f *Fsm) DelByNode(node string) int {
	return f.DelByValue(node)
}

func (f *Fsm) Snapshot() (raft.FSMSnapshot, error) {
	return f, nil
}

func (f *Fsm) Restore(ir io.ReadCloser) error {
	if err := gob.NewDecoder(ir).Decode(f.GetAll()); err != nil {
		return err
	}
	f.notifyReplay()
	return nil
}

func (f *Fsm) notifyReplay() {
	for filter, ns := range *f.GetAll() {
		msg := message.Message{
			Type:    packets.Subscribe,
			NodeID:  strings.Join(ns, ","),
			Payload: []byte(filter),
		}
		f.notifyCh <- &msg
		log.Info("raft replay", "from", msg.NodeID, "filter", filter, "type", msg.Type)
	}
}

func (f *Fsm) Persist(sink raft.SnapshotSink) error {
	var buffer bytes.Buffer
	err := gob.NewEncoder(&buffer).Encode(f.GetAll())
	if err != nil {
		return err
	}
	sink.Write(buffer.Bytes())
	sink.Close()
	return nil
}

func (f *Fsm) Release() {}
