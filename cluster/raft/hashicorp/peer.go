// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package hashicorp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/cluster/utils"
	"github.com/wind-c/comqtt/v2/config"

	"github.com/hashicorp/raft"
	raftdb "github.com/hashicorp/raft-boltdb/v2"
)

const (
	// maxPool controls how many connections we will pool.
	maxPool = 3

	// DefaultRaftTimeout is used to apply I/O deadlines. For InstallSnapshot, we multiply
	// DefaultRaftTimeout by (SnapshotSize / TimeoutScale).
	DefaultRaftTimeout = 10 * time.Second

	// raft storage
	raftDBFile = "raft.db"

	// peers file
	peersFIle = "peers.json"

	// The `retain` parameter controls how many
	// snapshots are retained. Must be at least 1.
	raftSnapShotRetain    = 2
	raftSnapshotThreshold = 1024

	// raftLogCacheSize is the maximum number of logs to cache in-memory.
	// This is used to reduce disk I/O for the recently committed entries.
	raftLogCacheSize = 512

	leaderWaitDelay  = 300 * time.Millisecond
	appliedWaitDelay = 500 * time.Millisecond
)

type Peer struct {
	config    *raft.Config
	raft      *raft.Raft
	fsm       *Fsm
	store     *raftdb.BoltStore
	transport raft.Transport
}

// peerEntry is used when decoding a new-style peers.json.
type peerEntry struct {
	// ID is the ID of the server (a UUID, usually).
	ID string `json:"id"`

	// Address is the host:port of the server.
	Address string `json:"address"`

	// NonVoter controls the suffrage. We choose this sense so people
	// can leave this out and get a Voter by default.
	NonVoter bool `json:"non_voter"`
}

func Setup(conf *config.Cluster, notifyCh chan<- *message.Message) (*Peer, error) {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(conf.NodeName)
	if conf.RaftLogLevel == "" {
		conf.RaftLogLevel = "ERROR"
	}
	config.LogLevel = conf.RaftLogLevel
	config.LogOutput = log.Writer()
	//config.ShutdownOnRemove = true             // Enable shutdown on removal
	//config.SnapshotInterval = 30 * time.Second // Check every 30 seconds to see if there are enough new entries for a snapshot, can be overridden
	//config.SnapshotThreshold = 16384           // Snapshots are created every 16384 entries by default, can be overridden
	//config.HeartbeatTimeout = 1000 * time.Millisecond
	//config.electionTimeout = 1000 * time.Millisecond
	//config.CommitTimeout = 500 * time.Millisecond
	//config.LeaderLeaseTimeout = 1000 * time.Millisecond

	var transport raft.Transport
	raftAddr := net.JoinHostPort(conf.BindAddr, strconv.Itoa(conf.RaftPort))
	addr, err := net.ResolveTCPAddr("tcp", raftAddr)
	if err != nil {
		return nil, err
	}
	if transport, err = raft.NewTCPTransport(raftAddr, addr, maxPool, DefaultRaftTimeout, config.LogOutput); err != nil {
		return nil, err
	}

	// create custom transport
	//transport := newRaftTrans(l)

	snapshot, err := raft.NewFileSnapshotStore(conf.RaftDir, raftSnapShotRetain, config.LogOutput)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(conf.RaftDir, raftDBFile)
	store, err := raftdb.NewBoltStore(path)
	if err != nil {
		return nil, err
	}
	stable := store

	// Wrap the store in a LogCache to improve performance.
	logCache, err := raft.NewLogCache(raftLogCacheSize, store)
	if err != nil {
		return nil, err
	}
	fm := NewFsm(notifyCh)

	// Manual Recovery Using peers.json，we need to create a [conf.RaftDir]/peers.json file. It should look something like:
	/*
		[
		  {
		    "id": "node1",
		    "address": "10.1.0.1:4647"
		  },
		  {
		    "id": "node2",
		    "address": "10.1.0.2:4647"
		  }
		]
	*/
	peersFile := filepath.Join(conf.RaftDir, peersFIle)
	if utils.PathExists(peersFile) {
		log.Info("found peers.json file, recovering Raft configuration...")

		var configuration raft.Configuration
		configuration, err = raft.ReadConfigJSON(peersFile)
		if err != nil {
			return nil, fmt.Errorf("recovery failed to parse peers.json: %v", err)
		}

		if err := raft.RecoverCluster(config, fm, logCache, stable, snapshot, transport, configuration); err != nil {
			return nil, fmt.Errorf("recovery failed: %v", err)
		}

		if err := os.RemoveAll(peersFile); err != nil {
			return nil, fmt.Errorf("recovery failed to delete peers.json, please delete manually (see peers.info for details): %v", err)
		}

		log.Info("deleted peers.json file after successful recovery")
	}

	if conf.RaftBootstrap {
		hasState, err := raft.HasExistingState(logCache, store, snapshot)
		if err != nil {
			return nil, err
		}

		if !hasState {
			configuration := raft.Configuration{
				Servers: []raft.Server{
					{
						ID:      config.LocalID,
						Address: transport.LocalAddr(),
					},
				},
			}
			if err := raft.BootstrapCluster(config, logCache, stable, snapshot, transport, configuration); err != nil {
				log.Error("raft bootstrap cluster", "error", err)
				return nil, err
			}
		}
	}

	rf, err := raft.NewRaft(config, fm, logCache, stable, snapshot, transport)
	if err != nil {
		return nil, err
	}

	peer := &Peer{config, rf, fm, store, transport}
	if id, err := peer.waitForLeader(peer.electionTimeout() * 3); err != nil {
		log.Warn("timeout waiting for raft leader", "leader", "unknown")
	} else {
		log.Info("found raft leader", "leader", id)
	}

	return peer, nil
}

