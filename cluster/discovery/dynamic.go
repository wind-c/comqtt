package discovery

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/storage/redis"
	"github.com/wind-c/comqtt/v2/cluster/utils"
	"github.com/wind-c/comqtt/v2/config"
)

const (
	defaultNodeNamePrefix           string = "co-"
	defaultEventLoopIntervalSec     uint   = 10
	defaultRedisNodeKeyPrefix       string = "nodes"
	defaultRedisNodeKeyExp          uint   = 30
	defaultRedisLockKey             string = "node-registry-mutex"
	defaultRedisLockLoopIntervalSec uint   = 5
	defaultMaxRedisLockAttempts     uint   = 30
)

var (
	ErrSerfRequired        = errors.New("discovery-way must be 0 for dynamic membership (serf)")
	ErrKeyExpMustBeGreater = errors.New("redis-node-key-exp must be greater than event-loop-interval-sec")
	ErrRedisNotAvail       = errors.New("redis not available")
	ErrInvalidOutboundIP   = errors.New("invalid or missing outbound IP")
)

type DynamicRegistry struct {
	NodeKey string
	cfg     *config.Cluster
	ctx     context.Context
	term    chan bool
}

func NewDynamicRegistry() *DynamicRegistry {
	return &DynamicRegistry{
		term: make(chan bool, 1),
	}
}

func (r *DynamicRegistry) Init(cfg *config.Cluster, ctx context.Context) (err error) {

	// setup defaults

	if len(cfg.DynamicMembership.NodeNamePrefix) == 0 {
		cfg.DynamicMembership.NodeNamePrefix = defaultNodeNamePrefix
	}
	if len(cfg.DynamicMembership.RedisNodeKeyPrefix) == 0 {
		cfg.DynamicMembership.RedisNodeKeyPrefix = defaultRedisNodeKeyPrefix
	}
	if len(cfg.DynamicMembership.RedisLockKey) == 0 {
		cfg.DynamicMembership.RedisLockKey = defaultRedisLockKey
	}

	// these should never be zero
	if cfg.DynamicMembership.EventLoopIntervalSec == 0 {
		cfg.DynamicMembership.EventLoopIntervalSec = defaultEventLoopIntervalSec
	}
	if cfg.DynamicMembership.RedisNodeKeyExp == 0 {
		cfg.DynamicMembership.RedisNodeKeyExp = defaultRedisNodeKeyExp
	}
	if cfg.DynamicMembership.RedisLockLoopIntervalSec == 0 {
		cfg.DynamicMembership.RedisLockLoopIntervalSec = defaultRedisLockLoopIntervalSec
	}
	if cfg.DynamicMembership.MaxRedisLockAttempts == 0 {
		cfg.DynamicMembership.MaxRedisLockAttempts = defaultMaxRedisLockAttempts
	}

	log.Info(fmt.Sprintf("dynamic: using node name prefix: %s", cfg.DynamicMembership.NodeNamePrefix))
	log.Info(fmt.Sprintf("dynamic: using redis node key prefix: %s", cfg.DynamicMembership.RedisNodeKeyPrefix))
	log.Info(fmt.Sprintf("dynamic: using redis lock key: %s", cfg.DynamicMembership.RedisLockKey))
	log.Info(fmt.Sprintf("dynamic: using event loop interval: %d secs", cfg.DynamicMembership.EventLoopIntervalSec))
	log.Info(fmt.Sprintf("dynamic: using node key exp: %d secs", cfg.DynamicMembership.RedisNodeKeyExp))
	log.Info(fmt.Sprintf("dynamic: using lock loop interval: %d secs", cfg.DynamicMembership.RedisLockLoopIntervalSec))
	log.Info(fmt.Sprintf("dynamic: using max lock attempts: %d", cfg.DynamicMembership.MaxRedisLockAttempts))

	// cluster mode requires redis, we shouldn't need to validate storage-way=3

	// requires serf
	if cfg.DiscoveryWay != 0 {
		err = ErrSerfRequired
		return
	}

	// a node's redis key MUST live longer than the event loop interval
	if cfg.DynamicMembership.EventLoopIntervalSec > cfg.DynamicMembership.RedisNodeKeyExp {
		err = ErrKeyExpMustBeGreater
		return
	}

	if redis.Client() == nil || redis.Sync() == nil {
		err = ErrRedisNotAvail
		return
	}

	r.cfg = cfg
	r.ctx = ctx

	err = r.Claim() // holds here until we make our claim
	return
}

