package imap_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	imapadapter "github.com/vul-os/vulos-mail/adapters/imap"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/model"
)

// A client idling on INBOX must receive an EXISTS when new mail arrives.
func TestIMAPIdlePush(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := account.Open(ctx, log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	be := &imapadapter.Backend{Auth: func(string, string) (*account.Runtime, error) { return rt, nil }}
	go imapadapter.NewServer(be, nil).Serve(ln)

	var latest atomic.Uint32
	c, err := imapclient.DialInsecure(ln.Addr().String(), &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(d *imapclient.UnilateralDataMailbox) {
				if d.NumMessages != nil {
					latest.Store(*d.NumMessages)
				}
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Login("bob", "pw").Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		t.Fatal(err)
	}

	idle, err := c.Idle()
	if err != nil {
		t.Fatal(err)
	}

	// Deliver a message while the client idles.
	time.Sleep(100 * time.Millisecond)
	if _, err := rt.Ingest(ctx, []byte("From: x@y\r\nTo: bob@vulos.to\r\nSubject: idle push\r\n\r\nhi\r\n"), []model.LabelID{model.LabelInbox}, nil); err != nil {
		t.Fatal(err)
	}

	// Wait for the pushed EXISTS.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && latest.Load() < 1 {
		time.Sleep(30 * time.Millisecond)
	}
	if err := idle.Close(); err != nil {
		t.Fatalf("idle close: %v", err)
	}
	_ = idle.Wait()

	if latest.Load() < 1 {
		t.Fatal("client did not receive an EXISTS push while idling")
	}
}
