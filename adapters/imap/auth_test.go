package imap_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	imapadapter "github.com/vul-os/vulos-mail/adapters/imap"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/authlimit"
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

// TestIMAPBruteForceThrottled proves repeated wrong passwords lock the account
// out (even a subsequent correct password is refused), while a fresh account
// reaching its budget with the correct password before lockout still works.
func TestIMAPBruteForceThrottled(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, _ := account.Open(ctx, log, store, ids.NewGen(), nil)

	clock := time.Unix(0, 0).UTC()
	lim := authlimit.New(authlimit.Config{MaxFailures: 3, Window: time.Hour, Lockout: 10 * time.Minute, Now: func() time.Time { return clock }})

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	be := &imapadapter.Backend{
		Auth: func(u, p string) (*account.Runtime, error) {
			if u == "bob" && p == "pw" {
				return rt, nil
			}
			return nil, net.ErrClosed
		},
		Limiter: lim,
	}
	go imapadapter.NewServer(be, nil).Serve(ln)

	// Each LOGIN command uses a fresh connection so the limiter (not protocol
	// state) is what blocks; the limiter is keyed on IP + account.
	login := func(user, pass string) error {
		c, err := imapclient.DialInsecure(ln.Addr().String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.Login(user, pass).Wait()
	}

	// 3 failures for bob → locked.
	for i := 0; i < 3; i++ {
		if err := login("bob", "wrong"); err == nil {
			t.Fatalf("attempt %d: wrong password should fail", i)
		}
	}
	// Even the correct password is now refused (locked out).
	if err := login("bob", "pw"); err == nil {
		t.Fatal("correct password should be refused after lockout")
	}
	// After the lockout window elapses, the correct password works again.
	clock = clock.Add(11 * time.Minute)
	if err := login("bob", "pw"); err != nil {
		t.Fatalf("correct password should work after lockout expires: %v", err)
	}
}
