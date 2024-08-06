package etcd

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/message"
	"github.com/wind-c/comqtt/v2/config"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func createTestPeer(t *testing.T) *Peer {
	conf := &config.Cluster{
		NodeName:      "1",
		BindAddr:      "127.0.0.1",
		RaftImpl:      config.RaftImplEtcd,
		RaftPort:      8948,
		RaftDir:       t.TempDir(),
		RaftBootstrap: true,
	}
	notifyCh := make(chan *message.Message, 1)
	peer, err := Setup(conf, notifyCh)
	require.Nil(t, err)
	return peer
}

func TestJoinAndLeave(t *testing.T) {
	log.Init(log.DefaultOptions())
	peer := createTestPeer(t)
	defer peer.Stop()

	node2ID := "2"
	node2Host := "127.0.0.1"
	node2Port := 8949
	conf2 := &config.Cluster{
		NodeName:      node2ID,
		BindAddr:      node2Host,
		RaftImpl:      config.RaftImplEtcd,
		RaftPort:      node2Port,
		RaftDir:       t.TempDir(),
		RaftBootstrap: true,
	}
	notifyCh := make(chan *message.Message, 1)
	_, err := Setup(conf2, notifyCh)
	require.NoError(t, err)

	node2IDUint, err := strconv.ParseUint(node2ID, 10, 64)
	require.NoError(t, err)

	// Test Join
	err = peer.Join(node2ID, net.JoinHostPort(node2Host, strconv.Itoa(node2Port)))
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	found := false
	for _, majorityConfig := range peer.node.Status().Config.Voters {
		if _, ok := majorityConfig[node2IDUint]; ok {
			found = true
			break
		}
	}
	require.True(t, found)

	err = peer.Leave(node2ID)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	found = false
	for _, c := range peer.node.Status().Config.Voters {
		t.Log(found)
		if _, ok := c[node2IDUint]; ok {
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
		NodeID:  "1",
		Payload: []byte("filter"),
	}

	err := peer.Propose(msg)
	require.NoError(t, err)

	key := "filter"
	expectedValue := "1"

	time.Sleep(2 * time.Second)

	result := peer.Lookup(key)
	require.Equal(t, []string{expectedValue}, result)
}

func TestGetLeader(t *testing.T) {
	peer := createTestPeer(t)
	defer peer.Stop()

	time.Sleep(2 * time.Second)

	_, id := peer.GetLeader()
	require.Equal(t, "1", id)
}
