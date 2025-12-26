package discovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	rdb "github.com/redis/go-redis/v9"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/storage/redis"
	"github.com/wind-c/comqtt/v2/cluster/utils"
	"github.com/wind-c/comqtt/v2/config"
)

const (
	logTag                      string = "dynamic registry"
	defaultNodeNamePrefix       string = "co-"
	defaultEventLoopIntervalSec int64  = 10
	defaultNodesRegistryKey     string = "nodes"
	defaultNodeRegistryExp      int64  = 30
	defaultLockKey              string = "node-registry-mutex"
	defaultLockLoopIntervalSec  uint   = 5
	defaultMaxLockAttempts      int    = 30
)

const (
	AddressWayPrivateIP uint = iota
	AddressWayPublicIP
	AddressWayHostname
)

const (
	NodeNameWayPrivateIP uint = iota
	NodeNameWayPublicIP
	NodeNameWayHostname
	NodeNameWayUUID
)

var (
	ErrSerfRequired        = errors.New("discovery-way must be 0 for dynamic membership (serf)")
	ErrKeyExpMustBeGreater = errors.New("redis-node-key-exp must be greater than event-loop-interval-sec")
	ErrRedisNotAvail       = errors.New("redis not available")
	ErrInvalidAddress      = errors.New("invalid or missing address")
	ErrInvalidNodeName     = errors.New("invalid or missing node name")
)

type DynamicRegistry struct {
	NodeKey   string
	NodesFile string
	cfg       *config.Cluster
	ctx       context.Context
	term      chan bool
	rsync     *redsync.Redsync
}

func NewDynamicRegistry() *DynamicRegistry {
	return &DynamicRegistry{
		term: make(chan bool, 1),
		ctx:  context.Background(),
	}
}

func (r *DynamicRegistry) Init(cfg *config.Cluster, nodesFile string) (err error) {

	// dynamic membership disabled
	if !cfg.DynamicMembership.Enable {
		return
	}

	r.NodesFile = nodesFile

	// setup defaults

	if len(cfg.DynamicMembership.NodeNamePrefix) == 0 {
		cfg.DynamicMembership.NodeNamePrefix = defaultNodeNamePrefix
	}
	if len(cfg.DynamicMembership.NodesRegistryKey) == 0 {
		cfg.DynamicMembership.NodesRegistryKey = defaultNodesRegistryKey
	}
	if len(cfg.DynamicMembership.LockKey) == 0 {
		cfg.DynamicMembership.LockKey = defaultLockKey
	}

	// these should never be zero
	if cfg.DynamicMembership.EventLoopIntervalSec == 0 {
		cfg.DynamicMembership.EventLoopIntervalSec = defaultEventLoopIntervalSec
	}
	if cfg.DynamicMembership.NodeRegistryExp == 0 {
		cfg.DynamicMembership.NodeRegistryExp = defaultNodeRegistryExp
	}
	if cfg.DynamicMembership.LockLoopIntervalSec == 0 {
		cfg.DynamicMembership.LockLoopIntervalSec = defaultLockLoopIntervalSec
	}
	if cfg.DynamicMembership.MaxLockAttempts == 0 {
		cfg.DynamicMembership.MaxLockAttempts = defaultMaxLockAttempts
	}

	log.Info(logTag, "startup", fmt.Sprintf("using node name prefix: %s", cfg.DynamicMembership.NodeNamePrefix))
	log.Info(logTag, "startup", fmt.Sprintf("using nodes registry table key: %s", cfg.DynamicMembership.NodesRegistryKey))
	log.Info(logTag, "startup", fmt.Sprintf("using node exp: %d secs", cfg.DynamicMembership.NodeRegistryExp))
	log.Info(logTag, "startup", fmt.Sprintf("using event loop interval: %d secs", cfg.DynamicMembership.EventLoopIntervalSec))
	log.Info(logTag, "startup", fmt.Sprintf("using lock key: %s", cfg.DynamicMembership.LockKey))
	log.Info(logTag, "startup", fmt.Sprintf("using lock frequency: %d secs", cfg.DynamicMembership.LockLoopIntervalSec))
	log.Info(logTag, "startup", fmt.Sprintf("using max lock attempts: %d", cfg.DynamicMembership.MaxLockAttempts))

	// cluster mode requires redis, we shouldn't need to validate storage-way=3

	// requires serf
	if cfg.DiscoveryWay != 0 {
		err = ErrSerfRequired
		return
	}

	// a node's redis key MUST live longer than the event loop interval
	if cfg.DynamicMembership.EventLoopIntervalSec > cfg.DynamicMembership.NodeRegistryExp {
		err = ErrKeyExpMustBeGreater
		return
	}

	if redis.Client() == nil {
		err = ErrRedisNotAvail
		return
	}

	pool := goredis.NewPool(redis.Client())
	r.rsync = redsync.New(pool)
	r.cfg = cfg

	err = r.Claim() // hold here until we claim/generate our name
	return
}

