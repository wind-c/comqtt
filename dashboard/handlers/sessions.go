// dashboard/handlers/sessions.go
package handlers

import (
	"net/http"
	"sort"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/dashboard/auth"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
	"github.com/wind-c/comqtt/v2/mqtt/rest"
)

type SessionsDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type sessionRowDash struct {
	ClientID string
	Online   bool
	Remote   string
	Subs     int
	Inflight int
}

type sessionsPageData struct {
	Title    string
	User     auth.User
	CSRF     string
	Cluster  bool
	Flash    string
	Online   string
	Page     rest.Page[sessionRowDash]
	Readonly bool
}

func SessionsList(d SessionsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := rest.ParsePage(r.URL.Query())
		onlineParam := r.URL.Query().Get("online")

		all := make([]sessionRowDash, 0, 64)
		// Online sessions only for v1 (offline session enumeration via
		// Server.Hooks().StoredClients() can land in Phase 4).
		if onlineParam != "false" {
			for _, cl := range d.Server.Clients.GetAll() {
				all = append(all, sessionRowDash{
					ClientID: cl.ID,
					Online:   true,
					Remote:   cl.Net.Remote,
					Subs:     cl.State.Subscriptions.Len(),
					Inflight: cl.State.Inflight.Len(),
				})
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].ClientID < all[j].ClientID })

		page := rest.Page[sessionRowDash]{
			Page:  params.Page,
			Size:  params.Size,
			Total: len(all),
			Items: rest.ApplyPagination(all, params),
		}
		u := auth.UserFromContext(r.Context())
		d.Renderer.Render(w, "sessions", sessionsPageData{
			Title:    "Sessions",
			User:     u,
			CSRF:     auth.NewCSRFToken(),
			Cluster:  d.Cluster,
			Online:   onlineParam,
			Page:     page,
			Readonly: u.Role != auth.RoleAdmin,
		})
	}
}

func SessionsClear(d SessionsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.UserFromContext(r.Context()).Role != auth.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		cl, ok := d.Server.Clients.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		d.Server.DisconnectClient(cl, packets.CodeDisconnect)
		http.Redirect(w, r, "/dashboard/sessions", http.StatusFound)
	}
}
