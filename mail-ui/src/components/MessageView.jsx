import { useEffect, useMemo, useState } from 'react'
import Icon from './Icon.jsx'
import { sanitizeEmailHtml } from './sanitize.js'
import { initials, avatarStyle } from './avatar.js'
import { fullDate } from './format.js'
import { FLAG_SEEN, FLAG_FLAGGED } from '../api.js'

const hasFlag = (m, f) => Array.isArray(m?.flags) && m.flags.includes(f)

/**
 * <MessageView/> — right pane. Renders a Thread as a collapsible conversation
 * (latest expanded); each message lazy-loads its full body on expand.
 */
export default function MessageView({
  thread, fullById = {}, onNeedBody, loading, error,
  onToggleStar, onArchive, onDelete, onReply, onReplyAll, onForward, onBack,
  canArchive = true,
}) {
  const messages = thread?.messages ?? []
  const latestId = messages.length ? messages[messages.length - 1].id : null
  const [expanded, setExpanded] = useState(() => new Set())

  // Reset expansion to "latest only" whenever the open thread changes.
  useEffect(() => {
    if (latestId == null) { setExpanded(new Set()); return }
    setExpanded(new Set([latestId]))
    onNeedBody?.(latestId)
  }, [thread?.id, latestId])  // eslint-disable-line react-hooks/exhaustive-deps

  const toggle = (id) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else { next.add(id); onNeedBody?.(id) }
      return next
    })
  }

  if (loading && !thread) {
    return (
      <section className="vm-read">
        <div className="vm-read-inner">
          <div className="vm-sk-line" style={{ width: '60%', height: 16 }} />
          <div className="vm-sk-line" style={{ width: '90%' }} />
          <div className="vm-sk-line" style={{ width: '82%' }} />
          <div className="vm-sk-line" style={{ width: '88%' }} />
        </div>
      </section>
    )
  }
  if (error) return <section className="vm-read"><div className="vm-empty vm-state" role="alert">{error}</div></section>
  if (!thread) {
    return (
      <section className="vm-read">
        <div className="vm-empty vm-state">
          <Icon name="mailopen" className="vm-empty-icon" />
          <p>Select a conversation to read</p>
        </div>
      </section>
    )
  }

  const starred = thread.starred

  return (
    <section className="vm-read" aria-label="Conversation">
      <div className="vm-read-actions">
        <button type="button" className="vm-iconbtn vm-back" onClick={onBack} aria-label="Back to list"><Icon name="back" /></button>
        {canArchive && (
          <button type="button" className="vm-iconbtn" aria-label="Archive" title="Archive" onClick={onArchive}><Icon name="archive" /></button>
        )}
        <button type="button" className="vm-iconbtn vm-danger" aria-label="Delete" title="Delete" onClick={onDelete}><Icon name="trash" /></button>
        <button type="button" className={'vm-iconbtn' + (starred ? ' vm-on' : '')} aria-label={starred ? 'Unstar' : 'Star'}
          aria-pressed={starred} onClick={() => onToggleStar?.(!starred)}><Icon name="star" fill={starred ? 'currentColor' : 'none'} /></button>
        <span className="vm-spacer" />
        <button type="button" className="vm-iconbtn" aria-label="Reply" title="Reply"
          onClick={() => onReply?.(thread.latest)}><Icon name="reply" /></button>
        <button type="button" className="vm-iconbtn" aria-label="Reply all" title="Reply all"
          onClick={() => onReplyAll?.(thread.latest)}><Icon name="replyall" /></button>
        <button type="button" className="vm-iconbtn" aria-label="Forward" title="Forward"
          onClick={() => onForward?.(thread.latest)}><Icon name="forward" /></button>
      </div>

      <div className="vm-read-inner">
        <div className="vm-read-headline">
          <h1 className="vm-read-subject">{thread.subject || '(no subject)'}</h1>
          {thread.count > 1 && <span className="vm-read-count">{thread.count}</span>}
        </div>

        {/* Mobile bottom action bar. */}
        <div className="vm-mobile-actions" role="toolbar" aria-label="Conversation actions">
          <button type="button" className="vm-iconbtn" aria-label="Reply" onClick={() => onReply?.(thread.latest)}><Icon name="reply" /></button>
          <button type="button" className="vm-iconbtn" aria-label="Reply all" onClick={() => onReplyAll?.(thread.latest)}><Icon name="replyall" /></button>
          <button type="button" className="vm-iconbtn" aria-label="Forward" onClick={() => onForward?.(thread.latest)}><Icon name="forward" /></button>
          {canArchive && <button type="button" className="vm-iconbtn" aria-label="Archive" onClick={onArchive}><Icon name="archive" /></button>}
          <button type="button" className="vm-iconbtn vm-danger" aria-label="Delete" onClick={onDelete}><Icon name="trash" /></button>
        </div>

        <div className="vm-thread" key={thread.id}>
          {messages.map((m, i) => {
            const isOpen = expanded.has(m.id)
            const isLast = i === messages.length - 1
            return (
              <Message
                key={m.id}
                summary={m}
                full={fullById[m.id]}
                open={isOpen}
                last={isLast}
                onToggle={() => toggle(m.id)}
                onReply={() => onReply?.(fullById[m.id] || m)}
                onReplyAll={() => onReplyAll?.(fullById[m.id] || m)}
                onForward={() => onForward?.(fullById[m.id] || m)}
              />
            )
          })}
        </div>
      </div>
    </section>
  )
}

