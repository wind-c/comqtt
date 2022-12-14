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
	a.conf.Addr = mr.Addr()
	err := a.Open()
	require.NoError(t, err)
	defer teardown(a, t)
	require.NotNil(t, a.db)
}

func TestAuthenticate(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	a, _ := New(path)
	a.conf.Addr = mr.Addr()
	err := a.Open()
	defer teardown(a, t)
	require.NoError(t, err)
	user := "zhangsan"
	password := "127834"
	err = a.db.HSet(context.Background(), a.conf.AuthPrefix+user, "password", password).Err()
	require.NoError(t, err)
	result := a.Authenticate([]byte(user), []byte(password))
	require.Equal(t, true, result)
}

func TestACL(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	a, _ := New(path)
	a.conf.Addr = mr.Addr()
	err := a.Open()
	defer teardown(a, t)
	require.NoError(t, err)
	user := "zhangsan"
	topic := "topictest/1"
	topic2 := "topictest/2"
	err = a.db.HSet(context.Background(), a.conf.AclPrefix+user, topic, a.conf.AclPublish).Err()
	require.NoError(t, err)
	result := a.ACL([]byte(user), topic, true) //publish
	require.Equal(t, true, result)
	result = a.ACL([]byte(user), topic, false) //subscribe
	require.Equal(t, false, result)
	result = a.ACL([]byte(user), topic2, true)
	require.Equal(t, false, result)
	result = a.ACL([]byte(user), topic2, false)
	require.Equal(t, false, result)

	err = a.db.HSet(context.Background(), a.conf.AclPrefix+user, topic2, a.conf.AclPubSub).Err()
	result = a.ACL([]byte(user), topic2, true) //publish
	require.Equal(t, true, result)
	result = a.ACL([]byte(user), topic2, false) //subscribe
	require.Equal(t, true, result)
}
