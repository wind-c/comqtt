// dashboard/auth/credstore_file.go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return &FileStore{path: path}, nil
}

func (s *FileStore) load() ([]User, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(b, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *FileStore) save(users []User) error {
	b, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func (s *FileStore) Seed(ctx context.Context, username string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.load()
	if err != nil {
		return "", err
	}
	if len(users) > 0 {
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
	users = append(users, User{
		Username:      username,
		Hash:          string(hash),
		Role:          RoleAdmin,
		MustChange:    true,
		PasswordSetAt: now,
		CreatedAt:     now,
	})
	if err := s.save(users); err != nil {
		return "", err
	}
	return pw, nil
}

func (s *FileStore) Authenticate(ctx context.Context, username, password string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.load()
	if err != nil {
		return User{}, err
	}
	for _, u := range users {
		if u.Username != username {
			continue
		}
		if u.LockedUntil > time.Now().Unix() {
			return User{}, ErrLocked
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.Hash), []byte(password)); err != nil {
			return User{}, ErrBadCredentials
		}
		return u, nil
	}
	return User{}, ErrBadCredentials
}

func (s *FileStore) SetPassword(ctx context.Context, username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.load()
	if err != nil {
		return err
	}
	for i, u := range users {
		if u.Username != username {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		users[i].Hash = string(hash)
		users[i].MustChange = false
		users[i].PasswordSetAt = time.Now().Unix()
		users[i].FailedAttempts = 0
		users[i].LockedUntil = 0
		return s.save(users)
	}
	return ErrUserNotFound
}

func (s *FileStore) CreateUser(ctx context.Context, username, password string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.load()
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.Username == username {
			return ErrUserExists
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	users = append(users, User{
		Username: username, Hash: string(hash), Role: role,
		MustChange: true, PasswordSetAt: now, CreatedAt: now,
	})
	return s.save(users)
}

func (s *FileStore) DeleteUser(ctx context.Context, username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.load()
	if err != nil {
		return err
	}
	admins := 0
	for _, u := range users {
		if u.Role == RoleAdmin {
			admins++
		}
	}
	out := users[:0]
	for _, u := range users {
		if u.Username == username {
			if u.Role == RoleAdmin && admins == 1 {
				return ErrCannotDeleteLastAdmin
			}
			continue
		}
		out = append(out, u)
	}
	if len(out) == len(users) {
		return ErrUserNotFound
	}
	return s.save(out)
}

func (s *FileStore) GetUser(ctx context.Context, username string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.load()
	if err != nil {
		return User{}, err
	}
	for _, u := range users {
		if u.Username == username {
			return u, nil
		}
	}
	return User{}, ErrUserNotFound
}

func (s *FileStore) ListUsers(ctx context.Context) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *FileStore) SetRole(ctx context.Context, username string, role Role) error {
	if !role.Valid() {
		return errors.New("auth: invalid role")
	}
	return s.mutate(username, func(u *User) { u.Role = role })
}

func (s *FileStore) SetLockedUntil(ctx context.Context, username string, until int64) error {
	return s.mutate(username, func(u *User) { u.LockedUntil = until })
}

func (s *FileStore) IncrementFailures(ctx context.Context, username string) (int, error) {
	var n int
	err := s.mutate(username, func(u *User) { u.FailedAttempts++; n = u.FailedAttempts })
	return n, err
}

func (s *FileStore) ResetFailures(ctx context.Context, username string) error {
	return s.mutate(username, func(u *User) { u.FailedAttempts = 0; u.LockedUntil = 0 })
}

func (s *FileStore) mutate(username string, fn func(*User)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.load()
	if err != nil {
		return err
	}
	for i, u := range users {
		if u.Username == username {
			fn(&users[i])
			return s.save(users)
		}
	}
	return ErrUserNotFound
}

func randomPassword(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf)[:n], nil
}
