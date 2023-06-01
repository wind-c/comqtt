// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package redis

import (
	"github.com/wind-c/comqtt/v2/cluster/utils"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/storage"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/mqtt/system"
	"os"
	"sort"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	redis "github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var (
	logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Level(zerolog.Disabled)

	client = &mqtt.Client{
		ID: "test",
		Net: mqtt.ClientConnection{
			Remote:   "test.addr",
			Listener: "listener",
		},
		Properties: mqtt.ClientProperties{
			Username: []byte("username"),
			Clean:    false,
		},
	}

	pkf = packets.Packet{Filters: packets.Subscriptions{{Filter: "a/b/c"}}}
)

func newHook(t *testing.T, addr string) *Storage {
	s := new(Storage)
	s.SetOpts(&logger, nil)

	err := s.Init(&Options{
		Options: &redis.Options{
			Addr: addr,
		},
	})
	require.NoError(t, err)

	return s
}

func teardown(t *testing.T, s *Storage) {
	if s.db != nil {
		err := s.db.FlushAll(s.ctx).Err()
		require.NoError(t, err)
		s.Stop()
	}
}

func TestClientKey(t *testing.T) {
	k := clientKey(&mqtt.Client{ID: "cl1"})
	require.Equal(t, "cl1", k)
}

func TestSubscriptionKey(t *testing.T) {
	k := subscriptionKey(&mqtt.Client{ID: "cl1"}, "a/b/c")
	require.Equal(t, "a/b/c", k)
}

func TestRetainedKey(t *testing.T) {
	k := retainedKey("a/b/c")
	require.Equal(t, "a/b/c", k)
}

func TestInflightKey(t *testing.T) {
	k := inflightKey(&mqtt.Client{ID: "cl1"}, packets.Packet{PacketID: 1})
	require.Equal(t, "1", k)
}

func TestID(t *testing.T) {
	s := new(Storage)
	s.SetOpts(&logger, nil)
	require.Equal(t, "redis-db", s.ID())
}

func TestProvides(t *testing.T) {
	s := new(Storage)
	s.SetOpts(&logger, nil)
	require.True(t, s.Provides(mqtt.OnSessionEstablished))
	require.True(t, s.Provides(mqtt.OnDisconnect))
	require.True(t, s.Provides(mqtt.OnSubscribed))
	require.True(t, s.Provides(mqtt.OnUnsubscribed))
	require.True(t, s.Provides(mqtt.OnRetainMessage))
	require.True(t, s.Provides(mqtt.OnQosPublish))
	require.True(t, s.Provides(mqtt.OnQosComplete))
	require.True(t, s.Provides(mqtt.OnQosDropped))
	require.True(t, s.Provides(mqtt.OnSysInfoTick))
	require.True(t, s.Provides(mqtt.StoredClients))
	require.True(t, s.Provides(mqtt.StoredInflightMessages))
	require.True(t, s.Provides(mqtt.StoredRetainedMessages))
	require.True(t, s.Provides(mqtt.StoredSubscriptions))
	require.True(t, s.Provides(mqtt.StoredSysInfo))
	require.False(t, s.Provides(mqtt.OnACLCheck))
	require.False(t, s.Provides(mqtt.OnConnectAuthenticate))
}

func TestHKey(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.SetOpts(&logger, nil)
	require.Equal(t, defaultHPrefix+":test", s.hKey("test"))
}

func TestInitUseDefaults(t *testing.T) {
	m := miniredis.RunT(t)
	m.StartAddr(defaultAddr)
	defer m.Close()

	s := newHook(t, defaultAddr)
	s.SetOpts(&logger, nil)
	err := s.Init(nil)
	require.NoError(t, err)
	defer teardown(t, s)

	require.Equal(t, defaultHPrefix+":", s.config.HPrefix)
	require.Equal(t, defaultAddr, s.config.Options.Addr)
}

func TestInitBadConfig(t *testing.T) {
	s := new(Storage)
	s.SetOpts(&logger, nil)

	err := s.Init(map[string]any{})
	require.Error(t, err)
}

func TestInitBadAddr(t *testing.T) {
	s := new(Storage)
	s.SetOpts(&logger, nil)
	err := s.Init(&Options{
		Options: &redis.Options{
			Addr: "abc:123",
		},
	})
	require.Error(t, err)
}

