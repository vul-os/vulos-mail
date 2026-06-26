/**
 * threading.js — client-side conversation grouping (a pragmatic JWZ).
 *
 * lilmail's /v1 returns a flat Email[]; Gmail groups those into conversations.
 * We union messages that reference one another via Message-ID / In-Reply-To /
 * References, producing Thread objects the list + reading pane render.
 */
import { FLAG_SEEN, FLAG_FLAGGED } from '../api.js'

const hasFlag = (m, f) => Array.isArray(m.flags) && m.flags.includes(f)

/** Stable key for a message (its Message-ID, else a synthetic uid key). */
function keyOf(m) {
  return m.messageId || 'uid:' + m.id
}

/** Normalise a subject for fallback grouping (strip Re:/Fwd: noise). */
export function normalizeSubject(s = '') {
  let out = s
  let prev
  do { prev = out; out = out.replace(/^\s*(re|fwd|fw|aw)\s*:\s*/i, '') } while (out !== prev)
  return out.trim().toLowerCase()
}

/**
 * Group a flat Email[] into Thread[] (newest-active first).
 *
 * @param {Array} messages
 * @param {object} [opts]
 * @param {boolean} [opts.threaded=true] - when false, every message is its own thread.
 * @returns {Array<Thread>}
 */
export function groupThreads(messages = [], { threaded = true } = {}) {
  if (!threaded) {
    return messages
      .map((m) => buildThread([m]))
      .sort((a, b) => b.ts - a.ts)
  }

  // Union-find over message keys + every id they reference.
  const parent = new Map()
  const find = (x) => {
    if (!parent.has(x)) parent.set(x, x)
    let r = x
    while (parent.get(r) !== r) r = parent.get(r)
    while (parent.get(x) !== r) { const n = parent.get(x); parent.set(x, r); x = n }
    return r
  }
  const union = (a, b) => { const ra = find(a), rb = find(b); if (ra !== rb) parent.set(ra, rb) }

  for (const m of messages) {
    const k = keyOf(m)
    find(k)
    const refs = [...(m.references || []), m.inReplyTo].filter(Boolean)
    let prev = k
    for (const r of refs) { union(prev, r); prev = r }
  }

  // Subject fallback: link messages that share a normalised subject when neither
  // carries threading headers (common with mailing lists / forwards).
  const bySubject = new Map()
  for (const m of messages) {
    const subj = normalizeSubject(m.subject)
    if (!subj) continue
    if (bySubject.has(subj)) union(keyOf(m), keyOf(bySubject.get(subj)))
    else bySubject.set(subj, m)
  }

  const groups = new Map()
  for (const m of messages) {
    const root = find(keyOf(m))
    if (!groups.has(root)) groups.set(root, [])
    groups.get(root).push(m)
  }

  return [...groups.values()].map(buildThread).sort((a, b) => b.ts - a.ts)
}

/** Build a Thread descriptor from a set of messages. */
function buildThread(msgs) {
  const sorted = [...msgs].sort((a, b) => ts(a) - ts(b))
  // True root = the conversation starter (no In-Reply-To), else the earliest.
  const root = sorted.find((m) => !m.inReplyTo) || sorted[0]
  const latest = sorted[sorted.length - 1]
  const participants = []
  const seen = new Set()
  for (const m of sorted) {
    const label = m.fromName || m.from || ''
    const k = (m.from || label).toLowerCase()
    if (label && !seen.has(k)) { seen.add(k); participants.push({ name: label, email: m.from }) }
  }
  return {
    id: latest.id,                 // open by the latest message's uid
    ids: sorted.map((m) => m.id),
    messages: sorted,
    count: sorted.length,
    root,
    latest,
    from: latest.from,
    fromName: latest.fromName,
    subject: root.subject || latest.subject,
    preview: latest.preview,
    date: latest.date,
    ts: ts(latest),
    participants,
    hasAttachments: sorted.some((m) => m.hasAttachments),
    unread: sorted.some((m) => !hasFlag(m, FLAG_SEEN)),
    starred: sorted.some((m) => hasFlag(m, FLAG_FLAGGED)),
  }
}

function ts(m) {
  const t = new Date(m.date).getTime()
  return Number.isNaN(t) ? 0 : t
}
