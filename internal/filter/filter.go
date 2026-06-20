// Package filter is the inbound message-scanning pipeline — the clean
// replacement for vulos-mail's VulosIncomingFilter kernel hook. Scanners (spam,
// URL-safety, CSAM, …) run over a received message and return a verdict; the
// delivery layer routes accordingly (inbox / junk / reject). Because vulos-mail's
// receive path is an adapter over the runtime, this needs no kernel edit — the
// Manager just runs the chain before choosing a delivery label.
package filter

import "context"

// Action is the disposition for a message (ordered by severity).
type Action int

const (
	Accept Action = iota // deliver to inbox
	Junk                 // deliver to the Spam label
	Reject               // refuse at SMTP time
)

// Verdict is a scanner's decision.
type Verdict struct {
	Action Action
	Reason string
}

// Scanner inspects a raw message and returns a verdict. Implementations must be
// fast and side-effect-light; long scans cause SMTP timeouts.
type Scanner interface {
	Name() string
	Scan(ctx context.Context, raw []byte) Verdict
}

// Chain runs scanners and folds their verdicts to the most severe outcome,
// short-circuiting on the first Reject.
type Chain struct {
	scanners []Scanner
}

// NewChain builds a chain from the given scanners.
func NewChain(scanners ...Scanner) *Chain { return &Chain{scanners: scanners} }

// Add appends a scanner.
func (c *Chain) Add(s Scanner) { c.scanners = append(c.scanners, s) }

// Scan runs all scanners. Reject wins immediately; otherwise the worst of
// Accept/Junk is returned, tagged with the scanner that raised it.
func (c *Chain) Scan(ctx context.Context, raw []byte) Verdict {
	worst := Verdict{Action: Accept}
	for _, s := range c.scanners {
		v := s.Scan(ctx, raw)
		if v.Action == Reject {
			if v.Reason == "" {
				v.Reason = s.Name()
			}
			return v
		}
		if v.Action > worst.Action {
			worst = v
			if worst.Reason == "" {
				worst.Reason = s.Name()
			}
		}
	}
	return worst
}
