// mqtt/dashboard/handlers/client_detail.go
package handlers

import (
	"net/http"
	"net/url"
	"sort"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
)

// ClientDetailDeps bundles dependencies for the Client detail page.
type ClientDetailDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type clientDetailPageData struct {
	Title    string
	User     auth.User
	CSRF     string
	Cluster  bool
	Flash    string
	Error    string
	Readonly bool

	ClientID string
	Online   bool
	Tab      string
	SubCount int
	Info     clientInfoSection
	Subs     []subscriptionRow
}

type clientInfoSection struct {
	Username        string
	Remote          string
	Listener        string
	ProtocolVersion byte
	Keepalive       uint16
	Inflight        int
	Clean           bool
}

type subscriptionRow struct {
	Topic        string
	TopicEncoded string
	QoS          byte
}

// ClientDetail handles GET /dashboard/clients/{id}.
func ClientDetail(d ClientDetailDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "client id required", http.StatusBadRequest)
			return
		}
		tab := r.URL.Query().Get("tab")
		if tab == "" {
			tab = "info"
		}

		u := auth.UserFromContext(r.Context())
		page := clientDetailPageData{
			Title:    "Client " + id,
			User:     u,
			CSRF:     auth.NewCSRFToken(),
			Cluster:  d.Cluster,
			Readonly: u.Role != auth.RoleAdmin,
			ClientID: id,
			Tab:      tab,
		}

		cl, online := d.Server.Clients.Get(id)
		if !online {
			http.NotFound(w, r)
			return
		}
		page.Online = true
		page.Info = clientInfoSection{
			Username:        string(cl.Properties.Username),
			Remote:          cl.Net.Remote,
			Listener:        cl.Net.Listener,
			ProtocolVersion: cl.Properties.ProtocolVersion,
			Keepalive:       cl.State.Keepalive,
			Inflight:        cl.State.Inflight.Len(),
			Clean:           cl.Properties.Clean,
		}

		subsMap := cl.State.Subscriptions.GetAll()
		page.SubCount = len(subsMap)
		page.Subs = make([]subscriptionRow, 0, len(subsMap))
		for filter, sub := range subsMap {
			page.Subs = append(page.Subs, subscriptionRow{
				Topic:        filter,
				TopicEncoded: url.PathEscape(filter),
				QoS:          sub.Qos,
			})
		}
		sort.Slice(page.Subs, func(i, j int) bool { return page.Subs[i].Topic < page.Subs[j].Topic })

		d.Renderer.Render(w, "client_detail", page)
	}
}

// ClientUnsubscribe handles POST /dashboard/clients/{id}/subscriptions/{topic}/delete.
// Admin-only. Removes the subscription from both the broker's topic trie
// (Topics.Unsubscribe) and the per-client map (Subscriptions.Delete) -
// matches the REST handler in mqtt/rest/client_unsubscribe.go.
func ClientUnsubscribe(d ClientDetailDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.UserFromContext(r.Context()).Role != auth.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id := r.PathValue("id")
		topic, err := url.PathUnescape(r.PathValue("topic"))
		if err != nil || topic == "" || id == "" {
			http.Error(w, "missing or malformed id/topic", http.StatusBadRequest)
			return
		}
		cl, ok := d.Server.Clients.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if _, has := cl.State.Subscriptions.Get(topic); !has {
			http.NotFound(w, r)
			return
		}
		d.Server.Topics.Unsubscribe(topic, id)
		cl.State.Subscriptions.Delete(topic)
		http.Redirect(w, r, "/dashboard/clients/"+id+"?tab=subs", http.StatusFound)
	}
}
