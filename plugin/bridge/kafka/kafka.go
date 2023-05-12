// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind

package kafka

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/segmentio/kafka-go"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/plugin"
	"strings"
	"time"
)

const defaultAddr = "localhost:9092"
const defaultTopic = "comqtt"

const (
	//Connect mqtt connect
	Connect = "connect"
	//Publish mqtt publish
	Publish = "publish"
	//Subscribe mqtt sub
	Subscribe = "subscribe"
	//Unsubscribe mqtt sub
	Unsubscribe = "unsubscribe"
	//Disconnect mqtt disconenct
	Disconnect = "disconnect"
)

const (
	balancerLeastBytes byte = iota
	balancerRoundRobin
	balancerHash
	balancerCRC32Balancer
)

// Message kafka publish message
type Message struct {
	Action          string   `json:"action"`
	ClientID        string   `json:"clientid"`                  // the client id
	Username        string   `json:"username"`                  // the username of the client
	Remote          string   `json:"remote,omitempty"`          // the remote address of the client
	Listener        string   `json:"listener,omitempty"`        // the listener the client connected on
	Topics          []string `json:"topics,omitempty"`          // publish topic or subscribe/unsubscribe filters
	reasonCodes     []byte   `json:"reasonCodes,omitempty"`     // subscribe/unsubscribe filters success(0) or failure(>0x80) code
	Payload         []byte   `json:"payload,omitempty"`         // publish payload
	ProtocolVersion byte     `json:"protocolVersion,omitempty"` // mqtt protocol version of the client
	Clean           bool     `json:"clean,omitempty"`           // if the client requested a clean start/session
	Timestamp       int64    `json:"ts"`                        // event time
}

// MarshalBinary encodes the values into a json string.
func (d Message) MarshalBinary() (data []byte, err error) {
	return json.Marshal(d)
}

// UnmarshalBinary decodes a json string into a struct.
func (d *Message) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, d)
}

type Options struct {
	KafkaOptions *kafkaOptions `json:"kafka-options" yaml:"kafka-options"`
	Rules        rules         `json:"rules" yaml:"rules"`
}

type kafkaOptions struct {
	Brokers      []string `json:"brokers" yaml:"brokers"`
	Topic        string   `json:"topic" yaml:"topic"`
	Balancer     byte     `json:"balancer" yaml:"balancer"` // 0 LeastBytes、1 RoundRobin、2 Hash、3 CRC32Balancer
	Async        bool     `json:"async" yaml:"async"`
	RequiredAcks byte     `json:"required-acks" yaml:"required-acks"` // 0 None、1 Leader、-1 All
	Compression  byte     `json:"compression" yaml:"compression"`     // 0 Node、1 Gzip、2 Snappy、3 Lz4、4 Zstd
	WriteTimeout int      `json:"write-timeout" yaml:"write-timeout"` // defaults to 10 seconds
}

type rules struct {
	Topics  []string `json:"topics" yaml:"topics"`
	Filters []string `json:"filters" yaml:"filters"`
}

type Bridge struct {
	mqtt.HookBase
	config *Options
	writer *kafka.Writer
	ctx    context.Context // a context for the connection
}

// ID returns the ID of the hook.
func (b *Bridge) ID() string {
	return "bridge-kafka"
}

// Provides indicates which hook methods this hook provides.
func (b *Bridge) Provides(bt byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnSessionEstablished,
		mqtt.OnDisconnect,
		mqtt.OnPublished,
		mqtt.OnSubscribed,
		mqtt.OnUnsubscribed,
	}, []byte{bt})
}

