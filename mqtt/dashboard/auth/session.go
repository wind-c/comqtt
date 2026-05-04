package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// SessionPayload is what gets HMAC-signed into the comqtt_session cookie.
// Stateless: server does no lookup beyond MAC verification + expiry check.
type SessionPayload struct {
	Username string `json:"u"`
	Role     string `json:"r"`
	Exp      int64  `json:"e"` // unix seconds
	Nonce    string `json:"n"` // 8 random bytes b64
}

var (
	ErrBadMAC    = errors.New("session: bad mac")
	ErrExpired   = errors.New("session: expired")
	ErrMalformed = errors.New("session: malformed")
)

func Sign(secret []byte, p SessionPayload) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	b := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(b))
	return b + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func Verify(secret []byte, cookie string) (SessionPayload, error) {
	parts := strings.SplitN(cookie, ".", 2)
	if len(parts) != 2 {
		return SessionPayload{}, ErrMalformed
	}
	expected := hmac.New(sha256.New, secret)
	expected.Write([]byte(parts[0]))
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return SessionPayload{}, ErrMalformed
	}
	if !hmac.Equal(expected.Sum(nil), got) {
		return SessionPayload{}, ErrBadMAC
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return SessionPayload{}, ErrMalformed
	}
	var p SessionPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return SessionPayload{}, ErrMalformed
	}
	if time.Now().Unix() > p.Exp {
		return SessionPayload{}, ErrExpired
	}
	return p, nil
}
