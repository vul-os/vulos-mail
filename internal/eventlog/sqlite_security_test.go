package eventlog_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/model"
)

// SQL-injection-laced message content must store + round-trip intact and must not
// affect the schema — proving parameterized queries, not string interpolation.
func TestSQLiteAdversarialContentSafe(t *testing.T) {
	ctx := context.Background()
	l, err := eventlog.OpenSQLite(filepath.Join(t.TempDir(), "log.db"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	evil := "'); DROP TABLE events;-- \"quoted\" <script>\r\nRobert\"); DROP TABLE events; --"
	if _, err := l.Append(ctx, "actor", event.MessageIngested{
		MessageID: "m1", BlobRef: "sha256:x",
		Envelope: model.Envelope{Subject: evil, From: []string{evil}},
	}); err != nil {
		t.Fatal(err)
	}
	recs, err := l.ReadFrom(ctx, 1)
	if err != nil || len(recs) != 1 { // the table must still exist + hold the record
		t.Fatalf("ReadFrom = %d records, %v (table dropped by injection?)", len(recs), err)
	}
	mi, ok := recs[0].Event.(event.MessageIngested)
	if !ok || mi.Envelope.Subject != evil {
		t.Fatalf("adversarial content was altered in storage: %+v", recs[0].Event)
	}
	if n, _ := l.Len(ctx); n != 1 {
		t.Fatalf("Len = %d after adversarial append, want 1", n)
	}
}
