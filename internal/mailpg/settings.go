package mailpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vul-os/vulos-mail/internal/mailsettings"
)

// PGSettings is a Postgres-backed implementation of mailsettings.SettingsStore.
// Settings are serialised as JSON and stored in mail.settings.data (JSONB).
type PGSettings struct {
	db *sql.DB
}

// NewPGSettings returns a PGSettings backed by db.
func NewPGSettings(db *sql.DB) *PGSettings {
	return &PGSettings{db: db}
}

// Get returns the settings for account (zero value if not stored yet).
func (s *PGSettings) Get(account string) mailsettings.Settings {
	row := s.db.QueryRowContext(context.Background(),
		`SELECT data FROM mail.settings WHERE account = $1`,
		strings.ToLower(account),
	)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		// sql.ErrNoRows → zero settings; other errors silently return zero too.
		return mailsettings.Settings{}
	}
	var st mailsettings.Settings
	if err := json.Unmarshal(raw, &st); err != nil {
		return mailsettings.Settings{}
	}
	return st
}

// Set persists the settings for account.
func (s *PGSettings) Set(account string, st mailsettings.Settings) {
	raw, err := json.Marshal(st)
	if err != nil {
		return // unreachable for well-typed struct
	}
	_, _ = s.db.ExecContext(context.Background(),
		`INSERT INTO mail.settings (account, data) VALUES ($1, $2)
		 ON CONFLICT (account) DO UPDATE SET data = EXCLUDED.data`,
		strings.ToLower(account), raw,
	)
}

// Ensure PGSettings satisfies the interface at compile time.
var _ mailsettings.SettingsStore = (*PGSettings)(nil)

// pgSettingsErr is returned when a settings operation fails.
type pgSettingsErr struct{ cause error }

func (e pgSettingsErr) Error() string {
	return fmt.Sprintf("mailpg settings: %v", e.cause)
}
