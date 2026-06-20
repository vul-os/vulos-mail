# vulos-mail — design

A next-generation mail system: ultra-efficient, Gmail-class feature set, built so that
**one clean domain model is the source of truth and every protocol (JMAP/IMAP/SMTP/CalDAV/CardDAV)
is a projection of it.**

This document is the spec. Code conforms to it; when they disagree, fix one of them deliberately.

---

## 1. Non-negotiable principles

1. **Domain model first, protocols as adapters.** The old system let IMAP's constraints (per-folder
   UID/MODSEQ, one-message-one-folder) leak into storage. Here, the internal model is protocol-free;
   IMAP UID/MODSEQ are *computed at the edge*, never stored as truth.
2. **Never fork protocol/crypto.** SMTP/IMAP/MIME/DKIM/SPF/DMARC are consumed as **libraries**
   (`emersion/go-smtp`, `go-imap` v2, `go-message`, `go-msgauth`). We implement their backend
   interfaces; we never absorb their source. This is the single rule that keeps the system
   maintainable.
3. **The log is the account.** Per-account append-only event log is the source of truth. Everything
   else is a rebuildable projection of it.
4. **Labels, not folders.** A message has *N* labels (many-to-many). Inbox, Archive, Star, Important,
   categories, Snooze — all are labels. This is the Gmail shape and it is structural, not bolted on.
5. **Single-writer per account.** Affinity routing gives one writer; the log gives total order;
   a fencing token gives clean failover. **No CRDT.**
6. **Stateless front-ends.** No durable per-node state. Push fans out over a shared event bus. Any
   node serves any client — this is what serves many clients on few resources.
7. **Correctness by invariant, not vibe.** Core invariants are property/fuzz-tested:
   `projection == fold(log)`, replay rebuilds byte-identical state, UID/MODSEQ monotonic at the edge.

---

## 2. Domain model

Protocol-free types (`internal/model`):

- **Message** — immutable. Identified by `MessageID` (internal ULID). Body is an immutable,
  content-addressed, compressed blob in the object store (`BlobRef` = sha256). Carries parsed
  envelope (from/to/cc/subject/date/message-id/in-reply-to/references), size, and a `ThreadID`.
- **Thread** — first-class. Computed from `Message-ID`/`References`/`In-Reply-To` (subject fallback).
- **Label** — a tag with a stable `LabelID`, a display name, and a `Kind`
  (`system` | `user` | `category`). System labels: `inbox`, `archive`, `sent`, `drafts`, `trash`,
  `spam`, `star`, `important`, `snoozed`. A message ↔ label relation is many-to-many.
- **Flag** — per-message booleans that are *not* labels: `seen`, `answered`, `flagged`, `draft`,
  `mdn-sent`. (Star/important are labels, deliberately.)
- **Account** / **Tenant** — Account owns a log + projections + blob namespace. Tenant groups
  accounts and owns quota/abuse policy.

### Folders are a projection, not a model concept
IMAP folders are derived: each system/user label maps to a folder view; a message's folder
membership is "has this label." `\All Mail` = no trash/spam. UID/MODSEQ are assigned per-folder-view
at the IMAP adapter from a deterministic ordering of the log, cached, never stored in the model.

---

## 3. Event log (source of truth)

`internal/event` defines the events; `internal/eventlog` defines the append-only store.

Every mutation is one event, appended under a per-account monotonic `Seq` (the account's MODSEQ
basis). Events are content-stable (deterministic JSON/CBOR) so a log can be hashed and diffed.

Event kinds (v1):
- `MessageIngested{MessageID, BlobRef, Envelope, Size, ThreadID, InitialLabels, InitialFlags}`
- `Labeled{MessageID, LabelID}` / `Unlabeled{MessageID, LabelID}`
- `FlagSet{MessageID, Flag, Value}`
- `LabelCreated{LabelID, Name, Kind}` / `LabelRenamed` / `LabelDeleted`
- `MessageExpunged{MessageID}` (tombstone; blob GC is async + refcounted)
- `ThreadMerged{Into, From}` (rare; for late-arriving references)

Each appended record: `{Seq, Time, ActorID, Event}`. The log is the only writer of `Seq`.
**Invariant:** applying events in `Seq` order to an empty projection yields the canonical state;
this is enforced by tests.