func (b *Bridge) Init(config any) error {
	if _, ok := config.(*Options); !ok && config != nil {
		return mqtt.ErrInvalidConfigType
	}

	b.ctx = context.Background()

	if config == nil {
		config = &Options{
			KafkaOptions: &kafkaOptions{
				Brokers: []string{defaultAddr},
				Topic:   defaultTopic,
			},
		}
	}

	b.config = config.(*Options)
	b.Log.Info().
		Str("brokers", strings.Join(b.config.KafkaOptions.Brokers, ",")).
		Str("topic", b.config.KafkaOptions.Topic).
		Bool("async", b.config.KafkaOptions.Async).
		Msg("connecting to kafka service")

	var balancer kafka.Balancer
	switch b.config.KafkaOptions.Balancer {
	case balancerLeastBytes:
		balancer = &kafka.LeastBytes{}
	case balancerRoundRobin:
		balancer = &kafka.RoundRobin{}
	case balancerHash:
		balancer = &kafka.Hash{}
	case balancerCRC32Balancer:
		balancer = &kafka.CRC32Balancer{}
	default:
		balancer = &kafka.LeastBytes{}
	}

	b.writer = &kafka.Writer{
		Addr:                   kafka.TCP(b.config.KafkaOptions.Brokers...),
		Topic:                  b.config.KafkaOptions.Topic,
		Async:                  b.config.KafkaOptions.Async,
		RequiredAcks:           kafka.RequiredAcks(b.config.KafkaOptions.RequiredAcks),
		Compression:            kafka.Compression(b.config.KafkaOptions.Compression),
		WriteTimeout:           time.Duration(b.config.KafkaOptions.WriteTimeout) * time.Second,
		Balancer:               balancer,
		AllowAutoTopicCreation: true,
		Completion:             b.handler,
	}

	// verify connect
	if _, err := b.kafkaTopics(); err != nil {
		b.Log.Error().Err(err).Msg("cannot connect to kafka service")
	} else {
		b.Log.Info().Msg("connected to kafka service")
	}

	return nil
}

func (b *Bridge) kafkaTopics() (map[string]struct{}, error) {
	conn, err := kafka.Dial("tcp", b.config.KafkaOptions.Brokers[0])
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	partitions, err := conn.ReadPartitions("comqtt") // if you want to get all topics, you don't need parameter
	if err != nil {
		return nil, err
	}

	m := map[string]struct{}{}

	for _, p := range partitions {
		m[p.Topic] = struct{}{}
	}

	return m, nil
}

// Stop closes the redis connection.
func (b *Bridge) Stop() error {
	b.Log.Info().Msg("disconnecting from kafka service")
	return b.writer.Close()
}

func (b *Bridge) handler(messages []kafka.Message, err error) {
	if err != nil {
		keys := make([]string, 1)
		for _, msg := range messages {
			keys = append(keys, string(msg.Key))
		}
		b.Log.Err(err).Strs("keys", keys).Msg("write msg to kafka")
	}
}

func (b *Bridge) checkTopic(topic string) bool {
	if b.config.Rules.Topics == nil || len(b.config.Rules.Topics) == 0 {
		return true
	}

	for _, t := range b.config.Rules.Topics {
		if ok := plugin.MatchTopic(t, topic); ok {
			return true
		}
	}
	return false
}

func (b *Bridge) checkFilter(filter string) bool {
	if b.config.Rules.Filters == nil || len(b.config.Rules.Filters) == 0 {
		return true
	}

	for _, f := range b.config.Rules.Filters {
		if ok := plugin.MatchTopic(f, filter); ok {
			return true
		}
	}
	return false
}

// OnSessionEstablished is called when a new client establishes a session (after OnConnect).
func (b *Bridge) OnSessionEstablished(cl *mqtt.Client, pk packets.Packet) {
	timestamp := genTimestamp(pk.Created)
	msg := &Message{
		Action:          Connect,
		ClientID:        cl.ID,
		Remote:          cl.Net.Remote,
		Listener:        cl.Net.Listener,
		Username:        string(cl.Properties.Username),
		Clean:           cl.Properties.Clean,
		ProtocolVersion: cl.Properties.ProtocolVersion,
		Timestamp:       timestamp,
	}
	data, err := msg.MarshalBinary()
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnSessionEstablished")
		return
	}

	err = b.writer.WriteMessages(b.ctx, kafka.Message{
		Key:   genKey(cl.ID, timestamp),
		Value: data,
	})
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnSessionEstablished")
	}
}

