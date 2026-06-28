package main

// cmd_migrate.go — `vulos-mail migrate` subcommand.
//
// Usage:
//
//	vulos-mail migrate up     — confirm the storage backend is ready.
//	                            vulos-mail is schemaless (JSONL) or self-
//	                            bootstrapping (SQLite), so this verifies the
//	                            data directory is accessible and exits 0.
//	vulos-mail migrate status — report the active event-log backend and the
//	                            state of the data directory.
//
// Both JSONL and SQLite backends apply their schema at startup (JSONL is
// schemaless; SQLite uses CREATE TABLE IF NOT EXISTS on every open), so
// "migrate up" is always a no-op — it is an operator readiness probe, not
// a DDL runner.

import (
	"flag"
	"fmt"
	"os"
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
	if env("VULOS_DB", "") == "sqlite" {
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

// mailMigrateUp confirms the storage layer is accessible.
// For vulos-mail both backends are schemaless / self-bootstrapping, so this is
// a reachability check: it ensures the data directory can be created and exits 0.
func mailMigrateUp(dataDir, backend string) int {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "migrate up: create data dir %q: %v\n", dataDir, err)
		return 1
	}
	switch backend {
	case "sqlite":
		fmt.Printf("migrate up: sqlite eventlog — schema is applied per-account at first open "+
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
	case "sqlite":
		fmt.Printf("%-30s %s\n", "sqlite_schema",
			"applied per-account at first open (idempotent)")
	default:
		fmt.Printf("%-30s %s\n", "jsonl_schema", "schemaless — no migrations needed")
	}
	return 0
}
