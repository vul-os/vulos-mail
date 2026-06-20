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
	SPFResolver    spf.DNSResolver               // nil => net default
	DKIMLookupTXT  func(string) ([]string, error) // nil => net.LookupTXT
	DMARCLookupTXT func(string) ([]string, error) // nil => net.LookupTXT
}

// Result holds the authentication outcomes for one message.
type Result struct {
	SPF         string // pass/fail/softfail/neutral/none/temperror/permerror
	SPFDomain   string
	DKIM        []dkim.Result
	DMARC       string // pass/fail/none
	DMARCPolicy string // none/quarantine/reject
	FromDomain  string
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

	// From domain (DMARC is keyed on the header From, not MAIL FROM).
	if env, err := mime.ParseEnvelope(raw); err == nil && len(env.From) > 0 {
		r.FromDomain = domainOf(env.From[0])
	}

	// DMARC = SPF-or-DKIM that passes AND aligns with the From domain.
	r.DMARC, r.DMARCPolicy = a.evalDMARC(r)
	return r
}

func (a *Authenticator) evalDMARC(r Result) (result, policy string) {
	if r.FromDomain == "" {
		return "none", ""
	}
	rec, err := dmarc.LookupWithOptions(r.FromDomain, &dmarc.LookupOptions{LookupTXT: a.DMARCLookupTXT})
	if err != nil || rec == nil {
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

func domainOf(addr string) string {
	addr = strings.Trim(strings.TrimSpace(addr), "<>")
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return strings.ToLower(addr[i+1:])
	}
	return strings.ToLower(addr)
}
