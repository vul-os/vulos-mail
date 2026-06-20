package mailsettings_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/mailsettings"
)

func TestResponderRateLimits(t *testing.T) {
	clock := time.Unix(0, 0).UTC()
	r := mailsettings.NewResponder(24*time.Hour, func() time.Time { return clock })

	if !r.ShouldReply("alice@x", "bob@y") {
		t.Fatal("first reply should be allowed")
	}
	if r.ShouldReply("alice@x", "bob@y") {
		t.Error("second reply within period should be suppressed")
	}
	// Different sender is independent.
	if !r.ShouldReply("alice@x", "carol@y") {
		t.Error("different sender should be allowed")
	}
	// After the period, reply again.
	clock = clock.Add(25 * time.Hour)
	if !r.ShouldReply("alice@x", "bob@y") {
		t.Error("after period should allow again")
	}
}

func TestIsAutomated(t *testing.T) {
	cases := map[string]bool{
		"From: a@x\r\nAuto-Submitted: auto-replied\r\n\r\nbody": true,
		"From: a@x\r\nList-Id: <l.x>\r\n\r\nbody":               true,
		"From: a@x\r\nPrecedence: bulk\r\n\r\nbody":             true,
		"From: a@x\r\nSubject: hi\r\n\r\nbody":                  false,
		"From: a@x\r\nAuto-Submitted: no\r\n\r\nbody":           false,
	}
	for raw, want := range cases {
		if got := mailsettings.IsAutomated([]byte(raw)); got != want {
			t.Errorf("IsAutomated(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestIsDaemonAddress(t *testing.T) {
	for _, a := range []string{"mailer-daemon@x.com", "postmaster@x.com", "no-reply@x.com"} {
		if !mailsettings.IsDaemonAddress(a) {
			t.Errorf("%s should be a daemon address", a)
		}
	}
	if mailsettings.IsDaemonAddress("alice@x.com") {
		t.Error("alice is not a daemon address")
	}
}

func TestBuildReply(t *testing.T) {
	out := string(mailsettings.BuildReply("alice@x", "bob@y", "OOO", "back monday"))
	for _, want := range []string{"From: alice@x", "To: bob@y", "Subject: OOO", "Auto-Submitted: auto-replied", "back monday"} {
		if !strings.Contains(out, want) {
			t.Errorf("reply missing %q:\n%s", want, out)
		}
	}
}

func TestStore(t *testing.T) {
	s := mailsettings.NewStore()
	s.Set("Alice@X.com", mailsettings.Settings{Signature: "-- Alice", Vacation: mailsettings.Vacation{Enabled: true}})
	got := s.Get("alice@x.com") // case-insensitive
	if got.Signature != "-- Alice" || !got.Vacation.Enabled {
		t.Errorf("settings round-trip failed: %+v", got)
	}
}
