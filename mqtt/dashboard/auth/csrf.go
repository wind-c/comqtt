// mqtt/dashboard/auth/csrf.go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

func NewCSRFToken() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

// ValidateCSRFToken constant-time compares the request token against the
// per-render token stamped into the form. Empty tokens never validate.
func ValidateCSRFToken(form, expected string) bool {
	if form == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(form), []byte(expected)) == 1
}
