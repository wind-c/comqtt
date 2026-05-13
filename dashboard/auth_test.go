// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestLoadUsers(t *testing.T) {
	dir := t.TempDir()
	users := []User{
		{Username: "admin", Hash: "$2a$10$f2kNwu6Gh3ivEPYDAjvbtOSI8EDcSeIB6OhA5S446ND5KEMjfsHhi", Role: "admin"},
	}
	data, _ := json.Marshal(users)
	file := filepath.Join(dir, "users.json")
	os.WriteFile(file, data, 0644)

	loaded, err := loadUsers(file)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Equal(t, "admin", loaded[0].Username)
	require.Equal(t, "admin", loaded[0].Role)
}

func TestLoadUsersFileNotExist(t *testing.T) {
	_, err := loadUsers("/nonexistent/path/users.json")
	require.Error(t, err)
}

func TestJwtSignAndVerify(t *testing.T) {
	secret := []byte("test-secret-key-32bytes-long!")
	token, err := signJWT(secret, "admin", "admin", time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := verifyJWT(secret, token)
	require.NoError(t, err)
	require.Equal(t, "admin", claims.Username)
	require.Equal(t, "admin", claims.Role)
}

func TestJwtExpired(t *testing.T) {
	secret := []byte("test-secret-key-32bytes-long!")
	token, err := signJWT(secret, "admin", "admin", -time.Hour)
	require.NoError(t, err)

	_, err = verifyJWT(secret, token)
	require.Error(t, err)
}

func TestJwtInvalidSignature(t *testing.T) {
	secret := []byte("test-secret-key-32bytes-long!")
	token, err := signJWT(secret, "admin", "admin", time.Hour)
	require.NoError(t, err)

	_, err = verifyJWT([]byte("wrong-secret-key-32bytes-long!!"), token)
	require.Error(t, err)
}

func TestHandleLoginSuccess(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), bcrypt.DefaultCost)
	h := &Dashboard{
		opts: Options{
			AuthSecret: "test-secret-key-32bytes-long!",
		},
		users: []User{
			{Username: "admin", Hash: string(hash), Role: "admin"},
		},
	}

	body := `{"username":"admin","password":"mypassword"}`
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, "token", cookies[0].Name)
	require.True(t, cookies[0].HttpOnly)
}

func TestHandleLoginInvalidPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), bcrypt.DefaultCost)
	h := &Dashboard{
		opts: Options{
			AuthSecret: "test-secret-key-32bytes-long!",
		},
		users: []User{
			{Username: "admin", Hash: string(hash), Role: "admin"},
		},
	}

	body := `{"username":"admin","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleLoginUserNotFound(t *testing.T) {
	h := &Dashboard{
		opts: Options{
			AuthSecret: "test-secret-key-32bytes-long!",
		},
		users: []User{},
	}

	body := `{"username":"nobody","password":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	secret := "test-secret-key-32bytes-long!"
	h := &Dashboard{
		opts: Options{AuthSecret: secret},
	}

	token, err := signJWT([]byte(secret), "admin", "admin", time.Hour)
	require.NoError(t, err)

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	w := httptest.NewRecorder()

	h.AuthMiddleware(next).ServeHTTP(w, req)
	require.True(t, called)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddlewareNoCookie(t *testing.T) {
	h := &Dashboard{
		opts: Options{AuthSecret: "test-secret-key-32bytes-long!"},
	}

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil)
	w := httptest.NewRecorder()

	h.AuthMiddleware(next).ServeHTTP(w, req)
	require.False(t, called)
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "/dashboard/login")
}

func TestHandleLogout(t *testing.T) {
	h := &Dashboard{
		opts: Options{AuthSecret: "test-secret-key-32bytes-long!"},
	}

	req := httptest.NewRequest(http.MethodPost, "/dashboard/logout", nil)
	w := httptest.NewRecorder()

	h.HandleLogout(w, req)
	require.Equal(t, http.StatusFound, w.Code)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, "token", cookies[0].Name)
	require.LessOrEqual(t, cookies[0].MaxAge, 0)
}
