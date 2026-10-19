// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package mqtt

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"gopkg.in/yaml.v3"
)

func TestTopicFilterDepth(t *testing.T) {
	tests := []struct {
		filter string
		want   int
	}{
		{"#", 1},
		{"+", 1},
		{"a", 1},
		{"a/b", 2},
		{"a/b/c", 3},
		{"a/#", 2},
		{"a/+/c", 3},
		{"/a/b", 3},
		{"", 0},
		// the "$share/<group>/" prefix is stripped before the trie walk, so it
		// adds no nodes and must not count towards the limit
		{"$share/g/a/b", 2},
		{"$share/g/a/b/c", 3},
		{"$SHARE/g/a", 1},
		{"$share/g/", 0},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, TopicFilterDepth(tc.filter), "filter %q", tc.filter)
	}
}

// subscribeWithFilter runs processSubscribe for a single filter and returns the
// SUBACK reason code the client would have seen. The broker writes to the
// server end of the pipe, so we read from r; net.Pipe is unbuffered, hence the
// goroutine.
func subscribeWithFilter(t *testing.T, s *Server, cl *Client, r net.Conn, filter string) byte {
	t.Helper()

	pk := *packets.TPacketData[packets.Subscribe].Get(packets.TSubscribe).Packet
	pk.Filters = []packets.Subscription{{Filter: filter, Qos: 0}}

	go func() { _ = s.processSubscribe(cl, pk) }()

	buf := make([]byte, 64)
	require.NoError(t, r.SetReadDeadline(time.Now().Add(3*time.Second)))
	n, err := r.Read(buf)
	require.NoError(t, err)
	require.Greater(t, n, 4, "expected a SUBACK")
	require.Equal(t, byte(packets.Suback), buf[0]>>4)
	return buf[n-1]
}

func TestServerRejectsTooDeepFilter(t *testing.T) {
	s := newServer()
	cl, r, _ := newTestClient()
	cl.Properties.ProtocolVersion = 5

	deep := "u000/" + strings.Repeat("a/", 32) + "a" // 34 levels, over the limit of 8
	require.Greater(t, TopicFilterDepth(deep), 8)

	code := subscribeWithFilter(t, s, cl, r, deep)
	require.Equal(t, packets.ErrTopicFilterInvalid.Code, code, "an over-deep filter must be refused")

	// and it must not have been inserted
	require.Empty(t, s.Topics.Subscribers(deep).Subscriptions,
		"a refused filter must not allocate trie nodes")
}

func TestServerAcceptsFilterAtLimit(t *testing.T) {
	s := newServer()
	cl, r, _ := newTestClient()
	cl.Properties.ProtocolVersion = 5

	filter := "a/b/c/d/e/f/g/h" // exactly 8
	require.Equal(t, 8, TopicFilterDepth(filter))

	code := subscribeWithFilter(t, s, cl, r, filter)
	require.LessOrEqual(t, code, packets.CodeGrantedQos2.Code, "a filter at the limit must be accepted")
	require.Len(t, s.Topics.Subscribers(filter).Subscriptions, 1)
}

func TestServerAcceptsFilterOverDefaultWhenConfigured(t *testing.T) {
	s := newServer()
	s.Options.Capabilities.MaximumTopicFilterDepth = 12
	cl, r, _ := newTestClient()
	cl.Properties.ProtocolVersion = 5

	filter := "a/b/c/d/e/f/g/h/i/j/k" // 11, over the default 8
	code := subscribeWithFilter(t, s, cl, r, filter)
	require.LessOrEqual(t, code, packets.CodeGrantedQos2.Code, "the limit must be configurable")
	require.Len(t, s.Topics.Subscribers(filter).Subscriptions, 1)
}

func TestServerRejectsFilterOverConfiguredLimit(t *testing.T) {
	s := newServer()
	s.Options.Capabilities.MaximumTopicFilterDepth = 4
	cl, r, _ := newTestClient()
	cl.Properties.ProtocolVersion = 5

	code := subscribeWithFilter(t, s, cl, r, "a/b/c/d/e")
	require.Equal(t, packets.ErrTopicFilterInvalid.Code, code)
}

// A Capabilities struct built without the new field must not reject everything.
func TestMaxTopicFilterDepthZeroFallsBackToDefault(t *testing.T) {
	var c Capabilities
	require.Equal(t, int(DefaultServerCapabilities.MaximumTopicFilterDepth), c.maxTopicFilterDepth())
	require.Equal(t, 8, c.maxTopicFilterDepth())

	c.MaximumTopicFilterDepth = 32
	require.Equal(t, 32, c.maxTopicFilterDepth())
}

func TestMaximumTopicFilterDepthDefaultIs8(t *testing.T) {
	require.Equal(t, uint16(8), DefaultServerCapabilities.MaximumTopicFilterDepth)
}

// A partial capabilities block leaves the raw field at zero. maxTopicFilterDepth
// must still yield the default so subscriptions keep working; without the
// fallback a partial config would refuse every SUBSCRIBE. (The zeroing itself
// is tracked separately -- see the capabilities default-merge change.)
func TestMaximumTopicFilterDepthSurvivesPartialConfig(t *testing.T) {
	var wrapper struct {
		Options Options `yaml:"options"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(`
options:
  capabilities:
    receive-maximum: 1
`), &wrapper))

	caps := wrapper.Options.Capabilities
	require.NotNil(t, caps)
	require.Equal(t, uint16(0), caps.MaximumTopicFilterDepth, "precondition: raw field is zeroed")
	require.Equal(t, 8, caps.maxTopicFilterDepth(),
		"the effective limit must fall back to the default, not refuse every filter")
}