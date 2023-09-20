package mysql

import (
	"io"
	"net"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/require"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/plugin"
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

	pkc = packets.Packet{Connect: packets.ConnectParams{Password: []byte("123456")}}
)

func teardown(a *Auth, t *testing.T) {
	if a.db != nil {
		a.Stop()
	}
}

func newAuth(t *testing.T) *Auth {
	a := new(Auth)
	a.SetOpts(logger, nil)

	err := a.Init(&Options{
		AuthMode: byte(auth.AuthUsername),
		AclMode:  byte(auth.AuthUsername),
		Dsn: DsnInfo{
			Host:          "localhost",
			Port:          3306,
			Schema:        "comqtt",
			Charset:       "utf8",
			LoginName:     "root",
			LoginPassword: "12345678",
		},
		Auth: AuthTable{
			Table:          "auth",
			UserColumn:     "username",
			PasswordColumn: "password",
			AllowColumn:    "allow",
			PasswordHash:   1,
			HashKey:        "",
		},
		Acl: AclTable{
			Table:        "acl",
			UserColumn:   "username",
			TopicColumn:  "topic",
			AccessColumn: "access",
		},
	})
	require.NoError(t, err)

	return a
}

func TestInitFromConfFile(t *testing.T) {
	if !hasMysql() {
		t.SkipNow()
	}
	a := new(Auth)
	a.SetOpts(logger, nil)
	opts := Options{}
	err := plugin.LoadYaml(path, &opts)
	require.NoError(t, err)

	err = a.Init(&opts)
	require.NoError(t, err)
}

func TestInitBadConfig(t *testing.T) {
	a := new(Auth)
	a.SetOpts(logger, nil)

	err := a.Init(map[string]any{})
	require.Error(t, err)
}

func TestOnConnectAuthenticate(t *testing.T) {
	if !hasMysql() {
		t.SkipNow()
	}
	a := newAuth(t)
	defer teardown(a, t)
	result := a.OnConnectAuthenticate(client, pkc)
	require.Equal(t, true, result)
}

func TestOnACLCheck(t *testing.T) {
	if !hasMysql() {
		t.SkipNow()
	}
	a := newAuth(t)
	defer teardown(a, t)
	topic := "topictest/1"
	topic2 := "topictest/2"
	result := a.OnACLCheck(client, topic, true) //publish
	require.Equal(t, true, result)
	result = a.OnACLCheck(client, topic, false) //subscribe
	require.Equal(t, false, result)
	result = a.OnACLCheck(client, topic2, true)
	require.Equal(t, false, result)
	result = a.OnACLCheck(client, topic2, false)
	require.Equal(t, false, result)
}

// hasMysql does a TCP connect to port 3306 to see if there is a MySQL server running on localhost.
func hasMysql() bool {
	c, err := net.Dial("tcp", "localhost:3306")
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
