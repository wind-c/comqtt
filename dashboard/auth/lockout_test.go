// dashboard/auth/lockout_test.go
package auth

import (
	"testing"
	"time"
)

func TestLockoutBelowThreshold(t *testing.T) {
	l := NewLockout(LockoutConfig{Threshold: 5, Window: time.Minute, Duration: 10 * time.Minute})
	for i := 0; i < 4; i++ {
		l.Record("alice")
	}
	if locked, _ := l.IsLocked("alice"); locked {
		t.Fatal("should not be locked yet")
	}
}

func TestLockoutAtThreshold(t *testing.T) {
	l := NewLockout(LockoutConfig{Threshold: 5, Window: time.Minute, Duration: 10 * time.Minute})
	for i := 0; i < 5; i++ {
		l.Record("alice")
	}
	locked, until := l.IsLocked("alice")
	if !locked {
		t.Fatal("should be locked")
	}
	if until.Before(time.Now()) {
		t.Fatalf("locked_until in past: %v", until)
	}
}

func TestLockoutResetsClearsState(t *testing.T) {
	l := NewLockout(LockoutConfig{Threshold: 5, Window: time.Minute, Duration: 10 * time.Minute})
	for i := 0; i < 5; i++ {
		l.Record("alice")
	}
	l.Reset("alice")
	if locked, _ := l.IsLocked("alice"); locked {
		t.Fatal("should be cleared after Reset")
	}
}
