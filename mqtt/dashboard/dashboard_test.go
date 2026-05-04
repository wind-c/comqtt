package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterServesPlaceholder(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, Options{Mode: "single"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "placeholder") {
		t.Fatalf("body: %q", rr.Body.String())
	}
}

func TestRedirectRoot(t *testing.T) {
	mux := http.NewServeMux()
	_ = Register(mux, Options{Mode: "single"})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("status: got %d want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/dashboard/" {
		t.Fatalf("location: %q", loc)
	}
}
