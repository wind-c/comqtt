// packet server provides a MQTT 3.1.1 & 5.0 compliant MQTT server.
package server

import (
	"errors"
	"fmt"
	"github.com/panjf2000/ants/v2"
	"io"
	"net"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/wind-c/comqtt/server/events"
	"github.com/wind-c/comqtt/server/internal/circ"
	"github.com/wind-c/comqtt/server/internal/clients"
	"github.com/wind-c/comqtt/server/internal/packets"
	"github.com/wind-c/comqtt/server/internal/topics"
	"github.com/wind-c/comqtt/server/internal/utils"
	"github.com/wind-c/comqtt/server/listeners"
	"github.com/wind-c/comqtt/server/listeners/auth"
	"github.com/wind-c/comqtt/server/persistence"
	"github.com/wind-c/comqtt/server/system"
)

const (
	// Version indicates the current server version.
	Version = "1.2.0"

	// Single The running environment is single node or cluster mode
	Single uint = iota
	Cluster
)

var (
	// ErrListenerIDExists indicates that a listener with the same id already exists.
	ErrListenerIDExists = errors.New("listener id already exists")

	// ErrReadConnectInvalid indicates that the connection packet was invalid.
	ErrReadConnectInvalid = errors.New("connect packet was not valid")

	// ErrConnectNotAuthorized indicates that the connection packet had incorrect
	// authentication parameters.
	ErrConnectNotAuthorized = errors.New("connect packet was not authorized")

	// ErrInvalidTopic indicates that the specified topic was not valid.
	ErrInvalidTopic = errors.New("cannot publish to $ and $SYS topics")

	// ErrRejectPacket indicates that a packet should be dropped instead of processed.
	ErrRejectPacket = errors.New("packet rejected")

	ErrClientDisconnect     = errors.New("Client disconnected")
	ErrClientReconnect      = errors.New("Client attemped to reconnect")
	ErrServerShutdown       = errors.New("Server is shutting down")
	ErrSessionReestablished = errors.New("Session reestablished")
	ErrConnectionFailed     = errors.New("Connection attempt failed")

	// SysTopicInterval is the number of milliseconds between $SYS topic publishes.
	SysTopicInterval time.Duration = 30000

	// inflightResendBackoff is a slice of seconds, which determines the
	// interval between inflight resend attempts.
	inflightResendBackoff = []int64{0, 1, 2, 10, 60, 120, 600, 3600, 21600}

	// inflightMaxResends is the maximum number of times to try resending QoS promises.
	inflightMaxResends = 6

	// defaultReceiveMaximum is the maximum number of QOS1 & 2 messages allowed to be "inflight"
	defaultReceiveMaximum = 256
)

// Server is an MQTT broker server. It should be created with server.New()
// in order to ensure all the internal fields are correctly populated.
type Server struct {
	inline    inlineMessages       // channels for direct publishing.
	Events    events.Events        // overrideable event hooks.
	Store     persistence.Store    // a persistent storage backend if desired.
	Options   *Options             // configurable server options.
	Listeners *listeners.Listeners // listeners are network interfaces which listen for new connections.
	Clients   *clients.Clients     // clients which are known to the broker.
	Topics    *topics.Index        // an index of topic filter subscriptions and retained messages.
	System    *system.Info         // values about the server commonly found in $SYS topics.
	bytepool  *circ.BytesPool      // a byte pool for incoming and outgoing packets.
	sysTicker *time.Ticker         // the interval ticker for sending updating $SYS topics.
	done      chan bool            // indicate that the server is ending.
	cleaner   *cleaner             // clean expired messages
	spool     *ants.PoolWithFunc   // storage goroutine pool
}

// Options contains configurable options for the server.
type Options struct {
	// RunMode program running mode，1 single or 2 cluster
	RunMode uint

	// BufferSize overrides the default buffer size (circ.DefaultBufferSize) for the client buffers.
	BufferSize int

	// BufferBlockSize overrides the default buffer block size (DefaultBlockSize) for the client buffers.
	BufferBlockSize int

	// ReceiveMaximum is the maximum number of QOS1 & 2 messages allowed to be "inflight"
	ReceiveMaximum int

	// InflightHandling is the handling mode of inflight message when the receive-maximum is exceeded, 0 closes the connection or 1 overwrites the old inflight message
	InflightHandling int
}

