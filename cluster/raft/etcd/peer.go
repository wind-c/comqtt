// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package etcd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.etcd.io/etcd/client/pkg/v3/transport"
	"go.etcd.io/etcd/client/pkg/v3/types"
	"go.etcd.io/etcd/raft/v3"
	"go.etcd.io/etcd/raft/v3/raftpb"
	"go.etcd.io/etcd/server/v3/etcdserver/api/rafthttp"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	stats "go.etcd.io/etcd/server/v3/etcdserver/api/v2stats"
	"go.etcd.io/etcd/server/v3/wal"
	"go.etcd.io/etcd/server/v3/wal/walpb"
)

var (
	ErrInvalidID = errors.New("node name must be a number")

	appliedWaitDelay = 100 * time.Millisecond
)

type commit struct {
	data       [][]byte
	applyDoneC chan<- struct{}
}

type Peer struct {
	conf        *config.Cluster
	kvStore     *KVStore
	proposeC    chan *message.Message  // proposed messages (k, v)
	confChangeC chan raftpb.ConfChange // proposed cluster config changes
	commitC     chan *commit           // entries committed to log (k, v)
	errorC      chan error             // errors from raft session

	id          uint64   // client ID for raft session
	peers       []string // raft peer URLs
	walDir      string   // path to WAL directory
	snapDir     string   // path to snapshot directory
	getSnapshot func() ([]byte, error)

	confState     raftpb.ConfState
	snapshotIndex uint64
	appliedIndex  uint64

	node        raft.Node
	raftStorage *raft.MemoryStorage
	wal         *wal.WAL

	snapshotter      *snap.Snapshotter
	snapshotterReady chan *snap.Snapshotter // signals when snapshotter is ready

	snapCount uint64

	transport *rafthttp.Transport

	stopC     chan struct{} // signals proposal channel closed
	httpStopC chan struct{} // signals http server to shutdown
	httpDoneC chan struct{} // signals http server shutdown complete

	logger *zap.Logger
}

var defaultSnapshotCount uint64 = 10000

// Setup initiates a raft instance and returns a committed log entry
// channel and error channel. Proposals for log updates are sent over the
// provided the proposal channel. All log entries are replayed over the
// commit channel, followed by a nil message (to indicate the channel is
// current), then new log entries. To shutdown, close proposeC and read errorC.
func Setup(conf *config.Cluster, notifyCh chan<- *message.Message) (*Peer, error) {
	id, err := strconv.Atoi(conf.NodeName)
	if err != nil {
		return nil, ErrInvalidID
	}

	peer := &Peer{
		conf:             conf,
		proposeC:         make(chan *message.Message),
		confChangeC:      make(chan raftpb.ConfChange),
		commitC:          make(chan *commit),
		errorC:           make(chan error),
		id:               uint64(id),
		peers:            genPeers(conf),
		walDir:           fmt.Sprintf("%v-%d", conf.RaftDir, id),
		snapDir:          fmt.Sprintf("%v-snapshot%d", conf.RaftDir, id),
		confState:        raftpb.ConfState{},
		snapshotterReady: make(chan *snap.Snapshotter, 1),
		snapCount:        defaultSnapshotCount,
		stopC:            make(chan struct{}),
		httpStopC:        make(chan struct{}),
		httpDoneC:        make(chan struct{}),
		logger:           getZapLogger(conf.RaftLogLevel),
	}

	go peer.startRaft()
	peer.kvStore = newKVStore(<-peer.snapshotterReady, peer.commitC, peer.errorC, notifyCh)
	peer.getSnapshot = func() ([]byte, error) { return peer.kvStore.getSnapshot() }

	return peer, nil
}

func (p *Peer) genLocalAddr() string {
	addr := net.JoinHostPort(p.conf.BindAddr, strconv.Itoa(p.conf.RaftPort))
	return addr
}

