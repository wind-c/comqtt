// mqtt/rest/subscriptions_list.go
package rest

import (
	"net/http"
	"sort"
	"strings"
)

type subscriptionSummary struct {
	ClientID          string `json:"client_id"`
	Topic             string `json:"topic"`
	QoS               byte   `json:"qos"`
	NoLocal           bool   `json:"no_local"`
	RetainAsPublished bool   `json:"retain_as_published"`
	Identifier        int    `json:"identifier,omitempty"`
}

// listSubscriptions handles GET /api/v1/mqtt/subscriptions.
// Returns a paginated list of subscriptions filtered by ?topic= substring on
// the topic filter and ?clientid= substring on the client ID.
func (s *Rest) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	page := ParsePage(r.URL.Query())
	topicFilter := strings.ToLower(r.URL.Query().Get("topic"))
	clientFilter := strings.ToLower(r.URL.Query().Get("clientid"))

	all := make([]subscriptionSummary, 0, 256)
	for _, cl := range s.server.Clients.GetAll() {
		if clientFilter != "" && !strings.Contains(strings.ToLower(cl.ID), clientFilter) {
			continue
		}
		for filter, sub := range cl.State.Subscriptions.GetAll() {
			if topicFilter != "" && !strings.Contains(strings.ToLower(filter), topicFilter) {
				continue
			}
			all = append(all, subscriptionSummary{
				ClientID:          cl.ID,
				Topic:             filter,
				QoS:               sub.Qos,
				NoLocal:           sub.NoLocal,
				RetainAsPublished: sub.RetainAsPublished,
				Identifier:        sub.Identifier,
			})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].ClientID != all[j].ClientID {
			return all[i].ClientID < all[j].ClientID
		}
		return all[i].Topic < all[j].Topic
	})

	resp := Page[subscriptionSummary]{
		Page:  page.Page,
		Size:  page.Size,
		Total: len(all),
		Items: ApplyPagination(all, page),
	}
	Ok(w, resp)
}
