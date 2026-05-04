// mqtt/dashboard/auth/csrf_test.go
package auth

import "testing"

func TestCSRFRoundTrip(t *testing.T) {
	tok := NewCSRFToken()
	if !ValidateCSRFToken(tok, tok) {
		t.Fatal("token should match itself")
	}
}

func TestCSRFRejectsMismatch(t *testing.T) {
	if ValidateCSRFToken(NewCSRFToken(), NewCSRFToken()) {
		t.Fatal("two random tokens should not match")
	}
}

func TestCSRFRejectsEmpty(t *testing.T) {
	if ValidateCSRFToken("", "") {
		t.Fatal("empty tokens must not validate")
	}
}
