/** format.js — date / address formatting helpers shared across the UI. */

/** Compact, list-friendly date: time today, "Mon DD" this year, else "MM/DD/YY". */
export function shortDate(iso) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const now = new Date()
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  }
  if (d.getFullYear() === now.getFullYear()) {
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  }
  return d.toLocaleDateString(undefined, { year: '2-digit', month: 'numeric', day: 'numeric' })
}

/** Human relative-ish date for the reading pane ("Jun 12, 2026, 3:04 PM"). */
export function fullDate(iso) {
  const d = new Date(iso)
  return Number.isNaN(d.getTime())
    ? ''
    : d.toLocaleString(undefined, {
        month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit',
      })
}

/** The display name from a "Name <addr>" or bare address. */
export function displayName(name, email = '') {
  if (name) return name
  const at = email.indexOf('@')
  return at > 0 ? email.slice(0, at) : email
}

/** Split a comma/semicolon address list into trimmed addresses. */
export function splitAddrs(s = '') {
  return s.split(/[,;]/).map((x) => x.trim()).filter(Boolean)
}
