import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MailApp } from '../lib/index.js'
import { createMockClient } from './mockClient.js'

// Standalone demo: drives the full Gmail-class <MailApp/> with an in-memory
// client (no backend). Calendar + Contacts are reachable from the in-app side
// rail; a production host omits `client` and talks to /v1 same-origin.
const client = createMockClient()

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <MailApp client={client} />
  </StrictMode>,
)
