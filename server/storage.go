package server

import (
	"fmt"
	"github.com/wind-c/comqtt/server/events"
	"github.com/wind-c/comqtt/server/internal/clients"
	"github.com/wind-c/comqtt/server/internal/packets"
	"github.com/wind-c/comqtt/server/persistence"
	"time"
)

// readStore reads in any data from the persistent datastore (if applicable).
func (s *Server) readStore() error {
	info, err := s.Store.ReadServerInfo()
	if err != nil {
		return fmt.Errorf("load server info; %w", err)
	}
	s.loadServerInfo(info)

	//In cluster mode, only server info is loaded
	if s.Options.RunMode == Cluster {
		return nil
	}

	clients, err := s.Store.ReadClients()
	if err != nil {
		return fmt.Errorf("load clients; %w", err)
	}
	s.loadClients(clients)

	subs, err := s.Store.ReadSubscriptions()
	if err != nil {
		return fmt.Errorf("load subscriptions; %w", err)
	}
	s.loadSubscriptions(subs)

	inflight, err := s.Store.ReadInflight()
	if err != nil {
		return fmt.Errorf("load inflight; %w", err)
	}
	s.loadInflight(inflight)

	retained, err := s.Store.ReadRetained()
	if err != nil {
		return fmt.Errorf("load retained; %w", err)
	}
	s.loadRetained(retained)

	return nil
}

// loadServerInfo restores server info from the datastore.
func (s *Server) loadServerInfo(v persistence.ServerInfo) {
	version := s.System.Version
	v.ClientsConnected = 0
	copySystemInfo(s.System, &v)
	s.System.Version = version
}

// loadSubscriptions restores subscriptions from the datastore.
func (s *Server) loadSubscriptions(v []persistence.Subscription) {
	for _, sub := range v {
		_, c := s.Topics.Subscribe(sub.Filter, sub.Client, sub.QoS)
		if cl, ok := s.Clients.Get(sub.Client); ok {
			cl.NoteSubscription(sub.Filter, packets.SubOptions{
				QoS:               sub.QoS,
				NoLocal:           sub.NoLocal,
				RetainHandling:    sub.RetainHandling,
				RetainAsPublished: sub.RetainAsPublished,
			})
			if s.Events.OnSubscribe != nil {
				s.Events.OnSubscribe(sub.Filter, cl.Info(), sub.QoS, c == 1)
			}
		}
	}
}

// loadClients restores clients from the datastore.
func (s *Server) loadClients(v []persistence.Client) {
	for _, c := range v {
		cl := clients.NewClientStub(s.System, s.Options.InflightHandling)
		cl.ID = c.ClientID
		cl.Listener = c.Listener
		cl.Username = c.Username
		cl.LWT = clients.LWT(c.LWT)
		s.Clients.Add(cl)
	}
}

// loadInflight restores inflight messages from the datastore.
func (s *Server) loadInflight(v []persistence.Message) {
	for _, msg := range v {
		if client, ok := s.Clients.Get(msg.Client); ok {
			client.Inflight.Set(msg.PacketID, &clients.InflightMessage{
				Packet: packets.Packet{
					FixedHeader: packets.FixedHeader(msg.FixedHeader),
					PacketID:    msg.PacketID,
					TopicName:   msg.TopicName,
					Payload:     msg.Payload,
				},
				Sent:    msg.Sent,
				Resends: msg.Resends,
			})
		}
	}
}

// loadRetained restores retained messages from the datastore.
func (s *Server) loadRetained(v []persistence.Message) {
	for _, msg := range v {
		s.Topics.RetainMessage(packets.Packet{
			FixedHeader: packets.FixedHeader(msg.FixedHeader),
			TopicName:   msg.TopicName,
			Payload:     msg.Payload,
		})
	}
}

// genPersistenceInflightMessage gen persistence.Message from  clients.InflightMessage
func (s *Server) genPersistenceInflightMessage(cl *clients.Client, inf *clients.InflightMessage) persistence.Message {
	msg := fromPacketToMessage(&inf.Packet)
	msg.ID = s.Store.GenInflightId(cl.ID, inf.Packet.PacketID)
	msg.T = persistence.KInflight
	msg.Client = cl.ID
	msg.Sent = inf.Sent
	msg.Resends = inf.Resends
	return msg
}

// genPersistenceRetainedMessage gen persistence.Message from  clients.InflightMessage
func (s *Server) genPersistenceRetainedMessage(pk *packets.Packet) persistence.Message {
	msg := fromPacketToMessage(pk)
	msg.ID = s.Store.GenRetainedId(pk.TopicName)
	msg.T = persistence.KRetained
	return msg
}

