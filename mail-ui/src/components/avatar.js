/**
 * avatar.js — deterministic colour-hashed initials, Gmail-style.
 * Colours are HSL (no hardcoded hex) so they read on both dark + light themes.
 */

/** First letter of a display name / email, uppercased. */
export function initials(name = '', email = '') {
  const s = (name || email).trim()
  if (!s) return '?'
  const parts = s.split(/\s+/).filter(Boolean)
  if (parts.length >= 2 && /[a-z]/i.test(parts[1][0])) {
    return (parts[0][0] + parts[1][0]).toUpperCase()
  }
  return s[0].toUpperCase()
}

/** Stable 0..359 hue from an arbitrary seed string. */
function hueOf(seed = '') {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) % 360
  return h
}

/** Inline-style background + text colour for an avatar chip (HSL, theme-safe). */
export function avatarStyle(seed = '') {
  const h = hueOf(seed.toLowerCase())
  return {
    background: `hsl(${h} 42% 38%)`,
    color: `hsl(${h} 60% 92%)`,
  }
}
