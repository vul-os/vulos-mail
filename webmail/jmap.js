// jmap.js — a tiny JMAP (RFC 8620/8621) client for the vmail API.
// HTTP Basic auth; one batched POST per call. No dependencies.
(function (global) {
  "use strict";

  const CAPS = ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"];

  class JMAP {
    constructor(base = "") {
      this.base = base.replace(/\/$/, "");
      this.auth = null;
      this.accountId = null;
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
    query(mailboxId) { return this.one("Email/query", { filter: { inMailbox: mailboxId } }); }
    emails(ids, properties) { return this.one("Email/get", { ids, properties }); }
    set(updates) { return this.one("Email/set", { update: updates }); }

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

  global.JMAP = JMAP;
})(window);