// inlineMessages contains channels for handling inline (direct) publishing.
type inlineMessages struct {
	done chan bool           // indicate that the server is ending.
	pub  chan packets.Packet // a channel of packets to publish to clients
}

// Info provides pseudo-client information for the inline messages processor.
// It provides a 'client' to which inline retained messages can be assigned.
func (*inlineMessages) Info() events.Client {
	return events.Client{
		ID:       "inline",
		Remote:   "inline",
		Listener: "inline",
	}
}

// New returns a new instance of MQTT server with no options.
// This method has been deprecated and will be removed in a future release.
// Please use NewServer instead.
func New() *Server {
	return NewServer(nil)
}

// NewServer returns a new instance of an MQTT broker with optional values where applicable.
func NewServer(opts *Options) *Server {
	if opts == nil {
		opts = new(Options)
	}
	if opts.ReceiveMaximum == 0 {
		opts.ReceiveMaximum = defaultReceiveMaximum
	}

	s := &Server{
		done:     make(chan bool),
		bytepool: circ.NewBytesPool(opts.BufferSize),
		Clients:  clients.New(),
		Topics:   topics.New(),
		System: &system.Info{
			Version: Version,
			Started: time.Now().Unix(),
		},
		sysTicker: time.NewTicker(SysTopicInterval * time.Millisecond),
		inline: inlineMessages{
			done: make(chan bool),
			pub:  make(chan packets.Packet, 4096),
		},
		Events:  events.Events{},
		Options: opts,
		cleaner: &cleaner{
			interval: 60 * time.Second,
			stop:     make(chan struct{}),
		},
	}

	// Expose server stats using the system listener so it can be used in the
	// dashboard and other more experimental listeners.
	s.Listeners = listeners.New(s.System)

	//create storage goroutine pool
	gps := runtime.GOMAXPROCS(0)
	spool, _ := ants.NewPoolWithFunc(gps, func(i interface{}) {
		if sw, ok := i.(storageWrap); !ok {
			return
		} else {
			var err error
			if sw.op == 1 {
				err = s.saveStorageEntity(sw.in)
			} else {
				err = s.delStorageEntity(sw.in)
			}
			if err == nil {
				return
			}
			_ = s.onError(sw.cl, fmt.Errorf("storage: %w", err))
		}
	})
	s.spool = spool

	return s
}

// AddStore assigns a persistent storage backend to the server. This must be
// called before calling server.Server().
func (s *Server) AddStore(p persistence.Store) error {
	s.Store = p
	err := s.Store.Open()
	if err != nil {
		return err
	}

	return nil
}

// AddListener adds a new network listener to the server.
func (s *Server) AddListener(listener listeners.Listener, config *listeners.Config) error {
	if _, ok := s.Listeners.Get(listener.ID()); ok {
		return ErrListenerIDExists
	}

	if config != nil {
		listener.SetConfig(config)
	}

	s.Listeners.Add(listener)
	err := listener.Listen(s.System)
	if err != nil {
		return err
	}

	return nil
}

// Serve starts the event loops responsible for establishing client connections
// on all attached listeners, and publishing the system topics.
func (s *Server) Serve() error {
	if s.Store != nil {
		err := s.readStore()
		if err != nil {
			return err
		}
	}

	go s.eventLoop()                            // spin up event loop for issuing $SYS values and closing server.
	go s.inlineClient()                         // spin up inline client for direct message publishing.
	s.Listeners.ServeAll(s.EstablishConnection) // start listening on all listeners.
	s.publishSysTopics()                        // begin publishing $SYS system values.
	s.runCleaner()                              // begin clean expired messages

	return nil
}

// eventLoop loops forever, running various server processes at different intervals.
func (s *Server) eventLoop() {
	for {
		select {
		case <-s.done:
			s.sysTicker.Stop()
			close(s.inline.done)
			return
		case <-s.sysTicker.C:
			s.publishSysTopics()
		}
	}
}

