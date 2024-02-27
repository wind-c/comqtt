package raft

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKV_Add(t *testing.T) {
	kv := NewKV()

	new := kv.Add("key1", "value1")
	require.Equal(t, true, new)
	new = kv.Add("key1", "value2")
	require.Equal(t, false, new)
	ln := len(kv.Get("key1"))
	require.Equal(t, 2, ln)

	new = kv.Add("key2", "value1")
	ln = len(kv.Get("key1"))
	require.Equal(t, true, new)
	require.Equal(t, 2, ln)
}

func TestKV_Del(t *testing.T) {
	kv := NewKV()

	kv.Add("key1", "value1")
	kv.Add("key1", "value2")
	ln := len(kv.Get("key1"))
	require.Equal(t, 2, ln)

	empty := kv.Del("key1", "value3")
	require.Equal(t, false, empty)
	empty = kv.Del("key1", "value2")
	require.Equal(t, false, empty)
	empty = kv.Del("key1", "value1")
	require.Equal(t, true, empty)
}

func TestKV_DelByValue(t *testing.T) {
	kv := NewKV()

	// Test for value exists in multiple keys
	kv.Add("key1", "value1")
	kv.Add("key2", "value1")
	kv.Add("key3", "value1")
	c := kv.DelByValue("value1")
	require.Equal(t, 3, c)
	vs := kv.Get("key1")
	require.Empty(t, nil, vs)
	ln := len(kv.data)
	require.Equal(t, 0, ln)

	kv.Add("key4", "value4")
	kv.Add("key5", "value5")
	kv.DelByValue("value4")
	ln = len(kv.data)
	require.Equal(t, 1, ln)
	vs = kv.Get("key5")
	require.EqualValues(t, []string{"value5"}, vs)
}
