package http

import (
	"encoding/json"
	"io"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/plugin"
	"gopkg.in/h2non/gock.v1"
)

const path = "./conf.yml"

var (
	// Currently, the input is directed to /dev/null. If you need to
	// output to stdout, just modify 'io.Discard' here to 'os.Stdout'.
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	client = &mqtt.Client{
		ID: "test",
		Net: mqtt.ClientConnection{
			Remote:   "test.addr",
			Listener: "listener",
		},
		Properties: mqtt.ClientProperties{
			Username: []byte("zhangsan"),
			Clean:    false,
		},
	}

	//pkf = packets.Packet{Filters: packets.Subscriptions{{Filter: "a/b/c"}}}

	pkc = packets.Packet{Connect: packets.ConnectParams{Password: []byte("321654")}}
)

func newAuth(t *testing.T) *Auth {
	a := new(Auth)
	a.SetOpts(logger, nil)

	err := a.Init(&Options{
		AuthMode:    byte(auth.AuthUsername),
		AclMode:     byte(auth.AuthUsername),
		Method:      "post",
		ContentType: "application/json",
		AuthUrl:     "http://localhost:8080/comqtt/auth",
		AclUrl:      "http://localhost:8080/comqtt/acl",
	})
	require.NoError(t, err)

	return a
}

func TestInitFromConfFile(t *testing.T) {
	a := new(Auth)
	a.SetOpts(logger, nil)
	opts := Options{}
	err := plugin.LoadYaml(path, &opts)
	require.NoError(t, err)

	err = a.Init(&opts)
	require.NoError(t, err)
}

func TestAuthenticateWithPost(t *testing.T) {
	a := newAuth(t)
	user := "zhangsan"
	password := "321654"
	defer gock.Off() // Flush pending mocks after test execution
	gock.New("http://localhost:8080").
		Post("/comqtt/auth").
		JSON(map[string]string{"user": user, "password": password}).
		Reply(200).BodyString("1")
	result := a.OnConnectAuthenticate(client, pkc)
	require.Equal(t, true, result)
}

func TestAuthenticateWithGet(t *testing.T) {
	a := newAuth(t)
	a.config.Method = "get"
	user := "zhangsan"
	password := "321654"

	defer gock.Off() // Flush pending mocks after test execution
	gock.New("http://localhost:8080").
		Get("/comqtt/auth").
		MatchParam("user", user).
		MatchParam("password", password).
		Reply(200).BodyString("1")

	result := a.OnConnectAuthenticate(client, pkc)
	require.Equal(t, true, result)
}

func TestAclWithPost(t *testing.T) {
	a := newAuth(t)
	user := "zhangsan"
	topic1 := "topictest/1"
	topic2 := "topictest/2"

	payload := map[string]int{
		topic1: 3,
	}
	body, _ := json.Marshal(payload)
	defer gock.Off() // Flush pending mocks after test execution

	//publish
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user}).
		Reply(200).BodyString(string(body))
	result := a.OnACLCheck(client, topic1, true)
	require.Equal(t, true, result)

	//subscribe
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user}).
		Reply(200).BodyString(string(body))
	result = a.OnACLCheck(client, topic1, false)
	require.Equal(t, true, result)

	//publish topic2, topic2 does not exist
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user}).
		Reply(200).BodyString(string(body))
	result = a.OnACLCheck(client, topic2, true)
	require.Equal(t, false, result)

	//subscribe topic2, topic2 does not exist
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user, "topic": topic2}).
		Reply(200).BodyString(string(body))
	result = a.OnACLCheck(client, topic2, false)
	require.Equal(t, false, result)

	//pubsub
	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user}).
		Reply(200).BodyString(string(body))
	result = a.OnACLCheck(client, topic2, true)
	require.Equal(t, false, result)

	gock.New("http://localhost:8080").
		Post("/comqtt/acl").
		JSON(map[string]string{"user": user}).
		Reply(200).BodyString(string(body))
	result = a.OnACLCheck(client, topic2, false)
	require.Equal(t, false, result)
}

func TestAclWithGet(t *testing.T) {
	a := newAuth(t)
	a.config.Method = "get"
	user := "zhangsan"
	topic1 := "topictest/1"
	topic2 := "topictest/2"
	topic3 := "topictest/3"

	defer gock.Off() // Flush pending mocks after test execution

	// make a body that will be injected. It's JSON of map[string]int
	resp := map[string]int{
		topic1: 2,
		topic2: 3,
	}
	// make it into json:
	body, _ := json.Marshal(resp)

	//publish
	gock.New("http://localhost:8080").
		Get("/comqtt/acl").
		MatchParam("user", user).
		Reply(200).BodyString(string(body))
	result := a.OnACLCheck(client, topic1, true)
	require.Equal(t, true, result)

	//subscribe
	gock.New("http://localhost:8080").
		Get("/comqtt/acl").
		MatchParam("user", user).
		// MatchParam("topic", topic1).
		Reply(200).BodyString(string(body))
	result = a.OnACLCheck(client, topic2, false)
	require.Equal(t, true, result)

	//publish topic3, topic3 does not exist
	gock.New("http://localhost:8080").
		Get("/comqtt/acl").
		MatchParam("user", user).
		// MatchParam("topic1", topic1).
		Reply(200).BodyString(string(body))
	result = a.OnACLCheck(client, topic3, true)
	require.Equal(t, false, result)
}
