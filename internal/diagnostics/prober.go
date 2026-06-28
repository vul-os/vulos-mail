package diagnostics

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
)

// ProberConfig configures the live SMTP→IMAP round-trip prober.
type ProberConfig struct {
	// From is the envelope sender of the probe (defaults to the test mailbox).
	From string
	// To is the probe recipient — the configured test mailbox.
	To string

	// SMTP submission endpoint + credentials.
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string

	// IMAP endpoint + credentials for the test mailbox.
	IMAPHost string
	IMAPPort string
	IMAPUser string
	IMAPPass string

	// PollInterval is how often the IMAP inbox is polled for the probe.
	PollInterval time.Duration
	// InsecureTLS skips TLS verification (for self-signed dev servers only).
	InsecureTLS bool
}

// smtpIMAPProber is the live Prober: it submits a probe over SMTP (STARTTLS) and
// polls the test mailbox over IMAPS until the probe arrives, then deletes it.
type smtpIMAPProber struct {
	cfg ProberConfig
	now func() time.Time
}

// NewSMTPIMAPProber builds the live round-trip prober. It is only useful when the
// test mailbox credentials are set; the composition root wires it (via
// [WithProber]) only then, so an unconfigured deployment reports the self-test as
// not-configured instead of failing.
func NewSMTPIMAPProber(cfg ProberConfig) Prober {
	if cfg.From == "" {
		cfg.From = cfg.To
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	return &smtpIMAPProber{cfg: cfg, now: time.Now}
}

func (p *smtpIMAPProber) Probe(ctx context.Context, token string) (time.Duration, error) {
	start := p.now()
	if err := p.send(ctx, token); err != nil {
		return 0, fmt.Errorf("submit probe: %w", err)
	}
	if err := p.awaitDelivery(ctx, token); err != nil {
		return 0, err
	}
	return p.now().Sub(start), nil
}

// send submits the probe message over SMTP submission with STARTTLS + AUTH.
func (p *smtpIMAPProber) send(ctx context.Context, token string) error {
	addr := net.JoinHostPort(p.cfg.SMTPHost, p.cfg.SMTPPort)
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	// Submission requires STARTTLS before AUTH; NewClientStartTLS does EHLO+upgrade.
	c, err := gosmtp.NewClientStartTLS(conn, &tls.Config{ServerName: p.cfg.SMTPHost, InsecureSkipVerify: p.cfg.InsecureTLS}) //nolint:gosec
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer c.Close()
	if p.cfg.SMTPUser != "" {
		if err := c.Auth(sasl.NewPlainClient("", p.cfg.SMTPUser, p.cfg.SMTPPass)); err != nil {
			return err
		}
	}
	if err := c.Mail(p.cfg.From, nil); err != nil {
		return err
	}
	if err := c.Rcpt(p.cfg.To, nil); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(probeMessage(p.cfg.From, p.cfg.To, token, p.now()))); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// awaitDelivery polls the test mailbox over IMAP until a message carrying token
// appears, then removes it. It returns nil on receipt, ctx.Err() on timeout.
func (p *smtpIMAPProber) awaitDelivery(ctx context.Context, token string) error {
	addr := net.JoinHostPort(p.cfg.IMAPHost, p.cfg.IMAPPort)
	opts := &imapclient.Options{TLSConfig: &tls.Config{ServerName: p.cfg.IMAPHost, InsecureSkipVerify: p.cfg.InsecureTLS}} //nolint:gosec

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()
	for {
		found, err := p.pollOnce(ctx, addr, opts, token)
		if err == nil && found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// pollOnce connects, searches the inbox for the token, and removes a match.
func (p *smtpIMAPProber) pollOnce(ctx context.Context, addr string, opts *imapclient.Options, token string) (bool, error) {
	c, err := imapclient.DialTLS(addr, opts)
	if err != nil {
		return false, err
	}
	defer c.Logout()
	if err := c.Login(p.cfg.IMAPUser, p.cfg.IMAPPass).Wait(); err != nil {
		return false, err
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		return false, err
	}
	data, err := c.Search(&imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: probeHeader, Value: token}},
	}, nil).Wait()
	if err != nil {
		return false, err
	}
	nums := data.AllSeqNums()
	if len(nums) == 0 {
		return false, nil
	}
	// Clean up: mark the probe(s) deleted and expunge so probes never accumulate.
	seqset := imap.SeqSet{}
	for _, n := range nums {
		seqset.AddNum(n)
	}
	_, _ = c.Store(seqset, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Collect()
	_ = c.Expunge().Close()
	return true, nil
}

// probeHeader is the custom header carrying the unique probe token, matched
// exactly by the IMAP search so only this probe is detected.
const probeHeader = "X-Vulos-Diag-Probe"

// probeMessage builds the RFC 5322 probe carrying token in a dedicated header and
// the subject.
func probeMessage(from, to, token string, now time.Time) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: vulos-mail diagnostics probe " + token + "\r\n")
	b.WriteString(probeHeader + ": " + token + "\r\n")
	b.WriteString("Date: " + now.UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString("Automated deliverability self-test. Token: " + token + "\r\n")
	return b.String()
}
