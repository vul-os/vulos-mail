// Package dkim wraps emersion/go-msgauth for outbound DKIM signing and inbound
// verification. DKIM signing with a key aligned to the From domain is what makes
// the shared warm-IP model (services/mtaout) actually deliver: receivers key
// reputation on the DKIM d= domain, so each tenant builds its own reputation even
// on shared transport (see the original "few IPs" design discussion).
package dkim

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/emersion/go-msgauth/dkim"
)

// GenerateRSAKey makes a signing key and the DNS TXT record (the public half) to
// publish at <selector>._domainkey.<domain>.
func GenerateRSAKey(bits int) (*rsa.PrivateKey, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, "", err
	}
	txt, err := PublicTXT(key)
	if err != nil {
		return nil, "", err
	}
	return key, txt, nil
}

// PublicTXT returns the DKIM DNS TXT record (public key) for a private key.
func PublicTXT(key *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", err
	}
	return "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der), nil
}

// MarshalPrivateKey serializes a signing key to PEM (PKCS#1) for persistence.
func MarshalPrivateKey(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

// ParsePrivateKey parses a PEM-encoded PKCS#1 signing key.
func ParsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode(data)
	if blk == nil {
		return nil, errors.New("dkim: invalid PEM")
	}
	return x509.ParsePKCS1PrivateKey(blk.Bytes)
}

type domainKey struct {
	selector string
	signer   crypto.Signer
}

// Signer holds per-domain DKIM keys and signs outbound messages.
type Signer struct {
	keys map[string]domainKey
}

// NewSigner returns an empty signer.
func NewSigner() *Signer { return &Signer{keys: map[string]domainKey{}} }

// AddDomain registers a signing key + selector for a sending domain.
func (s *Signer) AddDomain(domain, selector string, key crypto.Signer) {
	s.keys[domain] = domainKey{selector: selector, signer: key}
}

// Has reports whether a key exists for the domain.
func (s *Signer) Has(domain string) bool { _, ok := s.keys[domain]; return ok }

// ErrNoKey is returned by Sign when no signing key is registered for the From
// domain. It is a distinct, inspectable error so callers can surface a loud
// "sending unsigned" signal (a deliverability/spam risk) instead of the previous
// behaviour of silently returning the message unsigned with a nil error.
var ErrNoKey = errors.New("dkim: no signing key for domain")

// oversignedHeaders are listed an extra time in the signed "h=" tag so that
// ADDING another instance of one of these fields downstream breaks the signature
// (RFC 6376 §5.4 oversigning). These are the headers a receiver and a human
// actually key trust/identity on, so they are the replay/spoof-via-added-header
// targets.
var oversignedHeaders = []string{"From", "Subject", "To"}

// signedHeaders is the curated set of header fields included in the signature
// (once), in addition to the oversigned ones above. Listing a field that is
// absent simply oversigns it (prevents a later addition) which is harmless.
var signedHeaders = []string{
	"Cc", "Date", "Message-ID", "Reply-To", "In-Reply-To", "References",
	"MIME-Version", "Content-Type", "Content-Transfer-Encoding",
}

// headerKeys builds the HeaderKeys list: each oversigned header twice (so an
// added duplicate fails verification), then the rest once.
func headerKeys() []string {
	keys := make([]string, 0, len(oversignedHeaders)*2+len(signedHeaders))
	for _, h := range oversignedHeaders {
		keys = append(keys, h, h) // listed twice → oversigned
	}
	keys = append(keys, signedHeaders...)
	return keys
}

// Sign prepends a DKIM-Signature for domain, oversigning From/Subject/To. If no
// key is registered for the domain it returns ErrNoKey (and a nil body) rather
// than silently returning the message unsigned — the caller MUST decide what to
// do, because an unsigned message is a spam-folder risk.
func (s *Signer) Sign(domain string, raw []byte) ([]byte, error) {
	dk, ok := s.keys[domain]
	if !ok {
		return nil, ErrNoKey
	}
	var buf bytes.Buffer
	opt := &dkim.SignOptions{
		Domain:                 domain,
		Selector:               dk.selector,
		Signer:                 dk.signer,
		Hash:                   crypto.SHA256,
		HeaderCanonicalization: dkim.CanonicalizationRelaxed,
		BodyCanonicalization:   dkim.CanonicalizationRelaxed,
		HeaderKeys:             headerKeys(),
	}
	if err := dkim.Sign(&buf, bytes.NewReader(raw), opt); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Result is one DKIM verification outcome.
type Result struct {
	Domain string
	OK     bool
	Err    error
}

// Verify checks all DKIM signatures on a message. lookupTXT resolves the public
// key (inject for tests; pass nil to use real DNS).
func Verify(raw []byte, lookupTXT func(domain string) ([]string, error)) ([]Result, error) {
	vs, err := dkim.VerifyWithOptions(bytes.NewReader(raw), &dkim.VerifyOptions{LookupTXT: lookupTXT})
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(vs))
	for _, v := range vs {
		out = append(out, Result{Domain: v.Domain, OK: v.Err == nil, Err: v.Err})
	}
	return out, nil
}

// AuthResults formats the DKIM portion of an Authentication-Results header value
// (RFC 8601), e.g. "dkim=pass header.d=vulos.to".
func AuthResults(results []Result) string {
	if len(results) == 0 {
		return "dkim=none"
	}
	parts := make([]string, 0, len(results))
	for _, r := range results {
		status := "fail"
		if r.OK {
			status = "pass"
		}
		parts = append(parts, fmt.Sprintf("dkim=%s header.d=%s", status, r.Domain))
	}
	return strings.Join(parts, "; ")
}

// Aligned reports DMARC-style identifier alignment: a passing DKIM result whose
// d= domain matches the From domain (relaxed alignment would also accept an
// organizational-domain match — a later refinement).
func Aligned(results []Result, fromDomain string) bool {
	for _, r := range results {
		if r.OK && r.Domain == fromDomain {
			return true
		}
	}
	return false
}
