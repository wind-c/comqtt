// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package raft

import (
	"encoding/gob"
	"io"
	"sync"

	"github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/cluster/utils"
)

type IPeer interface {
	Join(nodeID, addr string) error
	Leave(nodeID string) error
	Propose(msg *message.Message) error
	Lookup(key string) []string
	IsApplyRight() bool
	GetLeader() (addr, id string)
	GenPeersFile(file string) error
	Stop()
}

type data map[string][]string

type KV struct {
	data data
	sync.RWMutex
}

func NewKV() *KV {
	return &KV{
		data: make(map[string][]string),
	}
}

func (k *KV) GetAll() *data {
	k.RLock()
	defer k.RUnlock()
	cp := make(data, len(k.data))
	for k, v := range k.data {
		cp[k] = append([]string(nil), v...)
	}
	return &cp
}

// Restore replaces the store contents with gob-encoded data read from r.
// A raft snapshot holds the complete state up to its index, so existing
// entries are discarded rather than merged.
func (k *KV) Restore(r io.Reader) error {
	var d data
	if err := gob.NewDecoder(r).Decode(&d); err != nil {
		return err
	}
	if d == nil {
		d = make(data)
	}
	k.Lock()
	defer k.Unlock()
	k.data = d
	return nil
}

func (k *KV) Get(key string) []string {
	k.RLock()
	defer k.RUnlock()
	vs := k.data[key]
	return vs
}

// Add return true if key is set for the first time
func (k *KV) Add(key, value string) (new bool) {
	k.Lock()
	defer k.Unlock()
	if vs, ok := k.data[key]; ok {
		if utils.Contains(vs, value) {
			return
		}
		k.data[key] = append(vs, value)
	} else {
		k.data[key] = []string{value}
		new = true
	}
	return
}

// Del return true if the array corresponding to key is deleted
// If the value is "", the key-values pair is deleted
func (k *KV) Del(key, value string) (empty bool) {
	k.Lock()
	defer k.Unlock()
	if vs, ok := k.data[key]; ok {
		if utils.Contains(vs, value) {
			for i, item := range vs {
				if item == value {
					k.data[key] = append(vs[:i], vs[i+1:]...)
				}
			}
		}

		if value == "" || len(k.data[key]) == 0 {
			delete(k.data, key)
			empty = true
		}
	}
	return
}

// DelByValue delete the specified value from the key-values array
// and delete the key-value pair if the key-values array is empty
func (k *KV) DelByValue(value string) int {
	k.Lock()
	defer k.Unlock()
	c := 0
	if value == "" {
		return c
	}

	for f, vs := range k.data {
		for i, v := range vs {
			if v == value {
				k.data[f] = append(vs[:i], vs[i+1:]...)
				c++
			}
		}
		if len(k.data[f]) == 0 {
			delete(k.data, f)
		}
	}
	return c
}
