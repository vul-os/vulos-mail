import { useEffect, useRef, useState } from 'react'
import Icon from './Icon.jsx'

/**
 * <Compose/> — compose modal wired to the /v1 send contract.
 *
 * By default it sends through `onSend(draft)`, which <MailApp/> supplies as
 * `client.sendMessage` (POST /v1/messages). A host app may override `onSend`
 * to route outbound mail through its own transport. `onSend` may return a
 * promise; the button shows a sending state and the modal closes on resolve.
 *
 * @param {object} props
 * @param {{to?:string,cc?:string,bcc?:string,subject?:string,body?:string}} [props.initial]
 * @param {(draft:{to,cc,bcc,subject,text}) => (void|Promise<void>)} props.onSend
 * @param {() => void} props.onClose
 */
export default function Compose({ initial = {}, onSend, onClose }) {
  const [to, setTo] = useState(initial.to ?? '')
  const [cc, setCc] = useState(initial.cc ?? '')
  const [bcc, setBcc] = useState(initial.bcc ?? '')
  const [subject, setSubject] = useState(initial.subject ?? '')
  const [body, setBody] = useState(initial.body ?? '')
  const [showCc, setShowCc] = useState(Boolean(initial.cc || initial.bcc))
  const [sending, setSending] = useState(false)
  const [err, setErr] = useState('')
  const toRef = useRef(null)

  useEffect(() => { toRef.current?.focus() }, [])
  useEffect(() => {
    const onKey = (e) => { if (e.key === 'Escape') onClose?.() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  async function send() {
    if (!onSend) return
    setErr('')
    setSending(true)
    try {
      await onSend({ to, cc, bcc, subject, text: body })
      onClose?.()
    } catch (e) {
      setErr(e?.message || 'Failed to send')
      setSending(false)
    }
  }

  return (
    <div className="vm-overlay" role="dialog" aria-modal="true" aria-label="Compose message"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose?.() }}>
      <div className="vm-compose">
        <header className="vm-compose-head">
          <span>New message</span>
          <button type="button" className="vm-iconbtn" aria-label="Close" onClick={onClose}>
            <Icon name="close" />
          </button>
        </header>

        <div className="vm-compose-body">
          <label className="vm-crow">
            <span>To</span>
            <input ref={toRef} type="text" value={to} onChange={(e) => setTo(e.target.value)} placeholder="recipient@example.com" />
            {!showCc && (
              <button type="button" className="vm-cc-toggle" onClick={() => setShowCc(true)}>Cc/Bcc</button>
            )}
          </label>
          {showCc && (
            <>
              <label className="vm-crow">
                <span>Cc</span>
                <input type="text" value={cc} onChange={(e) => setCc(e.target.value)} placeholder="cc@example.com" />
              </label>
              <label className="vm-crow">
                <span>Bcc</span>
                <input type="text" value={bcc} onChange={(e) => setBcc(e.target.value)} placeholder="bcc@example.com" />
              </label>
            </>
          )}
          <label className="vm-crow">
            <span>Subject</span>
            <input type="text" value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="Subject" />
          </label>
          <textarea
            className="vm-ctext"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="Write your message…"
          />
        </div>

        {err && <div className="vm-error" role="alert">{err}</div>}

        <footer className="vm-compose-foot">
          <span className="vm-note" />
          <button type="button" className="vm-btn vm-btn-ghost" onClick={onClose}>Discard</button>
          <button type="button" className="vm-btn vm-btn-primary" onClick={send} disabled={sending || !onSend}>
            <Icon name="send" /> {sending ? 'Sending…' : 'Send'}
          </button>
        </footer>
      </div>
    </div>
  )
}
