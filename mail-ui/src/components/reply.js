/** reply.js — build reply / reply-all / forward prefill from an open message. */
import { sanitizeEmailHtml, stripHtml } from './sanitize.js'
import { fullDate, splitAddrs } from './format.js'

function escapeHtml(s = '') {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

/** Original body as safe HTML (prefer html, else escape plain text). */
function originalHtml(m) {
  if (m.html) return sanitizeEmailHtml(m.html)
  return escapeHtml(m.body || m.preview || '').replace(/\n/g, '<br>')
}

/** A Gmail-style attribution line + blockquote for replies. */
export function quoteReply(m) {
  const who = escapeHtml(m.fromName || m.from || '')
  return `<br><br><div class="vm-quote-attr">On ${escapeHtml(fullDate(m.date))}, ${who} wrote:</div>` +
    `<blockquote class="vm-quote">${originalHtml(m)}</blockquote>`
}

/** Forwarded-message header block + body. */
export function quoteForward(m) {
  const lines = [
    '---------- Forwarded message ----------',
    `From: ${m.fromName ? m.fromName + ' <' + m.from + '>' : m.from}`,
    `Date: ${fullDate(m.date)}`,
    `Subject: ${m.subject || ''}`,
    m.to ? `To: ${m.to}` : '',
  ].filter(Boolean)
  return `<br><br><div class="vm-quote-attr">${lines.map(escapeHtml).join('<br>')}</div>` +
    `<blockquote class="vm-quote">${originalHtml(m)}</blockquote>`
}

/** Reply-all CC = original To + Cc, minus my own address and the sender. */
export function replyAllCc(m, myEmail = '') {
  const mine = myEmail.toLowerCase()
  const from = (m.from || '').toLowerCase()
  const seen = new Set([mine, from])
  const out = []
  for (const a of [...splitAddrs(m.to || ''), ...splitAddrs(m.cc || '')]) {
    const key = a.toLowerCase()
    if (key && !seen.has(key)) { seen.add(key); out.push(a) }
  }
  return out.join(', ')
}

export { stripHtml }
