// mqtt/rest/clients_list.go
package rest

import (
	"net/http"
	"sort"
	"strings"
)

type clientSummary struct {
	ClientID    string `json:"client_id"`
	Username    string `json:"username"`
	Remote      string `json:"remote"`
	ConnectedAt int64  `json:"connected_at"`
	Keepalive   uint16 `json:"keepalive"`
	Subs        int    `json:"subs"`
	Pending     int    `json:"pending"`
}

// listClients handles GET /api/v1/mqtt/clients.
// Returns a paginated list filtered by ?q= prefix on ClientID.
func (s *Rest) listClients(w http.ResponseWriter, r *http.Request) {
	page := ParsePage(r.URL.Query())
	q := strings.ToLower(r.URL.Query().Get("q"))

	all := make([]clientSummary, 0, 256)
	for _, cl := range s.server.Clients.GetAll() {
		if q != "" && !strings.Contains(strings.ToLower(cl.ID), q) {
			continue
		}
		all = append(all, clientSummary{
			ClientID:  cl.ID,
			Username:  string(cl.Properties.Username),
			Remote:    cl.Net.Remote,
			Keepalive: cl.State.Keepalive,
			Subs:      cl.State.Subscriptions.Len(),
			Pending:   cl.State.Inflight.Len(),
		})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ClientID < all[j].ClientID })

	resp := Page[clientSummary]{
		Page:  page.Page,
		Size:  page.Size,
		Total: len(all),
		Items: ApplyPagination(all, page),
	}
	Ok(w, resp)
}
