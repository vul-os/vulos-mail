import { describe, it, expect } from 'vitest'
import { groupThreads, normalizeSubject } from '../components/threading.js'

const mk = (id, over = {}) => ({
  id, from: 'a@x.com', fromName: 'A', subject: 'S', preview: 'p',
  date: new Date(Date.now() - Number(id) * 1000).toISOString(), flags: [], ...over,
})

describe('groupThreads', () => {
  it('groups messages linked by References / In-Reply-To', () => {
    const msgs = [
      mk('1', { messageId: '<a>', subject: 'Hello' }),
      mk('2', { messageId: '<b>', inReplyTo: '<a>', references: ['<a>'], subject: 'Re: Hello' }),
      mk('3', { messageId: '<c>', inReplyTo: '<b>', references: ['<a>', '<b>'], subject: 'Re: Hello' }),
      mk('4', { messageId: '<z>', subject: 'Unrelated' }),
    ]
    const threads = groupThreads(msgs)
    expect(threads).toHaveLength(2)
    const conv = threads.find((t) => t.count === 3)
    expect(conv).toBeTruthy()
    expect(conv.ids.sort()).toEqual(['1', '2', '3'])
    // Root subject (earliest) wins; latest message drives the row id.
    expect(conv.subject).toBe('Hello')
  })

  it('falls back to normalised subject when headers are absent', () => {
    const msgs = [
      mk('1', { subject: 'Invoice' }),
      mk('2', { subject: 'Re: Invoice' }),
    ]
    expect(groupThreads(msgs)).toHaveLength(1)
  })

  it('treats every message as its own thread when threaded=false', () => {
    const msgs = [
      mk('1', { messageId: '<a>', subject: 'Hello' }),
      mk('2', { messageId: '<b>', inReplyTo: '<a>', subject: 'Re: Hello' }),
    ]
    expect(groupThreads(msgs, { threaded: false })).toHaveLength(2)
  })

  it('marks a thread unread/starred if ANY message is', () => {
    const msgs = [
      mk('1', { messageId: '<a>', flags: ['\\Seen'] }),
      mk('2', { messageId: '<b>', references: ['<a>'], flags: ['\\Flagged'] }),
    ]
    const [t] = groupThreads(msgs)
    expect(t.unread).toBe(true)   // message 1 has Seen, message 2 does not
    expect(t.starred).toBe(true)
  })

  it('sorts threads newest-active first', () => {
    const msgs = [
      mk('1', { messageId: '<a>', date: new Date(Date.now() - 10000).toISOString() }),
      mk('2', { messageId: '<b>', date: new Date().toISOString() }),
    ]
    const threads = groupThreads(msgs)
    expect(threads[0].id).toBe('2')
  })
})

describe('normalizeSubject', () => {
  it('strips reply/forward prefixes', () => {
    expect(normalizeSubject('Re: Fwd: Hello')).toBe('hello')
    expect(normalizeSubject('FW: Report')).toBe('report')
  })
})
