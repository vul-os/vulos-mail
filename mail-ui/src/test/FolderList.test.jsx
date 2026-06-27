import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import FolderList from '../components/FolderList.jsx'

describe('<FolderList/> mobile drawer extras', () => {
  it('surfaces Calendar / Contacts / Settings / Shortcuts when handlers are passed', () => {
    const onOpenPanel = vi.fn()
    const onOpenHelp = vi.fn()
    render(<FolderList folders={[]} onOpenPanel={onOpenPanel} onOpenHelp={onOpenHelp} />)

    fireEvent.click(screen.getByRole('button', { name: 'Calendar' }))
    expect(onOpenPanel).toHaveBeenCalledWith('calendar')
    fireEvent.click(screen.getByRole('button', { name: 'Contacts' }))
    expect(onOpenPanel).toHaveBeenCalledWith('contacts')
    fireEvent.click(screen.getByRole('button', { name: 'Settings' }))
    expect(onOpenPanel).toHaveBeenCalledWith('settings')
    fireEvent.click(screen.getByRole('button', { name: 'Shortcuts' }))
    expect(onOpenHelp).toHaveBeenCalled()
  })

  it('omits the extras block when no handlers are provided', () => {
    render(<FolderList folders={[]} />)
    expect(screen.queryByRole('button', { name: 'Calendar' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Shortcuts' })).not.toBeInTheDocument()
  })
})
