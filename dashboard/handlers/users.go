// dashboard/handlers/users.go
package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wind-c/comqtt/v2/dashboard/auth"
)

// UsersDeps bundles dependencies for the Users page.
type UsersDeps struct {
	Store    auth.CredStore
	Renderer *Renderer
	Cluster  bool
}

type usersPageData struct {
	Title   string
	User    auth.User
	CSRF    string
	Cluster bool
	Flash   string
	Error   string
	Items   []userRow
}

type userRow struct {
	Username   string
	Role       string
	MustChange bool
	Locked     bool
}

// UsersList handles GET /dashboard/users. Admin-only - the route should be
// wrapped in RequireRole(RoleAdmin) by the mounter.
func UsersList(d UsersDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderUsersPage(d, w, r, "", "")
	}
}

// UsersCreate handles POST /dashboard/users. Admin-only.
func UsersCreate(d UsersDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		username := strings.TrimSpace(r.PostFormValue("username"))
		password := r.PostFormValue("password")
		role := auth.Role(r.PostFormValue("role"))

		if username == "" || len(password) < minPasswordLen || !role.Valid() {
			renderUsersPage(d, w, r, "", "Invalid input. Username required, password 8+ chars, role admin or viewer.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := d.Store.CreateUser(ctx, username, password, role); err != nil {
			if errors.Is(err, auth.ErrUserExists) {
				renderUsersPage(d, w, r, "", "User already exists.")
				return
			}
			renderUsersPage(d, w, r, "", "Failed to create user: "+err.Error())
			return
		}
		http.Redirect(w, r, "/dashboard/users", http.StatusFound)
	}
}

// UsersDelete handles POST /dashboard/users/{username}/delete. Admin-only.
func UsersDelete(d UsersDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")
		if username == "" {
			http.Error(w, "username required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := d.Store.DeleteUser(ctx, username); err != nil {
			if errors.Is(err, auth.ErrCannotDeleteLastAdmin) {
				renderUsersPage(d, w, r, "", "Cannot delete the last admin.")
				return
			}
			if errors.Is(err, auth.ErrUserNotFound) {
				http.NotFound(w, r)
				return
			}
			renderUsersPage(d, w, r, "", "Failed to delete: "+err.Error())
			return
		}
		http.Redirect(w, r, "/dashboard/users", http.StatusFound)
	}
}

// UsersToggleRole handles POST /dashboard/users/{username}/role. Admin-only.
// Flips the role between admin and viewer. Refuses to demote the last admin.
func UsersToggleRole(d UsersDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")
		if username == "" {
			http.Error(w, "username required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		users, err := d.Store.ListUsers(ctx)
		if err != nil {
			renderUsersPage(d, w, r, "", "Failed to load users: "+err.Error())
			return
		}
		var current auth.User
		admins := 0
		for _, u := range users {
			if u.Role == auth.RoleAdmin {
				admins++
			}
			if u.Username == username {
				current = u
			}
		}
		if current.Username == "" {
			http.NotFound(w, r)
			return
		}
		newRole := auth.RoleAdmin
		if current.Role == auth.RoleAdmin {
			if admins == 1 {
				renderUsersPage(d, w, r, "", "Cannot demote the last admin.")
				return
			}
			newRole = auth.RoleViewer
		}
		if err := d.Store.SetRole(ctx, username, newRole); err != nil {
			renderUsersPage(d, w, r, "", "Failed to toggle role: "+err.Error())
			return
		}
		http.Redirect(w, r, "/dashboard/users", http.StatusFound)
	}
}

func renderUsersPage(d UsersDeps, w http.ResponseWriter, r *http.Request, flash, errMsg string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	users, err := d.Store.ListUsers(ctx)
	if err != nil {
		http.Error(w, "list users: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rows := make([]userRow, 0, len(users))
	now := time.Now().Unix()
	for _, u := range users {
		rows = append(rows, userRow{
			Username:   u.Username,
			Role:       string(u.Role),
			MustChange: u.MustChange,
			Locked:     u.LockedUntil > now,
		})
	}
	d.Renderer.Render(w, "users", usersPageData{
		Title:   "Users",
		User:    auth.UserFromContext(r.Context()),
		CSRF:    auth.NewCSRFToken(),
		Cluster: d.Cluster,
		Flash:   flash,
		Error:   errMsg,
		Items:   rows,
	})
}
