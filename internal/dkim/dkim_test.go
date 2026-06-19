package dkim_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vul-os/vmail/internal/dkim"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	key, txt, err := dkim.GenerateRSAKey(1024) // small key: fast tests
	if err != nil {
		t.Fatal(err)
	}
	s := dkim.NewSigner()
	s.AddDomain("vmail.test", "s1", key)

	raw := []byte("From: alice@vmail.test\r\nTo: bob@gmail.com\r\nSubject: Hi\r\n\r\nbody text\r\n")
	signed, err := s.Sign("vmail.test", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(signed, []byte("DKIM-Signature:")) {
		t.Fatal("signed message lacks DKIM-Signature header")
	}

	// Inject the published public key for s1._domainkey.vmail.test.
	lookup := func(domain string) ([]string, error) {
		if domain == "s1._domainkey.vmail.test" {
			return []string{txt}, nil
		}
		return nil, nil
	}
	results, err := dkim.Verify(signed, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].OK || results[0].Domain != "vmail.test" {
		t.Fatalf("verification = %+v, want one passing result for vmail.test", results)
	}
	if !dkim.Aligned(results, "vmail.test") {
		t.Error("DKIM should be aligned with From domain")
	}

	// Tampering the body must break verification.
	tampered := bytes.Replace(signed, []byte("body text"), []byte("evil text"), 1)
	rt, err := dkim.Verify(tampered, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt) == 1 && rt[0].OK {
		t.Error("tampered message must not verify")
	}
}

func TestSignNoKeyPassesThrough(t *testing.T) {
	s := dkim.NewSigner()
	raw := []byte("From: x@unknown.test\r\n\r\nhi\r\n")
	out, err := s.Sign("unknown.test", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) || strings.Contains(string(out), "DKIM-Signature") {
		t.Error("no key should pass message through unsigned")
	}
}
