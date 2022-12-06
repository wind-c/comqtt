package grpc

import (
	"context"
	"github.com/wind-c/comqtt/plugin"
	"github.com/wind-c/comqtt/plugin/auth/grpc/pb"
	"google.golang.org/grpc"
	"time"
)

type config struct {
	Address string `json:"address" yaml:"address"`
	Timeout int    `json:"timeout" yaml:"timeout"`
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

func (a *Auth) Authenticate(user, password []byte) bool {
	ctx1, cel := context.WithTimeout(context.Background(), time.Duration(a.conf.Timeout)*time.Second)
	defer cel()
	conn, err := grpc.DialContext(ctx1, a.conf.Address, grpc.WithBlock(), grpc.WithInsecure())
	response, err := pb.NewAuthenticateClient(conn).Authenticate(ctx1, &pb.AuthReq{
		User:     user,
		Password: password,
	})
	if err != nil {
		return false
	}
	return response.Code
}

func (a *Auth) ACL(user []byte, topic string, write bool) bool {
	ctx1, cel := context.WithTimeout(context.Background(), time.Duration(a.conf.Timeout)*time.Second)
	defer cel()
	conn, err := grpc.DialContext(ctx1, a.conf.Address, grpc.WithBlock(), grpc.WithInsecure())
	response, err := pb.NewAuthenticateClient(conn).Acl(ctx1, &pb.ACLReq{
		User:  user,
		Topic: topic,
		Write: write,
	})
	if err != nil {
		return false
	}
	return response.Code
}
