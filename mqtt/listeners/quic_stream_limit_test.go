// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package listeners

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
)

// TestQUICLimitsConcurrentStreams verifies that a single peer cannot open an
// unbounded number of MQTT connections (QUIC streams) on one QUIC connection.
// Each accepted stream starts a goroutine and allocates client state before the
// MQTT CONNECT packet arrives, so the listener must cap them.
func TestQUICLimitsConcurrentStreams(t *testing.T) {
	tlsConfig := tlsConfigBasic.Clone()
	tlsConfig.MinVersion = tls.VersionTLS13
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.NextProtos = []string{"mqtt"}

	l := NewQUIC("t1", quicTestAddr, &Config{TLSConfig: tlsConfig})
	require.NoError(t, l.Init(logger))

	release := make(chan struct{})
	var established int32
	go l.Serve(func(id string, c net.Conn) error {
		atomic.AddInt32(&established, 1)
		<-release
		return nil
	})

	_, port, err := net.SplitHostPort(l.listen.Addr().String())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, net.JoinHostPort("127.0.0.1", port), tlsConfig, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.CloseWithError(0, "")
		close(release)
		l.Close(MockCloser)
	})

	opened := 0
	for i := 0; i < 100; i++ {
		sctx, scancel := context.WithTimeout(ctx, 300*time.Millisecond)
		stream, err := conn.OpenStreamSync(sctx)
		scancel()
		if err != nil {
			break
		}
		opened++
		// Only the first CONNECT byte: the establish handler would block in
		// the remaining-length reader if it were allowed to run.
		_, _ = stream.Write([]byte{0x10})
	}

	require.Greater(t, opened, 0, "no stream was opened")
	require.LessOrEqual(t, opened, int(maxIncomingStreams),
		"peer opened %d streams on one connection", opened)

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&established) >= int32(opened)
	}, 2*time.Second, 10*time.Millisecond)

	// Any handler spawned beyond the cap would show up here.
	time.Sleep(100 * time.Millisecond)
	require.LessOrEqual(t, atomic.LoadInt32(&established), int32(maxIncomingStreams),
		"%d establish handlers were started", atomic.LoadInt32(&established))
}

// TestQUICClosesStalledStream verifies that a peer which sends only the first
// byte of an MQTT CONNECT packet cannot hold the establish handler open
// indefinitely. The listener replaces the deadline with the keepalive deadline
// once the client is authenticated.
func TestQUICClosesStalledStream(t *testing.T) {
	prev := establishReadTimeout
	establishReadTimeout = 200 * time.Millisecond
	t.Cleanup(func() { establishReadTimeout = prev })

	tlsConfig := tlsConfigBasic.Clone()
	tlsConfig.MinVersion = tls.VersionTLS13
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.NextProtos = []string{"mqtt"}

	l := NewQUIC("t1", quicTestAddr, &Config{TLSConfig: tlsConfig})
	require.NoError(t, l.Init(logger))
	t.Cleanup(func() { l.Close(MockCloser) })

	go l.Serve(func(id string, c net.Conn) error {
		buf := make([]byte, 1)
		if _, err := c.Read(buf); err != nil { // the single CONNECT byte
			return err
		}
		_, err := c.Read(buf) // blocks in the remaining-length reader
		return err
	})

	_, port, err := net.SplitHostPort(l.listen.Addr().String())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, net.JoinHostPort("127.0.0.1", port), tlsConfig, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseWithError(0, "") })

	stream, err := conn.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = stream.Write([]byte{0x10})
	require.NoError(t, err)

	// The server must close the stream when the CONNECT read deadline fires.
	// Without the deadline the read below fails with the client-side timeout,
	// not EOF.
	_ = stream.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = stream.Read(make([]byte, 1))
	require.True(t, errors.Is(err, io.EOF), "expected the server to close the stream, got %v", err)
}
