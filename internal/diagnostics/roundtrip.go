package diagnostics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Prober runs a single end-to-end mail round trip: it sends a probe message
// carrying token to the configured test mailbox via SMTP submission and blocks
// until that exact message is observed via IMAP (then removes it). It returns the
// measured send→receive latency.
//
// The default implementation ([NewSMTPIMAPProber]) makes live SMTP/IMAP
// connections; tests inject a fake. A Runner built without [WithProber] reports
// the round-trip check as not-configured rather than sending anything.
type Prober interface {
	Probe(ctx context.Context, token string) (time.Duration, error)
}

// newProbeToken returns a unique, unguessable token embedded in the probe so the
// receiver can match exactly this message (and never a stale or unrelated one).
func newProbeToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "vulos-diag-" + hex.EncodeToString(b[:])
}

// roundTripCheck runs (or explains why it skipped) the send→deliver→receive
// self-test. It is rate-limited: within RoundTripMinInterval of the last probe it
// reports warn (rate-limited) instead of sending another message.
func (r *Runner) roundTripCheck(ctx context.Context) Check {
	c := Check{ID: "roundtrip", Title: "Round-trip self-test"}

	if !r.cfg.RoundTripEnabled {
		c.Status = StatusWarn
		c.Detail = "round-trip self-test is disabled by configuration"
		c.Remediation = "set [diagnostics] roundtrip = true (and configure the test mailbox) to enable end-to-end delivery checks"
		return c
	}
	if r.prober == nil {
		c.Status = StatusWarn
		c.Detail = "round-trip self-test is enabled but no prober is configured"
		c.Remediation = "configure the test mailbox credentials so the self-test can send and read a probe"
		return c
	}

	// Rate-limit: at most one probe per RoundTripMinInterval.
	r.mu.Lock()
	now := r.now()
	if !r.lastProbe.IsZero() && now.Sub(r.lastProbe) < r.cfg.RoundTripMinInterval {
		wait := r.cfg.RoundTripMinInterval - now.Sub(r.lastProbe)
		r.mu.Unlock()
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("rate-limited: last probe was %s ago (min interval %s)", now.Sub(r.lastProbe).Round(time.Second), r.cfg.RoundTripMinInterval)
		c.Remediation = fmt.Sprintf("retry in %s, or lower [diagnostics] roundtrip_min_interval", wait.Round(time.Second))
		return c
	}
	r.lastProbe = now
	r.mu.Unlock()

	token := newProbeToken()
	pctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	var latency time.Duration
	var perr error
	c.LatencyMS = r.measure(func() {
		latency, perr = r.prober.Probe(pctx, token)
	})
	c.Value = "to=" + r.cfg.TestMailbox
	if perr != nil {
		c.Status = StatusFail
		c.Detail = fmt.Sprintf("probe to %s did not complete within %s: %v", r.cfg.TestMailbox, r.cfg.Timeout, perr)
		c.Remediation = "verify submission, delivery and IMAP all work for the test mailbox; check the queue and logs"
		return c
	}
	// Prefer the prober's measured latency (clock-injectable for tests); fall back
	// to the wall-clock measure.
	if latency > 0 {
		c.LatencyMS = latency.Milliseconds()
	}
	c.Status = StatusOK
	c.Detail = fmt.Sprintf("probe delivered and received in %s", latency.Round(time.Millisecond))
	return c
}
