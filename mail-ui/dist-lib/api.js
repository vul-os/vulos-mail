const u = "INBOX";
class y extends Error {
  constructor(l, f) {
    super(l), this.name = "ApiError", this.status = f;
  }
}
function g(a = {}) {
  const l = (a.baseUrl ?? "/v1").replace(/\/$/, ""), f = a.fetch ?? globalThis.fetch;
  function m(e, t) {
    let s = l + e;
    if (t) {
      const r = new URLSearchParams();
      for (const [o, c] of Object.entries(t))
        c == null || c === "" || r.set(o, String(c));
      const d = r.toString();
      d && (s += "?" + d);
    }
    return s;
  }
  async function n(e, { query: t, method: s = "GET", body: r } = {}) {
    const d = { method: s, credentials: "include", headers: {} };
    r !== void 0 && (d.headers["Content-Type"] = "application/json", d.body = JSON.stringify(r));
    const o = await f(m(e, t), d);
    if (o.status === 204) return null;
    if (!o.ok) {
      const c = await o.json().catch(() => ({}));
      throw new y(c.error || o.statusText || "request failed", o.status);
    }
    return o.json();
  }
  return {
    baseUrl: l,
    buildUrl: m,
    /** GET /v1/me → { email, username } */
    me() {
      return n("/me");
    },
    /** GET /v1/folders → MailboxInfo[] */
    async listFolders() {
      return (await n("/folders")).folders ?? [];
    },
    /** GET /v1/messages?folder=&limit= → Email[] */
    async listMessages({ folder: e = u, limit: t = 50 } = {}) {
      return (await n("/messages", { query: { folder: e, limit: t } })).messages ?? [];
    },
    /** GET /v1/messages/:uid?folder= → Email */
    getMessage(e, { folder: t = u } = {}) {
      return n(`/messages/${encodeURIComponent(e)}`, { query: { folder: t } });
    },
    /** GET /v1/search?folder=&q=&limit= → Email[] */
    async search(e, { folder: t = u, limit: s = 100 } = {}) {
      return (await n("/search", { query: { folder: t, q: e, limit: s } })).messages ?? [];
    },
    /** PATCH /v1/messages/:uid/flags?folder= body {flag, add} → 204 */
    setFlag(e, t, s, { folder: r = u } = {}) {
      return n(`/messages/${encodeURIComponent(e)}/flags`, {
        method: "PATCH",
        query: { folder: r },
        body: { flag: t, add: !!s }
      });
    },
    /**
     * DELETE /v1/messages/:uid?folder=&hard= → 204
     * Default moves to Trash (lilmail branch v1-mail-actions); hard=true expunges.
     */
    deleteMessage(e, { folder: t = u, hard: s = !1 } = {}) {
      return n(`/messages/${encodeURIComponent(e)}`, {
        method: "DELETE",
        query: { folder: t, hard: s ? "true" : void 0 }
      });
    },
    /**
     * POST /v1/messages/:uid/move?folder= body {toFolder} → 204
     * Archive / move to another folder via IMAP MOVE (lilmail v1-mail-actions).
     * Rejects if the endpoint is absent so callers can degrade gracefully.
     */
    moveMessage(e, t, { folder: s = u } = {}) {
      return n(`/messages/${encodeURIComponent(e)}/move`, {
        method: "POST",
        query: { folder: s },
        body: { toFolder: t }
      });
    },
    /**
     * POST /v1/messages — send a message.
     * @param {{to,cc?,bcc?,subject,text?,html?,inReplyTo?}} draft
     */
    sendMessage(e) {
      return n("/messages", { method: "POST", body: e });
    },
    /** POST /v1/drafts — save a draft. Same body shape as sendMessage. */
    saveDraft(e) {
      return n("/drafts", { method: "POST", body: e });
    },
    // ── Calendar (requires lilmail [caldav] enabled) ──────────────────────
    /** GET /v1/calendar/events?start=&end= → CalendarEvent[] */
    async listEvents({ start: e, end: t } = {}) {
      return (await n("/calendar/events", { query: { start: i(e), end: i(t) } })).events ?? [];
    },
    /** POST /v1/calendar/events → { created } */
    createEvent(e) {
      return n("/calendar/events", {
        method: "POST",
        body: { ...e, start: i(e.start), end: i(e.end) }
      });
    },
    /** DELETE /v1/calendar/events/:uid → 204 */
    deleteEvent(e) {
      return n(`/calendar/events/${encodeURIComponent(e)}`, { method: "DELETE" });
    },
    /** GET /v1/calendar/freebusy?start=&end= → { start, end }[] */
    async freeBusy({ start: e, end: t } = {}) {
      return (await n("/calendar/freebusy", { query: { start: i(e), end: i(t) } })).busy ?? [];
    },
    // ── Contacts (requires lilmail [carddav] enabled) ─────────────────────
    /** GET /v1/contacts?q=&limit= → { email, name }[] */
    async listContacts({ q: e = "", limit: t } = {}) {
      return (await n("/contacts", { query: { q: e, limit: t } })).contacts ?? [];
    }
  };
}
function i(a) {
  if (!(a == null || a === ""))
    return a instanceof Date ? a.toISOString() : String(a);
}
const h = "\\Seen", E = "\\Flagged";
export {
  y as ApiError,
  E as FLAG_FLAGGED,
  h as FLAG_SEEN,
  g as createMailClient,
  g as default
};
