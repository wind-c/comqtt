// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package etcd

import (
	"errors"
	"net"
	"time"
)

// @Author Chen Zikang
// @Email  parakovo@gmail.com
// @Since  2022-08-16

type stoppableListener struct {
	*net.TCPListener
	stopC <-chan struct{}
}

func newStoppableListener(addr string, stopC <-chan struct{}) (*stoppableListener, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &stoppableListener{
		TCPListener: listener.(*net.TCPListener),
		stopC:       stopC,
	}, nil
}

func (l stoppableListener) Accept() (c net.Conn, err error) {
	connC := make(chan *net.TCPConn, 1)
	errC := make(chan error, 1)
	go func() {
		tc, err := l.AcceptTCP()
		if err != nil {
			errC <- err
			return
		}
		connC <- tc
	}()
	select {
	case <-l.stopC:
		return nil, errors.New("server stopped")
	case err := <-errC:
		return nil, err
	case tc := <-connC:
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(3 * time.Minute)
		return tc, nil
	}
}