func (r *DynamicRegistry) Claim() (err error) {

	ip, err := utils.GetOutBoundIP()
	if err != nil {
		return err
	} else if len(ip) == 0 {
		err = ErrInvalidOutboundIP
		return err
	}

	// only one node can claim a node number at a time
	// use a distributed lock to wait until its our turn to claim a number, within reason
	log.Info("dynamic: waiting to acquire claim lock")
	lock, err := redis.Lock(
		r.cfg.DynamicMembership.RedisLockKey,
		r.cfg.DynamicMembership.MaxRedisLockAttempts,
		r.cfg.DynamicMembership.RedisLockLoopIntervalSec,
		true,
	)
	if err != nil {
		return err
	}

	log.Info("dynamic: claim lock acquired")

	var nodenum int = 0
	var members []*Member

	defer func() {
		if fErr := r.FinalizeClaim(ip, nodenum, members, lock); fErr != nil {
			err = fErr
		}
	}()

	// get the list of current nodes
	members, err = r.GetInventory()
	if err != nil {
		return err
	}

	// if no nodes yet, we're the first one
	if len(members) == 0 {
		nodenum = 1
		return
	}

	nodenums := []int{}
	for _, member := range members {
		var num int
		// our IP already exists in the inventory, we can resume
		if member.Addr == ip {
			num, err = strconv.Atoi(member.Name)
			if err != nil {
				return err
			}
			nodenum = num
			return err
		}
		num, err = strconv.Atoi(member.Name)
		if err != nil {
			return err
		}
		nodenums = append(nodenums, num)
	}

	// if for some reason we don't any valid node nums, just claim "1"
	if len(nodenums) == 0 {
		nodenum = 1
		return
	}

	// sort nums into sequence
	sort.Ints(nodenums[:])

	// if there are missing numbers in the sequence, claim the first available
	present := make(map[int]bool)
	for _, num := range nodenums {
		present[num] = true
	}
	var gaps []int
	for i := 1; i <= nodenums[len(nodenums)-1]; i++ {
		if !present[i] {
			gaps = append(gaps, i)
		}
	}

	if len(gaps) > 0 {
		// ensure our gaps are in sequential order
		sort.Ints(gaps[:])
		// claim the first gap
		nodenum = gaps[0]
		return
	}

	// if we reach this point, its should be safe just to claim the next num in the sequence
	nodenum = len(nodenums) + 1
	return
}

func (r *DynamicRegistry) FinalizeClaim(ip string, nodenum int, members []*Member, lock *redsync.Mutex) (err error) {

	if len(ip) == 0 || nodenum <= 0 {
		err = errors.New("unable to generate node number")
		return err
	}

	log.Info(fmt.Sprintf("dynamic: node %d claimed for %s", nodenum, ip))

	r.NodeKey = fmt.Sprintf("%d:%s", nodenum, ip)

	// first node get the bootstrap flag
	if nodenum == 1 {
		r.cfg.RaftBootstrap = true
		log.Info(fmt.Sprintf("dynamic: %s assuming raft leader", ip))
	}

	// in case set via config file,
	// overwrite with new dynamic values
	r.cfg.BindAddr = ip
	r.cfg.NodeName = fmt.Sprintf("%s%0*d", r.cfg.DynamicMembership.NodeNamePrefix, r.cfg.DynamicMembership.NodeNumZeroPad, nodenum)

	// register the node before we release the lock to other nodes
	r.SaveNode()

	// re-initialize members list with members from inventory
	r.cfg.Members = []string{}
	for _, member := range members {
		r.cfg.Members = append(r.cfg.Members, fmt.Sprintf("%s:%d", member.Addr, r.cfg.BindPort))
	}
	// add ourselves if we're new here
	if !slices.Contains(r.cfg.Members, fmt.Sprintf("%s:%d", ip, r.cfg.BindPort)) {
		r.cfg.Members = append(r.cfg.Members, fmt.Sprintf("%s:%d", ip, r.cfg.BindPort))
	}

	// keep node up to date
	go r.StartEventLoop()

	// release the lock
	err = redis.Unlock(lock)
	if err != nil {
		return err
	}

	log.Info("dynamic: claim lock released")

	return
}

