package eventlog_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vul-os/vmail/internal/event"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/model"
)

// The SQLite log must persist across reopen: recover Seq, continue numbering
// without gaps, decode events to their concrete types, and honor ReadFrom.
func TestSQLiteLogPersistsAndRecovers(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "log.db")

	l, err := eventlog.OpenSQLite(p, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(ctx, "a", event.MessageIngested{MessageID: "m1", BlobRef: "sha256:x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(ctx, "a", event.Labeled{MessageID: "m1", LabelID: model.LabelStar}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the same path: Seq recovered, next append continues at 3.
	l2, err := eventlog.OpenSQLite(p, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()

	if n, _ := l2.Len(ctx); n != 2 {
		t.Fatalf("Len after reopen = %d, want 2", n)
	}
	r3, err := l2.Append(ctx, "a", event.FlagSet{MessageID: "m1", Flag: model.FlagSeen, Value: true})
	if err != nil {
		t.Fatal(err)
	}
	if r3.Seq != 3 {
		t.Fatalf("Seq = %d, want 3", r3.Seq)
	}

	// ReadFrom(2) returns seq 2 and 3 only, in order.
	recs, err := l2.ReadFrom(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("ReadFrom(2) returned %d records, want 2", len(recs))
	}
	if recs[0].Seq != 2 || recs[1].Seq != 3 {
		t.Fatalf("ReadFrom(2) seqs = %d,%d, want 2,3", recs[0].Seq, recs[1].Seq)
	}
	if _, ok := recs[0].Event.(event.Labeled); !ok {
		t.Fatalf("record 0 decoded to %T, want Labeled", recs[0].Event)
	}

	// Full read decodes the first record to its concrete type with fields intact.
	all, err := l2.ReadFrom(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ReadFrom(1) returned %d records, want 3", len(all))
	}
	mi, ok := all[0].Event.(event.MessageIngested)
	if !ok {
		t.Fatalf("record 0 decoded to %T, want MessageIngested", all[0].Event)
	}
	if mi.MessageID != "m1" || mi.BlobRef != "sha256:x" {
		t.Fatalf("record 0 payload wrong: %+v", mi)
	}
}

func TestSQLiteReadFromOffset(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "offset.db")

	l, err := eventlog.OpenSQLite(p, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	for i := 0; i < 5; i++ {
		if _, err := l.Append(ctx, "a", event.FlagSet{MessageID: "m1", Flag: model.FlagSeen, Value: true}); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := l.ReadFrom(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0].Seq != 3 {
		t.Fatalf("ReadFrom(3) = %d records starting at %d, want 3 starting at 3", len(recs), recs[0].Seq)
	}
}

// An empty log reports Len 0 and an empty ReadFrom.
func TestSQLiteEmpty(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "empty.db")

	l, err := eventlog.OpenSQLite(p, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if n, _ := l.Len(ctx); n != 0 {
		t.Fatalf("Len on empty = %d, want 0", n)
	}
	recs, err := l.ReadFrom(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("ReadFrom on empty returned %d records, want 0", len(recs))
	}
}

// SQLite must satisfy the Log interface.
var _ eventlog.Log = (*eventlog.SQLite)(nil)
