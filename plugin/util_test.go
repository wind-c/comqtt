package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchTopic(t *testing.T) {
	require.True(t, MatchTopic("a/+/c/+", "a/b/c/d"))
	require.True(t, MatchTopic("stuff/#", "stuff/things/yeah"))
	require.True(t, MatchTopic("test", "test"))
	require.True(t, MatchTopic("a/#", "a"))
	require.False(t, MatchTopic("test", "test/child"))
	require.False(t, MatchTopic("a/+/c/+", "a/b/c/d/e"))
	require.False(t, MatchTopic("a/+", "a"))
}
