package persistence

import (
	"errors"

	"github.com/wind-c/comqtt/server/system"
)

const (

	// KSubscription is the key for subscription data.
	KSubscription = "sub"

	// KServerInfo is the key for server info data.
	KServerInfo = "srv"

	// KRetained is the key for retained messages data.
	KRetained = "ret"

	// KInflight is the key for inflight messages data.
	KInflight = "ifm"

	// KClient is the key for client data.
	KClient = "cl"
)

// Store is an interface which details a persistent storage connector.
type Store interface {
	Open() error
	Close()

	GenEntityId
	WriteEntity
	DeleteEntity
	ReadClient
	ReadAll
}

type WriteEntity interface {
	WriteSubscription(v Subscription) error
	WriteClient(v Client) error
	WriteInflight(v Message) error
	WriteServerInfo(v ServerInfo) error
	WriteRetained(v Message) error
}

type DeleteEntity interface {
	DeleteSubscription(cid, filter string) error
	DeleteClient(id string) error
	DeleteInflight(cid string, pid uint16) error
	DeleteInflightBatch(cid string, pid []uint16) error
	DeleteRetained(topic string) error
}

type ReadAll interface {
	ReadSubscriptions() (v []Subscription, err error)
	ReadInflight() (v []Message, err error)
	ReadRetained() (v []Message, err error)
	ReadClients() (v []Client, err error)
	ReadServerInfo() (v ServerInfo, err error)
}

type ReadClient interface {
	ReadSubscriptionsByCid(cid string) (v []Subscription, err error)
	ReadInflightByCid(cid string) (v []Message, err error)
	ReadRetainedByTopic(topic string) (v Message, err error)
	ReadClientByCid(cid string) (v Client, err error)
}

type GenEntityId interface {
	GenInflightId(cid string, pid uint16) string
	GenSubscriptionId(cid, filter string) string
	GenRetainedId(topic string) string
}

//go:generate msgp -io=false

// ServerInfo contains information and statistics about the server.
type ServerInfo struct {
	system.Info `json:"info" msg:"info"` // embed the system info struct.
	ID          string                   `json:"id" msg:"id"` // the storage key.
}

// Subscription contains the details of a topic filter subscription.
type Subscription struct {
	ID     string `json:"-" msg:"-"`           // the storage key.
	T      string `json:"-" msg:"-"`           // the type of the stored data.
	Client string `json:"-" msg:"-"`           // the id of the client who the subscription belongs to.
	Filter string `json:"filter" msg:"filter"` // the topic filter being subscribed to.
	QoS    byte   `json:"qos" msg:"qos"`       // the desired QoS byte.

	// v5.0
	NoLocal           bool `json:"nl,omitempty" msg:"nl,omitempty"`
	RetainHandling    byte `json:"rh,omitempty" msg:"rh,omitempty"`
	RetainAsPublished bool `json:"rap,omitempty" msg:"rap,omitempty"`
}

// Message contains the details of a retained or inflight message.
type Message struct {
	Payload     []byte      `json:"payload" msg:"payload"`                 // the message payload (if retained).
	FixedHeader FixedHeader `json:"fixed_header" msg:"fixed_header"`       // the header properties of the message.
	T           string      `json:"-" msg:"-"`                             // the type of the stored data.
	ID          string      `json:"-" msg:"-"`                             // the storage key.
	Client      string      `json:"-" msg:"-"`                             // the id of the client who sent the message (if inflight).
	TopicName   string      `json:"topic_name" msg:"topic_name"`           // the topic the message was sent to (if retained).
	Sent        int64       `json:"sent" msg:"sent"`                       // the last time the message was sent (for retries) in unixtime (if inflight).
	Resends     int         `json:"resends" msg:"resends"`                 // the number of times the message was attempted to be sent (if inflight).
	PacketID    uint16      `json:"packet_id" msg:"packet_id"`             // the unique id of the packet (if inflight).
	Properties  Properties  `json:"props,omitempty" msg:"props,omitempty"` // for v5
}

type Properties struct {
	Expiry          int64      `json:"expiry" msg:"expiry"` // the message expiration time in unixtime.
	PayloadFormat   *byte      `json:"payload_format,omitempty" msg:"payload_format,omitempty"`
	ContentType     string     `json:"content_type,omitempty" msg:"content_type,omitempty"`
	ResponseTopic   string     `json:"resp_topic,omitempty" msg:"resp_topic,omitempty"`
	CorrelationData []byte     `json:"corr_data,omitempty" msg:"corr_data,omitempty"`
	UserProperties  []KeyValue `json:"user_props,omitempty" msg:"user_props,omitempty"`
}

