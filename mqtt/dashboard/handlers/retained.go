// mqtt/dashboard/handlers/retained.go
package handlers

import (
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
	"github.com/wind-c/comqtt/v2/mqtt/rest"
)

const retainedDashboardPayloadCap = 4096

type RetainedDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type retainedRow struct {
	Topic        string
	TopicEncoded string
	QoS          byte
	Size         int
	StoredAt     int64
	PayloadHex   string
}

type retainedPageData struct {
	Title      string
	User       auth.User
	CSRF       string
	Cluster    bool
	Flash      string
	Topic      string
	IncludeSys bool
	Page       rest.Page[retainedRow]
	Readonly   bool
}

func RetainedList(d RetainedDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := rest.ParsePage(r.URL.Query())
		topicFilter := strings.ToLower(r.URL.Query().Get("topic"))
		// $SYS retained messages are hidden by default - they're broker
		// internals that crowd out the user's own retained traffic.
		// Tick the checkbox (?include_sys=1) to bring them back.
		includeSys := r.URL.Query().Get("include_sys") == "1"

		all := make([]retainedRow, 0, 64)
		for topic, pk := range d.Server.Topics.Retained.GetAll() {
			if !includeSys && strings.HasPrefix(topic, "$SYS/") {
				continue
			}
			if topicFilter != "" && !strings.Contains(strings.ToLower(topic), topicFilter) {
				continue
			}
			payload := pk.Payload
			if len(payload) > retainedDashboardPayloadCap {
				payload = payload[:retainedDashboardPayloadCap]
			}
			all = append(all, retainedRow{
				Topic:        topic,
				TopicEncoded: url.PathEscape(topic),
				QoS:          pk.FixedHeader.Qos,
				Size:         len(pk.Payload),
				StoredAt:     pk.Created,
				PayloadHex:   hex.Dump(payload),
			})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Topic < all[j].Topic })

		page := rest.Page[retainedRow]{
			Page:  params.Page,
			Size:  params.Size,
			Total: len(all),
			Items: rest.ApplyPagination(all, params),
		}
		u := auth.UserFromContext(r.Context())
		d.Renderer.Render(w, "retained", retainedPageData{
			Title:      "Retained",
			User:       u,
			CSRF:       auth.NewCSRFToken(),
			Cluster:    d.Cluster,
			Topic:      r.URL.Query().Get("topic"),
			IncludeSys: includeSys,
			Page:       page,
			Readonly:   u.Role != auth.RoleAdmin,
		})
	}
}

func RetainedClear(d RetainedDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.UserFromContext(r.Context()).Role != auth.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		topic, err := url.PathUnescape(r.PathValue("topic"))
		if err != nil || topic == "" {
			http.Error(w, "topic required", http.StatusBadRequest)
			return
		}
		if _, ok := d.Server.Topics.Retained.Get(topic); !ok {
			http.NotFound(w, r)
			return
		}
		d.Server.Topics.Retained.Delete(topic)
		http.Redirect(w, r, "/dashboard/retained", http.StatusFound)
	}
}