Properties the log buys us for free: replication (ship the log), audit trail, GDPR history &
erasure (rewrite/redact + rebuild), rebuildable/[re-shapeable projections, point-in-time recovery.

---

## 4. Storage layout

- **Blobs:** object store (S3/Tigris; FS for dev). Immutable, content-addressed (`sha256`),
  **zstd-compressed**, deduplicated across the whole system. Hot/cold is a *cache policy*, not a
  storage class.
- **Per-account log + projections:** one embedded store per account (SQLite/DuckDB, WAL), holding:
  - `events` (the log; append-only, PK=Seq)
  - projection tables: `messages`, `labels`, `message_labels` (many-to-many), `flags`, `threads`
  - rebuildable: drop projection tables, replay `events`, get identical state.
  **One store per account — not one per mailbox.** (The old per-mailbox-SQLite design caused 50k
  handles and made global search impossible.)
- **Search:** per-account ranked FTS + attachment-text + **embeddings emitted at ingest**
  (`internal/search`). Search is the primary navigation surface.

---

## 5. Consistency & failover

- **Single-writer per account**, affinity-routed (consistent hash → owner node).
- **Lease + fencing token:** the owner holds a lease; every write carries the fencing token; a
  stale owner's writes are rejected. No split-brain, no CRDT.
- **Failover:** lease expiry → new owner replays the tail of the log → resumes. Because state is
  `fold(log)`, takeover is deterministic and warm-startable.

---

## 6. Service topology

Start as a **modular monolith** (one Go binary, hard package seams); split along seams later.

- `adapters/{jmap,imap,smtp,dav}` — protocol edges over the model.
- `services/ingest` — MX/receive → parse (go-message) → auth (go-msgauth) → abuse → append
  `MessageIngested`.
- `services/mtaout` — outbound: **warm-IP pool, per-tenant DKIM alignment, throttle-aware
  per-destination scheduler (concurrency caps + adaptive 4xx/5xx backoff), reputation/FBL.**
  First-class from day one; this is the deliverability moat.
- `internal/*` — the spine (event, eventlog, blob, model, projection, search, ids).
- Push: shared event bus (NATS/Redis Streams) fans change events to stateless front-ends;
  mobile via JMAP `PushSubscription` → APNs/FCM.

---

## 7. Feature plan (Gmail parity, all fall out of the model)

- **Labels / categories / priority inbox** → label kinds + classifier-emitted labels at ingest.
- **Search** → the FTS+embedding projection; cross-account ranked, attachment-aware, snippets.
- **Conversations** → Thread projection, first-class everywhere.
- **Snooze / scheduled send / undo-send** → events (`Labeled snoozed`, a `scheduled` outbox with a
  wake worker, a hold window before append-to-queue). *Enforced*, unlike the old "accepted but
  ignored" `SendAt`.
- **AI-native:** embeddings + classifications emitted as part of ingest → semantic search, smart
  reply/compose, summarize, priority — all read a projection.
- **Calendar/contacts:** real RRULE expansion, ITIP scheduling, free/busy (not the old stubs).
- **Modern auth:** OAuth2/OIDC, TOTP, WebAuthn/passkeys.
- **Privacy:** remote-image proxy, hide-my-email aliases, confidential/expiring mode.
- **Reused good ideas from the old system** (ported as clean modules, not coupled code):
  URL-safety feeds, outbound ATO auto-suspend, CSAM PDQ+NCMEC, LLM gray-zone phishing,
  tenant registry, age-encrypted BYO queue.

---

## 8. Correctness strategy ("perfection")

- **Property/fuzz** on the spine: any event sequence → deterministic projection; replay rebuilds
  identical bytes; expunge/label/flag converge regardless of interleaving with append.
- **Protocol conformance** suites (IMAP/JMAP) in CI against the adapters.
- **Golden MIME corpus** for parser/threading regression.
- **UID/MODSEQ monotonicity** checked as a property at the IMAP edge.

---

## 9. Repo layout

```
internal/ids         ULID/threadid generation (deterministic in tests)
internal/event       event types + stable codec
internal/eventlog    append-only log (iface + sqlite impl)
internal/blob        object store (iface + fs/s3 impl, zstd, content-addressed)
internal/model       protocol-free domain types
internal/projection  fold(log) -> account index (messages/labels/threads/flags); rebuildable
internal/search      FTS + embeddings projection
adapters/{jmap,imap,smtp,dav}   protocol edges
services/{ingest,mtaout}        receive pipeline; outbound deliverability
cmd/vulos-mail            the binary
docs/DESIGN.md       this file
```

---

## 10. Waves (execution plan)

- **Wave 1 — spec + skeleton.** (this doc + repo) ✔
- **Wave 2 — the spine.** event + eventlog(sqlite) + blob(fs,zstd) + model + projection(fold/rebuild)
  with invariant tests. *Must build & test green offline.*
- **Wave 3 — vertical slice.** ingest one message → label → search → serve over JMAP (read path).
- **Wave 4 — protocol breadth.** SMTP-in (MX) + IMAP adapter as projections.
- **Wave 5 — deliverability.** mtaout: warm-IP pool + per-destination scheduler + reputation.
- **Wave 6 — the rest as modules.** AI, calendar/contacts, auth, privacy, compliance.

Each wave: green build, invariant/conformance tests, no protocol/crypto forks.
