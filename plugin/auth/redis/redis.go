package redis

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/wind-c/comqtt/plugin"
)

type config struct {
	Addr         string `json:"addr" yaml:"addr"`
	Password     string `json:"password" yaml:"password"`
	DB           int    `json:"db" yaml:"db"`
	AuthPrefix   string `json:"auth_prefix" yaml:"auth_prefix"`
	AclPrefix    string `json:"acl_prefix" yaml:"acl_prefix"`
	AclPublish   string `json:"acl_publish" yaml:"acl_publish"`
	AclSubscribe string `json:"acl_subscribe" yaml:"acl_subscribe"`
}

// Auth is an auth controller which allows access to all connections and topics.
type Auth struct {
	conf config
	db   *redis.Client
}

func New(path string) (*Auth, error) {
	conf := config{}
	err := plugin.LoadYaml(path, &conf)
	if err != nil {
		return nil, err
	}
	conf.AuthPrefix = conf.AuthPrefix + ":"
	conf.AclPrefix = conf.AclPrefix + ":"
	return &Auth{
		conf: conf,
	}, nil
}

func (a *Auth) Open() error {
	a.db = redis.NewClient(&redis.Options{
		Addr:     a.conf.Addr,
		Password: a.conf.Password, // no password set
		DB:       a.conf.DB,       // use default DB
	})
	_, err := a.db.Ping(context.TODO()).Result()
	return err
}

// Close closes the redis instance.
func (a *Auth) Close() {
	a.db.Close()
}

// Authenticate returns true if a username and password are acceptable. Allow always
// returns true.
func (a *Auth) Authenticate(user, password []byte) bool {
	res, err := a.db.HGet(context.Background(), a.conf.AuthPrefix+string(user), "password").Result()
	if err != nil && err != redis.Nil {
		return false
	}
	if res != "" {
		if res == string(password) {
			return true
		} else {
			return false
		}
	}

	return false
}

// ACL returns true if a user has access permissions to read or write on a topic.
// Allow always returns true.
func (a *Auth) ACL(user []byte, topic string, write bool) bool {
	res, err := a.db.HGet(context.Background(), a.conf.AclPrefix+string(user), topic).Result()
	if err != nil && err != redis.Nil {
		return false
	}
	if res != "" {
		if res == a.conf.AclPublish && write { //publish
			return true
		} else if res == a.conf.AclSubscribe && !write { //subscribe
			return true
		} else {
			return false
		}
	}

	return false
}
