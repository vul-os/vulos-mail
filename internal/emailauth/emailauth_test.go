package emailauth_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/dkim"
	"github.com/vul-os/vulos-mail/internal/emailauth"
)

// fakeDNS serves TXT records from a map and satisfies both blitiri's
// spf.DNSResolver and the dkim/dmarc LookupTXT signatures.
type fakeDNS struct{ txt map[string][]string }

func (f fakeDNS) lookupTXT(name string) ([]string, error) { return f.txt[name], nil }

func (f fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	return f.txt[name], nil
}
func (f fakeDNS) LookupMX(context.Context, string) ([]*net.MX, error)        { return nil, nil }
func (f fakeDNS) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return nil, nil }
func (f fakeDNS) LookupAddr(context.Context, string) ([]string, error)       { return nil, nil }

func TestVerifyAllPass(t *testing.T) {
	ctx := context.Background()

	// DKIM key for sender.test, sign a message From alice@sender.test.
	key, dkimTXT, err := dkim.GenerateRSAKey(1024)
	if err != nil {
		t.Fatal(err)
	}
	signer := dkim.NewSigner()
	signer.AddDomain("sender.test", "s1", key)
	raw := []byte("From: alice@sender.test\r\nTo: bob@vulos.to\r\nSubject: Hi\r\n\r\nbody\r\n")
	signed, err := signer.Sign("sender.test", raw)
	if err != nil {
		t.Fatal(err)
	}

	dns := fakeDNS{txt: map[string][]string{
		"sender.test":               {"v=spf1 ip4:203.0.113.5 -all"}, // SPF: only this IP
		"s1._domainkey.sender.test": {dkimTXT},                       // DKIM public key
		"_dmarc.sender.test":        {"v=DMARC1; p=reject"},          // DMARC policy
	}}

	a := &emailauth.Authenticator{
		SPFResolver:    dns,
		DKIMLookupTXT:  dns.lookupTXT,
		DMARCLookupTXT: dns.lookupTXT,
	}

	r := a.Verify(ctx, signed, net.ParseIP("203.0.113.5"), "mx.sender.test", "alice@sender.test")

	if r.SPF != "pass" {
		t.Errorf("SPF = %q, want pass", r.SPF)
	}
	if len(r.DKIM) != 1 || !r.DKIM[0].OK {
		t.Errorf("DKIM = %+v, want one passing", r.DKIM)
	}
	if r.DMARC != "pass" {
		t.Errorf("DMARC = %q, want pass", r.DMARC)
	}
	if r.DMARCPolicy != "reject" {
		t.Errorf("DMARC policy = %q, want reject", r.DMARCPolicy)
	}

	ar := r.AuthResults()
	for _, want := range []string{"spf=pass", "dkim=pass header.d=sender.test", "dmarc=pass header.from=sender.test"} {
		if !strings.Contains(ar, want) {
			t.Errorf("Authentication-Results %q missing %q", ar, want)
		}
	}
}

func TestVerifySPFFailFromWrongIP(t *testing.T) {
	ctx := context.Background()
	dns := fakeDNS{txt: map[string][]string{
		"sender.test":        {"v=spf1 ip4:203.0.113.5 -all"},
		"_dmarc.sender.test": {"v=DMARC1; p=quarantine"},
	}}
	a := &emailauth.Authenticator{SPFResolver: dns, DMARCLookupTXT: dns.lookupTXT}
	// Connect from a different IP, no DKIM → SPF fail, DMARC fail.
	r := a.Verify(ctx, []byte("From: alice@sender.test\r\n\r\nx\r\n"), net.ParseIP("198.51.100.9"), "x", "alice@sender.test")
	if r.SPF == "pass" {
		t.Errorf("SPF should not pass from wrong IP, got %q", r.SPF)
	}
	if r.DMARC != "fail" {
		t.Errorf("DMARC = %q, want fail (no aligned auth)", r.DMARC)
	}
}

// tempDNS returns a temporary (Temporary()==true) DNS error for DMARC lookups,
// simulating a SERVFAIL/timeout.
type tempDNSErr struct{}

func (tempDNSErr) Error() string   { return "temporary failure" }
func (tempDNSErr) Timeout() bool   { return true }
func (tempDNSErr) Temporary() bool { return true }

// TestVerifyDMARCTemperrorDefers ensures a transient DNS failure looking up the
// DMARC record yields DMARC="temperror" (so the caller DEFERS) — NOT "none"
// (which would accept a possible p=reject spoof on a slow resolver).
func TestVerifyDMARCTemperror(t *testing.T) {
	ctx := context.Background()
	a := &emailauth.Authenticator{
		DMARCLookupTXT: func(string) ([]string, error) { return nil, tempDNSErr{} },
	}
	r := a.Verify(ctx, []byte("From: alice@sender.test\r\nTo: bob@vulos.to\r\n\r\nx\r\n"), nil, "x", "")
	if r.DMARC != "temperror" {
		t.Errorf("DMARC = %q, want temperror on DNS temp failure (must defer, not accept)", r.DMARC)
	}
}

// TestVerifyMultipleFromRejected ensures a message with two From header fields is
// flagged FromMalformed (unevaluable) rather than silently downgraded to none.
func TestVerifyMultipleFromRejected(t *testing.T) {
	ctx := context.Background()
	dns := fakeDNS{txt: map[string][]string{"_dmarc.evil.test": {"v=DMARC1; p=reject"}}}
	a := &emailauth.Authenticator{DMARCLookupTXT: dns.lookupTXT}
	raw := []byte("From: real@bank.test\r\nFrom: attacker@evil.test\r\nTo: bob@vulos.to\r\n\r\nx\r\n")
	r := a.Verify(ctx, raw, nil, "x", "")
	if !r.FromMalformed {
		t.Errorf("FromMalformed = false, want true for double From header")
	}
	if r.DMARC != "fail" {
		t.Errorf("DMARC = %q, want fail for unevaluable From", r.DMARC)
	}
}

// TestVerifyMissingFrom ensures a message with no From header is flagged.
func TestVerifyMissingFrom(t *testing.T) {
	ctx := context.Background()
	a := &emailauth.Authenticator{}
	r := a.Verify(ctx, []byte("To: bob@vulos.to\r\nSubject: hi\r\n\r\nx\r\n"), nil, "x", "")
	if !r.FromMalformed {
		t.Errorf("FromMalformed = false, want true for missing From header")
	}
}
