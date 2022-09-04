//Modified from https://github.com/orca-zhang/ecache, thanks for the orca-zhang

package cache

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

var p, n = uint16(0), uint16(1)

func hashBKRD(s string) (hash int32) {
	for i := 0; i < len(s); i++ {
		hash = hash*131 + int32(s[i])
	}
	return hash
}
func maskOfNextPowOf2(cap uint16) uint16 {
	if cap > 0 && cap&(cap-1) == 0 {
		return cap - 1
	}
	cap |= (cap >> 1)
	cap |= (cap >> 2)
	cap |= (cap >> 4)
	return cap | (cap >> 8)
}

type value struct {
	i any    // interface{}
	b []byte // bytes
}

type node struct {
	k        string
	v        value
	expireAt int64 // nano timestamp, expireAt=0 if marked as deleted, `createdAt`=`expireAt`-`expiration`
}

type cache struct {
	dlnk [][2]uint16       // double link list, 0 for prev, 1 for next, the first node stands for [tail, head]
	m    []node            // memory pre-allocated
	hmap map[string]uint16 // key -> idx in []node
	last uint16            // last element index when not full
}

func create(cap uint16) *cache {
	return &cache{make([][2]uint16, uint32(cap)+1), make([]node, cap), make(map[string]uint16, cap), 0}
}

// put a cache item into lru cache, if added return 1, updated return 0
func (c *cache) put(k string, i any, b []byte, expireAt int64, on inspector) int {
	if x, ok := c.hmap[k]; ok {
		c.m[x-1].v.i, c.m[x-1].v.b, c.m[x-1].expireAt = i, b, expireAt
		c.ajust(x, p, n) // refresh to head
		return 0
	}

	if c.last == uint16(cap(c.m)) {
		tail := &c.m[c.dlnk[0][p]-1]
		if (*tail).expireAt > 0 { // do not notify for mark delete ones
			on(EVICT, (*tail).k, (*tail).v.i, (*tail).v.b, int((*tail).expireAt-time.Now().UnixNano()))
		} else {
			on(EVICT, (*tail).k, (*tail).v.i, (*tail).v.b, 0)
		}
		delete(c.hmap, (*tail).k)
		c.hmap[k], (*tail).k, (*tail).v.i, (*tail).v.b, (*tail).expireAt = c.dlnk[0][p], k, i, b, expireAt // reuse to reduce gc
		c.ajust(c.dlnk[0][p], p, n)                                                                        // refresh to head
		return 1
	}

	c.last++
	if len(c.hmap) <= 0 {
		c.dlnk[0][p] = c.last
	} else {
		c.dlnk[c.dlnk[0][n]][p] = c.last
	}
	c.m[c.last-1].k, c.m[c.last-1].v.i, c.m[c.last-1].v.b, c.m[c.last-1].expireAt, c.dlnk[c.last], c.hmap[k], c.dlnk[0][n] = k, i, b, expireAt, [2]uint16{0, c.dlnk[0][n]}, c.last, c.last
	return 1
}

// get value of key from lru cache with result
func (c *cache) get(k string) (*node, int) {
	if x, ok := c.hmap[k]; ok {
		c.ajust(x, p, n) // refresh to head
		return &c.m[x-1], 1
	}
	return nil, 0
}

// delete item by key from lru cache
func (c *cache) del(k string) (_ *node, _ int, e int64) {
	if x, ok := c.hmap[k]; ok && c.m[x-1].expireAt > 0 {
		c.m[x-1].expireAt, e = 0, c.m[x-1].expireAt // mark as deleted
		c.ajust(x, n, p)                            // sink to tail
		return &c.m[x-1], 1, e
	}
	return nil, 0, 0
}

// calls f sequentially for each valid item in the lru cache
func (c *cache) walk(walker func(key string, ia any, bytes []byte, expireAt int64) bool) {
	for idx := c.dlnk[0][n]; idx != 0; idx = c.dlnk[idx][n] {
		fmt.Println(c.m[idx-1].k)
		if c.m[idx-1].expireAt == 0 || c.m[idx-1].expireAt-time.Now().UnixNano() > 0 {
			if !walker(c.m[idx-1].k, c.m[idx-1].v.i, c.m[idx-1].v.b, c.m[idx-1].expireAt) {
				return
			}
		}
	}
}