func TestOnSessionEstablishedThenOnDisconnect(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	s.OnSessionEstablished(client, packets.Packet{})

	r := new(storage.Client)
	row, err := s.db.HGet(s.ctx, s.hKey(storage.ClientKey), clientKey(client)).Result()
	require.NoError(t, err)
	err = r.UnmarshalBinary([]byte(row))
	require.NoError(t, err)

	require.Equal(t, client.ID, r.ID)
	require.Equal(t, client.Net.Remote, r.Remote)
	require.Equal(t, client.Net.Listener, r.Listener)
	require.Equal(t, client.Properties.Username, r.Username)
	require.Equal(t, client.Properties.Clean, r.Clean)
	require.NotSame(t, client, r)

	s.OnDisconnect(client, nil, false)
	r2 := new(storage.Client)
	row, err = s.db.HGet(s.ctx, s.hKey(storage.ClientKey), clientKey(client)).Result()
	require.NoError(t, err)
	err = r2.UnmarshalBinary([]byte(row))
	require.NoError(t, err)
	require.Equal(t, client.ID, r.ID)

	s.OnDisconnect(client, nil, true)
	r3 := new(storage.Client)
	_, err = s.db.HGet(s.ctx, s.hKey(storage.ClientKey), clientKey(client)).Result()
	require.Error(t, err)
	require.ErrorIs(t, err, redis.Nil)
	require.Empty(t, r3.ID)
}

func TestOnSessionEstablishedNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())

	s.db = nil
	s.OnSessionEstablished(client, packets.Packet{})
}

func TestOnSessionEstablishedClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnSessionEstablished(client, packets.Packet{})
}

func TestOnWillSent(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	c1 := client
	c1.Properties.Will.Flag = 1
	s.OnWillSent(c1, packets.Packet{})

	r := new(storage.Client)
	row, err := s.db.HGet(s.ctx, s.hKey(storage.ClientKey), clientKey(client)).Result()
	require.NoError(t, err)
	err = r.UnmarshalBinary([]byte(row))
	require.NoError(t, err)

	require.Equal(t, uint32(1), r.Will.Flag)
	require.NotSame(t, client, r)
}

func TestOnClientExpired(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	cl := &mqtt.Client{ID: "cl1"}
	clientKey := clientKey(cl)

	err := s.db.HSet(s.ctx, s.hKey(storage.ClientKey), clientKey, &storage.Client{ID: cl.ID}).Err()
	require.NoError(t, err)

	r := new(storage.Client)
	row, err := s.db.HGet(s.ctx, s.hKey(storage.ClientKey), clientKey).Result()
	require.NoError(t, err)
	err = r.UnmarshalBinary([]byte(row))
	require.NoError(t, err)
	require.Equal(t, clientKey, r.ID)

	s.OnClientExpired(cl)
	_, err = s.db.HGet(s.ctx, s.hKey(storage.ClientKey), clientKey).Result()
	require.Error(t, err)
	require.ErrorIs(t, redis.Nil, err)
}

func TestOnClientExpiredClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnClientExpired(client)
}

func TestOnClientExpiredNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnClientExpired(client)
}

func TestOnDisconnectNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnDisconnect(client, nil, false)
}

func TestOnDisconnectClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnDisconnect(client, nil, false)
}

func TestOnSubscribedThenOnUnsubscribed(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	s.OnSubscribed(client, pkf, []byte{0}, []int{1})

	r := new(storage.Subscription)
	row, err := s.db.HGet(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, client.ID)), pkf.Filters[0].Filter).Result()
	require.NoError(t, err)
	err = r.UnmarshalBinary([]byte(row))
	require.NoError(t, err)
	require.Equal(t, byte(0), r.Qos)

	s.OnUnsubscribed(client, pkf, []byte{0}, []int{0})
	_, err = s.db.HGet(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, client.ID)), pkf.Filters[0].Filter).Result()
	require.Error(t, err)
	require.ErrorIs(t, err, redis.Nil)
}

func TestOnSubscribedNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnSubscribed(client, pkf, []byte{0}, []int{0})
}

func TestOnSubscribedClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnSubscribed(client, pkf, []byte{0}, []int{1})
}

func TestOnUnsubscribedNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnUnsubscribed(client, pkf, []byte{0}, []int{0})
}

func TestOnUnsubscribedClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnUnsubscribed(client, pkf, []byte{0}, []int{0})
}

func TestOnRetainMessageThenUnset(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	pk := packets.Packet{
		FixedHeader: packets.FixedHeader{
			Retain: true,
		},
		Payload:   []byte("hello"),
		TopicName: "a/b/c",
	}

	s.OnRetainMessage(client, pk, 1)

	r, err := s.StoredRetainedMessageByTopic(pk.TopicName)
	require.NoError(t, err)
	require.Equal(t, pk.TopicName, r.TopicName)
	require.Equal(t, pk.Payload, r.Payload)

	s.OnRetainMessage(client, pk, -1)
	_, err = s.db.HGet(s.ctx, s.hKey(storage.RetainedKey), retainedKey(pk.TopicName)).Result()
	require.Error(t, err)
	require.ErrorIs(t, err, redis.Nil)
}

