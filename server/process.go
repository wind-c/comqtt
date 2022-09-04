package server

import (
	"fmt"
	"github.com/wind-c/comqtt/server/events"
	"github.com/wind-c/comqtt/server/internal/clients"
	"github.com/wind-c/comqtt/server/internal/packets"
	"github.com/wind-c/comqtt/server/persistence"
	"sync/atomic"
	"time"
)

// processPacket processes an inbound packet for a client. Since the method is
// typically called as a goroutine, errors are primarily for test checking purposes.
func (s *Server) processPacket(cl *clients.Client, pk packets.Packet) error {
	switch pk.FixedHeader.Type {
	case packets.Connect:
		return s.processConnect(cl, pk)
	case packets.Disconnect:
		return s.processDisconnect(cl, pk)
	case packets.Pingreq:
		return s.processPingreq(cl, pk)
	case packets.Publish:
		r, err := pk.PublishValidate()
		if r != packets.Accepted {
			return err
		}
		return s.processPublish(cl, pk)
	case packets.Puback:
		return s.processPuback(cl, pk)
	case packets.Pubrec:
		return s.processPubrec(cl, pk)
	case packets.Pubrel:
		return s.processPubrel(cl, pk)
	case packets.Pubcomp:
		return s.processPubcomp(cl, pk)
	case packets.Subscribe:
		r, err := pk.SubscribeValidate()
		if r != packets.Accepted {
			return err
		}
		return s.processSubscribe(cl, pk)
	case packets.Unsubscribe:
		r, err := pk.UnsubscribeValidate()
		if r != packets.Accepted {
			return err
		}
		return s.processUnsubscribe(cl, pk)
	default:
		return fmt.Errorf("No valid packet available; %v", pk.FixedHeader.Type)
	}
}

// processConnect processes a Connect packet. The packet cannot be used to
// establish a new connection on an existing connection. See EstablishConnection
// instead.
func (s *Server) processConnect(cl *clients.Client, pk packets.Packet) error {
	s.closeClient(cl, true, ErrClientReconnect)
	return nil
}

// processDisconnect processes a Disconnect packet.
func (s *Server) processDisconnect(cl *clients.Client, pk packets.Packet) error {
	s.closeClient(cl, false, ErrClientDisconnect)
	return nil
}

