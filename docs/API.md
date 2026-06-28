# HTTP API

This documents the operations / integration HTTP endpoints `vulos-mail` exposes on
its HTTP listener (`VULOS_JMAP_ADDR`, default `:2080`). For the webmail/`/v1`,
JMAP, CalDAV/CardDAV and signup surfaces see the source and `SELFHOST.md`.

## Broker authentication

Admin / brokered endpoints are gated with the same shared-secret pattern used for
the lilmail engine broker: the caller presents the secret in the
`X-Vulos-Broker-Auth` request header, and the server compares it (constant-time)
against `LILMAIL_BROKER_SECRET`.

- If `LILMAIL_BROKER_SECRET` is **unset/empty**, the broker-gated endpoints are
  **closed** (every request gets `401`).
- A request without the matching secret gets `401 {"error":"broker authentication required"}`.

These endpoints are intended to be called server-to-server by Vulos Cloud's control
plane, not by browsers.

---

## `GET /healthz`

Unauthenticated liveness probe for the status page / load balancer. Reports only
that the HTTP server is up (it does **not** run the diagnostics suite).

**Response** `200`:

```json
{ "status": "ok" }
```

---

## `GET /api/diagnostics`

Broker-gated. Runs the deliverability/health diagnostics suite and returns the
full JSON `Report` (see [DIAGNOSTICS.md](DIAGNOSTICS.md) for the check list and the
exact JSON shape). Always returns HTTP `200` on success — the per-check and overall
`status` (`ok` | `warn` | `fail`) live in the body, which the status page reads.

**Request**

```
GET /api/diagnostics
X-Vulos-Broker-Auth: <LILMAIL_BROKER_SECRET>
```

**Response** `200`: a `Report` object:

```json
{
  "domain": "vulos.to",
  "generatedAt": "2026-06-28T12:00:00Z",
  "status": "ok",
  "summary": { "ok": 9, "warn": 0, "fail": 0, "total": 9 },
  "checks": [
    { "id": "dns.mx", "title": "MX records", "status": "ok",
      "detail": "1 MX record(s) published", "value": "10 mx.vulos.to", "latencyMs": 8 }
  ]
}
```

Errors: `401` (broker auth), `405` (non-GET).

---

## `POST /api/admin/provision-mailbox`

Broker-gated. The **free-mail mailbox provisioning seam** used by Vulos Cloud's
free-org-mail feature: it creates/ensures a mailbox `<localpart>@<domain>` on the
configured mail server. It is **idempotent** — provisioning an address that already
exists succeeds without changing it.

**Request**

```
POST /api/admin/provision-mailbox
X-Vulos-Broker-Auth: <LILMAIL_BROKER_SECRET>
Content-Type: application/json
```

```json
{
  "localpart": "alice",
  "domain": "vulos.to",
  "org": "acme",
  "password": "optional-explicit-password"
}
```

- `localpart` (required): the part before `@`. Must not contain `@`, spaces or tabs.
- `domain` (optional): defaults to the server's `VULOS_DOMAIN`.
- `org` (optional): the owning organisation (for the caller's bookkeeping; logged).
- `password` (optional): when omitted, a strong random password is generated. The
  credential is expected to be owned/brokered out of band by the caller (Cloud
  resets or brokers it).

**Response** `200`:

```json
{ "address": "alice@vulos.to", "created": true }
```

`created` is `false` when the mailbox already existed (idempotent re-provision).

**The self-provision seam.** Provisioning is delegated to the active identity
provider:

- **Standalone** (local file-backed identity, the default): the mailbox is created
  locally.
- **Vulos Cloud control plane** (`VULOS_CP_URL` set): provisioning is brokered to
  the control plane's mail-signup endpoint.
- **A provider that does not own account creation** returns `seam.ErrUnsupported`;
  this endpoint then responds `501 Not Implemented` with a clear message, signalling
  the caller to provision the mailbox through that external system instead.

Errors: `400` (invalid localpart / body), `401` (broker auth), `405` (non-POST),
`501` (server cannot self-provision), `502` (provisioning backend error).