type KeyValue struct {
	Key   string `json:"key" msg:"key"`
	Value string `json:"value" msg:"value"`
}

//go:generate msgp -io=false
// FixedHeader contains the fixed header properties of a message.
type FixedHeader struct {
	Remaining int  `json:"remaining" msg:"remaining"` // the number of remaining bytes in the payload.
	Type      byte `json:"type" msg:"type"`           // the type of the packet (PUBLISH, SUBSCRIBE, etc) from bits 7 - 4 (byte 1).
	Qos       byte `json:"qos" msg:"qos"`             // indicates the quality of service expected.
	Dup       bool `json:"dup" msg:"dup"`             // indicates if the packet was already sent at an earlier time.
	Retain    bool `json:"retain" msg:"retain"`       // whether the message should be retained.
}

//go:generate msgp -io=false
// Client contains client data that can be persistently stored.
type Client struct {
	LWT      LWT    `json:"lwt" msg:"lwt"`           // the last-will-and-testament message for the client.
	Username []byte `json:"username" msg:"username"` // the username the client authenticated with.
	ID       string `json:"-" msg:"-"`               // this field is ignored// the storage key.
	ClientID string `json:"-" msg:"-"`               // the id of the client.
	T        string `json:"-" msg:"-"`               // the type of the stored data.
	Listener string `json:"listener" msg:"listener"` // the last known listener id for the client
}

// LWT contains details about a clients LWT payload.
type LWT struct {
	Message []byte `json:"message" msg:"message"` // the message that shall be sent when the client disconnects.
	Topic   string `json:"topic" msg:"topic"`     // the topic the will message shall be sent to.
	Qos     byte   `json:"qos" msg:"qos"`         // the quality of service desired.
	Retain  bool   `json:"retain" msg:"retain"`   // indicates whether the will message should be retained
}

// MockStore is a mock storage backend for testing.
type MockStore struct {
	Fail     map[string]bool // issue errors for different methods.
	FailOpen bool            // error on open.
	Closed   bool            // indicate mock store is closed.
	Opened   bool            // indicate mock store is open.
}

// Open opens the storage instance.
func (s *MockStore) Open() error {
	if s.FailOpen {
		return errors.New("test")
	}

	s.Opened = true
	return nil
}

// Close closes the storage instance.
func (s *MockStore) Close() {
	s.Closed = true
}

// WriteSubscription writes a single subscription to the storage instance.
func (s *MockStore) WriteSubscription(v Subscription) error {
	if _, ok := s.Fail["write_subs"]; ok {
		return errors.New("test")
	}
	return nil
}

// WriteClient writes a single client to the storage instance.
func (s *MockStore) WriteClient(v Client) error {
	if _, ok := s.Fail["write_clients"]; ok {
		return errors.New("test")
	}
	return nil
}

// WriteInFlight writes a single InFlight message to the storage instance.
func (s *MockStore) WriteInflight(v Message) error {
	if _, ok := s.Fail["write_inflight"]; ok {
		return errors.New("test")
	}
	return nil
}

// WriteRetained writes a single retained message to the storage instance.
func (s *MockStore) WriteRetained(v Message) error {
	if _, ok := s.Fail["write_retained"]; ok {
		return errors.New("test")
	}
	return nil
}

// WriteServerInfo writes server info to the storage instance.
func (s *MockStore) WriteServerInfo(v ServerInfo) error {
	if _, ok := s.Fail["write_info"]; ok {
		return errors.New("test")
	}
	return nil
}

// DeleteSubscription deletes a subscription from the persistent store.
func (s *MockStore) DeleteSubscription(cid, filter string) error {
	if _, ok := s.Fail["delete_subs"]; ok {
		return errors.New("test")
	}

	return nil
}

// DeleteClient deletes a client from the persistent store.
func (s *MockStore) DeleteClient(id string) error {
	if _, ok := s.Fail["delete_clients"]; ok {
		return errors.New("test")
	}

	return nil
}

// DeleteInflight deletes an inflight message from the persistent store.
func (s *MockStore) DeleteInflight(cid string, pid uint16) error {
	if _, ok := s.Fail["delete_inflight"]; ok {
		return errors.New("test")
	}

	return nil
}

// DeleteInflightBatch
func (s *MockStore) DeleteInflightBatch(cid string, pid []uint16) error {
	if _, ok := s.Fail["delete_inflight"]; ok {
		return errors.New("test")
	}

	return nil
}

// DeleteRetained deletes a retained message from the persistent store.
func (s *MockStore) DeleteRetained(topic string) error {
	if _, ok := s.Fail["delete_retained"]; ok {
		return errors.New("test")
	}

	return nil
}

