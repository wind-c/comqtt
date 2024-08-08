package serf

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/utils"
	"github.com/wind-c/comqtt/v2/config"
)

func TestMain(m *testing.M) {
	log.Init(log.DefaultOptions())
	code := m.Run()
	os.Exit(code)
}

func TestJoinAndLeave(t *testing.T) {
	bindPort1, err := utils.GetFreePort()
	assert.NoError(t, err)
	conf1 := &config.Cluster{
		BindAddr: "127.0.0.1",
		BindPort: bindPort1,
		NodeName: "test-node-1",
	}
	inboundMsgCh1 := make(chan []byte)
	membership1 := New(conf1, inboundMsgCh1)
	err = membership1.Setup()
	assert.NoError(t, err)
	defer membership1.Stop()

	assert.Equal(t, 1, membership1.numMembers())

	bindPort2, err := utils.GetFreePort()
	assert.NoError(t, err)
	conf2 := &config.Cluster{
		BindAddr: "127.0.0.1",
		BindPort: bindPort2,
		NodeName: "test-node-2",
	}
	inboundMsgCh2 := make(chan []byte)
	membership2 := New(conf2, inboundMsgCh2)
	err = membership2.Setup()
	assert.NoError(t, err)
	defer membership2.Stop()

	numJoined, err := membership2.Join([]string{"127.0.0.1:" + strconv.Itoa(bindPort1)})
	assert.NoError(t, err)
	time.Sleep(3 * time.Second)
	assert.Equal(t, numJoined, 1)
	assert.Equal(t, 2, membership1.numMembers())
	assert.Equal(t, 2, membership2.numMembers())

	t.Log("Leave node 2")
	err = membership2.Leave()
	assert.NoError(t, err)

	time.Sleep(5 * time.Second)

	assert.Equal(t, 1, membership1.numMembers())
}

func TestSendToNode(t *testing.T) {
	bindPort1, err := utils.GetFreePort()
	assert.NoError(t, err)
	bindPort2, err := utils.GetFreePort()
	assert.NoError(t, err)

	conf1 := &config.Cluster{
		BindAddr: "127.0.0.1",
		BindPort: bindPort1,
		NodeName: "test-node-1",
	}
	conf2 := &config.Cluster{
		BindAddr: "127.0.0.1",
		BindPort: bindPort2,
		NodeName: "test-node-2",
		Members:  []string{"127.0.0.1:" + strconv.Itoa(bindPort1)},
	}
	inboundMsgCh1 := make(chan []byte)
	inboundMsgCh2 := make(chan []byte)

	membership1 := New(conf1, inboundMsgCh1)
	err = membership1.Setup()
	assert.NoError(t, err)
	defer membership1.Stop()

	membership2 := New(conf2, inboundMsgCh2)
	err = membership2.Setup()
	assert.NoError(t, err)
	defer membership2.Stop()

	time.Sleep(3 * time.Second)

	err = membership1.SendToNode("test-node-2", []byte("test message"))
	assert.NoError(t, err)

	select {
	case msg := <-inboundMsgCh2:
		assert.Equal(t, []byte("test message"), msg)
	case <-time.After(5 * time.Second):
		t.Fatal("Did not receive the message in membership2")
	}
}

func TestSendToOthers(t *testing.T) {
	bindPort1, err := utils.GetFreePort()
	assert.NoError(t, err)
	bindPort2, err := utils.GetFreePort()
	assert.NoError(t, err)
	bindPort3, err := utils.GetFreePort()
	assert.NoError(t, err)

	conf1 := &config.Cluster{
		BindAddr: "127.0.0.1",
		BindPort: bindPort1,
		NodeName: "test-node-1",
	}
	conf2 := &config.Cluster{
		BindAddr: "127.0.0.1",
		BindPort: bindPort2,
		NodeName: "test-node-2",
		Members:  []string{"127.0.0.1:" + strconv.Itoa(bindPort1)},
	}
	conf3 := &config.Cluster{
		BindAddr: "127.0.0.1",
		BindPort: bindPort3,
		NodeName: "test-node-3",
		Members:  []string{"127.0.0.1:" + strconv.Itoa(bindPort1)},
	}
	inboundMsgCh1 := make(chan []byte)
	inboundMsgCh2 := make(chan []byte)
	inboundMsgCh3 := make(chan []byte)

	membership1 := New(conf1, inboundMsgCh1)
	err = membership1.Setup()
	assert.NoError(t, err)
	defer membership1.Stop()

	membership2 := New(conf2, inboundMsgCh2)
	err = membership2.Setup()
	assert.NoError(t, err)
	defer membership2.Stop()

	membership3 := New(conf3, inboundMsgCh3)
	err = membership3.Setup()
	assert.NoError(t, err)
	defer membership3.Stop()

	time.Sleep(3 * time.Second)

	membership1.SendToOthers([]byte("test message"))

	select {
	case msg := <-inboundMsgCh2:
		assert.Equal(t, []byte("test message"), msg)
	case <-time.After(5 * time.Second):
		t.Fatal("Did not receive the message in membership2")
	}

	select {
	case msg := <-inboundMsgCh3:
		assert.Equal(t, []byte("test message"), msg)
	case <-time.After(5 * time.Second):
		t.Fatal("Did not receive the message in membership3")
	}
}
