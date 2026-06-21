package server_test

import (
	"context"
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
