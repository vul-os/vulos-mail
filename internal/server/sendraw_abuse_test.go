package server_test

// TestSendRawAbuseGate verifies that Manager.SendRaw enforces the outbound
// abuse filter on all programmatic send paths (JMAP, webapi, webmail) — not
// only on SMTP authenticated submission.

import (
	"context"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/abuse"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/server"
)

func TestSendRawAbuseGate(t *testing.T) {
	dir := t.TempDir()
	blobs, err := blob.NewFS(dir + "/blobs")
	if err != nil {
		t.Fatal(err)
	}
	mgr := server.NewManager(dir, blobs, nil)
	if err := mgr.AddAccount("alice@example.com", "pw"); err != nil {
		t.Fatal(err)
	}

	// Wire an abuse filter that suspends after exceeding the recipient burst
	// threshold (MaxRecipients: 1 → any send with ≥2 recipients is blocked).
	mgr.Abuse = abuse.New(abuse.Config{MaxRecipients: 1})

	raw := []byte("From: alice@example.com\r\nTo: b@r.example\r\nSubject: t\r\n\r\nhi\r\n")

	// Two recipients exceeds MaxRecipients: 1 → abuse filter should block.
	err = mgr.SendRaw(context.Background(), "alice@example.com",
		[]string{"b@r.example", "c@r.example"}, raw)
	if err == nil {
		t.Fatal("SendRaw must be blocked by the abuse filter (recipient burst)")
	}
	if !strings.Contains(err.Error(), "abuse") && !strings.Contains(err.Error(), "recipient") {
		t.Errorf("error should mention abuse or recipient; got: %v", err)
	}
}

func TestSendRawAbuseGateAllowsNormal(t *testing.T) {
	dir := t.TempDir()
	blobs, err := blob.NewFS(dir + "/blobs")
	if err != nil {
		t.Fatal(err)
	}
	mgr := server.NewManager(dir, blobs, nil)
	if err := mgr.AddAccount("alice@example.com", "pw"); err != nil {
		t.Fatal(err)
	}

	// Default-configured abuse filter (200 msgs/hour, 100 recipients max).
	mgr.Abuse = abuse.New(abuse.Config{})

	raw := []byte("From: alice@example.com\r\nTo: b@r.example\r\nSubject: t\r\n\r\nhi\r\n")

	// Single recipient well within limits — should not be blocked by abuse gate.
	// (No scheduler is wired, so the message is enqueued into a nil sched — that's
	// fine; we're only testing the gate, not delivery.)
	err = mgr.SendRaw(context.Background(), "alice@example.com",
		[]string{"b@r.example"}, raw)
	if err != nil {
		t.Errorf("normal send should not be blocked by abuse filter; got: %v", err)
	}
}
