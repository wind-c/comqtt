// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package redis

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	redis "github.com/go-redis/redis/v8"
	"github.com/wind-c/comqtt/v2/cluster/utils"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/storage"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/mqtt/system"
)

// defaultAddr is the default address to the redis service.
const defaultAddr = "localhost:6379"

// defaultHPrefix is a prefix to better identify hsets created by mochi mqtt.
const defaultHPrefix = "comqtt"

var localIP = "127.0.0.1"

// clientKey returns a primary key for a client.
func clientKey(cl *mqtt.Client) string {
	return cl.ID
}

// subscriptionKey returns a primary key for a subscription.
func subscriptionKey(cl *mqtt.Client, filter string) string {
	return filter
}

// retainedKey returns a primary key for a retained message.
func retainedKey(topic string) string {
	return topic
}

// inflightKey returns a primary key for an inflight message.
func inflightKey(cl *mqtt.Client, pk packets.Packet) string {
	return pk.FormatID()
}

// sysInfoKey returns a primary key for system info.
func sysInfoKey() string {
	return localIP
}

// Options contains configuration settings for the bolt instance.
type Options struct {
	HPrefix string `json:"prefix" yaml:"prefix"`
	Options *redis.Options
}

// Storage is a persistent storage hook based using Redis as a backend.
type Storage struct {
	mqtt.HookBase
	config *Options        // options for connecting to the Redis instance.
	db     *redis.Client   // the Redis instance
	ctx    context.Context // a context for the connection
}

// ID returns the id of the hook.
func (s *Storage) ID() string {
	return "redis-db"
}

// Provides indicates which hook methods this hook provides.
func (s *Storage) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnSessionEstablished,
		mqtt.OnDisconnect,
		mqtt.OnSubscribed,
		mqtt.OnUnsubscribed,
		mqtt.OnRetainMessage,
		mqtt.OnQosPublish,
		mqtt.OnQosComplete,
		mqtt.OnQosDropped,
		mqtt.OnWillSent,
		mqtt.OnSysInfoTick,
		mqtt.OnClientExpired,
		mqtt.OnRetainedExpired,
		mqtt.StoredClients,
		mqtt.StoredInflightMessages,
		mqtt.StoredRetainedMessages,
		mqtt.StoredSubscriptions,
		mqtt.StoredSysInfo,
	}, []byte{b})
}

// hKey returns a hash set key with a unique prefix.
func (s *Storage) hKey(str string) string {
	return s.config.HPrefix + str
}

// Init initializes and connects to the redis service.
func (s *Storage) Init(config any) error {
	if _, ok := config.(*Options); !ok && config != nil {
		return mqtt.ErrInvalidConfigType
	}

	localIP, _ = utils.GetOutBoundIP()
	s.ctx = context.Background()

	if config == nil {
		config = &Options{
			Options: &redis.Options{
				Addr: defaultAddr,
			},
		}
	}

	s.config = config.(*Options)
	if s.config.HPrefix == "" {
		s.config.HPrefix = defaultHPrefix
	}
	s.config.HPrefix += ":"

	s.Log.Info("connecting to redis service",
		"address", s.config.Options.Addr,
		"username", s.config.Options.Username,
		"password-len", len(s.config.Options.Password),
		"db", s.config.Options.DB)

	s.db = redis.NewClient(s.config.Options)
	_, err := s.db.Ping(context.Background()).Result()
	if err != nil {
		return fmt.Errorf("failed to ping service: %w", err)
	}

	s.Log.Info("connected to redis service")

	return nil
}

// Stop closes the redis connection.
func (s *Storage) Stop() error {
	s.Log.Info("disconnecting from redis service")
	return s.db.Close()
}

// OnSessionEstablished adds a client to the store when their session is established.
func (s *Storage) OnSessionEstablished(cl *mqtt.Client, pk packets.Packet) {
	s.updateClient(cl)
}

// OnWillSent is called when a client sends a will message and the will message is removed
// from the client record.
func (s *Storage) OnWillSent(cl *mqtt.Client, pk packets.Packet) {
	s.updateClient(cl)
}

// updateClient writes the client data to the store.
func (s *Storage) updateClient(cl *mqtt.Client) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	props := cl.Properties.Props.Copy(false)
	in := &storage.Client{
		ID:              cl.ID,
		Remote:          cl.Net.Remote,
		Listener:        cl.Net.Listener,
		Username:        cl.Properties.Username,
		Clean:           cl.Properties.Clean,
		ProtocolVersion: cl.Properties.ProtocolVersion,
		Properties: storage.ClientProperties{
			SessionExpiryInterval: props.SessionExpiryInterval,
			AuthenticationMethod:  props.AuthenticationMethod,
			AuthenticationData:    props.AuthenticationData,
			RequestProblemInfo:    props.RequestProblemInfo,
			RequestResponseInfo:   props.RequestResponseInfo,
			ReceiveMaximum:        props.ReceiveMaximum,
			TopicAliasMaximum:     props.TopicAliasMaximum,
			User:                  props.User,
			MaximumPacketSize:     props.MaximumPacketSize,
		},
		Will: storage.ClientWill(cl.Properties.Will),
	}

	err := s.db.HSet(s.ctx, s.hKey(storage.ClientKey), clientKey(cl), in).Err()
	if err != nil {
		s.Log.Error("failed to hset client data", "error", storage.ErrDBFileNotOpen, "data", in)
	}
}

