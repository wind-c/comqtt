package hashicorp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/config"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func createTestPeer(t *testing.T) *Peer {
	conf := &config.Cluster{
		NodeName:      "node1",
		BindAddr:      "127.0.0.1",
		RaftImpl:      config.RaftImplHashicorp,
		RaftPort:      8946,
		RaftDir:       t.TempDir(),
		RaftBootstrap: true,
	}
	notifyCh := make(chan *message.Message, 1)
	peer, err := Setup(conf, notifyCh)
	require.Nil(t, err)
	return peer
}

func TestJoinAndLeave(t *testing.T) {
	peer := createTestPeer(t)
	defer peer.Stop()

	nodeID := "node2"
	nodeAddr := "127.0.0.1:8947"

	// Test Join
	err := peer.Join(nodeID, nodeAddr)
	require.NoError(t, err)

	configFuture := peer.raft.GetConfiguration()
	require.NoError(t, configFuture.Error())

	var found bool
	for _, server := range configFuture.Configuration().Servers {
		if server.ID == raft.ServerID(nodeID) && server.Address == raft.ServerAddress(nodeAddr) {
			found = true
			break
		}
	}
	require.True(t, found)

	// Test Leave
	err = peer.Leave(nodeID)
	require.NoError(t, err)

	configFuture = peer.raft.GetConfiguration()
	require.NoError(t, configFuture.Error())

	found = false
	for _, server := range configFuture.Configuration().Servers {
		if server.ID == raft.ServerID(nodeID) && server.Address == raft.ServerAddress(nodeAddr) {
			found = true
			break
		}
	}
	require.False(t, found)
}

func TestProposeAndLookup(t *testing.T) {
	peer := createTestPeer(t)
	defer peer.Stop()

	msg := &message.Message{
		Type:    packets.Subscribe,
		NodeID:  "node1",
		Payload: []byte("filter"),
	}

	err := peer.Propose(msg)
	require.NoError(t, err)

	key := "filter"
	expectedValue := "node1"

	time.Sleep(2 * time.Second)

	result := peer.Lookup(key)
	require.Equal(t, []string{expectedValue}, result)
}

func TestIsApplyRight(t *testing.T) {
	peer := createTestPeer(t)
	defer peer.Stop()

	require.True(t, peer.IsApplyRight())
}

func TestGetLeader(t *testing.T) {
	peer := createTestPeer(t)
	defer peer.Stop()

	addr, id := peer.GetLeader()
	require.Equal(t, "127.0.0.1:8946", addr)
	require.Equal(t, "node1", id)
}

func TestGenPeersFile(t *testing.T) {
	peer := createTestPeer(t)
	defer peer.Stop()

	file := filepath.Join(t.TempDir(), "peers.json")

	err := peer.GenPeersFile(file)
	require.NoError(t, err)

	_, err = os.Stat(file)
	require.False(t, os.IsNotExist(err))

	content, err := os.ReadFile(file)
	require.NoError(t, err)

	expectedContent := `[
		{
			"id": "node1",
			"address": "127.0.0.1:8946",
			"non_voter":false
		}
	]`

	require.JSONEq(t, expectedContent, string(content))
}
