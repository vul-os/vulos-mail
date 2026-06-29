package mtaout

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

// SMTPSender is the production Sender: it resolves the recipient domain's MX and
// delivers over SMTP, binding the connection to the chosen source IP. It maps
// 4xx→TempFail and 5xx→PermFail by inspecting the SMTP error code.
//
// STARTTLS is negotiated opportunistically by default: if the remote MX advertises
// STARTTLS in its EHLO response, the connection is upgraded before any mail
// commands are issued. Set STARTTLSEnforce to require encryption — delivery fails
// (TempFail, retriable) when the remote does not offer STARTTLS.
//
// NOTE: this is integration code (requires DNS + network), exercised in
// deployment, not in unit tests — the scheduler is tested with a fake Sender.
type SMTPSender struct {
	HELO        string
	DialTimeout time.Duration
	// STARTTLSEnforce, if true, refuses to deliver to hosts that do not advertise
	// STARTTLS in their EHLO response. Default (false): opportunistic — STARTTLS is
	// negotiated when offered but delivery continues in the clear when not advertised.
	// Enable for domains whose MTA-STS or DANE policy mandates encryption.
	STARTTLSEnforce bool
}

func (s *SMTPSender) Send(ctx context.Context, msg OutMessage, sourceIP string) SendResult {
	hosts, err := mxHosts(msg.RcptDomain)
	if err != nil {
		return SendResult{Status: TempFail, Err: err}
	}
	dialer := &net.Dialer{Timeout: s.timeout()}
	if ip := net.ParseIP(sourceIP); ip != nil {
		dialer.LocalAddr = &net.TCPAddr{IP: ip}
	}

	var lastErr error
	for _, host := range hosts {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, "25"))
		if err != nil {
			lastErr = err
			continue
		}
		res := s.deliver(host, conn, msg)
		if res.Status != TempFail || res.Err == nil {
			return res
		}
		lastErr = res.Err
	}
	return SendResult{Status: TempFail, Err: lastErr}
}

func (s *SMTPSender) deliver(host string, conn net.Conn, msg OutMessage) SendResult {
	// Use the standard library SMTP client which exposes StartTLS publicly.
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return classify(err)
	}
	defer c.Close()

	helo := s.HELO
	if helo == "" {
		helo = "localhost"
	}
	if err := c.Hello(helo); err != nil {
		return classify(err)
	}

	// Opportunistic STARTTLS: upgrade the connection if the remote advertises it.
	// In enforce mode, refuse delivery when STARTTLS is absent (TempFail so the
	// scheduler retries — the operator can inspect logs and act before mail is lost).
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return classify(err)
		}
	} else if s.STARTTLSEnforce {
		return SendResult{
			Status: TempFail,
			Err:    fmt.Errorf("STARTTLS required but not advertised by %s (enforce mode)", host),
		}
	}

	if err := c.Mail(msg.From); err != nil {
		return classify(err)
	}
	for _, rcpt := range msg.Rcpts {
		if err := c.Rcpt(rcpt); err != nil {
			return classify(err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return classify(err)
	}
	if _, err := w.Write(msg.Raw); err != nil {
		return classify(err)
	}
	if err := w.Close(); err != nil {
		return classify(err)
	}
	_ = c.Quit()
	return SendResult{Status: Delivered}
}

func (s *SMTPSender) timeout() time.Duration {
	if s.DialTimeout > 0 {
		return s.DialTimeout
	}
	return 30 * time.Second
}

// classify maps an SMTP error to Temp/Perm based on its response code.
// Errors from net/smtp SMTP transactions arrive as *textproto.Error (4xx/5xx).
// Unknown / network errors are treated as TempFail so the scheduler retries.
func classify(err error) SendResult {
	if err == nil {
		return SendResult{Status: Delivered}
	}
	var te *textproto.Error
	if errors.As(err, &te) {
		if te.Code >= 500 {
			return SendResult{Status: PermFail, Err: err}
		}
		return SendResult{Status: TempFail, Err: err}
	}
	return SendResult{Status: TempFail, Err: err} // network/unknown: retry
}

func mxHosts(domain string) ([]string, error) {
	mxs, err := net.LookupMX(domain)
	if err != nil {
		// Distinguish "no such domain / no MX records" (fall back to A/AAAA per
		// RFC 5321 §5.1) from a transient resolver failure (SERVFAIL/timeout),
		// which must TempFail so delivery retries instead of mis-routing to the
		// bare domain.
		var de *net.DNSError
		if errors.As(err, &de) && de.IsNotFound {
			return []string{domain}, nil
		}
		return nil, err
	}
	if len(mxs) == 0 {
		return []string{domain}, nil
	}
	sort.Slice(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	hosts := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		hosts = append(hosts, strings.TrimSuffix(mx.Host, "."))
	}
	return hosts, nil
}
