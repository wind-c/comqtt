package cluster

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/config"
)

func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func TestCluster(t *testing.T) {
	log.Init(log.DefaultOptions())

	bindPort1, err := getFreePort()
	require.NoError(t, err, "Failed to get free port for node1")
	raftPort1, err := getFreePort()
	require.NoError(t, err, "Failed to get free port for node1 Raft")

	bindPort2, err := getFreePort()
	require.NoError(t, err, "Failed to get free port for node2")
	raftPort2, err := getFreePort()
	require.NoError(t, err, "Failed to get free port for node2 Raft")

	bindPort3, err := getFreePort()
	require.NoError(t, err, "Failed to get free port for node3")
	raftPort3, err := getFreePort()
	require.NoError(t, err, "Failed to get free port for node3 Raft")

	members := []string{
		"127.0.0.1:" + strconv.Itoa(bindPort1),
		"127.0.0.1:" + strconv.Itoa(bindPort2),
		"127.0.0.1:" + strconv.Itoa(bindPort3),
	}

	conf1 := &config.Cluster{
		NodeName:      "node1",
		RaftImpl:      config.RaftImplHashicorp,
		BindAddr:      "127.0.0.1",
		BindPort:      bindPort1,
		RaftPort:      raftPort1,
		RaftBootstrap: true,
		GrpcEnable:    false,
		Members:       members,
		DiscoveryWay:  config.DiscoveryWaySerf,
	}
	agent1 := NewAgent(conf1)
	err = agent1.Start()
	require.NoError(t, err, "Agent start failed for node: %s", conf1.NodeName)

	conf2 := &config.Cluster{
		NodeName:      "node2",
		RaftImpl:      config.RaftImplHashicorp,
		BindAddr:      "127.0.0.1",
		BindPort:      bindPort2,
		RaftPort:      raftPort2,
		RaftBootstrap: false,
		GrpcEnable:    false,
		Members:       members,
		DiscoveryWay:  config.DiscoveryWaySerf,
	}
	agent2 := NewAgent(conf2)
	err = agent2.Start()
	defer agent2.Stop()
	require.NoError(t, err, "Agent start failed for node: %s", conf2.NodeName)

	conf3 := &config.Cluster{
		NodeName:      "node3",
		RaftImpl:      config.RaftImplHashicorp,
		BindAddr:      "127.0.0.1",
		BindPort:      bindPort3,
		RaftPort:      raftPort3,
		RaftBootstrap: false,
		GrpcEnable:    false,
		Members:       members,
		DiscoveryWay:  config.DiscoveryWaySerf,
	}
	agent3 := NewAgent(conf3)
	err = agent3.Start()
	defer agent3.Stop()
	require.NoError(t, err, "Agent start failed for node: %s", conf3.NodeName)

	time.Sleep(5 * time.Second)

	_, leader1 := agent1.raftPeer.GetLeader()
	_, leader2 := agent2.raftPeer.GetLeader()
	_, leader3 := agent3.raftPeer.GetLeader()

	require.Equal(t, leader1, "node1")
	require.Equal(t, leader2, "node1")
	require.Equal(t, leader3, "node1")

	members1 := agent1.GetMemberList()
	members2 := agent2.GetMemberList()
	members3 := agent3.GetMemberList()

	require.Equal(t, len(members1), 3)
	require.Equal(t, len(members2), 3)
	require.Equal(t, len(members3), 3)

	// Stop agent1 and check new leader
	agent1.Stop()
	time.Sleep(5 * time.Second)

	_, newLeader2 := agent2.raftPeer.GetLeader()
	_, newLeader3 := agent3.raftPeer.GetLeader()

	// Check that either agent2 or agent3 becomes the new leader
	if newLeader2 == "node2" || newLeader2 == "node3" {
		require.Equal(t, newLeader2, newLeader3, "Leaders should be the same for agent2 and agent3")
	} else {
		require.Fail(t, "New leader should be either node2 or node3")
	}

	// Restart agent1 and verify it is a follower
	err = agent1.Start()
	require.NoError(t, err, "Agent restart failed for node: %s", conf1.NodeName)
	defer agent1.Stop()

	time.Sleep(5 * time.Second)

	_, leaderAfterRestart1 := agent1.raftPeer.GetLeader()
	_, leaderAfterRestart2 := agent2.raftPeer.GetLeader()
	_, leaderAfterRestart3 := agent3.raftPeer.GetLeader()

	require.Equal(t, leaderAfterRestart2, leaderAfterRestart1)
	require.Equal(t, leaderAfterRestart3, leaderAfterRestart1)

	require.NotEqual(t, leaderAfterRestart1, "node1", "After restart, node1 should not be the leader")

	t.Log("Test completed successfully")
}
