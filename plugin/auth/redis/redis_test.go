package redis

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"testing"
)

const path = "./conf.yml"

func teardown(a *Auth, t *testing.T) {
	a.Close()
}

func TestNew(t *testing.T) {
	a, _ := New(path)
	require.NotNil(t, a)
}

func TestOpen(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	a, _ := New(path)
	err := a.Open()
	require.NoError(t, err)
	defer teardown(a, t)
	require.NotNil(t, a.db)
}

func TestAuthenticate(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	a, _ := New(path)
	err := a.Open()
	defer teardown(a, t)
	require.NoError(t, err)
	username := "zhangsan"
	password := "127834"
	err = a.db.HSet(context.Background(), a.conf.AuthPrefix+username, "password", password).Err()
	require.NoError(t, err)
	result := a.Authenticate([]byte(username), []byte(password))
	require.Equal(t, true, result)
}

func TestACL(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	a, _ := New(path)
	err := a.Open()
	defer teardown(a, t)
	require.NoError(t, err)
	username := "zhangsan"
	topic := "topictest/1"
	err = a.db.HSet(context.Background(), a.conf.AclPrefix+username, topic, 1).Err()
	require.NoError(t, err)
	result := a.ACL([]byte(username), topic, true)
	require.Equal(t, true, result)
	result = a.ACL([]byte(username), topic, false)
	require.Equal(t, false, result)
	result = a.ACL([]byte(username), "topictest/2", true)
	require.Equal(t, false, result)
	result = a.ACL([]byte(username), "topictest/2", false)
	require.Equal(t, false, result)
}
