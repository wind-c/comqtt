package http

import (
	"bytes"
	"encoding/json"
	"github.com/wind-c/comqtt/plugin"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	TypeJson = "application/json"
	TypeForm = "application/x-www-form-urlencoded"
)

type config struct {
	TlsEnable    bool   `json:"tls-enable" yaml:"tls-enable"`
	TlsCert      string `json:"tls-cert" yaml:"tls-cert"`
	TlsKey       string `json:"tls-key" yaml:"tls-key"`
	Method       string `json:"method" yaml:"method"`
	ContentType  string `json:"content-type" yaml:"content-type"`
	AuthUrl      string `json:"auth-url" yaml:"auth-url"`
	AuthSuccess  string `json:"auth-success" yaml:"auth-success"`
	AclUrl       string `json:"acl-url" yaml:"acl-url"`
	AclPublish   string `json:"acl-publish" yaml:"acl-publish"`
	AclSubscribe string `json:"acl-subscribe" yaml:"acl-subscribe"`
	AclPubSub    string `json:"acl-pubsub" yaml:"acl-pubsub"`
}

// Auth is an auth controller which allows access to all connections and topics.
type Auth struct {
	conf config
}

func New(confFile string) (*Auth, error) {
	conf := config{}
	err := plugin.LoadYaml(confFile, &conf)
	if err != nil {
		return nil, err
	}
	return &Auth{
		conf: conf,
	}, nil
}

func (a *Auth) Open() error {
	return nil
}

func (a *Auth) Close() {
}

// Authenticate returns true if a username and password are acceptable. Allow always
// returns true.
func (a *Auth) Authenticate(user, password []byte) bool {
	var err error
	var resp *http.Response
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	if a.conf.Method == "get" {
		var builder strings.Builder
		builder.WriteString(a.conf.AuthUrl)
		builder.WriteString("?")
		builder.WriteString("user=")
		builder.Write(user)
		builder.WriteString("&")
		builder.WriteString("password=")
		builder.Write(password)
		resp, err = http.Get(builder.String())
	} else {
		if a.conf.ContentType == TypeJson {
			payload := make(map[string]string, 2)
			payload["user"] = string(user)
			payload["password"] = string(password)
			bytesData, _ := json.Marshal(payload)
			resp, err = http.Post(a.conf.AuthUrl, TypeJson, bytes.NewBuffer(bytesData))
		} else {
			payload := url.Values{}
			payload.Add("user", string(user))
			payload.Add("password", string(password))
			resp, err = http.Post(a.conf.AuthUrl, TypeForm, strings.NewReader(payload.Encode()))
		}
	}

	if err != nil {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	if string(body) == a.conf.AuthSuccess {
		return true
	} else {
		return false
	}
}

// ACL returns true if a user has access permissions to read or write on a topic.
// Allow always returns true.
func (a *Auth) ACL(user []byte, topic string, write bool) bool {
	var err error
	var resp *http.Response
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	if a.conf.Method == "get" {
		var builder strings.Builder
		builder.WriteString(a.conf.AclUrl)
		builder.WriteString("?")
		builder.WriteString("user=")
		builder.Write(user)
		builder.WriteString("&")
		builder.WriteString("topic=")
		builder.WriteString(topic)
		resp, err = http.Get(builder.String())
	} else {
		if a.conf.ContentType == TypeJson {
			payload := make(map[string]string, 2)
			payload["user"] = string(user)
			payload["topic"] = topic
			bytesData, _ := json.Marshal(payload)
			resp, err = http.Post(a.conf.AclUrl, TypeJson, bytes.NewBuffer(bytesData))
		} else {
			payload := url.Values{}
			payload.Add("user", string(user))
			payload.Add("topic", topic)
			resp, err = http.Post(a.conf.AclUrl, TypeForm, strings.NewReader(payload.Encode()))
		}
	}

	if err != nil {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	if string(body) == a.conf.AclPubSub {
		return true
	} else if string(body) == a.conf.AclPublish && write {
		return true
	} else if string(body) == a.conf.AclSubscribe && !write { //subscribe
		return true
	} else {
		return false
	}
}