func TestOnRetainedExpired(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	msg := &storage.Message{
		ID:        retainedKey("a/b/c"),
		T:         storage.RetainedKey,
		TopicName: "a/b/c",
	}

	err := s.db.HSet(s.ctx, s.hKey(storage.RetainedKey), msg.ID, msg).Err()
	require.NoError(t, err)

	r := new(storage.Message)
	row, err := s.db.HGet(s.ctx, s.hKey(storage.RetainedKey), msg.ID).Result()
	require.NoError(t, err)
	err = r.UnmarshalBinary([]byte(row))
	require.NoError(t, err)
	require.NoError(t, err)
	require.Equal(t, msg.TopicName, r.TopicName)

	s.OnRetainedExpired(msg.TopicName)

	_, err = s.db.HGet(s.ctx, s.hKey(storage.RetainedKey), msg.ID).Result()
	require.Error(t, err)
	require.ErrorIs(t, err, redis.Nil)
}

func TestOnRetainedExpiredClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnRetainedExpired("a/b/c")
}

func TestOnRetainedExpiredNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnRetainedExpired("a/b/c")
}

func TestOnRetainMessageNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnRetainMessage(client, packets.Packet{}, 0)
}

func TestOnRetainMessageClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnRetainMessage(client, packets.Packet{}, 0)
}

func TestOnQosPublishThenQOSComplete(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	pk := packets.Packet{
		FixedHeader: packets.FixedHeader{
			Retain: true,
			Qos:    2,
		},
		Payload:   []byte("hello"),
		TopicName: "a/b/c",
	}

	s.OnQosPublish(client, pk, time.Now().Unix(), 0)

	r := new(storage.Message)
	row, err := s.db.HGet(s.ctx, s.hKey(utils.JoinStrings(storage.InflightKey, client.ID)), inflightKey(client, pk)).Result()
	require.NoError(t, err)
	err = r.UnmarshalBinary([]byte(row))
	require.NoError(t, err)
	require.Equal(t, pk.TopicName, r.TopicName)
	require.Equal(t, pk.Payload, r.Payload)

	// ensure dates are properly saved to bolt
	require.True(t, r.Sent > 0)
	require.True(t, time.Now().Unix()-1 < r.Sent)

	// OnQosDropped is a passthrough to OnQosComplete here
	s.OnQosDropped(client, pk)
	_, err = s.db.HGet(s.ctx, s.hKey(storage.InflightKey), inflightKey(client, pk)).Result()
	require.Error(t, err)
	require.ErrorIs(t, err, redis.Nil)
}

func TestOnQosPublishNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnQosPublish(client, packets.Packet{}, time.Now().Unix(), 0)
}

func TestOnQosPublishClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnQosPublish(client, packets.Packet{}, time.Now().Unix(), 0)
}

func TestOnQosCompleteNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnQosComplete(client, packets.Packet{})
}

func TestOnQosCompleteClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnQosComplete(client, packets.Packet{})
}

func TestOnQosDroppedNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnQosDropped(client, packets.Packet{})
}

func TestOnSysInfoTick(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	info := &system.Info{
		Version:       "2.0.0",
		BytesReceived: 100,
	}

	s.OnSysInfoTick(info)

	r := new(storage.SystemInfo)
	row, err := s.db.HGet(s.ctx, s.hKey(storage.SysInfoKey), sysInfoKey()).Result()
	require.NoError(t, err)
	err = r.UnmarshalBinary([]byte(row))
	require.NoError(t, err)
	require.Equal(t, info.Version, r.Version)
	require.Equal(t, info.BytesReceived, r.BytesReceived)
	require.NotSame(t, info, r)
}

func TestOnSysInfoTickClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)
	s.OnSysInfoTick(new(system.Info))
}
func TestOnSysInfoTickNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	s.OnSysInfoTick(new(system.Info))
}

func TestStoredClientByCid(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	err := s.db.HSet(s.ctx, s.hKey(storage.ClientKey), "cl1", &storage.Client{ID: "cl1", T: storage.ClientKey}).Err()
	require.NoError(t, err)

	r, err := s.StoredClientByCid("cl1")
	require.NoError(t, err)
	require.Equal(t, "cl1", r.ID)
}

func TestStoredClientByCidNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	v, err := s.StoredClientByCid("cl1")
	require.Empty(t, v)
	require.NoError(t, err)
}

func TestStoredClientByCidClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)

	v, err := s.StoredClientByCid("cl1")
	require.Empty(t, v)
	require.Error(t, err)
}

func TestStoredSubscriptions(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	// populate with subscriptions
	err := s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, "cl1")), "sub1", &storage.Subscription{ID: "sub1", T: storage.SubscriptionKey}).Err()
	require.NoError(t, err)

	err = s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, "cl1")), "sub2", &storage.Subscription{ID: "sub2", T: storage.SubscriptionKey}).Err()
	require.NoError(t, err)

	err = s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.SubscriptionKey, "cl1")), "sub3", &storage.Subscription{ID: "sub3", T: storage.SubscriptionKey}).Err()
	require.NoError(t, err)

	r, err := s.StoredSubscriptionsByCid("cl1")
	require.NoError(t, err)
	require.Len(t, r, 3)
	sort.Slice(r[:], func(i, j int) bool { return r[i].ID < r[j].ID })
	require.Equal(t, "sub1", r[0].ID)
	require.Equal(t, "sub2", r[1].ID)
	require.Equal(t, "sub3", r[2].ID)
}

func TestStoredSubscriptionsByCidNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	v, err := s.StoredSubscriptionsByCid("cl1")
	require.Empty(t, v)
	require.NoError(t, err)
}

func TestStoredSubscriptionsByCidClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)

	v, err := s.StoredSubscriptionsByCid("cl1")
	require.Empty(t, v)
	require.Error(t, err)
}

func TestStoredRetainedMessageByTopic(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	err := s.db.HSet(s.ctx, s.hKey(storage.RetainedKey), "m1", &storage.Message{ID: "m1", T: storage.RetainedKey}).Err()
	require.NoError(t, err)

	r, err := s.StoredRetainedMessageByTopic("m1")
	require.NoError(t, err)
	require.Equal(t, "m1", r.ID)
}

func TestStoredRetainedMessageByTopicNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	v, err := s.StoredRetainedMessageByTopic("m1")
	require.Empty(t, v)
	require.NoError(t, err)
}

func TestStoredRetainedMessageByTopicClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)

	v, err := s.StoredRetainedMessageByTopic("m1")
	require.Empty(t, v)
	require.Error(t, err)
}

func TestStoredInflightMessages(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	// populate with messages
	err := s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.InflightKey, "cl1")), "i1", &storage.Message{ID: "i1", T: storage.InflightKey}).Err()
	require.NoError(t, err)

	err = s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.InflightKey, "cl1")), "i2", &storage.Message{ID: "i2", T: storage.InflightKey}).Err()
	require.NoError(t, err)

	err = s.db.HSet(s.ctx, s.hKey(utils.JoinStrings(storage.InflightKey, "cl1")), "i3", &storage.Message{ID: "i3", T: storage.InflightKey}).Err()
	require.NoError(t, err)

	r, err := s.StoredInflightMessagesByCid("cl1")
	require.NoError(t, err)
	require.Len(t, r, 3)
	sort.Slice(r[:], func(i, j int) bool { return r[i].ID < r[j].ID })
	require.Equal(t, "i1", r[0].ID)
	require.Equal(t, "i2", r[1].ID)
	require.Equal(t, "i3", r[2].ID)
}

func TestStoredInflightMessagesByCidNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	v, err := s.StoredInflightMessagesByCid("cl1")
	require.Empty(t, v)
	require.NoError(t, err)
}

func TestStoredInflightMessagesByCidClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)

	v, err := s.StoredInflightMessagesByCid("cl1")
	require.Empty(t, v)
	require.Error(t, err)
}

func TestStoredSysInfo(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	defer teardown(t, s)

	// populate with sys info
	err := s.db.HSet(s.ctx, s.hKey(storage.SysInfoKey), sysInfoKey(),
		&storage.SystemInfo{
			ID: storage.SysInfoKey,
			Info: system.Info{
				Version: "2.0.0",
			},
			T: storage.SysInfoKey,
		}).Err()
	require.NoError(t, err)

	r, err := s.StoredSysInfo()
	require.NoError(t, err)
	require.Equal(t, "2.0.0", r.Info.Version)
}

func TestStoredSysInfoNoDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	s.db = nil
	v, err := s.StoredSysInfo()
	require.Empty(t, v)
	require.NoError(t, err)
}

func TestStoredSysInfoClosedDB(t *testing.T) {
	m := miniredis.RunT(t)
	defer m.Close()
	s := newHook(t, m.Addr())
	teardown(t, s)

	v, err := s.StoredSysInfo()
	require.Empty(t, v)
	require.Error(t, err)
}
