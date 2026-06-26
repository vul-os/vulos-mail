/**
 * src/lib/index.js — @vulos/mail-ui public barrel.
 *
 * Talks to lilmail's /v1 JSON API. Import the stylesheet once in your host app:
 *   import '@vulos/mail-ui/style.css'
 */

export { default as MailApp } from '../components/MailApp.jsx'
export { default as Calendar } from '../components/Calendar.jsx'
export { default as Contacts } from '../components/Contacts.jsx'
export { default as FolderList } from '../components/FolderList.jsx'
export { default as MessageList } from '../components/MessageList.jsx'
export { default as MessageView } from '../components/MessageView.jsx'
export { default as Compose } from '../components/Compose.jsx'
export { default as Icon } from '../components/Icon.jsx'

export { sanitizeEmailHtml, stripHtml } from '../components/sanitize.js'
export {
  createMailClient,
  ApiError,
  FLAG_SEEN,
  FLAG_FLAGGED,
} from '../api.js'
