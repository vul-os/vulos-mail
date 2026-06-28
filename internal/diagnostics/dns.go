package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// dnsChecks runs the DNS-record deliverability checks: MX, SPF, DKIM (per
// selector), DMARC, A/AAAA, and PTR/reverse-DNS vs HELO.
func (r *Runner) dnsChecks(ctx context.Context) []Check {
	var out []Check
	out = append(out, r.mxCheck(ctx))
	out = append(out, r.spfCheck(ctx))
	for _, sel := range r.cfg.DKIMSelectors {
		out = append(out, r.dkimCheck(ctx, sel))
	}
	out = append(out, r.dmarcCheck(ctx))
	out = append(out, r.addrCheck(ctx))
	out = append(out, r.ptrCheck(ctx))
	return out
}

// lookupCtx returns a child context bounded by the per-check timeout.
func (r *Runner) lookupCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.cfg.Timeout)
}

func (r *Runner) mxCheck(ctx context.Context) Check {
	c := Check{ID: "dns.mx", Title: "MX records"}
	c.LatencyMS = r.measure(func() {
		cctx, cancel := r.lookupCtx(ctx)
		defer cancel()
		mxs, err := r.resolver.LookupMX(cctx, r.cfg.Domain)
		if err != nil {
			c.Status = StatusFail
			c.Detail = fmt.Sprintf("MX lookup for %s failed: %v", r.cfg.Domain, err)
			c.Remediation = "publish at least one MX record for the domain pointing at this mail server"
			return
		}
		if len(mxs) == 0 {
			c.Status = StatusFail
			c.Detail = fmt.Sprintf("no MX records published for %s", r.cfg.Domain)
			c.Remediation = "publish an MX record (e.g. " + r.cfg.Domain + ".  IN MX 10 mx." + r.cfg.Domain + ".)"
			return
		}
		hosts := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			hosts = append(hosts, fmt.Sprintf("%d %s", mx.Pref, strings.TrimSuffix(mx.Host, ".")))
		}
		c.Status = StatusOK
		c.Value = strings.Join(hosts, ", ")
		c.Detail = fmt.Sprintf("%d MX record(s) published", len(mxs))
	})
	return c
}

func (r *Runner) spfCheck(ctx context.Context) Check {
	c := Check{ID: "dns.spf", Title: "SPF record"}
	c.LatencyMS = r.measure(func() {
		cctx, cancel := r.lookupCtx(ctx)
		defer cancel()
		txt, err := r.resolver.LookupTXT(cctx, r.cfg.Domain)
		if err != nil {
			c.Status = StatusFail
			c.Detail = fmt.Sprintf("TXT lookup for %s failed: %v", r.cfg.Domain, err)
			c.Remediation = "publish an SPF TXT record (e.g. \"v=spf1 mx ~all\")"
			return
		}
		spf := ""
		for _, t := range sortedTXT(txt) {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
				spf = strings.TrimSpace(t)
				break
			}
		}
		if spf == "" {
			c.Status = StatusFail
			c.Detail = "no SPF record (v=spf1 …) published"
			c.Remediation = "publish an SPF TXT record authorising this server, e.g. \"v=spf1 mx ~all\""
			return
		}
		c.Value = spf
		// "+all" (or a bare "all") authorises the whole internet — useless and a
		// deliverability liability. "?all" (neutral) is weak.
		low := strings.ToLower(spf)
		switch {
		case strings.Contains(low, "+all") || strings.HasSuffix(low, " all"):
			c.Status = StatusWarn
			c.Detail = "SPF ends in +all (passes for any sender) — this defeats SPF"
			c.Remediation = "tighten the SPF all-qualifier to ~all (softfail) or -all (fail)"
		case strings.Contains(low, "?all"):
			c.Status = StatusWarn
			c.Detail = "SPF ends in ?all (neutral) — provides little protection"
			c.Remediation = "tighten the SPF all-qualifier to ~all (softfail) or -all (fail)"
		default:
			c.Status = StatusOK
			c.Detail = "SPF record present"
		}
	})
	return c
}

