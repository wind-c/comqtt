// mqtt/dashboard/handlers/account.go
package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
)

// AccountDeps bundles the dependencies the account handlers need.
type AccountDeps struct {
	Store    auth.CredStore
	Lockout  *auth.Lockout
	Secret   []byte
	Renderer *Renderer
	// SessionTTL is the cookie Max-Age; defaults to 12h if zero.
	SessionTTL time.Duration
}

func (d AccountDeps) ttl() time.Duration {
	if d.SessionTTL == 0 {
		return 12 * time.Hour
	}
	return d.SessionTTL
}

// LoginGet renders the login form.
func LoginGet(d AccountDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Renderer.Render(w, "login", map[string]any{
			"CSRF":  auth.NewCSRFToken(),
			"Next":  sanitizeNext(r.URL.Query().Get("next")),
			"Error": "",
		})
	}
}

// LoginPost validates credentials, sets the session cookie on success, or
// re-renders the form with an error.
func LoginPost(d AccountDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		username := strings.TrimSpace(r.PostFormValue("username"))
		password := r.PostFormValue("password")
		next := sanitizeNext(r.PostFormValue("next"))

		if locked, until := d.Lockout.IsLocked(username); locked {
			renderLoginError(d, w, r, next, "Account temporarily locked. Try again at "+until.Format("15:04")+".")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		u, err := d.Store.Authenticate(ctx, username, password)
		if err != nil {
			if errors.Is(err, auth.ErrLocked) {
				renderLoginError(d, w, r, next, "Account locked.")
				return
			}
			d.Lockout.Record(username)
			renderLoginError(d, w, r, next, "Invalid credentials.")
			return
		}
		d.Lockout.Reset(username)

		payload := auth.SessionPayload{
			Username: u.Username,
			Role:     string(u.Role),
			Exp:      time.Now().Add(d.ttl()).Unix(),
			Nonce:    auth.NewCSRFToken(),
		}
		cookie, err := auth.Sign(d.Secret, payload)
		if err != nil {
			http.Error(w, "session: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "comqtt_session",
			Value:    cookie,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
			MaxAge:   int(d.ttl().Seconds()),
		})
		http.Redirect(w, r, next, http.StatusFound)
	}
}

// LogoutPost clears the session cookie and redirects to the login page.
func LogoutPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:   "comqtt_session",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		http.Redirect(w, r, "/dashboard/login", http.StatusFound)
	}
}

func renderLoginError(d AccountDeps, w http.ResponseWriter, r *http.Request, next, msg string) {
	w.WriteHeader(http.StatusUnauthorized)
	d.Renderer.Render(w, "login", map[string]any{
		"CSRF":  auth.NewCSRFToken(),
		"Next":  next,
		"Error": msg,
	})
}

// sanitizeNext only accepts paths under /dashboard/ to prevent open-redirect.
// External targets, scheme-relative URLs, and missing leading slash all fall
// back to the default landing page.
func sanitizeNext(raw string) string {
	if raw == "" {
		return "/dashboard/"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host != "" || parsed.Scheme != "" {
		return "/dashboard/"
	}
	if !strings.HasPrefix(parsed.Path, "/dashboard/") {
		return "/dashboard/"
	}
	return parsed.Path
}
