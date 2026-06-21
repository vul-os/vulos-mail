# Self-hosting vulos-mail (standalone)

vulos-mail is a **complete, independent mail server**. It runs with **no external
services and no dependency on any other Vulos project**. Cloud integration (with
the Vulos control plane) is strictly optional and lives behind a small interface
seam — the core never imports it.

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
