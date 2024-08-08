// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package discovery

import (
	"encoding/json"
	"net"
	"os"
	"strconv"

	"github.com/wind-c/comqtt/v2/mqtt"
)

const (
	EventJoin = iota
	EventLeave
	EventFailed
	EventUpdate
	EventReap
)

const (
	TagRaftPort = "raft-port"
	TagGrpcPort = "grpc-port"
)

type Node interface {
	Setup() error
	Stop()
	BindMqttServer(server *mqtt.Server)
	LocalAddr() string
	LocalName() string
	Members() []Member
	EventChan() <-chan *Event
	SendToNode(nodeName string, msg []byte) error
	SendToOthers(msg []byte)
	Stat() map[string]int64
	Join(existing []string) (int, error)
	Leave() error
}

type Event struct {
	Member
	Type int
}

type Member struct {
	Name string            `json:"name"`
	Addr string            `json:"addr"`
	Port int               `json:"port"`
	Tags map[string]string `json:"tags,omitempty"`
}

func GenMemberAddrs(ms []Member) (addrs []string) {
	for _, m := range ms {
		addrs = append(addrs, net.JoinHostPort(m.Addr, strconv.Itoa(m.Port)))
	}
	return
}

func ReadMembers(file string) []Member {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var ms []Member
	if err := json.Unmarshal(content, &ms); err != nil {
		return nil
	}
	return ms
}

func GenNodesFile(file string, ms []Member) error {
	f, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	content, err := json.Marshal(&ms)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		return err
	}
	return nil
}
