/**
 * mockClient.js — in-memory /v1 stand-in for the standalone demo / screenshots.
 * Mirrors the createMailClient() surface so <MailApp/> works with no backend.
 */
const seed = [
  {
    id: '3', from: 'alice@vulos.org', fromName: 'Alice Mokoena', to: 'me@vulos.org',
    subject: 'Welcome to Vulos Mail', preview: 'Your sovereign inbox is ready — here is what is new…',
    body: 'Your sovereign inbox is ready.\n\nEverything runs on your own server.',
    html: '<p>Your <strong>sovereign inbox</strong> is ready. Everything runs on your own server.</p>',
    date: new Date(Date.now() - 3600e3).toISOString(), hasAttachments: false, flags: [],
  },
  {
    id: '2', from: 'team@vulos.org', fromName: 'Vulos Team', to: 'me@vulos.org',
    subject: 'Your weekly digest', preview: 'A summary of activity across your workspace this week.',
    body: 'A summary of activity across your workspace this week.',
    html: '<p>A summary of activity across your workspace this week.</p>',
    date: new Date(Date.now() - 26 * 3600e3).toISOString(), hasAttachments: true, flags: ['\\Seen', '\\Flagged'],
  },
  {
    id: '1', from: 'noreply@example.com', fromName: 'Example', to: 'me@vulos.org',
    subject: 'Invoice #1042', preview: 'Here is the invoice you requested.',
    body: 'Here is the invoice you requested.', html: '<p>Here is the invoice you requested.</p>',
    date: new Date(Date.now() - 72 * 3600e3).toISOString(), hasAttachments: true, flags: ['\\Seen'],
  },
]

const now = new Date()
const at = (dayOffset, hour) =>
  new Date(now.getFullYear(), now.getMonth(), now.getDate() + dayOffset, hour, 0, 0).toISOString()

const calSeed = [
  { uid: 'e1', summary: 'Standup', start: at(0, 9), end: at(0, 9.5 | 0), allDay: false, location: 'Jitsi' },
  { uid: 'e2', summary: 'Design review', start: at(1, 14), end: at(1, 15), allDay: false },
  { uid: 'e3', summary: 'Release', start: at(4, 11), end: at(4, 12), allDay: false, location: 'War room' },
  { uid: 'e4', summary: 'Company offsite', start: at(7, 0), end: at(8, 0), allDay: true },
]

const contactSeed = [
  { email: 'alice@vulos.org', name: 'Alice Mokoena' },
  { email: 'bob@vulos.org', name: 'Bob Nkosi' },
  { email: 'team@vulos.org', name: 'Vulos Team' },
  { email: 'security@vulos.org', name: 'Security' },
]

export function createMockClient() {
  let msgs = seed.map((m) => ({ ...m, flags: [...m.flags] }))
  return {
    me: async () => ({ email: 'me@vulos.org', username: 'me' }),
    listFolders: async () => [
      { path: 'INBOX', name: 'INBOX', unread: 1 },
      { path: 'INBOX/Sent', name: 'Sent' },
      { path: 'INBOX/Archive', name: 'Archive' },
      { path: 'INBOX/Trash', name: 'Trash' },
    ],
    listMessages: async () => msgs.map((m) => ({ ...m })),
    getMessage: async (uid) => ({ ...msgs.find((m) => m.id === uid) }),
    search: async (q) => msgs.filter((m) =>
      (m.subject + m.preview + m.from).toLowerCase().includes(q.toLowerCase())),
    setFlag: async (uid, flag, add) => {
      msgs = msgs.map((m) => {
        if (m.id !== uid) return m
        const f = new Set(m.flags)
        add ? f.add(flag) : f.delete(flag)
        return { ...m, flags: [...f] }
      })
      return null
    },
    deleteMessage: async (uid) => { msgs = msgs.filter((m) => m.id !== uid); return null },
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
