import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import MessageList from '../components/MessageList.jsx'

const messages = [
  {
    id: '1', from: 'a@x.com', fromName: 'Alice', subject: 'Hello',
    preview: 'hi there', date: new Date().toISOString(), flags: [],
  },
  {
    id: '2', from: 'b@x.com', fromName: 'Bob', subject: 'Read one',
    preview: 'seen', date: new Date().toISOString(), flags: ['\\Seen', '\\Flagged'],
  },
]

describe('<MessageList/>', () => {
  it('renders a row per message with sender + subject', () => {
    render(<MessageList messages={messages} />)
    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('Hello')).toBeInTheDocument()
    expect(screen.getByText('Read one')).toBeInTheDocument()
  })

  it('marks unread rows (no \\Seen flag) with vm-unread', () => {
    const { container } = render(<MessageList messages={messages} />)
    const rows = container.querySelectorAll('.vm-row')
    expect(rows[0].className).toContain('vm-unread')
    expect(rows[1].className).not.toContain('vm-unread')
  })

  it('fires onSelect when a row is clicked', () => {
    const onSelect = vi.fn()
    render(<MessageList messages={messages} onSelect={onSelect} />)
    fireEvent.click(screen.getByText('Hello'))
    expect(onSelect).toHaveBeenCalledWith(messages[0])
  })

  it('shows empty state when there are no messages', () => {
    render(<MessageList messages={[]} />)
    expect(screen.getByText('No messages')).toBeInTheDocument()
  })

  it('submits the search query', () => {
    const onSearch = vi.fn()
    render(<MessageList messages={messages} onSearch={onSearch} />)
    const input = screen.getByLabelText('Search mail')
    fireEvent.change(input, { target: { value: 'invoice' } })
    fireEvent.submit(input.closest('form'))
    expect(onSearch).toHaveBeenCalledWith('invoice')
  })
})
