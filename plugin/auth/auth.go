package auth

import oauth "github.com/wind-c/comqtt/server/listeners/auth"

type Auth interface {
	Open() error
	Close()
	oauth.Controller
}
