// mqtt/dashboard/auth/lockout.go
package auth

import (
	"sync"
	"time"
)

type LockoutConfig struct {
	Threshold int
	Window    time.Duration
	Duration  time.Duration
}

type Lockout struct {
	cfg LockoutConfig
	mu  sync.Mutex
	st  map[string]*lockoutState
}

type lockoutState struct {
	failures    []time.Time
	lockedUntil time.Time
}

func NewLockout(cfg LockoutConfig) *Lockout {
	return &Lockout{cfg: cfg, st: map[string]*lockoutState{}}
}

func (l *Lockout) Record(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.st[username]
	if !ok {
		s = &lockoutState{}
		l.st[username] = s
	}
	now := time.Now()
	cutoff := now.Add(-l.cfg.Window)
	out := s.failures[:0]
	for _, t := range s.failures {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	s.failures = append(out, now)
	if len(s.failures) >= l.cfg.Threshold {
		s.lockedUntil = now.Add(l.cfg.Duration)
		s.failures = nil
	}
}

func (l *Lockout) IsLocked(username string) (bool, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.st[username]
	if !ok {
		return false, time.Time{}
	}
	if time.Now().Before(s.lockedUntil) {
		return true, s.lockedUntil
	}
	return false, time.Time{}
}

func (l *Lockout) Reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.st, username)
}
