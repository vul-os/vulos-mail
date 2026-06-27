import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { MailApp, Icon } from '../lib/index.js'
import { createMockClient } from './mockClient.js'

// Standalone demo: drives the full Gmail-class <MailApp/> with an in-memory
// client (no backend). Calendar + Contacts are reachable from the in-app side
// rail; a production host omits `client` and talks to /v1 same-origin.
const client = createMockClient()

// A static mock of the standalone webmail's account surface, injected into the
// Settings panel via settingsExtra. Mirrors webmail/src/components/AccountSettings
// (which is backed by /api/webmail/account) so the demo + screenshots show the
// real self-hosted account/connection/password experience with zero backend.
function DemoAccountSettings() {
  return (
    <>
      <section className="vm-set-section">
        <h3 className="vm-set-section-title">Account</h3>
        <div className="vm-set-group vm-acct">
          <div className="vm-acct-card">
            <span className="vm-acct-avatar" aria-hidden="true">M</span>
            <span className="vm-acct-who">
              <span className="vm-acct-email">me@vulos.org</span>
              <span className="vm-acct-sub">Signed in · vulos.org</span>
            </span>
          </div>
          <button type="button" className="vm-btn vm-btn-ghost vm-btn-block"><Icon name="logout" /> Sign out</button>
        </div>
      </section>

      <section className="vm-set-section">
        <h3 className="vm-set-section-title">Mail client setup</h3>
        <div className="vm-set-group vm-acct">
          <div className="vm-kv"><span className="vm-kv-key">IMAP server</span><span className="vm-kv-val">mail.vulos.org</span><CopyIcon /></div>
          <div className="vm-kv"><span className="vm-kv-key">IMAP port</span><span className="vm-kv-val">993 · SSL/TLS</span><CopyIcon /></div>
          <div className="vm-kv"><span className="vm-kv-key">SMTP server</span><span className="vm-kv-val">mail.vulos.org</span><CopyIcon /></div>
          <div className="vm-kv"><span className="vm-kv-key">SMTP port</span><span className="vm-kv-val">587 · STARTTLS</span><CopyIcon /></div>
          <p className="vm-acct-note">
            <Icon name="server" />
            Use these settings with your mailbox password to connect Thunderbird, Apple Mail, K-9 or any IMAP client.
          </p>
        </div>
      </section>

      <section className="vm-set-section">
        <h3 className="vm-set-section-title">Password</h3>
        <form className="vm-set-group vm-acct" onSubmit={(e) => e.preventDefault()}>
          <label className="vm-field"><span>Current password</span><input className="vm-input" type="password" defaultValue="" /></label>
          <label className="vm-field"><span>New password</span>
            <div className="vm-input-wrap">
              <input className="vm-input" type="password" defaultValue="" />
              <button type="button" className="vm-input-reveal" aria-label="Show passwords"><Icon name="eye" /></button>
            </div>
          </label>
          <label className="vm-field"><span>Confirm new password</span><input className="vm-input" type="password" defaultValue="" /></label>
          <button type="submit" className="vm-btn vm-btn-primary vm-btn-block"><Icon name="key" /> Change password</button>
        </form>
      </section>
    </>
  )
}

function CopyIcon() {
  const [ok, setOk] = useState(false)
  return (
    <button type="button" className={'vm-copybtn' + (ok ? ' vm-ok' : '')} aria-label="Copy"
      onClick={() => { setOk(true); setTimeout(() => setOk(false), 1400) }}>
      <Icon name={ok ? 'check' : 'copy'} />
    </button>
  )
}

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <MailApp client={client} settingsExtra={<DemoAccountSettings />} />
  </StrictMode>,
)
