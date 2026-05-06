// dashboard/auth/middleware.go
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type ctxKey string

const userCtxKey ctxKey = "comqtt-user"

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userCtxKey, u)
}

func UserFromContext(ctx context.Context) User {
	if u, ok := ctx.Value(userCtxKey).(User); ok {
		return u
	}
	return User{}
}

// RequireAuth verifies the cookie, loads the User, then enforces force-rotate
// and password-expiry policies. It exempts the login/logout/static paths.
func RequireAuth(secret []byte, store CredStore, expiryDays int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			c, err := r.Cookie("comqtt_session")
			if err != nil {
				redirectLogin(w, r)
				return
			}
			payload, err := Verify(secret, c.Value)
			if err != nil {
				redirectLogin(w, r)
				return
			}
			u, err := store.GetUser(r.Context(), payload.Username)
			if err != nil {
				redirectLogin(w, r)
				return
			}
			if u.MustChange && !strings.HasPrefix(r.URL.Path, "/dashboard/account/password") {
				http.Redirect(w, r, "/dashboard/account/password?reason=must_change", http.StatusFound)
				return
			}
			if expiryDays > 0 && time.Now().Unix()-u.PasswordSetAt > int64(expiryDays)*86400 {
				if !strings.HasPrefix(r.URL.Path, "/dashboard/account/password") {
					http.Redirect(w, r, "/dashboard/account/password?reason=expired", http.StatusFound)
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		})
	}
}

// RequireRole rejects users whose role is below the required level.
// It assumes RequireAuth has already populated the context.
func RequireRole(min Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u.Role == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if min == RoleAdmin && u.Role != RoleAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isExempt(path string) bool {
	switch {
	case strings.HasPrefix(path, "/dashboard/login"):
		return true
	case strings.HasPrefix(path, "/dashboard/logout"):
		return true
	case strings.HasPrefix(path, "/dashboard/static/"):
		return true
	}
	return false
}

func redirectLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard/login?next="+r.URL.Path, http.StatusFound)
}
