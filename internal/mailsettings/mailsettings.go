// Package mailsettings holds per-account settings (signature, send-as aliases,
// vacation responder) and the vacation auto-reply logic. Ported from vulos-mail's
// mailsettings + vacation. The responder rate-limits replies per (account,
// sender) and refuses to reply to automated/daemon mail (loop protection).
package mailsettings

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Vacation is an out-of-office auto-reply configuration.
type Vacation struct {
	Enabled bool
	Subject string
	Body    string
}

// Settings is one account's preferences.
type Settings struct {
	Signature string
	Aliases   []string // additional addresses the account may send as
	Vacation  Vacation
}

// Store keeps per-account settings (in-memory; a persisted backend is a later
// swap behind this type).
type Store struct {
	mu sync.RWMutex
	m  map[string]Settings
}

// NewStore returns an empty settings store.
func NewStore() *Store { return &Store{m: map[string]Settings{}} }

// Get returns the account's settings (zero value if unset).
func (s *Store) Get(account string) Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[strings.ToLower(account)]
}

// Set stores the account's settings.
func (s *Store) Set(account string, st Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[strings.ToLower(account)] = st
}

// Responder decides whether to send a vacation reply, rate-limited per
// (account, sender) over a period.
type Responder struct {
	period time.Duration
	now    func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
}

// NewResponder builds a responder that replies at most once per period per
// sender.
func NewResponder(period time.Duration, now func() time.Time) *Responder {
	if period <= 0 {
		period = 7 * 24 * time.Hour
	}
	if now == nil {
		now = time.Now
	}
	return &Responder{period: period, now: now, last: map[string]time.Time{}}
}

// ShouldReply reports whether a vacation reply to sender is due, and records it.
func (r *Responder) ShouldReply(account, sender string) bool {
	key := strings.ToLower(account) + "|" + strings.ToLower(sender)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if last, ok := r.last[key]; ok && now.Sub(last) < r.period {
		return false
	}
	r.last[key] = now
	return true
}

// BuildReply constructs the vacation auto-reply message. Auto-Submitted marks it
// so well-behaved servers won't auto-reply back.
func BuildReply(from, to, subject, body string) []byte {
	if subject == "" {
		subject = "Out of office"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("Auto-Submitted: auto-replied\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String())
}

// IsAutomated reports whether a raw message looks automated (a bounce, list, or
// auto-reply), so we don't create a mail loop.
func IsAutomated(raw []byte) bool {
	// Inspect the header block only.
	head := raw
	if i := strings.Index(string(raw), "\r\n\r\n"); i >= 0 {
		head = raw[:i]
	}
	h := strings.ToLower(string(head))
	if strings.Contains(h, "\nauto-submitted:") && !strings.Contains(h, "auto-submitted: no") {
		return true
	}
	if strings.HasPrefix(h, "auto-submitted:") && !strings.HasPrefix(h, "auto-submitted: no") {
		return true
	}
	return strings.Contains(h, "\nlist-id:") || strings.Contains(h, "\nprecedence: bulk") || strings.Contains(h, "\nprecedence: list")
}

// IsDaemonAddress reports whether an address is a no-reply / system mailbox.
func IsDaemonAddress(addr string) bool {
	lp := strings.ToLower(addr)
	if i := strings.Index(lp, "@"); i >= 0 {
		lp = lp[:i]
	}
	switch lp {
	case "mailer-daemon", "postmaster", "noreply", "no-reply", "donotreply", "do-not-reply":
		return true
	}
	return false
}
