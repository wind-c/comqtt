package auth

import (
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

func CheckAcl(tam map[string]auth.Access, write bool) bool {
	// access 0 = deny, 1 = read only, 2 = write only, 3 = read and write
	rm := make(map[string]bool)
	for filter, access := range tam {
		if access == auth.Deny {
			rm[filter] = false
		} else if !write && (access == auth.ReadOnly || access == auth.ReadWrite) {
			rm[filter] = true
		} else if write && (access == auth.WriteOnly || access == auth.ReadWrite) {
			rm[filter] = true
		} else {
			rm[filter] = false
		}
	}

	//A more specific topic will win out over a wildcard topic.
	//Therefore, when defining permissions, we should let go of the big ones and restrict the small ones.
	//Or limit the big and let go of the small.
	//For example,"testtopic/user/#" allow and "testtopic/user/delete" deny;
	//And vice versa, "testtopic/user/#" deny and "testtopic/user/find" allow
	//Avoid the confusion of defining a bunch of permissions.
	//When there are multiple match filters, we use a clever method to select, usually the topic with wildcard is shorter.
	//So avoid single-character hierarchies for specific topics
	final := false
	maxLen := 0
	for filter, ok := range rm {
		l := len(filter)
		if l > maxLen {
			maxLen = l
			final = ok
		}
	}

	return final
}

type Blacklist struct {
	rules *auth.Ledger
}

func (b *Blacklist) SetBlacklist(bl *auth.Ledger) {
	b.rules = bl
}

func (b *Blacklist) CheckBLAuth(cl *mqtt.Client, pk packets.Packet) (n int, ok bool) {
	if b.rules == nil {
		return -1, false
	}
	for n, rule := range b.rules.Auth {
		if rule.Client.Matches(cl.ID) &&
			rule.Username.Matches(string(cl.Properties.Username)) &&
			rule.Remote.Matches(cl.Net.Remote) {
			return n, rule.Allow
		}
	}

	return -1, false
}

func (b *Blacklist) CheckBLAcl(cl *mqtt.Client, topic string, write bool) (n int, ok bool) {
	for _, rule := range b.rules.ACL {
		if rule.Client.Matches(cl.ID) &&
			rule.Username.Matches(string(cl.Properties.Username)) &&
			rule.Remote.Matches(cl.Net.Remote) {
			if len(rule.Filters) == 0 {
				return n, true
			}

			for filter, access := range rule.Filters {
				if filter.FilterMatches(topic) {
					if !write && (access == auth.ReadOnly || access == auth.ReadWrite) {
						return n, true
					} else if write && (access == auth.WriteOnly || access == auth.ReadWrite) {
						return n, true
					} else {
						return n, false
					}
				}
			}
		}
	}

	return -1, false
}
