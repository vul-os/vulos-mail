import { useState } from 'react'
import Icon from './Icon.jsx'
import { FLAG_SEEN, FLAG_FLAGGED } from '../api.js'

function initial(email = '', name = '') {
  const s = (name || email).trim()
  return s ? s[0].toUpperCase() : '?'
}

function shortDate(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const now = new Date()
  const sameDay = d.toDateString() === now.toDateString()
  return sameDay
    ? d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
    : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

const hasFlag = (m, flag) => Array.isArray(m.flags) && m.flags.includes(flag)

/**
 * <MessageList/> — middle pane. Renders Email[] from /v1/messages or /v1/search.
 *
 * @param {object} props
 * @param {Array} props.messages
 * @param {string|null} props.selectedId
 * @param {(msg) => void} props.onSelect
 * @param {(msg, next: boolean) => void} [props.onToggleStar]
 * @param {boolean} [props.loading]
 * @param {string} [props.error]
 * @param {string} [props.query]
 * @param {(q: string) => void} [props.onSearch]
 */
export default function MessageList({
  messages = [], selectedId, onSelect, onToggleStar,
  loading, error, query = '', onSearch,
}) {
  const [q, setQ] = useState(query)

  function submit(e) {
    e.preventDefault()
    onSearch?.(q.trim())
  }

  return (
    <section className="vm-list" aria-label="Messages">
      <div className="vm-topbar">
        <form className="vm-search" onSubmit={submit} role="search">
          <Icon name="search" className="vm-icon" />
          <input
            type="search"
            value={q}
            placeholder="Search mail"
            onChange={(e) => setQ(e.target.value)}
            aria-label="Search mail"
          />
        </form>
      </div>

      {error && <div className="vm-error" role="alert">{error}</div>}

      {loading ? (
        <ul className="vm-rows">
          {Array.from({ length: 8 }).map((_, i) => (
            <li key={i} className="vm-skeleton" aria-hidden="true">
              <div className="vm-sk-line" style={{ width: '40%' }} />
              <div className="vm-sk-line" style={{ width: '80%' }} />
            </li>
          ))}
        </ul>
      ) : messages.length === 0 ? (
        <div className="vm-empty">No messages</div>
      ) : (
        <ul className="vm-rows">
          {messages.map((m) => {
            const unread = !hasFlag(m, FLAG_SEEN)
            const starred = hasFlag(m, FLAG_FLAGGED)
            return (
              <li key={m.id}>
                <button
                  type="button"
                  className={
                    'vm-row' +
                    (m.id === selectedId ? ' vm-active' : '') +
                    (unread ? ' vm-unread' : '')
                  }
                  onClick={() => onSelect?.(m)}
                >
                  <span className="vm-avatar" aria-hidden="true">{initial(m.from, m.fromName)}</span>
                  <span className="vm-row-main">
                    <span className="vm-row-top">
                      <span className="vm-row-from">{m.fromName || m.from}</span>
                      <span className="vm-row-date">{shortDate(m.date)}</span>
                    </span>
                    <span className="vm-row-subj">{m.subject || '(no subject)'}</span>
                    <span className="vm-row-snip">{m.preview}</span>
                  </span>
                  <span className="vm-row-flags">
                    <span
                      role="button"
                      tabIndex={0}
                      aria-label={starred ? 'Unstar' : 'Star'}
                      aria-pressed={starred}
                      className={'vm-star' + (starred ? ' vm-on' : '')}
                      onClick={(e) => { e.stopPropagation(); onToggleStar?.(m, !starred) }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault(); e.stopPropagation(); onToggleStar?.(m, !starred)
                        }
                      }}
                    >
                      <Icon name="star" className="vm-star" fill={starred ? 'currentColor' : 'none'} />
                    </span>
                    {m.hasAttachments && <Icon name="paperclip" className="vm-attach-dot" />}
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
