import { useEffect, useId, useRef, useState } from 'react'
import Icon from './Icon.jsx'
import { stripHtml } from './sanitize.js'

/**
 * <Compose/> — Gmail-style docked composer (bottom-right). Minimise / maximise
 * to full-screen, To/Cc/Bcc with contact autocomplete, a rich-text body
 * (contenteditable → HTML, plain-text fallback), debounced draft auto-save, and
 * a disabled attachments control (upload not yet exposed over /v1).
 */
export default function Compose({
  initial = {}, onSend, onClose, onSaveDraft, onContactSearch, signature = '',
}) {
  const [to, setTo] = useState(initial.to ?? '')
  const [cc, setCc] = useState(initial.cc ?? '')
  const [bcc, setBcc] = useState(initial.bcc ?? '')
  const [subject, setSubject] = useState(initial.subject ?? '')
  const [showCc, setShowCc] = useState(Boolean(initial.cc || initial.bcc))
  const [minimised, setMinimised] = useState(false)
  const [maximised, setMaximised] = useState(false)
  const [sending, setSending] = useState(false)
  const [savedAt, setSavedAt] = useState('')
  const [err, setErr] = useState('')

  const bodyRef = useRef(null)
  const toRef = useRef(null)
  const dockRef = useRef(null)
  const saveTimer = useRef(null)
  const dirty = useRef(false)

  // Seed the contenteditable body once.
  useEffect(() => {
    if (!bodyRef.current) return
    const sig = signature ? `<br><br><div class="vm-sig">${escapeHtml(signature)}</div>` : ''
    bodyRef.current.innerHTML = (initial.html ?? (initial.body ? escapeHtml(initial.body) : '')) + sig
  }, [])  // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { toRef.current?.focus() }, [])

  function collectDraft() {
    const html = bodyRef.current?.innerHTML ?? ''
    return {
      to, cc, bcc, subject,
      html,
      text: stripHtml(html),
      inReplyTo: initial.inReplyTo,
      references: initial.references,
    }
  }

  // Debounced auto-save (POST /v1/drafts) whenever content changes.
  const scheduleSave = () => {
    dirty.current = true
    if (!onSaveDraft) return
    clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(async () => {
      const d = collectDraft()
      if (!d.to && !d.subject && !d.text.trim()) return
      try {
        await onSaveDraft(d)
        setSavedAt(new Date().toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }))
        dirty.current = false
      } catch { /* keep dirty; will retry on next change */ }
    }, 1200)
  }
  useEffect(() => { scheduleSave() }, [to, cc, bcc, subject])  // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => clearTimeout(saveTimer.current), [])

  async function send() {
    if (!onSend) return
    setErr('')
    if (!to.trim()) { setErr('Add at least one recipient'); return }
    setSending(true)
    try {
      await onSend(collectDraft())
      onClose?.()
    } catch (e) {
      setErr(e?.message || 'Failed to send')
      setSending(false)
    }
  }

  // Discarding deletes the draft. No /v1 draft-delete endpoint exists yet, so we
  // confirm before throwing away a draft that has content / has been auto-saved.
  function discard() {
    if ((savedAt || dirty.current) && !window.confirm('Discard this draft?')) return
    onClose?.()
  }

  // Esc closes this compose; Tab is trapped within the dialog when maximised.
  function onDockKeyDown(e) {
    if (e.key === 'Escape') {
      // Stop the app-wide keyboard handler (window listener) from also acting,
      // so Esc only closes *this* focused compose.
      e.nativeEvent?.stopImmediatePropagation?.()
      e.stopPropagation()
      onClose?.()
      return
    }
    if (e.key !== 'Tab' || !maximised || !dockRef.current) return
    const focusable = dockRef.current.querySelectorAll(
      'button:not([disabled]), [href], input:not([disabled]), textarea, [contenteditable="true"], [tabindex]:not([tabindex="-1"])',
    )
    const list = Array.from(focusable).filter((el) => el.offsetParent !== null || el === document.activeElement)
    if (!list.length) return
    const first = list[0]
    const last = list[list.length - 1]
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
  }

  const exec = (cmd, val) => {
    bodyRef.current?.focus()
    try { document.execCommand(cmd, false, val) } catch { /* unsupported in test env */ }
    scheduleSave()
  }
  const addLink = () => {
    const url = window.prompt('Link URL')
    if (url) exec('createLink', url)
  }

  if (minimised) {
    return (
      <div className="vm-compose-dock vm-min">
        <div className="vm-compose-bar">
          <button type="button" className="vm-compose-bar-title" onClick={() => setMinimised(false)}>
            <span className="vm-compose-title">{subject || 'New message'}</span>
          </button>
          <span className="vm-compose-bar-actions">
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Restore" title="Restore" onClick={() => setMinimised(false)}><Icon name="chevup" /></button>
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Close" title="Close" onClick={() => onClose?.()}><Icon name="close" /></button>
          </span>
        </div>
      </div>
    )
  }

  return (
    <div ref={dockRef} className={'vm-compose-dock' + (maximised ? ' vm-max' : '')} role="dialog" aria-modal={maximised ? 'true' : undefined} aria-label="Compose message" onKeyDown={onDockKeyDown}>
      <div className="vm-compose">
        <header className="vm-compose-head">
          <span className="vm-compose-title">{subject || 'New message'}</span>
          <span className="vm-compose-bar-actions">
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Minimise" onClick={() => setMinimised(true)}><Icon name="minus" /></button>
            <button type="button" className="vm-iconbtn vm-sm" aria-label={maximised ? 'Restore' : 'Full screen'} onClick={() => setMaximised((v) => !v)}>
              <Icon name={maximised ? 'collapse' : 'expand'} />
            </button>
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Close" onClick={onClose}><Icon name="close" /></button>
          </span>
        </header>

        <div className="vm-compose-body">
          <RecipientField label="To" value={to} setValue={setTo} inputRef={toRef} onContactSearch={onContactSearch} onChange={scheduleSave}>
            <button type="button" className="vm-cc-toggle" aria-expanded={showCc}
              onClick={() => setShowCc((v) => !v)}>{showCc ? 'Hide' : 'Cc Bcc'}</button>
          </RecipientField>
          {showCc && (
            <>
              <RecipientField label="Cc" value={cc} setValue={setCc} onContactSearch={onContactSearch} onChange={scheduleSave} />
              <RecipientField label="Bcc" value={bcc} setValue={setBcc} onContactSearch={onContactSearch} onChange={scheduleSave} />
            </>
          )}
          <label className="vm-crow">
            <input className="vm-subject" type="text" value={subject} placeholder="Subject"
              onChange={(e) => setSubject(e.target.value)} aria-label="Subject" />
          </label>

          <div
            ref={bodyRef}
            className="vm-ctext"
            contentEditable
            suppressContentEditableWarning
            role="textbox"
            aria-multiline="true"
            aria-label="Message body"
            data-placeholder="Write your message…"
            onInput={scheduleSave}
          />
        </div>

        {err && <div className="vm-error" role="alert">{err}</div>}

        <footer className="vm-compose-foot">
          <button type="button" className="vm-btn vm-btn-primary" onClick={send} disabled={sending || !onSend}>
            <Icon name="send" /> {sending ? 'Sending…' : 'Send'}
          </button>
          <div className="vm-fmt" role="toolbar" aria-label="Formatting">
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Bold" title="Bold" onMouseDown={(e) => e.preventDefault()} onClick={() => exec('bold')}><Icon name="bold" /></button>
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Italic" title="Italic" onMouseDown={(e) => e.preventDefault()} onClick={() => exec('italic')}><Icon name="italic" /></button>
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Bulleted list" title="Bulleted list" onMouseDown={(e) => e.preventDefault()} onClick={() => exec('insertUnorderedList')}><Icon name="ul" /></button>
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Numbered list" title="Numbered list" onMouseDown={(e) => e.preventDefault()} onClick={() => exec('insertOrderedList')}><Icon name="ol" /></button>
            <button type="button" className="vm-iconbtn vm-sm" aria-label="Insert link" title="Insert link" onMouseDown={(e) => e.preventDefault()} onClick={addLink}><Icon name="link" /></button>
          </div>
          <span className="vm-spacer" />
          <button type="button" className="vm-iconbtn vm-sm vm-attach-btn" aria-label="Attach files (coming soon)"
            title="Attachments are not yet available over /v1" disabled><Icon name="attach" /></button>
          {savedAt && <span className="vm-note">Saved {savedAt}</span>}
          <button type="button" className="vm-iconbtn vm-sm vm-danger" aria-label="Discard draft" title="Discard" onClick={discard}><Icon name="trash" /></button>
        </footer>
      </div>
    </div>
  )
}

