// dashboard/handlers/blacklist.go
package handlers

import (
	"net/http"
	"slices"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/dashboard/auth"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// BlacklistDeps bundles dependencies for the Blacklist page.
type BlacklistDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type blacklistPageData struct {
	Title    string
	User     auth.User
	CSRF     string
	Cluster  bool
	Flash    string
	Error    string
	Items    []string
	Readonly bool
}

// BlacklistGet renders the blacklist page (GET /dashboard/blacklist).
func BlacklistGet(d BlacklistDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromContext(r.Context())
		d.Renderer.Render(w, "blacklist", blacklistPageData{
			Title:    "Blacklist",
			User:     u,
			CSRF:     auth.NewCSRFToken(),
			Cluster:  d.Cluster,
			Items:    snapshotBlacklist(d.Server),
			Readonly: u.Role != auth.RoleAdmin,
		})
	}
}

// BlacklistAdd handles POST /dashboard/blacklist.
// Admin-only. Adds the client_id to the blacklist and disconnects them if
// currently connected.
func BlacklistAdd(d BlacklistDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.UserFromContext(r.Context()).Role != auth.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = r.ParseForm()
		id := strings.TrimSpace(r.PostFormValue("client_id"))
		if id == "" {
			http.Error(w, "client_id required", http.StatusBadRequest)
			return
		}
		if !slices.Contains(d.Server.Blacklist, id) {
			d.Server.Blacklist = append(d.Server.Blacklist, id)
		}
		if cl, ok := d.Server.Clients.Get(id); ok {
			d.Server.DisconnectClient(cl, packets.ErrNotAuthorized)
		}
		http.Redirect(w, r, "/dashboard/blacklist", http.StatusFound)
	}
}

// BlacklistRemove handles POST /dashboard/blacklist/{id}/delete.
// Admin-only. Removes the entry from the blacklist.
func BlacklistRemove(d BlacklistDeps) http.HandlerFunc {
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
		d.Server.Blacklist = slices.DeleteFunc(d.Server.Blacklist, func(s string) bool { return s == id })
		http.Redirect(w, r, "/dashboard/blacklist", http.StatusFound)
	}
}

// snapshotBlacklist copies the slice so concurrent broker mutation can't
// race the template execution. Cheap relative to bcrypt et al.
func snapshotBlacklist(s *mqtt.Server) []string {
	out := make([]string, len(s.Blacklist))
	copy(out, s.Blacklist)
	return out
}
