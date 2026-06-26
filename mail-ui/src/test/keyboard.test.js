import { describe, it, expect } from 'vitest'
import { keyToAction } from '../components/useKeyboard.js'

const ev = (key, mods = {}) => ({ key, altKey: false, ctrlKey: false, metaKey: false, ...mods })

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
