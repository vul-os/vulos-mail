package account_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	smtpin "github.com/vul-os/vmail/adapters/smtp"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/model"
)

// Receive path end-to-end: SMTP adapter -> Deliver -> runtime.Ingest -> query.
// External test package (account_test) so it may import the smtp adapter, which
// itself imports account — avoiding the internal-test import cycle.
func TestReceivePathThroughSMTPAdapter(t *testing.T) {
	ctx := context.Background()
	store, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := account.Open(ctx, log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}

	be := &smtpin.Backend{
		Deliver: func(ctx context.Context, _ string, raw []byte) error {
			_, err := rt.Ingest(ctx, raw, []model.LabelID{model.LabelInbox}, nil)
			return err
		},
	}
	sess, _ := be.NewSession(nil)
	_ = sess.Mail("ext@out.example", nil)
	_ = sess.Rcpt("bob@vmail.test", nil)
	raw := []byte("From: ext@out.example\r\nTo: bob@vmail.test\r\nSubject: Hello over SMTP\r\n\r\nthe body keyword zebra\r\n")
	if err := sess.Data(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	if inbox := rt.MessagesWithLabel(model.LabelInbox); len(inbox) != 1 {
		t.Fatalf("inbox should have 1 message after SMTP delivery, got %d", len(inbox))
	}
	hits, err := rt.Search(ctx, "zebra")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("body search 'zebra' should find the delivered message, got %d", len(hits))
	}
}
