package snooze

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

var base = time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

func TestScheduleAndDue(t *testing.T) {
	tests := []struct {
		name      string
		dueOffset time.Duration // item DueAt = base + dueOffset
		nowOffset time.Duration // clock = base + nowOffset
		wantDue   bool
	}{
		{"not yet due", time.Hour, 0, false},
		{"just before due", time.Hour, time.Hour - time.Nanosecond, false},
		{"exactly due", time.Hour, time.Hour, true},
		{"past due", time.Hour, 2 * time.Hour, true},
		{"already overdue at schedule", -time.Hour, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := base
			sc := New(NewMemStore(), func() time.Time { return clock })
			sc.Schedule(Item{ID: "m1", Account: "a", Kind: KindSnooze, DueAt: base.Add(tc.dueOffset)})

			clock = base.Add(tc.nowOffset)
			due := sc.Due()

			if tc.wantDue {
				if len(due) != 1 || due[0].ID != "m1" {
					t.Fatalf("want item m1 due, got %v", due)
				}
				if sc.Pending() != 0 {
					t.Fatalf("due item should be removed, pending=%d", sc.Pending())
				}
			} else {
				if len(due) != 0 {
					t.Fatalf("want nothing due, got %v", due)
				}
				if sc.Pending() != 1 {
					t.Fatalf("item should remain pending, pending=%d", sc.Pending())
				}
			}
		})
	}
}

func TestDueOrder(t *testing.T) {
	clock := base
	sc := New(NewMemStore(), func() time.Time { return clock })

	// Insert out of order; expect ascending DueAt back.
	sc.Schedule(Item{ID: "c", DueAt: base.Add(3 * time.Minute)})
	sc.Schedule(Item{ID: "a", DueAt: base.Add(1 * time.Minute)})
	sc.Schedule(Item{ID: "b", DueAt: base.Add(2 * time.Minute)})
	// Tie on DueAt with "a": broken by ID -> "a" before "z".
	sc.Schedule(Item{ID: "z", DueAt: base.Add(1 * time.Minute)})
	// Future item, should not appear.
	sc.Schedule(Item{ID: "future", DueAt: base.Add(time.Hour)})

	clock = base.Add(10 * time.Minute)
	due := sc.Due()

	var gotIDs []string
	for _, it := range due {
		gotIDs = append(gotIDs, it.ID)
	}
	wantIDs := []string{"a", "z", "b", "c"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("due order = %v, want %v", gotIDs, wantIDs)
	}
	if sc.Pending() != 1 {
		t.Fatalf("future item should remain, pending=%d", sc.Pending())
	}
}

func TestCancel(t *testing.T) {
	clock := base
	sc := New(NewMemStore(), func() time.Time { return clock })
	sc.Schedule(Item{ID: "keep", DueAt: base.Add(time.Hour)})
	sc.Schedule(Item{ID: "drop", DueAt: base.Add(time.Hour)})

	if sc.Pending() != 2 {
		t.Fatalf("pending=%d, want 2", sc.Pending())
	}
	sc.Cancel("drop")
	if sc.Pending() != 1 {
		t.Fatalf("pending=%d after cancel, want 1", sc.Pending())
	}
	sc.Cancel("nonexistent") // no-op
	if sc.Pending() != 1 {
		t.Fatalf("pending=%d after no-op cancel, want 1", sc.Pending())
	}

	clock = base.Add(2 * time.Hour)
	due := sc.Due()
	if len(due) != 1 || due[0].ID != "keep" {
		t.Fatalf("after cancel, due=%v, want [keep]", due)
	}
}

func TestPendingCounts(t *testing.T) {
	clock := base
	store := NewMemStore()
	sc := New(store, func() time.Time { return clock })

	if sc.Pending() != 0 {
		t.Fatalf("empty pending=%d, want 0", sc.Pending())
	}
	sc.Schedule(Item{ID: "1", DueAt: base.Add(time.Minute)})
	sc.Schedule(Item{ID: "2", DueAt: base.Add(2 * time.Minute)})
	if sc.Pending() != 2 {
		t.Fatalf("pending=%d, want 2", sc.Pending())
	}
	// Re-scheduling same ID is an upsert, not a new entry.
	sc.Schedule(Item{ID: "1", DueAt: base.Add(5 * time.Minute)})
	if sc.Pending() != 2 {
		t.Fatalf("pending after upsert=%d, want 2", sc.Pending())
	}

	clock = base.Add(3 * time.Minute) // item "2" due, "1" rescheduled to +5m
	due := sc.Due()
	if len(due) != 1 || due[0].ID != "2" {
		t.Fatalf("due=%v, want [2]", due)
	}
	if sc.Pending() != 1 {
		t.Fatalf("pending after drain=%d, want 1", sc.Pending())
	}
}

func TestRunDrainsToHandler(t *testing.T) {
	clock := base
	sc := New(NewMemStore(), func() time.Time { return clock })
	sc.Schedule(Item{ID: "overdue", Kind: KindSend, DueAt: base.Add(-time.Minute)})

	var mu sync.Mutex
	var fired []string
	handler := func(it Item) {
		mu.Lock()
		defer mu.Unlock()
		fired = append(fired, it.ID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sc.Run(ctx, time.Millisecond, handler)
		close(done)
	}()

	// The startup drain handles the overdue item.
	deadline := time.After(time.Second)
	for {
		mu.Lock()
		n := len(fired)
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("handler never fired for overdue item")
		case <-time.After(time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(fired) < 1 || fired[0] != "overdue" {
		t.Fatalf("fired=%v, want first to be overdue", fired)
	}
}
