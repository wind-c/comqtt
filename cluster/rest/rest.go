package rest

import (
	"encoding/json"
	"fmt"
	cs "github.com/wind-c/comqtt/v2/cluster"
	"github.com/wind-c/comqtt/v2/cluster/discovery"
	rt "github.com/wind-c/comqtt/v2/mqtt/rest"
	"net/http"
	"net/netip"
	"strings"
)

type rest struct {
	agent *cs.Agent
}

func New(agent *cs.Agent) *rest {
	return &rest{
		agent: agent,
	}
}

func (s *rest) GenHandlers() map[string]rt.Handler {
	return map[string]rt.Handler{
		"GET /api/v1/node/config":                                   s.viewConfig,
		"DELETE /api/v1/node/{name}":                                s.leave,
		"GET /api/v1/cluster/nodes":                                 s.getNodes,
		"POST /api/v1/cluster/nodes":                                s.join,
		"POST /api/v1/cluster/peers":                                s.addRaftPeer,
		"DELETE /api/v1/cluster/peers/{name}":                       s.removeRaftPeer,
		"GET /api/v1/cluster/stat/online":                           s.getOnlineCount,
		"GET /api/v1/cluster/clients/{id}":                          s.getClient,
		"POST /api/v1/cluster/blacklist/{id}":                       s.kickClient,
		"DELETE /api/v1/cluster/blacklist/{id}":                     s.blanchClient,
		"GET /api/v1/cluster/clients":                               s.listClients,
		"GET /api/v1/cluster/subscriptions":                         s.listSubscriptions,
		"GET /api/v1/cluster/topics":                                s.topicsTree,
		"DELETE /api/v1/cluster/clients/{id}/subscriptions/{topic}": s.unsubscribeClient,
		"GET /api/v1/cluster/retained":                              s.listRetained,
		"DELETE /api/v1/cluster/retained/{topic}":                   s.clearRetained,
		"GET /api/v1/cluster/sessions":                              s.listSessions,
		"DELETE /api/v1/cluster/sessions/{id}":                      s.clearSession,
	}
}

// viewConfig return the configuration parameters of this node
// GET api/v1/node/config
func (s *rest) viewConfig(w http.ResponseWriter, r *http.Request) {
	rt.Ok(w, s.agent.Config)
}

// getMembers return all nodes in the cluster
// GET api/v1/cluster/nodes
func (s *rest) getNodes(w http.ResponseWriter, r *http.Request) {
	rt.Ok(w, s.agent.GetMemberList())
}

// join add a node to the cluster
// POST api/v1/cluster/nodes
func (s *rest) join(w http.ResponseWriter, r *http.Request) {
	var n node
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		rt.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	n.Name = strings.TrimSpace(n.Name)
	n.Addr = strings.TrimSpace(n.Addr)
	if n.Name == "" || n.Addr == "" {
		rt.Error(w, http.StatusBadRequest, "name and addr cannot be empty")
		return
	}
	if _, err := netip.ParseAddrPort(n.Addr); err != nil {
		rt.Error(w, http.StatusBadRequest, "invalid address")
		return
	}

	if err := s.agent.Join(n.Name, n.Addr); err != nil {
		rt.Error(w, http.StatusInternalServerError, err.Error())
	} else {
		rt.Ok(w, n)
	}
}

// leave local node gracefully exits the cluster
// DELETE api/v1/node/{name}
func (s *rest) leave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	localName := s.agent.GetLocalName()
	if name != localName {
		rt.Error(w, http.StatusBadRequest, fmt.Sprintf("cannot remove not local node %s", localName))
		return
	}

	if err := s.agent.Leave(); err != nil {
		rt.Error(w, http.StatusInternalServerError, err.Error())
		return
	} else {
		rt.Ok(w, name)
	}
}

// addRaftPeer add peer to raft cluster
// POST api/v1/cluster/peers
func (s *rest) addRaftPeer(w http.ResponseWriter, r *http.Request) {
	var p node
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		rt.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Addr = strings.TrimSpace(p.Addr)
	if p.Name == "" || p.Addr == "" {
		rt.Error(w, http.StatusBadRequest, "name and addr cannot be empty")
		return
	}
	if _, err := netip.ParseAddrPort(p.Addr); err != nil {
		rt.Error(w, http.StatusBadRequest, "invalid address")
		return
	}

	s.agent.AddRaftPeer(p.Name, p.Addr)
	rt.Ok(w, p)
}

