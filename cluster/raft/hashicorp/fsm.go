// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package hashicorp

import (
	"bytes"
	"encoding/gob"
	"github.com/wind-c/comqtt/cluster/message"
	"github.com/wind-c/comqtt/mqtt/packets"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

type Fsm struct {
	kv       KVStore
	notifyCh chan<- *message.Message
}

func NewFsm(notifyCh chan<- *message.Message) *Fsm {
	fsm := &Fsm{
		kv:       NewKVStore(),
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
	if msg.Type == packets.Subscribe {
		f.kv.Set(filter, msg.NodeID)
		//zero.Info().Str("from", msg.NodeID).Str("event", "subscribe").Str("filter", filter).Msg("apply")
	} else if msg.Type == packets.Unsubscribe {
		f.kv.Del(filter, msg.NodeID)
		//zero.Info().Str("from", msg.NodeID).Str("event", "unsubscribe").Str("filter", filter).Msg("apply")
	} else {
		return nil
	}
	if f.notifyCh != nil {
		f.notifyCh <- &msg
	}

	return nil
}

func (f *Fsm) Search(key string) []string {
	return f.kv.Get(key)
}

func (f *Fsm) DelByNode(node string) int {
	return f.kv.DelByValue(node)
}

func (f *Fsm) Snapshot() (raft.FSMSnapshot, error) {
	return &f.kv, nil
}

func (f *Fsm) Restore(ir io.ReadCloser) error {
	if err := gob.NewDecoder(ir).Decode(&f.kv.Data); err != nil {
		return err
	}
	f.notifyReplay()
	return nil
}

func (f *Fsm) notifyReplay() {
	for filter, ns := range f.kv.Data {
		for _, nodeId := range ns {
			msg := message.Message{
				Type:    packets.Subscribe,
				NodeID:  nodeId,
				Payload: []byte(filter),
			}
			f.notifyCh <- &msg
		}
	}
}

type KVStore struct {
	Data map[string][]string
	mu   sync.RWMutex
}

func NewKVStore() KVStore {
	return KVStore{
		Data: make(map[string][]string),
	}
}

func (d *KVStore) Get(key string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	vs := d.Data[key]
	return vs
}

func (d *KVStore) Set(key, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if vs, ok := d.Data[key]; ok {
		for _, item := range vs {
			if item == value {
				return
			}
		}
		d.Data[key] = append(vs, value)
	} else {
		d.Data[key] = []string{value}
	}
}

func (d *KVStore) Del(key, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if vs, ok := d.Data[key]; ok {
		if value == "" || len(vs) == 1 {
			delete(d.Data, key)
			return
		}

		for i, item := range vs {
			if item == value {
				d.Data[key] = append(vs[:i], vs[i+1:]...)
			}
		}
	}
}

func (d *KVStore) DelByValue(value string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := 0
	for k, vs := range d.Data {
		for i, v := range vs {
			if v == value {
				if len(vs) == 1 {
					delete(d.Data, k)
				} else {
					d.Data[k] = append(vs[:i], vs[i+1:]...)
				}
				c++
			}
		}
	}
	return c
}

func (d *KVStore) Persist(sink raft.SnapshotSink) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var buffer bytes.Buffer
	err := gob.NewEncoder(&buffer).Encode(d.Data)
	if err != nil {
		return err
	}
	sink.Write(buffer.Bytes())
	sink.Close()
	return nil
}

func (d *KVStore) Release() {}
