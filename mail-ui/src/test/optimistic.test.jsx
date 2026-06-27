import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import MailApp from '../components/MailApp.jsx'
import { FLAG_FLAGGED } from '../api.js'

function makeClient() {
  let msgs = [
    { id: 'm1', from: 'a@x.com', fromName: 'Alice', subject: 'First', preview: 'one', date: new Date().toISOString(), flags: ['\\Seen'], messageId: '<m1>' },
    { id: 'm2', from: 'b@x.com', fromName: 'Bob', subject: 'Second', preview: 'two', date: new Date(Date.now() - 1000).toISOString(), flags: ['\\Seen'], messageId: '<m2>' },
  ]
  return {
    me: vi.fn(async () => ({ email: 'me@x.com', username: 'me' })),
    listFolders: vi.fn(async () => [{ path: 'INBOX', name: 'INBOX', attributes: ['\\Inbox'] }]),
    listMessages: vi.fn(async () => msgs.map((m) => ({ ...m, flags: [...m.flags] }))),
    getMessage: vi.fn(async (uid) => ({ ...msgs.find((m) => m.id === uid) })),
    search: vi.fn(async () => []),
    setFlag: vi.fn(async () => null),
    deleteMessage: vi.fn(async (uid) => { msgs = msgs.filter((m) => m.id !== uid); return null }),
    moveMessage: vi.fn(async () => null),
    saveDraft: vi.fn(async () => ({ saved: true })),
    sendMessage: vi.fn(async () => ({ sent: true })),
    listContacts: vi.fn(async () => []),
  }
}

beforeEach(() => { localStorage.clear() })

describe('MailApp optimistic actions', () => {
  it('stars a conversation immediately and calls setFlag(\\Flagged, true)', async () => {
    const client = makeClient()
    render(<MailApp client={client} />)
    await screen.findByText('First')

    const star = screen.getAllByLabelText('Star')[0]
    fireEvent.click(star)

    // Optimistic: the button flips to "Unstar" before any network settles.
    expect(screen.getAllByLabelText('Unstar').length).toBeGreaterThan(0)
    await waitFor(() => expect(client.setFlag).toHaveBeenCalled())
    const [, flag, add] = client.setFlag.mock.calls[0]
    expect(flag).toBe(FLAG_FLAGGED)
    expect(add).toBe(true)
  })

  it('removes a conversation optimistically and commits deleteMessage after the undo window', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const client = makeClient()
      render(<MailApp client={client} />)
      await screen.findByText('First')

      fireEvent.click(screen.getAllByLabelText('Delete')[0])

      // Optimistic: row gone immediately; the other conversation remains.
      await waitFor(() => expect(screen.queryByText('First')).not.toBeInTheDocument())
      expect(screen.getByText('Second')).toBeInTheDocument()
      // Deferred: the server call has NOT fired yet (undo still possible).
      expect(client.deleteMessage).not.toHaveBeenCalled()

      // After the undo window lapses, the delete commits.
      await act(async () => { vi.advanceTimersByTime(7000) })
      expect(client.deleteMessage).toHaveBeenCalledWith('m1', expect.objectContaining({ folder: 'INBOX' }))
    } finally {
      vi.useRealTimers()
    }
  })

  it('Undo restores a deleted conversation and never hits the server', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const client = makeClient()
      render(<MailApp client={client} />)
      await screen.findByText('First')

      fireEvent.click(screen.getAllByLabelText('Delete')[0])
      await waitFor(() => expect(screen.queryByText('First')).not.toBeInTheDocument())

      // Undo before the window lapses re-fetches (server untouched) → restored.
      fireEvent.click(screen.getByRole('button', { name: 'Undo' }))
      await screen.findByText('First')

      await act(async () => { vi.advanceTimersByTime(7000) })
      expect(client.deleteMessage).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })
})
