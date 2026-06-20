package server_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/internal/server"
	"github.com/vul-os/vmail/services/mtaout"
)

type permFailSender struct{}

func (permFailSender) Send(context.Context, mtaout.OutMessage, string) mtaout.SendResult {
	return mtaout.SendResult{Status: mtaout.PermFail, Err: errReason("550 no such user")}
}

type errReason string

func (e errReason) Error() string { return string(e) }

// A permanent delivery failure produces a DSN delivered to the local sender.
func TestBounceDeliversDSNToSender(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	blobs, _ := blob.NewFS(filepath.Join(dir, "blobs"))
	sched := mtaout.NewScheduler(mtaout.Config{Sender: permFailSender{}, MaxPerDomain: 10})
	mgr := server.NewManager(dir, blobs, sched)
	sched.SetOnBounce(func(msg mtaout.OutMessage, reason string) { mgr.HandleBounce("vmail.test", msg, reason) })
	_ = mgr.AddAccount("alice@vmail.test", "pw")

	sched.Enqueue(mtaout.OutMessage{
		Tenant: "vmail.test", FromDomain: "vmail.test", RcptDomain: "nowhere.example",
		From: "alice@vmail.test", Rcpts: []string{"ghost@nowhere.example"},
		Raw: []byte("From: alice@vmail.test\r\nTo: ghost@nowhere.example\r\n\r\nhi\r\n"),
	})
	if st := sched.Tick(ctx, time.Now()); st.Bounced != 1 {
		t.Fatalf("expected 1 bounce, got %d", st.Bounced)
	}

	alice, _ := mgr.AuthIMAP("alice@vmail.test", "pw")
	inbox := alice.MessagesWithLabel(model.LabelInbox)
	if len(inbox) != 1 {
		t.Fatalf("alice should have a bounce DSN in inbox, got %d messages", len(inbox))
	}
	if !strings.Contains(inbox[0].Envelope.Subject, "Undelivered") {
		t.Errorf("DSN subject = %q, want an 'Undelivered' bounce", inbox[0].Envelope.Subject)
	}
	body, _ := alice.Body(ctx, inbox[0].BlobRef)
	if !strings.Contains(string(body), "ghost@nowhere.example") || !strings.Contains(string(body), "550 no such user") {
		t.Errorf("DSN body missing recipient/reason:\n%s", body)
	}
}
