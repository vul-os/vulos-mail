/**
 * useKeyboard.js — Gmail-style keyboard shortcuts.
 *
 * `keyToAction` is a pure mapping from a KeyboardEvent to an action name (or null)
 * so it can be unit-tested in isolation; `useKeyboard` wires it to window and
 * dispatches into a handler map. Typing in inputs/textareas/contenteditable is
 * never hijacked (except Escape and "/" focus-search semantics).
 */
import { useEffect } from 'react'

/** Map a keyboard event to a Gmail-like action name, or null to ignore. */
export function keyToAction(e) {
  if (e.altKey || e.ctrlKey || e.metaKey) return null
  const k = e.key
  switch (k) {
    case 'j': return 'next'
    case 'k': return 'prev'
    case 'o': return 'open'
    case 'Enter': return 'open'
    case 'u': return 'back'
    case 'e': return 'archive'
    case '#': return 'delete'
    case 'r': return 'reply'
    case 'a': return 'replyAll'
    case 'f': return 'forward'
    case 'c': return 'compose'
    case 's': return 'star'
    case 'x': return 'select'
    case '/': return 'search'
    case '?': return 'help'
    case 'Escape': return 'escape'
    default: return null
  }
}

/** True when the event originates from an editable element. */
function isEditable(t) {
  if (!t) return false
  const tag = t.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || t.isContentEditable
}

/** True for native/ARIA interactive controls that handle their own activation. */
function isInteractive(t) {
  if (!t) return false
  const tag = t.tagName
  return tag === 'BUTTON' || tag === 'A' || tag === 'SUMMARY' || t.getAttribute?.('role') === 'button'
}

/**
 * Wire keyboard shortcuts.
 * @param {Record<string, () => void>} handlers - action → callback
 * @param {boolean} enabled
 */
export function useKeyboard(handlers, enabled = true) {
  useEffect(() => {
    if (!enabled) return undefined
    const onKey = (e) => {
      const editing = isEditable(e.target)
      const action = keyToAction(e)
      if (!action) return
      // In editable fields, only Escape is honoured (e.g. close compose/help).
      if (editing && action !== 'escape') return
      // "open" (Enter / o) on a focused control would double-fire alongside the
      // control's own activation — let the button/link/row handle it itself.
      if (action === 'open' && isInteractive(e.target)) return
      const fn = handlers[action]
      if (fn) {
        // "/" and "?" would otherwise type into a just-focused search box.
        if (action === 'search' || action === 'help' || action === 'delete') e.preventDefault()
        fn(e)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [handlers, enabled])
}
