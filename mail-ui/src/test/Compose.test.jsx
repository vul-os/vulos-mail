import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import Compose from '../components/Compose.jsx'

afterEach(() => { vi.restoreAllMocks() })

describe('<Compose/>', () => {
  it('minimised compose exposes real Restore/Close buttons (focusable, not bare SVGs)', () => {
    render(<Compose onClose={() => {}} onSend={() => {}} />)
    fireEvent.click(screen.getByLabelText('Minimise'))
    const restore = screen.getByRole('button', { name: 'Restore' })
    const close = screen.getByRole('button', { name: 'Close' })
    expect(restore.tagName).toBe('BUTTON')
    expect(close.tagName).toBe('BUTTON')
  })

  it('toggles Cc/Bcc visibility both ways', () => {
    render(<Compose onClose={() => {}} onSend={() => {}} />)
    expect(screen.queryByLabelText('Cc')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('Cc Bcc'))
    expect(screen.getByLabelText('Cc')).toBeInTheDocument()
    expect(screen.getByLabelText('Bcc')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Hide'))
    expect(screen.queryByLabelText('Cc')).not.toBeInTheDocument()
  })

  it('discard confirms before throwing away a dirty draft', () => {
    const onClose = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<Compose initial={{ subject: 'Hi' }} onClose={onClose} onSend={() => {}} />)
    fireEvent.click(screen.getByLabelText('Discard draft'))
    expect(window.confirm).toHaveBeenCalled()
    expect(onClose).toHaveBeenCalled()
  })

  it('discard aborts when the confirm is declined', () => {
    const onClose = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    render(<Compose initial={{ subject: 'Hi' }} onClose={onClose} onSend={() => {}} />)
    fireEvent.click(screen.getByLabelText('Discard draft'))
    expect(onClose).not.toHaveBeenCalled()
  })

  it('Escape closes the focused compose', () => {
    const onClose = vi.fn()
    render(<Compose onClose={onClose} onSend={() => {}} />)
    fireEvent.keyDown(screen.getByLabelText('Subject'), { key: 'Escape' })
    expect(onClose).toHaveBeenCalled()
  })
})
