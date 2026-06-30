package mtaout_test

import (
	"context"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/services/mtaout"
)

// TestFileQueueDurabilityAndRecovery proves the durability guarantee: a message
// that was Enqueued (250 OK) is persisted, survives a "restart" (a fresh
// Scheduler built over the same FileQueue), and is then delivered — never lost.
func TestFileQueueDurabilityAndRecovery(t *testing.T) {
	dir := t.TempDir()
	fq, err := mtaout.NewFileQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Process #1: enqueue, then crash before any Tick.
	s1 := mtaout.NewScheduler(mtaout.Config{Sender: &fakeSender{}, Store: fq, MaxPerDomain: 10})
	if err := s1.Enqueue(mtaout.OutMessage{Tenant: "t", RcptDomain: "x.com", From: "a@s.com", Rcpts: []string{"b@x.com"}}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// The message is on disk independently of the in-memory scheduler.
	items, err := fq.Load()
	if err != nil || len(items) != 1 {
		t.Fatalf("after enqueue: load=%d err=%v, want 1 persisted item", len(items), err)
	}
	if items[0].Msg.ID == "" {
		t.Fatal("persisted message has no ID")
	}

	// Process #2: fresh scheduler over the same store recovers the message.
	delivered := &fakeSender{}
	s2 := mtaout.NewScheduler(mtaout.Config{Sender: delivered, Store: fq, MaxPerDomain: 10})
	if err := s2.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if s2.Pending() != 1 {
		t.Fatalf("after recovery pending=%d, want 1", s2.Pending())
	}
	st := s2.Tick(context.Background(), time.Now())
	if st.Delivered != 1 {
		t.Fatalf("delivered=%d, want 1", st.Delivered)
	}
	// Delivered → removed from durable storage so a later restart won't re-send.
	if items, _ := fq.Load(); len(items) != 0 {
		t.Fatalf("after delivery store has %d items, want 0", len(items))
	}
}

// TestFileQueueDeferStatePersists checks that a TempFail defer persists the
// updated attempt count so a restart resumes (rather than restarting) the climb
// toward the eventual bounce.
func TestFileQueueDeferStatePersists(t *testing.T) {
	dir := t.TempDir()
	fq, err := mtaout.NewFileQueue(dir)
	if err != nil {
		t.Fatal(err)
	}
	temp := &fakeSender{result: func(mtaout.OutMessage) mtaout.SendResult { return mtaout.SendResult{Status: mtaout.TempFail} }}
	s := mtaout.NewScheduler(mtaout.Config{Sender: temp, Store: fq, MaxPerDomain: 10, MaxAttempts: 5,
		Backoff: func(int) time.Duration { return time.Minute }})
	if err := s.Enqueue(mtaout.OutMessage{ID: "m1", Tenant: "t", RcptDomain: "x.com"}); err != nil {
		t.Fatal(err)
	}
	s.Tick(context.Background(), time.Unix(0, 0).UTC())

	items, err := fq.Load()
	if err != nil || len(items) != 1 {
		t.Fatalf("load=%d err=%v, want 1", len(items), err)
	}
	if items[0].Attempts != 1 {
		t.Fatalf("persisted attempts=%d, want 1", items[0].Attempts)
	}
	if items[0].NextAttempt.IsZero() {
		t.Fatal("persisted nextAttempt is zero; backoff not recorded")
	}
}
