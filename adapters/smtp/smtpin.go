// Package smtpin is the inbound MX adapter: it terminates SMTP using
// emersion/go-smtp and hands each accepted message to a delivery callback (the
// ingest pipeline). It is deliberately thin — all parsing/threading/storage
// lives behind Deliver, so the protocol edge has no knowledge of the model.
package smtpin

import (
	"context"
	"io"
	"time"

	gosmtp "github.com/emersion/go-smtp"
)

// DeliverFunc routes one accepted message to one recipient. Returning an error
// causes the SMTP transaction to fail (the sending MX will retry).
type DeliverFunc func(ctx context.Context, rcpt string, raw []byte) error

// Backend implements gosmtp.Backend.
type Backend struct {
	Deliver DeliverFunc
}

// NewSession starts a new SMTP session.
func (b *Backend) NewSession(_ *gosmtp.Conn) (gosmtp.Session, error) {
	return &session{deliver: b.Deliver}, nil
}

type session struct {
	deliver DeliverFunc
	from    string
	rcpts   []string
}

func (s *session) Reset()        { s.from = ""; s.rcpts = nil }
func (s *session) Logout() error { return nil }

func (s *session) Mail(from string, _ *gosmtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
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
