package listeners

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	quic "github.com/quic-go/quic-go"
)

// maxIncomingStreams is the maximum number of MQTT connections (QUIC streams)
// a single peer may hold open on one QUIC connection. Every accepted stream
// starts a goroutine and allocates client state before authentication, so the
// quic-go default of 100 lets one handshake reserve far too much.
const maxIncomingStreams int64 = 16

// establishReadTimeout bounds the first read on a new stream. MQTT requires a
// CONNECT packet first, and until it is fully read the stream holds a goroutine
// and client state before authentication. Once the client is authenticated the
// server replaces this deadline with the keepalive deadline, so established
// sessions are unaffected. It is a variable so tests can shorten it.
var establishReadTimeout = 10 * time.Second

type (
	QuicListener interface {
		Accept(context.Context) (*quic.Conn, error)
		Addr() net.Addr
		Close() error
	}

	// QUIC is a listener for establishing client connections on basic QUIC protocol.
	QUIC struct { // [MQTT-4.2.0-1]
		sync.RWMutex
		id      string       // the internal id of the listener
		address string       // the network address to bind to
		listen  QuicListener // a QuicListener which will listen for new clients
		config  *Config      // configuration values for the listener
		log     *slog.Logger // server logger
		end     uint32       // ensure the close methods are only called once
	}
)

// NewQUIC initialises and returns a new QUIC listener, listening on an address.
func NewQUIC(id, address string, config *Config) *QUIC {
	if config == nil {
		config = new(Config)
	}

	return &QUIC{
		id:      id,
		address: address,
		config:  config,
	}
}

// ID returns the id of the listener.
func (l *QUIC) ID() string {
	return l.id
}

// Address returns the address of the listener.
func (l *QUIC) Address() string {
	return l.address
}

// Protocol returns the address of the listener.
func (l *QUIC) Protocol() string {
	return "quic"
}

// Init initializes the listener.
func (l *QUIC) Init(log *slog.Logger) error {
	l.log = log

	config := &quic.Config{
		MaxIncomingStreams: maxIncomingStreams,
	}

	var err error

	if l.config.ZeroRTT {
		l.listen, err = quic.ListenAddrEarly(l.address, l.config.TLSConfig, config)
	} else {
		l.listen, err = quic.ListenAddr(l.address, l.config.TLSConfig, config)
	}

	return err
}

// Serve starts waiting for new QUIC connections, and calls the establish
// connection callback for any received.
func (l *QUIC) Serve(establish EstablishFn) {
	for {
		if atomic.LoadUint32(&l.end) == 1 {
			return
		}

		conn, err := l.listen.Accept(context.Background())
		if err != nil {
			return
		}

		go l.handleConn(conn, establish)
	}
}

func (l *QUIC) handleConn(
	conn *quic.Conn,
	establish EstablishFn,
) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return
		}

		netConn := newQUICNet(conn, stream)

		if atomic.LoadUint32(&l.end) == 0 {
			// Bound the unauthenticated CONNECT read (see establishReadTimeout).
			_ = netConn.SetReadDeadline(time.Now().Add(establishReadTimeout))

			go func() {
				if err := establish(l.id, netConn); err != nil {
					l.log.Warn("", "error", err)
					netConn.Close()
				}
			}()
		}
	}
}

// Close closes the listener and any client connections.
func (l *QUIC) Close(closeClients CloseFn) {
	l.Lock()
	defer l.Unlock()

	if atomic.CompareAndSwapUint32(&l.end, 0, 1) {
		closeClients(l.id)
	}

	if l.listen != nil {
		err := l.listen.Close()
		if err != nil {
			return
		}
	}
}
