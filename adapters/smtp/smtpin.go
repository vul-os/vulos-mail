// Package smtpin is the inbound MX adapter: it terminates SMTP using
// emersion/go-smtp and hands each accepted message to a delivery callback (the
// ingest pipeline). It is deliberately thin — all parsing/threading/storage
// lives behind Deliver, so the protocol edge has no knowledge of the model.
package smtpin

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"time"

	gosmtp "github.com/emersion/go-smtp"
)

// stripAuthResults removes pre-existing authentication-trace header fields from
// untrusted inbound mail so an attacker can't smuggle forged results past our
// own evaluation and downstream filters/clients (RFC 8601 §5). It drops:
//
//   - ALL Authentication-Results fields (not just our own authserv-id — a forged
//     "dmarc=pass" attributed to *another* host is just as misleading to a
//     human/filter that doesn't pin the authserv-id);
//   - Received-SPF (we re-derive SPF ourselves);
//   - ARC-Seal / ARC-Message-Signature / ARC-Authentication-Results (we don't
//     verify ARC, so a foreign chain must not be trusted or forwarded as ours).
//
// The servID parameter is retained for API stability but no longer narrows the
// Authentication-Results match. Folded continuation lines are dropped with their
// field.
func stripAuthResults(raw []byte, servID string) []byte {
	i := bytes.Index(raw, []byte("\r\n\r\n"))
	if i < 0 {
		i = bytes.Index(raw, []byte("\n\n"))
	}
	if i < 0 {
		return raw // no header/body boundary
	}
	header := string(raw[:i])
	rest := raw[i:] // blank-line separator + body, preserved verbatim

	var fields []string
	for _, ln := range strings.Split(header, "\n") {
		l := strings.TrimRight(ln, "\r")
		if l == "" {
			continue
		}
		if (l[0] == ' ' || l[0] == '\t') && len(fields) > 0 {
			fields[len(fields)-1] += "\r\n" + l // folded continuation
		} else {
			fields = append(fields, l)
		}
	}
	kept := fields[:0]
	for _, f := range fields {
		if isUntrustedAuthTrace(f) {
			continue
		}
		kept = append(kept, f)
	}
	return append([]byte(strings.Join(kept, "\r\n")), rest...)
}

// strippedAuthPrefixes are header field-name prefixes whose pre-existing values
// from untrusted inbound mail are discarded (case-insensitive).
var strippedAuthPrefixes = []string{
	"authentication-results:",
	"received-spf:",
	"arc-seal:",
	"arc-message-signature:",
	"arc-authentication-results:",
}

func isUntrustedAuthTrace(field string) bool {
	lf := strings.ToLower(field)
	for _, p := range strippedAuthPrefixes {
		if strings.HasPrefix(lf, p) {
			return true
		}
	}
	return false
}

// DeliverFunc routes one accepted message to one recipient. Returning an error
// causes the SMTP transaction to fail (the sending MX will retry).
type DeliverFunc func(ctx context.Context, rcpt string, raw []byte) error

// AuthVerdict is the outcome of inbound authentication for one message.
type AuthVerdict struct {
	// AuthResults is the Authentication-Results header value to prepend.
	AuthResults string
	// Reject is true when delivery must be refused with SMTP 550 — set when DMARC
	// fails AND the sender domain's published policy is p=reject, or when the
	// message has no evaluable From identifier. A quarantine/none policy never sets
	// this (annotate-only).
	Reject bool
	// Defer is true when authentication could not be completed for a transient
	// reason (e.g. a DMARC DNS temperror): the message is refused with a 4xx so the
	// sender retries, rather than being accepted with auth fail-open. Reject takes
	// precedence over Defer if both are set.
	Defer bool
}

