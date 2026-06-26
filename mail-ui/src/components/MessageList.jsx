import { useEffect, useRef, useState } from 'react'
import Icon from './Icon.jsx'
import { initials, avatarStyle } from './avatar.js'
import { shortDate } from './format.js'

/**
 * <MessageList/> — middle pane. Renders Thread[] (see threading.js) with
 * multi-select, a bulk action bar, per-row hover quick-actions, and star.
 */
export default function MessageList({
  threads = [], selectedId, focusId,
  selection, onToggleSelect, onSelectAll,
  onOpen, onToggleStar, onArchive, onDelete, onToggleRead, onRefresh,
  loading, error, onRetry,
  query = '', onSearch, onClearSearch,
  canArchive = true, folder = 'INBOX', searchRef, onMenu,
}) {
  const [q, setQ] = useState(query)
  const innerRef = useRef(null)
  const localSearch = useRef(null)
  useEffect(() => { setQ(query) }, [query])

  const selCount = selection ? selection.size : 0
  const allSelected = threads.length > 0 && selCount === threads.length

  function submit(e) {
    e.preventDefault()
    onSearch?.(q.trim())
  }

  const stop = (fn) => (e) => { e.stopPropagation(); fn?.(e) }

  return (
    <section className="vm-list" aria-label="Messages">
      <div className="vm-topbar">
        <button type="button" className="vm-iconbtn vm-menu-btn" aria-label="Menu" onClick={onMenu}>
          <Icon name="menu" />
        </button>
        <form className="vm-search" onSubmit={submit} role="search">
          <Icon name="search" className="vm-icon" />
          <input
            ref={(el) => { localSearch.current = el; if (searchRef) searchRef.current = el }}
            type="search"
            value={q}
            placeholder="Search mail"
            onChange={(e) => setQ(e.target.value)}
            aria-label="Search mail"
          />
          {q && (
            <button type="button" className="vm-search-clear" aria-label="Clear search"
              onClick={() => { setQ(''); onClearSearch?.() }}>
              <Icon name="close" />
            </button>
          )}
        </form>
      </div>

      {/* Toolbar: select-all + (bulk actions | refresh). */}
      <div className="vm-toolbar">
        <button
          type="button"
          className={'vm-checkbox' + (allSelected ? ' vm-on' : (selCount > 0 ? ' vm-some' : ''))}
          role="checkbox"
          aria-checked={allSelected ? 'true' : (selCount > 0 ? 'mixed' : 'false')}
          aria-label={allSelected ? 'Deselect all' : 'Select all'}
          onClick={() => onSelectAll?.(!allSelected)}
        >
          <Icon name={selCount > 0 && !allSelected ? 'minus' : 'check'} />
        </button>

        {selCount > 0 ? (
          <div className="vm-bulk" role="toolbar" aria-label="Bulk actions">
            <span className="vm-bulk-count">{selCount} selected</span>
            {canArchive && (
              <button type="button" className="vm-iconbtn" aria-label="Archive selected" title="Archive"
                onClick={() => onArchive?.(null)}><Icon name="archive" /></button>
            )}
            <button type="button" className="vm-iconbtn vm-danger" aria-label="Delete selected" title="Delete"
              onClick={() => onDelete?.(null)}><Icon name="trash" /></button>
            <button type="button" className="vm-iconbtn" aria-label="Mark read" title="Mark read"
              onClick={() => onToggleRead?.(null, true)}><Icon name="mailopen" /></button>
            <button type="button" className="vm-iconbtn" aria-label="Mark unread" title="Mark unread"
              onClick={() => onToggleRead?.(null, false)}><Icon name="mail" /></button>
            <button type="button" className="vm-iconbtn" aria-label="Star selected" title="Star"
              onClick={() => onToggleStar?.(null, true)}><Icon name="star" /></button>
          </div>
        ) : (
          <>
            {query && (
              <span className="vm-query-chip">
                <Icon name="search" /> {query}
                <button type="button" aria-label="Clear search" onClick={onClearSearch}><Icon name="close" /></button>
              </span>
            )}
            <span className="vm-spacer" />
            <button type="button" className="vm-iconbtn" aria-label="Refresh" title="Refresh" onClick={onRefresh}>
              <Icon name="refresh" />
            </button>
          </>
        )}
      </div>

      {error ? (
        <div className="vm-empty vm-state" role="alert">
          <Icon name="refresh" className="vm-empty-icon" />
          <p>{error}</p>
          <button type="button" className="vm-btn vm-btn-ghost" onClick={onRetry}>Retry</button>
        </div>
      ) : loading ? (
        <ul className="vm-rows">
          {Array.from({ length: 9 }).map((_, i) => (
            <li key={i} className="vm-skeleton" aria-hidden="true">
              <div className="vm-sk-avatar" />
              <div className="vm-sk-lines">
                <div className="vm-sk-line" style={{ width: '38%' }} />
                <div className="vm-sk-line" style={{ width: '72%' }} />
              </div>
            </li>
          ))}
        </ul>
      ) : threads.length === 0 ? (
        <div className="vm-empty vm-state">
          <Icon name={query ? 'search' : 'inbox'} className="vm-empty-icon" />
          <p>{query ? 'No results' : emptyText(folder)}</p>
        </div>
      ) : (
        <ul className="vm-rows" ref={innerRef}>
          {threads.map((t) => {
            const selected = selection?.has(t.id)
            const sender = t.fromName || t.from || '(unknown)'
            return (
              <li key={t.id}>
                <div
                  className={
                    'vm-row' +
                    (t.id === selectedId ? ' vm-active' : '') +
                    (t.id === focusId ? ' vm-focus' : '') +
                    (t.unread ? ' vm-unread' : '') +
                    (selected ? ' vm-selected' : '')
                  }
                  role="button"
                  tabIndex={0}
                  aria-label={`${sender}: ${t.subject || '(no subject)'}`}
                  onClick={() => onOpen?.(t)}
                  onKeyDown={(e) => { if (e.key === 'Enter') onOpen?.(t) }}
                >
                  <button
                    type="button"
                    className={'vm-checkbox vm-row-check' + (selected ? ' vm-on' : '')}
                    role="checkbox"
                    aria-checked={selected ? 'true' : 'false'}
                    aria-label={selected ? 'Deselect' : 'Select'}
                    onClick={stop(() => onToggleSelect?.(t.id))}
                  >
                    <Icon name="check" />
                  </button>

                  <button
                    type="button"
                    className={'vm-star' + (t.starred ? ' vm-on' : '')}
                    aria-label={t.starred ? 'Unstar' : 'Star'}
                    aria-pressed={t.starred}
                    onClick={stop(() => onToggleStar?.(t, !t.starred))}
                  >
                    <Icon name="star" fill={t.starred ? 'currentColor' : 'none'} />
                  </button>

                  <span className="vm-avatar" style={avatarStyle(t.from || sender)} aria-hidden="true">
                    {initials(t.fromName, t.from)}
                  </span>

                  <span className="vm-row-main">
                    <span className="vm-row-top">
                      <span className="vm-row-from">
                        {sender}
                        {t.count > 1 && <span className="vm-thread-count">{t.count}</span>}
                      </span>
                      <span className="vm-row-date">{shortDate(t.date)}</span>
                    </span>
                    <span className="vm-row-line">
                      <span className="vm-row-subj">{t.subject || '(no subject)'}</span>
                      {t.hasAttachments && <Icon name="paperclip" className="vm-attach-dot" />}
                    </span>
                    <span className="vm-row-snip">{t.preview}</span>
                  </span>

                  {/* Hover quick-actions (replace date on hover). */}
                  <span className="vm-row-actions">
                    {canArchive && (
                      <button type="button" className="vm-iconbtn" aria-label="Archive" title="Archive"
                        onClick={stop(() => onArchive?.(t))}><Icon name="archive" /></button>
                    )}
                    <button type="button" className="vm-iconbtn vm-danger" aria-label="Delete" title="Delete"
                      onClick={stop(() => onDelete?.(t))}><Icon name="trash" /></button>
                    <button type="button" className="vm-iconbtn" aria-label={t.unread ? 'Mark read' : 'Mark unread'}
                      title={t.unread ? 'Mark read' : 'Mark unread'}
                      onClick={stop(() => onToggleRead?.(t, t.unread))}>
                      <Icon name={t.unread ? 'mailopen' : 'mail'} />
                    </button>
                  </span>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

function emptyText(folder) {
  const f = String(folder || '').toLowerCase()
  if (f === '__starred') return 'No starred conversations'
  if (f.includes('sent')) return 'No sent messages'
  if (f.includes('draft')) return 'No drafts'
  if (f.includes('trash') || f.includes('deleted')) return 'Trash is empty'
  if (f.includes('archive')) return 'Nothing archived'
  return 'Nothing here — inbox zero'
}
