package clients

import (
	"github.com/wind-c/comqtt/server/internal/cache"
	"strconv"
)

// InflightCache is a cache of InflightMessage keyed on packet id.
type InflightCache struct {
	internal *cache.Cache // internal contains the inflight messages.
}

func NewCache(cap uint16) *InflightCache {
	return &InflightCache{
		internal: cache.NewLRUCache(1, cap, 0),
	}
}

// Set stores the packet of an Inflight message, keyed on message id. Returns
// true if the inflight message was new.
func (i *InflightCache) Set(key uint16, in *InflightMessage) bool {
	i.internal.PutExAt(strconv.Itoa(int(key)), in, in.Expiry)
	return true
}

// Get returns the value of an in-flight message if it exists.
func (i *InflightCache) Get(key uint16) (*InflightMessage, bool) {
	if v, ok := i.internal.Get(strconv.Itoa(int(key))); ok {
		return v.(*InflightMessage), ok
	}
	return nil, false
}

// Len returns the size of the in-flight messages map.
func (i *InflightCache) Len() int {
	return i.internal.Len()
}

// GetAll returns all the in-flight messages.
func (i *InflightCache) GetAll() map[uint16]*InflightMessage {
	ims := make(map[uint16]*InflightMessage, i.Len())
	i.internal.Walk(func(key string, ia any, bytes []byte, expireAt int64) bool {
		if k, err := strconv.Atoi(key); err == nil {
			ims[uint16(k)] = ia.(*InflightMessage)
			return true
		} else {
			return false
		}
	})
	return ims
}

// Delete removes an in-flight message from the map. Returns true if the
// message existed.
func (i *InflightCache) Delete(key uint16) bool {
	i.internal.Del(strconv.Itoa(int(key)))
	return true
}

// Walk
func (i *InflightCache) Walk(cl *Client, handler func(cl *Client, im *InflightMessage, force bool) error) {
	i.internal.Walk(func(key string, ia any, bytes []byte, expireAt int64) bool {
		if err := handler(cl, ia.(*InflightMessage), true); err != nil {
			return false
		}
		return true
	})
}