// Backend implements gosmtp.Backend.
type Backend struct {
	Deliver DeliverFunc
	// Verify, if set, authenticates a received message (SPF/DKIM/DMARC) given the
	// connecting IP, HELO name, and MAIL FROM, returning an Authentication-Results
	// header value that is prepended before delivery.
	Verify func(raw []byte, ip net.IP, helo, mailFrom string) string
	// VerifyVerdict, if set, takes precedence over Verify: it returns the A-R
	// header value AND a DMARC reject decision, so a p=reject failure can be
	// refused at SMTP time (550) rather than delivered with a failing A-R line.
	VerifyVerdict func(raw []byte, ip net.IP, helo, mailFrom string) AuthVerdict
	// AuthServID is the authserv-id placed in the Authentication-Results header
	// (typically this host's name).
	AuthServID string
	// KnownRcpt, if set, reports whether a recipient is deliverable here. When it
	// returns false the MX rejects at RCPT (550 5.1.1) instead of accepting the
	// whole DATA and failing — cheaper and a smaller amplification surface.
	KnownRcpt func(rcpt string) bool
}

// NewSession starts a new SMTP session.
func (b *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &session{backend: b, deliver: b.Deliver, conn: c}, nil
}

type session struct {
	backend *Backend
	deliver DeliverFunc
	conn    *gosmtp.Conn
	from    string
	rcpts   []string
}

func (s *session) Reset()        { s.from = ""; s.rcpts = nil }
func (s *session) Logout() error { return nil }

func (s *session) Mail(from string, _ *gosmtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) helo() string {
	if s.conn == nil {
		return ""
	}
	return s.conn.Hostname()
}

func (s *session) remoteIP() net.IP {
	if s.conn == nil || s.conn.Conn() == nil {
		return nil
	}
	switch a := s.conn.Conn().RemoteAddr().(type) {
	case *net.TCPAddr:
		return a.IP
	default:
		host, _, _ := net.SplitHostPort(s.conn.Conn().RemoteAddr().String())
		return net.ParseIP(host)
	}
}

func (s *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	if s.backend != nil && s.backend.KnownRcpt != nil && !s.backend.KnownRcpt(to) {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 1, 1}, Message: "no such user here"}
	}
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	// Inbound authentication (DKIM): record results as a top header so downstream
	// (filters, clients) can see them. Prepending a header at the start of the
	// message is RFC-valid.
	if s.backend != nil && (s.backend.Verify != nil || s.backend.VerifyVerdict != nil) {
		servID := s.backend.AuthServID
		if servID == "" {
			servID = "vulos-mail"
		}
		// Strip any pre-existing Authentication-Results / Received-SPF / ARC-* from
		// untrusted inbound mail so an attacker can't smuggle a forged "dmarc=pass"
		// (or other auth) line past downstream filters.
		raw = stripAuthResults(raw, servID)
		var ar string
		if s.backend.VerifyVerdict != nil {
			v := s.backend.VerifyVerdict(raw, s.remoteIP(), s.helo(), s.from)
			ar = v.AuthResults
			if v.Reject {
				// DMARC failed and the domain publishes p=reject (or the From is
				// unevaluable): refuse delivery permanently.
				return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "message rejected by DMARC policy"}
			}
			if v.Defer {
				// Authentication could not be completed (transient, e.g. a DMARC DNS
				// temperror): defer so the sender retries rather than accepting a
				// possible spoof on a slow/failing resolver. Fail closed, not open.
				return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 7, 1}, Message: "temporary authentication failure, try again later"}
			}
		} else {
			ar = s.backend.Verify(raw, s.remoteIP(), s.helo(), s.from)
		}
		if ar != "" {
			hdr := []byte("Authentication-Results: " + servID + "; " + ar + "\r\n")
			raw = append(hdr, raw...)
		}
	}
	ctx := context.Background()
	for _, rcpt := range s.rcpts {
		if err := s.deliver(ctx, rcpt, raw); err != nil {
			return err
		}
	}
	return nil
}

// NewServer returns a configured go-smtp server. TLS/auth wiring is added in the
// deployment layer; for the MX role inbound mail is unauthenticated.
func NewServer(be *Backend, addr, domain string) *gosmtp.Server {
	s := gosmtp.NewServer(be)
	s.Addr = addr
	s.Domain = domain
	s.ReadTimeout = 60 * time.Second
	s.WriteTimeout = 60 * time.Second
	s.MaxMessageBytes = 50 * 1024 * 1024
	s.MaxRecipients = 100
	return s
}
