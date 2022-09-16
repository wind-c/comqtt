package http

import (
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
	"testing"
)

const path = "./conf.yml"

func TestAuthenticateWithPost(t *testing.T) {
	a, _ := New(path)
	a.conf.Method = "post"
	user := "zhangsan"
	password := "127834"

	defer gock.Off() // Flush pending mocks after test execution
	gock.New("http://localhost:8080").
		Post("/comqtt/auth").
		JSON(map[string]string{"user": user, "password": password}).
		Reply(200).BodyString(a.conf.AuthSuccess)
	result := a.Authenticate([]byte(user), []byte(password))
	require.Equal(t, true, result)
}

func TestAuthenticateWithGet(t *testing.T) {
	a, _ := New(path)
	a.conf.Method = "get"
	user := "zhangsan"
	password := "127834"

	defer gock.Off() // Flush pending mocks after test execution
	gock.New("http://localhost:8080").
		Get("/comqtt/auth").
		MatchParam("user", user).
		MatchParam("password", password).
		Reply(200).BodyString(a.conf.AuthSuccess)

	result := a.Authenticate([]byte(user), []byte(password))
	require.Equal(t, true, result)
}

func TestAclWithPost(t *testing.T) {
	a, _ := New(path)
	a.conf.Method = "post"
	user := "zhangsan"
	topic := "topictest/1"
	topic2 := "topictest/2"
	defer gock.Off() // Flush pending mocks after test execution

	//publish
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user, "topic": topic}).
		Reply(200).BodyString(a.conf.AuthSuccess)
	result := a.ACL([]byte(user), topic, true)
	require.Equal(t, true, result)

	//subscribe
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user, "topic": topic}).
		Reply(200).BodyString(a.conf.AuthSuccess)
	result = a.ACL([]byte(user), topic, false)
	require.Equal(t, false, result)

	//publish topic2, topic2 does not exist
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user, "topic": topic}).
		Reply(200).BodyString(a.conf.AuthSuccess)
	result = a.ACL([]byte(user), topic2, true)
	require.Equal(t, false, result)

	//subscribe topic2, topic2 does not exist
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user, "topic": topic}).
		Reply(200).BodyString(a.conf.AuthSuccess)
	result = a.ACL([]byte(user), topic2, false)
	require.Equal(t, false, result)

	//pubsub
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user, "topic": topic2}).
		Reply(200).BodyString(a.conf.AclPubSub)
	result = a.ACL([]byte(user), topic2, true)
	require.Equal(t, true, result)

	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user, "topic": topic2}).
		Reply(200).BodyString(a.conf.AclPubSub)
	result = a.ACL([]byte(user), topic2, false)
	require.Equal(t, true, result)
}

func TestAclWithGet(t *testing.T) {
	a, _ := New(path)
	a.conf.Method = "get"
	user := "zhangsan"
	topic := "topictest/1"
	topic2 := "topictest/2"

	defer gock.Off() // Flush pending mocks after test execution

	//publish
	gock.New("http://localhost:8080").
		Get("/comqtt/acl").
		MatchParam("user", user).
		MatchParam("topic", topic).
		Reply(200).BodyString(a.conf.AclPublish)
	result := a.ACL([]byte(user), topic, true)
	require.Equal(t, true, result)

	//subscribe
	gock.New("http://localhost:8080").
		Get("/comqtt/acl").
		MatchParam("user", user).
		MatchParam("topic", topic).
		Reply(200).BodyString(a.conf.AclPublish)
	result = a.ACL([]byte(user), topic, false)
	require.Equal(t, false, result)

	//publish topic2, topic2 does not exist
	gock.New("http://localhost:8080").
		Get("/comqtt/acl").
		MatchParam("user", user).
		MatchParam("topic", topic).
		Reply(200).BodyString(a.conf.AclPublish)
	result = a.ACL([]byte(user), topic2, true)
	require.Equal(t, false, result)
}
