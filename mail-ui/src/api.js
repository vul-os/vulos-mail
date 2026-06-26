/**
 * api.js — typed-ish JS client for the lilmail JSON API (`/v1`).
 *
 * Contract: see lilmail/docs/API.md. Session-cookie auth (credentials are
 * always included). 401 responses return JSON `{ error }`; this client surfaces
 * them as an ApiError with `.status === 401` so the UI can react in code.
 *
 * Folders ride as the `?folder=` query param (default INBOX). UIDs are numeric
 * path segments. Flag/delete return 204 (no body).
 */

const DEFAULT_FOLDER = 'INBOX'

/** Error thrown for any non-2xx response, carrying the HTTP status. */
export class ApiError extends Error {
  constructor(message, status) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

/**
 * Create a mail API client bound to a base URL.
 *
 * @param {object} [opts]
 * @param {string} [opts.baseUrl='/v1'] - origin + prefix, e.g. '/v1' (same
 *   origin) or 'https://mail.example.com/v1'. Trailing slash is trimmed.
 * @param {typeof fetch} [opts.fetch] - fetch impl override (tests / SSR).
 */
export function createMailClient(opts = {}) {
  const baseUrl = (opts.baseUrl ?? '/v1').replace(/\/$/, '')
  const fetchImpl = opts.fetch ?? globalThis.fetch

  /** Build a full URL for a path + query object (omits undefined/empty values). */
  function buildUrl(path, query) {
    let url = baseUrl + path
    if (query) {
      const qs = new URLSearchParams()
      for (const [k, v] of Object.entries(query)) {
        if (v === undefined || v === null || v === '') continue
        qs.set(k, String(v))
      }
      const s = qs.toString()
      if (s) url += '?' + s
    }
    return url
  }

  async function request(path, { query, method = 'GET', body } = {}) {
    const init = { method, credentials: 'include', headers: {} }
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json'
      init.body = JSON.stringify(body)
    }
    const res = await fetchImpl(buildUrl(path, query), init)
    if (res.status === 204) return null
    if (!res.ok) {
      const payload = await res.json().catch(() => ({}))
      throw new ApiError(payload.error || res.statusText || 'request failed', res.status)
    }
    // Some success responses (204 handled above) always carry JSON.
    return res.json()
  }

  return {
    baseUrl,
    buildUrl,

    /** GET /v1/me → { email, username } */
    me() {
      return request('/me')
    },

    /** GET /v1/folders → MailboxInfo[] */
    async listFolders() {
      const data = await request('/folders')
      return data.folders ?? []
    },

    /** GET /v1/messages?folder=&limit= → Email[] */
    async listMessages({ folder = DEFAULT_FOLDER, limit = 50 } = {}) {
      const data = await request('/messages', { query: { folder, limit } })
      return data.messages ?? []
    },

    /** GET /v1/messages/:uid?folder= → Email */
    getMessage(uid, { folder = DEFAULT_FOLDER } = {}) {
      return request(`/messages/${encodeURIComponent(uid)}`, { query: { folder } })
    },

    /** GET /v1/search?folder=&q=&limit= → Email[] */
    async search(q, { folder = DEFAULT_FOLDER, limit = 100 } = {}) {
      const data = await request('/search', { query: { folder, q, limit } })
      return data.messages ?? []
    },

    /** PATCH /v1/messages/:uid/flags?folder= body {flag, add} → 204 */
    setFlag(uid, flag, add, { folder = DEFAULT_FOLDER } = {}) {
      return request(`/messages/${encodeURIComponent(uid)}/flags`, {
        method: 'PATCH',
        query: { folder },
        body: { flag, add: !!add },
      })
    },

    /** DELETE /v1/messages/:uid?folder= → 204 */
    deleteMessage(uid, { folder = DEFAULT_FOLDER } = {}) {
      return request(`/messages/${encodeURIComponent(uid)}`, {
        method: 'DELETE',
        query: { folder },
      })
    },

    /**
     * POST /v1/messages — send a message.
     * @param {{to,cc?,bcc?,subject,text?,html?,inReplyTo?}} draft
     */
    sendMessage(draft) {
      return request('/messages', { method: 'POST', body: draft })
    },

    /** POST /v1/drafts — save a draft. Same body shape as sendMessage. */
    saveDraft(draft) {
      return request('/drafts', { method: 'POST', body: draft })
    },

    // ── Calendar (requires lilmail [caldav] enabled) ──────────────────────

    /** GET /v1/calendar/events?start=&end= → CalendarEvent[] */
    async listEvents({ start, end } = {}) {
      const data = await request('/calendar/events', { query: { start: iso(start), end: iso(end) } })
      return data.events ?? []
    },

    /** POST /v1/calendar/events → { created } */
    createEvent(event) {
      return request('/calendar/events', {
        method: 'POST',
        body: { ...event, start: iso(event.start), end: iso(event.end) },
      })
    },

    /** DELETE /v1/calendar/events/:uid → 204 */
    deleteEvent(uid) {
      return request(`/calendar/events/${encodeURIComponent(uid)}`, { method: 'DELETE' })
    },

    /** GET /v1/calendar/freebusy?start=&end= → { start, end }[] */
    async freeBusy({ start, end } = {}) {
      const data = await request('/calendar/freebusy', { query: { start: iso(start), end: iso(end) } })
      return data.busy ?? []
    },

    // ── Contacts (requires lilmail [carddav] enabled) ─────────────────────

    /** GET /v1/contacts?q=&limit= → { email, name }[] */
    async listContacts({ q = '', limit } = {}) {
      const data = await request('/contacts', { query: { q, limit } })
      return data.contacts ?? []
    },
  }
}

/** Coerce a Date | ISO string | undefined to an RFC 3339 string (or undefined). */
function iso(v) {
  if (v == null || v === '') return undefined
  if (v instanceof Date) return v.toISOString()
  return String(v)
}

// IMAP system flags used across the UI.
export const FLAG_SEEN = '\\Seen'
export const FLAG_FLAGGED = '\\Flagged'

export default createMailClient
