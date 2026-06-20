package tenant_test

import (
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/tenant"
)

func TestRegistryResolution(t *testing.T) {
	r := tenant.NewRegistry()
	r.Map("acme.com", "tenant-acme")
	r.Map("acme.io", "tenant-acme") // two domains, one tenant
	if got := r.TenantFor("alice@acme.com"); got != "tenant-acme" {
		t.Errorf("acme.com -> %q, want tenant-acme", got)
	}
	if got := r.TenantFor("bob@acme.io"); got != "tenant-acme" {
		t.Errorf("acme.io -> %q, want tenant-acme", got)
	}
	// Unmapped domain is its own tenant.
	if got := r.TenantFor("x@other.org"); got != "other.org" {
		t.Errorf("unmapped -> %q, want other.org", got)
	}
}

func TestQuotaMessageCapAndDailyReset(t *testing.T) {
	clock := time.Unix(0, 0).UTC()
	q := tenant.NewQuota(2, 0, func() time.Time { return clock })

	if ok, _ := q.Allow("t", 100); !ok {
		t.Fatal("1st send should be allowed")
	}
	if ok, _ := q.Allow("t", 100); !ok {
		t.Fatal("2nd send should be allowed")
	}
	if ok, reason := q.Allow("t", 100); ok || reason == "" {
		t.Fatalf("3rd send should be denied with a reason, got ok=%v", ok)
	}
	// A different tenant is independent.
	if ok, _ := q.Allow("u", 100); !ok {
		t.Error("other tenant should be allowed")
	}
	// Next day resets.
	clock = clock.Add(24 * time.Hour)
	if ok, _ := q.Allow("t", 100); !ok {
		t.Error("quota should reset the next day")
	}
}

func TestQuotaByteCap(t *testing.T) {
	q := tenant.NewQuota(0, 1000, func() time.Time { return time.Unix(0, 0).UTC() })
	if ok, _ := q.Allow("t", 600); !ok {
		t.Fatal("600 bytes under 1000 cap should allow")
	}
	if ok, _ := q.Allow("t", 600); ok {
		t.Error("another 600 (total 1200 > 1000) should deny")
	}
}
