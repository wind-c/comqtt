package listeners

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
)

// quicTestAddr binds to an OS-assigned ephemeral UDP port so that
// successive QUIC tests don't fight over the same fixed port - the
// previous test's UDP socket isn't always released by quic-go quickly
// enough for the next test to reuse it.
const quicTestAddr = ":0"

func TestNewQUIC(t *testing.T) {
	l := NewQUIC("t1", quicTestAddr, nil)
	require.Equal(t, "t1", l.id)
	require.Equal(t, quicTestAddr, l.address)
}

func TestQUICID(t *testing.T) {
	l := NewQUIC("t1", quicTestAddr, nil)
	require.Equal(t, "t1", l.ID())
}

func TestQUICAddress(t *testing.T) {
	l := NewQUIC("t1", quicTestAddr, nil)
	require.Equal(t, quicTestAddr, l.Address())
}

func TestQUICProtocol(t *testing.T) {
	l := NewQUIC("t1", quicTestAddr, nil)
	require.Equal(t, "quic", l.Protocol())
}

func TestQUICInit(t *testing.T) {
	l := NewQUIC("t2", quicTestAddr, &Config{
		TLSConfig: tlsConfigBasic,
	})
	err := l.Init(logger)
	l.Close(MockCloser)
	require.NoError(t, err)
	require.NotNil(t, l.config.TLSConfig)
}

func TestQUICServeAndClose(t *testing.T) {
	l := NewQUIC("t1", quicTestAddr, &Config{
		TLSConfig: tlsConfigBasic,
	})
	err := l.Init(logger)
	require.NoError(t, err)

	o := make(chan bool)
	go func(o chan bool) {
		l.Serve(MockEstablisher)
		o <- true
	}(o)

	time.Sleep(time.Millisecond)

	var closed bool
	l.Close(func(id string) {
		closed = true
	})

	require.True(t, closed)
	<-o

	l.Close(MockCloser)      // coverage: close closed
	l.Serve(MockEstablisher) // coverage: serve closed
}

func TestQUICEstablishThenEnd(t *testing.T) {
	tlsConfig := tlsConfigBasic.Clone()

	tlsConfig.MinVersion = tls.VersionTLS13
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.KeyLogWriter = os.Stdout
	tlsConfig.NextProtos = []string{"mqtt"}
	l := NewQUIC("t1", quicTestAddr, &Config{
		TLSConfig: tlsConfig,
	})
	err := l.Init(logger)
	require.NoError(t, err)

	o := make(chan bool)
	established := make(chan bool)
	go func() {
		l.Serve(func(id string, c net.Conn) error {
			established <- true
			return errors.New("ending") // return an error to exit immediately
		})
		o <- true
	}()

	time.Sleep(time.Millisecond)
	_, port, _ := net.SplitHostPort(l.listen.Addr().String())
	conn, err := quic.DialAddr(context.Background(), fmt.Sprintf("127.0.0.1:%s", port), tlsConfig, nil)
	require.NoError(t, err)
	stream, err := conn.OpenStream()
	require.NoError(t, err)
	stream.Close()
	require.Equal(t, true, <-established)
	l.Close(MockCloser)
	<-o
}

func TestQUICRTTEstablishThenEnd(t *testing.T) {
	tlsConfig := tlsConfigBasic.Clone()
	tlsConfig.MinVersion = tls.VersionTLS13
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.KeyLogWriter = os.Stdout
	tlsConfig.NextProtos = []string{"mqtt"}
	l := NewQUIC("t1", quicTestAddr, &Config{
		TLSConfig: tlsConfig,
	})
	err := l.Init(logger)
	require.NoError(t, err)

	o := make(chan bool)
	established := make(chan bool)
	go func() {
		l.Serve(func(id string, c net.Conn) error {
			established <- true
			return errors.New("ending") // return an error to exit immediately
		})
		o <- true
	}()

	time.Sleep(time.Millisecond)
	_, port, _ := net.SplitHostPort(l.listen.Addr().String())
	conn, err := quic.DialAddrEarly(context.Background(), fmt.Sprintf("127.0.0.1:%s", port), tlsConfig, nil)
	require.NoError(t, err)
	stream, err := conn.OpenStream()
	require.NoError(t, err)
	stream.Close()
	require.Equal(t, true, <-established)
	l.Close(MockCloser)
	<-o
}