/** A single recipient input with debounced contact autocomplete. */
function RecipientField({ label, value, setValue, inputRef, onContactSearch, onChange, children }) {
  const [open, setOpen] = useState(false)
  const [sugs, setSugs] = useState([])
  const [active, setActive] = useState(0)
  const timer = useRef(null)
  const listId = useId()

  const lastToken = () => {
    const parts = value.split(',')
    return parts[parts.length - 1].trim()
  }
  const replaceLast = (email) => {
    const parts = value.split(',')
    parts[parts.length - 1] = ' ' + email
    setValue(parts.join(',').replace(/^\s+/, '') + ', ')
    setOpen(false); setSugs([])
    onChange?.()
  }

  function onType(e) {
    const v = e.target.value
    setValue(v)
    onChange?.()
    const token = v.split(',').pop().trim()
    clearTimeout(timer.current)
    if (!onContactSearch || token.length < 1) { setOpen(false); setSugs([]); return }
    timer.current = setTimeout(async () => {
      try {
        const rows = await onContactSearch(token)
        setSugs((rows || []).slice(0, 6)); setActive(0); setOpen((rows || []).length > 0)
      } catch { setSugs([]); setOpen(false) }
    }, 160)
  }

  function onKeyDown(e) {
    if (!open) return
    if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => Math.min(a + 1, sugs.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)) }
    else if (e.key === 'Enter' && sugs[active]) { e.preventDefault(); replaceLast(sugs[active].email) }
    else if (e.key === 'Escape') { setOpen(false) }
  }

  return (
    <div className="vm-crow vm-recip">
      <span className="vm-crow-label">{label}</span>
      <div className="vm-recip-wrap">
        <input
          ref={inputRef}
          type="text"
          value={value}
          onChange={onType}
          onKeyDown={onKeyDown}
          onBlur={() => setTimeout(() => setOpen(false), 120)}
          aria-label={label}
          aria-autocomplete="list"
          aria-expanded={open}
          aria-controls={listId}
          autoComplete="off"
        />
        {open && (
          <ul className="vm-autocomplete" id={listId} role="listbox">
            {sugs.map((s, i) => (
              <li key={s.email + i} role="option" aria-selected={i === active}
                className={'vm-ac-item' + (i === active ? ' vm-on' : '')}
                onMouseDown={(e) => { e.preventDefault(); replaceLast(s.email) }}
                onMouseEnter={() => setActive(i)}>
                <span className="vm-ac-name">{s.name || s.email}</span>
                {s.name && <span className="vm-ac-email">{s.email}</span>}
              </li>
            ))}
          </ul>
        )}
      </div>
      {children}
    </div>
  )
}

function escapeHtml(s = '') {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
}
