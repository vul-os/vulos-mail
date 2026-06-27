# Self-hosting vulos-mail (standalone)

vulos-mail is a **complete, independent mail server**. The server — SMTP, IMAP,
JMAP, CalDAV, CardDAV — runs with **no external services and no dependency on any
other Vulos project**. Cloud integration (with the Vulos control plane) is
strictly optional and lives behind a small interface seam — the core never
imports it. (The one add-on with a dependency is the *bundled React webmail*,
which needs a lilmail engine — see [The bundled webmail](#the-bundled-webmail-needs-a-lilmail-engine)
below; the protocol surfaces above never do.)

## Run it

```sh
go build -o vulos-mail ./cmd/vulos-mail

VULOS_DOMAIN=example.com \
VULOS_DATA_DIR=/var/lib/vulos-mail \
VULOS_ACCOUNT=you@example.com VULOS_PASSWORD=change-me \
./vulos-mail
```

That's the whole dependency list: a domain, a data directory, and (optionally) a
seed account. Blobs default to the local filesystem; the event log defaults to
append-only JSONL. No database server, no cloud, no third-party API.

### Accounts

- **Seed account** — `VULOS_ACCOUNT` / `VULOS_PASSWORD` provisions one account at
  startup (config is authoritative across restarts).
- **Self-serve signup** — `POST /api/signup` creates `handle@VULOS_DOMAIN`,
  gated by an **Altcha proof-of-work** challenge (`GET /api/signup/challenge`).
  Altcha is self-hosted PoW — no captcha service, no tracking. Disable public
  signup with `VULOS_SIGNUP=off`.
- Accounts persist to `VULOS_DATA_DIR/accounts.json` (bcrypt-hashed).

### Common options

| Env | Purpose |
|---|---|
| `VULOS_DOMAIN` | the mail domain (default `vulos.to`) |
| `VULOS_DATA_DIR` | data root (logs, blobs, accounts, DKIM) |
| `VULOS_TLS_CERT` / `VULOS_TLS_KEY` | bring-your-own TLS |
| `VULOS_ACME_DOMAINS` | Let's Encrypt via ACME (HTTP-01 on :80) |
| `VULOS_S3_ENDPOINT` … | store blobs in S3/MinIO instead of local FS |
| `VULOS_DB=sqlite` | durable SQLite event log backend |
| `RSPAMD_URL` | route inbound through rspamd spam scanning |
| `VULOS_ALTCHA_SECRET` | stable signing key for signup challenges |
| `VULOS_SIGNUP=off` | disable public self-serve signup |

## The bundled webmail (needs a lilmail engine)

The server itself — SMTP, IMAP, JMAP, CalDAV, CardDAV — is fully standalone and
needs nothing else. The **bundled React webmail** (`@vulos/mail-ui`), however,
speaks only the lilmail `/v1` JSON contract: vulos-mail is the *server*, and
[lilmail](https://github.com/vul-os/lilmail) is the *client engine*. So the
standalone webmail deployment is **vulos-mail + a lilmail engine + the UI**.

vulos-mail reverse-proxies `/v1/*` to the engine and brokers the signed-in user's
credentials to it (lilmail's CP-brokered credential mode). The flow:

1. The webmail signs in via `POST /api/webmail/login` → vulos-mail validates the
   mailbox credentials and sets an HttpOnly session cookie (the password is held
   server-side, never in the browser).
2. The UI's `/v1` calls carry that cookie; vulos-mail injects the credentials as
   `X-Vulos-Mail-*` broker headers (gated by `LILMAIL_BROKER_SECRET`) and proxies
   to the lilmail engine.
3. lilmail connects back to vulos-mail's **IMAP** (reads) and **SMTP submission**
   (sends) with those credentials and returns JSON.

Run lilmail with `LILMAIL_BROKER_SECRET=<secret>`, then point vulos-mail at it:

| Env | Purpose |
|---|---|
| `LILMAIL_ENGINE_URL` | base URL of the lilmail engine to proxy `/v1` to (e.g. `http://lilmail:8080`) |
| `LILMAIL_BROKER_SECRET` | shared secret authorizing brokered credentials (must equal lilmail's `LILMAIL_BROKER_SECRET`) |
| `VULOS_MAIL_IMAP_HOST` / `VULOS_MAIL_IMAP_PORT` | IMAP endpoint the engine dials back (default `VULOS_DOMAIN` / `993`; **implicit TLS** — lilmail dials IMAPS) |
| `VULOS_MAIL_SMTP_HOST` / `VULOS_MAIL_SMTP_PORT` | SMTP submission endpoint the engine dials back (default `VULOS_DOMAIN` / `587`; STARTTLS, or `465` for implicit TLS) |
| `LILMAIL_CALDAV_URL` | trusted CalDAV base URL injected into the broker headers so the standalone **Calendar** surface works (off → Calendar hidden). See *Calendar & Contacts* below. |
| `LILMAIL_CARDDAV_URL` | trusted CardDAV base URL injected into the broker headers so the standalone **Contacts** surface works (off → Contacts hidden). |

### Calendar & Contacts (standalone)

The `/v1` proxy always **strips** any client-supplied `X-Vulos-Mail-Caldav-Url` /
`X-Vulos-Mail-Carddav-Url` (SSRF / credential-exfil guard) and re-injects them
**only** from the trusted, operator-set `LILMAIL_CALDAV_URL` /
`LILMAIL_CARDDAV_URL`. When set, lilmail dials those DAV endpoints with the
signed-in user's brokered credential and the Calendar/Contacts surfaces become
functional; the `/api/webmail/account` capability flags (`calendar`, `contacts`)
flip on and the `/calendar` and `/contacts` deep links are mounted. When unset,
the surfaces stay hidden.

> **Auth constraint:** lilmail's brokered DAV dial presents `X-Vulos-Mail-Secret`
> as an HTTP **Bearer** token (its oauth2 DAV mode), so the configured endpoints
> must accept Bearer auth. vulos-mail's own built-in `/dav` backend is **Basic
> auth** (IMAP credentials) only, so these URLs are **not** auto-derived from
> `LILMAIL_ENGINE_URL` — point them at a Bearer-capable DAV service (or leave them
> unset to keep the surfaces hidden).

### Abuse / hardening

| Env | Purpose |
|---|---|
| `VULOS_TRUSTED_PROXIES` | CIDR/IP allowlist of fronting proxies. Their `X-Forwarded-For` is honoured for HTTP auth rate-limiting and their `X-Forwarded-Proto: https` is honoured for the Secure-cookie decision; a direct client can't spoof either. |
| `VULOS_FORCE_SECURE_COOKIE` | set (any non-empty value) to force the session cookie's `Secure` flag when TLS is terminated upstream and `X-Forwarded-Proto` isn't available. |

The webmail HTTP auth endpoints (`POST /api/webmail/login`, the
`/api/webmail/account/password` current-password check, the `POST
/api/webmail/send` Basic-auth gate, and the `/api/llm` gate) share the same
per-IP/per-account brute-force limiter already wired into IMAP/SMTP/JMAP; a
locked key is refused (HTTP 429, or 401 on the LLM gate) before the credential
check runs.

If `LILMAIL_ENGINE_URL` is **unset**, the webmail's `/v1` calls return a clear
`{"error":"mail engine not configured"}` (HTTP 503) and the UI shows a "mail
engine not configured" state — the rest of the server (JMAP/IMAP/DAV and the
`/api/webmail/send` API) is unaffected, so external mail clients keep working.

> Note: `@vulos/mail-ui`'s `createMailClient`/`<MailApp/>` default `baseUrl` is
> `/v1` (same-origin). The bundled webmail passes it explicitly
> (`<MailApp baseUrl="/v1">`), which resolves to vulos-mail's proxy.

### Account & settings (self-hoster surface)

The webmail's **Settings → Account** section is backed by two session-scoped
endpoints on vulos-mail itself (independent of the lilmail engine), so a
self-hoster gets a sensible account surface even with `/v1` degraded:

| Endpoint | Purpose |
|---|---|
| `GET /api/webmail/account` | The signed-in identity, the **IMAP/SMTP connection settings** (host/port/security, from `VULOS_MAIL_*`) to configure an external mail client, and the deployment's `capabilities` (`changePassword`, `signup`, `engine`). |
| `POST /api/webmail/account/password` | Change the mailbox password in place: re-verifies the current password, persists the new one to the local account store, and rotates the live session's brokered credential (no forced re-login). |

The UI only shows what the backend reports: change-password is offered **only on
the standalone local-identity path** (hidden when `VULOS_CP_URL` makes the cloud
control plane own identity — the endpoint then returns `501`). The IMAP/SMTP
client-setup card always reflects the configured `VULOS_MAIL_IMAP_*` /
`VULOS_MAIL_SMTP_*` values, and **Sign out** clears the session.

## The integration seam (why it stays independent)

The core depends only on the interfaces in [`internal/seam`](internal/seam/seam.go):

| Interface | Standalone default (`internal/seam/local`, `…/altcha`) | Optional cloud adapter (`integration/cloud`) |
|---|---|---|
| `Identity` | file-backed bcrypt account store | vulos-cloud identity |
| `Entitlements` | unlimited (`self-hosted`) | vulos-cloud quota/billing |
| `Usage` | no-op | vulos-cloud metered events |
| `SignupGate` | Altcha proof-of-work | vulos-cloud PoW + invites |

The mail core (`internal/*`, `adapters/*`) has **zero imports** of
`integration/cloud` — enforced and easy to verify:

```sh
go list -deps ./internal/... ./adapters/... | grep integration/cloud   # → no output
```

Only the command (`cmd/vulos-mail`) wires an implementation, and it picks the
**standalone defaults unless `VULOS_CP_URL` is set**. To run with the Vulos
control plane instead, set `VULOS_CP_URL` (+ `VULOS_CP_SECRET`); everything else
is unchanged. Remove those env vars and you're fully independent again.
