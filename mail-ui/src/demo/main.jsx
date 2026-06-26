import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { MailApp, Calendar, Contacts } from '../lib/index.js'
import { createMockClient } from './mockClient.js'

// Standalone demo: drives the shared components with an in-memory client (no
// backend). In production a host app omits `client` and lets each component talk
// to /v1 same-origin.
const client = createMockClient()

function Demo() {
  const [tab, setTab] = useState('mail')
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <nav style={{
        display: 'flex', gap: 4, padding: '8px 12px', background: '#0c0c0c',
        borderBottom: '1px solid #1a1a1a', fontFamily: 'ui-monospace, monospace',
      }}>
        {['mail', 'calendar', 'contacts'].map((t) => (
          <button key={t} onClick={() => setTab(t)} style={{
            padding: '6px 14px', borderRadius: 8, border: 0, cursor: 'pointer',
            textTransform: 'capitalize', fontFamily: 'inherit',
            background: tab === t ? '#1a1a2e' : 'transparent',
            color: tab === t ? '#e5e5e5' : '#888',
          }}>{t}</button>
        ))}
      </nav>
      <div style={{ flex: 1, minHeight: 0 }}>
        {tab === 'mail' && <MailApp client={client} />}
        {tab === 'calendar' && <Calendar client={client} />}
        {tab === 'contacts' && <Contacts client={client} onSelect={(c) => console.log('compose to', c)} />}
      </div>
    </div>
  )
}

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <Demo />
  </StrictMode>,
)