func (p *Peer) Stop() {
	// close net transport
	if tp, ok := p.transport.(*raft.NetworkTransport); ok {
		tp.Close()
	}

	// snapshot
	if err := p.snapshot().Error(); err != "" {
		log.Warn("failed to create snapshot!")
	}

	// close raft
	shutdownFuture := p.raft.Shutdown()
	if err := shutdownFuture.Error(); err != nil {
		log.Error("shutdown raft", "error", err)
	}
	// close store
	p.store.Close()
}

func (p *Peer) Propose(msg *message.Message) error {
	future := p.raft.Apply(msg.MsgpackBytes(), appliedWaitDelay)
	if err := future.Error(); err != nil {
		return err
	}
	return nil
}

func (p *Peer) Lookup(key string) []string {
	return p.fsm.Lookup(key)
}

func (p *Peer) DelByNode(node string) int {
	return p.fsm.DelByNode(node)
}

func (p *Peer) electionTimeout() time.Duration {
	return p.config.ElectionTimeout
}

func (p *Peer) IsApplyRight() bool {
	return p.raft.State() == raft.Leader
}

func (p *Peer) GetLeader() (addr, id string) {
	leaderAddr, leaderId := p.raft.LeaderWithID()
	addr = string(leaderAddr)
	id = string(leaderId)
	return
}

func (p *Peer) Status() ([]byte, error) {
	return json.Marshal(p.raft.Stats())
}

func (p *Peer) Join(nodeId, addr string) error {
	if !p.IsApplyRight() {
		return errors.New("not leader cannot join a node")
	}

	configFuture := p.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return errors.New("failed to get raft configuration")
	}

	for _, srv := range configFuture.Configuration().Servers {
		// If a node already exists with either the joining node's ID or address,
		// that node may need to be removed from the config first.
		if srv.ID == raft.ServerID(nodeId) || srv.Address == raft.ServerAddress(addr) {
			// However if *both* the ID and the address are the same, then nothing -- not even
			// a join operation -- is needed.
			if srv.Address == raft.ServerAddress(addr) && srv.ID == raft.ServerID(nodeId) {
				log.Warn("it is already a cluster member, ignoring join request", "node", nodeId, "addr", addr)
				return nil
			}

			future := p.raft.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				return fmt.Errorf("error removing existing node %s at %s: %s", nodeId, addr, err)
			}
		}
	}

	f := p.raft.AddVoter(raft.ServerID(nodeId), raft.ServerAddress(addr), 0, 0)
	if err := f.Error(); err != nil {
		return err
	}

	return nil
}

func (p *Peer) Leave(nodeId string) error {
	if !p.IsApplyRight() {
		// not leader cannot remove a node
		return nil
	}

	cf := p.raft.GetConfiguration()
	if err := cf.Error(); err != nil {
		return errors.New("failed to get raft configuration")
	}

	for _, server := range cf.Configuration().Servers {
		if server.ID == raft.ServerID(nodeId) {
			f := p.raft.RemoveServer(server.ID, 0, 0)
			if err := f.Error(); err != nil {
				return fmt.Errorf("failed to remove server %s", nodeId)
			}
			return nil
		}
	}

	return nil
}

func (p *Peer) snapshot() error {
	f := p.raft.Snapshot()
	return f.Error()
}

func (p *Peer) peerEntries() ([]peerEntry, error) {
	future := p.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, err
	}
	entries := make([]peerEntry, len(future.Configuration().Servers))
	for i := range future.Configuration().Servers {
		entries[i] = peerEntry{
			ID:       string(future.Configuration().Servers[i].ID),
			Address:  string(future.Configuration().Servers[i].Address),
			NonVoter: future.Configuration().Servers[i].Suffrage != 0,
		}
	}
	return entries, nil
}

func (p *Peer) GenPeersFile(file string) error {
	entries, err := p.peerEntries()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	f, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	content, err := json.Marshal(&entries)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		return err
	}
	return nil
}

// waitForLeader blocks until a leader is detected, or the timeout expires.
func (p *Peer) waitForLeader(timeout time.Duration) (string, error) {
	ticker := time.NewTicker(leaderWaitDelay)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var id string
	for {
		if _, id = p.GetLeader(); id != "" {
			return id, nil
		}
		select {
		case <-p.raft.LeaderCh():
			if _, id = p.GetLeader(); id != "" {
				return id, nil
			}
		case <-ticker.C:
		case <-timer.C:
			return "", fmt.Errorf("wait for leader timed out")
		}
	}
}
