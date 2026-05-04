// mqtt/dashboard/auth/middleware_test.go
package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupForMiddleware seeds an admin in a fresh FileStore. The user is
// returned in the must_change=true state. Callers that want a "settled"
// admin should call store.SetPassword to clear the flag.
func setupForMiddleware(t *testing.T) (*FileStore, []byte, string) {
	t.Helper()
	store, _ := NewFileStore(filepath.Join(t.TempDir(), "users.json"))
	_, _ = store.Seed(context.Background(), "admin")
	secret := []byte("0123456789abcdef0123456789abcdef")
	cookie, _ := Sign(secret, SessionPayload{Username: "admin", Role: string(RoleAdmin), Exp: time.Now().Add(time.Hour).Unix()})
	return store, secret, cookie
}

func TestRequireAuthRedirectsAnon(t *testing.T) {
	store, secret, _ := setupForMiddleware(t)
	mw := RequireAuth(secret, store, 90)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/dashboard/", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.HasPrefix(rr.Header().Get("Location"), "/dashboard/login") {
		t.Fatalf("redirect: %q", rr.Header().Get("Location"))
	}
}

func TestRequireAuthAcceptsValidCookie(t *testing.T) {
	store, secret, cookie := setupForMiddleware(t)
	// Clear must_change so the handler is allowed to run.
	if err := store.SetPassword(context.Background(), "admin", "newpass1234"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	mw := RequireAuth(secret, store, 90)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		w.Write([]byte("hello " + u.Username))
	}))
	req := httptest.NewRequest("GET", "/dashboard/", nil)
	req.AddCookie(&http.Cookie{Name: "comqtt_session", Value: cookie})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) != "hello admin" {
		t.Fatalf("body: %q", body)
	}
}

func TestRequireAuthForceRotateRedirects(t *testing.T) {
	store, secret, cookie := setupForMiddleware(t)
	// Seed leaves must_change=true; we want this state.
	mw := RequireAuth(secret, store, 90)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	req := httptest.NewRequest("GET", "/dashboard/", nil)
	req.AddCookie(&http.Cookie{Name: "comqtt_session", Value: cookie})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.HasPrefix(rr.Header().Get("Location"), "/dashboard/account/password") {
		t.Fatalf("redirect: %q", rr.Header().Get("Location"))
	}
}

func TestRequireRoleRejectsViewerOnAdminRoute(t *testing.T) {
	mw := RequireRole(RoleAdmin)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	req := httptest.NewRequest("DELETE", "/api/v1/whatever", nil)
	ctx := WithUser(req.Context(), User{Username: "bob", Role: RoleViewer})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: %d", rr.Code)
	}
}
