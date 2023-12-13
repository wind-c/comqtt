// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package raft

import (
	"github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/cluster/utils"
	"sync"
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
	return &k.data
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

		if value == "" || len(vs) == 0 {
			delete(k.data, key)
			empty = true
		}
	}
	return
}

func (k *KV) DelByValue(value string) int {
	k.Lock()
	defer k.Unlock()
	c := 0
	for f, vs := range k.data {
		for i, v := range vs {
			if v == value {
				if len(vs) == 1 {
					delete(k.data, f)
				} else {
					k.data[f] = append(vs[:i], vs[i+1:]...)
				}
				c++
			}
		}
	}
	return c
}
