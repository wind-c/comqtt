// mqtt/dashboard/handlers/users_test.go
package handlers

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
)

func newUsersRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/users.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "users"}}{{template "layout" .}}{{end}}{{define "content"}}{{with .Error}}<div class="err">{{.}}</div>{{end}}{{range .Items}}<div class="row">{{.Username}}={{.Role}} mc={{.MustChange}} locked={{.Locked}}</div>{{else}}<div class="empty">none</div>{{end}}{{end}}`)},
	}))
}

func newUsersDeps(t *testing.T) (UsersDeps, *auth.FileStore) {
	t.Helper()
	store, err := auth.NewFileStore(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	_, _ = store.Seed(context.Background(), "admin")
	return UsersDeps{Store: store, Renderer: newUsersRenderer(t)}, store
}

func TestUsersListShowsSeed(t *testing.T) {
	deps, _ := newUsersDeps(t)
	rr := httptest.NewRecorder()
	UsersList(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/users", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "admin=admin") {
		t.Fatalf("expected seeded admin: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "mc=true") {
		t.Fatalf("expected must_change=true: %q", rr.Body.String())
	}
}

func TestUsersCreateHappy(t *testing.T) {
	deps, store := newUsersDeps(t)
	form := url.Values{"username": {"bob"}, "password": {"bobpass1234"}, "role": {"viewer"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	UsersCreate(deps)(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	users, _ := store.ListUsers(context.Background())
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestUsersCreateInvalid(t *testing.T) {
	deps, _ := newUsersDeps(t)
	form := url.Values{"username": {""}, "password": {"x"}, "role": {"admin"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	UsersCreate(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Invalid input") {
		t.Fatalf("expected error: %q", rr.Body.String())
	}
}

func TestUsersDeleteRefusesLastAdmin(t *testing.T) {
	deps, _ := newUsersDeps(t)
	req := httptest.NewRequest(http.MethodPost, "/dashboard/users/admin/delete", nil)
	req.SetPathValue("username", "admin")
	rr := httptest.NewRecorder()
	UsersDelete(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Cannot delete the last admin") {
		t.Fatalf("expected last-admin error: %q", rr.Body.String())
	}
}

func TestUsersDeleteHappyPath(t *testing.T) {
	deps, store := newUsersDeps(t)
	_ = store.CreateUser(context.Background(), "bob", "bobpass1234", auth.RoleViewer)
	req := httptest.NewRequest(http.MethodPost, "/dashboard/users/bob/delete", nil)
	req.SetPathValue("username", "bob")
	rr := httptest.NewRecorder()
	UsersDelete(deps)(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
}

func TestUsersToggleRolePromotesViewer(t *testing.T) {
	deps, store := newUsersDeps(t)
	_ = store.CreateUser(context.Background(), "bob", "bobpass1234", auth.RoleViewer)
	req := httptest.NewRequest(http.MethodPost, "/dashboard/users/bob/role", nil)
	req.SetPathValue("username", "bob")
	rr := httptest.NewRecorder()
	UsersToggleRole(deps)(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	u, _ := store.GetUser(context.Background(), "bob")
	if u.Role != auth.RoleAdmin {
		t.Fatalf("expected admin: %s", u.Role)
	}
}

func TestUsersToggleRoleRefusesLastAdmin(t *testing.T) {
	deps, _ := newUsersDeps(t)
	req := httptest.NewRequest(http.MethodPost, "/dashboard/users/admin/role", nil)
	req.SetPathValue("username", "admin")
	rr := httptest.NewRecorder()
	UsersToggleRole(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Cannot demote the last admin") {
		t.Fatalf("expected demote error: %q", rr.Body.String())
	}
}
