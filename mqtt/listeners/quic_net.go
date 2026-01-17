package listeners

import (
	"net"
	"time"

	quic "github.com/quic-go/quic-go"
)

type quicNet struct {
	stream *quic.Stream
	conn   *quic.Conn
}

func newQUICNet(
	conn *quic.Conn,
	stream *quic.Stream,
) net.Conn {
	return &quicNet{
		stream: stream,
		conn:   conn,
	}
}

func (q *quicNet) Read(p []byte) (int, error) {
	return q.stream.Read(p)
}

func (q *quicNet) Write(p []byte) (int, error) {
	return q.stream.Write(p)
}

func (q *quicNet) Close() error {
	return q.stream.Close()
}

func (q *quicNet) LocalAddr() net.Addr {
	return q.conn.LocalAddr()
}

func (q *quicNet) RemoteAddr() net.Addr {
	return q.conn.RemoteAddr()
}

func (q *quicNet) SetDeadline(t time.Time) error {
	if err := q.stream.SetReadDeadline(t); err != nil {
		return err
	}
	return q.stream.SetWriteDeadline(t)
}

func (q *quicNet) SetReadDeadline(t time.Time) error {
	return q.stream.SetReadDeadline(t)
}

func (q *quicNet) SetWriteDeadline(t time.Time) error {
	return q.stream.SetWriteDeadline(t)
}
