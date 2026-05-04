// mqtt/dashboard/handlers/clients.go
package handlers

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
	"github.com/wind-c/comqtt/v2/mqtt/rest"
)

// ClientsDeps bundles the dependencies for the Clients page.
type ClientsDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type clientsPageData struct {
	Title      string
	User       auth.User
	CSRF       string
	Cluster    bool
	Flash      string
	Q          string
	Page       rest.Page[clientRow]
	TotalPages int
	PrevQuery  string
	NextQuery  string
}

// clientRow mirrors the JSON fields of rest/clients_list.go::clientSummary so
// the same template can be driven by either source in future SSE upgrades.
type clientRow struct {
	ClientID  string
	Username  string
	Remote    string
	Keepalive uint16
	Subs      int
	Pending   int
}

// ClientsList handles GET /dashboard/clients.
func ClientsList(d ClientsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("q"))
		params := rest.ParsePage(r.URL.Query())

		all := make([]clientRow, 0, 64)
		for _, cl := range d.Server.Clients.GetAll() {
			if q != "" && !strings.Contains(strings.ToLower(cl.ID), q) {
				continue
			}
			all = append(all, clientRow{
				ClientID:  cl.ID,
				Username:  string(cl.Properties.Username),
				Remote:    cl.Net.Remote,
				Keepalive: cl.State.Keepalive,
				Subs:      cl.State.Subscriptions.Len(),
				Pending:   cl.State.Inflight.Len(),
			})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].ClientID < all[j].ClientID })

		page := rest.Page[clientRow]{
			Page:  params.Page,
			Size:  params.Size,
			Total: len(all),
			Items: rest.ApplyPagination(all, params),
		}
		totalPages := page.Total / page.Size
		if page.Total%page.Size > 0 {
			totalPages++
		}
		if totalPages < 1 {
			totalPages = 1
		}

		d.Renderer.Render(w, "clients_list", clientsPageData{
			Title:      "Clients",
			User:       auth.UserFromContext(r.Context()),
			CSRF:       auth.NewCSRFToken(),
			Cluster:    d.Cluster,
			Q:          r.URL.Query().Get("q"),
			Page:       page,
			TotalPages: totalPages,
			PrevQuery:  pageQuery(r.URL.Query(), params.Page-1),
			NextQuery:  pageQuery(r.URL.Query(), params.Page+1),
		})
	}
}

// pageQuery returns the URL-encoded query string with `page=N` substituted.
// All other params (size, q) are preserved.
func pageQuery(q url.Values, page int) string {
	out := url.Values{}
	for k, v := range q {
		if k == "page" {
			continue
		}
		for _, vv := range v {
			out.Add(k, vv)
		}
	}
	out.Set("page", itoa(int64(page)))
	return out.Encode()
}