// inlineClient loops forever, sending directly-published messages
// from the Publish method to subscribers.
func (s *Server) inlineClient() {
	for {
		select {
		case <-s.inline.done:
			close(s.inline.pub)
			return
		case pk := <-s.inline.pub:
			s.publishToSubscribers(pk)
		}
	}
}

// onError is a pass-through method which triggers the OnError
// event hook (if applicable), and returns the provided error.
func (s *Server) onError(cl events.Client, err error) error {
	if err == nil {
		return err
	}
	// Note: if the error originates from a real cause, it will
	// have been captured as the StopCause. The two cases ignored
	// below are ordinary consequences of closing the connection.
	// If one of these ordinary conditions stops the connection,
	// then the client closed or broke the connection.
	if s.Events.OnError != nil &&
		!errors.Is(err, io.EOF) {
		s.Events.OnError(cl, err)
	}

	return err
}

// EstablishConnection establishes a new client when a listener
// accepts a new connection.
func (s *Server) EstablishConnection(lid string, c net.Conn, ac auth.Controller) error {
	xbr := s.bytepool.Get() // Get byte buffer from pools for receiving packet data.
	xbw := s.bytepool.Get() // and for sending.
	defer s.bytepool.Put(xbr)
	defer s.bytepool.Put(xbw)

	cl := clients.NewClient(c,
		circ.NewReaderFromSlice(s.Options.BufferBlockSize, xbr),
		circ.NewWriterFromSlice(s.Options.BufferBlockSize, xbw),
		s.System,
		s.Options.ReceiveMaximum,
	)
	cl.Start()
	defer cl.ClearBuffers()
	defer cl.Stop(nil)

	pk, err := s.readConnectionPacket(cl)
	if err != nil {
		return s.onError(cl.Info(), fmt.Errorf("read connection: %w", err))
	}

	cl.Identify(lid, pk, ac)
	ci := cl.Info()

	ackCode, err := pk.ConnectValidate()
	if err != nil {
		if err := s.ackConnection(cl, ackCode, false); err != nil {
			return s.onError(ci, fmt.Errorf("invalid connection send ack: %w", err))
		}
		return s.onError(ci, fmt.Errorf("validate connection packet: %w", err))
	}

	if !ac.Authenticate(pk.Username, pk.Password) {
		if err := s.ackConnection(cl, packets.CodeConnectBadAuthValues, false); err != nil {
			return s.onError(ci, fmt.Errorf("invalid connection send ack: %w", err))
		}
		return s.onError(ci, ErrConnectionFailed)
	}

	atomic.AddInt64(&s.System.ConnectionsTotal, 1)
	atomic.AddInt64(&s.System.ClientsConnected, 1)
	defer atomic.AddInt64(&s.System.ClientsConnected, -1)
	defer atomic.AddInt64(&s.System.ClientsDisconnected, 1)

	sessionPresent, firstConnect := s.inheritClientSession(pk, cl)
	ci.First = firstConnect
	s.Clients.Add(cl)

	err = s.ackConnection(cl, ackCode, sessionPresent)
	if err != nil {
		return s.onError(cl.Info(), fmt.Errorf("ack connection packet: %w", err))
	}

	// resend inflight
	cl.SendInflight(s.resendClientInflight)

	// CleanSession is false to store
	if !cl.CleanSession && s.Store != nil {
		s.onStorage(cl, StorageAdd, persistence.Client{
			ID:       "cl_" + cl.ID,
			ClientID: cl.ID,
			T:        persistence.KClient,
			Listener: cl.Listener,
			Username: cl.Username,
			LWT:      persistence.LWT(cl.LWT),
		})
	}

	// hook connect
	if s.Events.OnConnect != nil {
		s.Events.OnConnect(ci, events.Packet(pk))
	}

	if err := cl.Read(s.processPacket); err != nil {
		s.closeClient(cl, true, err)
		s.onError(cl.Info(), err)
	}

	err = cl.StopCause() // Determine true cause of stop.
	if s.Events.OnDisconnect != nil {
		s.Events.OnDisconnect(ci, err)
	}

	return err
}

