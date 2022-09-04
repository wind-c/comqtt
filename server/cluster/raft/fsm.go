package raft

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/hashicorp/raft"
)

type Fsm struct {
	kv       KVStore
	listener func(op, filter, nodeID string)
}

func NewFsm() *Fsm {
	fsm := &Fsm{
		kv: NewKVStore(),
	}
	return fsm
}

func (f *Fsm) Apply(l *raft.Log) interface{} {
	fmt.Println("apply data:", string(l.Data))
	data := strings.Split(string(l.Data), ":")
	op := data[0]
	key := data[1]
	value := data[2]
	if op == "set" {
		f.kv.Set(key, value)
	} else if op == "del" {
		f.kv.Del(key, value)
	} else {
		return nil
	}
	if f.listener != nil {
		f.listener(op, key, value)
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
	err := gob.NewDecoder(ir).Decode(&f.kv.Data)
	return err
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
	//d.mu.RLock()
	vs := d.Data[key]
	//d.mu.RUnlock()
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
	d.mu.Unlock()
}

func (d *KVStore) DelByValue(value string) int {
	d.mu.Lock()
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
	d.mu.Unlock()
	return c
}

func (d *KVStore) Persist(sink raft.SnapshotSink) error {
	d.mu.Lock()
	var buffer bytes.Buffer
	err := gob.NewEncoder(&buffer).Encode(d.Data)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	sink.Write(buffer.Bytes())
	sink.Close()
	return nil
}

func (d *KVStore) Release() {}