// ReadSubscriptions loads the subscriptions from the storage instance.
func (s *MockStore) ReadSubscriptions() (v []Subscription, err error) {
	if _, ok := s.Fail["read_subs"]; ok {
		return v, errors.New("test_subs")
	}

	return []Subscription{
		Subscription{
			ID:     "test:a/b/c",
			Client: "test",
			Filter: "a/b/c",
			QoS:    1,
			T:      KSubscription,
		},
	}, nil
}

// ReadClients loads the clients from the storage instance.
func (s *MockStore) ReadClients() (v []Client, err error) {
	if _, ok := s.Fail["read_clients"]; ok {
		return v, errors.New("test_clients")
	}

	return []Client{
		Client{
			ID:       "cl_client1",
			ClientID: "client1",
			T:        KClient,
			Listener: "tcp1",
		},
	}, nil
}

// ReadInflight loads the inflight messages from the storage instance.
func (s *MockStore) ReadInflight() (v []Message, err error) {
	if _, ok := s.Fail["read_inflight"]; ok {
		return v, errors.New("test_inflight")
	}

	return []Message{
		Message{
			ID:        "client1_if_100",
			T:         KInflight,
			Client:    "client1",
			PacketID:  100,
			TopicName: "d/e/f",
			Payload:   []byte{'y', 'e', 's'},
			Sent:      200,
			Resends:   1,
		},
	}, nil
}

// ReadRetained loads the retained messages from the storage instance.
func (s *MockStore) ReadRetained() (v []Message, err error) {
	if _, ok := s.Fail["read_retained"]; ok {
		return v, errors.New("test_retained")
	}

	return []Message{
		Message{
			ID: "client1_ret_200",
			T:  KRetained,
			FixedHeader: FixedHeader{
				Retain: true,
			},
			PacketID:  200,
			TopicName: "a/b/c",
			Payload:   []byte{'h', 'e', 'l', 'l', 'o'},
			Sent:      100,
			Resends:   0,
		},
	}, nil
}

//ReadServerInfo loads the server info from the storage instance.
func (s *MockStore) ReadServerInfo() (v ServerInfo, err error) {
	if _, ok := s.Fail["read_info"]; ok {
		return v, errors.New("test_info")
	}

	return ServerInfo{
		system.Info{
			Version: "test",
			Started: 100,
		},
		KServerInfo,
	}, nil
}

func (s *MockStore) ReadSubscriptionsByCid(cid string) (v []Subscription, err error) {
	if _, ok := s.Fail["read_subs_by_cid"]; ok {
		return v, errors.New("test_subs")
	}

	return []Subscription{
		Subscription{
			ID:     "test:a/b/c",
			Client: "test",
			Filter: "a/b/c",
			QoS:    1,
			T:      KSubscription,
		},
	}, nil
}

func (s *MockStore) ReadInflightByCid(cid string) (v []Message, err error) {
	if _, ok := s.Fail["read_inflight_by_cid"]; ok {
		return v, errors.New("test_inflight")
	}

	return []Message{
		Message{
			ID:        "client1_if_100",
			T:         KInflight,
			Client:    "client1",
			PacketID:  100,
			TopicName: "d/e/f",
			Payload:   []byte{'y', 'e', 's'},
			Sent:      200,
			Resends:   1,
		},
	}, nil
}

func (s *MockStore) ReadRetainedByTopic(topic string) (v Message, err error) {
	if _, ok := s.Fail["read_retained_by_topic"]; ok {
		return v, errors.New("test_retained")
	}

	return Message{
		ID: "client1_ret_200",
		T:  KRetained,
		FixedHeader: FixedHeader{
			Retain: true,
		},
		PacketID:  200,
		TopicName: "a/b/c",
		Payload:   []byte{'h', 'e', 'l', 'l', 'o'},
		Sent:      100,
		Resends:   0,
	}, nil
}

func (s *MockStore) ReadClientByCid(cid string) (v Client, err error) {
	if _, ok := s.Fail["read_client_by_cid"]; ok {
		return v, errors.New("test_clients")
	}

	return Client{
		ID:       "cl_client1",
		ClientID: "client1",
		T:        KClient,
		Listener: "tcp1",
	}, nil
}

func (s *MockStore) GenInflightId(cid string, pid uint16) string {
	return "ifm_client1_101"
}

func (s *MockStore) GenSubscriptionId(cid, filter string) string {
	return "sub_client1_a/b/c"
}

func (s *MockStore) GenRetainedId(topic string) string {
	return "ret_client1_102"
}
