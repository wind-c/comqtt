// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package cluster

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/wind-c/comqtt/v2/cluster/log"
	"github.com/wind-c/comqtt/v2/cluster/message"
	crpc "github.com/wind-c/comqtt/v2/cluster/rpc"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/health"
	"google.golang.org/grpc/keepalive"
)

const (
	ReqTimeout = 1 * time.Second
)

var kaep = keepalive.EnforcementPolicy{
	MinTime:             5 * time.Second, // If a client pings more than once every 5 seconds, terminate the connection
	PermitWithoutStream: true,            // Allow pings even when there are no active streams
}

var kasp = keepalive.ServerParameters{
	MaxConnectionIdle:     15 * time.Second, // If a client is idle for 15 seconds, send a GOAWAY
	MaxConnectionAge:      30 * time.Second, // If any connection is alive for more than 30 seconds, send a GOAWAY
	MaxConnectionAgeGrace: 5 * time.Second,  // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second,  // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               1 * time.Second,  // Wait 1 second for the ping ack before assuming the connection is dead
}

var kacp = keepalive.ClientParameters{
	Time:                15 * time.Second, // send pings every 15 seconds if there is no activity
	Timeout:             3 * time.Second,  // wait 3 second for ping ack before considering the connection dead
	PermitWithoutStream: true,             // send pings even without active streams
}

type RpcService struct {
	agent      *Agent
	grpcServer *grpc.Server
}

func NewRpcService(a *Agent) *RpcService {
	return &RpcService{agent: a}
}

func (s *RpcService) StartRpcServer() error {
	// grpc server
	addr := net.JoinHostPort(s.agent.Config.BindAddr, strconv.Itoa(s.agent.Config.GrpcPort))
	grpcListen, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// s.grpcServer = grpc.NewServer()
	s.grpcServer = grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp))
	// register client services
	crpc.RegisterRelaysServer(s.grpcServer, s)

	// serve grpc
	go func() {
		if err := s.grpcServer.Serve(grpcListen); err != nil {
			log.Error("grpc server serve", "error", err)
		}
	}()

	return nil
}

func (s *RpcService) StopRpcServer() {
	// if s or the grpc server is nil, return, there is nothing to stop.
	if s == nil || s.grpcServer == nil {
		return
	}
	// Gracefully stop, allowing 5 seconds for ongoing requests to complete
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		log.Info("grpc server stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Warn("grpc server graceful stop timeout, forcing stop")
		s.grpcServer.Stop() // Force stop
	}
}

func (s *RpcService) PublishPacket(ctx context.Context, req *crpc.PublishRequest) (*crpc.Response, error) {
	msg := message.Message{
		Type:            packets.Publish,
		NodeID:          req.NodeId,
		ClientID:        req.ClientId,
		ProtocolVersion: uint8(req.ProtocolVersion),
		Payload:         req.Payload,
	}
	s.agent.grpcMsgCh <- &msg

	return &crpc.Response{Ok: true}, nil
}

func (s *RpcService) ConnectNotify(ctx context.Context, req *crpc.ConnectRequest) (*crpc.Response, error) {
	msg := message.Message{
		Type:     packets.Connect,
		NodeID:   req.NodeId,
		ClientID: req.ClientId,
	}
	s.agent.grpcMsgCh <- &msg

	return &crpc.Response{Ok: true}, nil
}

func (s *RpcService) RaftApply(ctx context.Context, req *crpc.ApplyRequest) (*crpc.Response, error) {
	msg := message.Message{
		Type:    uint8(req.Action),
		NodeID:  req.NodeId,
		Payload: req.Filter,
	}
	s.agent.grpcMsgCh <- &msg

	return &crpc.Response{Ok: true}, nil
}

func (s *RpcService) RaftJoin(ctx context.Context, req *crpc.JoinRequest) (*crpc.Response, error) {
	addr := net.JoinHostPort(req.Addr, strconv.Itoa(int(req.Port)))
	msg := message.Message{
		Type:    message.RaftJoin,
		NodeID:  req.NodeId,
		Payload: []byte(addr),
	}
	s.agent.grpcMsgCh <- &msg

	return &crpc.Response{Ok: true}, nil
}

type ClientManager struct {
	agent *Agent
	cs    map[string]*client
	sync.Mutex
}

type client struct {
	conn *grpc.ClientConn
	crpc.RelaysClient
}

func NewClientManager(a *Agent) *ClientManager {
	return &ClientManager{
		agent: a,
		cs:    make(map[string]*client),
	}
}

func (c *ClientManager) RemoveGrpcClient(nodeId string) {
	c.Lock()
	defer c.Unlock()

	cli, ok := c.cs[nodeId]
	if !ok {
		return
	}
	delete(c.cs, nodeId)
	if err := cli.conn.Close(); err != nil {
		log.Error("close grpc client error", "nodeId", nodeId, "error", err)
	}
}

func (c *ClientManager) getNodeAddr(nodeId string) (string, error) {
	m := c.agent.getNodeMember(nodeId)
	if m == nil {
		return "", errors.New("node not found")
	}

	return getGrpcAddr(m), nil
}

