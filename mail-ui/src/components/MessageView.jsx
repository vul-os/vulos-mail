import { useMemo } from 'react'
import Icon from './Icon.jsx'
import { sanitizeEmailHtml } from './sanitize.js'
import { FLAG_SEEN, FLAG_FLAGGED } from '../api.js'

function fullDate(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

const hasFlag = (m, flag) => Array.isArray(m?.flags) && m.flags.includes(flag)

/**
 * <MessageView/> — right pane. Renders a single Email, sanitising any HTML body.
 *
 * @param {object} props
 * @param {object|null} props.message - full Email (with body/html)
 * @param {boolean} [props.loading]
 * @param {string} [props.error]
 * @param {(next: boolean) => void} [props.onToggleStar]
 * @param {() => void} [props.onDelete]
 * @param {() => void} [props.onReply]
 * @param {() => void} [props.onBack]   - mobile "back to list"
 */
export default function MessageView({ message, loading, error, onToggleStar, onDelete, onReply, onBack }) {
  const safeHtml = useMemo(
    () => (message?.html ? sanitizeEmailHtml(message.html) : ''),
    [message?.html],
  )

  if (loading) {
    return (
      <section className="vm-read">
        <div className="vm-read-inner">
          <div className="vm-sk-line" style={{ width: '60%', height: 14 }} />
          <div className="vm-sk-line" style={{ width: '90%' }} />
          <div className="vm-sk-line" style={{ width: '85%' }} />
        </div>
      </section>
    )
  }

  if (error) {
    return <section className="vm-read"><div className="vm-empty" role="alert">{error}</div></section>
  }

  if (!message) {
    return <section className="vm-read"><div className="vm-empty">Select a message to read</div></section>
  }

  const starred = hasFlag(message, FLAG_FLAGGED)

  return (
    <section className="vm-read" aria-label="Message">
      <div className="vm-read-inner">
        <div className="vm-read-actions">
          <button type="button" className="vm-iconbtn vm-back" onClick={onBack} aria-label="Back">
            <Icon name="back" />
          </button>
          <button
            type="button"
            className={'vm-iconbtn' + (starred ? ' vm-on' : '')}
            aria-pressed={starred}
            aria-label={starred ? 'Unstar' : 'Star'}
            onClick={() => onToggleStar?.(!starred)}
          >
            <Icon name="star" fill={starred ? 'currentColor' : 'none'} />
          </button>
          <button type="button" className="vm-iconbtn" aria-label="Reply" onClick={onReply}>
            <Icon name="send" />
          </button>
          <span className="vm-spacer" />
          <button type="button" className="vm-iconbtn vm-danger" aria-label="Delete" onClick={onDelete}>
            <Icon name="trash" />
          </button>
        </div>

        <h1 className="vm-read-subject">{message.subject || '(no subject)'}</h1>

        {Array.isArray(message.flags) && message.flags.length > 0 && (
          <div className="vm-chips">
            {message.flags
              .filter((f) => f !== FLAG_SEEN)
              .map((f) => <span key={f} className="vm-chip">{f.replace(/^\\/, '')}</span>)}
          </div>
        )}

        <article className="vm-msg">
          <header className="vm-msg-head">
            <div className="vm-msg-meta">
              <div className="vm-msg-from">{message.fromName || message.from}</div>
              <div className="vm-msg-addr">{message.from}</div>
              {message.to && <div className="vm-msg-to">to {message.to}</div>}
            </div>
            <time className="vm-msg-date">{fullDate(message.date)}</time>
          </header>

          {safeHtml ? (
            <div className="vm-msg-body" dangerouslySetInnerHTML={{ __html: safeHtml }} />
          ) : (
            <div className="vm-msg-body vm-plain">{message.body || ''}</div>
          )}
        </article>
      </div>
    </section>
  )
}