func (r *DynamicRegistry) GetInventory() (inventory []*Member, err error) {

	if redis.Client() == nil {
		err = ErrRedisNotAvail
		return
	}

	var keys []string
	// The pattern should end with a wildcard '*' to match the prefix
	pattern := r.cfg.DynamicMembership.RedisNodeKeyPrefix + "*"
	// Use Scan to iterate over keys
	// 0 is the initial cursor, pattern is the match pattern, 0 is the count (default)
	iter := redis.Client().Scan(r.ctx, 0, pattern, 0).Iterator()
	for iter.Next(r.ctx) {
		keys = append(keys, iter.Val())
	}
	if err = iter.Err(); err != nil {
		return
	}

	members := make([]*Member, 0)
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid key: %s", key)
		}
		members = append(members, &Member{
			Name: parts[1], // ensure we are only storing the number in redis, without the name prefix
			Addr: parts[2],
		})
	}

	return members, nil
}

func (r *DynamicRegistry) SaveNode() (err error) {
	if redis.Client() == nil {
		err = ErrRedisNotAvail
		return
	}
	if len(r.NodeKey) > 0 {
		// ensure r.NodeKey doesn't have the name prefix
		// needs to be num:ip format
		key := fmt.Sprintf("%s:%s", r.cfg.DynamicMembership.RedisNodeKeyPrefix, r.NodeKey)
		err = redis.Client().Set(r.ctx, key, r.NodeKey, time.Second*time.Duration(r.cfg.DynamicMembership.RedisNodeKeyExp)).Err()
		if err != nil {
			return err
		}
	}
	return
}

func (r *DynamicRegistry) RemoveNode() (err error) {
	if redis.Client() == nil {
		err = ErrRedisNotAvail
		return
	}
	if len(r.NodeKey) > 0 {
		key := fmt.Sprintf("%s:%s", r.cfg.DynamicMembership.RedisNodeKeyPrefix, r.NodeKey)
		_, err = redis.Client().Del(r.ctx, key).Result()
	}
	return
}

func (r *DynamicRegistry) StartEventLoop() {
	for {
		select {
		// stop
		case <-r.term:
			log.Info("dynamic: stopping event loop...")
			return
		default:
			// if working properly, we should see join/leave events happening automatically in serf
			// no need to log activity here

			// bump the redis exp
			err := r.SaveNode()
			if err != nil {
				log.Error("dynamic: r.SaveNode():", err.Error())
				return
			}
			nodes, err := r.GetInventory()
			if err != nil {
				log.Error("dynamic: r.GetInventory():", err.Error())
				return
			}
			// new members with new IPs
			for _, node := range nodes {
				membership := fmt.Sprintf("%s:%d", node.Addr, r.cfg.BindPort)
				if !slices.Contains(r.cfg.Members, membership) {
					log.Info(fmt.Sprintf("dynamic: new membership: %s", membership))
					r.cfg.Members = append(r.cfg.Members, membership)
				}
			}
			// remove the ones that have gone away
			keep := []string{}
			for _, member := range r.cfg.Members {
				for _, node := range nodes {
					membership := fmt.Sprintf("%s:%d", node.Addr, r.cfg.BindPort)
					if membership == member {
						keep = append(keep, member)
					}
				}
			}
			r.cfg.Members = keep
			time.Sleep(time.Second * time.Duration(r.cfg.DynamicMembership.EventLoopIntervalSec))
		}
	}
}

func (r *DynamicRegistry) Stop() (err error) {

	if !r.cfg.DynamicMembership.Enable {
		return
	}

	log.Info("dynamic: stopping registry...")

	// stop the event loop
	close(r.term)

	// manually pull the node from redis
	err = r.RemoveNode()
	return
}