func (r *Runner) dkimCheck(ctx context.Context, selector string) Check {
	c := Check{ID: "dns.dkim." + selector, Title: "DKIM key (" + selector + ")"}
	name := selector + "._domainkey." + r.cfg.Domain
	c.LatencyMS = r.measure(func() {
		cctx, cancel := r.lookupCtx(ctx)
		defer cancel()
		txt, err := r.resolver.LookupTXT(cctx, name)
		if err != nil || len(txt) == 0 {
			c.Status = StatusFail
			c.Detail = fmt.Sprintf("no DKIM record at %s", name)
			c.Remediation = "publish the DKIM public key TXT record for selector " + selector + " (see `vulos-mail` startup log / dkimgen)"
			return
		}
		rec := strings.TrimSpace(strings.Join(txt, ""))
		c.Value = rec
		tags := parseTagList(rec)
		p, hasP := tags["p"]
		if !hasP {
			c.Status = StatusFail
			c.Detail = "DKIM record has no p= public-key tag"
			c.Remediation = "republish the DKIM record including the p= public key"
			return
		}
		if p == "" {
			c.Status = StatusWarn
			c.Detail = "DKIM record has an empty p= (key revoked)"
			c.Remediation = "publish a live DKIM public key, or remove the selector if intentionally revoked"
			return
		}
		c.Status = StatusOK
		c.Detail = "DKIM public key published for selector " + selector
	})
	return c
}

func (r *Runner) dmarcCheck(ctx context.Context) Check {
	c := Check{ID: "dns.dmarc", Title: "DMARC policy"}
	name := "_dmarc." + r.cfg.Domain
	c.LatencyMS = r.measure(func() {
		cctx, cancel := r.lookupCtx(ctx)
		defer cancel()
		txt, err := r.resolver.LookupTXT(cctx, name)
		if err != nil || len(txt) == 0 {
			c.Status = StatusFail
			c.Detail = fmt.Sprintf("no DMARC record at %s", name)
			c.Remediation = "publish a DMARC TXT record, e.g. \"v=DMARC1; p=quarantine; rua=mailto:dmarc@" + r.cfg.Domain + "\""
			return
		}
		var dmarc string
		for _, t := range sortedTXT(txt) {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
				dmarc = strings.TrimSpace(t)
				break
			}
		}
		if dmarc == "" {
			c.Status = StatusFail
			c.Detail = "no DMARC record (v=DMARC1 …) published"
			c.Remediation = "publish a DMARC TXT record with at least p=none and a rua= report address"
			return
		}
		c.Value = dmarc
		policy := parseTagList(dmarc)["p"]
		switch strings.ToLower(policy) {
		case "reject", "quarantine":
			c.Status = StatusOK
			c.Detail = "DMARC policy p=" + policy
		case "none", "":
			c.Status = StatusWarn
			c.Detail = "DMARC policy is p=none (monitor only) — spoofed mail is not rejected"
			c.Remediation = "after confirming alignment from rua reports, move to p=quarantine then p=reject"
		default:
			c.Status = StatusWarn
			c.Detail = "DMARC policy p=" + policy + " is not recognised"
			c.Remediation = "set p= to none, quarantine, or reject"
		}
	})
	return c
}

func (r *Runner) addrCheck(ctx context.Context) Check {
	c := Check{ID: "dns.a", Title: "A/AAAA records"}
	c.LatencyMS = r.measure(func() {
		cctx, cancel := r.lookupCtx(ctx)
		defer cancel()
		addrs, err := r.resolver.LookupIPAddr(cctx, r.cfg.Domain)
		if err != nil || len(addrs) == 0 {
			c.Status = StatusFail
			c.Detail = fmt.Sprintf("no A/AAAA records resolve for %s", r.cfg.Domain)
			c.Remediation = "publish an A (and/or AAAA) record for the mail host"
			return
		}
		ips := make([]string, 0, len(addrs))
		for _, a := range addrs {
			ips = append(ips, a.IP.String())
		}
		c.Status = StatusOK
		c.Value = strings.Join(ips, ", ")
		c.Detail = fmt.Sprintf("%d address record(s) resolve", len(addrs))
	})
	return c
}