func (c *ClientManager) getClient(nodeId string) (*client, error) {
	c.Lock()
	defer c.Unlock()

	// Check cache, return healthy connection directly
	if cli, ok := c.healthCheck(nodeId); ok {
		return cli, nil
	}

	// Get node address
	addr, err := c.getNodeAddr(nodeId)
	if err != nil {
		return nil, fmt.Errorf("get node addr failed: %w", err)
	}

	// Configure retry interceptor
	retryOpts := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(grpc_retry.BackoffExponential(ReqTimeout)),
		grpc_retry.WithMax(3),
	}

	// Create client
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
		grpc.WithChainUnaryInterceptor(grpc_retry.UnaryClientInterceptor(retryOpts...)),
		grpc.WithKeepaliveParams(kacp),
	)
	if err != nil {
		return nil, fmt.Errorf("create grpc client failed: %w", err)
	}

	// Wrap and cache the client
	rpcClient := crpc.NewRelaysClient(conn)
	wrapper := &client{conn: conn, RelaysClient: rpcClient}
	c.cs[nodeId] = wrapper

	return wrapper, nil
}

func (c *ClientManager) RelayPublishPacket(nodeId string, msg *message.Message) {
	client, err := c.getClient(nodeId)
	if err != nil {
		log.Error("get grpc client", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ReqTimeout)
	defer cancel()
	req := crpc.PublishRequest{
		NodeId:          msg.NodeID,
		ClientId:        msg.ClientID,
		ProtocolVersion: uint32(msg.ProtocolVersion),
		Payload:         msg.Payload,
	}
	if _, err := client.PublishPacket(ctx, &req); err != nil {
		log.Error("relay publish packet", "error", err, "to", nodeId, "cid", msg.ClientID)
	}
}

func (c *ClientManager) ConnectNotifyToNode(nodeId, clientId string) {
	client, err := c.getClient(nodeId)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ReqTimeout)
	defer cancel()
	req := crpc.ConnectRequest{
		NodeId:   c.agent.GetLocalName(),
		ClientId: clientId,
	}
	OnConnectPacketLog(DirectionOutbound, nodeId, clientId)
	if _, err := client.ConnectNotify(ctx, &req); err != nil {
		log.Error("connection notification", "error", err, "to", nodeId, "cid", clientId)
	}
}

func (c *ClientManager) ConnectNotifyToOthers(msg *message.Message) {
	ms := c.agent.membership.Members()
	for _, m := range ms {
		if m.Name == c.agent.GetLocalName() {
			continue
		}
		c.ConnectNotifyToNode(m.Name, msg.ClientID)
	}
}

func (c *ClientManager) RelayRaftApply(nodeId string, msg *message.Message) {
	client, err := c.getClient(nodeId)
	if err != nil {
		log.Error("get grpc client", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*ReqTimeout)
	defer cancel()
	req := crpc.ApplyRequest{
		Action: uint32(msg.Type),
		NodeId: msg.NodeID,
		Filter: msg.Payload,
	}
	if _, err := client.RaftApply(ctx, &req); err != nil {
		OnApplyLog(nodeId, msg.NodeID, msg.Type, msg.Payload, "to leader do apply", err)
	}
}

func (c *ClientManager) RaftApplyToOthers(msg *message.Message) {
	ms := c.agent.membership.Members()
	for _, m := range ms {
		if m.Name == c.agent.GetLocalName() {
			continue
		}
		c.RelayRaftApply(m.Name, msg)
	}
}

func (c *ClientManager) RelayRaftJoin(nodeId string) {
	client, err := c.getClient(nodeId)
	if err != nil {
		log.Error("get grpc client", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ReqTimeout)
	defer cancel()
	req := crpc.JoinRequest{
		NodeId: c.agent.GetLocalName(),
		Addr:   c.agent.Config.BindAddr,
		Port:   uint32(c.agent.Config.RaftPort),
	}
	if _, err := client.RaftJoin(ctx, &req); err != nil {
		addr := c.agent.Config.BindAddr + ":" + strconv.Itoa(c.agent.Config.RaftPort)
		OnJoinLog(nodeId, addr, "raft join", err)
	}
}

func (c *ClientManager) RaftJoinToOthers() {
	ms := c.agent.membership.Members()
	for _, m := range ms {
		if m.Name == c.agent.GetLocalName() {
			continue
		}
		c.RelayRaftJoin(m.Name)
	}
}

func (c *ClientManager) healthCheck(nodeId string) (*client, bool) {
	cli, ok := c.cs[nodeId]
	if !ok {
		return nil, false
	}
	if c.isConnHealth(cli) {
		return cli, true
	}
	c.removeClient(nodeId, cli)
	return nil, false
}

func (c *ClientManager) isConnHealth(cli *client) bool {
	state := cli.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Connecting
}

func (c *ClientManager) removeClient(nodeId string, cli *client) {
	delete(c.cs, nodeId)
	if err := cli.conn.Close(); err != nil {
		log.Error("close grpc client error", "nodeId", nodeId, "error", err)
		return
	}
	log.Warn("removed unhealthy grpc client", "node", nodeId)
}

func (c *ClientManager) RemoveUnhealthyClients() {
	c.Lock()
	defer c.Unlock()

	for nodeId, cli := range c.cs {
		if !c.isConnHealth(cli) {
			c.removeClient(nodeId, cli)
		}
	}
}