func genPeers(conf *config.Cluster) []string {
	addr := net.JoinHostPort(conf.BindAddr, strconv.Itoa(conf.RaftPort))
	return []string{"http://" + addr}
}

func genID(nodeName string) uint64 {
	var r uint64
	for _, c := range nodeName {
		r += uint64(c)
	}
	return r % 10
}

func (p *Peer) Join(nodeId, addr string) error {
	id, err := strconv.ParseUint(nodeId, 0, 64)
	if err != nil {
		return ErrInvalidID
	}

	cc := raftpb.ConfChange{
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  id,
		Context: []byte("http://" + addr),
	}
	p.confChangeC <- cc

	return nil
}

func (p *Peer) Leave(nodeId string) error {
	id, err := strconv.ParseUint(nodeId, 0, 64)
	if err != nil {
		return ErrInvalidID
	}

	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: id,
	}
	p.confChangeC <- cc

	return nil
}

func (p *Peer) Propose(msg *message.Message) error {
	p.proposeC <- msg
	return nil
}

func (p *Peer) Lookup(key string) []string {
	rs := p.kvStore.Lookup(key)
	return rs
}

func (p *Peer) DelByNode(node string) int {
	return p.kvStore.DelByNode(node)
}

func mapRaftLogLevelToZap(raftLogLevel string) zapcore.Level {
	raftLogLevel = strings.ToLower(raftLogLevel)
	switch raftLogLevel {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.ErrorLevel
	}
}

func getZapLogger(raftLogLevel string) *zap.Logger {
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), zapcore.AddSync(log.Writer()), mapRaftLogLevelToZap(raftLogLevel))
	return zap.New(core)
}

func (p *Peer) startRaft() {
	if !fileutil.Exist(p.snapDir) {
		if err := os.MkdirAll(p.snapDir, 0750); err != nil {
			log.Fatal("[raft] failed to create dir for dnapshot", "error", err)
		}
	}
	p.snapshotter = snap.New(p.logger, p.snapDir)

	oldWal := wal.Exist(p.walDir)
	p.wal = p.replayWAL()

	// signal replay has finished
	p.snapshotterReady <- p.snapshotter

	npeers := make([]raft.Peer, len(p.peers))
	for i := range p.peers {
		npeers[i] = raft.Peer{ID: uint64(i + 1)}
	}

	c := &raft.Config{
		ID:                        p.id,
		ElectionTick:              10,
		HeartbeatTick:             1,
		Storage:                   p.raftStorage,
		MaxSizePerMsg:             1024 * 1024,
		MaxUncommittedEntriesSize: 256,
		MaxInflightMsgs:           1 << 30,
	}

	if oldWal {
		p.node = raft.RestartNode(c)
	} else {
		p.node = raft.StartNode(c, npeers)
	}

	p.transport = &rafthttp.Transport{
		Logger:      p.logger,
		TLSInfo:     transport.TLSInfo{},
		ID:          types.ID(p.id),
		ClusterID:   0x1199,
		Raft:        p,
		ServerStats: stats.NewServerStats("", ""),
		LeaderStats: stats.NewLeaderStats(p.logger, strconv.Itoa(int(p.id))),
		ErrorC:      make(chan error),
	}

	if err := p.transport.Start(); err != nil {
		log.Fatal("[raft] transport start", "error", err)
	}

	for i := range p.peers {
		if uint64(i+1) != p.id {
			p.transport.AddPeer(types.ID(i+1), []string{p.peers[i]})
		}
	}

	go p.serveRaft()
	go p.serveChannels()
}

func (p *Peer) serveRaft() {
	addr := net.JoinHostPort(p.conf.BindAddr, strconv.Itoa(p.conf.RaftPort))
	listener, err := newStoppableListener(addr, p.httpStopC)
	if err != nil {
		log.Fatal("[raft] failed ti listen rafthttp", "error", err)
	}

	log.Info("[raft] http is listeing at", "host", p.genLocalAddr())
	err = (&http.Server{Handler: p.transport.Handler()}).Serve(listener)
	select {
	case <-p.httpStopC:
	default:
		log.Fatal("[raft] failed to serve rafthttp", "error", err)
	}
	close(p.httpStopC)
}

