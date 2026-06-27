import { useEffect, useState } from "react";
import { Icon } from "@vulos/mail-ui";

// AccountSettings — the standalone, self-hosted account surface, injected into
// the shared mail UI's Settings panel via <MailApp settingsExtra>. Everything
// here is backed by the vulos-mail server's /api/webmail endpoints and degrades
// to only what the server reports it supports (server-honest): identity + sign
// out, the IMAP/SMTP connection settings for an external client, and — when the
// deployment owns identity locally — an in-place change-password form.
export default function AccountSettings({ onLogout }) {
  const [acct, setAcct] = useState(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    let live = true;
    fetch("/api/webmail/account", { credentials: "include" })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error("Could not load account"))))
      .then((a) => live && setAcct(a))
      .catch((e) => live && setErr(e.message));
    return () => { live = false; };
  }, []);

  if (err) return <Frame><p className="vm-set-desc">{err}</p></Frame>;
  if (!acct) return <Frame><p className="vm-set-desc">Loading…</p></Frame>;

  const initial = (acct.email?.[0] || "?").toUpperCase();
  const caps = acct.capabilities || {};

  return (
    <>
      <section className="vm-set-section">
        <h3 className="vm-set-section-title">Account</h3>
        <div className="vm-set-group vm-acct">
          <div className="vm-acct-card">
            <span className="vm-acct-avatar" aria-hidden="true">{initial}</span>
            <span className="vm-acct-who">
              <span className="vm-acct-email">{acct.email}</span>
              <span className="vm-acct-sub">Signed in{acct.domain ? ` · ${acct.domain}` : ""}</span>
            </span>
          </div>
          <button type="button" className="vm-btn vm-btn-ghost vm-btn-block" onClick={onLogout}>
            <Icon name="logout" /> Sign out
          </button>
        </div>
      </section>

      <section className="vm-set-section">
        <h3 className="vm-set-section-title">Mail client setup</h3>
        <div className="vm-set-group vm-acct">
          <ConnRows label="IMAP" conn={acct.imap} />
          <ConnRows label="SMTP" conn={acct.smtp} />
          <p className="vm-acct-note">
            <Icon name="server" />
            Use these settings with your mailbox password to connect Thunderbird, Apple Mail, K-9 or any IMAP client.
          </p>
        </div>
      </section>

      {caps.apps && (
        <section className="vm-set-section">
          <h3 className="vm-set-section-title">Apps &amp; bots</h3>
          <div className="vm-set-group vm-acct">
            <p className="vm-acct-note">
              <Icon name="server" />
              Install and manage apps &amp; bots that read and act on your mail via the lilmail /v1 API.
            </p>
            <a className="vm-btn vm-btn-ghost vm-btn-block" href="/apps">
              <Icon name="settings" /> Manage apps &amp; bots
            </a>
          </div>
        </section>
      )}

      {caps.changePassword && <ChangePassword />}
    </>
  );
}

function Frame({ children }) {
  return (
    <section className="vm-set-section">
      <h3 className="vm-set-section-title">Account</h3>
      <div className="vm-set-group">{children}</div>
    </section>
  );
}

function ConnRows({ label, conn }) {
  if (!conn) return null;
  return (
    <>
      <div className="vm-kv">
        <span className="vm-kv-key">{label} server</span>
        <span className="vm-kv-val" title={conn.host}>{conn.host}</span>
        <CopyBtn text={conn.host} label={`Copy ${label} server`} />
      </div>
      <div className="vm-kv">
        <span className="vm-kv-key">{label} port</span>
        <span className="vm-kv-val">{conn.port}{conn.security ? ` · ${conn.security}` : ""}</span>
        <CopyBtn text={conn.port} label={`Copy ${label} port`} />
      </div>
    </>
  );
}

function CopyBtn({ text, label }) {
  const [ok, setOk] = useState(false);
  return (
    <button
      type="button"
      className={"vm-copybtn" + (ok ? " vm-ok" : "")}
      aria-label={label}
      title={label}
      onClick={async () => {
        try {
          await navigator.clipboard?.writeText(String(text ?? ""));
          setOk(true);
          setTimeout(() => setOk(false), 1400);
        } catch { /* clipboard blocked — no-op */ }
      }}
    >
      <Icon name={ok ? "check" : "copy"} />
    </button>
  );
}

function ChangePassword() {
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [show, setShow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState(null); // { ok: boolean, text: string }

  async function submit(e) {
    e.preventDefault();
    setMsg(null);
    if (next.length < 8) { setMsg({ ok: false, text: "New password must be at least 8 characters." }); return; }
    if (next !== confirm) { setMsg({ ok: false, text: "New passwords do not match." }); return; }
    setBusy(true);
    try {
      const r = await fetch("/api/webmail/account/password", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ currentPassword: cur, newPassword: next }),
      });
      if (!r.ok) {
        const j = await r.json().catch(() => ({}));
        throw new Error(j.error || "Could not change password");
      }
      setMsg({ ok: true, text: "Password changed." });
      setCur(""); setNext(""); setConfirm("");
    } catch (ex) {
      setMsg({ ok: false, text: ex.message });
    } finally {
      setBusy(false);
    }
  }

  const type = show ? "text" : "password";
  return (
    <section className="vm-set-section">
      <h3 className="vm-set-section-title">Password</h3>
      <form className="vm-set-group vm-acct" onSubmit={submit}>
        <label className="vm-field">
          <span>Current password</span>
          <input className="vm-input" type={type} autoComplete="current-password"
            value={cur} onChange={(e) => setCur(e.target.value)} required />
        </label>
        <label className="vm-field">
          <span>New password</span>
          <div className="vm-input-wrap">
            <input className="vm-input" type={type} autoComplete="new-password" minLength={8}
              value={next} onChange={(e) => setNext(e.target.value)} required />
            <button type="button" className="vm-input-reveal"
              aria-label={show ? "Hide passwords" : "Show passwords"} title={show ? "Hide" : "Show"}
              onClick={() => setShow((v) => !v)}>
              <Icon name={show ? "eyeoff" : "eye"} />
            </button>
          </div>
        </label>
        <label className="vm-field">
          <span>Confirm new password</span>
          <input className="vm-input" type={type} autoComplete="new-password" minLength={8}
            value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
        </label>
        {msg && <div className={"vm-acct-msg " + (msg.ok ? "vm-ok" : "vm-err")} role="status">{msg.text}</div>}
        <button type="submit" className="vm-btn vm-btn-primary vm-btn-block" disabled={busy}>
          <Icon name="key" /> {busy ? "Changing…" : "Change password"}
        </button>
      </form>
    </section>
  );
}
