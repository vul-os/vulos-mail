package eventlog_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/event"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/model"
)

func fixedNow() time.Time { return time.Unix(0, 0).UTC() }

// The durable log must survive reopen: recover Seq, continue numbering without
// gaps, and decode events back to their concrete types.
func TestFileLogPersistsAndRecovers(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "log.jsonl")

	l, err := eventlog.OpenFile(p, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(ctx, "a", event.MessageIngested{MessageID: "m1", BlobRef: "sha256:x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(ctx, "a", event.Labeled{MessageID: "m1", LabelID: model.LabelStar}); err != nil {
		t.Fatal(err)
	}

	// Reopen: Seq must be recovered, next append continues at 3.
	l2, err := eventlog.OpenFile(p, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
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

	recs, err := l2.ReadFrom(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("ReadFrom returned %d records, want 3", len(recs))
	}
	if _, ok := recs[0].Event.(event.MessageIngested); !ok {
		t.Fatalf("record 0 decoded to %T, want MessageIngested", recs[0].Event)
	}
	if mi, ok := recs[0].Event.(event.MessageIngested); !ok || mi.MessageID != "m1" {
		t.Fatalf("record 0 payload wrong: %+v", recs[0].Event)
	}
}

func TestReadFromOffset(t *testing.T) {
	ctx := context.Background()
	l := eventlog.NewMem(fixedNow)
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
