package clients

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"

	"github.com/wind-c/comqtt/server/events"
	"github.com/wind-c/comqtt/server/internal/circ"
	"github.com/wind-c/comqtt/server/internal/packets"
	"github.com/wind-c/comqtt/server/listeners/auth"
	"github.com/wind-c/comqtt/server/system"
)

var (
	// defaultKeepalive is the default connection keepalive value in seconds.
	defaultKeepalive uint16 = 10

	// ErrConnectionClosed is returned when operating on a closed
	// connection and/or when no error cause has been given.
	ErrConnectionClosed = errors.New("Connection not open")
)

// Clients contains a map of the clients known by the broker.
type Clients struct {
	sync.RWMutex
	internal map[string]*Client // clients known by the broker, keyed on client id.
}

// New returns an instance of Clients.
func New() *Clients {
	return &Clients{
		internal: make(map[string]*Client),
	}
}

// Add adds a new client to the clients map, keyed on client id.
func (cl *Clients) Add(val *Client) {
	cl.Lock()
	cl.internal[val.ID] = val
	cl.Unlock()
}

// Get returns the value of a client if it exists.
func (cl *Clients) Get(id string) (*Client, bool) {
	cl.RLock()
	val, ok := cl.internal[id]
	cl.RUnlock()
	return val, ok
}

// Len returns the length of the clients map.
func (cl *Clients) Len() int {
	cl.RLock()
	val := len(cl.internal)
	cl.RUnlock()
	return val
}

// Delete removes a client from the internal map.
func (cl *Clients) Delete(id string) {
	cl.Lock()
	delete(cl.internal, id)
	cl.Unlock()
}

// GetByListener returns clients matching a listener id.
func (cl *Clients) GetByListener(id string) []*Client {
	clients := make([]*Client, 0, cl.Len())
	cl.RLock()
	for _, v := range cl.internal {
		if v.Listener == id && atomic.LoadUint32(&v.State.Done) == 0 {
			clients = append(clients, v)
		}
	}
	cl.RUnlock()
	return clients
}

// GetAll returns all clients
func (cl *Clients) GetAll() map[string]*Client {
	return cl.internal
}

// Client contains information about a client known by the broker.
type Client struct {
	State           State                         // the operational state of the client.
	LWT             LWT                           // the last will and testament for the client.
	Inflight        Inflight                      // a map of in-flight qos messages.
	sync.RWMutex                                  // mutex
	Username        []byte                        // the username the client authenticated with.
	AC              auth.Controller               // an auth controller inherited from the listener.
	Listener        string                        // the id of the listener the client is connected to.
	ID              string                        // the client id.
	conn            net.Conn                      // the net.Conn used to establish the connection.
	R               *circ.Reader                  // a reader for reading incoming bytes.
	W               *circ.Writer                  // a writer for writing outgoing bytes.
	Subscriptions   map[string]packets.SubOptions // a map of the subscription filters a client maintains.
	systemInfo      *system.Info                  // pointers to server system info.
	packetID        uint32                        // the current highest packetID.
	keepalive       uint16                        // the number of seconds the connection can wait.
	CleanSession    bool                          // indicates if the client expects a clean-session.
	ProtocolVersion byte                          // mqtt protocol version, optional value: v3.1 = 3、v3.1.1 = 4、v5.0 = 5
	TopicAlias      map[uint16]string             // key is alias, value is topic, for v5

	encoder func(*packets.Packet) (*bytes.Buffer, error)
	decoder func(*[]byte, *packets.Packet) error
}

// State tracks the state of the client.
type State struct {
	started   *sync.WaitGroup // tracks the goroutines which have been started.
	endedW    *sync.WaitGroup // tracks when the writer has ended.
	endedR    *sync.WaitGroup // tracks when the reader has ended.
	Done      uint32          // atomic counter which indicates that the client has closed.
	endOnce   sync.Once       // only end once.
	stopCause atomic.Value    // reason for stopping (error).
}

