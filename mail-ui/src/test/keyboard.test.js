import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { keyToAction, useKeyboard } from '../components/useKeyboard.js'

const ev = (key, mods = {}) => ({ key, altKey: false, ctrlKey: false, metaKey: false, ...mods })

afterEach(() => { document.body.innerHTML = '' })

function fireKey(target, key) {
  target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
}

describe('keyToAction', () => {
  it('maps the Gmail navigation + action keys', () => {
    const cases = {
      j: 'next', k: 'prev', o: 'open', Enter: 'open', u: 'back',
      e: 'archive', '#': 'delete', r: 'reply', a: 'replyAll', f: 'forward',
      c: 'compose', s: 'star', x: 'select', '/': 'search', '?': 'help', Escape: 'escape',
    }
    for (const [key, action] of Object.entries(cases)) {
      expect(keyToAction(ev(key))).toBe(action)
    }
  })

  it('ignores keys with modifiers (so browser shortcuts still work)', () => {
    expect(keyToAction(ev('j', { metaKey: true }))).toBeNull()
    expect(keyToAction(ev('c', { ctrlKey: true }))).toBeNull()
    expect(keyToAction(ev('a', { altKey: true }))).toBeNull()
  })

  it('returns null for unmapped keys', () => {
    expect(keyToAction(ev('z'))).toBeNull()
    expect(keyToAction(ev('1'))).toBeNull()
  })
})

describe('useKeyboard', () => {
  it('ignores Enter "open" when focus is on an interactive control (no double-fire)', () => {
    const open = vi.fn()
    renderHook(() => useKeyboard({ open }, true))

    const btn = document.createElement('button')
    document.body.appendChild(btn)
    fireKey(btn, 'Enter')
    expect(open).not.toHaveBeenCalled()

    // Enter from a non-interactive element still opens the focused thread.
    const div = document.createElement('div')
    document.body.appendChild(div)
    fireKey(div, 'Enter')
    expect(open).toHaveBeenCalledTimes(1)
  })

  it('honours Escape even from inside an editable field', () => {
    const escape = vi.fn()
    renderHook(() => useKeyboard({ escape }, true))
    const input = document.createElement('input')
    document.body.appendChild(input)
    fireKey(input, 'Escape')
    expect(escape).toHaveBeenCalledTimes(1)
  })
})