func (p *Peer) serveChannels() {
	snapshot, err := p.raftStorage.Snapshot()
	if err != nil {
		log.Fatal("[raft] snapshot", "error", err)
	}
	p.confState = snapshot.Metadata.ConfState
	p.snapshotIndex = snapshot.Metadata.Index
	p.appliedIndex = snapshot.Metadata.Index

	defer p.wal.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// send proposals over raft
	go func() {
		confChangeCount := uint64(0)

		for p.proposeC != nil && p.confChangeC != nil {
			select {
			case prop, ok := <-p.proposeC:
				if !ok {
					p.proposeC = nil
				} else {
					if err := p.node.Propose(context.TODO(), prop.MsgpackBytes()); err != nil {
						log.Error("Propose", "error", err, "filter", prop.Payload, "nid", prop.NodeID, "type", prop.Type)
					}
				}

			case cc, ok := <-p.confChangeC:
				if !ok {
					p.confChangeC = nil
				} else {
					confChangeCount++
					cc.ID = confChangeCount
					if err := p.node.ProposeConfChange(context.TODO(), cc); err != nil {
						log.Error("ProposeConfChange", "error", err, "nid", cc.NodeID, "type", cc.Type)
					}
				}
			}
		}
		// client closed channel; shutdown raft if not ready
		close(p.stopC)
	}()

	// event loop on raft state machine updates
	for {
		select {
		case <-ticker.C:
			p.node.Tick()

		// store raft entries to wal, then publish over commit channel
		case rd := <-p.node.Ready():
			// must save the snapshot file and WAL snapshot entry before saving any other entries
			// or hard state to ensure that recovery after a snapshot restore is possible.

			if !raft.IsEmptySnap(rd.Snapshot) {
				if err := p.saveSnap(rd.Snapshot); err != nil {
					log.Error("saveSnap", "error", err)
				}
			}
			if err := p.wal.Save(rd.HardState, rd.Entries); err != nil {
				log.Error("wal.Save", "error", err)
			}
			if !raft.IsEmptySnap(rd.Snapshot) {
				p.raftStorage.ApplySnapshot(rd.Snapshot)
				p.publishSnapshot(rd.Snapshot)
			}
			p.raftStorage.Append(rd.Entries)
			p.transport.Send(p.processMessages(rd.Messages))
			applyDoneC, ok := p.publishEntries(p.entriesToApply(rd.CommittedEntries))
			if !ok {
				p.Stop()
				return
			}
			p.maybeTriggerSnapshot(applyDoneC)
			p.node.Advance()

		case err = <-p.transport.ErrorC:
			p.writeError(err)
			return

		case <-p.stopC:
			p.Stop()
			return
		}
	}
}

// replayWAL replays WAL entries into the raft instance.
func (p *Peer) replayWAL() *wal.WAL {

	log.Info("[raft] replaying WAL of membe", "id", p.id)
	snapshot := p.loadSnapshot()
	w := p.openWAL(snapshot)
	_, st, ents, err := w.ReadAll()
	if err != nil {
		log.Error("[raft] failed to read WAL", "error", err)
	}
	p.raftStorage = raft.NewMemoryStorage()
	if snapshot != nil {
		p.raftStorage.ApplySnapshot(*snapshot)
	}
	p.raftStorage.SetHardState(st)

	// append to storage so raft starts st the right place in log
	p.raftStorage.Append(ents)

	return w
}

func (p *Peer) loadSnapshot() *raftpb.Snapshot {
	if wal.Exist(p.walDir) {
		walSnaps, err := wal.ValidSnapshotEntries(p.logger, p.walDir)
		if err != nil {
			log.Fatal("[raft] error listening snapshots", "error", err)
		}
		snapshot, err := p.snapshotter.LoadNewestAvailable(walSnaps)
		if err != nil && err != snap.ErrNoSnapshot {
			log.Fatal("[raft] error loading snapshit", "error", err)
		}
		return snapshot
	}
	return &raftpb.Snapshot{}
}

