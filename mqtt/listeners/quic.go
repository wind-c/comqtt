package listeners

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	quic "github.com/quic-go/quic-go"
)

// QUIC is a listener for establishing client connections on basic QUIC protocol.
type QUIC struct { // [MQTT-4.2.0-1]
	sync.RWMutex
	id      string         // the internal id of the listener
	address string         // the network address to bind to
	listen  *quic.Listener // a quic.Listener which will listen for new clients
	config  *Config        // configuration values for the listener
	log     *slog.Logger   // server logger
	end     uint32         // ensure the close methods are only called once
}

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

	var err error

	l.listen, err = quic.ListenAddr(l.address, l.config.TLSConfig, nil)

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
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return
	}

	netConn := newQUICNet(conn, stream)

	if atomic.LoadUint32(&l.end) == 0 {
		if err := establish(l.id, netConn); err != nil {
			l.log.Warn("", "error", err)
			netConn.Close()
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
