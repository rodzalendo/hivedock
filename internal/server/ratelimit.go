package server

import (
	"sync"
	"time"
)

const (
	loginFailThreshold = 5                // failures before backoff kicks in
	loginBaseBackoff   = 30 * time.Second // first block after the threshold
	loginMaxBackoff    = 15 * time.Minute // cap
)

type limiterEntry struct {
	failures     int
	blockedUntil time.Time
}

// loginLimiter is an in-memory brute-force damper keyed by identity|ip. After
// loginFailThreshold failures it blocks with exponential backoff capped at
// loginMaxBackoff. State is process-local (a restart resets it) — a damper, not
// an access control.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: map[string]*limiterEntry{}}
}

// retryAfter returns how long key must wait before another attempt, or 0.
func (l *loginLimiter) retryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.entries[key]; e != nil {
		if d := time.Until(e.blockedUntil); d > 0 {
			return d
		}
	}
	return 0
}

// fail records a failed attempt and returns the resulting block duration (0
// until the threshold is crossed).
func (l *loginLimiter) fail(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked()
	e := l.entries[key]
	if e == nil {
		e = &limiterEntry{}
		l.entries[key] = e
	}
	e.failures++
	if e.failures < loginFailThreshold {
		return 0
	}
	shift := e.failures - loginFailThreshold
	if shift > 20 {
		shift = 20 // guard against Duration overflow on absurd counts
	}
	backoff := loginBaseBackoff << uint(shift)
	if backoff <= 0 || backoff > loginMaxBackoff {
		backoff = loginMaxBackoff
	}
	e.blockedUntil = time.Now().Add(backoff)
	return backoff
}

// reset clears a key's counter after a successful auth.
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

// pruneLocked bounds the map by dropping settled (unblocked, sub-threshold)
// entries once it grows large. Caller holds the lock.
func (l *loginLimiter) pruneLocked() {
	if len(l.entries) < 1024 {
		return
	}
	now := time.Now()
	for k, e := range l.entries {
		if e.failures < loginFailThreshold && e.blockedUntil.Before(now) {
			delete(l.entries, k)
		}
	}
}
