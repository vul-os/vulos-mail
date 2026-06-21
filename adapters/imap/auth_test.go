package imap_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	imapadapter "github.com/vul-os/vulos-mail/adapters/imap"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/ids"
)

// SELECT before LOGIN must be refused (no pre-auth mailbox access), wrong
// passwords rejected, and a valid login then grants access.
func TestIMAPRequiresAuthBeforeSelect(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, _ := account.Open(ctx, log, store, ids.NewGen(), nil)

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	be := &imapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) {
		if u == "bob" && p == "pw" {
			return rt, nil
		}
		return nil, net.ErrClosed
	}}
	go imapadapter.NewServer(be, nil).Serve(ln)

	c, err := imapclient.DialInsecure(ln.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.Select("INBOX", nil).Wait(); err == nil {
		t.Fatal("SELECT succeeded before LOGIN — pre-auth mailbox access")
	}
	if err := c.Login("bob", "wrong").Wait(); err == nil {
		t.Fatal("LOGIN accepted a wrong password")
	}
	if err := c.Login("bob", "pw").Wait(); err != nil {
		t.Fatalf("valid login should succeed: %v", err)
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		t.Fatalf("SELECT after login should succeed: %v", err)
	}
}
