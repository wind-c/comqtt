// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package packets

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildPropertySection returns a property section body (varint length prefix
// included) made of n repeated units of the given kind.
func buildPropertySection(kind byte, n int) []byte {
	body := new(bytes.Buffer)
	unit := new(bytes.Buffer)
	switch kind {
	case PropUser:
		unit.Write([]byte{PropUser, 0x00, 0x01, 'k', 0x00, 0x01, 'v'})
	case PropSubscriptionIdentifier:
		unit.Write([]byte{PropSubscriptionIdentifier, 0x01})
	}
	for i := 0; i < n; i++ {
		body.Write(unit.Bytes())
	}

	out := new(bytes.Buffer)
	encodeLength(out, int64(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// A CONNECT property section must not be able to grow the User slice without
// bound: a ~10 MB section of two-byte properties previously produced ~46 MB of
// slice, duplicated again by Properties.Copy, from a single unauthenticated
// packet.
func TestPropertiesDecodeUserPropertyLimit(t *testing.T) {
	p := new(Properties)
	_, err := p.Decode(Connect, bytes.NewBuffer(buildPropertySection(PropUser, MaxUserProperties)))
	require.NoError(t, err)
	require.Len(t, p.User, MaxUserProperties)
}

func TestPropertiesDecodeUserPropertyLimitExceeded(t *testing.T) {
	p := new(Properties)
	_, err := p.Decode(Connect, bytes.NewBuffer(buildPropertySection(PropUser, MaxUserProperties+1)))
	require.ErrorIs(t, err, ErrMalformedTooManyUserProperties)
	require.LessOrEqual(t, len(p.User), MaxUserProperties)
}

// The amplification vector from the report: 1,428,571 short User Properties.
func TestPropertiesDecodeUserPropertyFlood(t *testing.T) {
	const flood = 1428571

	section := buildPropertySection(PropUser, flood)
	require.Greater(t, len(section), 9_000_000, "payload should be ~10 MB on the wire")

	p := new(Properties)
	_, err := p.Decode(Connect, bytes.NewBuffer(section))
	require.ErrorIs(t, err, ErrMalformedTooManyUserProperties)

	// The point of the cap: allocation stays bounded no matter the input size.
	require.LessOrEqual(t, len(p.User), MaxUserProperties)

	// Copy() must not be able to re-amplify what Decode did not accept.
	c := p.Copy(false)
	require.LessOrEqual(t, len(c.User), MaxUserProperties)
}

func TestPropertiesDecodeSubscriptionIdentifierLimit(t *testing.T) {
	p := new(Properties)
	_, err := p.Decode(Publish, bytes.NewBuffer(buildPropertySection(PropSubscriptionIdentifier, MaxSubscriptionIdentifiers)))
	require.NoError(t, err)
	require.Len(t, p.SubscriptionIdentifier, MaxSubscriptionIdentifiers)
}

func TestPropertiesDecodeSubscriptionIdentifierLimitExceeded(t *testing.T) {
	p := new(Properties)
	_, err := p.Decode(Publish, bytes.NewBuffer(buildPropertySection(PropSubscriptionIdentifier, MaxSubscriptionIdentifiers+1)))
	require.ErrorIs(t, err, ErrMalformedTooManySubscriptionIdentifiers)
	require.LessOrEqual(t, len(p.SubscriptionIdentifier), MaxSubscriptionIdentifiers)
}

// MQTT v5 permits one Subscription Identifier per filter in a SUBSCRIBE
// packet, so a conforming client needs as many as it has filters. Regression
// guard: an earlier revision capped this at 128 and rejected legal
// 129-filter SUBSCRIBEs.
func TestPropertiesDecodeSubscriptionIdentifiersForManyFilters(t *testing.T) {
	const filters = 512 // well beyond any realistic client, still legal

	p := new(Properties)
	_, err := p.Decode(Subscribe, bytes.NewBuffer(buildPropertySection(PropSubscriptionIdentifier, filters)))
	require.NoError(t, err)
	require.Len(t, p.SubscriptionIdentifier, filters)
}

// The cap must be reported as a malformed-properties error so callers that
// already wrap Decode() errors map it onto the right CONNACK reason code.
func TestPropertiesDecodeLimitErrorIsMalformedProperties(t *testing.T) {
	p := new(Properties)
	_, err := p.Decode(Connect, bytes.NewBuffer(buildPropertySection(PropUser, MaxUserProperties+1)))
	require.Error(t, err)
	require.Equal(t, byte(0x81), err.(Code).Code)
	require.True(t, strings.Contains(err.(Code).Reason, "too many user properties"))
}

func TestPropertiesDecodeUserPropertiesRoundTrip(t *testing.T) {
	// Legitimate multi-property packets must still encode and decode unchanged.
	orig := new(Properties)
	for i := 0; i < 8; i++ {
		orig.User = append(orig.User, UserProperty{
			Key: fmt.Sprintf("k%d", i),
			Val: fmt.Sprintf("v%d", i),
		})
	}

	b := new(bytes.Buffer)
	orig.Encode(Connect, Mods{}, b, 0)

	got := new(Properties)
	_, err := got.Decode(Connect, bytes.NewBuffer(b.Bytes()))
	require.NoError(t, err)
	require.Equal(t, orig.User, got.User)
}
