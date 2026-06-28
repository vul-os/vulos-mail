package diagnostics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	gosmtp "github.com/emersion/go-smtp"
)

// TLSMode selects how a TLS connection is reached for a probe.
type TLSMode int

const (
	// TLSImplicit dials TLS directly (e.g. IMAPS on 993, SMTPS on 465).
	TLSImplicit TLSMode = iota
	// TLSStartTLSSMTP connects in cleartext, issues EHLO + STARTTLS, then upgrades.
	TLSStartTLSSMTP
)

// TLSDialer probes a TLS endpoint and returns the negotiated connection state so
// the certificate can be inspected. The default ([defaultTLSDialer]) makes a live
// connection; tests inject a fake that returns a synthesised state.
//
// The dialer does NOT verify the certificate itself — it captures the presented
// chain (dialing with verification disabled) so the check can report a precise
// reason (expired, hostname mismatch, untrusted chain) rather than an opaque
// handshake error.
type TLSDialer interface {
	ProbeTLS(ctx context.Context, mode TLSMode, host, port string) (*tls.ConnectionState, error)
}

// defaultTLSDialer is the live TLS prober.
type liveTLSDialer struct{}

var defaultTLSDialer TLSDialer = liveTLSDialer{}

func (liveTLSDialer) ProbeTLS(ctx context.Context, mode TLSMode, host, port string) (*tls.ConnectionState, error) {
	addr := net.JoinHostPort(host, port)
	d := &net.Dialer{}
	tcp, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = tcp.SetDeadline(dl)
	}
	// Capture the chain even when it would not verify (self-signed/expired), so the
	// check can describe the actual problem.
	tlsCfg := &tls.Config{ServerName: host, InsecureSkipVerify: true} //nolint:gosec // verification is done explicitly in evalCert

	switch mode {
	case TLSStartTLSSMTP:
		// NewClientStartTLS issues EHLO + STARTTLS and upgrades in one step; it
		// errors if the server doesn't advertise STARTTLS.
		c, err := gosmtp.NewClientStartTLS(tcp, tlsCfg)
		if err != nil {
			_ = tcp.Close()
			return nil, err
		}
		defer c.Close()
		state, ok := c.TLSConnectionState()
		if !ok {
			return nil, fmt.Errorf("no TLS state after STARTTLS")
		}
		return &state, nil
	default: // TLSImplicit
		conn := tls.Client(tcp, tlsCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = tcp.Close()
			return nil, err
		}
		defer conn.Close()
		state := conn.ConnectionState()
		return &state, nil
	}
}

// tlsChecks probes the SMTP submission STARTTLS endpoint and the IMAPS endpoint.
func (r *Runner) tlsChecks(ctx context.Context) []Check {
	return []Check{
		r.tlsCheck(ctx, "tls.smtp", "SMTP STARTTLS", TLSStartTLSSMTP, r.cfg.SMTPHost, r.cfg.SMTPPort),
		r.tlsCheck(ctx, "tls.imap", "IMAP TLS", TLSImplicit, r.cfg.IMAPHost, r.cfg.IMAPPort),
	}
}

func (r *Runner) tlsCheck(ctx context.Context, id, title string, mode TLSMode, host, port string) Check {
	c := Check{ID: id, Title: title}
	c.LatencyMS = r.measure(func() {
		cctx, cancel := r.lookupCtx(ctx)
		defer cancel()
		state, err := r.dialTLS.ProbeTLS(cctx, mode, host, port)
		if err != nil {
			c.Status = StatusFail
			c.Detail = fmt.Sprintf("could not establish TLS to %s: %v", net.JoinHostPort(host, port), err)
			c.Remediation = "ensure the endpoint is reachable and offers TLS (STARTTLS for submission, implicit TLS for IMAPS)"
			return
		}
		st, detail, rem, val := r.evalCert(state, host)
		c.Status, c.Detail, c.Remediation, c.Value = st, detail, rem, val
	})
	return c
}

// evalCert inspects the negotiated chain for the given host and classifies it.
func (r *Runner) evalCert(state *tls.ConnectionState, host string) (status Status, detail, remediation, value string) {
	if state == nil || len(state.PeerCertificates) == 0 {
		return StatusFail, "no certificate presented", "configure a valid TLS certificate", ""
	}
	leaf := state.PeerCertificates[0]
	now := r.now()
	value = fmt.Sprintf("CN=%s; expires %s", leaf.Subject.CommonName, leaf.NotAfter.UTC().Format(time.RFC3339))

	if now.Before(leaf.NotBefore) {
		return StatusFail, "certificate is not yet valid (NotBefore " + leaf.NotBefore.UTC().Format(time.RFC3339) + ")",
			"install a currently-valid certificate", value
	}
	if now.After(leaf.NotAfter) {
		return StatusFail, "certificate expired on " + leaf.NotAfter.UTC().Format(time.RFC3339),
			"renew the TLS certificate (ACME/Let's Encrypt auto-renews)", value
	}
	if err := leaf.VerifyHostname(host); err != nil {
		return StatusFail, fmt.Sprintf("certificate is not valid for %s: %v", host, err),
			"issue a certificate whose SAN includes " + host, value
	}

	// Chain verification against the system roots (or an injected pool, for tests).
	// A failure here, with time and hostname already good, usually means a
	// self-signed or privately-rooted cert — fine for a closed/dev deployment but a
	// warn for public deliverability.
	roots := r.rootCAs
	if roots == nil {
		roots, _ = x509.SystemCertPool()
	}
	inter := x509.NewCertPool()
	for _, ic := range state.PeerCertificates[1:] {
		inter.AddCert(ic)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: inter}); err != nil {
		return StatusWarn, fmt.Sprintf("certificate chain is not trusted by system roots (self-signed?): %v", err),
			"use a publicly-trusted certificate (e.g. via ACME) for external deliverability", value
	}

	if remaining := leaf.NotAfter.Sub(now); remaining < 14*24*time.Hour {
		return StatusWarn, fmt.Sprintf("certificate expires in %d day(s)", int(remaining.Hours()/24)),
			"renew the certificate soon (ACME/Let's Encrypt auto-renews)", value
	}
	return StatusOK, "valid certificate, trusted chain", "", value
}
