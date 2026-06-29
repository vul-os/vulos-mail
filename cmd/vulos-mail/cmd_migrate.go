package main

// cmd_migrate.go — `vulos-mail migrate` subcommand.
//
// Usage:
//
//	vulos-mail migrate up     — apply/verify the storage backend.
//	                            JSONL and SQLite are schemaless/self-bootstrapping
//	                            (reachability check only).  When DATABASE_URL /
//	                            VULOS_DATABASE_URL is set, the Postgres "mail"
//	                            schema DDL is applied idempotently (safe to run
//	                            on every deploy).
//	vulos-mail migrate status — report the active event-log backend.

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/vul-os/vulos-mail/internal/mailpg"
)

// runMigrate is the entry point for the `migrate` subcommand.
func runMigrate(args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dataDir := fs.String("data-dir", env("VULOS_DATA_DIR", "./data"),
		"path to data directory (overrides VULOS_DATA_DIR)")
	_ = fs.Parse(args)

	subcmd := "up"
	if fs.NArg() > 0 {
		subcmd = fs.Arg(0)
	}

	backend := "jsonl"
	if mailpg.DSN() != "" {
		backend = "postgres"
	} else if env("VULOS_DB", "") == "sqlite" {
		backend = "sqlite"
	}

	switch subcmd {
	case "up":
		return mailMigrateUp(*dataDir, backend)
	case "status":
		return mailMigrateStatus(*dataDir, backend)
	default:
		fmt.Fprintf(os.Stderr, "migrate: unknown subcommand %q (valid: up, status)\n", subcmd)
		return 2
	}
}

// mailMigrateUp applies/verifies the storage layer.
func mailMigrateUp(dataDir, backend string) int {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "migrate up: create data dir %q: %v\n", dataDir, err)
		return 1
	}
	switch backend {
	case "postgres":
		dsn := mailpg.DSN()
		db, err := mailpg.Open(dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate up: postgres open: %v\n", err)
			return 1
		}
		defer db.Close()
		if err := mailpg.Migrate(context.Background(), db); err != nil {
			fmt.Fprintf(os.Stderr, "migrate up: postgres migrate: %v\n", err)
			return 1
		}
		fmt.Printf("migrate up: postgres — schema=mail applied (idempotent); data dir %s ok\n", dataDir)
	case "sqlite":
		fmt.Printf("migrate up: sqlite eventlog — schema applied per-account at first open "+
			"(CREATE TABLE IF NOT EXISTS); data dir %s ok\n", dataDir)
	default:
		fmt.Printf("migrate up: jsonl eventlog — schemaless; data dir %s ok\n", dataDir)
	}
	return 0
}

// mailMigrateStatus prints the active backend and data-directory reachability.
func mailMigrateStatus(dataDir, backend string) int {
	fmt.Printf("%-30s %s\n", "COMPONENT", "STATUS")
	fmt.Printf("%-30s %s\n", "eventlog_backend", backend)

	dirStatus := "ok"
	if _, err := os.Stat(dataDir); err != nil {
		dirStatus = fmt.Sprintf("not yet created (created at startup): %v", err)
	}
	fmt.Printf("%-30s %s\n", "data_dir", dirStatus)

	switch backend {
	case "postgres":
		fmt.Printf("%-30s %s\n", "postgres_schema", "mail (events, snapshots, settings, sessions)")
		fmt.Printf("%-30s %s\n", "blob_store", "object storage (unchanged — bodies never in PG)")
	case "sqlite":
		fmt.Printf("%-30s %s\n", "sqlite_schema", "applied per-account at first open (idempotent)")
	default:
		fmt.Printf("%-30s %s\n", "jsonl_schema", "schemaless — no migrations needed")
	}
	return 0
}