// when f=0, t=1, move to head, otherwise to tail
func (c *cache) ajust(idx, f, t uint16) {
	if c.dlnk[idx][f] != 0 { // f=0, t=1, not head node, otherwise not tail
		c.dlnk[c.dlnk[idx][t]][f], c.dlnk[c.dlnk[idx][f]][t], c.dlnk[idx][f], c.dlnk[idx][t], c.dlnk[c.dlnk[0][t]][f], c.dlnk[0][t] = c.dlnk[idx][f], c.dlnk[idx][t], 0, c.dlnk[0][t], idx, idx
	}
}

// Cache - concurrent cache structure
type Cache struct {
	locks      []sync.Mutex
	insts      [][2]*cache // level-0 for normal LRU, level-1 for LRU-2
	expiration time.Duration
	on         inspector
	mask       int32
}

// NewLRUCache - create lru cache
// `bucketCnt` is buckets that shard items to reduce lock racing
// `capPerBkt` is length of each bucket, can store `capPerBkt * bucketCnt` count of items in Cache at most
// optional `expiration` is item alive time (and we only use lazy eviction here), default `0` stands for permanent
func NewLRUCache(bucketCnt, capPerBkt uint16, expiration ...time.Duration) *Cache {
	mask := maskOfNextPowOf2(bucketCnt)
	c := &Cache{make([]sync.Mutex, mask+1), make([][2]*cache, mask+1), 0, func(int, string, any, []byte, int) {}, int32(mask)}
	for i := range c.insts {
		c.insts[i][0] = create(capPerBkt)
	}
	if len(expiration) > 0 {
		c.expiration = expiration[0]
	}
	return c
}

// LRU2 - add LRU-2 support (especially LRU-2 that when item visited twice it moves to upper-level-cache)
// `capPerBkt` is length of each LRU-2 bucket, can store extra `capPerBkt * bucketCnt` count of items in Cache at most
func (c *Cache) LRU2(capPerBkt uint16) *Cache {
	for i := range c.insts {
		c.insts[i][1] = create(capPerBkt)
	}
	return c
}

// put - put a item into cache
func (c *Cache) put(key string, i any, b []byte) {
	idx := hashBKRD(key) & c.mask
	c.locks[idx].Lock()
	status := c.insts[idx][0].put(key, i, b, time.Now().UnixNano()+int64(c.expiration), c.on)
	c.locks[idx].Unlock()
	c.on(PUT, key, i, b, status)
}

// putEx - put a item into cache
func (c *Cache) putEx(key string, i any, b []byte, ex time.Duration) {
	idx := hashBKRD(key) & c.mask
	c.locks[idx].Lock()
	status := c.insts[idx][0].put(key, i, b, time.Now().UnixNano()+int64(ex), c.on)
	c.locks[idx].Unlock()
	c.on(PUT, key, i, b, status)
}

// putExAt - put a item into cache
func (c *Cache) putExAt(key string, i any, b []byte, ea int64) {
	idx := hashBKRD(key) & c.mask
	c.locks[idx].Lock()
	status := c.insts[idx][0].put(key, i, b, ea, c.on)
	c.locks[idx].Unlock()
	c.on(PUT, key, i, b, status)
}

// ToInt64 - convert bytes to int64
func ToInt64(b []byte) (int64, bool) {
	if len(b) >= 8 {
		return int64(binary.LittleEndian.Uint64(b)), true
	}
	return 0, false
}

// Put - put an item into cache
func (c *Cache) Put(key string, val any) { c.put(key, val, nil) }

// PutEx - put an item into cache
func (c *Cache) PutEx(key string, val any, ex time.Duration) {
	c.putEx(key, val, nil, ex)
}

// PutExAt - put an item into cache
func (c *Cache) PutExAt(key string, val any, ea int64) {
	c.putExAt(key, val, nil, ea)
}

// PutInt64 - put a digit item into cache
func (c *Cache) PutInt64(key string, d int64) {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], uint64(d))
	c.put(key, nil, data[:])
}

// PutBytes - put a bytes item into cache
func (c *Cache) PutBytes(key string, b []byte) { c.put(key, nil, b) }

