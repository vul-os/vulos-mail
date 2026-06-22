// Package authlimit is the inbound credential-check brute-force limiter — the
// receive-side counterpart to internal/abuse (which guards outbound sends). It
// throttles repeated failed authentication attempts keyed independently by
// client IP and by account, so an attacker can neither spray one password
// across many accounts from one host nor grind one account from many hosts.
//
// Model: a per-key sliding window of recent failures. Once a key accumulates
// MaxFailures within the window it is "locked" until Lockout elapses from the
// last failure; further attempts are refused without ever reaching the real
// credential check. A successful authentication clears the key. Fail-open on an
// empty key so legitimate first-time logins are never penalised.
package authlimit

import (
	"sync"
	"time"
)

// Limiter throttles failed credential checks per key. Safe for concurrent use.
type Limiter struct {
	max     int
	window  time.Duration
	lockout time.Duration
	now     func() time.Time

	mu    sync.Mutex
	fails map[string][]time.Time
}

// Config configures the limiter. Zero fields take sensible defaults.
type Config struct {
	MaxFailures int           // failures within Window before a key is locked
	Window      time.Duration // sliding window over which failures accumulate
	Lockout     time.Duration // how long a key stays locked after its last failure
	Now         func() time.Time
}

// New builds a Limiter, defaulting unset fields.
func New(cfg Config) *Limiter {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.Window <= 0 {
		cfg.Window = 15 * time.Minute
	}
	if cfg.Lockout <= 0 {
		cfg.Lockout = 15 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Limiter{
		max:     cfg.MaxFailures,
		window:  cfg.Window,
		lockout: cfg.Lockout,
		now:     cfg.Now,
		fails:   map[string][]time.Time{},
	}
}

// Locked reports whether key is currently locked out (too many recent
// failures). It does not mutate state, so it is safe to call before the real
// credential check.
func (l *Limiter) Locked(key string) bool {
	if key == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lockedLocked(key)
}

// AnyLocked reports whether any of the given keys is locked. Used so a request
// is refused if either its IP or its account is over the limit.
func (l *Limiter) AnyLocked(keys ...string) bool {
	for _, k := range keys {
		if l.Locked(k) {
			return true
		}
	}
	return false
}

func (l *Limiter) lockedLocked(key string) bool {
	times := l.fails[key]
	if len(times) < l.max {
		return false
	}
	// Locked until Lockout elapses from the most recent failure.
	last := times[len(times)-1]
	if l.now().Sub(last) >= l.lockout {
		delete(l.fails, key)
		return false
	}
	return true
}

// Fail records a failed authentication for each key (typically the client IP
// and the attempted account).
func (l *Limiter) Fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-l.window)
	for _, k := range keys {
		if k == "" {
			continue
		}
		kept := l.fails[k][:0]
		for _, t := range l.fails[k] {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		l.fails[k] = append(kept, now)
	}
}

// Success clears the failure history for each key after a correct credential
// check, so a legitimate login is never throttled by its own earlier typos.
func (l *Limiter) Success(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.fails, k)
	}
}
