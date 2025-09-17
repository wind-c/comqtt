package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/plugin"
	pa "github.com/wind-c/comqtt/v2/plugin/auth"
)

const (
	TypeJson = "application/json"
	TypeForm = "application/x-www-form-urlencoded"
)

type Options struct {
	pa.Blacklist
	AuthMode    byte   `json:"auth-mode" yaml:"auth-mode"`
	AclMode     byte   `json:"acl-mode" yaml:"acl-mode"`
	TlsEnable   bool   `json:"tls-enable" yaml:"tls-enable"`
	TlsCert     string `json:"tls-cert" yaml:"tls-cert"`
	TlsKey      string `json:"tls-key" yaml:"tls-key"`
	Method      string `json:"method" yaml:"method"`
	ContentType string `json:"content-type" yaml:"content-type"`
	AuthUrl     string `json:"auth-url" yaml:"auth-url"`
	AclUrl      string `json:"acl-url" yaml:"acl-url"`
}

// Auth is an auth controller which allows access to all connections and topics.
type Auth struct {
	mqtt.HookBase
	config *Options
}

// ID returns the ID of the hook.
func (a *Auth) ID() string {
	return "auth-http"
}

// Provides indicates which hook methods this hook provides.
func (a *Auth) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnectAuthenticate,
		mqtt.OnACLCheck,
	}, []byte{b})
}

func (a *Auth) Init(config any) error {
	if _, ok := config.(*Options); !ok || config == nil {
		return mqtt.ErrInvalidConfigType
	}

	a.config = config.(*Options)
	a.Log.Info("", "auth-url", a.config.AuthUrl, "acl-url", a.config.AclUrl)

	return nil
}

// OnConnectAuthenticate returns true if the connecting client has rules which provide access
// in the auth ledger.
func (a *Auth) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	if a.config.AuthMode == byte(auth.AuthAnonymous) {
		return true
	}

	// check blacklist
	if n, ok := a.config.CheckBLAuth(cl, pk); n >= 0 { // It's on the blacklist
		return ok
	}

	// normal verification
	var key string
	if a.config.AuthMode == byte(auth.AuthUsername) {
		key = string(cl.Properties.Username)
	} else if a.config.AuthMode == byte(auth.AuthClientID) {
		key = cl.ID
	} else {
		return false
	}

	var err error
	var resp *http.Response
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	if a.config.Method == "get" {
		var builder strings.Builder
		builder.WriteString(a.config.AuthUrl)
		builder.WriteString("?")
		builder.WriteString("user=")
		builder.WriteString(key)
		builder.WriteString("&")
		builder.WriteString("password=")
		builder.Write(pk.Connect.Password)
		resp, err = http.Get(builder.String())
	} else {
		if a.config.ContentType == TypeJson {
			payload := make(map[string]string, 2)
			payload["user"] = key
			payload["password"] = string(pk.Connect.Password)
			bytesData, _ := json.Marshal(payload)
			resp, err = http.Post(a.config.AuthUrl, TypeJson, bytes.NewBuffer(bytesData))
		} else {
			payload := url.Values{}
			payload.Add("user", key)
			payload.Add("password", string(pk.Connect.Password))
			resp, err = http.Post(a.config.AuthUrl, TypeForm, strings.NewReader(payload.Encode()))
		}
	}

	if err != nil {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	if string(body) == "1" {
		fmt.Println("auth success")
		return true
	} else {
		fmt.Println("auth failed")
		return false
	}
}

// OnACLCheck returns true if the connecting client has matching read or write access to subscribe
// or publish to a given topic.
func (a *Auth) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	if a.config.AclMode == byte(auth.AuthAnonymous) {
		return true
	}

	// check blacklist
	if n, ok := a.config.CheckBLAcl(cl, topic, write); n >= 0 { // It's on the blacklist
		return ok
	}
	// normal verification
	var key string
	if a.config.AclMode == byte(auth.AuthUsername) {
		key = string(cl.Properties.Username)
	} else if a.config.AclMode == byte(auth.AuthClientID) {
		key = cl.ID
	} else {
		return false
	}
	var err error
	var resp *http.Response
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()
	if a.config.Method == "get" {
		var builder strings.Builder
		builder.WriteString(a.config.AclUrl)
		builder.WriteString("?")
		builder.WriteString("user=")
		builder.WriteString(key)
		resp, err = http.Get(builder.String())
	} else {
		if a.config.ContentType == TypeJson {
			payload := make(map[string]string, 2)
			payload["user"] = key
			bytesData, _ := json.Marshal(payload)
			resp, err = http.Post(a.config.AclUrl, TypeJson, bytes.NewBuffer(bytesData))
		} else {
			payload := url.Values{}
			payload.Add("user", key)
			resp, err = http.Post(a.config.AclUrl, TypeForm, strings.NewReader(payload.Encode()))
		}
	}
	if err != nil {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	fam1 := map[string]int{}
	if err := json.Unmarshal(body, &fam1); err != nil {
		return false
	}
	fam2 := make(map[string]auth.Access)
	for filter, access := range fam1 {
		if !plugin.MatchTopic(filter, topic) {
			continue
		}
		fam2[filter] = auth.Access(access)
	}
	return pa.CheckAcl(fam2, write)
}