// ptrCheck verifies that each sending IP has a PTR (reverse DNS) record and that
// it is forward-confirmed and consistent with the HELO name — a common reason
// outbound mail is rejected or scored as spam.
func (r *Runner) ptrCheck(ctx context.Context) Check {
	c := Check{ID: "dns.ptr", Title: "PTR / reverse DNS"}
	c.LatencyMS = r.measure(func() {
		ips := r.sendingIPs(ctx)
		if len(ips) == 0 {
			c.Status = StatusWarn
			c.Detail = "no sending IP configured — cannot verify reverse DNS"
			c.Remediation = "set [diagnostics] sending_ips (or enable auto_detect_ip) so PTR can be checked"
			return
		}
		var details, problems []string
		worst := StatusOK
		for _, ip := range ips {
			cctx, cancel := r.lookupCtx(ctx)
			names, err := r.resolver.LookupAddr(cctx, ip.String())
			cancel()
			if err != nil || len(names) == 0 {
				worst = mergeStatus(worst, StatusFail)
				problems = append(problems, ip.String()+": no PTR record")
				continue
			}
			ptr := strings.TrimSuffix(names[0], ".")
			details = append(details, ip.String()+" -> "+ptr)
			// Forward-confirm: the PTR name must resolve back to the same IP.
			fctx, fcancel := r.lookupCtx(ctx)
			fwd, ferr := r.resolver.LookupHost(fctx, ptr)
			fcancel()
			confirmed := false
			if ferr == nil {
				for _, h := range fwd {
					if h == ip.String() {
						confirmed = true
						break
					}
				}
			}
			if !confirmed {
				worst = mergeStatus(worst, StatusWarn)
				problems = append(problems, ip.String()+": PTR "+ptr+" is not forward-confirmed")
			}
			if r.cfg.HELO != "" && !strings.EqualFold(ptr, r.cfg.HELO) {
				worst = mergeStatus(worst, StatusWarn)
				problems = append(problems, fmt.Sprintf("%s: PTR %s does not match HELO %s", ip.String(), ptr, r.cfg.HELO))
			}
		}
		c.Value = strings.Join(details, "; ")
		c.Status = worst
		if worst == StatusOK {
			c.Detail = "all sending IPs have forward-confirmed reverse DNS matching HELO"
			return
		}
		c.Detail = strings.Join(problems, "; ")
		c.Remediation = "set a PTR record on each sending IP that matches the HELO name and forward-resolves back to the IP"
	})
	return c
}

// parseTagList parses a "key=value; key=value" DNS record (SPF/DKIM/DMARC tag
// lists) into a map. Keys are lower-cased; values keep their case.
func parseTagList(rec string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(rec, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}

// isNotFound reports whether err is an NXDOMAIN / no-such-host DNS error (the
// "not listed" / "no record" answer), as opposed to a transient resolver fault.
func isNotFound(err error) bool {
	var de *net.DNSError
	if errors.As(err, &de) {
		return de.IsNotFound
	}
	return false
}

// mergeStatus returns the worse of two statuses.
func mergeStatus(a, b Status) Status {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// reverseIP returns the reversed-nibble/octet label form of ip used for DNSBL and
// PTR queries (e.g. 1.2.3.4 -> "4.3.2.1"). It returns "" for an invalid IP.
func reverseIP(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d", v4[3], v4[2], v4[1], v4[0])
	}
	v6 := ip.To16()
	if v6 == nil {
		return ""
	}
	const hex = "0123456789abcdef"
	var b strings.Builder
	for i := len(v6) - 1; i >= 0; i-- {
		b.WriteByte(hex[v6[i]&0x0f])
		b.WriteByte('.')
		b.WriteByte(hex[v6[i]>>4])
		if i > 0 {
			b.WriteByte('.')
		}
	}
	return b.String()
}
