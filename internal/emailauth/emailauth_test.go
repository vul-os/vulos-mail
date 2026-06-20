package emailauth_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/vul-os/vmail/internal/dkim"
	"github.com/vul-os/vmail/internal/emailauth"
)

// fakeDNS serves TXT records from a map and satisfies both blitiri's
// spf.DNSResolver and the dkim/dmarc LookupTXT signatures.
type fakeDNS struct{ txt map[string][]string }

func (f fakeDNS) lookupTXT(name string) ([]string, error) { return f.txt[name], nil }

func (f fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	return f.txt[name], nil
}
func (f fakeDNS) LookupMX(context.Context, string) ([]*net.MX, error)      { return nil, nil }
func (f fakeDNS) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return nil, nil }
func (f fakeDNS) LookupAddr(context.Context, string) ([]string, error)     { return nil, nil }

func TestVerifyAllPass(t *testing.T) {
	ctx := context.Background()

	// DKIM key for sender.test, sign a message From alice@sender.test.
	key, dkimTXT, err := dkim.GenerateRSAKey(1024)
	if err != nil {
		t.Fatal(err)
	}
	signer := dkim.NewSigner()
	signer.AddDomain("sender.test", "s1", key)
	raw := []byte("From: alice@sender.test\r\nTo: bob@vmail.test\r\nSubject: Hi\r\n\r\nbody\r\n")
	signed, err := signer.Sign("sender.test", raw)
	if err != nil {
		t.Fatal(err)
	}

	dns := fakeDNS{txt: map[string][]string{
		"sender.test":                {"v=spf1 ip4:203.0.113.5 -all"}, // SPF: only this IP
		"s1._domainkey.sender.test":  {dkimTXT},                       // DKIM public key
		"_dmarc.sender.test":         {"v=DMARC1; p=reject"},          // DMARC policy
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