// openWAL returns a WAL ready for reading.
func (p *Peer) openWAL(snapshot *raftpb.Snapshot) *wal.WAL {
	if !wal.Exist(p.walDir) {
		err := os.Mkdir(p.walDir, 0750)
		if err != nil {
			log.Fatal("[raft] cannot create dir for wal", "error", err)
		}
		w, err := wal.Create(p.logger, p.walDir, nil)
		if err != nil {
			log.Fatal("[raft] create dir for error", "error", err)
		}
		w.Close()
	}

	walSnap := walpb.Snapshot{}
	if snapshot != nil {
		walSnap.Index, walSnap.Term = snapshot.Metadata.Index, snapshot.Metadata.Term
	}
	log.Info("[raft] loading WAL", "term", walSnap.Term, "index", walSnap.Index)
	w, err := wal.Open(p.logger, p.walDir, walSnap)
	if err != nil {
		log.Fatal("[raft] error loading wal", "error", err)
	}
	return w
}

func (p *Peer) saveSnap(snap raftpb.Snapshot) error {
	walSnap := walpb.Snapshot{
		Index:     snap.Metadata.Index,
		Term:      snap.Metadata.Term,
		ConfState: &snap.Metadata.ConfState,
	}
	if err := p.snapshotter.SaveSnap(snap); err != nil {
		return err
	}
	if err := p.wal.SaveSnapshot(walSnap); err != nil {
		return err
	}
	return p.wal.ReleaseLockTo(snap.Metadata.Index)
}

func (p *Peer) entriesToApply(ents []raftpb.Entry) (nents []raftpb.Entry) {
	if len(ents) == 0 {
		return ents
	}
	firstIdx := ents[0].Index
	if firstIdx > p.appliedIndex+1 {
		log.Fatal("[raft] fisrt index of committed entry should <= progress.appliedIndex+1", "first-idx", firstIdx, "fapplied-idx", p.appliedIndex)
	}
	if p.appliedIndex-firstIdx+1 < uint64(len(ents)) {
		nents = ents[p.appliedIndex-firstIdx+1:]
	}
	return nents
}

func (p *Peer) publishSnapshot(snapshotToSave raftpb.Snapshot) {
	if raft.IsEmptySnap(snapshotToSave) {
		return
	}

	log.Info("[raft] publishing snapshot at index", "snap-idx", p.snapshotIndex)
	defer log.Info("[raft] finished publishing snapshot at index", "snap-idx", p.snapshotIndex)
	if snapshotToSave.Metadata.Index <= p.appliedIndex {
		log.Info("[raft] snapshot index shuold > progress.appliedIndex", "snap-idx", snapshotToSave.Metadata.Index, "applied-idx", p.appliedIndex)
	}

	p.commitC <- nil // trigger kvstore to load snapshot

	p.confState = snapshotToSave.Metadata.ConfState
	p.snapshotIndex = snapshotToSave.Metadata.Index
	p.appliedIndex = snapshotToSave.Metadata.Index
}

