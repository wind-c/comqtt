// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package dashboard

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

type Options struct {
	AuthSecret string
	UsersFile  string
	IsCluster  bool
	SiteTitle  string
}

type Dashboard struct {
	opts     Options
	users    []User
	pages    map[string]*template.Template
	loginTpl *template.Template
}

//go:embed templates/* static/*
var assets embed.FS

func New(opts Options) (*Dashboard, error) {
	users, err := loadUsers(opts.UsersFile)
	if err != nil {
		return nil, err
	}

	if opts.SiteTitle == "" {
		opts.SiteTitle = "Comqtt Dashboard"
	}

	d := &Dashboard{
		opts:  opts,
		users: users,
	}

	if err := d.parseTemplates(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Dashboard) parseTemplates() error {
	d.pages = make(map[string]*template.Template)

	loginTpl, err := template.New("login").ParseFS(assets, "templates/login.html")
	if err != nil {
		return err
	}
	d.loginTpl = loginTpl

	base, err := template.New("").ParseFS(assets, "templates/layout.html")
	if err != nil {
		return err
	}
	layout := base.Lookup("layout")
	if layout == nil {
		return errLayoutNotFound
	}

	pages := []string{"overview", "clients", "subscriptions", "retained", "nodes", "publish", "auth", "acl"}
	for _, name := range pages {
		page, err := layout.Clone()
		if err != nil {
			return err
		}
		if _, err := page.ParseFS(assets, "templates/" + name + ".html"); err != nil {
			return err
		}
		d.pages[name] = page
	}
	return nil
}

func (d *Dashboard) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /dashboard/login", d.HandleLogin)
	mux.HandleFunc("POST /dashboard/logout", d.HandleLogout)
	mux.HandleFunc("GET /dashboard/login", d.serveLogin)

	staticFS, _ := fs.Sub(assets, "static")
	mux.Handle("GET /dashboard/static/", http.StripPrefix("/dashboard/static/", http.FileServer(http.FS(staticFS))))

	protected := http.NewServeMux()
	protected.HandleFunc("GET /dashboard/", d.servePage("overview"))
	protected.HandleFunc("GET /dashboard/overview", d.servePage("overview"))
	protected.HandleFunc("GET /dashboard/clients", d.servePage("clients"))
	protected.HandleFunc("GET /dashboard/subscriptions", d.servePage("subscriptions"))


	protected.HandleFunc("GET /dashboard/retained", d.servePage("retained"))
	protected.HandleFunc("GET /dashboard/nodes", d.servePage("nodes"))
	protected.HandleFunc("GET /dashboard/publish", d.servePage("publish"))
	protected.HandleFunc("GET /dashboard/auth", d.servePage("auth"))
	protected.HandleFunc("GET /dashboard/acl", d.servePage("acl"))

	mux.Handle("/dashboard/", d.AuthMiddleware(protected))

	return mux
}

func (d *Dashboard) serveLogin(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"SiteTitle": d.opts.SiteTitle,
	}
	if err := d.loginTpl.ExecuteTemplate(w, "login", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (d *Dashboard) servePage(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := d.pages[name]
		if page == nil {
			http.Error(w, "template not found: "+name, http.StatusInternalServerError)
			return
		}
		data := map[string]any{
			"SiteTitle": d.opts.SiteTitle,
			"IsCluster": d.opts.IsCluster,
		}
		if err := page.ExecuteTemplate(w, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
