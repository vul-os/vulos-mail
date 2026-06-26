/**
 * mockClient.js — in-memory /v1 stand-in for the standalone demo / screenshots.
 * Mirrors createMailClient() (incl. moveMessage + hard-delete) so the full
 * Gmail-class <MailApp/> works with zero backend. Seeded with threads, unread,
 * starred, attachments, multiple folders, contacts and calendar events.
 */
const H = 3600e3
const D = 24 * H
const ago = (ms) => new Date(Date.now() - ms).toISOString()

const inbox = [
  {
    id: '1010', from: 'maya@startup.co', fromName: 'Maya Chen', to: 'me@vulos.org', cc: 'team@startup.co',
    subject: 'Onboarding call recap + next steps',
    preview: 'Great call today! Recapping the key decisions: launch moves to July 14, pricing stays at $29/mo for beta, docs sprint starts Monday.',
    html: '<p>Great call today! Recapping the key decisions:</p><ol><li>Launch date moved to <strong>July 14</strong></li><li>Pricing stays at $29/mo for beta</li><li>Docs sprint starts Monday</li></ol><p>Talk soon,<br>Maya</p>',
    date: ago(0.6 * H), flags: [], messageId: '<onb-1@startup.co>',
  },
  {
    id: '1009', from: 'alice@vulos.org', fromName: 'Alice Mokoena', to: 'me@vulos.org',
    subject: 'Re: Product roadmap Q3 — feedback welcome',
    preview: 'Thanks for sharing the draft. I left comments on sections 2 and 4. The timeline looks ambitious but achievable.',
    html: '<p>Thanks for sharing the draft. I left comments on sections 2 and 4.</p><p>The timeline looks ambitious but achievable if we front-load the infra work. Sync Thursday — does 14:00 UTC work?</p><p>– Alice</p>',
    date: ago(2 * H), flags: ['\\Flagged'],
    messageId: '<road-3@vulos.org>', inReplyTo: '<road-2@vulos.org>',
    references: ['<road-1@vulos.org>', '<road-2@vulos.org>'],
  },
  {
    id: '1008', from: 'me@vulos.org', fromName: 'Me', to: 'alice@vulos.org',
    subject: 'Re: Product roadmap Q3 — feedback welcome',
    preview: 'Sharing the Q3 roadmap draft — would love your thoughts on the timeline before we present to the board.',
    html: '<p>Sharing the Q3 roadmap draft — would love your thoughts on the timeline before we present to the board.</p>',
    date: ago(5 * H), flags: ['\\Seen'],
    messageId: '<road-2@vulos.org>', inReplyTo: '<road-1@vulos.org>', references: ['<road-1@vulos.org>'],
  },
  {
    id: '1007', from: 'alice@vulos.org', fromName: 'Alice Mokoena', to: 'me@vulos.org',
    subject: 'Product roadmap Q3 — feedback welcome',
    preview: 'Hi team, attaching the Q3 roadmap draft. Please review sections 2–4 and share feedback by Friday.',
    html: '<p>Hi team, attaching the Q3 roadmap draft. Please review sections 2–4 and share feedback by Friday.</p>',
    date: ago(8 * H), flags: ['\\Seen'], messageId: '<road-1@vulos.org>',
  },
  {
    id: '1006', from: 'noreply@github.com', fromName: 'GitHub', to: 'me@vulos.org',
    subject: '[vulos/mail-ui] PR #42: Gmail-class webmail',
    preview: 'imranparuk opened a pull request. A full Gmail-class three-pane webmail with threading, multi-select and keyboard shortcuts.',
    html: '<p><strong>imranparuk</strong> opened pull request #42</p><p>A full Gmail-class three-pane webmail with threading, multi-select and keyboard shortcuts.</p><p>Changes: +3120 −876</p>',
    date: ago(5 * H), flags: [], messageId: '<gh-42@github.com>',
  },
  {
    id: '1005', from: 'invoice@stripe.com', fromName: 'Stripe', to: 'me@vulos.org',
    subject: 'Your invoice — $49.00 due',
    preview: 'Invoice INV-2026-0614. Amount due: $49.00 USD. Due date: 30 June 2026.',
    html: '<p>Invoice <strong>INV-2026-0614</strong></p><p>Amount due: $49.00 USD<br>Due date: 30 June 2026</p>',
    date: ago(18 * H), flags: [], hasAttachments: true, messageId: '<inv-0614@stripe.com>',
    attachments: [{ id: '1005/1', filename: 'invoice-INV-2026-0614.pdf', contentType: 'application/pdf', size: 84320 }],
  },
  {
    id: '1004', from: 'bob@designco.io', fromName: 'Bob Osei', to: 'me@vulos.org',
    subject: 'Moodboard for the new landing page',
    preview: 'Hey! Attached are three concept directions for the hero section. Leaning toward option B (the gradient mesh).',
    html: '<p>Hey!</p><p>Attached are three concept directions for the hero section. Leaning toward option B (the gradient mesh).</p><p>Cheers,<br>Bob</p>',
    date: ago(2 * D), flags: ['\\Seen'], hasAttachments: true, messageId: '<mood-1@designco.io>',
    attachments: [
      { id: '1004/1', filename: 'hero-concept-A.png', contentType: 'image/png', size: 512000 },
      { id: '1004/2', filename: 'hero-concept-B.png', contentType: 'image/png', size: 489000 },
    ],
  },
  {
    id: '1003', from: 'security@accounts.google.com', fromName: 'Google', to: 'me@vulos.org',
    subject: 'Security alert: new sign-in on macOS',
    preview: 'Your account was just signed in to from macOS. If this was you, you can ignore this message.',
    html: '<p>Your account was just signed in to from macOS.</p><p>If this was you, you can ignore this message.</p>',
    date: ago(3 * D), flags: ['\\Seen'], messageId: '<sec-1@google.com>',
  },
  {
    id: '1002', from: 'team@linear.app', fromName: 'Linear', to: 'me@vulos.org',
    subject: 'ENG-419 was closed: IMAP IDLE reconnect drops',
    preview: 'Issue ENG-419 — Investigate IMAP IDLE reconnect drops — was closed by imranparuk.',
    html: '<p>Issue <strong>ENG-419</strong> — Investigate IMAP IDLE reconnect drops — was closed by imranparuk.</p>',
    date: ago(4 * D), flags: ['\\Seen'], messageId: '<lin-419@linear.app>',
  },
  {
    id: '1001', from: 'newsletter@techdigest.io', fromName: 'Tech Digest', to: 'me@vulos.org',
    subject: 'This week in open source: Go 1.24, the SFU debate, HTMX hits 30k',
    preview: 'Go 1.24 ships with range-over-func and improved PGO. HTMX crosses 30k GitHub stars. Plus: why SSE is back.',
    html: '<p>Go 1.24 ships with range-over-func and improved PGO. HTMX crosses 30k GitHub stars. Plus: why SSE is back in fashion.</p>',
    date: ago(6 * D), flags: ['\\Seen'], messageId: '<td-w24@techdigest.io>',
  },
]

