package raft

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestKV_Restore(t *testing.T) {
	src := NewKV()
	src.Add("key1", "value1")
	src.Add("key1", "value2")
	src.Add("key2", "value3")

	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(src.GetAll()))

	dst := NewKV()
	dst.Add("stale", "valueX") // a snapshot holds the complete state, so this must be discarded
	require.NoError(t, dst.Restore(&buf))

	require.ElementsMatch(t, []string{"value1", "value2"}, dst.Get("key1"))
	require.ElementsMatch(t, []string{"value3"}, dst.Get("key2"))
	require.Empty(t, dst.Get("stale"))

	// the restored map must not share slices with the source
	dst.Add("key1", "value4")
	require.ElementsMatch(t, []string{"value1", "value2"}, src.Get("key1"))
}

func TestKV_RestoreCorrupt(t *testing.T) {
	src := NewKV()
	src.Add("key1", "value1")
	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(src.GetAll()))

	kv := NewKV()
	kv.Add("key2", "value2")
	require.Error(t, kv.Restore(bytes.NewReader(buf.Bytes()[:buf.Len()/2])))
	// a failed restore leaves the current state untouched
	require.ElementsMatch(t, []string{"value2"}, kv.Get("key2"))
	require.Empty(t, kv.Get("key1"))
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