// NewClient returns a new instance of Client.
func NewClient(c net.Conn, r *circ.Reader, w *circ.Writer, s *system.Info, rm int) *Client {
	cl := &Client{
		conn:          c,
		R:             r,
		W:             w,
		systemInfo:    s,
		keepalive:     defaultKeepalive,
		Inflight:      NewMap(rm),
		Subscriptions: make(map[string]packets.SubOptions),
		State: State{
			started: new(sync.WaitGroup),
			endedW:  new(sync.WaitGroup),
			endedR:  new(sync.WaitGroup),
		},
		ProtocolVersion: 4,
		TopicAlias:      make(map[uint16]string),
	}
	// default v3
	cl.encoder = cl.encodePacketV3x
	cl.decoder = cl.decodePacketV3x

	cl.refreshDeadline(cl.keepalive)

	return cl
}

// NewClientStub returns an instance of Client with basic initializations. This
// method is typically called by the persistence restoration system.
func NewClientStub(s *system.Info, rm int) *Client {
	return &Client{
		Inflight:      NewMap(rm),
		Subscriptions: make(map[string]packets.SubOptions),
		State: State{
			Done: 1,
		},
	}
}

// Identify sets the identification values of a client instance.
func (cl *Client) Identify(lid string, pk packets.Packet, ac auth.Controller) {
	cl.Listener = lid
	cl.AC = ac

	cl.ID = pk.ClientIdentifier
	if cl.ID == "" {
		cl.ID = xid.New().String()
	}

	cl.R.ID = cl.ID + " READER"
	cl.W.ID = cl.ID + " WRITER"

	cl.Username = pk.Username
	cl.CleanSession = pk.CleanSession
	cl.keepalive = pk.Keepalive

	if pk.WillFlag {
		cl.LWT = LWT{
			Topic:   pk.WillTopic,
			Message: pk.WillMessage,
			Qos:     pk.WillQos,
			Retain:  pk.WillRetain,
		}
	}

	cl.refreshDeadline(cl.keepalive)
}

// refreshDeadline refreshes the read/write deadline for the net.Conn connection.
func (cl *Client) refreshDeadline(keepalive uint16) {
	if cl.conn != nil {
		var expiry time.Time // Nil time can be used to disable deadline if keepalive = 0
		if keepalive > 0 {
			expiry = time.Now().Add(time.Duration(keepalive+(keepalive/2)) * time.Second)
		}
		_ = cl.conn.SetDeadline(expiry)
	}
}

// Info returns an event-version of a client, containing minimal information.
func (cl *Client) Info() events.Client {
	addr := "unknown"
	if cl.conn != nil && cl.conn.RemoteAddr() != nil {
		addr = cl.conn.RemoteAddr().String()
	}
	return events.Client{
		ID:           cl.ID,
		Remote:       addr,
		Listener:     cl.Listener,
		Username:     cl.Username,
		CleanSession: cl.CleanSession,
	}
}

// NextPacketID returns the next packet id for a client, looping back to 0
// if the maximum ID has been reached.
func (cl *Client) NextPacketID() uint32 {
	i := atomic.LoadUint32(&cl.packetID)
	if i == uint32(65535) || i == uint32(0) {
		atomic.StoreUint32(&cl.packetID, 1)
		return 1
	}

	return atomic.AddUint32(&cl.packetID, 1)
}

// NoteSubscription makes a note of a subscription for the client.
func (cl *Client) NoteSubscription(filter string, so packets.SubOptions) {
	cl.Lock()
	cl.Subscriptions[filter] = so
	cl.Unlock()
}

// ForgetSubscription forgests a subscription note for the client.
func (cl *Client) ForgetSubscription(filter string) {
	cl.Lock()
	delete(cl.Subscriptions, filter)
	cl.Unlock()
}

// Start begins the client goroutines reading and writing packets.
func (cl *Client) Start() {
	cl.State.started.Add(2)
	cl.State.endedW.Add(1)
	cl.State.endedR.Add(1)

	go func() {
		cl.State.started.Done()
		_, err := cl.W.WriteTo(cl.conn)
		if err != nil {
			err = fmt.Errorf("writer: %w", err)
		}
		cl.State.endedW.Done()
		cl.Stop(err)
	}()

	go func() {
		cl.State.started.Done()
		_, err := cl.R.ReadFrom(cl.conn)
		if err != nil {
			err = fmt.Errorf("reader: %w", err)
		}
		cl.State.endedR.Done()
		cl.Stop(err)
	}()

	cl.State.started.Wait()
}

