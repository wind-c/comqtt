// dashboard/auth/credstore_file_test.go
package auth

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFileStoreSeedAndAuthenticate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "users.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
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
		t.Fatalf("role: got %s want %s", u.Role, RoleAdmin)
	}
	if !u.MustChange {
		t.Fatal("seeded user should have must_change=true")
	}
}

func TestFileStoreAuthenticateRejectsBadPassword(t *testing.T) {
	store, _ := NewFileStore(filepath.Join(t.TempDir(), "users.json"))
	_, _ = store.Seed(context.Background(), "admin")
	if _, err := store.Authenticate(context.Background(), "admin", "wrong"); err != ErrBadCredentials {
		t.Fatalf("err: %v", err)
	}
}

func TestFileStoreSetPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	store, _ := NewFileStore(path)
	_, _ = store.Seed(context.Background(), "admin")
	if err := store.SetPassword(context.Background(), "admin", "newpass1234"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	u, err := store.Authenticate(context.Background(), "admin", "newpass1234")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.MustChange {
		t.Fatal("must_change should be cleared after rotation")
	}
	store2, _ := NewFileStore(path)
	if _, err := store2.Authenticate(context.Background(), "admin", "newpass1234"); err != nil {
		t.Fatalf("re-open Authenticate: %v", err)
	}
}

func TestFileStoreCreateAndDeleteUser(t *testing.T) {
	store, _ := NewFileStore(filepath.Join(t.TempDir(), "users.json"))
	_, _ = store.Seed(context.Background(), "admin")
	if err := store.CreateUser(context.Background(), "bob", "bobpass1234", RoleViewer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	users, err := store.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users got %d", len(users))
	}
	if err := store.DeleteUser(context.Background(), "bob"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := store.Authenticate(context.Background(), "bob", "bobpass1234"); err == nil {
		t.Fatal("expected user to be gone")
	}
	if err := store.DeleteUser(context.Background(), "admin"); err != ErrCannotDeleteLastAdmin {
		t.Fatalf("expected ErrCannotDeleteLastAdmin, got %v", err)
	}
}

func TestFileStoreLockoutFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	store, _ := NewFileStore(path)
	_, _ = store.Seed(context.Background(), "admin")
	if err := store.SetLockedUntil(context.Background(), "admin", 99999999999); err != nil {
		t.Fatalf("SetLockedUntil: %v", err)
	}
	store2, _ := NewFileStore(path)
	u, _ := store2.GetUser(context.Background(), "admin")
	if u.LockedUntil != 99999999999 {
		t.Fatalf("locked_until persisted as %d", u.LockedUntil)
	}
}