const sent = [
  {
    id: '2001', from: 'me@vulos.org', fromName: 'Me', to: 'bob@designco.io',
    subject: 'Re: Moodboard for the new landing page',
    preview: 'Option B all the way — the gradient mesh feels modern without being too trendy.',
    html: '<p>Option B all the way — the gradient mesh feels modern without being too trendy.</p>',
    date: ago(1.5 * D), flags: ['\\Seen'], messageId: '<mood-r1@vulos.org>',
  },
]

const drafts = [
  {
    id: '3001', from: 'me@vulos.org', fromName: 'Me', to: 'team@startup.co',
    subject: 'Sprint planning notes — week of June 16',
    preview: 'Capturing the key points from today’s planning. Still working through the acceptance criteria…',
    html: '<p>Capturing the key points from today’s planning. Still working through the acceptance criteria…</p>',
    date: ago(0.5 * H), flags: ['\\Draft', '\\Seen'], messageId: '<draft-1@vulos.org>',
  },
]

const archive = [
  {
    id: '4001', from: 'noreply@status.io', fromName: 'Statuspage', to: 'me@vulos.org',
    subject: 'Resolved: elevated API latency',
    preview: 'The incident affecting API latency has been resolved.',
    html: '<p>The incident affecting API latency has been resolved.</p>',
    date: ago(9 * D), flags: ['\\Seen'], messageId: '<stat-1@status.io>',
  },
]

const FOLDERS = () => ({
  INBOX: inbox.map(clone),
  Sent: sent.map(clone),
  Drafts: drafts.map(clone),
  Archive: archive.map(clone),
  Trash: [],
})

const clone = (m) => ({ ...m, flags: [...(m.flags || [])] })

const now = new Date()
const at = (dayOffset, hour) =>
  new Date(now.getFullYear(), now.getMonth(), now.getDate() + dayOffset, Math.floor(hour), (hour % 1) * 60).toISOString()

