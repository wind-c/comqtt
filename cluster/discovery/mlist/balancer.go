// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package mlist

import (
	"github.com/hashicorp/memberlist"
	"sync"
)

type RoundRobinBalancer struct {
	sync.Mutex

	membership *Membership
	current    int
	pool       []*memberlist.Node
}

func NewRoundRobinBalancer(m *Membership) *RoundRobinBalancer {
	return &RoundRobinBalancer{
		membership: m,
		current:    0,
		pool:       m.list.Members(),
	}
}

func (r *RoundRobinBalancer) Get() *memberlist.Node {
	r.Lock()
	defer r.Unlock()

	r.pool = r.membership.list.Members()
	if r.current >= len(r.pool) {
		r.current = r.current % len(r.pool)
	}

	result := r.pool[r.current]
	r.current++
	return result
}
