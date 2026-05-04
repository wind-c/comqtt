// mqtt/dashboard/handlers/subscriptions.go
package handlers

import (
	"net/http"
	"sort"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
	"github.com/wind-c/comqtt/v2/mqtt/rest"
)

type SubscriptionsDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type subRow struct {
	ClientID          string
	Topic             string
	QoS               byte
	NoLocal           bool
	RetainAsPublished bool
}

type subPageData struct {
	Title    string
	User     auth.User
	CSRF     string
	Cluster  bool
	Flash    string
	Topic    string
	ClientID string
	Page     rest.Page[subRow]
}

func SubscriptionsList(d SubscriptionsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := rest.ParsePage(r.URL.Query())
		topicFilter := strings.ToLower(r.URL.Query().Get("topic"))
		clientFilter := strings.ToLower(r.URL.Query().Get("clientid"))

		all := make([]subRow, 0, 64)
		for _, cl := range d.Server.Clients.GetAll() {
			if clientFilter != "" && !strings.Contains(strings.ToLower(cl.ID), clientFilter) {
				continue
			}
			for filter, sub := range cl.State.Subscriptions.GetAll() {
				if topicFilter != "" && !strings.Contains(strings.ToLower(filter), topicFilter) {
					continue
				}
				all = append(all, subRow{
					ClientID:          cl.ID,
					Topic:             filter,
					QoS:               sub.Qos,
					NoLocal:           sub.NoLocal,
					RetainAsPublished: sub.RetainAsPublished,
				})
			}
		}
		sort.Slice(all, func(i, j int) bool {
			if all[i].ClientID != all[j].ClientID {
				return all[i].ClientID < all[j].ClientID
			}
			return all[i].Topic < all[j].Topic
		})

		page := rest.Page[subRow]{
			Page:  params.Page,
			Size:  params.Size,
			Total: len(all),
			Items: rest.ApplyPagination(all, params),
		}
		d.Renderer.Render(w, "subscriptions", subPageData{
			Title:    "Subscriptions",
			User:     auth.UserFromContext(r.Context()),
			CSRF:     auth.NewCSRFToken(),
			Cluster:  d.Cluster,
			Topic:    r.URL.Query().Get("topic"),
			ClientID: r.URL.Query().Get("clientid"),
			Page:     page,
		})
	}
}
