// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package etcd

import (
	"bytes"
	"encoding/gob"
	"github.com/wind-c/comqtt/cluster/log/zero"
	"github.com/wind-c/comqtt/cluster/message"
	"github.com/wind-c/comqtt/mqtt/packets"
	"sync"

	"go.etcd.io/etcd/raft/v3/raftpb"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
)

// KVStore is a key-value store backed by raft
type KVStore struct {
	mu          sync.RWMutex
	data        map[string][]string // current committed key-value pairs
	snapshotter *snap.Snapshotter
	commitC     <-chan *commit
	errorC      <-chan error
	notifyCh    chan<- *message.Message
}

func newKVStore(snapshotter *snap.Snapshotter, commitC <-chan *commit, errorC <-chan error, notifyCh chan<- *message.Message) *KVStore {
	s := &KVStore{
		data:        make(map[string][]string),
		snapshotter: snapshotter,
		commitC:     commitC,
		errorC:      errorC,
		notifyCh:    notifyCh,
	}
	snapshot, err := s.loadSnapshot()
	if err != nil {
		zero.Fatal().Err(err).Msg("[store] load snapshot")
	}
	if snapshot != nil {
		zero.Info().Uint64("term", snapshot.Metadata.Term).Uint64("index", snapshot.Metadata.Index).Msg("[store] loading snapshot at term and index")
		if err := s.recoverFromSnapshot(snapshot.Data); err != nil {
			zero.Fatal().Err(err).Msg("[store] recover snapshot")
		}
	}
	// read commits from raft into kvStore map until error
	go s.readCommits()
	return s
}

func (s *KVStore) Lookup(key string) ([]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *KVStore) Del(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if vs, ok := s.data[key]; ok {
		if value == "" || len(vs) == 1 {
			delete(s.data, key)
			return
		}

		for i, item := range vs {
			if item == value {
				s.data[key] = append(vs[:i], vs[i+1:]...)
			}
		}
	}
}

func (d *KVStore) DelByValue(value string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := 0
	for k, vs := range d.data {
		for i, v := range vs {
			if v == value {
				if len(vs) == 1 {
					delete(d.data, k)
				} else {
					d.data[k] = append(vs[:i], vs[i+1:]...)
				}
				c++
			}
		}
	}
	return c
}

func (s *KVStore) Add(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if vs, ok := s.data[key]; ok {
		for _, item := range vs {
			if item == value {
				return
			}
		}
		s.data[key] = append(vs, value)
	} else {
		s.data[key] = []string{value}
	}
}

func (s *KVStore) GetErrorC(key, value string) <-chan error {
	return s.errorC
}

func (s *KVStore) readCommits() {
	for commit := range s.commitC {
		if commit == nil {
			// signaled to load snapshot
			snapshot, err := s.loadSnapshot()
			if err != nil {
				zero.Fatal().Err(err).Msg("[store] load snapshot")
			}
			if snapshot != nil {
				zero.Info().Uint64("term", snapshot.Metadata.Term).Uint64("index", snapshot.Metadata.Index).Msg("[store] loading snapshot at term and index")
				if err := s.recoverFromSnapshot(snapshot.Data); err != nil {
					zero.Fatal().Err(err).Msg("[store] recover snapshot")
				}
			}
			continue
		}

		for _, data := range commit.data {
			var msg message.Message
			if err := msg.MsgpackLoad(data); err != nil {
				continue
			}
			if msg.Type == packets.Subscribe {
				s.Add(string(msg.Payload), msg.NodeID)
			} else if msg.Type == packets.Unsubscribe {
				s.Del(string(msg.Payload), msg.NodeID)
			} else {
				continue
			}

			s.notifyCh <- &msg
		}
		close(commit.applyDoneC)
	}
	if err, ok := <-s.errorC; ok {
		zero.Fatal().Err(err).Msg("[store] read commit")
	}
}

func (s *KVStore) getSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(s.data); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *KVStore) loadSnapshot() (*raftpb.Snapshot, error) {
	if snapshot, err := s.snapshotter.Load(); err != nil {
		if err == snap.ErrNoSnapshot {
			return nil, nil
		} else {
			return nil, err
		}
	} else {
		return snapshot, nil
	}
}

func (s *KVStore) recoverFromSnapshot(snapshot []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	buffer := bytes.NewBuffer(snapshot)
	if err := gob.NewDecoder(buffer).Decode(&s.data); err != nil {
		return err
	}
	s.notifyReplay()
	return nil
}

func (s *KVStore) notifyReplay() {
	for filter, ns := range s.data {
		for _, nodeId := range ns {
			msg := message.Message{
				Type:    packets.Subscribe,
				NodeID:  nodeId,
				Payload: []byte(filter),
			}
			s.notifyCh <- &msg
		}
	}
}
