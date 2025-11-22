package rest

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

const (
	MqttGetOverallPath     = "/api/v1/mqtt/stat/overall"
	MqttGetOnlinePath      = "/api/v1/mqtt/stat/online"
	MqttGetClientPath      = "/api/v1/mqtt/clients/{id}"
	MqttGetBlacklistPath   = "/api/v1/mqtt/blacklist"
	MqttAddBlacklistPath   = "/api/v1/mqtt/blacklist/{id}"
	MqttDelBlacklistPath   = "/api/v1/mqtt/blacklist/{id}"
	MqttPublishMessagePath = "/api/v1/mqtt/message"
	MqttGetConfigPath      = "/api/v1/mqtt/config"
	PrometheusMetrics      = "/metrics"
)

type Handler = func(http.ResponseWriter, *http.Request)

type Rest struct {
	server *mqtt.Server
}

func New(server *mqtt.Server) *Rest {
	return &Rest{
		server: server,
	}
}

func (s *Rest) GenHandlers() map[string]Handler {
	return map[string]Handler{
		"GET " + MqttGetConfigPath:       s.viewConfig,
		"GET " + MqttGetOverallPath:      s.getOverallInfo,
		"GET " + MqttGetOnlinePath:       s.getOnlineCount,
		"GET " + MqttGetClientPath:       s.getClient,
		"GET " + MqttGetBlacklistPath:    s.blacklist,
		"POST " + MqttAddBlacklistPath:   s.kickClient,
		"DELETE " + MqttDelBlacklistPath: s.blanchClient,
		"POST " + MqttPublishMessagePath: s.publishMessage,
		"GET " + PrometheusMetrics:       promhttp.Handler().ServeHTTP,
	}
}

// getOverallInfo return server info
// GET api/v1/mqtt/stat/overall
func (s *Rest) getOverallInfo(w http.ResponseWriter, r *http.Request) {
	Ok(w, s.server.Info)
}

// viewConfig return the configuration parameters of broker
// GET api/v1/mqtt/config
func (s *Rest) viewConfig(w http.ResponseWriter, r *http.Request) {
	Ok(w, s.server.Options)
}

// getOnlineCount return online number
// GET api/v1/mqtt/stat/online
func (s *Rest) getOnlineCount(w http.ResponseWriter, r *http.Request) {
	count := s.server.Info.ClientsConnected
	Ok(w, count)
}

// getClient return a client information
// GET api/v1/mqtt/clients/{id}
func (s *Rest) getClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if cl, ol := s.server.Clients.Get(id); ol {
		Ok(w, genClient(cl))
	} else {
		Error(w, http.StatusNotFound, "client not found")
	}
}

// publishMessage a message
// POST api/v1/mqtt/message
func (s *Rest) publishMessage(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var msg message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.server.Publish(msg.TopicName, []byte(msg.Payload), msg.Retain, msg.Qos); err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
	} else {
		Ok(w, msg)
	}
}

// kickClient disconnect the client and add it to the blacklist
// POST api/v1/mqtt/blacklist/{id}
func (s *Rest) kickClient(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	if !slices.Contains(s.server.Blacklist, cid) {
		s.server.Blacklist = append(s.server.Blacklist, cid)
	}
	if cl, ol := s.server.Clients.Get(cid); ol {
		s.server.DisconnectClient(cl, packets.ErrNotAuthorized)
		Ok(w, cid)
	} else {
		Error(w, http.StatusNotFound, "client not found")
	}
}

// blanchClient remove from the blacklist
// DELETE api/v1/mqtt/blacklist/{id}
func (s *Rest) blanchClient(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	if slices.Contains(s.server.Blacklist, cid) {
		slices.DeleteFunc(s.server.Blacklist, func(s string) bool { return s == cid })
		Ok(w, cid)
	}
}

// blacklist return to the blacklist
// GET api/v1/mqtt/blacklist
func (s *Rest) blacklist(w http.ResponseWriter, r *http.Request) {
	if s.server.Blacklist == nil {
		Error(w, http.StatusNotFound, "blacklist not found")
	} else {
		Ok(w, s.server.Blacklist)
	}
}
