// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package listeners

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// flakyListener fails Accept a fixed number of times before delegating to a
// real listener, standing in for EMFILE / ECONNABORTED.
type flakyListener struct {
	net.Listener
	remaining int
}

func (f *flakyListener) Accept() (net.Conn, error) {
	if f.remaining > 0 {
		f.remaining--
		return nil, errors.New("simulated EMFILE")
	}
	return f.Listener.Accept()
}

// Serve must survive a transient Accept failure. Listeners.Serve calls
// listener.Serve exactly once with nothing to restart it, so a single failure
// that returns takes the listener down permanently and silently.
func TestServeSurvivesTransientAcceptError(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	l := NewTCP("t", base.Addr().String(), nil)
	l.listen = &flakyListener{Listener: base, remaining: 1}
	l.log = discardLog()

	returned := make(chan struct{})
	go func() {
		l.Serve(func(_ string, c net.Conn) error { _ = c.Close(); return nil })
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("Serve returned after a single transient accept error: the listener is dead for good")
	case <-time.After(500 * time.Millisecond):
	}
}

// The client-visible consequence of a dead listener is worse than a refused
// connection: dials keep succeeding and then hang, occupying an accept slot.
func TestClientIsServedAfterTransientAcceptError(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	l := NewTCP("t", base.Addr().String(), nil)
	l.listen = &flakyListener{Listener: base, remaining: 1}
	l.log = discardLog()

	go l.Serve(func(_ string, c net.Conn) error {
		defer c.Close()
		_, err := c.Write([]byte("served"))
		return err
	})

	time.Sleep(200 * time.Millisecond) // let the injected failure happen and recover

	c, err := net.Dial("tcp", base.Addr().String())
	require.NoError(t, err, "dial must succeed")
	defer c.Close()

	require.NoError(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, 16)
	n, err := c.Read(buf)
	require.NoError(t, err, "a dead listener leaves the client hanging, not refused")
	require.Equal(t, "served", string(buf[:n]))
}

// A listener closed deliberately must stop promptly, without waiting out a
// backoff.
func TestServeStopsPromptlyOnClose(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	l := NewTCP("t", base.Addr().String(), nil)
	// base is already bound, so wire it up directly rather than calling Init,
	// which would try to bind the same address again.
	l.listen = base
	l.log = discardLog()

	returned := make(chan struct{})
	go func() {
		l.Serve(func(_ string, c net.Conn) error { _ = c.Close(); return nil })
		close(returned)
	}()

	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	l.Close(func(string) {})
	<-returned
	require.Less(t, time.Since(start), time.Second, "Close must not wait for a backoff")
}