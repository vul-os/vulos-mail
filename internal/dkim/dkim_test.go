package dkim_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/dkim"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	key, txt, err := dkim.GenerateRSAKey(1024) // small key: fast tests
	if err != nil {
		t.Fatal(err)
	}
	s := dkim.NewSigner()
	s.AddDomain("vulos.to", "s1", key)

	raw := []byte("From: alice@vulos.to\r\nTo: bob@gmail.com\r\nSubject: Hi\r\n\r\nbody text\r\n")
	signed, err := s.Sign("vulos.to", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(signed, []byte("DKIM-Signature:")) {
		t.Fatal("signed message lacks DKIM-Signature header")
	}

	// Inject the published public key for s1._domainkey.vulos.to.
	lookup := func(domain string) ([]string, error) {
		if domain == "s1._domainkey.vulos.to" {
			return []string{txt}, nil
		}
		return nil, nil
	}
	results, err := dkim.Verify(signed, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].OK || results[0].Domain != "vulos.to" {
		t.Fatalf("verification = %+v, want one passing result for vulos.to", results)
	}
	if !dkim.Aligned(results, "vulos.to") {
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

func TestAuthResultsFormat(t *testing.T) {
	if got := dkim.AuthResults(nil); got != "dkim=none" {
		t.Errorf("empty = %q, want dkim=none", got)
	}
	got := dkim.AuthResults([]dkim.Result{{Domain: "vulos.to", OK: true}, {Domain: "evil.test", OK: false}})
	want := "dkim=pass header.d=vulos.to; dkim=fail header.d=evil.test"
	if got != want {
		t.Errorf("AuthResults = %q, want %q", got, want)
	}
}

func TestSignNoKeyReturnsErrNoKey(t *testing.T) {
	s := dkim.NewSigner()
	raw := []byte("From: x@unknown.test\r\n\r\nhi\r\n")
	out, err := s.Sign("unknown.test", raw)
	// New contract: a missing key surfaces ErrNoKey rather than silently returning
	// the message unsigned with a nil error.
	if !errors.Is(err, dkim.ErrNoKey) {
		t.Fatalf("err = %v, want ErrNoKey", err)
	}
	if out != nil {
		t.Error("no key should return a nil body, not the unsigned message")
	}
}

// TestSignOversignsFromSubjectTo verifies the signed h= tag oversigns
// From/Subject/To (each listed twice), so adding a duplicate of one of those
// headers downstream breaks verification (added-header replay defence).
func TestSignOversignsFromSubjectTo(t *testing.T) {
	key, _, err := dkim.GenerateRSAKey(1024)
	if err != nil {
		t.Fatal(err)
	}
	s := dkim.NewSigner()
	s.AddDomain("sender.test", "s1", key)
	raw := []byte("From: a@sender.test\r\nTo: b@vulos.to\r\nSubject: Hi\r\n\r\nbody\r\n")
	signed, err := s.Sign("sender.test", raw)
	if err != nil {
		t.Fatal(err)
	}
	// Unfold the DKIM-Signature header (strip CRLF + leading whitespace) so the h=
	// tag is on one logical line, then isolate its value.
	unfolded := strings.NewReplacer("\r\n", "", "\n", "", "\t", "", " ", "").Replace(string(signed))
	low := strings.ToLower(unfolded)
	hi := strings.Index(low, ";h=") // ";h=" avoids matching "bh=" (body hash)
	if hi < 0 {
		t.Fatal("no h= tag in signature")
	}
	htag := low[hi+3:]
	if end := strings.Index(htag, ";"); end >= 0 {
		htag = htag[:end]
	}
	fields := strings.Split(htag, ":")
	count := map[string]int{}
	for _, f := range fields {
		count[f]++
	}
	for _, h := range []string{"from", "subject", "to"} {
		if count[h] < 2 {
			t.Errorf("h= tag oversigns %q %d times, want >=2 (h=%s)", h, count[h], htag)
		}
	}
}
