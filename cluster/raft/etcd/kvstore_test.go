package etcd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/cluster/message"
	base "github.com/wind-c/comqtt/v2/cluster/raft"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// A snapshot must survive a getSnapshot -> recoverFromSnapshot round trip:
// the routing table has to be readable afterwards, and any state held
// before the snapshot must be discarded.
func TestKVStoreRecoverFromSnapshot(t *testing.T) {
	src := &KVStore{KV: base.NewKV()}
	src.Add("topic/a", "node1")
	src.Add("topic/a", "node2")

	snapshot, err := src.getSnapshot()
	require.NoError(t, err)

	notifyCh := make(chan *message.Message, 8)
	dst := &KVStore{KV: base.NewKV(), notifyCh: notifyCh}
	dst.Add("topic/stale", "nodeX")

	require.NoError(t, dst.recoverFromSnapshot(snapshot))
	require.ElementsMatch(t, []string{"node1", "node2"}, dst.Lookup("topic/a"))
	require.Empty(t, dst.Lookup("topic/stale"))

	// restored filters are replayed so the local subscription tree can be rebuilt
	require.Equal(t, 1, len(notifyCh))
	msg := <-notifyCh
	require.Equal(t, packets.Subscribe, msg.Type)
	require.Equal(t, "topic/a", string(msg.Payload))
	require.Equal(t, "node1,node2", msg.NodeID)
}

// A corrupt snapshot must leave the existing state untouched.
func TestKVStoreRecoverFromCorruptSnapshot(t *testing.T) {
	src := &KVStore{KV: base.NewKV()}
	src.Add("topic/a", "node1")
	snapshot, err := src.getSnapshot()
	require.NoError(t, err)

	dst := &KVStore{KV: base.NewKV()}
	dst.Add("topic/b", "node2")

	require.Error(t, dst.recoverFromSnapshot(snapshot[:len(snapshot)/2]))
	require.ElementsMatch(t, []string{"node2"}, dst.Lookup("topic/b"))
	require.Empty(t, dst.Lookup("topic/a"))
}
