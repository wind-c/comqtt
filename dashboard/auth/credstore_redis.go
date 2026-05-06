// dashboard/auth/credstore_redis.go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// RedisStore implements CredStore using a single Redis hash. All cluster
// nodes read/write the same hash so password/role changes propagate.
//
// Layout:
//   key   = "<KeyPrefix>:users"  (default "comqtt:dashboard:users")
//   field = username
//   value = JSON-marshaled User struct
type RedisStore struct {
	Client    *redis.Client
	KeyPrefix string
}

// NewRedisStore constructs a RedisStore with sensible defaults. KeyPrefix
// defaults to "comqtt:dashboard" if empty. The caller owns the underlying
// redis.Client lifecycle.
func NewRedisStore(client *redis.Client, keyPrefix string) *RedisStore {
	if keyPrefix == "" {
		keyPrefix = "comqtt:dashboard"
	}
	return &RedisStore{Client: client, KeyPrefix: keyPrefix}
}

func (s *RedisStore) usersKey() string { return s.KeyPrefix + ":users" }

// Seed creates the admin user once. Idempotent: returns "" with no error
// if a user already exists in the hash.
func (s *RedisStore) Seed(ctx context.Context, username string) (string, error) {
	exists, err := s.Client.HLen(ctx, s.usersKey()).Result()
	if err != nil {
		return "", err
	}
	if exists > 0 {
		return "", nil
	}
	pw, err := randomPassword(16)
	if err != nil {
		return "", err
	}
	if env := os.Getenv("DASHBOARD_INITIAL_PASSWORD"); env != "" {
		pw = env
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	u := User{
		Username:      username,
		Hash:          string(hash),
		Role:          RoleAdmin,
		MustChange:    true,
		PasswordSetAt: now,
		CreatedAt:     now,
	}
	body, _ := json.Marshal(u)
	if err := s.Client.HSet(ctx, s.usersKey(), username, body).Err(); err != nil {
		return "", err
	}
	return pw, nil
}

func (s *RedisStore) Authenticate(ctx context.Context, username, password string) (User, error) {
	u, err := s.GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return User{}, ErrBadCredentials
		}
		return User{}, err
	}
	if u.LockedUntil > time.Now().Unix() {
		return User{}, ErrLocked
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Hash), []byte(password)); err != nil {
		return User{}, ErrBadCredentials
	}
	return u, nil
}

func (s *RedisStore) SetPassword(ctx context.Context, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.mutate(ctx, username, func(u *User) {
		u.Hash = string(hash)
		u.MustChange = false
		u.PasswordSetAt = time.Now().Unix()
		u.FailedAttempts = 0
		u.LockedUntil = 0
	})
}

func (s *RedisStore) CreateUser(ctx context.Context, username, password string, role Role) error {
	exists, err := s.Client.HExists(ctx, s.usersKey(), username).Result()
	if err != nil {
		return err
	}
	if exists {
		return ErrUserExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	u := User{
		Username:      username,
		Hash:          string(hash),
		Role:          role,
		MustChange:    true,
		PasswordSetAt: now,
		CreatedAt:     now,
	}
	body, _ := json.Marshal(u)
	return s.Client.HSet(ctx, s.usersKey(), username, body).Err()
}

func (s *RedisStore) DeleteUser(ctx context.Context, username string) error {
	users, err := s.ListUsers(ctx)
	if err != nil {
		return err
	}
	admins := 0
	var target *User
	for i, u := range users {
		if u.Role == RoleAdmin {
			admins++
		}
		if u.Username == username {
			target = &users[i]
		}
	}
	if target == nil {
		return ErrUserNotFound
	}
	if target.Role == RoleAdmin && admins == 1 {
		return ErrCannotDeleteLastAdmin
	}
	return s.Client.HDel(ctx, s.usersKey(), username).Err()
}

func (s *RedisStore) GetUser(ctx context.Context, username string) (User, error) {
	body, err := s.Client.HGet(ctx, s.usersKey(), username).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *RedisStore) ListUsers(ctx context.Context) ([]User, error) {
	all, err := s.Client.HGetAll(ctx, s.usersKey()).Result()
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(all))
	for _, raw := range all {
		var u User
		if err := json.Unmarshal([]byte(raw), &u); err != nil {
			continue
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *RedisStore) SetLockedUntil(ctx context.Context, username string, until int64) error {
	return s.mutate(ctx, username, func(u *User) { u.LockedUntil = until })
}

func (s *RedisStore) IncrementFailures(ctx context.Context, username string) (int, error) {
	var n int
	err := s.mutate(ctx, username, func(u *User) { u.FailedAttempts++; n = u.FailedAttempts })
	return n, err
}

func (s *RedisStore) ResetFailures(ctx context.Context, username string) error {
	return s.mutate(ctx, username, func(u *User) { u.FailedAttempts = 0; u.LockedUntil = 0 })
}

func (s *RedisStore) SetRole(ctx context.Context, username string, role Role) error {
	if !role.Valid() {
		return errors.New("auth: invalid role")
	}
	return s.mutate(ctx, username, func(u *User) { u.Role = role })
}

// mutate atomically updates one user via WATCH/MULTI/EXEC.
func (s *RedisStore) mutate(ctx context.Context, username string, fn func(*User)) error {
	key := s.usersKey()
	return s.Client.Watch(ctx, func(tx *redis.Tx) error {
		body, err := tx.HGet(ctx, key, username).Bytes()
		if errors.Is(err, redis.Nil) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		var u User
		if err := json.Unmarshal(body, &u); err != nil {
			return err
		}
		fn(&u)
		updated, _ := json.Marshal(&u)
		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			p.HSet(ctx, key, username, updated)
			return nil
		})
		return err
	}, key)
}
