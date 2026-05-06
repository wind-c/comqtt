// dashboard/auth/credstore.go
package auth

import (
	"context"
	"errors"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

func (r Role) Valid() bool {
	return r == RoleAdmin || r == RoleViewer
}

type User struct {
	Username       string `json:"username"`
	Hash           string `json:"hash"`
	Role           Role   `json:"role"`
	MustChange     bool   `json:"must_change"`
	PasswordSetAt  int64  `json:"password_set_at"`
	FailedAttempts int    `json:"failed_attempts"`
	LockedUntil    int64  `json:"locked_until"`
	CreatedAt      int64  `json:"created_at"`
}

var (
	ErrBadCredentials        = errors.New("auth: bad credentials")
	ErrUserNotFound          = errors.New("auth: user not found")
	ErrUserExists            = errors.New("auth: user exists")
	ErrCannotDeleteLastAdmin = errors.New("auth: cannot delete the last admin")
	ErrLocked                = errors.New("auth: account locked")
)

type CredStore interface {
	Seed(ctx context.Context, username string) (plaintext string, err error)
	Authenticate(ctx context.Context, username, password string) (User, error)
	SetPassword(ctx context.Context, username, password string) error
	CreateUser(ctx context.Context, username, password string, role Role) error
	DeleteUser(ctx context.Context, username string) error
	SetRole(ctx context.Context, username string, role Role) error
	GetUser(ctx context.Context, username string) (User, error)
	ListUsers(ctx context.Context) ([]User, error)
	SetLockedUntil(ctx context.Context, username string, until int64) error
	IncrementFailures(ctx context.Context, username string) (int, error)
	ResetFailures(ctx context.Context, username string) error
}
