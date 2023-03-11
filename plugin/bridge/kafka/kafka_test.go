package kafka

import (
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/mqtt"
	"github.com/wind-c/comqtt/mqtt/packets"
	"github.com/wind-c/comqtt/plugin"
	"os"
	"testing"
)

var (
	logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Level(zerolog.Disabled)

	client = &mqtt.Client{
		ID: "test",
		Net: mqtt.ClientConnection{
			Remote:   "test.addr",
			Listener: "listener",
		},
		Properties: mqtt.ClientProperties{
			Username: []byte("zhangsan"),
			Clean:    false,
		},
	}

	pkp = packets.Packet{TopicName: "a/b/c", Payload: []byte("hello")}
	pkf = packets.Packet{Filters: packets.Subscriptions{{Filter: "a/b/c"}}}
	pkc = packets.Packet{Connect: packets.ConnectParams{Password: []byte("123456")}}
)

func teardown(t *testing.T, b *Bridge) {
	if b.writer != nil {
		err := b.Stop()
		require.NoError(t, err)
	}
}

func newBridge(t *testing.T) *Bridge {
	b := new(Bridge)
	b.SetOpts(&logger, nil)
	opts := &Options{}
	plugin.LoadYaml("./conf.yml", opts)
	err := b.Init(opts)
	require.NoError(t, err)
	return b
}

func TestOnSessionEstablished(t *testing.T) {
	b := newBridge(t)
	defer teardown(t, b)

	b.OnSessionEstablished(client, pkc)
}

func TestOnPublished(t *testing.T) {
	b := newBridge(t)
	defer teardown(t, b)

	b.OnPublished(client, pkp)
}

func TestOnSubscribed(t *testing.T) {
	b := newBridge(t)
	defer teardown(t, b)

	b.OnSubscribed(client, pkf, []byte{0}, []int{1})
}
