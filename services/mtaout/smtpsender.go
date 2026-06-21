package mtaout

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"time"

	gosmtp "github.com/emersion/go-smtp"
)

// SMTPSender is the production Sender: it resolves the recipient domain's MX and
// delivers over SMTP, binding the connection to the chosen source IP. It maps
// 4xx→TempFail and 5xx→PermFail by inspecting the SMTP error code.
//
// NOTE: this is integration code (requires DNS + network), exercised in
// deployment, not in unit tests — the scheduler is tested with a fake Sender.
type SMTPSender struct {
	HELO        string
	DialTimeout time.Duration
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
		res := s.deliver(conn, msg)
		if res.Status != TempFail || res.Err == nil {
			return res
		}
		lastErr = res.Err
	}
	return SendResult{Status: TempFail, Err: lastErr}
}

func (s *SMTPSender) deliver(conn net.Conn, msg OutMessage) SendResult {
	c := gosmtp.NewClient(conn)
	defer c.Close()

	helo := s.HELO
	if helo == "" {
		helo = "localhost"
	}
	if err := c.Hello(helo); err != nil {
		return classify(err)
	}
	if err := c.Mail(msg.From, nil); err != nil {
		return classify(err)
	}
	for _, rcpt := range msg.Rcpts {
		if err := c.Rcpt(rcpt, nil); err != nil {
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

// classify maps an SMTP error to Temp/Perm based on its 5xx vs 4xx code.
func classify(err error) SendResult {
	if err == nil {
		return SendResult{Status: Delivered}
	}
	if se, ok := err.(*gosmtp.SMTPError); ok {
		if se.Code >= 500 {
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