// processPingreq processes a Pingreq packet.
func (s *Server) processPingreq(cl *clients.Client, pk packets.Packet) error {
	err := s.writeClient(cl, packets.Packet{
		FixedHeader: packets.FixedHeader{
			Type: packets.Pingresp,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// processPublish processes a Publish packet.
func (s *Server) processPublish(cl *clients.Client, pk packets.Packet) error {
	// for v5, topic alias
	if pk.Properties != nil && pk.Properties.TopicAlias != nil && *pk.Properties.TopicAlias != 0 {
		if pk.TopicName == "" {
			pk.TopicName = cl.TopicAlias[*pk.Properties.TopicAlias]
		} else {
			cl.TopicAlias[*pk.Properties.TopicAlias] = pk.TopicName
		}
	}

	if len(pk.TopicName) == 0 || (len(pk.TopicName) >= 4 && pk.TopicName[0:4] == "$SYS") {
		return nil // Clients can't publish to $SYS topics, so fail silently as per spec.
	}

	if !cl.AC.ACL(cl.Username, pk.TopicName, true) {
		return nil
	}

	ci := cl.Info()
	// if an OnProcessMessage hook exists, potentially modify the packet.
	if s.Events.OnProcessMessage != nil {
		pkx, err := s.Events.OnProcessMessage(ci, events.Packet(pk))
		if err == nil {
			pk = packets.Packet(pkx) // Only use the new package changes if there's no errors.
		} else {
			// If the ErrRejectPacket is return, abandon processing the packet.
			if err == ErrRejectPacket {
				return nil
			}

			if s.Events.OnError != nil {
				s.Events.OnError(ci, err)
			}
		}
	}

	if pk.FixedHeader.Qos > 0 {
		ack := packets.Packet{
			FixedHeader: packets.FixedHeader{
				Type: packets.Puback,
			},
			PacketID: pk.PacketID,
		}

		if pk.FixedHeader.Qos == 2 {
			ack.FixedHeader.Type = packets.Pubrec
		}

		// omit errors in case of broken connection / LWT publish. ack send failures
		// will be handled by in-flight resending on next reconnect.
		s.onError(ci, s.writeClient(cl, ack))
	}

	// Retain Message
	if pk.FixedHeader.Retain {
		s.retainMessage(cl, &pk)
	}

	// if an OnMessage hook exists, potentially modify the packet.
	if s.Events.OnMessage != nil {
		if pkx, err := s.Events.OnMessage(ci, events.Packet(pk)); err == nil {
			pk = packets.Packet(pkx)
		}
	}

	// write packet to the byte buffers of any clients with matching topic filters.
	s.publishToSubscribers(pk)

	// for v5 Request/Response
	if pk.Properties != nil && pk.Properties.ResponseTopic != "" {
		sp := packets.Packet{
			FixedHeader: packets.FixedHeader{
				Type: packets.Subscribe,
			},
			Topics:     []string{pk.Properties.ResponseTopic},
			SubOss:     []packets.SubOptions{packets.SubOptions{QoS: pk.FixedHeader.Qos, NoLocal: true}},
			Properties: &packets.Properties{ResponseTopic: pk.Properties.ResponseTopic},
		}
		s.processSubscribe(cl, sp)
	}

	return nil
}

// processPuback processes a Puback packet.
func (s *Server) processPuback(cl *clients.Client, pk packets.Packet) error {
	q := cl.Inflight.Delete(pk.PacketID)
	if q {
		atomic.AddInt64(&s.System.Inflight, -1)
	}
	if !cl.CleanSession && s.Store != nil {
		s.onStorage(cl, StorageDel, persistence.Message{Client: cl.ID, PacketID: pk.PacketID, T: persistence.KInflight})
	}
	return nil
}

// processPubrec processes a Pubrec packet.
func (s *Server) processPubrec(cl *clients.Client, pk packets.Packet) error {
	out := packets.Packet{
		FixedHeader: packets.FixedHeader{
			Type: packets.Pubrel,
			Qos:  1,
		},
		PacketID: pk.PacketID,
	}

	err := s.writeClient(cl, out)
	if err != nil {
		return err
	}

	return nil
}

// processPubrel processes a Pubrel packet.
func (s *Server) processPubrel(cl *clients.Client, pk packets.Packet) error {
	out := packets.Packet{
		FixedHeader: packets.FixedHeader{
			Type: packets.Pubcomp,
		},
		PacketID: pk.PacketID,
	}

	err := s.writeClient(cl, out)
	if err != nil {
		return err
	}
	q := cl.Inflight.Delete(pk.PacketID)
	if q {
		atomic.AddInt64(&s.System.Inflight, -1)
	}

	if !cl.CleanSession && s.Store != nil {
		s.onStorage(cl, StorageDel, persistence.Message{Client: cl.ID, PacketID: pk.PacketID, T: persistence.KInflight})
	}

	return nil
}

// processPubcomp processes a Pubcomp packet.
func (s *Server) processPubcomp(cl *clients.Client, pk packets.Packet) error {
	q := cl.Inflight.Delete(pk.PacketID)
	if q {
		atomic.AddInt64(&s.System.Inflight, -1)
	}
	if !cl.CleanSession && s.Store != nil {
		s.onStorage(cl, StorageDel, persistence.Message{Client: cl.ID, PacketID: pk.PacketID, T: persistence.KInflight})
	}
	return nil
}

// processSubscribe processes a Subscribe packet.
func (s *Server) processSubscribe(cl *clients.Client, pk packets.Packet) error {
	retCodes := make([]byte, len(pk.Topics))
	ci := cl.Info()
	for i := 0; i < len(pk.Topics); i++ {
		if !cl.AC.ACL(cl.Username, pk.Topics[i], false) {
			retCodes[i] = packets.ErrSubAckNetworkError
		} else {
			q, c := s.Topics.Subscribe(pk.Topics[i], cl.ID, pk.SubOss[i].QoS)
			if q {
				atomic.AddInt64(&s.System.Subscriptions, 1)
			}
			cl.NoteSubscription(pk.Topics[i], pk.SubOss[i])
			//retCodes[i] = pk.SubOss[i].QoS
			retCodes[i] = packets.Accepted

			if s.Events.OnSubscribe != nil {
				s.Events.OnSubscribe(pk.Topics[i], ci, pk.SubOss[i].QoS, c == 1)
			}
		}
	}

	// Not from the publish response topic
	if pk.Properties == nil || pk.Properties.ResponseTopic == "" {
		err := s.writeClient(cl, packets.Packet{
			FixedHeader: packets.FixedHeader{
				Type: packets.Suback,
			},
			PacketID:    pk.PacketID,
			ReturnCodes: retCodes,
		})
		if err != nil {
			return err
		}
	}

	// Publish out any retained messages matching the subscription filter and the user has
	// been allowed to subscribe to.
	for i := 0; i < len(pk.Topics); i++ {
		if retCodes[i] == packets.ErrSubAckNetworkError {
			continue
		}
		hasRetained := false
		for _, pkv := range s.Topics.Messages(pk.Topics[i]) {
			s.onError(ci, s.writeClient(cl, pkv))
			hasRetained = true
		}
		// If local does not retained messages, read from Redis
		if !hasRetained && s.Store != nil {
			if msg, err := s.Store.ReadRetainedByTopic(pk.Topics[i]); err != nil {
				s.onError(ci, err)
			} else {
				var pe uint32
				if &msg.Properties != nil && msg.Properties.Expiry != 0 {
					pe = uint32(msg.Properties.Expiry - time.Now().Unix())
				}
				pk := fromMessageToPacket(&msg)
				if pe > 0 {
					pk.Properties.MessageExpiry = &pe
				}
				s.onError(ci, s.writeClient(cl, *pk))
			}
		}

		if !cl.CleanSession && s.Store != nil {
			s.onStorage(cl, StorageAdd, persistence.Subscription{
				ID:                s.Store.GenSubscriptionId(cl.ID, pk.Topics[i]),
				T:                 persistence.KSubscription,
				Filter:            pk.Topics[i],
				Client:            cl.ID,
				QoS:               pk.SubOss[i].QoS,
				NoLocal:           pk.SubOss[i].NoLocal,
				RetainHandling:    pk.SubOss[i].RetainHandling,
				RetainAsPublished: pk.SubOss[i].RetainAsPublished,
			})
		}
	}

	// if an OnMessage hook exists
	if s.Events.OnMessage != nil {
		pk.ReturnCodes = retCodes
		s.Events.OnMessage(ci, events.Packet(pk))
	}

	return nil
}

// processUnsubscribe processes an unsubscribe packet.
func (s *Server) processUnsubscribe(cl *clients.Client, pk packets.Packet) error {
	ci := cl.Info()
	for i := 0; i < len(pk.Topics); i++ {
		q, c := s.Topics.Unsubscribe(pk.Topics[i], cl.ID)
		if q {
			atomic.AddInt64(&s.System.Subscriptions, -1)
			if s.Events.OnUnsubscribe != nil {
				s.Events.OnUnsubscribe(pk.Topics[i], ci, c == 0)
			}
		}
		cl.ForgetSubscription(pk.Topics[i])

		if !cl.CleanSession && s.Store != nil {
			s.onStorage(cl, StorageDel, persistence.Subscription{Client: cl.ID, Filter: pk.Topics[i]})
		}
	}

	// unsuback
	err := s.writeClient(cl, packets.Packet{
		FixedHeader: packets.FixedHeader{
			Type: packets.Unsuback,
		},
		PacketID: pk.PacketID,
	})
	if err != nil {
		return err
	}

	// if an OnMessage hook exists
	if s.Events.OnMessage != nil {
		s.Events.OnMessage(ci, events.Packet(pk))
	}

	return nil
}
