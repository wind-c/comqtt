// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package mqtt

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"gopkg.in/yaml.v3"
)

// decodeOptions parses a YAML document shaped like the `mqtt.options` block of
// comqtt's config file and returns the Options it describes.
func decodeOptions(t *testing.T, doc string) *Options {
	t.Helper()
	var wrapper struct {
		Options Options `yaml:"options"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(doc), &wrapper))
	return &wrapper.Options
}

// The core bug: a partial `capabilities:` block used to zero every capability
// it did not mention, because yaml allocated the struct and New() then skipped
// DefaultServerCapabilities.
func TestCapabilitiesPartialBlockKeepsDefaults(t *testing.T) {
	opts := decodeOptions(t, `
options:
  capabilities:
    receive-maximum: 1
`)

	require.NotNil(t, opts.Capabilities)
	require.Equal(t, uint16(1), opts.Capabilities.ReceiveMaximum) // from the config
	require.Equal(t, int64(86400), opts.Capabilities.MaximumMessageExpiryInterval)
	require.Equal(t, uint32(4294967295), opts.Capabilities.MaximumSessionExpiryInterval)
	require.Equal(t, byte(2), opts.Capabilities.MaximumQos)
	require.Equal(t, byte(1), opts.Capabilities.RetainAvailable)
	require.Equal(t, uint16(65535), opts.Capabilities.TopicAliasMaximum)
	require.Equal(t, byte(3), opts.Capabilities.MinimumProtocolVersion)
}

// The specific damage the partial block caused: a zero expiry interval makes
// ClearInflights match every queued message on the next tick.
func TestCapabilitiesPartialBlockDoesNotZeroMessageExpiry(t *testing.T) {
	opts := decodeOptions(t, `
options:
  capabilities:
    receive-maximum: 1
`)

	require.NotZero(t, opts.Capabilities.MaximumMessageExpiryInterval,
		"zero here would silently delete the whole offline queue each second")

	now := int64(1_700_000_000)
	cl, _, _ := newTestClient()
	cl.State.Inflight.Set(packets.Packet{PacketID: 1, Created: now - 10})

	cl.ClearInflights(now, opts.Capabilities.MaximumMessageExpiryInterval)
	require.Equal(t, 1, cl.State.Inflight.Len(),
		"a recently queued message must not be swept by the expiry tick")
}

// Zero is a legitimate value for several capabilities and must survive the
// merge -- otherwise the fix would reintroduce the bug in the other direction.
func TestCapabilitiesExplicitZeroIsPreserved(t *testing.T) {
	opts := decodeOptions(t, `
options:
  capabilities:
    maximum-packet-size: 0
    retain-available: 0
    maximum-qos: 0
    topic-alias-maximum: 0
    receive-maximum: 0
`)

	require.Equal(t, uint32(0), opts.Capabilities.MaximumPacketSize, "0 means unlimited")
	require.Equal(t, byte(0), opts.Capabilities.RetainAvailable)
	require.Equal(t, byte(0), opts.Capabilities.MaximumQos)
	require.Equal(t, uint16(0), opts.Capabilities.TopicAliasMaximum)
	// The default is 1024, so this proves the explicit zero was kept rather
	// than replaced by the default.
	require.Equal(t, uint16(0), opts.Capabilities.ReceiveMaximum)
	// ...while omitted keys still fall back to the defaults
	require.Equal(t, int64(86400), opts.Capabilities.MaximumMessageExpiryInterval)
}

// Non-zero overrides still win, and compatibilities sub-block still decodes.
func TestCapabilitiesFullBlockOverridesEverything(t *testing.T) {
	opts := decodeOptions(t, `
options:
  capabilities:
    compatibilities:
      obscure-not-authorized: true
      passive-client-disconnect: true
    maximum-message-expiry-interval: 120
    receive-maximum: 32
    maximum-qos: 1
`)

	require.True(t, opts.Capabilities.Compatibilities.ObscureNotAuthorized)
	require.True(t, opts.Capabilities.Compatibilities.PassiveClientDisconnect)
	require.False(t, opts.Capabilities.Compatibilities.AlwaysReturnResponseInfo)
	require.Equal(t, int64(120), opts.Capabilities.MaximumMessageExpiryInterval)
	require.Equal(t, uint16(32), opts.Capabilities.ReceiveMaximum)
	require.Equal(t, byte(1), opts.Capabilities.MaximumQos)
	require.Equal(t, uint16(65535), opts.Capabilities.TopicAliasMaximum) // default kept
}

// No capabilities block at all must leave the pointer nil so New() applies the
// package defaults, exactly as before.
func TestCapabilitiesAbsentBlockStaysNil(t *testing.T) {
	opts := decodeOptions(t, `
options:
  inline-client: true
`)

	require.Nil(t, opts.Capabilities)

	s := New(opts)
	require.Equal(t, int64(86400), s.Options.Capabilities.MaximumMessageExpiryInterval)
	require.Equal(t, byte(2), s.Options.Capabilities.MaximumQos)
	require.Equal(t, uint32(65535), s.Options.Capabilities.maximumPacketID)
}

// New() must not hand the package-level default to callers: it mutates
// maximumPacketID right after, which would otherwise write through to the
// global shared by every other server in the process.
func TestNewDoesNotShareDefaultCapabilities(t *testing.T) {
	a := New(nil)
	b := New(nil)

	require.NotSame(t, a.Options.Capabilities, b.Options.Capabilities)
	require.NotSame(t, DefaultServerCapabilities, a.Options.Capabilities)

	a.Options.Capabilities.MaximumQos = 1
	require.Equal(t, byte(2), b.Options.Capabilities.MaximumQos, "servers must not share state")
	require.Equal(t, byte(2), DefaultServerCapabilities.MaximumQos, "the package default must not be mutated")
}
