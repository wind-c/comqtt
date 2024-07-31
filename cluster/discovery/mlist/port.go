package mlist

// serf.Member has Tags, but memberlist.Node does not, so the Raft and gRPC ports cannot be relayed through events.
// Ports are derived based on the bind port instead.

func GetRaftPortFromBindPort(bindPort int) int {
	return bindPort + 1000
}

func GetGRPCPortFromBindPort(bindPort int) int {
	return bindPort + 10000
}
