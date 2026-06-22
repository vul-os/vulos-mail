import { useState } from "react";
import { useToast } from "./Toasts.jsx";

export default function Settings({ jmap, settings, setSettings, theme, setTheme, onClose }) {
  const toast = useToast();
  const v = settings.vacation || {};
  const [sig, setSig] = useState(settings.signature || "");
  const [vacEnabled, setVacEnabled] = useState(!!v.enabled);
  const [vacSubj, setVacSubj] = useState(v.subject || "");
  const [vacBody, setVacBody] = useState(v.body || "");
  const [curTheme, setCurTheme] = useState(theme);
  const [busy, setBusy] = useState(false);

  function pickTheme(t) { setCurTheme(t); setTheme(t); }

  async function save() {
    const s = { signature: sig, vacation: { enabled: vacEnabled, subject: vacSubj, body: vacBody } };
    setBusy(true);
    try { await jmap.saveSettings(s); setSettings(s); onClose(); toast("Settings saved"); }
    catch (ex) { toast(ex.message); }
    finally { setBusy(false); }
  }

  return (
    <div className="overlay" id="settings" onClick={(e) => { if (e.target.id === "settings") onClose(); }}>
      <div className="settings-card">
        <h2>Settings</h2>
        <div className="set-theme">
          <span>Appearance</span>
          <div className="seg" id="set-theme">
            <button type="button" data-theme="dark" className={curTheme === "dark" ? "on" : ""} onClick={() => pickTheme("dark")}>Dark</button>
            <button type="button" data-theme="light" className={curTheme === "light" ? "on" : ""} onClick={() => pickTheme("light")}>Light</button>
          </div>
        </div>
        <label className="field"><span>Signature</span>
          <textarea id="set-sig" rows={3} placeholder={"-- \nAlice · Vulos Mail"} value={sig} onChange={(e) => setSig(e.target.value)} />
        </label>
        <div className="set-section">
          <label className="switch">
            <input type="checkbox" id="set-vac" checked={vacEnabled} onChange={(e) => setVacEnabled(e.target.checked)} />
            <span>Vacation responder</span>
          </label>
          <div id="set-vac-fields" className={vacEnabled ? "on" : ""}>
            <label className="field"><span>Subject</span>
              <input id="set-vac-subj" type="text" placeholder="Out of office" value={vacSubj} onChange={(e) => setVacSubj(e.target.value)} />
            </label>
            <label className="field"><span>Message</span>
              <textarea id="set-vac-body" rows={3} placeholder="I'm away until…" value={vacBody} onChange={(e) => setVacBody(e.target.value)} />
            </label>
          </div>
        </div>
        <div className="set-actions">
          <button className="btn btn-ghost" id="set-cancel" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" id="set-save" disabled={busy} onClick={save}>{busy ? "Saving…" : "Save"}</button>
        </div>
      </div>
    </div>
  );
}
