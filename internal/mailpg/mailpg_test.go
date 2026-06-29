// Package mailpg_test contains integration tests for the Postgres mail store.
//
// SQLite and JSONL default tests always run (no env required).
// Postgres tests run only when VULOS_TEST_POSTGRES is set to a valid postgres://
// DSN pointing at a test database with CREATE SCHEMA privileges.
//
// Example:
//
//	VULOS_TEST_POSTGRES=postgres://user:pass@localhost:5432/testdb go test ./internal/mailpg/...
package mailpg_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/mailpg"
	"github.com/vul-os/vulos-mail/internal/mailsettings"
	"github.com/vul-os/vulos-mail/internal/model"
)

// openPGDB connects and migrates a test database, or skips when no DSN is set.
func openPGDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("VULOS_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("VULOS_TEST_POSTGRES not set; skipping Postgres integration tests")
	}
	db, err := mailpg.Open(dsn)
	if err != nil {
		t.Fatalf("mailpg.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := mailpg.Migrate(context.Background(), db); err != nil {
		t.Fatalf("mailpg.Migrate: %v", err)
	}
	return db
}

// ---------------------------------------------------------------------------
// Event log tests (Postgres-gated)
// ---------------------------------------------------------------------------

func TestPGLog_AppendReadFromLen(t *testing.T) {
	db := openPGDB(t)
	ctx := context.Background()
	account := "test+pglog@pg.test"

	// Clean up from previous runs.
	db.ExecContext(ctx, `DELETE FROM mail.events WHERE account = $1`, account)
	db.ExecContext(ctx, `DELETE FROM mail.snapshots WHERE account = $1`, account)

	lg := mailpg.NewPGLog(db, account, nil)

	r1, err := lg.Append(ctx, "test", event.MessageIngested{
		MessageID: model.ID("msg1"),
		BlobRef:   model.BlobRef("sha256:0001"),
	})
	if err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if r1.Seq != 1 {
		t.Errorf("first seq = %d; want 1", r1.Seq)
	}

	r2, err := lg.Append(ctx, "test", event.MessageIngested{
		MessageID: model.ID("msg2"),
		BlobRef:   model.BlobRef("sha256:0002"),
	})
	if err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	if r2.Seq != 2 {
		t.Errorf("second seq = %d; want 2", r2.Seq)
	}

	recs, err := lg.ReadFrom(ctx, 1)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("ReadFrom returned %d records; want 2", len(recs))
	}
	if recs[0].Seq != 1 || recs[1].Seq != 2 {
		t.Errorf("seq order wrong: %v", []uint64{recs[0].Seq, recs[1].Seq})
	}

	// ReadFrom with a starting seq past all records.
	tail, err := lg.ReadFrom(ctx, 3)
	if err != nil {
		t.Fatalf("ReadFrom tail: %v", err)
	}
	if len(tail) != 0 {
		t.Errorf("ReadFrom(3) returned %d records; want 0", len(tail))
	}

	n, err := lg.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if n != 2 {
		t.Errorf("Len = %d; want 2", n)
	}
}

