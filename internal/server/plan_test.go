package server_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/seam"
)

type stubPlans struct{ plan seam.Plan }

func (s stubPlans) For(context.Context, string) (seam.Plan, error) { return s.plan, nil }

// errPlans simulates an unreachable entitlement source (must fail open).
type errPlans struct{}

func (errPlans) For(context.Context, string) (seam.Plan, error) {
	return seam.Plan{}, context.DeadlineExceeded
}

// togglePlans returns a plan for the first failAfter calls, then errors — to
// exercise the bounded last-known-entitlement cache.
type togglePlans struct {
	plan      seam.Plan
	failAfter int
	n         int
}

func (t *togglePlans) For(context.Context, string) (seam.Plan, error) {
	t.n++
	if t.n > t.failAfter {
		return seam.Plan{}, errors.New("cp down")
	}
	return t.plan, nil
}

func TestPlanDailySendCapEnforced(t *testing.T) {
	m := newMgr(t)
	m.Plans = stubPlans{seam.Plan{Tier: "free", MaxSendPerDay: 3}}
	for i := 0; i < 3; i++ {
		if err := m.CheckQuota("a@vulos.to", 100); err != nil {
			t.Fatalf("send %d within cap rejected: %v", i+1, err)
		}
	}
	if err := m.CheckQuota("a@vulos.to", 100); err == nil {
		t.Fatal("4th send over the daily cap should be rejected")
	}
	// A different account has its own independent budget.
	if err := m.CheckQuota("b@vulos.to", 100); err != nil {
		t.Fatalf("other account should be unaffected: %v", err)
	}
}

func TestPlanSuspendedBlocksSend(t *testing.T) {
	m := newMgr(t)
	m.Plans = stubPlans{seam.Plan{Tier: "pro", Suspended: true}}
	if err := m.CheckQuota("a@vulos.to", 1); err == nil {
		t.Fatal("suspended account must not be able to send")
	}
}

func TestNoPlansMeansUnlimited(t *testing.T) {
	m := newMgr(t) // m.Plans == nil (standalone)
	for i := 0; i < 200; i++ {
		if err := m.CheckQuota("a@vulos.to", 1); err != nil {
			t.Fatalf("standalone send %d rejected: %v", i, err)
		}
	}
}

func TestPlanLookupErrorFailsOpen(t *testing.T) {
	m := newMgr(t)
	m.Plans = errPlans{}
	if err := m.CheckQuota("a@vulos.to", 1); err != nil {
		t.Fatalf("entitlement-source error must fail open, got: %v", err)
	}
}

func TestPlanStorageCapEnforced(t *testing.T) {
	ctx := context.Background()
	m := newMgr(t)
	_ = m.AddAccount("alice@vulos.to", "pw")
	msg := []byte("From: x@y\r\nTo: alice@vulos.to\r\nSubject: s\r\n\r\n" + strings.Repeat("A", 500) + "\r\n")
	// Room for roughly one message.
	m.Plans = stubPlans{seam.Plan{Tier: "free", MaxBytes: int64(len(msg)) + 100}}
	if err := m.Deliver(ctx, "alice@vulos.to", msg); err != nil {
		t.Fatalf("first delivery within quota rejected: %v", err)
	}
	if err := m.Deliver(ctx, "alice@vulos.to", msg); err == nil {
		t.Fatal("delivery over the storage quota must be rejected")
	}
}

func TestPlanBoundedCachePreservesSuspensionOnError(t *testing.T) {
	m := newMgr(t)
	// First fetch returns suspended (cached); subsequent fetches error.
	m.Plans = &togglePlans{plan: seam.Plan{Tier: "pro", Suspended: true}, failAfter: 1}
	if err := m.CheckQuota("a@vulos.to", 1); err == nil {
		t.Fatal("suspended account should be blocked on the fresh fetch")
	}
	if err := m.CheckQuota("a@vulos.to", 1); err == nil {
		t.Fatal("suspension must persist from the bounded cache during a cp error (not fail fully open)")
	}
}
