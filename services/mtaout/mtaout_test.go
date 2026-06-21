package mtaout_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/services/mtaout"
)

// fakeSender returns programmed results and records calls.
type fakeSender struct {
	result func(msg mtaout.OutMessage) mtaout.SendResult
	calls  []string // "rcptDomain@sourceIP"
}

func (f *fakeSender) Send(_ context.Context, msg mtaout.OutMessage, ip string) mtaout.SendResult {
	f.calls = append(f.calls, msg.RcptDomain+"@"+ip)
	if f.result != nil {
		return f.result(msg)
	}
	return mtaout.SendResult{Status: mtaout.Delivered}
}

func TestPoolStableAndClassSegregated(t *testing.T) {
	p := mtaout.NewPool([]string{"10.0.0.1", "10.0.0.2"}, []string{"10.1.0.1", "10.1.0.2"})
	a := p.IPFor("tenantX", mtaout.Transactional)
	if a != p.IPFor("tenantX", mtaout.Transactional) {
		t.Error("tenant->IP must be stable")
	}
	if a == "" {
		t.Fatal("empty IP")
	}
	// Transactional and bulk draw from disjoint pools.
	bulk := p.IPFor("tenantX", mtaout.Bulk)
	if bulk != "10.1.0.1" && bulk != "10.1.0.2" {
		t.Errorf("bulk IP %q not from bulk pool", bulk)
	}
}

func TestWarmupRamp(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	w := mtaout.NewWarmup([]int{2, 5}) // day0: 2, day1+: 5
	for i := 0; i < 2; i++ {
		if !w.Allow("acme.com", now) {
			t.Fatalf("day0 send %d should be allowed", i)
		}
		w.Record("acme.com", now)
	}
	if w.Allow("acme.com", now) {
		t.Error("day0 third send should be held (cap 2)")
	}
	// Next day: cap rises to 5.
	day1 := now.Add(24 * time.Hour)
	if !w.Allow("acme.com", day1) {
		t.Error("day1 should allow again")
	}
}

func TestReputationThrottle(t *testing.T) {
	r := mtaout.NewReputation(20, 0.10, 0.05) // min 20, 10% bounce, 5% complaint
	for i := 0; i < 95; i++ {
		r.RecordDelivered("t1")
	}
	for i := 0; i < 5; i++ {
		r.RecordBounced("t1")
	}
	if r.Throttled("t1") {
		t.Error("5% bounce should not throttle (<=10%)")
	}
	for i := 0; i < 6; i++ {
		r.RecordComplaint("t1") // 6/100 = 6% > 5%
	}
	if !r.Throttled("t1") {
		t.Error("6% complaint should throttle")
	}
	if r.Throttled("fresh") {
		t.Error("unknown tenant must not be throttled")
	}
}

func TestSchedulerPerDomainCap(t *testing.T) {
	fs := &fakeSender{}
	s := mtaout.NewScheduler(mtaout.Config{
		Sender:       fs,
		Pool:         mtaout.NewPool([]string{"10.0.0.1"}, nil),
		MaxPerDomain: 3,
	})
	for i := 0; i < 10; i++ {
		s.Enqueue(mtaout.OutMessage{ID: fmt.Sprint(i), Tenant: "t", FromDomain: "s.com", RcptDomain: "gmail.com", From: "a@s.com", Rcpts: []string{"b@gmail.com"}})
	}
	st := s.Tick(context.Background(), time.Unix(0, 0).UTC())
	if st.Delivered != 3 {
		t.Fatalf("delivered = %d, want 3 (per-domain cap)", st.Delivered)
	}
	if st.RateHeld != 7 {
		t.Errorf("rate-held = %d, want 7", st.RateHeld)
	}
	if s.Pending() != 7 {
		t.Errorf("pending = %d, want 7", s.Pending())
	}
}

func TestSchedulerBackoffAndBounce(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	temp := &fakeSender{result: func(mtaout.OutMessage) mtaout.SendResult { return mtaout.SendResult{Status: mtaout.TempFail} }}
	s := mtaout.NewScheduler(mtaout.Config{
		Sender:       temp,
		MaxPerDomain: 10,
		MaxAttempts:  2,
		Backoff:      func(a int) time.Duration { return time.Duration(a) * time.Minute },
	})
	s.Enqueue(mtaout.OutMessage{ID: "1", Tenant: "t", FromDomain: "s.com", RcptDomain: "x.com"})

	// Attempt 1: TempFail -> deferred (attempts=1, < maxAttempts).
	st := s.Tick(context.Background(), now)
	if st.Deferred != 1 || s.Pending() != 1 {
		t.Fatalf("after attempt1: deferred=%d pending=%d, want 1,1", st.Deferred, s.Pending())
	}
	// Immediately ticking again: still backing off, nothing dispatched.
	st = s.Tick(context.Background(), now)
	if st.Deferred != 0 || st.Delivered != 0 {
		t.Fatalf("should be backing off, got %+v", st)
	}
	// After backoff window, attempt 2 hits maxAttempts -> bounce.
	st = s.Tick(context.Background(), now.Add(2*time.Minute))
	if st.Bounced != 1 || s.Pending() != 0 {
		t.Fatalf("after attempt2: bounced=%d pending=%d, want 1,0", st.Bounced, s.Pending())
	}
}

func TestSchedulerPermFailBounces(t *testing.T) {
	perm := &fakeSender{result: func(mtaout.OutMessage) mtaout.SendResult { return mtaout.SendResult{Status: mtaout.PermFail} }}
	s := mtaout.NewScheduler(mtaout.Config{Sender: perm, MaxPerDomain: 10})
	s.Enqueue(mtaout.OutMessage{ID: "1", Tenant: "t", RcptDomain: "x.com"})
	st := s.Tick(context.Background(), time.Unix(0, 0).UTC())
	if st.Bounced != 1 || s.Pending() != 0 {
		t.Fatalf("permfail should bounce immediately: bounced=%d pending=%d", st.Bounced, s.Pending())
	}
}

func TestSchedulerReputationGate(t *testing.T) {
	fs := &fakeSender{}
	rep := mtaout.NewReputation(1, 0.5, 0.5)
	rep.RecordBounced("bad") // 1/1 = 100% bounce -> throttled
	s := mtaout.NewScheduler(mtaout.Config{Sender: fs, Reputation: rep, MaxPerDomain: 10})
	s.Enqueue(mtaout.OutMessage{ID: "1", Tenant: "bad", RcptDomain: "x.com"})
	st := s.Tick(context.Background(), time.Unix(0, 0).UTC())
	if st.Throttled != 1 || st.Delivered != 0 {
		t.Fatalf("throttled tenant should not send: %+v", st)
	}
}

// A backward clock jump (NTP correction / VM migration) must not panic the
// warmup day-index math (which previously indexed schedule[-1]).
func TestWarmupBackwardClockNoPanic(t *testing.T) {
	w := mtaout.NewWarmup([]int{2, 5, 10})
	t1 := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	w.Record("example.com", t1) // firstSeen = t1
	// Clock jumps back two days; Allow/Record must clamp to day 0, not panic.
	earlier := t1.Add(-48 * time.Hour)
	if !w.Allow("example.com", earlier) {
		t.Error("expected day-0 budget to allow a send after a backward clock jump")
	}
	w.Record("example.com", earlier) // must not panic
}