function Message({ summary, full, open, last, onToggle, onReply, onReplyAll, onForward }) {
  const m = full || summary
  const safeHtml = useMemo(() => (m?.html ? sanitizeEmailHtml(m.html) : ''), [m?.html])
  const starred = hasFlag(m, FLAG_FLAGGED)
  const unread = !hasFlag(summary, FLAG_SEEN)
  const sender = m.fromName || m.from || '(unknown)'
  const bodyReady = full || m.html || m.body

  if (!open) {
    return (
      <article className={'vm-msg vm-collapsed' + (unread ? ' vm-unread' : '')}>
        <button type="button" className="vm-msg-head" onClick={onToggle} aria-expanded="false">
          <span className="vm-avatar vm-avatar-sm" style={avatarStyle(m.from || sender)} aria-hidden="true">{initials(m.fromName, m.from)}</span>
          <span className="vm-msg-meta">
            <span className="vm-msg-from">{sender}</span>
            <span className="vm-msg-collapsed-snip">{summary.preview || m.preview}</span>
          </span>
          {starred && <Icon name="star" className="vm-msg-star" fill="currentColor" />}
          <time className="vm-msg-date">{fullDate(m.date)}</time>
        </button>
      </article>
    )
  }

  return (
    <article className="vm-msg">
      <header className="vm-msg-head" onClick={last ? undefined : onToggle} role={last ? undefined : 'button'}>
        <span className="vm-avatar" style={avatarStyle(m.from || sender)} aria-hidden="true">{initials(m.fromName, m.from)}</span>
        <span className="vm-msg-meta">
          <span className="vm-msg-fromline">
            <span className="vm-msg-from">{sender}</span>
            <span className="vm-msg-addr">&lt;{m.from}&gt;</span>
          </span>
          {m.to && <span className="vm-msg-to">to {m.to}{m.cc ? `, ${m.cc}` : ''}</span>}
        </span>
        <time className="vm-msg-date">{fullDate(m.date)}</time>
      </header>

      {!bodyReady ? (
        <div className="vm-msg-body">
          <div className="vm-sk-line" style={{ width: '90%' }} />
          <div className="vm-sk-line" style={{ width: '80%' }} />
        </div>
      ) : safeHtml ? (
        <div className="vm-msg-body" dangerouslySetInnerHTML={{ __html: safeHtml }} />
      ) : (
        <div className="vm-msg-body vm-plain">{m.body || ''}</div>
      )}

      {Array.isArray(m.attachments) && m.attachments.length > 0 && (
        <div className="vm-attach-list">
          {m.attachments.map((a, i) => (
            <span key={a.id || i} className="vm-attach-chip" title="Download not yet available over /v1">
              <Icon name="attach" /> <span className="vm-attach-name">{a.filename || a.Filename || 'attachment'}</span>
              {(a.size || a.Size) ? <span className="vm-attach-size">{fmtSize(a.size || a.Size)}</span> : null}
            </span>
          ))}
        </div>
      )}

      <footer className="vm-msg-foot">
        <button type="button" className="vm-btn vm-btn-ghost" onClick={onReply}><Icon name="reply" /> Reply</button>
        <button type="button" className="vm-btn vm-btn-ghost" onClick={onReplyAll}><Icon name="replyall" /> Reply all</button>
        <button type="button" className="vm-btn vm-btn-ghost" onClick={onForward}><Icon name="forward" /> Forward</button>
      </footer>
    </article>
  )
}

function fmtSize(bytes) {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(0) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}
