package diagnostics

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// dnsblChecks queries each configured DNS blocklist (DNSBL) zone for every
// sending IP. A listing is a deliverability emergency, so a hit is a fail.
func (r *Runner) dnsblChecks(ctx context.Context) []Check {
	ips := r.sendingIPs(ctx)
	if len(ips) == 0 {
		return []Check{{
			ID:          "dnsbl",
			Title:       "DNSBL / blocklist",
			Status:      StatusWarn,
			Detail:      "no sending IP configured — cannot check blocklists",
			Remediation: "set [diagnostics] sending_ips (or enable auto_detect_ip) so blocklists can be checked",
		}}
	}
	out := make([]Check, 0, len(r.cfg.DNSBLs))
	for _, zone := range r.cfg.DNSBLs {
		out = append(out, r.dnsblZoneCheck(ctx, zone, ips))
	}
	return out
}

func (r *Runner) dnsblZoneCheck(ctx context.Context, zone string, ips []net.IP) Check {
	c := Check{ID: "dnsbl." + zone, Title: "Blocklist " + zone}
	c.LatencyMS = r.measure(func() {
		var listed, errored []string
		for _, ip := range ips {
			rev := reverseIP(ip)
			if rev == "" {
				continue
			}
			query := rev + "." + strings.TrimSuffix(zone, ".")
			cctx, cancel := r.lookupCtx(ctx)
			addrs, err := r.resolver.LookupHost(cctx, query)
			cancel()
			if err != nil {
				// NXDOMAIN is the not-listed answer and surfaces as a *net.DNSError
				// with IsNotFound — treat anything that resolves to nothing as clean.
				if isNotFound(err) {
					continue
				}
				errored = append(errored, fmt.Sprintf("%s: lookup error: %v", ip.String(), err))
				continue
			}
			if len(addrs) > 0 {
				listed = append(listed, fmt.Sprintf("%s (%s)", ip.String(), strings.Join(addrs, ",")))
			}
		}
		switch {
		case len(listed) > 0:
			c.Status = StatusFail
			c.Value = strings.Join(listed, "; ")
			c.Detail = "sending IP listed on " + zone
			c.Remediation = "request delisting from " + zone + " after fixing the cause (open relay, compromise, poor reputation)"
		case len(errored) > 0:
			c.Status = StatusWarn
			c.Detail = strings.Join(errored, "; ")
			c.Remediation = "the blocklist could not be queried; verify DNS resolution"
		default:
			c.Status = StatusOK
			c.Detail = "not listed on " + zone
		}
	})
	return c
}
