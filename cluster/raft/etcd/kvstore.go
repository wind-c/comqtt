// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package etcd

import (
	"bytes"
	"encoding/gob"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/message"
	base "github.com/wind-c/comqtt/v2/cluster/raft"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/raft/v3/raftpb"
	"strings"
)

// KVStore is a key-value store backed by raft
type KVStore struct {
	*base.KV
	snapshotter *snap.Snapshotter
	commitC     <-chan *commit
	errorC      <-chan error
	notifyCh    chan<- *message.Message
}

func newKVStore(snapshotter *snap.Snapshotter, commitC <-chan *commit, errorC <-chan error, notifyCh chan<- *message.Message) *KVStore {
	s := &KVStore{
		KV:          base.NewKV(),
		snapshotter: snapshotter,
		commitC:     commitC,
		errorC:      errorC,
		notifyCh:    notifyCh,
	}
	snapshot, err := s.loadSnapshot()
	if err != nil {
		log.Fatal("[store] load snapshot", "error", err)
	}
	if snapshot != nil {
		log.Info("[store] loading snapshot at term and index", "term", snapshot.Metadata.Term, "index", snapshot.Metadata.Index)
		if err := s.recoverFromSnapshot(snapshot.Data); err != nil {
			log.Fatal("[store] recover snapshot", "error", err)
		}
	}
	// read commits from raft into kvStore map until error
	go s.readCommits()
	return s
}

func (s *KVStore) Lookup(key string) []string {
	return s.Get(key)
}

func (s *KVStore) DelByNode(node string) int {
	return s.DelByValue(node)
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
				log.Fatal("[store] load snapshot", "error", err)
			}
			if snapshot != nil {
				log.Info("[store] loading snapshot at term and index", "term", snapshot.Metadata.Term, "index", snapshot.Metadata.Index)
				if err := s.recoverFromSnapshot(snapshot.Data); err != nil {
					log.Fatal("[store] recover snapshot", "error", err)
				}
			}
			continue
		}

		for _, data := range commit.data {
			var msg message.Message
			if err := msg.MsgpackLoad(data); err != nil {
				continue
			}
			filter := string(msg.Payload)
			deliverable := false
			if msg.Type == packets.Subscribe {
				deliverable = s.Add(filter, msg.NodeID)
			} else if msg.Type == packets.Unsubscribe {
				deliverable = s.Del(filter, msg.NodeID)
			} else {
				continue
			}
			log.Info("raft apply", "from", msg.NodeID, "filter", filter, "type", msg.Type)
			if s.notifyCh != nil && deliverable {
				s.notifyCh <- &msg
			}
		}
		close(commit.applyDoneC)
	}
	if err, ok := <-s.errorC; ok {
		log.Fatal("[store] read commit", "error", err)
	}
}

func (s *KVStore) getSnapshot() ([]byte, error) {
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(s.GetAll()); err != nil {
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
	buffer := bytes.NewBuffer(snapshot)
	if err := gob.NewDecoder(buffer).Decode(s.GetAll()); err != nil {
		return err
	}
	s.notifyReplay()
	return nil
}

func (s *KVStore) notifyReplay() {
	for filter, ns := range *s.GetAll() {
		msg := message.Message{
			Type:    packets.Subscribe,
			NodeID:  strings.Join(ns, ","),
			Payload: []byte(filter),
		}
		s.notifyCh <- &msg
		log.Info("raft replay", "from", msg.NodeID, "filter", filter, "type", msg.Type)
	}
}
