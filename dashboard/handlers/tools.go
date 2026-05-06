// dashboard/handlers/tools.go
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/dashboard/auth"
)

// ToolsDeps bundles dependencies for the Tools page.
type ToolsDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type publishForm struct {
	Topic   string
	Payload string
	QoS     int
	Retain  bool
}

type toolsPageData struct {
	Title    string
	User     auth.User
	CSRF     string
	Cluster  bool
	Flash    string
	Error    string
	Form     publishForm
	Readonly bool
}

// ToolsGet renders the publish-a-message form.
func ToolsGet(d ToolsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromContext(r.Context())
		d.Renderer.Render(w, "tools", toolsPageData{
			Title:    "Tools",
			User:     u,
			CSRF:     auth.NewCSRFToken(),
			Cluster:  d.Cluster,
			Form:     publishForm{},
			Readonly: u.Role != auth.RoleAdmin,
		})
	}
}

// ToolsPublish handles POST /dashboard/tools/publish. Admin-only.
func ToolsPublish(d ToolsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromContext(r.Context())
		if u.Role != auth.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = r.ParseForm()
		form := publishForm{
			Topic:   strings.TrimSpace(r.PostFormValue("topic")),
			Payload: r.PostFormValue("payload"),
			Retain:  r.PostFormValue("retain") != "",
		}
		qosRaw := r.PostFormValue("qos")
		qos, err := strconv.Atoi(qosRaw)
		if err != nil || qos < 0 || qos > 2 {
			renderToolsError(d, w, r, form, "QoS must be 0, 1, or 2.")
			return
		}
		form.QoS = qos
		if form.Topic == "" {
			renderToolsError(d, w, r, form, "Topic is required.")
			return
		}
		if err := d.Server.Publish(form.Topic, []byte(form.Payload), form.Retain, byte(qos)); err != nil {
			renderToolsError(d, w, r, form, "Publish failed: "+err.Error())
			return
		}
		d.Renderer.Render(w, "tools", toolsPageData{
			Title:    "Tools",
			User:     u,
			CSRF:     auth.NewCSRFToken(),
			Cluster:  d.Cluster,
			Flash:    "Published to " + form.Topic + ".",
			Form:     publishForm{},
			Readonly: false,
		})
	}
}

func renderToolsError(d ToolsDeps, w http.ResponseWriter, r *http.Request, form publishForm, msg string) {
	w.WriteHeader(http.StatusBadRequest)
	u := auth.UserFromContext(r.Context())
	d.Renderer.Render(w, "tools", toolsPageData{
		Title:    "Tools",
		User:     u,
		CSRF:     auth.NewCSRFToken(),
		Cluster:  d.Cluster,
		Error:    msg,
		Form:     form,
		Readonly: u.Role != auth.RoleAdmin,
	})
}
