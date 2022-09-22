package postgresql

import (
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
	a, _ := New(path)
	defer teardown(a, t)
	err := a.Open()
	require.NoError(t, err)
	require.NotNil(t, a.db)
}

func TestAuthenticate(t *testing.T) {
	a, _ := New(path)
	defer teardown(a, t)
	err := a.Open()
	require.NoError(t, err)
	user := "zhangsan"
	password := "321654"
	result := a.Authenticate([]byte(user), []byte(password))
	require.Equal(t, true, result)
}

func TestACL(t *testing.T) {
	a, _ := New(path)
	defer teardown(a, t)
	err := a.Open()
	require.NoError(t, err)
	user := "zhangsan"
	topic := "topictest/1"
	topic2 := "topictest/2"
	result := a.ACL([]byte(user), topic, true) //publish
	require.Equal(t, true, result)
	result = a.ACL([]byte(user), topic, false) //subscribe
	require.Equal(t, false, result)
	result = a.ACL([]byte(user), topic2, true)
	require.Equal(t, false, result)
	result = a.ACL([]byte(user), topic2, false)
	require.Equal(t, false, result)
}
