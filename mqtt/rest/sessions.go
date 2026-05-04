// mqtt/rest/sessions.go
package rest

import (
	"net/http"
	"sort"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

type sessionSummary struct {
	ClientID              string `json:"client_id"`
	Username              string `json:"username"`
	Online                bool   `json:"online"`
	Remote                string `json:"remote"`
	Listener              string `json:"listener,omitempty"`
	ProtocolVersion       byte   `json:"protocol_version"`
	Clean                 bool   `json:"clean"`
	Subs                  int    `json:"subs"`
	Inflight              int    `json:"inflight"`
	SessionExpiryInterval uint32 `json:"session_expiry_interval,omitempty"`
}

// listSessions handles GET /api/v1/mqtt/sessions.
// Optional ?online=true|false filters to online-only or offline-only.
// Online sessions are pulled from the in-memory Clients map; offline sessions
// are pulled from the storage backend via s.server.Hooks().StoredClients().
func (s *Rest) listSessions(w http.ResponseWriter, r *http.Request) {
	page := ParsePage(r.URL.Query())
	onlineParam := strings.ToLower(r.URL.Query().Get("online"))

	all := make([]sessionSummary, 0, 128)
	online := map[string]bool{}

	for _, cl := range s.server.Clients.GetAll() {
		online[cl.ID] = true
		if onlineParam == "false" {
			continue
		}
		all = append(all, sessionSummary{
			ClientID:        cl.ID,
			Username:        string(cl.Properties.Username),
			Online:          true,
			Remote:          cl.Net.Remote,
			Listener:        cl.Net.Listener,
			ProtocolVersion: cl.Properties.ProtocolVersion,
			Clean:           cl.Properties.Clean,
			Subs:            cl.State.Subscriptions.Len(),
			Inflight:        cl.State.Inflight.Len(),
		})
	}

	if onlineParam != "true" && s.server.Hooks() != nil && s.server.Hooks().Provides(mqtt.StoredClients) {
		stored, err := s.server.Hooks().StoredClients()
		if err == nil {
			for _, sc := range stored {
				if online[sc.ID] {
					continue
				}
				all = append(all, sessionSummary{
					ClientID:              sc.ID,
					Username:              string(sc.Username),
					Online:                false,
					Remote:                sc.Remote,
					Listener:              sc.Listener,
					ProtocolVersion:       sc.ProtocolVersion,
					Clean:                 sc.Clean,
					SessionExpiryInterval: sc.Properties.SessionExpiryInterval,
				})
			}
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].ClientID < all[j].ClientID })

	resp := Page[sessionSummary]{
		Page:  page.Page,
		Size:  page.Size,
		Total: len(all),
		Items: ApplyPagination(all, page),
	}
	Ok(w, resp)
}

// clearSession handles DELETE /api/v1/mqtt/sessions/{id}.
// Disconnects the client if online. Storage-backed session cleanup happens
// automatically when the broker processes the disconnect with clean=true,
// or via the existing storage hook eviction path on session expiry.
func (s *Rest) clearSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing client id", http.StatusBadRequest)
		return
	}
	cl, online := s.server.Clients.Get(id)
	storedExists := false
	if !online && s.server.Hooks() != nil && s.server.Hooks().Provides(mqtt.StoredClients) {
		if stored, err := s.server.Hooks().StoredClients(); err == nil {
			for _, sc := range stored {
				if sc.ID == id {
					storedExists = true
					break
				}
			}
		}
	}
	if !online && !storedExists {
		http.NotFound(w, r)
		return
	}
	if online {
		s.server.DisconnectClient(cl, packets.CodeDisconnect)
	}
	w.WriteHeader(http.StatusNoContent)
}