// removeRaftPeer remove peer from raft cluster
// DELETE api/v1/cluster/peers/{name}
func (s *rest) removeRaftPeer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if strings.TrimSpace(name) == "" {
		rt.Error(w, http.StatusBadRequest, "name cannot be empty")
		return
	}

	s.agent.RemoveRaftPeer(name)
	rt.Ok(w, name)
}

// getOnlineCount return online number from all nodes in the cluster
// GET api/v1/cluster/stat/online
func (s *rest) getOnlineCount(w http.ResponseWriter, r *http.Request) {
	path := rt.MqttGetOnlinePath
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpGet, urls, nil)
	rt.Ok(w, rs)
}

// getClient return a client information, search from all nodes in the cluster
// GET api/v1/cluster/clients/{id}
func (s *rest) getClient(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	path := strings.Replace(rt.MqttGetClientPath, "{id}", cid, 1)
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpGet, urls, nil)
	rt.Ok(w, rs)
}

// kickClient add it to the blacklist on all nodes in the cluster
// POST api/v1/cluster/blacklist/{id}
func (s *rest) kickClient(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	path := strings.Replace(rt.MqttAddBlacklistPath, "{id}", cid, 1)
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpPost, urls, nil)
	rt.Ok(w, rs)
}

// blanchClient remove from the blacklist on all nodes in the cluster
// DELETE api/v1/cluster/blacklist/{id}
func (s *rest) blanchClient(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	path := strings.Replace(rt.MqttDelBlacklistPath, "{id}", cid, 1)
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpDelete, urls, nil)
	rt.Ok(w, rs)
}

// listClients fan out client listing to every node in the cluster
// GET api/v1/cluster/clients
func (s *rest) listClients(w http.ResponseWriter, r *http.Request) {
	path := rt.MqttListClientsPath
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpGet, urls, nil)
	rt.Ok(w, rs)
}

// listSubscriptions fan out subscription listing to every node in the cluster
// GET api/v1/cluster/subscriptions
func (s *rest) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	path := rt.MqttListSubscriptionsPath
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpGet, urls, nil)
	rt.Ok(w, rs)
}

// topicsTree fan out topic tree retrieval to every node in the cluster
// GET api/v1/cluster/topics
func (s *rest) topicsTree(w http.ResponseWriter, r *http.Request) {
	urls := genUrls(s.agent.GetMemberList(), rt.MqttTopicsTreePath)
	rs := fetchM(HttpGet, urls, nil)
	rt.Ok(w, rs)
}

// unsubscribeClient fan out unsubscribe to every node in the cluster
// DELETE api/v1/cluster/clients/{id}/subscriptions/{topic}
func (s *rest) unsubscribeClient(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	topic := r.PathValue("topic")
	path := strings.Replace(rt.MqttUnsubscribeClientPath, "{id}", cid, 1)
	path = strings.Replace(path, "{topic}", topic, 1)
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpDelete, urls, nil)
	rt.Ok(w, rs)
}

// listRetained fan out retained message listing to every node in the cluster
// GET api/v1/cluster/retained
func (s *rest) listRetained(w http.ResponseWriter, r *http.Request) {
	path := rt.MqttListRetainedPath
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpGet, urls, nil)
	rt.Ok(w, rs)
}

// clearRetained fan out retained clear to every node in the cluster
// DELETE api/v1/cluster/retained/{topic}
func (s *rest) clearRetained(w http.ResponseWriter, r *http.Request) {
	topic := r.PathValue("topic")
	path := strings.Replace(rt.MqttClearRetainedPath, "{topic}", topic, 1)
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpDelete, urls, nil)
	rt.Ok(w, rs)
}

// listSessions fan out session listing to every node in the cluster
// GET api/v1/cluster/sessions
func (s *rest) listSessions(w http.ResponseWriter, r *http.Request) {
	path := rt.MqttListSessionsPath
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpGet, urls, nil)
	rt.Ok(w, rs)
}

// clearSession fan out session clear to every node in the cluster
// DELETE api/v1/cluster/sessions/{id}
func (s *rest) clearSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := strings.Replace(rt.MqttClearSessionPath, "{id}", id, 1)
	urls := genUrls(s.agent.GetMemberList(), path)
	rs := fetchM(HttpDelete, urls, nil)
	rt.Ok(w, rs)
}

// genUrls generate urls
func genUrls(ms []discovery.Member, path string) []string {
	urls := make([]string, len(ms))
	for i, m := range ms {
		urls[i] = "http://" + m.Addr + ":8080" + path
	}
	return urls
}