// readConnectionPacket reads the first incoming header for a connection, and if
// acceptable, returns the valid connection packet.
func (s *Server) readConnectionPacket(cl *clients.Client) (packets.Packet, error) {
	fh := new(packets.FixedHeader)
	if err := cl.ReadFixedHeader(fh); err != nil {
		return packets.Packet{}, err
	}

	pk, err := cl.ReadConnectPacket(fh)
	if err != nil {
		return pk, err
	}

	if pk.FixedHeader.Type != packets.Connect {
		return pk, ErrReadConnectInvalid
	}

	return pk, nil
}

// ackConnection returns a Connack packet to a client.
func (s *Server) ackConnection(cl *clients.Client, ack byte, present bool) error {
	return s.writeClient(cl, packets.Packet{
		FixedHeader: packets.FixedHeader{
			Type: packets.Connack,
		},
		SessionPresent: present,
		ReturnCode:     ack,
	})
}

// inheritClientSession inherits the state of an existing client sharing the same
// connection ID. If cleanSession is true, the state of any previously existing client
// session is abandoned.
func (s *Server) inheritClientSession(pk packets.Packet, cl *clients.Client) (sessionPresent, firstConnect bool) {
	if existing, ok := s.Clients.Get(pk.ClientIdentifier); ok {
		existing.Lock()
		defer existing.Unlock()

		existing.Stop(ErrSessionReestablished) // Issue a stop on the old client.

		// Per [MQTT-3.1.2-6]:
		// If CleanSession is set to 1, the Client and Server MUST discard any previous Session and start a new one.
		// The state associated with a CleanSession MUST NOT be reused in any subsequent session.
		if pk.CleanSession || existing.CleanSession {
			s.unsubscribeClient(existing)
			// clean session (client info and subscriptions and inflights)
			if !existing.CleanSession && s.Store != nil {
				s.onStorage(cl, StorageDel, persistence.Client{ClientID: existing.ID})
			}
			return false, false
		}

		cl.Inflight = existing.Inflight // Take address of existing session.
		cl.Subscriptions = existing.Subscriptions
		return true, false
	} else {
		atomic.AddInt64(&s.System.ClientsTotal, 1)
		if atomic.LoadInt64(&s.System.ClientsConnected) > atomic.LoadInt64(&s.System.ClientsMax) {
			atomic.AddInt64(&s.System.ClientsMax, 1)
		}
		// cluster mode and first connected to local node
		if s.Options.RunMode == Cluster && !pk.CleanSession {
			s.loadClientHistory(cl)
			if len(cl.Subscriptions) > 0 {
				return true, true
			}
		}
		return false, true
	}
}

// unsubscribeClient unsubscribes a client from all of their subscriptions.
func (s *Server) unsubscribeClient(cl *clients.Client) {
	ci := cl.Info()
	for k := range cl.Subscriptions {
		delete(cl.Subscriptions, k)
		if ok, c := s.Topics.Unsubscribe(k, cl.ID); ok {
			atomic.AddInt64(&s.System.Subscriptions, -1)
			if s.Events.OnUnsubscribe != nil {
				s.Events.OnUnsubscribe(k, ci, c == 0)
			}
		}
	}
}

// LoadClientHistory loads its history info for the connected client,
func (s *Server) loadClientHistory(cl *clients.Client) {
	if s.Store == nil {
		return
	}
	ss, err := s.Store.ReadSubscriptionsByCid(cl.ID)
	if err != nil {
		return
	}
	ci := cl.Info()
	for _, sub := range ss {
		q, c := s.Topics.Subscribe(sub.Filter, cl.ID, sub.QoS)
		if q {
			atomic.AddInt64(&s.System.Subscriptions, 1)
		}
		cl.NoteSubscription(sub.Filter, packets.SubOptions{
			QoS:               sub.QoS,
			NoLocal:           sub.NoLocal,
			RetainHandling:    sub.RetainHandling,
			RetainAsPublished: sub.RetainAsPublished,
		})

		if s.Events.OnSubscribe != nil {
			s.Events.OnSubscribe(sub.Filter, ci, sub.QoS, c == 1)
		}
	}

	fs, err := s.Store.ReadInflightByCid(cl.ID)
	if err != nil {
		return
	}
	for _, msg := range fs {
		cl.Inflight.Set(msg.PacketID, s.genInflightMessage(&msg))
	}
}