// ClearBuffers sets the read/write buffers to nil so they can be
// deallocated automatically when no longer in use.
func (cl *Client) ClearBuffers() {
	cl.R = nil
	cl.W = nil
}

// Stop instructs the client to shut down all processing goroutines and disconnect.
// A cause error may be passed to identfy the reason for stopping.
func (cl *Client) Stop(err error) {
	if atomic.LoadUint32(&cl.State.Done) == 1 {
		return
	}

	cl.State.endOnce.Do(func() {
		cl.R.Stop()
		cl.W.Stop()
		cl.State.endedW.Wait()

		_ = cl.conn.Close() // omit close error

		cl.State.endedR.Wait()
		atomic.StoreUint32(&cl.State.Done, 1)

		if err == nil {
			err = ErrConnectionClosed
		}
		cl.State.stopCause.Store(err)
	})
}

// StopCause returns the reason the client connection was stopped, if any.
func (cl *Client) StopCause() error {
	if cl.State.stopCause.Load() == nil {
		return nil
	}
	return cl.State.stopCause.Load().(error)
}

// ReadFixedHeader reads in the values of the next packet's fixed header.
func (cl *Client) ReadFixedHeader(fh *packets.FixedHeader) error {
	p, err := cl.R.Read(1)
	if err != nil {
		return err
	}

	err = fh.Decode(p[0])
	if err != nil {
		return err
	}

	// The remaining length value can be up to 5 bytes. Read through each byte
	// looking for continue values, and if found increase the read. Otherwise
	// decode the bytes that were legit.
	buf := make([]byte, 0, 6)
	i := 1
	n := 2
	for ; n < 6; n++ {
		p, err = cl.R.Read(n)
		if err != nil {
			return err
		}

		buf = append(buf, p[i])

		// If it's not a continuation flag, end here.
		if p[i] < 128 {
			break
		}

		// If i has reached 4 without a length terminator, return a protocol violation.
		i++
		if i == 4 {
			return packets.ErrOversizedLengthIndicator
		}
	}

	// Calculate and store the remaining length of the packet payload.
	rem, _ := binary.Uvarint(buf)
	fh.Remaining = int(rem)

	// Having successfully read n bytes, commit the tail forward.
	cl.R.CommitTail(n)
	atomic.AddInt64(&cl.systemInfo.BytesRecv, int64(n))

	return nil
}

// readRemaining reads in the values of the next packet's fixed header.
func (cl *Client) readRemaining(fh *packets.FixedHeader) (bx *[]byte, err error) {
	atomic.AddInt64(&cl.systemInfo.MessagesRecv, 1)
	p, err := cl.R.Read(fh.Remaining)
	if err != nil {
		return
	}
	atomic.AddInt64(&cl.systemInfo.BytesRecv, int64(len(p)))

	// Decode the remaining packet values using a fresh copy of the bytes,
	// otherwise the next packet will change the data of this one.
	bs := append([]byte{}, p[:]...)
	bx = &bs

	return
}

// Read loops forever reading new packets from a client connection until
// an error is encountered (or the connection is closed).
func (cl *Client) Read(packetHandler func(*Client, packets.Packet) error) error {
	for {
		if atomic.LoadUint32(&cl.State.Done) == 1 && cl.R.CapDelta() == 0 {
			return nil
		}

		cl.refreshDeadline(cl.keepalive)
		fh := new(packets.FixedHeader)
		err := cl.ReadFixedHeader(fh)
		if err != nil {
			return err
		}

		pk, err := cl.ReadPacket(fh)
		if err != nil {
			return err
		}

		err = packetHandler(cl, pk) // Process inbound packet.
		if err != nil {
			return err
		}
	}
}