func TestPGLog_Snapshot(t *testing.T) {
	db := openPGDB(t)
	ctx := context.Background()
	account := "test+snap@pg.test"

	db.ExecContext(ctx, `DELETE FROM mail.events WHERE account = $1`, account)
	db.ExecContext(ctx, `DELETE FROM mail.snapshots WHERE account = $1`, account)

	lg := mailpg.NewPGLog(db, account, nil)

	// No snapshot yet.
	_, _, ok, err := lg.LoadSnapshot()
	if err != nil || ok {
		t.Fatalf("LoadSnapshot before save: ok=%v err=%v", ok, err)
	}

	// Append two events.
	for i := 0; i < 2; i++ {
		if _, err := lg.Append(ctx, "t", event.MessageIngested{
			MessageID: model.ID("m"),
			BlobRef:   model.BlobRef("sha256:snap"),
		}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// Save snapshot covering seq 1 → should compact seq 1.
	snap := []byte(`{"snapshot":"test"}`)
	if err := lg.SaveSnapshot(1, snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Seq 1 was compacted; seq 2 remains.
	recs, err := lg.ReadFrom(ctx, 1)
	if err != nil {
		t.Fatalf("ReadFrom after snapshot: %v", err)
	}
	if len(recs) != 1 || recs[0].Seq != 2 {
		t.Errorf("after snapshot, ReadFrom(1) = %d records; want 1 at seq=2", len(recs))
	}

	// Round-trip the snapshot.
	gotData, gotSeq, gotOk, gotErr := lg.LoadSnapshot()
	if gotErr != nil || !gotOk {
		t.Fatalf("LoadSnapshot after save: ok=%v err=%v", gotOk, gotErr)
	}
	if gotSeq != 1 || string(gotData) != string(snap) {
		t.Errorf("snapshot mismatch: seq=%d data=%s", gotSeq, gotData)
	}
}

// ---------------------------------------------------------------------------
// Settings store tests (Postgres-gated)
// ---------------------------------------------------------------------------

func TestPGSettings_GetSet(t *testing.T) {
	db := openPGDB(t)
	ctx := context.Background()
	account := "test+settings@pg.test"

	db.ExecContext(ctx, `DELETE FROM mail.settings WHERE account = $1`, account)

	st := mailpg.NewPGSettings(db)

	// Zero value for unknown account.
	got := st.Get(account)
	if got.HomeRegion != "" || got.Signature != "" {
		t.Errorf("initial settings non-zero: %+v", got)
	}

	want := mailsettings.Settings{
		Signature:  "Sent from Vulos Mail",
		HomeRegion: "eu",
		Vacation: mailsettings.Vacation{
			Enabled: true,
			Subject: "OOO",
			Body:    "Back next week",
		},
	}
	st.Set(account, want)
	got = st.Get(account)

	if got.HomeRegion != "eu" {
		t.Errorf("HomeRegion = %q; want eu", got.HomeRegion)
	}
	if got.Signature != want.Signature {
		t.Errorf("Signature = %q; want %q", got.Signature, want.Signature)
	}
	if !got.Vacation.Enabled || got.Vacation.Subject != "OOO" {
		t.Errorf("Vacation mismatch: %+v", got.Vacation)
	}

	// Overwrite.
	st.Set(account, mailsettings.Settings{HomeRegion: "us"})
	got = st.Get(account)
	if got.HomeRegion != "us" {
		t.Errorf("after overwrite HomeRegion = %q; want us", got.HomeRegion)
	}
}

// ---------------------------------------------------------------------------
// Session store tests (Postgres-gated)
// ---------------------------------------------------------------------------

func TestPGSessionStore_CRUD(t *testing.T) {
	db := openPGDB(t)

	ss := mailpg.NewPGSessionStore(db, time.Hour)

	tok := ss.Create("alice@pg.test", "hunter2")
	if tok == "" {
		t.Fatal("Create returned empty token")
	}

	user, pass, ok := ss.Get(tok)
	if !ok {
		t.Fatal("Get: not found after Create")
	}
	if user != "alice@pg.test" || pass != "hunter2" {
		t.Errorf("Get = (%q, %q); want (alice@pg.test, hunter2)", user, pass)
	}

	ss.SetPass(tok, "newpass")
	_, pass2, ok2 := ss.Get(tok)
	if !ok2 || pass2 != "newpass" {
		t.Errorf("after SetPass: ok=%v pass=%q; want (true, newpass)", ok2, pass2)
	}

	ss.Delete(tok)
	if _, _, ok3 := ss.Get(tok); ok3 {
		t.Error("Get after Delete returned ok=true")
	}
}

func TestPGSessionStore_Expiry(t *testing.T) {
	db := openPGDB(t)
	// Very short TTL so we can test expiry without sleeping.
	ss := mailpg.NewPGSessionStore(db, -1*time.Second) // already expired

	tok := ss.Create("bob@pg.test", "pass")
	_, _, ok := ss.Get(tok)
	if ok {
		t.Error("Get of already-expired session returned ok=true")
	}
}

// ---------------------------------------------------------------------------
// Compile-time interface checks (always run, no external env needed)
// ---------------------------------------------------------------------------

var _ mailsettings.SettingsStore = (*mailpg.PGSettings)(nil)
