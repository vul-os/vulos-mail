package mailpg

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
)

// PGSessionStore is a Postgres-backed web-session store.  It backs
// cmd/vulos-mail.sessionStore so webmail sessions survive server restarts in
// multi-replica cloud deployments.
//
// Security note: the IMAP password is stored as plaintext in mail.sessions so
// the /v1 reverse proxy can broker it to the lilmail engine on every request.
// Ensure the Postgres connection uses TLS and restrict access to the mail schema.
type PGSessionStore struct {
	db  *sql.DB
	ttl time.Duration
}

// NewPGSessionStore returns a store with the given session TTL.
func NewPGSessionStore(db *sql.DB, ttl time.Duration) *PGSessionStore {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &PGSessionStore{db: db, ttl: ttl}
}

// Create stores a new session for (user, pass) and returns the opaque token.
func (s *PGSessionStore) Create(user, pass string) string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	tok := hex.EncodeToString(b[:])
	exp := time.Now().Add(s.ttl)
	_, _ = s.db.ExecContext(context.Background(),
		`INSERT INTO mail.sessions (token, account, pass, expires_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (token) DO UPDATE SET account = EXCLUDED.account, pass = EXCLUDED.pass, expires_at = EXCLUDED.expires_at`,
		tok, strings.ToLower(user), pass, exp,
	)
	// Opportunistically prune expired sessions.
	go s.prune()
	return tok
}

// Get returns the (user, pass) for a token and whether it is valid and unexpired.
func (s *PGSessionStore) Get(token string) (user, pass string, ok bool) {
	row := s.db.QueryRowContext(context.Background(),
		`SELECT account, pass, expires_at FROM mail.sessions WHERE token = $1`,
		token,
	)
	var exp time.Time
	if err := row.Scan(&user, &pass, &exp); err != nil {
		return "", "", false
	}
	if time.Now().After(exp) {
		s.Delete(token)
		return "", "", false
	}
	return user, pass, true
}

// Delete removes a session by token.
func (s *PGSessionStore) Delete(token string) {
	_, _ = s.db.ExecContext(context.Background(),
		`DELETE FROM mail.sessions WHERE token = $1`,
		token,
	)
}

// SetPass rotates the stored password in an existing session (after an in-place
// password change), preserving the token and expiry so the browser stays signed in.
func (s *PGSessionStore) SetPass(token, pass string) {
	_, _ = s.db.ExecContext(context.Background(),
		`UPDATE mail.sessions SET pass = $2 WHERE token = $1`,
		token, pass,
	)
}

// prune deletes all expired sessions.  Called opportunistically on Create.
func (s *PGSessionStore) prune() {
	_, _ = s.db.ExecContext(context.Background(),
		`DELETE FROM mail.sessions WHERE expires_at < NOW()`,
	)
}