const calSeed = [
  { uid: 'e1', summary: 'Standup', start: at(0, 9), end: at(0, 9.5), location: 'Jitsi' },
  { uid: 'e2', summary: 'Roadmap sync w/ Alice', start: at(0, 14), end: at(0, 15) },
  { uid: 'e3', summary: 'Design review', start: at(1, 11), end: at(1, 12) },
  { uid: 'e4', summary: '1:1 with Maya', start: at(2, 16), end: at(2, 16.5) },
  { uid: 'e5', summary: 'Release v1.2', start: at(4, 11), end: at(4, 12), location: 'War room' },
  { uid: 'e6', summary: 'Company offsite', start: at(7, 0), end: at(8, 0), allDay: true },
]

const contactSeed = [
  { email: 'alice@vulos.org', name: 'Alice Mokoena' },
  { email: 'bob@designco.io', name: 'Bob Osei' },
  { email: 'maya@startup.co', name: 'Maya Chen' },
  { email: 'team@vulos.org', name: 'Vulos Team' },
  { email: 'security@vulos.org', name: 'Security' },
  { email: 'imran@vulos.org', name: 'Imran Paruk' },
  { email: 'nadia@vulos.org', name: 'Nadia Khan' },
  { email: 'sipho@vulos.org', name: 'Sipho Dlamini' },
]

export function createMockClient() {
  const store = FOLDERS()
  const find = (folder, uid) => (store[folder] || []).find((m) => m.id === uid)

  return {
    me: async () => ({ email: 'me@vulos.org', username: 'me' }),
    listFolders: async () => [
      { path: 'INBOX', name: 'INBOX', attributes: ['\\Inbox'], unread: store.INBOX.filter((m) => !m.flags.includes('\\Seen')).length },
      { path: 'Sent', name: 'Sent', attributes: ['\\Sent'] },
      { path: 'Drafts', name: 'Drafts', attributes: ['\\Drafts'], unread: store.Drafts.length },
      { path: 'Archive', name: 'Archive', attributes: ['\\Archive'] },
      { path: 'Trash', name: 'Trash', attributes: ['\\Trash'] },
    ],
    listMessages: async ({ folder = 'INBOX' } = {}) => (store[folder] || []).map(clone),
    getMessage: async (uid, { folder = 'INBOX' } = {}) => {
      for (const f of Object.keys(store)) { const m = find(f, uid); if (m) return clone(m) }
      return { ...(find(folder, uid) || {}) }
    },
    search: async (q, { folder = 'INBOX' } = {}) => {
      const t = q.toLowerCase()
      const pool = folder ? (store[folder] || []) : Object.values(store).flat()
      return pool.filter((m) => (m.subject + m.preview + m.from + (m.fromName || '')).toLowerCase().includes(t)).map(clone)
    },
    setFlag: async (uid, flag, add, { folder = 'INBOX' } = {}) => {
      const m = find(folder, uid) || Object.values(store).flat().find((x) => x.id === uid)
      if (m) { const f = new Set(m.flags); add ? f.add(flag) : f.delete(flag); m.flags = [...f] }
      return null
    },
    deleteMessage: async (uid, { folder = 'INBOX', hard = false } = {}) => {
      const list = store[folder] || []
      const i = list.findIndex((m) => m.id === uid)
      if (i >= 0) { const [m] = list.splice(i, 1); if (!hard && folder !== 'Trash') store.Trash.unshift(m) }
      return null
    },
    moveMessage: async (uid, toFolder, { folder = 'INBOX' } = {}) => {
      const list = store[folder] || []
      const i = list.findIndex((m) => m.id === uid)
      if (i >= 0 && store[toFolder]) { const [m] = list.splice(i, 1); store[toFolder].unshift(m) }
      return null
    },
    sendMessage: async (draft) => { console.log('demo send', draft); return { sent: true } },
    saveDraft: async (draft) => { console.log('demo draft', draft); return { saved: true } },
    listEvents: async () => calSeed.map((e) => ({ ...e })),
    createEvent: async (e) => { calSeed.push({ ...e, uid: 'e' + (calSeed.length + 1) }); return { created: true } },
    deleteEvent: async () => null,
    freeBusy: async () => calSeed.filter((e) => !e.allDay).map(({ start, end }) => ({ start, end })),
    listContacts: async ({ q = '' } = {}) =>
      contactSeed.filter((c) => (c.name + c.email).toLowerCase().includes(q.toLowerCase())),
  }
}