// ReadPacket reads the remaining buffer into an MQTT packet，not connect packet.
func (cl *Client) ReadPacket(fh *packets.FixedHeader) (pk packets.Packet, err error) {
	bx, err := cl.readRemaining(fh)
	pk.FixedHeader = *fh
	err = cl.decoder(bx, &pk)
	cl.R.CommitTail(pk.FixedHeader.Remaining)

	return
}

// ReadConnectPacket reads the remaining buffer into an MQTT connect packet.
func (cl *Client) ReadConnectPacket(fh *packets.FixedHeader) (pk packets.Packet, err error) {
	if fh.Remaining == 0 {
		return
	}
	bx, err := cl.readRemaining(fh)
	pk.FixedHeader = *fh
	err = pk.ConnectDecode(*bx)
	cl.ProtocolVersion = pk.ProtocolVersion
	if cl.ProtocolVersion == 5 {
		cl.encoder = cl.encodePacketV5
		cl.decoder = cl.decodePacketV5
	}
	cl.R.CommitTail(pk.FixedHeader.Remaining)

	return
}

// decodePacketV3x reads the remaining buffer into an MQTT packet.
func (cl *Client) decodePacketV3x(bx *[]byte, pk *packets.Packet) (err error) {
	switch pk.FixedHeader.Type {
	case packets.Connack:
		err = pk.ConnackDecode(*bx)
	case packets.Publish:
		err = pk.PublishDecode(*bx)
		if err == nil {
			atomic.AddInt64(&cl.systemInfo.PublishRecv, 1)
		}
	case packets.Puback:
		err = pk.PubackDecode(*bx)
	case packets.Pubrec:
		err = pk.PubrecDecode(*bx)
	case packets.Pubrel:
		err = pk.PubrelDecode(*bx)
	case packets.Pubcomp:
		err = pk.PubcompDecode(*bx)
	case packets.Subscribe:
		err = pk.SubscribeDecode(*bx)
	case packets.Suback:
		err = pk.SubackDecode(*bx)
	case packets.Unsubscribe:
		err = pk.UnsubscribeDecode(*bx)
	case packets.Unsuback:
		err = pk.UnsubackDecode(*bx)
	case packets.Pingreq:
	case packets.Pingresp:
	case packets.Disconnect:
	default:
		err = fmt.Errorf("No valid packet available; %v", pk.FixedHeader.Type)
	}

	return
}

// decodePacketV5 reads the remaining buffer into an MQTT packet.
func (cl *Client) decodePacketV5(bx *[]byte, pk *packets.Packet) (err error) {
	switch pk.FixedHeader.Type {
	case packets.Connack:
		err = pk.ConnectDecodeV5(*bx)
	case packets.Publish:
		err = pk.PublishDecodeV5(*bx)
		if err == nil {
			atomic.AddInt64(&cl.systemInfo.PublishRecv, 1)
		}
	case packets.Puback:
		err = pk.PubackDecodeV5(*bx)
	case packets.Pubrec:
		err = pk.PubrecDecodeV5(*bx)
	case packets.Pubrel:
		err = pk.PubrelDecodeV5(*bx)
	case packets.Pubcomp:
		err = pk.PubcompDecodeV5(*bx)
	case packets.Subscribe:
		err = pk.SubscribeDecodeV5(*bx)
	case packets.Suback:
		err = pk.SubackDecodeV5(*bx)
	case packets.Unsubscribe:
		err = pk.UnsubscribeDecodeV5(*bx)
	case packets.Unsuback:
		err = pk.UnsubackDecodeV5(*bx)
	case packets.Pingreq:
	case packets.Pingresp:
	case packets.Disconnect:
		err = pk.DisconnectDecodeV5(*bx)
	default:
		err = fmt.Errorf("No valid packet available; %v", pk.FixedHeader.Type)
	}

	return
}

