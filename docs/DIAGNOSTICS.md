# Mail diagnostics

`vulos-mail` ships an advanced deliverability and health diagnostics suite. The
same check suite drives two surfaces:

- **CLI** — `vulos-mail diagnostics` (human report; `--json` for machine output),
  for self-hosters.
- **HTTP API** — `GET /api/diagnostics` (broker-gated JSON), consumed by Vulos
  Cloud's status page.

The package (`internal/diagnostics`) is self-contained: it has **no dependency on
Vulos Cloud**, and every network dependency (DNS, TLS, the round-trip prober) is
injected behind an interface, so the check logic is fully unit-tested offline.

## Checks

Each check returns: `id` (stable dotted key), `title`, `status` (`ok` | `warn` |
`fail`), `detail`, `remediation` (on warn/fail), `value` (the measured record /
address / cert), and `latencyMs`.

| id | What it verifies | warn | fail |
|----|------------------|------|------|
| `dns.mx` | MX records published for the domain | — | no MX / lookup error |
| `dns.spf` | SPF (`v=spf1`) TXT record present | `+all` / `?all` (too permissive) | missing |
| `dns.dkim.<selector>` | DKIM public key at `<selector>._domainkey.<domain>` (one per configured selector) | empty `p=` (revoked) | missing / no `p=` |
| `dns.dmarc` | DMARC (`v=DMARC1`) policy at `_dmarc.<domain>` | `p=none` (monitor only) | missing |
| `dns.a` | A/AAAA records resolve for the domain | — | none resolve |
| `dns.ptr` | Each sending IP has forward-confirmed reverse DNS matching HELO | not forward-confirmed / PTR≠HELO | no PTR |
| `dnsbl.<zone>` | Sending IP not listed on each configured blocklist | lookup error | listed |
| `tls.smtp` | SMTP submission STARTTLS certificate valid (time, hostname, chain) | expiring <14d / untrusted chain (self-signed) | expired / hostname mismatch / unreachable |
| `tls.imap` | IMAPS implicit-TLS certificate valid | expiring <14d / untrusted chain | expired / hostname mismatch / unreachable |
| `roundtrip` | End-to-end send→deliver→receive self-test (see below) | disabled / not configured / rate-limited | probe not received within timeout |

If no sending IP is configured (and auto-detect is off), `dns.ptr` and the
blocklist checks report a single `warn` explaining they were skipped.

## Round-trip self-test

When enabled (`roundtrip = true`) and the test-mailbox credentials are configured,
the suite sends a probe message over SMTP submission (STARTTLS + AUTH) to the
configured test mailbox (`test@<domain>` by default), carrying a unique,
unguessable token in a dedicated `X-Vulos-Diag-Probe` header. It then polls the
mailbox over IMAPS until that exact message arrives, measures the end-to-end
latency, and **deletes the probe** (expunged) so probes never accumulate.

It is **rate-limited**: within `roundtrip_min_interval` (default 1 minute) of the
previous probe the check reports `warn` (rate-limited) instead of sending another
message. Without configured credentials the check reports `warn` (not configured)
and never sends anything — the prober has no live default (open-core seam).

## Overall status & summary

The report's top-level `status` is the worst status across all checks, and
`summary` counts `ok` / `warn` / `fail` / `total`. The CLI exits non-zero when the
overall status is `fail`.

## Report JSON shape

```json
{
  "domain": "vulos.to",
  "generatedAt": "2026-06-28T12:00:00Z",
  "status": "warn",
  "summary": { "ok": 8, "warn": 1, "fail": 0, "total": 9 },
  "checks": [
    {
      "id": "dns.spf",
      "title": "SPF record",
      "status": "ok",
      "detail": "SPF record present",
      "value": "v=spf1 mx -all",
      "latencyMs": 12
    },
    {
      "id": "dns.dmarc",
      "title": "DMARC policy",
      "status": "warn",
      "detail": "DMARC policy is p=none (monitor only) — spoofed mail is not rejected",
      "remediation": "after confirming alignment from rua reports, move to p=quarantine then p=reject",
      "value": "v=DMARC1; p=none",
      "latencyMs": 9
    }
  ]
}
```

## Configuration

See the `[diagnostics]` section in [CONFIGURATION.md](CONFIGURATION.md). The HTTP
endpoint is gated by `LILMAIL_BROKER_SECRET` (the same broker-auth pattern as the
other admin/brokered routes); see [API.md](API.md).
