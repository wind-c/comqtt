// mqtt/dashboard/dashboard.go
package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/handlers"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/sse"
	"github.com/wind-c/comqtt/v2/mqtt/rest"
)

// Options bundles wiring choices for the dashboard.
type Options struct {
	// Cluster is true when running inside cmd/cluster. The nav menu shows
	// the Cluster link only when this is set.
	Cluster bool

	// ClusterAgent provides cluster topology to the Cluster page. nil in
	// single-mode; required when Cluster=true to mount /dashboard/cluster.
	ClusterAgent handlers.ClusterAgent

	// Server is the broker. Required.
	Server *mqtt.Server

	// Store is the credential store. If nil, a FileStore is created at
	// CredStorePath (defaulting to ./data/dashboard-users.json) and seeded
	// with username "admin" and a random password printed to stdout.
	Store auth.CredStore

	// CredStorePath is the file path for the FileStore. Used only when
	// Store is nil. Default: ./data/dashboard-users.json.
	CredStorePath string

	// Secret is the HMAC secret for the session cookie. If nil, one is
	// loaded from SecretPath (auto-generated 32-byte file) or from the
	// COMQTT_DASHBOARD_SESSION_SECRET env var (base64).
	Secret []byte

	// SecretPath is the file path for the auto-generated secret. Default:
	// ./data/dashboard-secret.
	SecretPath string

	// PasswordExpiryDays. 0 = never expire. Default: 90.
	PasswordExpiryDays int

	// Lockout configuration. Defaults: 5 fails / 5 min window / 10 min lock.
	LockoutThreshold int
	LockoutWindow    time.Duration
	LockoutDuration  time.Duration

	// SessionTTL. Default: 12h.
	SessionTTL time.Duration

	// Redis is the cluster-mode redis client. When set, the cred store and
	// HMAC secret are read from redis (not the file paths), and a pub/sub
	// bridge fans events between cluster nodes. nil in single-mode.
	Redis *redis.Client
}