// fromPacketToMessage
func fromPacketToMessage(pk *packets.Packet) persistence.Message {
	msg := persistence.Message{
		FixedHeader: persistence.FixedHeader(pk.FixedHeader),
		TopicName:   pk.TopicName,
		Payload:     pk.Payload,
		PacketID:    pk.PacketID,
	}
	if pk.Properties != nil {
		msg.Properties = persistence.Properties{
			PayloadFormat:   pk.Properties.PayloadFormat,
			ResponseTopic:   pk.Properties.ResponseTopic,
			CorrelationData: pk.Properties.CorrelationData,
			ContentType:     pk.Properties.ContentType,
		}
		if pk.Properties.MessageExpiry != nil {
			msg.Properties.Expiry = time.Now().Add(time.Duration(*pk.Properties.MessageExpiry) * time.Second).Unix()
		}
		if pk.Properties.User != nil {
			for _, pu := range pk.Properties.User {
				msg.Properties.UserProperties = append(msg.Properties.UserProperties, persistence.KeyValue{Key: pu.Key, Value: pu.Value})
			}
		}
	}

	return msg
}

// genInflightMessage
func (s *Server) genInflightMessage(msg *persistence.Message) *clients.InflightMessage {
	pk := fromMessageToPacket(msg)
	inf := clients.InflightMessage{
		Packet:  *pk,
		Sent:    msg.Sent,
		Resends: msg.Resends,
	}
	if &msg.Properties != nil {
		inf.Expiry = msg.Properties.Expiry
	}

	return &inf
}

// fromMessageToPacket
func fromMessageToPacket(msg *persistence.Message) *packets.Packet {
	pk := &packets.Packet{
		FixedHeader: packets.FixedHeader(msg.FixedHeader),
		PacketID:    msg.PacketID,
		TopicName:   msg.TopicName,
		Payload:     msg.Payload,
	}
	if &msg.Properties != nil {
		pk.Properties = &packets.Properties{
			ResponseTopic:   msg.Properties.ResponseTopic,
			ContentType:     msg.Properties.ContentType,
			PayloadFormat:   msg.Properties.PayloadFormat,
			CorrelationData: msg.Properties.CorrelationData,
		}
		if msg.Properties.UserProperties != nil {
			for _, up := range msg.Properties.UserProperties {
				pk.Properties.User = append(pk.Properties.User, packets.User{Key: up.Key, Value: up.Value})
			}
		}
	}

	return pk
}

// storageWrap
type storageWrap struct {
	cl events.Client
	op int
	in interface{}
}

const (
	StorageDel = iota
	StorageAdd
)

// onStorage is a pass-through method which delegates errors from
// the persistent storage adapter to the onError event hook.
func (s *Server) onStorage(cl events.Clientlike, op int, in interface{}) {
	if op != StorageDel && op != StorageAdd {
		return
	}
	//sw := storageWrap{cl: cl.Info(), op: op, in: in}
	//s.spool.Invoke(sw)

	var err error
	if op == 1 {
		err = s.saveStorageEntity(in)
	} else {
		err = s.delStorageEntity(in)
	}
	if err == nil {
		return
	}
	_ = s.onError(cl.Info(), fmt.Errorf("storage: %w", err))
}

func (s *Server) saveStorageEntity(in interface{}) error {
	var err error
	switch entity := in.(type) {
	case persistence.Client:
		err = s.Store.WriteClient(entity)
	case persistence.Subscription:
		err = s.Store.WriteSubscription(entity)
	case persistence.Message:
		if entity.T == persistence.KInflight {
			err = s.Store.WriteInflight(entity)
		}
		if entity.T == persistence.KRetained {
			err = s.Store.WriteRetained(entity)
		}
	case persistence.ServerInfo:
		err = s.Store.WriteServerInfo(entity)
	}
	return err
}

func (s *Server) delStorageEntity(in interface{}) error {
	var err error
	switch entity := in.(type) {
	case persistence.Client:
		err = s.Store.DeleteClient(entity.ClientID)
	case persistence.Subscription:
		err = s.Store.DeleteSubscription(entity.Client, entity.Filter)
	case persistence.Message:
		if entity.T == persistence.KInflight {
			err = s.Store.DeleteInflight(entity.Client, entity.PacketID)
		}
		if entity.T == persistence.KRetained {
			err = s.Store.DeleteRetained(entity.TopicName)
		}
	}
	return err
}

// DeleteExpired the clean expired messages
func (s *Server) deleteExpired() {
	for _, c := range s.Clients.GetAll() {
		var pids []uint16
		for k, m := range c.Inflight.GetAll() {
			if m.Expiry > 0 && time.Now().Unix() > m.Expiry {
				pids = append(pids, k)
			}
		}
		for _, k := range pids {
			c.Inflight.Delete(k)
		}
		if s.Store != nil {
			s.Store.DeleteInflightBatch(c.ID, pids)
		}
	}
}

type cleaner struct {
	interval time.Duration
	stop     chan struct{}
}

func (s *Server) runCleaner() {
	go func() {
		ticker := time.NewTicker(s.cleaner.interval)
		for {
			select {
			case <-ticker.C:
				s.deleteExpired()
			case <-s.cleaner.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *Server) stopCleaner() {
	s.cleaner.stop <- struct{}{}
}
