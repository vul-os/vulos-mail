// jmap.js — a tiny JMAP (RFC 8620/8621) client for the vulos-mail API.
// HTTP Basic auth; one batched POST per call. No dependencies.
// Ported from the vanilla SPA's webmail/jmap.js, kept logic-identical and
// extended with paginated query + thread fetching for the React rewrite.

const CAPS = ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"];

export class JMAP {
  constructor(base = "") {
    this.base = base.replace(/\/$/, "");
    this.auth = null;
    this.accountId = null;
    this.user = null;
  }

  setAuth(user, pass) {
    this.user = user;
    this.auth = "Basic " + btoa(user + ":" + pass);
  }

  _headers(json) {
    const h = { Authorization: this.auth };
    if (json) h["Content-Type"] = "application/json";
    return h;
  }

  async session() {
    const r = await fetch(this.base + "/jmap/session", { headers: this._headers() });
    if (!r.ok) throw new Error(r.status === 401 ? "Invalid credentials" : "Session failed (" + r.status + ")");
    const s = await r.json();
    this.accountId = s.primaryAccounts && s.primaryAccounts["urn:ietf:params:jmap:mail"];
    this.session_ = s;
    return s;
  }

  // calls: [[name, args, callId], ...] -> [{name, result, callId}, ...]
  async call(methodCalls) {
    const r = await fetch(this.base + "/jmap/api", {
      method: "POST",
      headers: this._headers(true),
      body: JSON.stringify({ using: CAPS, methodCalls }),
    });
    if (!r.ok) throw new Error("API error " + r.status);
    const body = await r.json();
    return body.methodResponses.map(([name, result, callId]) => ({ name, result, callId }));
  }

  async one(name, args) {
    const [resp] = await this.call([[name, { accountId: this.accountId, ...args }, "0"]]);
    if (resp.name === "error") throw new Error(resp.result.type || "method error");
    return resp.result;
  }

  mailboxes() { return this.one("Mailbox/get", {}); }

  // Paginated, optionally thread-collapsed Email/query.
  query(mailboxId, { limit, position = 0, collapseThreads = false } = {}) {
    const args = {
      filter: { inMailbox: mailboxId },
      sort: [{ property: "receivedAt", isAscending: false }],
      position,
      collapseThreads,
    };
    if (limit != null) args.limit = limit;
    return this.one("Email/query", args);
  }

  emails(ids, properties) { return this.one("Email/get", { ids, properties }); }
  set(updates) { return this.one("Email/set", { update: updates }); }
  // TODO(threading): the server does not yet implement Thread/get or honor
  // collapseThreads on Email/query. Once it does, the message list can collapse
  // conversations (pass collapseThreads:true to query()) and the read view can
  // fetch a whole thread here. Until then this returns unknownMethod.
  threads(ids) { return this.one("Thread/get", { ids }); }

  async pushToken() {
    const r = await fetch(this.base + "/api/webmail/pushtoken", { headers: this._headers() });
    if (!r.ok) throw new Error("push token failed");
    return (await r.json()).token;
  }

  async getSettings() {
    const r = await fetch(this.base + "/api/webmail/settings", { headers: this._headers() });
    return r.ok ? r.json() : { signature: "", vacation: { enabled: false } };
  }
  async saveSettings(s) {
    const r = await fetch(this.base + "/api/webmail/settings", { method: "POST", headers: this._headers(true), body: JSON.stringify(s) });
    if (!r.ok) throw new Error("Save failed");
    return r.json();
  }

  async contacts() {
    const r = await fetch(this.base + "/api/webmail/contacts", { headers: this._headers() });
    return r.ok ? r.json() : [];
  }
  async addContact(c) {
    const r = await fetch(this.base + "/api/webmail/contacts", { method: "POST", headers: this._headers(true), body: JSON.stringify(c) });
    if (!r.ok) throw new Error("Add failed");
    return r.json();
  }
  async delContact(id) {
    await fetch(this.base + "/api/webmail/contacts?id=" + encodeURIComponent(id), { method: "DELETE", headers: this._headers() });
  }
  async events() {
    const r = await fetch(this.base + "/api/webmail/calendar", { headers: this._headers() });
    return r.ok ? r.json() : [];
  }
  async addEvent(e) {
    const r = await fetch(this.base + "/api/webmail/calendar", { method: "POST", headers: this._headers(true), body: JSON.stringify(e) });
    if (!r.ok) throw new Error("Add failed");
    return r.json();
  }
  async delEvent(id) {
    await fetch(this.base + "/api/webmail/calendar?id=" + encodeURIComponent(id), { method: "DELETE", headers: this._headers() });
  }

  // Authenticated binary fetch (e.g. attachment download) -> Blob.
  async download(path) {
    const r = await fetch(this.base + path, { headers: this._headers() });
    if (!r.ok) throw new Error("Download failed (" + r.status + ")");
    return r.blob();
  }

  // Compose+send via the authenticated webmail endpoint.
  async send(msg) {
    const r = await fetch(this.base + "/api/webmail/send", {
      method: "POST",
      headers: this._headers(true),
      body: JSON.stringify(msg),
    });
    if (!r.ok) throw new Error("Send failed (" + r.status + "): " + (await r.text()).trim());
    return r.json().catch(() => ({}));
  }
}
