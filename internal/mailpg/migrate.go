package mailpg

import (
	"context"
	"database/sql"
	"fmt"
)

// schema is the canonical DDL for the "mail" schema.  Every statement is
// idempotent (IF NOT EXISTS / IF NOT EXISTS) so Migrate can be run at every
// boot and by `vulos-mail migrate up`.
const schema = `
-- Dedicated schema so all mail tables live in one namespace on a shared Neon
-- database alongside other Vulos products (cp, files, etc.).
CREATE SCHEMA IF NOT EXISTS mail;

-- per-account event log: the source of truth for all message state changes.
-- (account, seq) is the composite primary key; seq is monotonically increasing
-- per account starting at 1 with no gaps.
CREATE TABLE IF NOT EXISTS mail.events (
    account TEXT   NOT NULL,
    seq     BIGINT NOT NULL,
    ts      BIGINT NOT NULL,
    actor   TEXT   NOT NULL DEFAULT '',
    event   BYTEA  NOT NULL,
    PRIMARY KEY (account, seq)
);

-- Projection checkpoints: the last snapshot persisted for an account so replay
-- on re-open only covers the tail (seq > through_seq).
CREATE TABLE IF NOT EXISTS mail.snapshots (
    account      TEXT   PRIMARY KEY,
    through_seq  BIGINT NOT NULL,
    data         BYTEA  NOT NULL
);

-- Per-account settings: JSON blob holding signature, aliases, vacation
-- responder, and the home_region cell identifier.
CREATE TABLE IF NOT EXISTS mail.settings (
    account TEXT  PRIMARY KEY,
    data    JSONB NOT NULL DEFAULT '{}'
);

-- Web sessions: token → (account, pass, expiry).  Allows webmail sessions to
-- survive restarts in multi-replica cloud deployments.
-- NOTE: the IMAP password is stored server-side so the /v1 proxy can broker
-- it to the lilmail engine.  Ensure the Postgres connection uses TLS.
CREATE TABLE IF NOT EXISTS mail.sessions (
    token      TEXT        PRIMARY KEY,
    account    TEXT        NOT NULL,
    pass       TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS mail_sessions_expires ON mail.sessions (expires_at);

-- Outbound queue: durable backing for the mtaout scheduler so acknowledged
-- (250 OK) mail survives a crash/redeploy on ephemeral compute. One row per
-- queued message; "item" is the JSON-encoded mtaout.QueuedItem (message + retry
-- state). Rows are deleted on delivery or final bounce.
CREATE TABLE IF NOT EXISTS mail.outqueue (
    id   TEXT  PRIMARY KEY,
    item JSONB NOT NULL
);
`

// Migrate applies the "mail" schema DDL.  It is idempotent: safe to call on
// every startup and from the `vulos-mail migrate up` subcommand.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("mailpg: migrate: %w", err)
	}
	return nil
}
