package clients

import (
	"container/list"
	"sync"
)

type inflightMapValue struct {
	key uint16
	in  *InflightMessage
}

// InflightMap is a map of InflightMessage keyed on packet id.
type InflightMap struct {
	sync.RWMutex
	cp       int
	ks       *list.List
	internal map[uint16]*list.Element // internal maps keys to list elements with corresponding inflight messages
}

func NewMap(cp int) *InflightMap {
	return &InflightMap{
		cp:       cp,
		ks:       list.New(),
		internal: make(map[uint16]*list.Element),
	}
}

// Set stores the packet of an Inflight message, keyed on message id. Returns
// true if the inflight message was new.
func (i *InflightMap) Set(key uint16, in *InflightMessage) bool {
	i.Lock()
	defer i.Unlock()
	if i.cp > 0 && i.cp == i.ks.Len() {
		e := i.ks.Front()
		i.ks.Remove(e)
		delete(i.internal, e.Value.(inflightMapValue).key)
	}
	if _, ok := i.internal[key]; !ok {
		e := i.ks.PushBack(inflightMapValue{key: key, in: in})
		i.internal[key] = e
		return true
	}
	return false
}

// Get returns the value of an in-flight message if it exists.
func (i *InflightMap) Get(key uint16) (*InflightMessage, bool) {
	i.RLock()
	defer i.RUnlock()
	if e, ok := i.internal[key]; ok {
		return e.Value.(inflightMapValue).in, ok
	}
	return nil, false
}

// Len returns the size of the in-flight messages map.
func (i *InflightMap) Len() int {
	i.RLock()
	defer i.RUnlock()
	return len(i.internal)
}

// GetAll returns all the in-flight messages.
func (i *InflightMap) GetAll() map[uint16]*InflightMessage {
	i.RLock()
	defer i.RUnlock()
	ims := make(map[uint16]*InflightMessage, len(i.internal))
	for e := i.ks.Front(); e != nil; e = e.Next() {
		key := e.Value.(inflightMapValue).key
		ims[key] = e.Value.(inflightMapValue).in
	}
	return ims
}

// Delete removes an in-flight message from the map. Returns true if the
// message existed.
func (i *InflightMap) Delete(key uint16) bool {
	i.Lock()
	defer i.Unlock()
	if e, ok := i.internal[key]; ok {
		i.ks.Remove(e)
		delete(i.internal, key)
		return true
	}
	return false
}

// Walk
func (i *InflightMap) Walk(cl *Client, handler func(cl *Client, im *InflightMessage, force bool) error) {
	for e := i.ks.Front(); e != nil; e = e.Next() {
		if err := handler(cl, e.Value.(inflightMapValue).in, true); err != nil {
			return
		}
	}
}
