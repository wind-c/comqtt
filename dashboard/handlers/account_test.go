// dashboard/handlers/account_test.go
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
	"time"

	"github.com/wind-c/comqtt/v2/dashboard/auth"
)

const loginTemplate = `{{define "login"}}<form>{{if .Error}}<p class="err">{{.Error}}</p>{{end}}<input name="next" value="{{.Next}}"></form>{{end}}`

const passwordTemplate = `{{define "account/password"}}<form>{{if .Error}}<p class="err">{{.Error}}</p>{{end}}<p>{{.Reason}}</p></form>{{end}}`

const personalTemplate = `{{define "layout"}}<html>{{template "content" .}}</html>{{end}}{{define "account_personal"}}{{template "layout" .}}{{end}}{{define "content"}}<div>user={{.Account.Username}} role={{.Account.Role}} mc={{.Account.MustChange}} locked={{.Account.Locked}} pset={{.Account.PasswordSetAt}}</div>{{end}}`

func newRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fakeTplFS(t))
}

func fakeTplFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"templates/login.html":            &fstest.MapFile{Data: []byte(loginTemplate)},
		"templates/account/password.html": &fstest.MapFile{Data: []byte(passwordTemplate)},
		"templates/account/personal.html": &fstest.MapFile{Data: []byte(personalTemplate)},
	}
}

func newAccountDeps(t *testing.T) (AccountDeps, *auth.FileStore) {
	t.Helper()
	store, err := auth.NewFileStore(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return AccountDeps{
		Store:    store,
		Lockout:  auth.NewLockout(auth.LockoutConfig{Threshold: 3, Window: time.Minute, Duration: 10 * time.Minute}),
		Secret:   []byte("0123456789abcdef0123456789abcdef"),
		Renderer: newRenderer(t),
	}, store
}

func TestLoginGetRendersForm(t *testing.T) {
	deps, _ := newAccountDeps(t)
	rr := httptest.NewRecorder()
	LoginGet(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/login", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<form>") {
		t.Fatalf("body: %q", rr.Body.String())
	}
}

func TestLoginPostValidCredentialsSetsCookie(t *testing.T) {
	deps, store := newAccountDeps(t)
	pw, _ := store.Seed(context.Background(), "admin")
	form := url.Values{"username": {"admin"}, "password": {pw}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	LoginPost(deps)(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "comqtt_session" || cookies[0].Value == "" {
		t.Fatalf("cookie not set: %+v", cookies)
	}
	if !cookies[0].HttpOnly {
		t.Fatal("cookie should be HttpOnly")
	}
}

func TestLoginPostBadCredsRecordsLockout(t *testing.T) {
	deps, store := newAccountDeps(t)
	_, _ = store.Seed(context.Background(), "admin")
	form := url.Values{"username": {"admin"}, "password": {"wrong"}}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		LoginPost(deps)(rr, req)
	}
	locked, _ := deps.Lockout.IsLocked("admin")
	if !locked {
		t.Fatal("expected lockout after threshold failures")
	}
}

func TestLoginPostLockedShowsError(t *testing.T) {
	deps, store := newAccountDeps(t)
	pw, _ := store.Seed(context.Background(), "admin")
	for i := 0; i < 3; i++ {
		deps.Lockout.Record("admin")
	}
	form := url.Values{"username": {"admin"}, "password": {pw}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	LoginPost(deps)(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "locked") {
		t.Fatalf("expected locked message: %q", rr.Body.String())
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	rr := httptest.NewRecorder()
	LogoutPost()(rr, httptest.NewRequest(http.MethodPost, "/dashboard/logout", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge >= 0 {
		t.Fatalf("cookie should be expired: %+v", cookies)
	}
}

func TestSanitizeNext(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "/dashboard/"},
		{"/dashboard/clients", "/dashboard/clients"},
		{"/etc/passwd", "/dashboard/"},
		{"//evil.com/x", "/dashboard/"},
		{"https://evil.com/x", "/dashboard/"},
		{"javascript:alert(1)", "/dashboard/"},
	}
	for _, c := range cases {
		if got := sanitizeNext(c.in); got != c.want {
			t.Errorf("sanitizeNext(%q): got %q want %q", c.in, got, c.want)
		}
	}
}

func TestChangePasswordGetRenders(t *testing.T) {
	deps, _ := newAccountDeps(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard/account/password?reason=must_change", nil)
	ChangePasswordGet(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "must_change") {
		t.Fatalf("expected reason in body: %q", rr.Body.String())
	}
}

func TestChangePasswordPostHappyPath(t *testing.T) {
	deps, store := newAccountDeps(t)
	pw, _ := store.Seed(context.Background(), "admin")
	form := url.Values{"current": {pw}, "new": {"newpass1234"}, "confirm": {"newpass1234"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ChangePasswordPost(deps)(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if _, err := store.Authenticate(context.Background(), "admin", "newpass1234"); err != nil {
		t.Fatalf("new password should work: %v", err)
	}
	u, _ := store.GetUser(context.Background(), "admin")
	if u.MustChange {
		t.Fatal("must_change should be cleared")
	}
}

func TestChangePasswordPostMismatchedConfirm(t *testing.T) {
	deps, store := newAccountDeps(t)
	pw, _ := store.Seed(context.Background(), "admin")
	form := url.Values{"current": {pw}, "new": {"newpass1234"}, "confirm": {"different5678"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ChangePasswordPost(deps)(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "do not match") {
		t.Fatalf("expected mismatch message: %q", rr.Body.String())
	}
}

func TestChangePasswordPostShortPassword(t *testing.T) {
	deps, store := newAccountDeps(t)
	pw, _ := store.Seed(context.Background(), "admin")
	form := url.Values{"current": {pw}, "new": {"short"}, "confirm": {"short"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ChangePasswordPost(deps)(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "8 characters") {
		t.Fatalf("expected length error: %q", rr.Body.String())
	}
}

func TestChangePasswordPostWrongCurrent(t *testing.T) {
	deps, store := newAccountDeps(t)
	_, _ = store.Seed(context.Background(), "admin")
	form := url.Values{"current": {"wrong"}, "new": {"newpass1234"}, "confirm": {"newpass1234"}}
	req := httptest.NewRequest(http.MethodPost, "/dashboard/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	ChangePasswordPost(deps)(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
}

func TestAccountGetRendersDetails(t *testing.T) {
	deps, store := newAccountDeps(t)
	pw, _ := store.Seed(context.Background(), "admin")
	if err := store.SetPassword(context.Background(), "admin", "newpass1234"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	_ = pw

	req := httptest.NewRequest(http.MethodGet, "/dashboard/account", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.User{Username: "admin", Role: auth.RoleAdmin}))
	rr := httptest.NewRecorder()
	AccountGet(deps)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "user=admin") {
		t.Fatalf("expected username: %q", body)
	}
	if !strings.Contains(body, "role=admin") {
		t.Fatalf("expected role: %q", body)
	}
	if !strings.Contains(body, "mc=false") {
		t.Fatalf("expected must_change cleared: %q", body)
	}
	if strings.Contains(body, "pset=never") {
		t.Fatalf("PasswordSetAt should be a timestamp not 'never': %q", body)
	}
}

func TestAccountGetUnauthorizedWithoutContext(t *testing.T) {
	deps, _ := newAccountDeps(t)
	rr := httptest.NewRecorder()
	AccountGet(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/account", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d", rr.Code)
	}
}