// ResendClientInflight attempts to resend all undelivered inflight messages
// to a client.
func (s *Server) resendClientInflight(cl *clients.Client, im *clients.InflightMessage, force bool) error {
	nt := time.Now().Unix()
	if (im.Expiry > 0 && nt > im.Expiry) || im.Resends >= inflightMaxResends { // After a reasonable time, drop inflight packets.
		cl.Inflight.Delete(im.Packet.PacketID)
		if im.Packet.FixedHeader.Type == packets.Publish {
			atomic.AddInt64(&s.System.PublishDropped, 1)
		}

		if !cl.CleanSession && s.Store != nil {
			s.onStorage(cl, StorageDel, persistence.Message{Client: cl.ID, PacketID: im.Packet.PacketID, T: persistence.KInflight})
		}

		return nil
	}

	// Only continue if the resend backoff time has passed and there's a backoff time.
	if !force && (nt-im.Sent < inflightResendBackoff[im.Resends] || len(inflightResendBackoff) < im.Resends) {
		return nil
	}

	if im.Packet.FixedHeader.Type == packets.Publish {
		im.Packet.FixedHeader.Dup = true
	}

	im.Resends++
	im.Sent = nt
	cl.Inflight.Set(im.Packet.PacketID, im)
	_, err := cl.WritePacket(im.Packet)
	if err != nil {
		err = fmt.Errorf("resend in flight: %w ", err)
		s.onError(cl.Info(), err)
		return err
	}

	if !cl.CleanSession && s.Store != nil {
		s.onStorage(cl, StorageAdd, s.genPersistenceInflightMessage(cl, im))
	}

	return nil
}

