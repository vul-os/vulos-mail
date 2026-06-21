# vulos-mail end-to-end test suite

Two tiers, both runnable on any machine with Go and Docker (OrbStack works as a
drop-in). Run everything with `make test-all`.

## Tier 1 — in-process (offline, fast)
`make race` / `make fullstack`

The full Go suite (58 test files), race- and vet-clean, including
`TestEndToEndAllProtocols`: a single process wiring SMTP-in, submission, IMAP,
JMAP, and the outbound scheduler into one closed loop, plus the pen-test /
security regression suite (open-relay, sender-spoof, cross-account isolation,
path traversal, A-R forgery, etc.).

## Tier 2 — Dockerized closed-loop ecosystem (the "complete" test)
`make e2e`  (wraps `test/e2e/run.sh`)

Brings up a **private mail universe** so we can test things a single instance
cannot:

```
        ┌─────────┐   MX/SPF/DKIM/DMARC for a.test + b.test
        │  CoreDNS │ 172.28.0.10
        └────┬────┘
   resolver  │  resolver
        ┌────┴─────┐         SMTP :25 (over the wire)        ┌──────────┐
        │  mta-a   │ ───────────────────────────────────▶   │  mta-b   │
        │ a.test   │ 172.28.0.20            172.28.0.30      │ b.test   │
        └──────────┘                                         └──────────┘
              ▲  drives every protocol            verifies SPF/DKIM/DMARC ▲
              └──────────────────  runner  ───────────────────────────────┘
```

`gen.sh` pre-generates each domain's DKIM key (in the exact PEM the server loads)
and publishes the matching public key — plus MX, SPF, and DMARC records — into
the CoreDNS zone, so authentication is evaluated **for real** against DNS.

The runner asserts **20 checks**: inbound MX delivery, IMAP login/select/search,
JMAP get/set/Identity, webmail contacts/calendar/push APIs, CalDAV+CardDAV
PROPFIND, security boundaries (open-relay rejected, unknown-recipient rejected at
RCPT, submission From-spoof rejected), Prometheus metrics, and the centerpiece:

> **alice@a.test → bob@b.test over real SMTP, received with
> `spf=pass; dkim=pass; dmarc=pass`** in bob's Authentication-Results.

Everything is torn down (`down -v`) on exit.

## What this suite canNOT test (true external dependencies)
These need the real internet / third parties and are out of scope for any
self-contained harness:

- **Real-world deliverability / inboxing** at Gmail, Outlook, Yahoo — requires
  real public IPs with PTR/rDNS, aged IP reputation, feedback loops (FBLs), and
  not being on blocklists. The warmup/reputation *logic* is unit-tested; actual
  inbox placement is not.
- **Public ACME / Let's Encrypt issuance** — needs a real public domain with
  reachable :80/:443 and real DNS. (The ACME *wiring* is in place; a local
  Pebble/step-ca container could exercise the flow but isn't included here.)
- **Real DNS at scale** — DNSSEC, real resolver quirks, propagation. We serve a
  controlled zone instead.
- **NCMEC reporting + real CSAM hash corpora** — gated by credentials and law;
  the hashing/scan plumbing is unit-tested with synthetic data only.
- **Third-party client interop** (Thunderbird, Apple Mail, Outlook desktop) — can
  be done manually against `make docker-up`, not automated here.
- **Production load / multi-region / anchor-inbox geo behavior** — needs a real
  multi-host deployment.

## Requirements
Go 1.25+, Docker (or OrbStack) with compose v2. First run pulls CoreDNS +
python:slim and builds the server image (~1–2 min); subsequent runs are cached.