// OnDisconnect removes a client from the store if they were using a clean session.
func (s *Storage) OnDisconnect(cl *mqtt.Client, _ error, expire bool) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	if !expire {
		return
	}

	err := s.db.HDel(s.ctx, s.hKey(storage.ClientKey), clientKey(cl)).Err()
	if err != nil {
		s.Log.Error("failed to delete client", "error", err, "id", clientKey(cl))
	}
}

// OnSubscribed adds one or more client subscriptions to the store.
func (s *Storage) OnSubscribed(cl *mqtt.Client, pk packets.Packet, reasonCodes []byte, counts []int) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	var in *storage.Subscription
	for i := 0; i < len(pk.Filters); i++ {
		if reasonCodes[i] == 0x80 {
			continue
		}
		in = &storage.Subscription{
			Qos:               reasonCodes[i],
			Identifier:        pk.Filters[i].Identifier,
			RetainHandling:    pk.Filters[i].RetainHandling,
			RetainAsPublished: pk.Filters[i].RetainAsPublished,
			NoLocal:           pk.Filters[i].NoLocal,
		}

		err := s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, cl.ID)), pk.Filters[i].Filter, in).Err()
		if err != nil {
			s.Log.Error("failed to hset subscription data", "error", err, "data", in)
		}
	}
}

// OnUnsubscribed removes one or more client subscriptions from the store.
func (s *Storage) OnUnsubscribed(cl *mqtt.Client, pk packets.Packet, reasonCodes []byte, counts []int) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	for i := 0; i < len(pk.Filters); i++ {
		err := s.db.HDel(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, cl.ID)), pk.Filters[i].Filter).Err()
		if err != nil {
			s.Log.Error("failed to delete subscription data", "error", err, "id", clientKey(cl))
		}
	}
}

// OnRetainMessage adds a retained message for a topic to the store.
func (s *Storage) OnRetainMessage(cl *mqtt.Client, pk packets.Packet, r int64) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	if r == -1 {
		err := s.db.HDel(s.ctx, s.hKey(storage.RetainedKey), retainedKey(pk.TopicName)).Err()
		if err != nil {
			s.Log.Error("failed to delete retained message data", "error", err, "id", clientKey(cl))
		}

		return
	}

	props := pk.Properties.Copy(false)
	in := &storage.Message{
		FixedHeader: pk.FixedHeader,
		Payload:     pk.Payload,
		Created:     pk.Created,
		Origin:      pk.Origin,
		Properties: storage.MessageProperties{
			PayloadFormat:          props.PayloadFormat,
			MessageExpiryInterval:  props.MessageExpiryInterval,
			ContentType:            props.ContentType,
			ResponseTopic:          props.ResponseTopic,
			CorrelationData:        props.CorrelationData,
			SubscriptionIdentifier: props.SubscriptionIdentifier,
			TopicAlias:             props.TopicAlias,
			User:                   props.User,
		},
	}

	err := s.db.HSet(s.ctx, s.hKey(storage.RetainedKey), retainedKey(pk.TopicName), in).Err()
	if err != nil {
		s.Log.Error("failed to hset retained message data", "error", err, "data", in)
	}
}

// OnQosPublish adds or updates an inflight message in the store.
func (s *Storage) OnQosPublish(cl *mqtt.Client, pk packets.Packet, sent int64, resends int) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	props := pk.Properties.Copy(false)
	in := &storage.Message{
		Origin:      pk.Origin,
		FixedHeader: pk.FixedHeader,
		TopicName:   pk.TopicName,
		Payload:     pk.Payload,
		PacketID:    pk.PacketID,
		Sent:        sent,
		Created:     pk.Created,
		Properties: storage.MessageProperties{
			PayloadFormat:          props.PayloadFormat,
			MessageExpiryInterval:  props.MessageExpiryInterval,
			ContentType:            props.ContentType,
			ResponseTopic:          props.ResponseTopic,
			CorrelationData:        props.CorrelationData,
			SubscriptionIdentifier: props.SubscriptionIdentifier,
			TopicAlias:             props.TopicAlias,
			User:                   props.User,
		},
	}

	err := s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.InflightKey, cl.ID)), inflightKey(cl, pk), in).Err()
	if err != nil {
		s.Log.Error("failed to hset qos inflight message data", "error", err, "data", in)
	}
}

