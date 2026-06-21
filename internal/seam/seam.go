// Package seam defines the integration boundary that keeps vulos-mail an
// independent, self-contained mail server.
//
// The core (internal/* and adapters/*) depends ONLY on these interfaces. The
// standalone, zero-dependency default implementations live in seam/local; an
// OPTIONAL vulos-cloud adapter lives in integration/cloud and is wired in only
// by the command's composition root when explicitly configured. The core never
// imports anything cloud-specific, so the OSS server runs with no external
// services — pluggable cloud integration is strictly opt-in.
package seam

import (
	"context"
	"errors"
)

// ErrUnsupported is returned by a provider that does not implement an operation
// (e.g. a cloud identity that owns account creation elsewhere).
var ErrUnsupported = errors.New("seam: operation not supported by this provider")

// Identity authenticates users and resolves/provisions accounts. The standalone
// default (seam/local.Identity) is a file-backed bcrypt store; the optional
// cloud adapter validates against vulos-cloud.
type Identity interface {
	// Authenticate verifies credentials and returns the canonical (lower-cased)
	// account address, or an error if the credentials are invalid.
	Authenticate(ctx context.Context, username, password string) (account string, err error)
	// Exists reports whether an account is provisioned (used for RCPT checks and
	// runtime resolution).
	Exists(account string) bool
	// Provision creates a new account. Returns ErrUnsupported if this provider
	// does not own account creation.
	Provision(ctx context.Context, account, password string) error
}

// Plan is an account's effective entitlements.
type Plan struct {
	Tier          string
	MaxSendPerDay int   // 0 = unlimited
	MaxBytes      int64 // mailbox storage cap; 0 = unlimited
	MaxAddresses  int   // 0 = unlimited
	Suspended     bool  // billing lapse / admin suspension
}

// Unlimited is the permissive plan used when no entitlement source is configured
// — i.e. the standalone, self-hosted OSS case.
var Unlimited = Plan{Tier: "self-hosted"}

// Entitlements reports an account's plan. Standalone default = a fixed plan;
// optional cloud adapter = the vulos-cloud quota API.
type Entitlements interface {
	For(ctx context.Context, account string) (Plan, error)
}

// Event is a metered usage record reported to a Usage sink.
type Event struct {
	Kind    string // "send" | "storage"
	Account string
	Count   int64
	Bytes   int64
}

// Usage sinks metered events. Standalone default = no-op; optional cloud adapter
// = vulos-cloud metered_events.
type Usage interface {
	Report(ctx context.Context, ev Event)
}

// SignupGate gates self-serve signup against abuse. Standalone default = Altcha
// proof-of-work (no external service); optional cloud adapter = vulos-cloud PoW
// plus invite codes.
type SignupGate interface {
	// Verify checks an anti-abuse solution for a signup originating from remoteIP.
	Verify(ctx context.Context, solution, remoteIP string) error
}

// OpenGate is a SignupGate that accepts everything — for closed/single-tenant
// instances that don't expose public signup.
type OpenGate struct{}

// Verify always succeeds.
func (OpenGate) Verify(context.Context, string, string) error { return nil }
