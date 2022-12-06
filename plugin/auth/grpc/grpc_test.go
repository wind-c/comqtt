package grpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	. "github.com/wind-c/comqtt/plugin/auth/grpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"log"
	"net"
	"testing"
)

type GrpcAuthServer struct {
	UnimplementedAuthenticateServer
}

func (GrpcAuthServer) Authenticate(ctx context.Context, req *AuthReq) (*AuthResp, error) {
	return &AuthResp{Code: bytes.Equal(req.User, []byte("test1"))}, nil
}

func (GrpcAuthServer) Acl(ctx context.Context, req *ACLReq) (*AuthResp, error) {
	return &AuthResp{Code: req.Topic == "test1"}, nil
}

func dialer() func(context.Context, string) (net.Conn, error) {
	listener := bufconn.Listen(1024 * 1024)

	server := grpc.NewServer()

	RegisterAuthenticateServer(server, &GrpcAuthServer{})

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	return func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
}

func TestAuthenticate(t *testing.T) {
	tests := []struct {
		caseName string
		user     []byte
		password []byte
		res      bool
		err      error
	}{
		{
			"test1",
			[]byte("test1"),
			[]byte("123456"),
			true,
			nil,
		},
		{
			"test2",
			[]byte("test2"),
			[]byte("123456"),
			false,
			fmt.Errorf("grpc:  %v", false),
		},
	}
	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "", grpc.WithInsecure(), grpc.WithContextDialer(dialer()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	for _, tt := range tests {
		t.Run(tt.caseName, func(t *testing.T) {
			response, err := NewAuthenticateClient(conn).Authenticate(context.Background(), &AuthReq{
				User:     tt.user,
				Password: tt.password,
			})

			if response.Code != tt.res {
				t.Error("error: expected", tt.res, "received", response)
			}

			if err != nil && errors.Is(err, tt.err) {
				t.Error("error: expected", tt.err, "received", err)
			}
		})
	}
}

func TestAcl(t *testing.T) {
	tests := []struct {
		user  []byte
		topic string
		write bool
		res   bool
		err   error
	}{
		{
			[]byte("test1"),
			"test1",
			true,
			true,
			nil,
		},
		{
			[]byte("test2"),
			"test2",
			true,
			false,
			fmt.Errorf("grpc:  %v", false),
		},
	}
	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "", grpc.WithInsecure(), grpc.WithContextDialer(dialer()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			response, err := NewAuthenticateClient(conn).Acl(context.Background(), &ACLReq{
				User:  tt.user,
				Topic: tt.topic,
				Write: tt.write,
			})

			if response.Code != tt.res {
				t.Error("error: expected", tt.res, "received", response)
			}

			if err != nil && errors.Is(err, tt.err) {
				t.Error("error: expected", tt.err, "received", err)
			}
		})
	}
}
