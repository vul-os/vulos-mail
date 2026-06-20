package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/mailsettings"
	"github.com/vul-os/vmail/internal/server"
	"github.com/vul-os/vmail/services/mtaout"
)

func TestVacationAutoReply(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	blobs, _ := blob.NewFS(filepath.Join(dir, "blobs"))
	sched := mtaout.NewScheduler(mtaout.Config{Sender: &okSender{}, MaxPerDomain: 10})
	mgr := server.NewManager(dir, blobs, sched)
	mgr.Settings = mailsettings.NewStore()
	mgr.Vacation = mailsettings.NewResponder(24*time.Hour, func() time.Time { return time.Unix(0, 0).UTC() })
	_ = mgr.AddAccount("alice@vmail.test", "pw")
	mgr.Settings.Set("alice@vmail.test", mailsettings.Settings{
		Vacation: mailsettings.Vacation{Enabled: true, Subject: "Away", Body: "back monday"},
	})

	deliver := func(from string, extraHeaders string) {
		raw := "From: " + from + "\r\nTo: alice@vmail.test\r\nSubject: hi\r\n" + extraHeaders + "\r\nhello\r\n"
		if err := mgr.Deliver(ctx, "alice@vmail.test", []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}

	deliver("bob@out.example", "")
	if sched.Pending() != 1 {
		t.Fatalf("expected 1 vacation reply queued, got %d", sched.Pending())
	}
	// Same sender again within the period: no new reply.
	deliver("bob@out.example", "")
	if sched.Pending() != 1 {
		t.Errorf("same sender should not get a second reply, pending=%d", sched.Pending())
	}
	// Different sender: a new reply.
	deliver("carol@out.example", "")
	if sched.Pending() != 2 {
		t.Errorf("new sender should get a reply, pending=%d", sched.Pending())
	}
	// Automated mail must not trigger a reply (loop protection).
	deliver("dave@out.example", "Auto-Submitted: auto-replied\r\n")
	if sched.Pending() != 2 {
		t.Errorf("automated mail must not auto-reply, pending=%d", sched.Pending())
	}
	// Daemon sender: no reply.
	deliver("mailer-daemon@out.example", "")
	if sched.Pending() != 2 {
		t.Errorf("daemon sender must not auto-reply, pending=%d", sched.Pending())
	}
}
