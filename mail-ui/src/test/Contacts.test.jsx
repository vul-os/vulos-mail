import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import Contacts from '../components/Contacts.jsx'

function mockClient(rows) {
  return { listContacts: vi.fn(async () => rows) }
}

describe('<Contacts/>', () => {
  it('renders contacts from the client', async () => {
    const client = mockClient([
      { email: 'alice@x.com', name: 'Alice' },
      { email: 'bob@x.com', name: 'Bob' },
    ])
    render(<Contacts client={client} />)
    expect(await screen.findByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('Bob')).toBeInTheDocument()
    expect(client.listContacts).toHaveBeenCalled()
  })

  it('fires onSelect when a contact is clicked', async () => {
    const onSelect = vi.fn()
    const client = mockClient([{ email: 'alice@x.com', name: 'Alice' }])
    render(<Contacts client={client} onSelect={onSelect} />)
    fireEvent.click(await screen.findByText('Alice'))
    expect(onSelect).toHaveBeenCalledWith({ email: 'alice@x.com', name: 'Alice' })
  })

  it('shows empty state when there are no contacts', async () => {
    const client = mockClient([])
    render(<Contacts client={client} />)
    await waitFor(() => expect(screen.getByText('No contacts')).toBeInTheDocument())
  })
})
