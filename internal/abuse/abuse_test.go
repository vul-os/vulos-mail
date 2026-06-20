package abuse_test

import (
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/abuse"
)

func TestRateLimitThrottles(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	f := abuse.New(abuse.Config{Window: time.Hour, MaxPerWindow: 3, MaxRecipients: 100, Now: func() time.Time { return now }})

	for i := 0; i < 3; i++ {
		if a, _ := f.Check("alice", 1); a != abuse.Allow {
			t.Fatalf("send %d should be allowed, got %v", i, a)
		}
	}
	if a, _ := f.Check("alice", 1); a != abuse.Throttle {
		t.Errorf("4th send should throttle, got %v", a)
	}
	// A different account is unaffected.
	if a, _ := f.Check("bob", 1); a != abuse.Allow {
		t.Errorf("bob should be allowed, got %v", a)
	}
}

func TestRecipientBurstAutoSuspends(t *testing.T) {
	f := abuse.New(abuse.Config{MaxRecipients: 50})
	if a, _ := f.Check("mallory", 500); a != abuse.Suspend {
		t.Fatalf("recipient burst should suspend, got %v", a)
	}
	if !f.Suspended("mallory") {
		t.Error("account should be suspended")
	}
	// Subsequent sends stay suspended until reinstated.
	if a, _ := f.Check("mallory", 1); a != abuse.Suspend {
		t.Errorf("suspended account should stay suspended, got %v", a)
	}
	f.Reinstate("mallory")
	if a, _ := f.Check("mallory", 1); a != abuse.Allow {
		t.Errorf("after reinstate should allow, got %v", a)
	}
}

func TestWindowSlides(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	clock := now
	f := abuse.New(abuse.Config{Window: time.Minute, MaxPerWindow: 2, MaxRecipients: 100, Now: func() time.Time { return clock }})
	f.Check("a", 1)
	f.Check("a", 1)
	if a, _ := f.Check("a", 1); a != abuse.Throttle {
		t.Fatalf("should throttle at limit, got %v", a)
	}
	clock = now.Add(2 * time.Minute) // window passes
	if a, _ := f.Check("a", 1); a != abuse.Allow {
		t.Errorf("after window slides should allow again, got %v", a)
	}
}