// Get - get value of key from cache with result
func (c *Cache) Get(key string) (any, bool) {
	if i, _, ok := c.get(key); ok && i != nil {
		return i, true
	}
	return nil, false
}

// GetBytes - get bytes value of key from cache with result
func (c *Cache) GetBytes(key string) ([]byte, bool) {
	if _, b, ok := c.get(key); ok {
		return b, true
	}
	return nil, false
}

// GetInt64 - get value of key from cache with result
func (c *Cache) GetInt64(key string) (int64, bool) {
	if _, b, ok := c.get(key); ok && len(b) >= 8 {
		return int64(binary.LittleEndian.Uint64(b)), true
	}
	return 0, false
}

func (c *Cache) _get(key string, idx, level int32) (*node, int) {
	if n, s := c.insts[idx][level].get(key); s > 0 && n.expireAt > 0 && (c.expiration <= 0 || time.Now().UnixNano() < n.expireAt) {
		return n, s // no necessary to remove the expired item here, otherwise will cause GC thrashing
	}
	return nil, 0
}

func (c *Cache) get(key string) (i any, b []byte, _ bool) {
	idx := hashBKRD(key) & c.mask
	c.locks[idx].Lock()
	n, s := (*node)(nil), 0
	if c.insts[idx][1] == nil { // (if LRU-2 mode not support, loss is little)
		n, s = c._get(key, idx, 0) // normal lru mode
	} else { // LRU-2 mode
		e := int64(0)
		if n, s, e = c.insts[idx][0].del(key); s <= 0 {
			n, s = c._get(key, idx, 1) // re-find in level-1
		} else {
			c.insts[idx][1].put(key, n.v.i, n.v.b, e, c.on) // find in level-0, move to level-1
		}
	}
	if s <= 0 {
		c.locks[idx].Unlock()
		c.on(GET, key, nil, nil, 0)
		return
	}
	i, b = n.v.i, n.v.b
	c.locks[idx].Unlock()
	c.on(GET, key, i, b, 1)
	return i, b, true
}

// Del - delete item by key from cache
func (c *Cache) Del(key string) {
	idx := hashBKRD(key) & c.mask
	c.locks[idx].Lock()
	n, s, e := c.insts[idx][0].del(key)
	if c.insts[idx][1] != nil { // (if LRU-2 mode not support, loss is little)
		if n2, s2, e2 := c.insts[idx][1].del(key); n2 != nil && (n == nil || e < e2) { // callback latest added one if both exists
			n, s = n2, s2
		}
	}
	if s > 0 {
		c.on(DEL, key, n.v.i, n.v.b, 1)
		n.v.i, n.v.b = nil, nil // release now
	} else {
		c.on(DEL, key, nil, nil, 0)
	}
	c.locks[idx].Unlock()
}

// Walk - calls f sequentially for each valid item in the lru cache, return false to stop iteration for every bucket
func (c *Cache) Walk(walker func(key string, ifc any, bytes []byte, expireAt int64) bool) {
	for i := range c.insts {
		c.locks[i].Lock()
		if c.insts[i][0].walk(walker); c.insts[i][1] != nil {
			c.insts[i][1].walk(walker)
		}
		c.locks[i].Unlock()
	}
}

// Len
func (c *Cache) Len() int {
	var count int
	for i := range c.insts {
		c.locks[i].Lock()
		if c.insts[i][0] != nil {
			count += len(c.insts[i][0].hmap)
		}
		if c.insts[i][1] != nil {
			count += len(c.insts[i][1].hmap)
		}
		c.locks[i].Unlock()
	}
	return count
}

const (
	PUT = iota + 1
	GET
	DEL
	EVICT
)

// inspector - can be used to statistics cache hit/miss rate or other scenario like ringbuf queue
//   more details about every parameter: https://github.com/orca-zhang/ecache/blob/master/README_en.md#inject-an-inspector
type inspector func(action int, key string, i any, bytes []byte, status int)

// Inspect - to inspect the actions
func (c *Cache) Inspect(ipr inspector) {
	old := c.on
	c.on = func(action int, key string, i any, bytes []byte, status int) {
		old(action, key, i, bytes, status) // call as the declared order, old first
		ipr(action, key, i, bytes, status)
	}
}
