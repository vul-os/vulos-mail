/**
 * sanitize.js — single source of truth for HTML email sanitisation.
 *
 * Email HTML is hostile by default, so we run every body through DOMPurify with
 * a strict allow-list: no scripts, iframes, forms, or inline event handlers, and
 * links are forced to open in a new tab with `rel="noopener noreferrer"`.
 */

import DOMPurify from 'dompurify'

const FORBID_EVENT_ATTR = [
  'onerror', 'onload', 'onclick', 'onmouseover', 'onfocus', 'onblur',
  'onchange', 'onsubmit', 'onkeydown', 'onkeyup', 'onkeypress', 'onanimationstart',
]

export const EMAIL_HTML_CONFIG = {
  USE_PROFILES: { html: true },
  FORBID_TAGS: ['script', 'iframe', 'object', 'embed', 'form', 'input', 'button', 'style', 'link', 'meta', 'base'],
  FORBID_ATTR: FORBID_EVENT_ATTR,
  ALLOW_DATA_ATTR: false,
}

// Harden anchors once per module load (DOMPurify hooks are global).
let hooked = false
function ensureHook() {
  if (hooked || typeof DOMPurify.addHook !== 'function') return
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A' && node.getAttribute('href')) {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer')
    }
  })
  hooked = true
}

/** Sanitise an HTML email body, returning a safe HTML string. */
export function sanitizeEmailHtml(html) {
  ensureHook()
  return DOMPurify.sanitize(html ?? '', EMAIL_HTML_CONFIG)
}

/** Strip all markup, returning plain text only. */
export function stripHtml(html) {
  return DOMPurify.sanitize(html ?? '', { ALLOWED_TAGS: [], ALLOWED_ATTR: [] })
}
