package smtpin

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/dkim"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/services/mtaout"
)

// SubmitBackend is the authenticated submission (MSA) backend on ports 587/465.
// On DATA it stores a Sent copy in the sender's account and hands one OutMessage
// per destination domain to the outbound scheduler. This is the send half of the
// mail loop (the MX Backend is the receive half).
type SubmitBackend struct {
	// Auth resolves credentials to the sender's runtime and tenant id.
	Auth func(username, password string) (rt *account.Runtime, tenant string, err error)
	// Enqueue hands a ready message to the mtaout scheduler.
	Enqueue func(mtaout.OutMessage)
	// Class tags submitted mail (defaults to Transactional).
	Class mtaout.TrafficClass
	// Signer, if set, DKIM-signs each message with the From domain's key before
	// it is stored and sent (no key for the domain → sent unsigned).
	Signer *dkim.Signer
}

func (b *SubmitBackend) NewSession(_ *gosmtp.Conn) (gosmtp.Session, error) {
	return &submitSession{backend: b}, nil
}

// NewSubmitServer builds an MSA server. AllowInsecureAuth is enabled for local/
// test use; in production this listens on implicit-TLS (465) or STARTTLS (587).
func NewSubmitServer(be *SubmitBackend, addr, domain string) *gosmtp.Server {
	s := gosmtp.NewServer(be)
	s.Addr = addr
	s.Domain = domain
	s.AllowInsecureAuth = true
	s.ReadTimeout = 60 * time.Second
	s.WriteTimeout = 60 * time.Second
	s.MaxMessageBytes = 50 * 1024 * 1024
	s.MaxRecipients = 100
	return s
}

type submitSession struct {
	backend *SubmitBackend
	rt      *account.Runtime
	tenant  string
	from    string
	rcpts   []string
}

func (s *submitSession) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *submitSession) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, username, password string) error {
		rt, tenant, err := s.backend.Auth(username, password)
		if err != nil {
			return gosmtp.ErrAuthFailed
		}
		s.rt = rt
		s.tenant = tenant
		return nil
	}), nil
}

func (s *submitSession) Reset()        { s.from = ""; s.rcpts = nil }
func (s *submitSession) Logout() error { return nil }

func (s *submitSession) Mail(from string, _ *gosmtp.MailOptions) error {
	if s.rt == nil {
		return gosmtp.ErrAuthRequired
	}
	s.from = from
	return nil
}

func (s *submitSession) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *submitSession) Data(r io.Reader) error {
	if s.rt == nil {
		return gosmtp.ErrAuthRequired
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// DKIM-sign with the From domain's key (aligned signing) before storing/sending.
	if s.backend.Signer != nil {
		if signed, err := s.backend.Signer.Sign(domainOf(s.from), raw); err == nil {
			raw = signed
		}
	}

	// Store the sender's own copy (Sent, already read).
	if _, err := s.rt.Ingest(ctx, raw, []model.LabelID{model.LabelSent}, []model.Flag{model.FlagSeen}); err != nil {
		return err
	}

	// One OutMessage per destination domain, for per-destination scheduling.
	byDomain := map[string][]string{}
	for _, rcpt := range s.rcpts {
		d := domainOf(rcpt)
		byDomain[d] = append(byDomain[d], rcpt)
	}
	for d, rcpts := range byDomain {
		s.backend.Enqueue(mtaout.OutMessage{
			Tenant:     s.tenant,
			FromDomain: domainOf(s.from),
			RcptDomain: d,
			From:       s.from,
			Rcpts:      rcpts,
			Raw:        raw,
			Class:      s.backend.Class,
		})
	}
	return nil
}

func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}
