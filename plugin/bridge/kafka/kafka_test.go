package kafka

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/plugin"
)

var (
	// Currently, the input is directed to /dev/null. If you need to
	// output to stdout, just modify 'io.Discard' here to 'os.Stdout'.
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))

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
	b.SetOpts(logger, nil)
	opts := &Options{}
	err := plugin.LoadYaml("./conf.yml", opts)
	require.NoError(t, err)
	err = b.Init(opts)
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

// TestBridge calls all the methods of the bridge and checks that the writer is called.
func TestBridge(t *testing.T) {
	b := newBridge(t)
	writer := newMockWriter()
	b.writer = writer
	b.OnSessionEstablished(client, pkc)
	require.Equal(t, 1, writer.count(), "writer not called on session established")
	b.OnDisconnect(client, errors.New("test"), true)
	require.Equal(t, 2, writer.count(), "writer not called on disconnect")
	b.OnPublished(client, pkp)
	require.Equal(t, 3, writer.count(), "writer not called on publish")
	b.OnSubscribed(client, pkf, []byte{0}, []int{1})
	require.Equal(t, 4, writer.count(), "writer not called	on subscribe")
	b.OnUnsubscribed(client, pkf, []byte{0}, []int{1})
	require.Equal(t, 5, writer.count(), "writer not called on unsubscribe")
	err := writer.Close()
	require.NoError(t, err, "writer close failed")
	if !writer.isClosed() {
		t.Error("writer not closed")
	}
}

type mockWriter struct {
	mu       sync.Mutex
	messages []kafka.Message
	closed   bool
	verbose  bool
}

func newMockWriter() *mockWriter {
	return &mockWriter{
		messages: make([]kafka.Message, 0),
	}
}

func (m *mockWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msgs...)
	if m.verbose {
		for _, msg := range msgs {
			fmt.Printf("mockWriter: %s\n", msg.Value)
		}
	}
	return nil
}

func (m *mockWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockWriter) getMessages() []kafka.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages
}

func (m *mockWriter) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func (m *mockWriter) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}