func (r *DynamicRegistry) Claim() (err error) {

	// determine who is first to boot for RaftBootstrap
	// use a distributed lock to wait until it's our turn to claim a name
	log.Info(logTag, "claim", "waiting to acquire claim lock")
	lock, err := r.Lock()
	if err != nil {
		return err
	}

	log.Info(logTag, "claim", "claim lock acquired")

	address, err := r.GenerateNodeAddress()
	if err != nil {
		return
	}

	var nodename string = ""

	defer func() {
		if fErr := r.FinalizeClaim(address, nodename, lock); fErr != nil {
			err = fErr
		}
	}()

	// if we already have a node cache file saved locally, resume
	if utils.PathExists(r.NodesFile) {
		ms := ReadMembers(r.NodesFile)
		for _, m := range ms {
			if m.Addr == address {
				nodename = m.Name
				log.Info(logTag, "claim", "found nodes.json file, resuming previous node name...")
				return
			}
		}
	}

	nodename, err = r.GenerateNodeName()
	return
}

func (r *DynamicRegistry) GenerateNodeAddress() (address string, err error) {
	switch r.cfg.DynamicMembership.AddressWay {
	case AddressWayPrivateIP:
		address, err = utils.GetPrivateIP()
	case AddressWayPublicIP:
		address, err = utils.GetPublicIP()
		return
	case AddressWayHostname:
		address, err = os.Hostname()
		return
	default:
		err = errors.New("invalid address-way")
	}
	if len(address) == 0 {
		err = ErrInvalidAddress
	}
	return
}

func (r *DynamicRegistry) GenerateNodeName() (name string, err error) {

	defer func() {
		name = fmt.Sprintf("%s%s", r.cfg.DynamicMembership.NodeNamePrefix, name)
	}()

	switch r.cfg.DynamicMembership.NodeNameWay {
	case NodeNameWayPrivateIP:
		name, err = utils.GetPrivateIP()
		return
	case NodeNameWayPublicIP:
		name, err = utils.GetPublicIP()
		return
	case NodeNameWayHostname:
		name, err = os.Hostname()
		return
	case NodeNameWayUUID:
		name = utils.GenerateUUID4()
		return
	default:
		err = errors.New("invalid node-name-way")
	}
	if len(name) == 0 {
		err = ErrInvalidNodeName
	}
	return
}

func (r *DynamicRegistry) FinalizeClaim(address string, nodename string, lock *redsync.Mutex) (err error) {

	if len(address) == 0 || len(nodename) == 0 {
		err = errors.New("invalid node name")
		return
	}

	log.Info(logTag, "claim", fmt.Sprintf("node %s claimed for %s", nodename, address))

	r.NodeKey = fmt.Sprintf("%s:%s", address, nodename)

	// get registry from redis
	registry, err := r.GetRegistry()
	if err != nil {
		return
	}

	// first node gets the bootstrap flag
	if len(registry) == 0 {
		r.cfg.RaftBootstrap = true
		log.Info(logTag, "claim", fmt.Sprintf("%s assuming raft leader", address))
	}

	// in case set via config file,
	// overwrite with new dynamic values
	r.cfg.BindAddr = address
	r.cfg.NodeName = nodename

	// register the node as soon as we have details
	r.SaveNode()

	// re-initialize members list with members from the registry
	r.cfg.Members = []string{}
	for _, member := range registry {
		r.cfg.Members = append(r.cfg.Members, fmt.Sprintf("%s:%d", member.Addr, r.cfg.BindPort))
	}
	// add ourselves if we're new here
	if !slices.Contains(r.cfg.Members, fmt.Sprintf("%s:%d", address, r.cfg.BindPort)) {
		r.cfg.Members = append(r.cfg.Members, fmt.Sprintf("%s:%d", address, r.cfg.BindPort))
	}

	// keep node updated and run garbage collections
	go r.StartEventLoop()

	// release the lock to other nodes
	err = r.Unlock(lock)
	if err != nil {
		return err
	}

	log.Info(logTag, "claim", "claim lock released")
	return
}