// OnQosComplete removes a resolved inflight message from the store.
func (s *Storage) OnQosComplete(cl *mqtt.Client, pk packets.Packet) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	err := s.db.HDel(s.ctx, s.hKey(utils.JoinStrings(storage.InflightKey, cl.ID)), inflightKey(cl, pk)).Err()
	if err != nil {
		s.Log.Error("failed to delete inflight message data", "error", err, "id", clientKey(cl))
	}
}

// OnQosDropped removes a dropped inflight message from the store.
func (s *Storage) OnQosDropped(cl *mqtt.Client, pk packets.Packet) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
	}

	s.OnQosComplete(cl, pk)
}

// OnSysInfoTick stores the latest system info in the store.
func (s *Storage) OnSysInfoTick(sys *system.Info) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	in := &storage.SystemInfo{
		Info: *sys,
	}

	err := s.db.HSet(s.ctx, s.hKey(storage.SysInfoKey), sysInfoKey(), in).Err()
	if err != nil {
		s.Log.Error("failed to hset server info data", "error", err, "data", in)
	}
}

// OnRetainedExpired deletes expired retained messages from the store.
func (s *Storage) OnRetainedExpired(filter string) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	err := s.db.HDel(s.ctx, s.hKey(storage.RetainedKey), retainedKey(filter)).Err()
	if err != nil {
		s.Log.Error("failed to delete retained message data", "error", err, "id", retainedKey(filter))
	}
}

// OnClientExpired deleted expired clients from the store.
func (s *Storage) OnClientExpired(cl *mqtt.Client) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	err := s.db.HDel(s.ctx, s.hKey(storage.ClientKey), clientKey(cl)).Err()
	if err != nil {
		s.Log.Error("failed to delete expired client", "error", err, "id", clientKey(cl))
	}
}

// StoredSysInfo returns the system info from the store.
func (s *Storage) StoredSysInfo() (v storage.SystemInfo, err error) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	row, err := s.db.HGet(s.ctx, s.hKey(storage.SysInfoKey), sysInfoKey()).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return
	}

	if err = v.UnmarshalBinary([]byte(row)); err != nil {
		s.Log.Error("failed to unmarshal sys info data", "error", err, "data", row)
	}

	return v, nil
}

// StoredClientByCid returns a stored client from the store.
func (s *Storage) StoredClientByCid(cid string) (v storage.Client, err error) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}
	row, err := s.db.HGet(s.ctx, s.hKey(storage.ClientKey), cid).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		s.Log.Error("failed to HGet client data", "error", err)
		return
	}

	if err = v.UnmarshalBinary([]byte(row)); err != nil {
		s.Log.Error("failed to unmarshal client data", "error", err, "data", row)
	}

	return v, nil
}

// StoredSubscriptionsByCid returns all stored subscriptions of client from the store.
func (s *Storage) StoredSubscriptionsByCid(cid string) (v []storage.Subscription, err error) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	rows, err := s.db.HGetAll(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, cid))).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		s.Log.Error("failed to HGetAll subscription data", "error", err)
		return
	}

	for filter, row := range rows {
		var d storage.Subscription
		if err = d.UnmarshalBinary([]byte(row)); err != nil {
			s.Log.Error("failed to unmarshal subscription data", "error", err, "data", row)
		}

		if d.Filter == "" {
			d.Filter = filter
		}

		v = append(v, d)
	}

	return v, nil
}

// StoredRetainedMessageByTopic returns a stored retained message of topic from the store.
func (s *Storage) StoredRetainedMessageByTopic(topic string) (v storage.Message, err error) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	row, err := s.db.HGet(s.ctx, s.hKey(storage.RetainedKey), retainedKey(topic)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		s.Log.Error("failed to HGetAll retained message data", "error", err)
		return
	}

	if err = v.UnmarshalBinary([]byte(row)); err != nil {
		s.Log.Error("failed to unmarshal retained message dat", "error", err, "data", row)
	}

	if v.TopicName == "" {
		v.TopicName = topic
	}

	return v, nil
}

// StoredInflightMessagesByCid returns all stored inflight messages of client from the store.
func (s *Storage) StoredInflightMessagesByCid(cid string) (v []storage.Message, err error) {
	if s.db == nil {
		s.Log.Error("", "error", storage.ErrDBFileNotOpen)
		return
	}

	rows, err := s.db.HGetAll(s.ctx, s.hKey(utils.JoinStrings(storage.InflightKey, cid))).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		s.Log.Error("failed to HGetAll inflight message data", "error", err)
		return
	}

	for _, row := range rows {
		var d storage.Message
		if err = d.UnmarshalBinary([]byte(row)); err != nil {
			s.Log.Error("failed to unmarshal inflight message data", "error", err, "data", row)
		}

		v = append(v, d)
	}

	return v, nil
}
