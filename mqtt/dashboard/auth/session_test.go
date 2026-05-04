package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyRoundTrip(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	want := SessionPayload{Username: "alice", Role: "admin", Exp: time.Now().Add(time.Hour).Unix(), Nonce: "n1"}
	cookie, err := Sign(secret, want)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := Verify(secret, cookie)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != want {
		t.Fatalf("payload mismatch: got %+v want %+v", got, want)
	}
}

func TestVerifyRejectsTampered(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	cookie, _ := Sign(secret, SessionPayload{Username: "alice", Role: "admin", Exp: time.Now().Add(time.Hour).Unix(), Nonce: "n1"})
	parts := strings.SplitN(cookie, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("malformed cookie: %q", cookie)
	}
	// Flip a byte in the encoded payload so the HMAC no longer matches.
	flipped := []byte(parts[0])
	flipped[3] ^= 0x01
	tampered := string(flipped) + "." + parts[1]
	if _, err := Verify(secret, tampered); err == nil {
		t.Fatal("expected error on tampered cookie")
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	cookie, _ := Sign(secret, SessionPayload{Username: "alice", Role: "admin", Exp: time.Now().Add(-time.Hour).Unix(), Nonce: "n1"})
	if _, err := Verify(secret, cookie); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestVerifyRejectsBadSecret(t *testing.T) {
	cookie, _ := Sign([]byte("0123456789abcdef0123456789abcdef"), SessionPayload{Username: "alice", Role: "admin", Exp: time.Now().Add(time.Hour).Unix()})
	if _, err := Verify([]byte("ffffffffffffffffffffffffffffffff"), cookie); err == nil {
		t.Fatal("expected bad-mac error")
	}
}
