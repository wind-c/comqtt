package clients

import (
	"container/list"
	"sync"
)

// InflightMap is a map of InflightMessage keyed on packet id.
type InflightMap struct {
	sync.RWMutex
	cp       int
	ks       *list.List
	internal map[uint16]*InflightMessage // internal contains the inflight messages.
}

func NewMap(cp int) *InflightMap {
	return &InflightMap{
		cp:       cp,
		ks:       list.New(),
		internal: make(map[uint16]*InflightMessage),
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
		delete(i.internal, e.Value.(uint16))
	}
	i.ks.PushBack(in)
	_, ok := i.internal[key]
	i.internal[key] = in
	return !ok
}

// Get returns the value of an in-flight message if it exists.
func (i *InflightMap) Get(key uint16) (*InflightMessage, bool) {
	i.RLock()
	defer i.RUnlock()
	val, ok := i.internal[key]
	return val, ok
}

// Len returns the size of the in-flight messages map.
func (i *InflightMap) Len() int {
	i.RLock()
	defer i.RUnlock()
	v := len(i.internal)
	return v
}

// GetAll returns all the in-flight messages.
func (i *InflightMap) GetAll() map[uint16]*InflightMessage {
	i.RLock()
	defer i.RUnlock()
	return i.internal
}

// Delete removes an in-flight message from the map. Returns true if the
// message existed.
func (i *InflightMap) Delete(key uint16) bool {
	i.Lock()
	defer i.Unlock()
	_, ok := i.internal[key]
	i.ks.Remove(&list.Element{Value: key})
	delete(i.internal, key)
	return ok
}

// Walk
func (i *InflightMap) Walk(cl *Client, handler func(cl *Client, im *InflightMessage, force bool) error) {
	for _, m := range i.internal {
		if err := handler(cl, m, true); err != nil {
			return
		}
	}
}