// publishEntries writes committed log entries to commit channel and
// returns all entries could be published.
func (p *Peer) publishEntries(ents []raftpb.Entry) (<-chan struct{}, bool) {
	if len(ents) == 0 {
		return nil, true
	}

	data := make([][]byte, 0, len(ents))
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				// ignore empty messages
				break
			}
			data = append(data, ents[i].Data)
		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(ents[i].Data); err != nil {
				break
			}
			p.confState = *p.node.ApplyConfChange(cc)
			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				if len(cc.Context) > 0 {
					p.transport.AddPeer(types.ID(cc.NodeID), []string{string(cc.Context)})
					log.Info("[raft] node is added to the cluster", "node", cc.NodeID)
				}
			case raftpb.ConfChangeRemoveNode:
				//if cc.NodeID == p.id {
				//	logger.Info().Uint64("node", p.id).Msg("[raft] node is removed from the cluster. shutting down.")
				//	return nil, false
				//}
				p.transport.RemovePeer(types.ID(cc.NodeID))
				log.Info("[raft] node is removed to the cluster", "node", cc.NodeID)
			}
		}
	}

	var applyDoneC chan struct{}

	if len(data) > 0 {
		applyDoneC = make(chan struct{}, 1)
		select {
		case p.commitC <- &commit{data, applyDoneC}:
		case <-p.stopC:
			return nil, false
		}
	}

	// after commit, update appliedIndex
	p.appliedIndex = ents[len(ents)-1].Index

	return applyDoneC, true
}

var snapshotCatchUpEntriesN uint64 = 10000

func (p *Peer) maybeTriggerSnapshot(applyDoneC <-chan struct{}) {
	if p.appliedIndex-p.snapshotIndex <= p.snapCount {
		return
	}

	// wait until all committed entries are applied (or server is closed)
	if applyDoneC != nil {
		select {
		case <-applyDoneC:
		case <-p.stopC:
			return
		}
	}

	log.Info("[raft] start snapshot", "applied-idx", p.appliedIndex, "snapshot-idx", p.snapshotIndex)
	data, err := p.getSnapshot()
	if err != nil {
		log.Fatal("[raft] get snapshot", "error", err)
	}
	snapshot, err := p.raftStorage.CreateSnapshot(p.appliedIndex, &p.confState, data)
	if err != nil {
		log.Fatal("[raft] create snapshot", "error", err)
	}
	if err = p.saveSnap(snapshot); err != nil {
		log.Fatal("[raft] save snapshot", "error", err)
	}

	compactIndex := uint64(1)
	if p.appliedIndex > snapshotCatchUpEntriesN {
		compactIndex = p.appliedIndex - snapshotCatchUpEntriesN
	}
	if err = p.raftStorage.Compact(compactIndex); err != nil {
		log.Fatal("[raft] compact snapshot", "error", err)
	}

	log.Info("compacted log at index", "compact-idx", compactIndex)
	p.snapshotIndex = p.appliedIndex
}

// When there is a `raftpb.EntryConfChange` after creating the snapshot,
// then the confState included in the snapshot is out of date, so we need
// to update the confState before sending a snapshot to a follower.
func (p *Peer) processMessages(ms []raftpb.Message) []raftpb.Message {
	for i := 0; i < len(ms); i++ {
		if ms[i].Type == raftpb.MsgSnap {
			ms[i].Snapshot.Metadata.ConfState = p.confState
		}
	}
	return ms
}

func (p *Peer) writeError(err error) {
	p.stopHTTP()
	close(p.commitC)
	p.errorC <- err
	close(p.errorC)
	p.node.Stop()
}

func (p *Peer) Stop() {
	p.stopHTTP()
	close(p.commitC)
	close(p.errorC)
	p.node.Stop()
}

func (p *Peer) stopHTTP() {
	p.transport.Stop()
	close(p.httpStopC)
	<-p.httpStopC
}

func (p *Peer) IsApplyRight() bool {
	return true
}

func (p *Peer) GetLeader() (addr, id string) {
	return
}

func (p *Peer) GenPeersFile(file string) error {
	return nil
}

func (p *Peer) Process(ctx context.Context, m raftpb.Message) error {
	return p.node.Step(ctx, m)
}

func (p *Peer) IsIDRemoved(id uint64) bool {
	return false
}

func (p *Peer) ReportUnreachable(id uint64) {
	p.node.ReportUnreachable(id)
}

func (p *Peer) ReportSnapshot(id uint64, status raft.SnapshotStatus) {
	p.node.ReportSnapshot(id, status)
}
