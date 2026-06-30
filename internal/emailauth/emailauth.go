// Package emailauth performs inbound sender authentication — SPF, DKIM, and
// DMARC — and renders a unified Authentication-Results header (RFC 8601). It
// ports vulos-mail's inbound auth: SPF via blitiri.com.ar/go/spf, DKIM via our
// internal/dkim, and DMARC via go-msgauth/dmarc with relaxed alignment. ARC
// sealing/verification is deferred (go-msgauth has no ARC; tracked in backlog).
//
// All three DNS lookups are injectable, so the whole evaluation is unit-tested
// offline.
package emailauth

import (
	"context"
	"fmt"
	"net"
	"strings"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-msgauth/dmarc"

	"github.com/vul-os/vulos-mail/internal/dkim"
	"github.com/vul-os/vulos-mail/internal/mime"
)

// Authenticator evaluates inbound sender authentication. Zero value uses real
// DNS; inject the lookups/resolver for tests.
type Authenticator struct {
	SPFResolver    spf.DNSResolver                // nil => net default
	DKIMLookupTXT  func(string) ([]string, error) // nil => net.LookupTXT
	DMARCLookupTXT func(string) ([]string, error) // nil => net.LookupTXT
}

// Result holds the authentication outcomes for one message.
type Result struct {
	SPF         string // pass/fail/softfail/neutral/none/temperror/permerror
	SPFDomain   string
	DKIM        []dkim.Result
	DMARC       string // pass/fail/none/temperror
	DMARCPolicy string // none/quarantine/reject
	FromDomain  string
	// FromMalformed is set when the message has no usable RFC5322.From identifier:
	// zero From header fields, more than one From header field, or an unparseable
	// From. DMARC cannot be evaluated against such a message and it is a known
	// spoofing vector, so the caller refuses it rather than silently treating it
	// as DMARC "none" (= accept).
	FromMalformed bool
}

// Verify runs SPF (needs the connecting ip + MAIL FROM), DKIM (over the message),
// and DMARC (record lookup + alignment over the From domain).
func (a *Authenticator) Verify(ctx context.Context, raw []byte, ip net.IP, helo, mailFrom string) Result {
	var r Result

	// SPF.
	if ip != nil && mailFrom != "" {
		opts := []spf.Option{spf.WithContext(ctx)}
		if a.SPFResolver != nil {
			opts = append(opts, spf.WithResolver(a.SPFResolver))
		}
		res, _ := spf.CheckHostWithSender(ip, helo, mailFrom, opts...)
		r.SPF = strings.ToLower(string(res))
		r.SPFDomain = domainOf(mailFrom)
	} else {
		r.SPF = "none"
	}

	// DKIM.
	r.DKIM, _ = dkim.Verify(raw, a.DKIMLookupTXT)

	// From domain (DMARC is keyed on the header From, not MAIL FROM). DMARC
	// requires EXACTLY ONE From header field with a single, parseable identifier;
	// zero, multiple, or garbled From fields are unevaluable (and a spoof vector),
	// so flag them rather than silently downgrading DMARC to "none" (= accept).
	switch n := headerFieldCount(raw, "From"); {
	case n != 1:
		r.FromMalformed = true
	default:
		env, err := mime.ParseEnvelope(raw)
		if err != nil || len(env.From) != 1 || domainOf(env.From[0]) == "" {
			r.FromMalformed = true
		} else {
			r.FromDomain = domainOf(env.From[0])
		}
	}

	// DMARC = SPF-or-DKIM that passes AND aligns with the From domain.
	r.DMARC, r.DMARCPolicy = a.evalDMARC(r)
	return r
}

func (a *Authenticator) evalDMARC(r Result) (result, policy string) {
	if r.FromMalformed || r.FromDomain == "" {
		// Unevaluable From — treat as a failure with no resolvable policy. The
		// caller refuses the message (it cannot be made DMARC-safe).
		return "fail", ""
	}
	rec, err := dmarc.LookupWithOptions(r.FromDomain, &dmarc.LookupOptions{LookupTXT: a.DMARCLookupTXT})
	if err != nil {
		// A temporary DNS failure (SERVFAIL/timeout) must NOT be read as "no
		// policy" — a p=reject domain would then have its spoof accepted on a slow
		// resolver. Signal temperror so the caller defers (4xx) and the sender
		// retries once DNS recovers.
		if dmarc.IsTempFail(err) {
			return "temperror", ""
		}
		// ErrNoPolicy / a malformed record: the domain genuinely publishes no
		// usable DMARC policy, so DMARC is "none" (annotate-only).
		return "none", ""
	}
	if rec == nil {
		return "none", ""
	}
	policy = string(rec.Policy)

	spfAligned := r.SPF == "pass" && aligned(r.SPFDomain, r.FromDomain)
	dkimAligned := false
	for _, d := range r.DKIM {
		if d.OK && aligned(d.Domain, r.FromDomain) {
			dkimAligned = true
			break
		}
	}
	if spfAligned || dkimAligned {
		return "pass", policy
	}
	return "fail", policy
}

// AuthResults renders the full Authentication-Results value for these results.
func (r Result) AuthResults() string {
	parts := make([]string, 0, 3)
	if r.SPF != "" {
		parts = append(parts, fmt.Sprintf("spf=%s smtp.mailfrom=%s", r.SPF, r.SPFDomain))
	}
	parts = append(parts, dkim.AuthResults(r.DKIM))
	if r.DMARC != "" {
		parts = append(parts, fmt.Sprintf("dmarc=%s header.from=%s", r.DMARC, r.FromDomain))
	}
	return strings.Join(parts, "; ")
}

// aligned implements DMARC relaxed alignment: equal, or one is an
// organizational-domain suffix of the other.
func aligned(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSuffix(a, ".")), strings.ToLower(strings.TrimSuffix(b, "."))
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

// headerFieldCount counts how many times a header field appears in the message
// header block (case-insensitive, unfolded). Continuation (folded) lines and the
// body are ignored. Used to detect the zero/multiple-From case that DMARC must
// treat as unevaluable rather than as a single From.
func headerFieldCount(raw []byte, field string) int {
	// Header block ends at the first blank line (CRLF CRLF or LF LF).
	header := raw
	if i := bytesIndexHeaderEnd(raw); i >= 0 {
		header = raw[:i]
	}
	prefix := strings.ToLower(field) + ":"
	count := 0
	for _, ln := range strings.Split(string(header), "\n") {
		if ln == "" {
			continue
		}
		// A leading space/tab is a folded continuation of the previous field.
		if ln[0] == ' ' || ln[0] == '\t' {
			continue
		}
		if strings.HasPrefix(strings.ToLower(ln), prefix) {
			count++
		}
	}
	return count
}

// bytesIndexHeaderEnd returns the offset of the header/body separator, or -1.
func bytesIndexHeaderEnd(raw []byte) int {
	for i := 0; i+1 < len(raw); i++ {
		if raw[i] == '\n' && (raw[i+1] == '\n') {
			return i
		}
		if raw[i] == '\n' && i+2 < len(raw) && raw[i+1] == '\r' && raw[i+2] == '\n' {
			return i
		}
	}
	return -1
}

func domainOf(addr string) string {
	addr = strings.Trim(strings.TrimSpace(addr), "<>")
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		addr = addr[i+1:]
	}
	// Strip CR/LF/control chars, spaces, and ';' so a crafted domain can never
	// break out of (or forge structure in) the Authentication-Results header it
	// is interpolated into.
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == ' ' || r == ';' {
			return -1
		}
		return r
	}, strings.ToLower(addr))
}