func (r *DynamicRegistry) GetRegistry() (inventory []*Member, err error) {

	if redis.Client() == nil {
		err = ErrRedisNotAvail
		return
	}

	var keys map[string]string
	keys, err = redis.Client().HGetAll(r.ctx, r.cfg.DynamicMembership.NodesRegistryKey).Result()
	if err != nil && err != rdb.Nil {
		return
	}

	// in case we aren't running redis 7.4+, manually expire nodes that have gone away
	keep := make([]string, 0)
	trash := make([]string, 0)
	for node, ts_str := range keys {
		var ts int64
		ts, err = strconv.ParseInt(ts_str, 10, 64)
		if err != nil {
			return
		}
		// expired
		if ts < (time.Now().Unix() - r.cfg.DynamicMembership.NodeRegistryExp) {
			trash = append(trash, node)
		} else {
			keep = append(keep, node)
		}
	}
	if len(trash) > 0 {
		err = redis.Client().HDel(r.ctx, r.cfg.DynamicMembership.NodesRegistryKey, trash...).Err()
		if err != nil {
			return
		}
	}

	members := make([]*Member, 0)
	for _, key := range keep {
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			err = fmt.Errorf("invalid key: %s", key)
			return
		}
		members = append(members, &Member{
			Addr: parts[0],
			Name: parts[1],
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

		// set the registry entry
		err = redis.Client().HSet(
			r.ctx,
			r.cfg.DynamicMembership.NodesRegistryKey,
			r.NodeKey,
			fmt.Sprintf("%d", time.Now().Unix()),
		).Err()
		if err != nil {
			return
		}

		// if supported, place an EXP directly on the specific node in the table
		hexp_err := redis.Client().HExpire(
			r.ctx,
			r.cfg.DynamicMembership.NodesRegistryKey,
			time.Second*time.Duration(r.cfg.DynamicMembership.NodeRegistryExp),
			r.NodeKey,
		).Err()
		if hexp_err != nil {
			log.Debug(logTag, "redis", "Could not use HEXPIRE directly. Ensure Redis 7.4+ is used and client is updated.", hexp_err)
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
		err = redis.Client().HDel(r.ctx, r.cfg.DynamicMembership.NodesRegistryKey, r.NodeKey).Err()
	}
	return
}

func (r *DynamicRegistry) StartEventLoop() {
	for {
		select {
		// stop
		case <-r.term:
			log.Info(logTag, "registry", "stopping event loop...")
			return
		default:
			// if working properly, we should see join/leave events happening automatically in serf
			// no need to log activity here

			// bump node TTL
			err := r.SaveNode()
			if err != nil {
				log.Error(logTag, "error", "r.SaveNode():", err.Error())
				return
			}
			nodes, err := r.GetRegistry()
			if err != nil {
				log.Error(logTag, "error", "r.GetRegistry():", err.Error())
				return
			}
			// new members with new IPs
			for _, node := range nodes {
				membership := fmt.Sprintf("%s:%d", node.Addr, r.cfg.BindPort)
				if !slices.Contains(r.cfg.Members, membership) {
					log.Info(logTag, "membership", fmt.Sprintf("new membership: %s", membership))
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

	// cfg will be nil if dyn wasn't enabled
	if r.cfg == nil || !r.cfg.DynamicMembership.Enable {
		return
	}

	log.Info(logTag, "registry", "stopping registry...")

	// stop the event loop
	close(r.term)

	// manually pull the node from redis
	err = r.RemoveNode()
	return
}

func (r *DynamicRegistry) Lock() (mutex *redsync.Mutex, err error) {
	if r.rsync == nil {
		err = errors.New("redis sync is not available")
		return
	}
	mutex = r.rsync.NewMutex(
		r.cfg.DynamicMembership.LockKey,
		redsync.WithRetryDelay(time.Duration(r.cfg.DynamicMembership.LockLoopIntervalSec)),
		redsync.WithTries(r.cfg.DynamicMembership.MaxLockAttempts),
	)
	err = mutex.Lock()
	return
}

func (r *DynamicRegistry) Unlock(mutex *redsync.Mutex) (err error) {
	_, err = mutex.Unlock()
	return
}