// WritePacket encodes and writes a packet to the client.
func (cl *Client) WritePacket(pk packets.Packet) (n int, err error) {
	if atomic.LoadUint32(&cl.State.Done) == 1 {
		return 0, ErrConnectionClosed
	}

	cl.W.Mu.Lock()
	defer cl.W.Mu.Unlock()

	var buf *bytes.Buffer
	buf, err = cl.encoder(&pk)
	if err != nil {
		return
	}

	// Write the packet bytes to the client byte buffer.
	n, err = cl.W.Write(buf.Bytes())
	if err != nil {
		return
	}

	atomic.AddInt64(&cl.systemInfo.BytesSent, int64(n))
	atomic.AddInt64(&cl.systemInfo.MessagesSent, 1)

	cl.refreshDeadline(cl.keepalive)

	return
}

// encodePacketV3x encodes a packet
func (cl *Client) encodePacketV3x(pk *packets.Packet) (buf *bytes.Buffer, err error) {
	buf = new(bytes.Buffer)
	switch pk.FixedHeader.Type {
	case packets.Connect:
		err = pk.ConnectEncode(buf)
	case packets.Connack:
		err = pk.ConnackEncode(buf)
	case packets.Publish:
		err = pk.PublishEncode(buf)
		if err == nil {
			atomic.AddInt64(&cl.systemInfo.PublishSent, 1)
		}
	case packets.Puback:
		err = pk.PubackEncode(buf)
	case packets.Pubrec:
		err = pk.PubrecEncode(buf)
	case packets.Pubrel:
		err = pk.PubrelEncode(buf)
	case packets.Pubcomp:
		err = pk.PubcompEncode(buf)
	case packets.Subscribe:
		err = pk.SubscribeEncode(buf)
	case packets.Suback:
		err = pk.SubackEncode(buf)
	case packets.Unsubscribe:
		err = pk.UnsubscribeEncode(buf)
	case packets.Unsuback:
		err = pk.UnsubackEncode(buf)
	case packets.Pingreq:
		err = pk.PingreqEncode(buf)
	case packets.Pingresp:
		err = pk.PingrespEncode(buf)
	case packets.Disconnect:
		err = pk.DisconnectEncode(buf)
	default:
		err = fmt.Errorf("No valid packet available; %v", pk.FixedHeader.Type)
	}

	return
}

// encodePacketV5 encodes a packet
func (cl *Client) encodePacketV5(pk *packets.Packet) (buf *bytes.Buffer, err error) {
	buf = new(bytes.Buffer)
	switch pk.FixedHeader.Type {
	case packets.Connect:
		err = pk.ConnectEncodeV5(buf)
	case packets.Connack:
		err = pk.ConnackEncodeV5(buf)
	case packets.Publish:
		err = pk.PublishEncodeV5(buf)
		if err == nil {
			atomic.AddInt64(&cl.systemInfo.PublishSent, 1)
		}
	case packets.Puback:
		err = pk.PubackEncodeV5(buf)
	case packets.Pubrec:
		err = pk.PubrecEncodeV5(buf)
	case packets.Pubrel:
		err = pk.PubrelEncodeV5(buf)
	case packets.Pubcomp:
		err = pk.PubcompEncodeV5(buf)
	case packets.Subscribe:
		err = pk.SubscribeEncodeV5(buf)
	case packets.Suback:
		err = pk.SubackEncodeV5(buf)
	case packets.Unsubscribe:
		err = pk.UnsubscribeEncodeV5(buf)
	case packets.Unsuback:
		err = pk.UnsubackEncodeV5(buf)
	case packets.Pingreq:
		err = pk.PingreqEncodeV5(buf)
	case packets.Pingresp:
		err = pk.PingrespEncodeV5(buf)
	case packets.Disconnect:
		err = pk.DisconnectEncodeV5(buf)
	default:
		err = fmt.Errorf("No valid packet available; %v", pk.FixedHeader.Type)
	}

	return
}

// SendInflight resend all inflight message
func (cl *Client) SendInflight(handler func(cl *Client, im *InflightMessage, force bool) error) {
	cl.Inflight.Walk(cl, handler)
}

// LWT contains the last will and testament details for a client connection.
type LWT struct {
	Message []byte // the message that shall be sent when the client disconnects.
	Topic   string // the topic the will message shall be sent to.
	Qos     byte   // the quality of service desired.
	Retain  bool   // indicates whether the will message should be retained
}
