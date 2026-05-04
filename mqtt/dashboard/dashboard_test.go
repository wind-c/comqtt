// mqtt/dashboard/dashboard_test.go
package dashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wind-c/comqtt/v2/mqtt"
)

func TestRoutesRequiresServer(t *testing.T) {
	if _, _, err := Routes(Options{}); err == nil {
		t.Fatal("expected error for missing Server")
	}
}

func TestRoutesIncludesExpectedKeys(t *testing.T) {
	server := mqtt.New(nil)
	dir := t.TempDir()
	rs, cleanup, err := Routes(Options{
		Server:        server,
		CredStorePath: filepath.Join(dir, "users.json"),
		SecretPath:    filepath.Join(dir, "secret"),
	})
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	defer cleanup()
	for _, want := range []string{
		"GET /{$}",
		"GET /dashboard/login",
		"POST /dashboard/login",
		"POST /dashboard/logout",
		"GET /dashboard/static/",
		"GET /dashboard/{$}",
		"GET /dashboard/clients",
		"GET /dashboard/users",
		"GET /dashboard/events",
	} {
		if _, ok := rs[want]; !ok {
			t.Errorf("missing route: %q", want)
		}
	}
}

func TestRoutesRedirectFromRoot(t *testing.T) {
	server := mqtt.New(nil)
	dir := t.TempDir()
	rs, cleanup, _ := Routes(Options{
		Server:        server,
		CredStorePath: filepath.Join(dir, "users.json"),
		SecretPath:    filepath.Join(dir, "secret"),
	})
	defer cleanup()
	mux := http.NewServeMux()
	for path, handler := range rs {
		mux.HandleFunc(path, handler)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/dashboard/") {
		t.Fatalf("location: %q", loc)
	}
}

func TestRoutesAnonRedirectsToLogin(t *testing.T) {
	server := mqtt.New(nil)
	dir := t.TempDir()
	rs, cleanup, _ := Routes(Options{
		Server:        server,
		CredStorePath: filepath.Join(dir, "users.json"),
		SecretPath:    filepath.Join(dir, "secret"),
	})
	defer cleanup()
	mux := http.NewServeMux()
	for path, handler := range rs {
		mux.HandleFunc(path, handler)
	}
	// Anon GET /dashboard/clients should redirect to /dashboard/login.
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/dashboard/clients", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/dashboard/login") {
		t.Fatalf("location: %q", loc)
	}
}

func TestRoutesStaticServesAsset(t *testing.T) {
	server := mqtt.New(nil)
	dir := t.TempDir()
	rs, cleanup, _ := Routes(Options{
		Server:        server,
		CredStorePath: filepath.Join(dir, "users.json"),
		SecretPath:    filepath.Join(dir, "secret"),
	})
	defer cleanup()
	mux := http.NewServeMux()
	for path, handler := range rs {
		mux.HandleFunc(path, handler)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/dashboard/static/tailwind.css", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "tailwindcss") {
		body := rr.Body.String()
		if len(body) > 200 {
			body = body[:200]
		}
		t.Fatalf("expected CSS body: %q", body)
	}
}
