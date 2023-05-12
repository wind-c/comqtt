// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package hashicorp

import (
	"github.com/hashicorp/raft"
	"github.com/wind-c/comqtt/v2/cluster/log/zero"
	"net"
	"time"
)

func newRaftTrans(ln net.Listener) *raft.NetworkTransport {
	layer := newRaftLayer(nil, ln)
	addr, ok := layer.Addr().(*net.TCPAddr)
	if !ok {
		if err := ln.Close(); err != nil {
			zero.Error().Err(err).Msg("raft addr is not tcp addr")
		}
		return nil
	}
	if addr.IP == nil || addr.IP.IsUnspecified() {
		if err := ln.Close(); err != nil {
			zero.Error().Err(err).Msg("raft addr is not valid")
		}
		return nil
	}
	return raft.NewNetworkTransport(layer, maxPool, DefaultRaftTimeout, zero.Logger())
}

type raftLayer struct {
	addr net.Addr
	ln   net.Listener
}

func newRaftLayer(addr net.Addr, ln net.Listener) *raftLayer {
	return &raftLayer{
		addr: addr,
		ln:   ln,
	}
}

func (l *raftLayer) Addr() net.Addr {
	if l.addr != nil {
		return l.addr
	}
	return l.ln.Addr()
}

func (l *raftLayer) Dial(address raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", string(address), timeout)
	if err != nil {
		return nil, err
	}
	return conn, err
}

func (l *raftLayer) Accept() (net.Conn, error) {
	return l.ln.Accept()
}

func (l *raftLayer) Close() error {
	return l.ln.Close()
}