// OnDisconnect is called when a client is disconnected for any reason.
func (b *Bridge) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	timestamp := time.Now().Unix()
	msg := &Message{
		Action:    Disconnect,
		ClientID:  cl.ID,
		Username:  string(cl.Properties.Username),
		Timestamp: timestamp,
	}
	data, err := msg.MarshalBinary()
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnDisconnect")
		return
	}

	err = b.writer.WriteMessages(b.ctx, kafka.Message{
		Key:   genKey(cl.ID, timestamp),
		Value: data,
	})
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnDisconnect")
	}
}

// OnPublished is called when a client has published a message to subscribers.
func (b *Bridge) OnPublished(cl *mqtt.Client, pk packets.Packet) {
	if !b.checkTopic(pk.TopicName) {
		return
	}

	timestamp := genTimestamp(pk.Created)
	msg := &Message{
		Action:    Publish,
		ClientID:  cl.ID,
		Username:  string(cl.Properties.Username),
		Topics:    []string{pk.TopicName},
		Payload:   pk.Payload,
		Timestamp: timestamp,
	}
	data, err := msg.MarshalBinary()
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnPublished")
		return
	}

	err = b.writer.WriteMessages(b.ctx, kafka.Message{
		Key:   genKey(fmt.Sprint(pk.PacketID), timestamp),
		Value: data,
	})
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnPublished")
	}
}

// OnSubscribed is called when a client subscribes to one or more filters.
func (b *Bridge) OnSubscribed(cl *mqtt.Client, pk packets.Packet, reasonCodes []byte, counts []int) {
	filters := make([]string, 0)
	codes := make([]byte, 0)
	for i, sub := range pk.Filters {
		if b.checkFilter(sub.Filter) {
			filters = append(filters, sub.Filter)
			codes = append(codes, reasonCodes[i])
		}
	}
	if len(filters) == 0 {
		return
	}

	timestamp := genTimestamp(pk.Created)
	msg := &Message{
		Action:      Subscribe,
		ClientID:    cl.ID,
		Username:    string(cl.Properties.Username),
		Topics:      filters,
		reasonCodes: codes,
		Timestamp:   timestamp,
	}
	data, err := msg.MarshalBinary()
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnSubscribed")
		return
	}

	err = b.writer.WriteMessages(b.ctx, kafka.Message{
		Key:   genKey(fmt.Sprint(pk.PacketID), timestamp),
		Value: data,
	})
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnSubscribed")
	}
}

// OnUnsubscribed is called when a client unsubscribes from one or more filters.
func (b *Bridge) OnUnsubscribed(cl *mqtt.Client, pk packets.Packet, reasonCodes []byte, counts []int) {
	filters := make([]string, 0)
	codes := make([]byte, 0)
	for i, sub := range pk.Filters {
		if b.checkFilter(sub.Filter) {
			filters = append(filters, sub.Filter)
			codes = append(codes, reasonCodes[i])
		}
	}
	timestamp := genTimestamp(pk.Created)
	msg := &Message{
		Action:      Unsubscribe,
		ClientID:    cl.ID,
		Username:    string(cl.Properties.Username),
		Topics:      filters,
		reasonCodes: codes,
		Timestamp:   timestamp,
	}
	data, err := msg.MarshalBinary()
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnUnsubscribed")
		return
	}

	err = b.writer.WriteMessages(b.ctx, kafka.Message{
		Key:   genKey(fmt.Sprint(pk.PacketID), timestamp),
		Value: data,
	})
	if err != nil {
		b.Log.Error().Err(err).Msg("bridge-kafka:OnUnsubscribed")
	}
}

func genKey(id string, timestamp int64) []byte {
	var buf bytes.Buffer
	buf.WriteString(id)
	buf.WriteString("-")
	buf.WriteString(fmt.Sprint(timestamp))
	return buf.Bytes()
}

func genTimestamp(created int64) int64 {
	if created == 0 {
		created = time.Now().Unix()
	}
	return created
}
