# Configuration

`vulos-mail` is configured via environment variables (optionally loaded from a
dotenv file selected by `VULOS_ENV_FILE` / `VULOS_ENV`, see `cmd/vulos-mail`). This
document covers the configuration relevant to the diagnostics suite and the
broker-gated operations endpoints. For the broader server configuration see
`README.md` and `SELFHOST.md`.

## Broker secret (admin/brokered endpoints)

| Variable | Default | Description |
|----------|---------|-------------|
| `LILMAIL_BROKER_SECRET` | _(unset)_ | Shared secret gating the broker/admin HTTP endpoints (`GET /api/diagnostics`, `POST /api/admin/provision-mailbox`) via the `X-Vulos-Broker-Auth` header. **When unset, those endpoints are closed.** Also gates the lilmail `/v1` engine broker. |

`GET /healthz` is always open (no secret).

## `[diagnostics]`

The diagnostics check suite (CLI `vulos-mail diagnostics` and `GET /api/diagnostics`)
reads the following. Conceptually these form a `[diagnostics]` section; concretely
they are environment variables.

| Variable | `[diagnostics]` key | Default | Description |
|----------|---------------------|---------|-------------|
| `VULOS_DIAG_ENABLED` | `enabled` | off | Master switch. When unset, the suite returns a single `warn` saying diagnostics are disabled. Set to any non-empty value to enable. |
| `VULOS_DIAG_DKIM_SELECTORS` | `dkim_selectors` | `vulos-mail` | Comma-separated DKIM selectors to verify (one `dns.dkim.<selector>` check each). |
| `VULOS_DIAG_DNSBLS` | `dnsbls` | `zen.spamhaus.org, bl.spamcop.net, b.barracudacentral.org` | Comma-separated DNS blocklist zones to query for the sending IP(s). |
| `VULOS_DIAG_SENDING_IPS` | `sending_ips` | _(none)_ | Comma-separated public IP(s) outbound mail leaves from, used for the PTR and blocklist checks. |
| `VULOS_DIAG_AUTODETECT_IP` | `auto_detect_ip` | off | When set and `sending_ips` is empty, use the domain's A/AAAA records as a best-effort sending IP. |
| `VULOS_DIAG_HELO` | `helo` | `VULOS_DOMAIN` | The HELO/EHLO name announced by the server; PTR records are matched against it. |
| `VULOS_DIAG_TEST_MAILBOX` | `test_mailbox` | `test@<domain>` | The round-trip self-test recipient. |
| `VULOS_DIAG_ROUNDTRIP` | `roundtrip` | off | Enable the send→deliver→receive self-test. |
| `VULOS_DIAG_TEST_USER` | `test_user` | `test_mailbox` | SMTP/IMAP username for the test mailbox (round-trip prober). |
| `VULOS_DIAG_TEST_PASSWORD` | `test_password` | _(none)_ | Password for the test mailbox. **The round-trip prober is only wired when this is set**; otherwise the self-test reports "not configured". |
| `VULOS_DIAG_ROUNDTRIP_INTERVAL` | `roundtrip_min_interval` | `1m` | Rate limit: minimum gap between probes. Within this window the self-test reports `warn` (rate-limited) instead of sending another probe. Go duration string. |
| `VULOS_DIAG_TIMEOUT` | `timeout` | `10s` | Per-check timeout (and default round-trip poll budget). Go duration string. |
| `VULOS_DIAG_INSECURE_TLS` | `insecure_tls` | off | Skip TLS verification for the round-trip prober's SMTP/IMAP connections (self-signed dev servers only). Does **not** affect the `tls.*` certificate checks, which always report the real verification result. |

The diagnostics SMTP/IMAP endpoints reuse the server's connection settings:

| Variable | Default | Used for |
|----------|---------|----------|
| `VULOS_MAIL_SMTP_HOST` / `VULOS_MAIL_SMTP_PORT` | `<domain>` / `587` | `tls.smtp` STARTTLS probe + round-trip submission |
| `VULOS_MAIL_IMAP_HOST` / `VULOS_MAIL_IMAP_PORT` | `<domain>` / `993` | `tls.imap` probe + round-trip IMAP poll |
| `VULOS_DOMAIN` | `vulos.to` | Domain under test (MX/SPF/DKIM/DMARC/A keyed on it) |

### Example

```sh
VULOS_DIAG_ENABLED=1
VULOS_DIAG_SENDING_IPS=203.0.113.10,2001:db8::10
VULOS_DIAG_DKIM_SELECTORS=vulos-mail,backup
VULOS_DIAG_ROUNDTRIP=1
VULOS_DIAG_TEST_PASSWORD=...redacted...
VULOS_DIAG_ROUNDTRIP_INTERVAL=5m
```

Run the CLI report:

```sh
vulos-mail diagnostics          # human-readable table; exit 1 if any check fails
vulos-mail diagnostics --json   # machine-readable Report JSON
```
