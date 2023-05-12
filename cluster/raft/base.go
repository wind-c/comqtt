// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package raft

import "github.com/wind-c/comqtt/v2/cluster/message"

type IPeer interface {
	Join(nodeID, addr string) error
	Leave(nodeID string) error
	Propose(msg *message.Message) error
	Lookup(key string) []string
	IsApplyRight() bool
	GetLeader() (addr, id string)
	GenPeersFile(file string) error
	Stop()
}
