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

// stripAuthResults removes any existing Authentication-Results header field whose
// authserv-id matches servID (RFC 8601 §5), so untrusted inbound mail can't carry
// a forged "dmarc=pass" line attributed to us. Folded continuation lines are
// dropped with their field.
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
		if isAuthResultsFor(f, servID) {
			continue
		}
		kept = append(kept, f)
	}
	return append([]byte(strings.Join(kept, "\r\n")), rest...)
}

func isAuthResultsFor(field, servID string) bool {
	if !strings.HasPrefix(strings.ToLower(field), "authentication-results:") {
		return false
	}
	v := strings.TrimSpace(field[len("authentication-results:"):])
	id := v
	if j := strings.IndexAny(v, "; \t\r\n"); j >= 0 {
		id = v[:j]
	}
	return strings.EqualFold(strings.TrimSpace(id), servID)
}

// DeliverFunc routes one accepted message to one recipient. Returning an error
// causes the SMTP transaction to fail (the sending MX will retry).
type DeliverFunc func(ctx context.Context, rcpt string, raw []byte) error

// Backend implements gosmtp.Backend.
type Backend struct {
	Deliver DeliverFunc
	// Verify, if set, authenticates a received message (SPF/DKIM/DMARC) given the
	// connecting IP, HELO name, and MAIL FROM, returning an Authentication-Results
	// header value that is prepended before delivery.
	Verify func(raw []byte, ip net.IP, helo, mailFrom string) string
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
	if s.backend != nil && s.backend.Verify != nil {
		servID := s.backend.AuthServID
		if servID == "" {
			servID = "vulos-mail"
		}
		// RFC 8601 §5: strip any pre-existing Authentication-Results header bearing
		// our authserv-id, so an attacker can't embed a forged "dmarc=pass" line.
		raw = stripAuthResults(raw, servID)
		if ar := s.backend.Verify(raw, s.remoteIP(), s.helo(), s.from); ar != "" {
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
