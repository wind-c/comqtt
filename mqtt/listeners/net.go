// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 mochi-mqtt, mochi-co
// SPDX-FileContributor: Jeroen Rinzema

package listeners

import (
	"net"
	"sync"
	"sync/atomic"

	"log/slog"
)

// Net is a listener for establishing client connections on basic TCP protocol.
type Net struct { // [MQTT-4.2.0-1]
	mu       sync.Mutex
	listener net.Listener // a net.Listener which will listen for new clients
	id       string       // the internal id of the listener
	log      *slog.Logger // server logger
	end      uint32       // ensure the close methods are only called once
	done     chan struct{}
}

// NewNet initialises and returns a listener serving incoming connections on the given net.Listener
func NewNet(id string, listener net.Listener) *Net {
	return &Net{
		id:       id,
		listener: listener,
		done:     make(chan struct{}),
	}
}

// ID returns the id of the listener.
func (l *Net) ID() string {
	return l.id
}

// Address returns the address of the listener.
func (l *Net) Address() string {
	return l.listener.Addr().String()
}

// Protocol returns the network of the listener.
func (l *Net) Protocol() string {
	return l.listener.Addr().Network()
}

// Init initializes the listener.
func (l *Net) Init(log *slog.Logger) error {
	l.log = log
	return nil
}

// Serve starts waiting for new TCP connections, and calls the establish
// connection callback for any received.
func (l *Net) Serve(establish EstablishFn) {
	accept := l.listener.Accept

	for {
		conn, err := acceptRetry(accept, isNetClosed, l.done, l.log)
		if err != nil {
			return
		}

		if atomic.LoadUint32(&l.end) == 1 {
			// Raced with Close: nothing will ever serve this connection.
			conn.Close()
			return
		}

		go func() {
			if err := establish(l.id, conn); err != nil {
				l.log.Warn("", "error", err)
			}
		}()
	}
}

// Close closes the listener and any client connections.
func (l *Net) Close(closeClients CloseFn) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if atomic.CompareAndSwapUint32(&l.end, 0, 1) {
		// Unblocks Serve if it is sleeping in an accept backoff.
		close(l.done)
		closeClients(l.id)
	}

	if l.listener != nil {
		err := l.listener.Close()
		if err != nil {
			return
		}
	}
}
