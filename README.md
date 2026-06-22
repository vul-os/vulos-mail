<div align="center">

<img src="webmail/vulos-mail.png" alt="Vulos Mail" width="128" height="128" />

# Vulos Mail

### Sovereign mail. You own it.

A complete, self-hostable, **event-sourced mail server** — SMTP · IMAP · JMAP ·
CalDAV · CardDAV — with a modern **React webmail** client, in a single static Go
binary. No database server, no cloud, no third-party API required.

</div>

---

## What it is

Vulos Mail is an independent mail server you can run anywhere. It speaks the
open protocols mail clients already use, ships a fast keyboard-driven webmail,
and stores everything in an append-only event log on disk. Cloud integration
(billing, hosted identity) is **strictly optional** and lives behind a small
interface seam — the mail core never imports it.

## Features

**Server**
- **SMTP** inbound (MX) + authenticated **submission**, with an outbound queue
  and scheduler (retry/defer/bounce), DKIM signing, and optional rspamd scanning.
- **IMAP** and **JMAP** (RFC 8620/8621) access to the same mailbox.
- **CalDAV** + **CardDAV** for calendars and contacts.
- **Event-sourced** storage: append-only JSONL event log (or durable SQLite),
  blobs on local FS or S3/MinIO.
- **Self-serve signup** gated by a self-hosted **Altcha proof-of-work** challenge
  (no captcha service, no tracking).
- TLS via bring-your-own certs or built-in **ACME / Let's Encrypt**.
- Prometheus metrics endpoint.

**Webmail** (`webmail/`, React + Vite + Tailwind)
- Token-based dark/light design system (teal accent, Vulos-purple brand).
- Mailbox / label list, message list with infinite scroll, reading pane.
- Compose / reply / forward with rich-text body, attachments (drag-and-drop),
  **draft autosave**, and a **5-second send-undo** window.
- Star · archive · delete · labels · multi-select bulk actions.
- Client-side **search**, a **⌘K command palette**, and full keyboard shortcuts
  (`j/k`, `c`, `r`, `e`, `#`, `s`, `g i`, `?`, …).
- Contacts, a month/agenda **calendar**, and settings (signature, vacation
  responder, theme).
- Self-serve **signup** that solves the Altcha PoW in-browser and signs you in.
- Live updates over Server-Sent Events; responsive down to a single-pane phone
  layout.
- **XSS-inert email rendering** — message bodies are sanitized to an inert safe
  subset (scripts / `onerror` / `javascript:` never survive), never injected raw.

## Architecture

```
              ┌──────────── single Go binary (cmd/vulos-mail) ────────────┐
  clients ───▶│  SMTP :2525   Submission :2587   IMAP :2143               │
              │  HTTP :2080 ── JMAP /jmap/*  ·  CalDAV  ·  CardDAV         │
              │              ── /api/webmail/*  (send, attachment,         │
              │                  calendar, contacts, settings, changes)   │
              │              ── /api/signup(+/challenge)  (Altcha PoW)     │
              │              ── /  static React webmail (webmail/dist)     │
              └──────────────────────────────┬────────────────────────────┘
                                              │
              event-sourced core  ── append-only JSONL log / SQLite
                                  ── blobs: local FS / S3-MinIO
                                  ── DKIM, accounts.json (bcrypt)

  optional vulos-cloud seam (internal/seam): identity · entitlements ·
  usage · signup-gate — wired only by cmd/* when VULOS_CP_URL is set.
  The mail core has zero imports of integration/cloud.
```

The webmail is a static SPA that talks to the server's JMAP endpoint and the
`/api/webmail/*` helpers. It reuses a tiny dependency-free JMAP client
(`webmail/src/lib/jmap.js`).

## Quickstart (self-host)

Build the webmail and the server:

```sh
# 1. build the React webmail → webmail/dist
cd webmail && npm ci && npm run build && cd ..

# 2. build + run the server (serves webmail/dist at :2080 by default)
go build -o vulos-mail ./cmd/vulos-mail

VULOS_DOMAIN=example.com \
VULOS_DATA_DIR=/var/lib/vulos-mail \
VULOS_ACCOUNT=you@example.com VULOS_PASSWORD=change-me \
./vulos-mail
```

Open <http://localhost:2080> and sign in. That's the whole dependency list: a
domain, a data directory, and (optionally) a seed account.

### Docker

```sh
docker build -t vulos-mail .      # multi-stage: builds webmail + Go binary
docker run -p 2080:2080 -p 2525:2525 -p 2587:2587 -p 2143:2143 \
  -e VULOS_DOMAIN=example.com -e VULOS_ACCOUNT=you@example.com \
  -e VULOS_PASSWORD=change-me -v vulos-data:/data vulos-mail
```

### Webmail development

```sh
cd webmail
npm install
npm run dev        # Vite dev server with HMR (proxy /jmap + /api to your server)
npm run build      # production build → webmail/dist
npm test           # Vitest unit tests (sanitizer / helpers)
```

## Configuration

All configuration is via environment variables. Common ones:

| Env | Default | Purpose |
|---|---|---|
| `VULOS_DOMAIN` | `vulos.to` | the mail domain |
| `VULOS_DATA_DIR` | `./data` | data root (event log, blobs, accounts, DKIM) |
| `VULOS_ACCOUNT` / `VULOS_PASSWORD` | — | provision one seed account at startup |
| `VULOS_WEBMAIL_DIR` | `./webmail/dist` | static webmail directory to serve |
| `VULOS_MX_ADDR` | `:2525` | inbound SMTP listen address |
| `VULOS_SUBMIT_ADDR` | `:2587` | authenticated submission address |
| `VULOS_IMAP_ADDR` | `:2143` | IMAP listen address |
| `VULOS_JMAP_ADDR` | `:2080` | HTTP (JMAP / DAV / API / webmail) address |
| `VULOS_METRICS_ADDR` | `:2090` | Prometheus metrics address |
| `VULOS_TLS_CERT` / `VULOS_TLS_KEY` | — | bring-your-own TLS |
| `VULOS_ACME_DOMAINS` | — | Let's Encrypt via ACME (HTTP-01 on :80) |
| `VULOS_DB` | JSONL | set `sqlite` for a durable SQLite event log |
| `VULOS_S3_ENDPOINT` | — | store blobs in S3 / MinIO instead of local FS |
| `VULOS_SIGNUP` | on | set `off` to disable public self-serve signup |
| `VULOS_ALTCHA_SECRET` | random | stable signing key for signup PoW challenges |
| `VULOS_CP_URL` / `VULOS_CP_SECRET` | — | opt into the vulos-cloud control-plane seam |

See [`SELFHOST.md`](SELFHOST.md) for the full list and the integration-seam
details.

## Testing

```sh
go test ./...                 # Go unit + integration tests
cd webmail && npm test        # webmail unit tests (Vitest)
./test/webmail/run.sh         # end-to-end: build webmail, boot server, seed
                              # mail, drive the SPA in headless Chrome
```

The end-to-end harness builds the React webmail, points `VULOS_WEBMAIL_DIR` at
`webmail/dist`, seeds an inbox (including a hostile XSS-probe message), and
asserts behavior across every surface — login, signup PoW, list, read,
XSS-inertness, star, search, command palette, compose, contacts, calendar,
settings, and bulk actions.

## License

See repository headers. This project is part of the Vulos suite; the mail core
is independent and self-hostable with no external service dependencies.
