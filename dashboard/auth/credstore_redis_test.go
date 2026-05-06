// dashboard/auth/credstore_redis_test.go
package auth

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRedisStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisStore(client, "test:dashboard"), mr
}

func TestRedisStoreSeedAndAuthenticate(t *testing.T) {
	store, _ := newRedisStore(t)
	ctx := context.Background()
	pw, err := store.Seed(ctx, "admin")
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if pw == "" {
		t.Fatal("seeded password should be non-empty")
	}
	u, err := store.Authenticate(ctx, "admin", pw)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.Role != RoleAdmin {
		t.Fatalf("role: %s", u.Role)
	}
	if !u.MustChange {
		t.Fatal("MustChange should be true on seed")
	}
}

func TestRedisStoreSeedIdempotent(t *testing.T) {
	store, _ := newRedisStore(t)
	ctx := context.Background()
	pw1, _ := store.Seed(ctx, "admin")
	pw2, err := store.Seed(ctx, "admin")
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if pw2 != "" {
		t.Fatalf("second Seed should return empty password, got %q", pw2)
	}
	if pw1 == "" {
		t.Fatal("first Seed should return password")
	}
}

func TestRedisStoreAuthenticateBadPassword(t *testing.T) {
	store, _ := newRedisStore(t)
	_, _ = store.Seed(context.Background(), "admin")
	if _, err := store.Authenticate(context.Background(), "admin", "wrong"); err != ErrBadCredentials {
		t.Fatalf("err: %v", err)
	}
}

func TestRedisStoreSetPassword(t *testing.T) {
	store, _ := newRedisStore(t)
	ctx := context.Background()
	_, _ = store.Seed(ctx, "admin")
	if err := store.SetPassword(ctx, "admin", "newpass1234"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	u, err := store.Authenticate(ctx, "admin", "newpass1234")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.MustChange {
		t.Fatal("must_change should be cleared")
	}
}

func TestRedisStoreCreateAndDeleteUser(t *testing.T) {
	store, _ := newRedisStore(t)
	ctx := context.Background()
	_, _ = store.Seed(ctx, "admin")
	if err := store.CreateUser(ctx, "bob", "bobpass1234", RoleViewer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	users, _ := store.ListUsers(ctx)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if err := store.DeleteUser(ctx, "bob"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if err := store.DeleteUser(ctx, "admin"); err != ErrCannotDeleteLastAdmin {
		t.Fatalf("expected ErrCannotDeleteLastAdmin: %v", err)
	}
}

func TestRedisStoreSetRole(t *testing.T) {
	store, _ := newRedisStore(t)
	ctx := context.Background()
	_, _ = store.Seed(ctx, "admin")
	_ = store.CreateUser(ctx, "bob", "bobpass1234", RoleViewer)
	if err := store.SetRole(ctx, "bob", RoleAdmin); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	u, _ := store.GetUser(ctx, "bob")
	if u.Role != RoleAdmin {
		t.Fatalf("role: %s", u.Role)
	}
}

func TestRedisStoreLockoutFields(t *testing.T) {
	store, _ := newRedisStore(t)
	ctx := context.Background()
	_, _ = store.Seed(ctx, "admin")
	if err := store.SetLockedUntil(ctx, "admin", 99999999999); err != nil {
		t.Fatalf("SetLockedUntil: %v", err)
	}
	u, _ := store.GetUser(ctx, "admin")
	if u.LockedUntil != 99999999999 {
		t.Fatalf("locked_until: %d", u.LockedUntil)
	}
	if _, err := store.IncrementFailures(ctx, "admin"); err != nil {
		t.Fatalf("IncrementFailures: %v", err)
	}
	u, _ = store.GetUser(ctx, "admin")
	if u.FailedAttempts != 1 {
		t.Fatalf("failed_attempts: %d", u.FailedAttempts)
	}
	if err := store.ResetFailures(ctx, "admin"); err != nil {
		t.Fatalf("ResetFailures: %v", err)
	}
	u, _ = store.GetUser(ctx, "admin")
	if u.FailedAttempts != 0 || u.LockedUntil != 0 {
		t.Fatalf("after reset: %+v", u)
	}
}
