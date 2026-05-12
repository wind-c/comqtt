// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
)

const (
	AuthUsersPath  = "/api/v1/auth/users"
	AuthUserPath   = "/api/v1/auth/users/{username}"
	AuthUserAclPath = "/api/v1/auth/users/{username}/acl"
	AuthUserAclFilterPath = "/api/v1/auth/users/{username}/acl/{filter...}"
)

type AuthEntry struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Allow    bool   `json:"allow"`
}

type AclEntry struct {
	Topic  string `json:"topic"`
	Access int    `json:"access"`
}

type AclUpdate struct {
	Topic  string `json:"topic"`
	Access int    `json:"access"`
}

type AuthManager struct {
	rdb      redis.UniversalClient
	authKey  string
	aclKeyPrefix string
}

func NewAuthManager(rdb redis.UniversalClient, authKey, aclPrefix string) *AuthManager {
	if authKey == "" { authKey = "comqtt:auth" }
	if aclPrefix == "" { aclPrefix = "comqtt:acl" }
	return &AuthManager{rdb: rdb, authKey: authKey, aclKeyPrefix: aclPrefix}
}

func (m *AuthManager) GenHandlers() map[string]Handler {
	return map[string]Handler{
		"GET " + AuthUsersPath:     m.listUsers,
		"POST " + AuthUsersPath:    m.createUser,
		"PUT " + AuthUserPath:      m.updateUser,
		"DELETE " + AuthUserPath:   m.deleteUser,
		"GET " + AuthUserAclPath:   m.listAcl,
		"POST " + AuthUserAclPath:  m.addAcl,
		"PUT " + AuthUserAclPath:   m.updateAcl,
		"DELETE " + AuthUserAclFilterPath: m.deleteAcl,
	}
}

func (m *AuthManager) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	users, err := m.rdb.HGetAll(ctx, m.authKey).Result()
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]AuthEntry, 0, len(users))
	for username, val := range users {
		var rule auth.AuthRule
		if json.Unmarshal([]byte(val), &rule) != nil { continue }
		result = append(result, AuthEntry{Username: username, Allow: rule.Allow})
	}
	Ok(w, result)
}

func (m *AuthManager) createUser(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Allow    bool   `json:"allow"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username == "" {
		Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rule := auth.AuthRule{Username: auth.RString(req.Username), Password: auth.RString(req.Password), Allow: req.Allow}
	data, _ := json.Marshal(rule)
	if err := m.rdb.HSet(ctx, m.authKey, req.Username, string(data)).Err(); err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	Ok(w, map[string]string{"username": req.Username})
}

func (m *AuthManager) updateUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	defer r.Body.Close()
	var req struct {
		Password string `json:"password"`
		Allow    *bool  `json:"allow"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	val, err := m.rdb.HGet(ctx, m.authKey, username).Result()
	if err != nil {
		Error(w, http.StatusNotFound, "user not found")
		return
	}
	var rule auth.AuthRule
	if json.Unmarshal([]byte(val), &rule) != nil {
		Error(w, http.StatusInternalServerError, "invalid user data")
		return
	}
	if req.Password != "" { rule.Password = auth.RString(req.Password) }
	if req.Allow != nil { rule.Allow = *req.Allow }
	data, _ := json.Marshal(rule)
	if err := m.rdb.HSet(ctx, m.authKey, username, string(data)).Err(); err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	Ok(w, map[string]string{"username": username})
}

func (m *AuthManager) deleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m.rdb.HDel(ctx, m.authKey, username)
	m.rdb.Del(ctx, m.aclKeyPrefix+":"+username)
	Ok(w, map[string]string{"username": username})
}

func (m *AuthManager) listAcl(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := m.rdb.HGetAll(ctx, m.aclKeyPrefix+":"+username).Result()
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]AclEntry, 0, len(data))
	for topic, v := range data {
		access, _ := strconv.Atoi(v)
		result = append(result, AclEntry{Topic: topic, Access: access})
	}
	Ok(w, result)
}

func (m *AuthManager) addAcl(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	defer r.Body.Close()
	var req AclUpdate
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Topic == "" {
		Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.rdb.HSet(ctx, m.aclKeyPrefix+":"+username, req.Topic, req.Access).Err(); err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	Ok(w, req)
}

func (m *AuthManager) updateAcl(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	defer r.Body.Close()
	var req AclUpdate
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Topic == "" {
		Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exists, _ := m.rdb.HExists(ctx, m.aclKeyPrefix+":"+username, req.Topic).Result()
	if !exists {
		Error(w, http.StatusNotFound, "acl entry not found")
		return
	}
	m.rdb.HSet(ctx, m.aclKeyPrefix+":"+username, req.Topic, req.Access)
	Ok(w, req)
}

func (m *AuthManager) deleteAcl(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	filter := r.PathValue("filter")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m.rdb.HDel(ctx, m.aclKeyPrefix+":"+username, filter)
	Ok(w, map[string]string{"topic": filter})
}
