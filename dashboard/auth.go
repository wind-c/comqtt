// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package dashboard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	errInvalidCredentials = errors.New("invalid credentials")
	errTokenExpired       = errors.New("token expired")
	errInvalidToken       = errors.New("invalid token")
	errLayoutNotFound     = errors.New("layout template not found")
)

type User struct {
	Username       string `json:"username"`
	Hash           string `json:"hash"`
	Role           string `json:"role"`
	MustChange     bool   `json:"must_change"`
	PasswordSetAt  int64  `json:"password_set_at"`
	FailedAttempts int    `json:"failed_attempts"`
	LockedUntil    int64  `json:"locked_until"`
	CreatedAt      int64  `json:"created_at"`
}

type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
}

func loadUsers(file string) ([]User, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func signJWT(secret []byte, username, role string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		Username: username,
		Role:     role,
		Iat:      now.Unix(),
		Exp:      now.Add(ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	toSign := header + "." + body

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(toSign))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return toSign + "." + signature, nil
}

func verifyJWT(secret []byte, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errInvalidToken
	}

	toSign := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(toSign))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, errTokenExpired
	}

	return &claims, nil
}

func (d *Dashboard) HandleLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var user *User
	for i := range d.users {
		if d.users[i].Username == creds.Username {
			user = &d.users[i]
			break
		}
	}
	if user == nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Hash), []byte(creds.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if user.Role != "admin" {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	token, err := signJWT([]byte(d.opts.AuthSecret), user.Username, user.Role, 24*time.Hour)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}

func (d *Dashboard) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/dashboard/login", http.StatusFound)
}

func (d *Dashboard) AuthMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil {
			http.Redirect(w, r, "/dashboard/login", http.StatusFound)
			return
		}

		claims, err := verifyJWT([]byte(d.opts.AuthSecret), cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/dashboard/login", http.StatusFound)
			return
		}

		r.Header.Set("X-Dashboard-User", fmt.Sprintf("%s:%s", claims.Username, claims.Role))
		next.ServeHTTP(w, r)
	}
}
