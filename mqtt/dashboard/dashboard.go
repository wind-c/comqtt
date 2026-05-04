package dashboard

import (
	"net/http"

	"github.com/wind-c/comqtt/v2/config"
)

// Options is what cmd/single and cmd/cluster pass to wire the dashboard
// into their existing http.ServeMux.
type Options struct {
	Mode       string // "single" or "cluster"
	Cfg        *config.Config
	StaticOnly bool // for testing
}

// Register attaches /dashboard/* and the SSE handler onto the given mux.
// Wiring of new /api/v1/* endpoints stays in mqtt/rest and cluster/rest.
func Register(mux *http.ServeMux, opts Options) error {
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dashboard/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
	mux.Handle("GET /dashboard/static/", http.StripPrefix("/dashboard/", http.FileServerFS(assetsFS)))
	mux.HandleFunc("GET /dashboard/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("comqtt dashboard placeholder"))
	})
	return nil
}
