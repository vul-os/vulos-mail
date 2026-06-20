package smtpin

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"github.com/vul-os/vulos-mail/internal/abuse"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/dkim"
	"github.com/vul-os/vulos-mail/internal/mime"
	"github.com/vul-os/vulos-mail/internal/model"
	"github.com/vul-os/vulos-mail/services/mtaout"
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
	// Abuse, if set, enforces per-account outbound limits (rate + recipient
	// burst → throttle/block/suspend) before a message is enqueued.
	Abuse *abuse.Filter
	// Quota, if set, enforces per-tenant daily send limits (over quota → 452).
	Quota func(account string, msgBytes int) error
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
	account string
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
		s.account = username
		return nil
	}), nil
}

func (s *submitSession) Reset()        { s.from = ""; s.rcpts = nil }
func (s *submitSession) Logout() error { return nil }

func (s *submitSession) Mail(from string, _ *gosmtp.MailOptions) error {
	if s.rt == nil {
		return gosmtp.ErrAuthRequired
	}
	// Bind the envelope sender to the authenticated account: an authenticated user
	// may only send as themselves (prevents spoofing other users/domains, incl.
	// DKIM-aligned same-domain spoofing). A null sender (<>) is allowed; the
	// visible From header is still bound in Data.
	if from != "" && !strings.EqualFold(from, s.account) {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "sender address not allowed for this account"}
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

	// Bind the visible From: header to the authenticated account. Recipients and
	// DMARC alignment key off this header, so without the check an authenticated
	// user could emit a DKIM-aligned message "From" any address.
	env, perr := mime.ParseEnvelope(raw)
	if perr != nil || len(env.From) == 0 || !strings.EqualFold(env.From[0], s.account) {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "From header must match the authenticated account"}
	}
	// Effective sender: the bound account (envelope may be a null sender).
	efFrom := s.from
	if efFrom == "" {
		efFrom = s.account
	}

	// Outbound abuse gate (reject-only): protect the shared relay reputation.
	if s.backend.Abuse != nil {
		switch action, reason := s.backend.Abuse.Check(s.account, len(s.rcpts)); action {
		case abuse.Throttle:
			return &gosmtp.SMTPError{Code: 451, Message: "submission throttled: " + reason}
		case abuse.Block:
			return &gosmtp.SMTPError{Code: 554, Message: "submission rejected: " + reason}
		case abuse.Suspend:
			return &gosmtp.SMTPError{Code: 554, Message: "account suspended: " + reason}
		}
	}

	// Per-tenant daily quota.
	if s.backend.Quota != nil {
		if err := s.backend.Quota(s.account, len(raw)); err != nil {
			return &gosmtp.SMTPError{Code: 452, Message: "over quota: " + err.Error()}
		}
	}

	// DKIM-sign with the From domain's key (aligned signing) before storing/sending.
	if s.backend.Signer != nil {
		if signed, err := s.backend.Signer.Sign(domainOf(efFrom), raw); err == nil {
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
			FromDomain: domainOf(efFrom),
			RcptDomain: d,
			From:       efFrom,
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
