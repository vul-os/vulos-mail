package server_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/server"
)

func newMgr(t *testing.T) *server.Manager {
	t.Helper()
	dir := t.TempDir()
	blobs, err := blob.NewFS(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	return server.NewManager(dir, blobs, nil)
}

// TestWrongCredentialsRejected verifies every auth entry point rejects a wrong
// password, a wrong (unknown) account, and an empty password — there is no
// bypass via missing/empty fields.
func TestWrongCredentialsRejected(t *testing.T) {
	mgr := newMgr(t)
	if err := mgr.AddAccount("alice@vulos.to", "correct-horse"); err != nil {
		t.Fatal(err)
	}

	bad := []struct{ user, pass string }{
		{"alice@vulos.to", "wrong"},
		{"alice@vulos.to", ""},
		{"alice@vulos.to", "correct-horse "}, // trailing space != password
		{"nobody@vulos.to", "correct-horse"},
		{"", "correct-horse"},
		{"", ""},
		{"ALICE@vulos.to", "wrong"},
	}
	for _, c := range bad {
		if _, err := mgr.AuthIMAP(c.user, c.pass); err == nil {
			t.Errorf("AuthIMAP(%q,%q): expected rejection, got success", c.user, c.pass)
		}
		if _, _, err := mgr.AuthSubmit(c.user, c.pass); err == nil {
			t.Errorf("AuthSubmit(%q,%q): expected rejection, got success", c.user, c.pass)
		}
	}

	// Correct credentials succeed (case-insensitive on the address).
	if _, err := mgr.AuthIMAP("Alice@Vulos.To", "correct-horse"); err != nil {
		t.Errorf("correct credentials should authenticate: %v", err)
	}
}

// TestCrossAccountMessageIsolation is the core authorization regression: the
// runtime returned for account B must never expose account A's messages. The
// webmail attachment endpoint and JMAP Email/get both gate on rt.Message(id)
// against the *authenticated* account's runtime, so a leaked/guessed message id
// from another account must resolve to "not found".
func TestCrossAccountMessageIsolation(t *testing.T) {
	ctx := context.Background()
	mgr := newMgr(t)
	mgr.AddAccount("alice@vulos.to", "pw-a")
	mgr.AddAccount("bob@vulos.to", "pw-b")

	// Deliver a message to alice only.
	raw := []byte("From: x@out.example\r\nTo: alice@vulos.to\r\nSubject: secret\r\n\r\ntop secret\r\n")
	if err := mgr.Deliver(ctx, "alice@vulos.to", raw); err != nil {
		t.Fatal(err)
	}

	alice, err := mgr.AuthIMAP("alice@vulos.to", "pw-a")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := mgr.AuthIMAP("bob@vulos.to", "pw-b")
	if err != nil {
		t.Fatal(err)
	}

	// Find alice's message id.
	all := alice.AllMail()
	if len(all) != 1 {
		t.Fatalf("alice should have 1 message, has %d", len(all))
	}
	aliceMsgID := all[0].ID

	// Alice can fetch her own message and its body.
	if _, ok := alice.Message(aliceMsgID); !ok {
		t.Fatal("alice cannot read her own message")
	}

	// Bob, using alice's message id, must get nothing — no cross-account read.
	if m, ok := bob.Message(aliceMsgID); ok {
		t.Fatalf("CROSS-ACCOUNT READ: bob fetched alice's message id %q: %+v", aliceMsgID, m)
	}
	if len(bob.AllMail()) != 0 {
		t.Fatal("CROSS-ACCOUNT READ: bob's mailbox is not empty")
	}

	// And the runtimes must be distinct objects bound to distinct accounts.
	if alice == bob {
		t.Fatal("alice and bob share a runtime")
	}
}

// TestPushTokenScopedAndOpaque verifies push tokens are account-scoped, not
// guessable across accounts, and a random token does not resolve.
func TestPushTokenScopedAndOpaque(t *testing.T) {
	mgr := newMgr(t)
	mgr.AddAccount("alice@vulos.to", "pw")
	mgr.AddAccount("bob@vulos.to", "pw")

	ta := mgr.PushToken("alice@vulos.to")
	tb := mgr.PushToken("bob@vulos.to")
	if ta == tb {
		t.Fatal("push tokens collide across accounts")
	}
	if len(ta) < 32 {
		t.Fatalf("push token too short to be unguessable: %q", ta)
	}
	if acct, ok := mgr.AccountForToken(ta); !ok || acct != "alice@vulos.to" {
		t.Fatalf("alice token resolved to %q (ok=%v)", acct, ok)
	}
	if acct, ok := mgr.AccountForToken(tb); !ok || acct != "bob@vulos.to" {
		t.Fatalf("bob token resolved to %q (ok=%v)", acct, ok)
	}
	if _, ok := mgr.AccountForToken("deadbeefdeadbeefdeadbeefdeadbeef"); ok {
		t.Fatal("a fabricated token resolved to an account")
	}
}
