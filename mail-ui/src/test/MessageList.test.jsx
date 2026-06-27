import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import MessageList from '../components/MessageList.jsx'

const threads = [
  {
    id: '1', from: 'a@x.com', fromName: 'Alice', subject: 'Hello', preview: 'hi there',
    date: new Date().toISOString(), count: 1, unread: true, starred: false, hasAttachments: false,
    messages: [{ id: '1', flags: [] }], latest: { id: '1' },
  },
  {
    id: '2', from: 'b@x.com', fromName: 'Bob', subject: 'Read one', preview: 'seen',
    date: new Date().toISOString(), count: 3, unread: false, starred: true, hasAttachments: true,
    messages: [{ id: '2', flags: ['\\Seen'] }], latest: { id: '2' },
  },
]

const noSel = new Set()

describe('<MessageList/>', () => {
  it('renders a row per thread with sender + subject + thread count', () => {
    render(<MessageList threads={threads} selection={noSel} />)
    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('Hello')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument() // thread count badge
  })

  it('marks unread threads with vm-unread', () => {
    const { container } = render(<MessageList threads={threads} selection={noSel} />)
    const rows = container.querySelectorAll('.vm-row')
    expect(rows[0].className).toContain('vm-unread')
    expect(rows[1].className).not.toContain('vm-unread')
  })

  it('opens a thread on row click', () => {
    const onOpen = vi.fn()
    render(<MessageList threads={threads} selection={noSel} onOpen={onOpen} />)
    fireEvent.click(screen.getByText('Hello'))
    expect(onOpen).toHaveBeenCalledWith(threads[0])
  })

  it('opens a thread on Enter and Space (keyboard activation)', () => {
    const onOpen = vi.fn()
    const { container } = render(<MessageList threads={threads} selection={noSel} onOpen={onOpen} />)
    const row = container.querySelector('.vm-row')
    fireEvent.keyDown(row, { key: 'Enter' })
    fireEvent.keyDown(row, { key: ' ' })
    expect(onOpen).toHaveBeenCalledTimes(2)
    expect(onOpen).toHaveBeenCalledWith(threads[0])
  })

  it('shift-clicking a checkbox selects a contiguous range', () => {
    const onToggleSelect = vi.fn()
    const onSelectRange = vi.fn()
    render(<MessageList threads={threads} selection={noSel} onToggleSelect={onToggleSelect} onSelectRange={onSelectRange} />)
    const checks = screen.getAllByLabelText('Select')
    fireEvent.click(checks[0])                          // anchor
    fireEvent.click(checks[1], { shiftKey: true })      // extend
    expect(onToggleSelect).toHaveBeenCalledWith('1')
    expect(onSelectRange).toHaveBeenCalledWith(['1', '2'])
  })

  it('offers a Compose CTA on inbox-zero (non-search empty state)', () => {
    const onCompose = vi.fn()
    render(<MessageList threads={[]} selection={noSel} folder="INBOX" onCompose={onCompose} />)
    fireEvent.click(screen.getByRole('button', { name: 'Compose' }))
    expect(onCompose).toHaveBeenCalled()
  })

  it('hides the Compose CTA on a no-results search', () => {
    render(<MessageList threads={[]} selection={noSel} folder="INBOX" query="zzz" onCompose={() => {}} />)
    expect(screen.getByText('No results')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Compose' })).not.toBeInTheDocument()
  })

  it('toggles selection via the row checkbox', () => {
    const onToggleSelect = vi.fn()
    render(<MessageList threads={threads} selection={noSel} onToggleSelect={onToggleSelect} />)
    fireEvent.click(screen.getAllByLabelText('Select')[0])
    expect(onToggleSelect).toHaveBeenCalledWith('1')
  })

  it('select-all checkbox selects every thread', () => {
    const onSelectAll = vi.fn()
    render(<MessageList threads={threads} selection={noSel} onSelectAll={onSelectAll} />)
    fireEvent.click(screen.getByLabelText('Select all'))
    expect(onSelectAll).toHaveBeenCalledWith(true)
  })

  it('shows the bulk action bar when a selection is active', () => {
    const onDelete = vi.fn()
    render(<MessageList threads={threads} selection={new Set(['1'])} onDelete={onDelete} canArchive />)
    expect(screen.getByText('1 selected')).toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('Delete selected'))
    expect(onDelete).toHaveBeenCalledWith(null) // null => operate on selection
  })

  it('fires star + hover quick-actions', () => {
    const onToggleStar = vi.fn(); const onArchive = vi.fn()
    render(<MessageList threads={threads} selection={noSel} onToggleStar={onToggleStar} onArchive={onArchive} canArchive />)
    fireEvent.click(screen.getAllByLabelText('Star')[0])
    expect(onToggleStar).toHaveBeenCalledWith(threads[0], true)
    fireEvent.click(screen.getAllByLabelText('Archive')[0])
    expect(onArchive).toHaveBeenCalledWith(threads[0])
  })

  it('submits the search query', () => {
    const onSearch = vi.fn()
    render(<MessageList threads={threads} selection={noSel} onSearch={onSearch} />)
    const input = screen.getByLabelText('Search mail')
    fireEvent.change(input, { target: { value: 'invoice' } })
    fireEvent.submit(input.closest('form'))
    expect(onSearch).toHaveBeenCalledWith('invoice')
  })

  it('shows an empty state with no threads', () => {
    render(<MessageList threads={[]} selection={noSel} folder="INBOX" />)
    expect(screen.getByText('Nothing here — inbox zero')).toBeInTheDocument()
  })
})