// writeClient writes packets to a client connection.
func (s *Server) writeClient(cl *clients.Client, pk packets.Packet) error {
	_, err := cl.WritePacket(pk)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// Publish creates a publish packet from a payload and sends it to the inline.pub
// channel, where it is written directly to the outgoing byte buffers of any
// clients subscribed to the given topic. Because the message is written directly
// within the server, QoS is inherently 2 (exactly once).
func (s *Server) Publish(topic string, payload []byte, retain bool) error {
	if len(topic) >= 4 && topic[0:4] == "$SYS" {
		return ErrInvalidTopic
	}

	pk := packets.Packet{
		FixedHeader: packets.FixedHeader{
			Type:   packets.Publish,
			Retain: retain,
		},
		TopicName: topic,
		Payload:   payload,
	}

	if retain {
		s.retainMessage(&s.inline, &pk)
	}

	// handoff packet to s.inline.pub channel for writing to client buffers
	// to avoid blocking the calling function.
	s.inline.pub <- pk

	return nil
}

// retainMessage adds a message to a topic, and if a persistent store is provided,
// adds the message to the store so it can be reloaded if necessary.
func (s *Server) retainMessage(cl events.Clientlike, pk *packets.Packet) {
	//out := pk.PublishCopy()
	r := s.Topics.RetainMessage(*pk)
	atomic.AddInt64(&s.System.Retained, r)

	if s.Store != nil {
		if r == 1 {
			s.onStorage(cl, StorageAdd, s.genPersistenceRetainedMessage(pk))
		} else {
			s.onStorage(cl, StorageDel, persistence.Message{TopicName: pk.TopicName, T: persistence.KRetained})
		}
	}
}

// publishToSubscribers publishes a publish packet to all subscribers with
// matching topic filters.
func (s *Server) publishToSubscribers(pk packets.Packet) {
	for id, qos := range s.Topics.Subscribers(pk.TopicName) {
		if client, ok := s.Clients.Get(id); ok {
			if client.ID == pk.ClientIdentifier {
				// If the NoLocal is true, do not receive messages sent by self; for v5
				if sops, ok := client.Subscriptions[pk.TopicName]; ok && sops.NoLocal {
					continue
				}
			}

			// If the AllowClients value is set, only deliver the packet if the subscribed
			// client exists in the AllowClients value. For use with the OnMessage event hook
			// in cases where you want to publish messages to clients selectively.
			if pk.AllowClients != nil && !utils.InSliceString(pk.AllowClients, id) {
				continue
			}

			//out := pk.PublishCopy()
			if qos > pk.FixedHeader.Qos { // Inherit higher desired qos values.
				pk.FixedHeader.Qos = qos
			}

			if pk.FixedHeader.Qos > 0 { // If QoS required, save to inflight index.
				//If the quota is exceeded, the connection is disconnected
				if s.Options.InflightHandling == 0 && client.Inflight.Len()+1 > s.Options.ReceiveMaximum {
					s.onError(client.Info(), s.writeClient(client, packets.Packet{
						FixedHeader: packets.FixedHeader{
							Type: packets.Disconnect,
						},
						PacketID:    pk.PacketID,
						ReturnCodes: []byte{byte(packets.ReceiveMaximumExceeded)},
					}))

					s.closeClient(client, true, errors.New(packets.ReceiveMaximumExceeded.String()))

					continue
				}

				if pk.PacketID == 0 {
					pk.PacketID = uint16(client.NextPacketID())
				}

				// If a message has a QoS, we need to ensure it is delivered to
				// the client at some point, one way or another. Store the publish
				// packet in the client's inflight queue and attempt to redeliver
				// if an appropriate ack is not received (or if the client is offline).
				var expiry int64
				sent := time.Now().Unix()
				if pk.Properties != nil && pk.Properties.MessageExpiry != nil && *pk.Properties.MessageExpiry > 0 {
					expiry = time.Now().Add(time.Duration(*pk.Properties.MessageExpiry) * time.Second).Unix()
				}
				infMsg := clients.InflightMessage{
					Packet: pk,
					Sent:   sent,
					Expiry: expiry,
				}
				q := client.Inflight.Set(pk.PacketID, &infMsg)
				if q {
					atomic.AddInt64(&s.System.Inflight, 1)
				}

				if !client.CleanSession && s.Store != nil {
					s.onStorage(client, StorageAdd, s.genPersistenceInflightMessage(client, &infMsg))
				}
			}

			// Send to subscriber
			s.onError(client.Info(), s.writeClient(client, pk))
		}
	}
}

// atomicItoa reads an *int64 and formats a decimal string.
func atomicItoa(ptr *int64) string {
	return strconv.FormatInt(atomic.LoadInt64(ptr), 10)
}

// publishSysTopics publishes the current values to the server $SYS topics.
// Due to the int to string conversions this method is not as cheap as
// some of the others so the publishing interval should be set appropriately.
func (s *Server) publishSysTopics() {
	pk := packets.Packet{
		FixedHeader: packets.FixedHeader{
			Type:   packets.Publish,
			Retain: true,
		},
	}

	uptime := time.Now().Unix() - atomic.LoadInt64(&s.System.Started)
	atomic.StoreInt64(&s.System.Uptime, uptime)
	topics := map[string]string{
		"$SYS/broker/version":                   s.System.Version,
		"$SYS/broker/uptime":                    atomicItoa(&s.System.Uptime),
		"$SYS/broker/timestamp":                 atomicItoa(&s.System.Started),
		"$SYS/broker/load/bytes/received":       atomicItoa(&s.System.BytesRecv),
		"$SYS/broker/load/bytes/sent":           atomicItoa(&s.System.BytesSent),
		"$SYS/broker/clients/connected":         atomicItoa(&s.System.ClientsConnected),
		"$SYS/broker/clients/disconnected":      atomicItoa(&s.System.ClientsDisconnected),
		"$SYS/broker/clients/maximum":           atomicItoa(&s.System.ClientsMax),
		"$SYS/broker/clients/total":             atomicItoa(&s.System.ClientsTotal),
		"$SYS/broker/connections/total":         atomicItoa(&s.System.ConnectionsTotal),
		"$SYS/broker/messages/received":         atomicItoa(&s.System.MessagesRecv),
		"$SYS/broker/messages/sent":             atomicItoa(&s.System.MessagesSent),
		"$SYS/broker/messages/publish/dropped":  atomicItoa(&s.System.PublishDropped),
		"$SYS/broker/messages/publish/received": atomicItoa(&s.System.PublishRecv),
		"$SYS/broker/messages/publish/sent":     atomicItoa(&s.System.PublishSent),
		"$SYS/broker/messages/retained/count":   atomicItoa(&s.System.Retained),
		"$SYS/broker/messages/inflight":         atomicItoa(&s.System.Inflight),
		"$SYS/broker/subscriptions/count":       atomicItoa(&s.System.Subscriptions),
	}

	for topic, payload := range topics {
		pk.TopicName = topic
		pk.Payload = []byte(payload)
		q := s.Topics.RetainMessage(pk.PublishCopy())
		atomic.AddInt64(&s.System.Retained, q)
		s.publishToSubscribers(pk)
	}

	if s.Store != nil {
		s.onStorage(&s.inline, StorageAdd, persistence.ServerInfo{
			Info: *s.System,
			ID:   persistence.KServerInfo,
		})
	}
}

// Close attempts to gracefully shutdown the server, all listeners, clients, and stores.
func (s *Server) Close() error {
	close(s.done)
	s.Listeners.CloseAll(s.closeListenerClients)
	s.stopCleaner()

	if s.Store != nil {
		s.Store.Close()
	}

	return nil
}

// closeListenerClients closes all clients on the specified listener.
func (s *Server) closeListenerClients(listener string) {
	clients := s.Clients.GetByListener(listener)
	for _, cl := range clients {
		cl.Stop(ErrServerShutdown)
	}
}

// sendLWT issues an LWT message to a topic when a client disconnects.
func (s *Server) sendLWT(cl *clients.Client) error {
	if cl.LWT.Topic != "" {
		err := s.processPublish(cl, packets.Packet{
			FixedHeader: packets.FixedHeader{
				Type:   packets.Publish,
				Retain: cl.LWT.Retain,
				Qos:    cl.LWT.Qos,
			},
			TopicName: cl.LWT.Topic,
			Payload:   cl.LWT.Message,
		})
		if err != nil {
			return s.onError(cl.Info(), fmt.Errorf("send lwt: %s %w; %+v", cl.ID, err, cl.LWT))
		}
	}

	return nil
}

// closeClient closes a client connection and publishes any LWT messages.
func (s *Server) closeClient(cl *clients.Client, isSendLWT bool, cause error) {
	if isSendLWT {
		s.sendLWT(cl)
	}

	cl.Stop(cause)
	if cl.CleanSession {
		s.CleanSession(cl)
	}
}

// CleanSession
func (s *Server) CleanSession(cl *clients.Client) {
	s.cleanSubscription(cl)
	s.Clients.Delete(cl.ID)
	atomic.AddInt64(&s.System.Inflight, -int64(cl.Inflight.Len()))
}

// CleanSubscription
func (s *Server) cleanSubscription(cl *clients.Client) {
	ci := cl.Info()
	for filter := range cl.Subscriptions {
		q, c := s.Topics.Unsubscribe(filter, cl.ID)
		if q {
			atomic.AddInt64(&s.System.Subscriptions, -1)
			if s.Events.OnUnsubscribe != nil {
				s.Events.OnUnsubscribe(filter, ci, c == 0)
			}
		}
		cl.ForgetSubscription(filter)
	}
}

// copySystemInfo
func copySystemInfo(dst *system.Info, src *persistence.ServerInfo) {
	dst.Version = src.Version
	dst.Uptime = src.Uptime
	dst.Started = src.Started
	dst.ConnectionsTotal = src.ConnectionsTotal
	dst.ClientsConnected = src.ClientsConnected
	dst.ClientsDisconnected = src.ClientsDisconnected
	dst.ClientsTotal = src.ClientsTotal
	dst.ClientsMax = src.ClientsMax
	dst.BytesSent = src.BytesSent
	dst.BytesRecv = src.BytesRecv
	dst.MessagesSent = src.MessagesSent
	dst.MessagesRecv = src.MessagesRecv
	dst.PublishSent = src.PublishSent
	dst.PublishRecv = src.PublishRecv
	dst.PublishDropped = src.PublishDropped
	dst.Inflight = src.Inflight
	dst.Retained = src.Retained
	dst.Subscriptions = src.Subscriptions
}
