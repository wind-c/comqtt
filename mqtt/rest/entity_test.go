// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package rest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetainedMsgJSON(t *testing.T) {
	msg := RetainedMsg{
		Topic:    "devices/status",
		Payload:  `{"online":true}`,
		Qos:      1,
		ClientID: "sensor-01",
		Created:  1700000000,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var out RetainedMsg
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "devices/status", out.Topic)
	require.Equal(t, `{"online":true}`, out.Payload)
	require.Equal(t, byte(1), out.Qos)
	require.Equal(t, "sensor-01", out.ClientID)
	require.Equal(t, int64(1700000000), out.Created)
}

func TestClientJSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := client{
		ID:              "test-001",
		IP:              "10.0.0.1:4321",
		Online:          true,
		Username:        "user1",
		TopicFilters:    []string{"sensors/#", "devices/+/status"},
		ProtocolVersion: 5,
		SessionClean:    true,
		ConnectedAt:     now,
		Keepalive:       30,
		InflightCount:   2,
	}

	data, err := json.Marshal(c)
	require.NoError(t, err)
	require.Contains(t, string(data), `"test-001"`)
	require.Contains(t, string(data), `"sensors/#",`)
	require.Contains(t, string(data), `"keepalive":30`)
	require.Contains(t, string(data), `"connected_at"`)

	var out client
	require.NoError(t, json.Unmarshal(data, &out))
	require.Equal(t, "test-001", out.ID)
	require.True(t, out.Online)
	require.Equal(t, uint16(30), out.Keepalive)
	require.Len(t, out.TopicFilters, 2)
}
