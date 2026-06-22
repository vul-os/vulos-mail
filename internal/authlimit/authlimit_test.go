package authlimit_test

import (
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/authlimit"
)

func TestLocksAfterMaxFailures(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	l := authlimit.New(authlimit.Config{MaxFailures: 3, Window: time.Hour, Lockout: time.Hour, Now: func() time.Time { return now }})

	if l.Locked("1.2.3.4") {
		t.Fatal("fresh key should not be locked")
	}
	for i := 0; i < 3; i++ {
		l.Fail("1.2.3.4")
	}
	if !l.Locked("1.2.3.4") {
		t.Fatal("key should be locked after reaching max failures")
	}
	// A different key is unaffected.
	if l.Locked("5.6.7.8") {
		t.Error("unrelated key should not be locked")
	}
}

func TestSuccessResets(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	l := authlimit.New(authlimit.Config{MaxFailures: 3, Window: time.Hour, Lockout: time.Hour, Now: func() time.Time { return now }})

	l.Fail("ip", "alice")
	l.Fail("ip", "alice")
	// A correct password resets before lockout.
	l.Success("ip", "alice")
	if l.Locked("ip") || l.Locked("alice") {
		t.Fatal("success should clear failure history")
	}
	// And after reset it takes a full max again to lock.
	for i := 0; i < 2; i++ {
		l.Fail("ip")
	}
	if l.Locked("ip") {
		t.Error("should not be locked yet after only 2 fresh failures")
	}
}

func TestLockoutExpires(t *testing.T) {
	clock := time.Unix(1000, 0).UTC()
	l := authlimit.New(authlimit.Config{MaxFailures: 2, Window: time.Hour, Lockout: 10 * time.Minute, Now: func() time.Time { return clock }})

	l.Fail("k")
	l.Fail("k")
	if !l.Locked("k") {
		t.Fatal("should be locked")
	}
	clock = clock.Add(11 * time.Minute) // lockout elapses
	if l.Locked("k") {
		t.Error("lockout should expire after Lockout elapses from last failure")
	}
}

func TestWindowSlides(t *testing.T) {
	clock := time.Unix(0, 0).UTC()
	l := authlimit.New(authlimit.Config{MaxFailures: 3, Window: time.Minute, Lockout: time.Hour, Now: func() time.Time { return clock }})

	l.Fail("k")
	l.Fail("k")
	clock = clock.Add(2 * time.Minute) // old failures age out of the window
	l.Fail("k")
	if l.Locked("k") {
		t.Error("stale failures outside the window should not count toward the lock")
	}
}

func TestAnyLockedAndEmptyKey(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	l := authlimit.New(authlimit.Config{MaxFailures: 1, Window: time.Hour, Lockout: time.Hour, Now: func() time.Time { return now }})

	if l.Locked("") {
		t.Fatal("empty key must never be locked (fail-open)")
	}
	l.Fail("acct")
	if !l.AnyLocked("freship", "acct") {
		t.Fatal("AnyLocked should report true when any key is locked")
	}
}