// Routes returns the full set of HTTP routes for the dashboard, ready to be
// merged into another route map (e.g. rest.New(server).GenHandlers()) and
// registered onto the existing :8080 listener.
//
// Routes are returned as map[string]rest.Handler keyed on the Go 1.22+
// pattern syntax "METHOD /path/{params}". The static asset path uses the
// {path...} catch-all.
func Routes(opts Options) (map[string]rest.Handler, error) {
	if opts.Server == nil {
		return nil, errors.New("dashboard: Options.Server is required")
	}
	if err := opts.applyDefaults(); err != nil {
		return nil, err
	}

	var store auth.CredStore = opts.Store
	if store == nil {
		if opts.Redis != nil {
			rs := auth.NewRedisStore(opts.Redis, "comqtt:dashboard")
			pw, err := rs.Seed(context.Background(), "admin")
			if err != nil {
				return nil, err
			}
			if pw != "" {
				println("[dashboard] seeded admin password:", pw, "(rotate via /dashboard/account/password)")
			}
			store = rs
		} else {
			fs, err := auth.NewFileStore(opts.CredStorePath)
			if err != nil {
				return nil, err
			}
			pw, err := fs.Seed(context.Background(), "admin")
			if err != nil {
				return nil, err
			}
			if pw != "" {
				// Printed exactly once: first boot. Operators must rotate.
				println("[dashboard] seeded admin password:", pw, "(rotate via /dashboard/account/password)")
			}
			store = fs
		}
	}

	lockout := auth.NewLockout(auth.LockoutConfig{
		Threshold: opts.LockoutThreshold,
		Window:    opts.LockoutWindow,
		Duration:  opts.LockoutDuration,
	})

	hub := sse.NewHub(1024)
	// Register the broker hook so connect/publish/disconnect events reach
	// the hub. The hook id is unique per dashboard instance.
	if err := opts.Server.AddHook(&sse.HubHook{Hub: hub, Node: hostname()}, nil); err != nil {
		return nil, err
	}

	if opts.Redis != nil {
		br := sse.NewBridge(opts.Redis, hub, hostname())
		br.Start(context.Background())
	}

	rdr := handlers.NewRenderer(assetsFS)
	sampler := handlers.NewRateSampler(opts.Server)

	accountDeps := handlers.AccountDeps{
		Store:      store,
		Lockout:    lockout,
		Secret:     opts.Secret,
		Renderer:   rdr,
		SessionTTL: opts.SessionTTL,
	}
	overviewDeps := handlers.OverviewDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster, Sampler: sampler, Agent: opts.ClusterAgent}
	clientsDeps := handlers.ClientsDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	clientDetailDeps := handlers.ClientDetailDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	subscriptionsDeps := handlers.SubscriptionsDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	topicsDeps := handlers.TopicsDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	retainedDeps := handlers.RetainedDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	sessionsDeps := handlers.SessionsDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	blacklistDeps := handlers.BlacklistDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	toolsDeps := handlers.ToolsDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	settingsDeps := handlers.SettingsDeps{Server: opts.Server, Renderer: rdr, Cluster: opts.Cluster}
	usersDeps := handlers.UsersDeps{Store: store, Renderer: rdr, Cluster: opts.Cluster}

	// Auth wrappers.
	requireAuth := auth.RequireAuth(opts.Secret, store, opts.PasswordExpiryDays)
	requireAdmin := auth.RequireRole(auth.RoleAdmin)

	// Helper to wrap a HandlerFunc through requireAuth (and optionally requireAdmin).
	wrap := func(h http.HandlerFunc) rest.Handler {
		wrapped := requireAuth(h)
		return wrapped.ServeHTTP
	}
	wrapAdmin := func(h http.HandlerFunc) rest.Handler {
		wrapped := requireAuth(requireAdmin(h))
		return wrapped.ServeHTTP
	}

	staticHandler := http.StripPrefix("/dashboard/", http.FileServerFS(assetsFS))

	routes := map[string]rest.Handler{
		// Public.
		"GET /{$}":              rootRedirect,
		"GET /dashboard/login":  handlers.LoginGet(accountDeps),
		"POST /dashboard/login": handlers.LoginPost(accountDeps),
		"POST /dashboard/logout": handlers.LogoutPost(),
		"GET /dashboard/static/": staticHandler.ServeHTTP,

		// Authenticated pages.
		"GET /dashboard/{$}":                       wrap(handlers.OverviewGet(overviewDeps)),
		"GET /dashboard/fragments/overview-cards":  wrap(handlers.OverviewCards(overviewDeps)),
		"GET /dashboard/clients":                   wrap(handlers.ClientsList(clientsDeps)),
		"GET /dashboard/clients/{id}":              wrap(handlers.ClientDetail(clientDetailDeps)),
		"POST /dashboard/clients/{id}/subscriptions/{topic}/delete": wrapAdmin(handlers.ClientUnsubscribe(clientDetailDeps)),
		"GET /dashboard/subscriptions":             wrap(handlers.SubscriptionsList(subscriptionsDeps)),
		"GET /dashboard/topics":                    wrap(handlers.TopicsTree(topicsDeps)),
		"GET /dashboard/retained":                  wrap(handlers.RetainedList(retainedDeps)),
		"POST /dashboard/retained/{topic}/delete":  wrapAdmin(handlers.RetainedClear(retainedDeps)),
		"GET /dashboard/sessions":                  wrap(handlers.SessionsList(sessionsDeps)),
		"POST /dashboard/sessions/{id}/delete":     wrapAdmin(handlers.SessionsClear(sessionsDeps)),
		"GET /dashboard/blacklist":                 wrap(handlers.BlacklistGet(blacklistDeps)),
		"POST /dashboard/blacklist":                wrapAdmin(handlers.BlacklistAdd(blacklistDeps)),
		"POST /dashboard/blacklist/{id}/delete":    wrapAdmin(handlers.BlacklistRemove(blacklistDeps)),
		"GET /dashboard/tools":                     wrap(handlers.ToolsGet(toolsDeps)),
		"POST /dashboard/tools/publish":            wrapAdmin(handlers.ToolsPublish(toolsDeps)),
		"GET /dashboard/settings":                  wrap(handlers.Settings(settingsDeps)),
		"GET /dashboard/account":                   wrap(handlers.AccountGet(accountDeps)),
		"GET /dashboard/account/password":          wrap(handlers.ChangePasswordGet(accountDeps)),
		"POST /dashboard/account/password":         wrap(handlers.ChangePasswordPost(accountDeps)),

		// Admin-only pages.
		"GET /dashboard/users":                     wrapAdmin(handlers.UsersList(usersDeps)),
		"POST /dashboard/users":                    wrapAdmin(handlers.UsersCreate(usersDeps)),
		"POST /dashboard/users/{username}/delete":  wrapAdmin(handlers.UsersDelete(usersDeps)),
		"POST /dashboard/users/{username}/role":    wrapAdmin(handlers.UsersToggleRole(usersDeps)),

		// SSE.
		"GET /dashboard/events": wrap(handlers.Events(hub)),
	}

	if opts.Cluster && opts.ClusterAgent != nil {
		routes["GET /dashboard/cluster"] = wrap(handlers.ClusterPage(handlers.ClusterDeps{
			Agent: opts.ClusterAgent, Renderer: rdr, Cluster: true,
		}))
	}

	return routes, nil
}

func rootRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard/", http.StatusFound)
}

func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown"
}

func (o *Options) applyDefaults() error {
	if o.CredStorePath == "" {
		o.CredStorePath = "./data/dashboard-users.json"
	}
	if o.SecretPath == "" {
		o.SecretPath = "./data/dashboard-secret"
	}
	if o.PasswordExpiryDays == 0 {
		o.PasswordExpiryDays = 90
	}
	if o.LockoutThreshold == 0 {
		o.LockoutThreshold = 5
	}
	if o.LockoutWindow == 0 {
		o.LockoutWindow = 5 * time.Minute
	}
	if o.LockoutDuration == 0 {
		o.LockoutDuration = 10 * time.Minute
	}
	if o.SessionTTL == 0 {
		o.SessionTTL = 12 * time.Hour
	}
	if o.Secret == nil {
		// In cluster mode, prefer the redis-backed secret so all nodes
		// share the same HMAC key. Fall through to env/file on error.
		if o.Redis != nil {
			if b, err := auth.EnsureSecret(context.Background(), o.Redis, "comqtt:dashboard:secret"); err == nil && len(b) >= 16 {
				o.Secret = b
				return nil
			}
		}
		// Try env first.
		if env := os.Getenv("COMQTT_DASHBOARD_SESSION_SECRET"); env != "" {
			b, err := base64.StdEncoding.DecodeString(env)
			if err == nil && len(b) >= 16 {
				o.Secret = b
				return nil
			}
		}
		// Then file.
		b, err := os.ReadFile(o.SecretPath)
		if err == nil && len(b) >= 16 {
			o.Secret = b
			return nil
		}
		// Generate.
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(o.SecretPath), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(o.SecretPath, buf, 0o600); err != nil {
			return err
		}
		o.Secret = buf
	}
	return nil
}
