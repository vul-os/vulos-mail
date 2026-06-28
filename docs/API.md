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

---

## `POST /api/admin/import/contacts`

Broker-gated. **Bulk contacts import** endpoint called by the Vulos OS import
engine when a user runs a Contacts import job. The OS pulls contacts from the
connected provider (Google Contacts via People API, Microsoft Contacts via Graph),
maps them to vCard format, and POSTs them here so vulos-mail stores
**owned copies that persist after the integration is disconnected or upstream
entries are deleted**.

### Idempotency

Contacts are keyed by their vCard `UID` field. Re-submitting the same UID
overwrites the stored copy — the store's ETag is recomputed from the new bytes.
If the bytes are identical the result is a no-op. This makes the endpoint safe to
call multiple times for the same contact (additive-only re-pull from the OS).

### Never deletes

The endpoint only **adds or updates** entries. The OS caller never sends a
delete instruction; Vulos-owned copies are never removed when the upstream
contact is deleted at the provider.

**Request**

```
POST /api/admin/import/contacts
X-Vulos-Broker-Auth: <LILMAIL_BROKER_SECRET>
Content-Type: application/json
```

```json
{
  "account": "alice@vulos.to",
  "vcards": [
    "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:abc-123\r\nFN:Bob Smith\r\nEMAIL:bob@example.com\r\nEND:VCARD\r\n",
    "BEGIN:VCARD\r\n..."
  ]
}
```

- `account` (required): the CardDAV account (email address) to import into.
- `vcards` (required): array of raw vCard strings (version 3.0 or 4.0). Each must
  contain a `UID` field; vCards without a UID are counted as errors and skipped.

**Response** `202`:

```json
{ "imported": 47, "errors": 0 }
```

- `imported`: number of vCards successfully stored.
- `errors`: vCards that were malformed or lacked a UID (logged at debug level).

Errors: `400` (missing account / bad JSON), `401` (broker auth), `405` (non-POST).

---

## `POST /api/admin/import/events`

Broker-gated. **Bulk calendar import** endpoint called by the Vulos OS import
engine when a user runs a Calendar import job. The OS pulls events from the
connected provider (Google Calendar API, Microsoft Graph Calendar), maps them to
iCalendar format, and POSTs them here so vulos-mail stores
**owned copies that persist after the integration is disconnected or upstream
events are deleted**.

### Idempotency

Events are keyed by the `UID` property of their `VEVENT` component.
Re-submitting the same UID overwrites the stored copy (safe re-pull). The OS
additive-only sync never sends a delete instruction.

**Request**

```
POST /api/admin/import/events
X-Vulos-Broker-Auth: <LILMAIL_BROKER_SECRET>
Content-Type: application/json
```

```json
{
  "account": "alice@vulos.to",
  "events": [
    "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//vulos//import//EN\r\nBEGIN:VEVENT\r\nUID:evt-001\r\nSUMMARY:Team sync\r\nDTSTART:20260628T100000Z\r\nDTEND:20260628T110000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
    "BEGIN:VCALENDAR\r\n..."
  ]
}
```

- `account` (required): the CalDAV account (email address) to import into.
- `events` (required): array of raw iCalendar strings, one `VCALENDAR` per
  element (each wrapping one `VEVENT`). Events without a `UID` are skipped.

**Response** `202`:

```json
{ "imported": 312, "errors": 2 }
```

Errors: `400` (missing account / bad JSON), `401` (broker auth), `405` (non-POST).
