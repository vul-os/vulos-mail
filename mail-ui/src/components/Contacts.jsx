import { useCallback, useEffect, useMemo, useState } from 'react'
import { createMailClient } from '../api.js'
import Icon from './Icon.jsx'
import { avatarStyle } from './avatar.js'
import '../index.css'

const initial = (name = '', email = '') => {
  const s = (name || email).trim()
  return s ? s[0].toUpperCase() : '?'
}

/**
 * <Contacts/> — searchable contact list over the /v1 contacts API (CardDAV).
 *
 * @param {object} props
 * @param {string} [props.baseUrl='/v1']
 * @param {object} [props.client]        - pre-built client (overrides baseUrl)
 * @param {(contact:{email,name}) => void} [props.onSelect] - e.g. start a compose
 * @param {(err) => void} [props.onAuthError]
 */
export default function Contacts({ baseUrl = '/v1', client: clientProp, onSelect, onAuthError }) {
  const client = useMemo(() => clientProp ?? createMailClient({ baseUrl }), [clientProp, baseUrl])

  const [q, setQ] = useState('')
  const [contacts, setContacts] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const handleError = useCallback((e) => {
    if (e?.status === 401) onAuthError?.(e)
    return e?.message || 'Could not load contacts'
  }, [onAuthError])

  // Debounced search.
  useEffect(() => {
    let live = true
    setLoading(true)
    setError('')
    const t = setTimeout(() => {
      client.listContacts({ q })
        .then((rows) => { if (live) setContacts(rows) })
        .catch((e) => { if (live) { setError(handleError(e)); setContacts([]) } })
        .finally(() => { if (live) setLoading(false) })
    }, q ? 200 : 0)
    return () => { live = false; clearTimeout(t) }
  }, [client, q, handleError])

  return (
    <div className="vm-contacts">
      <header className="vm-contacts-head">
        <div className="vm-brand">
          <Icon name="users" className="vm-icon vm-brand-mark" />
          <span>Contacts</span>
        </div>
        <form className="vm-search" role="search" onSubmit={(e) => e.preventDefault()}>
          <Icon name="search" className="vm-icon" />
          <input
            type="search"
            value={q}
            placeholder="Search contacts"
            aria-label="Search contacts"
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
      </header>

      {error && <div className="vm-error" role="alert">{error}</div>}

      {loading ? (
        <ul className="vm-rows">
          {Array.from({ length: 6 }).map((_, i) => (
            <li key={i} className="vm-skeleton" aria-hidden="true">
              <div className="vm-sk-line" style={{ width: '40%' }} />
              <div className="vm-sk-line" style={{ width: '70%' }} />
            </li>
          ))}
        </ul>
      ) : contacts.length === 0 ? (
        <div className="vm-empty">No contacts</div>
      ) : (
        <ul className="vm-contact-list">
          {contacts.map((ct, i) => (
            <li key={(ct.email || '') + i}>
              <button
                type="button"
                className="vm-contact-row"
                onClick={() => onSelect?.(ct)}
                disabled={!onSelect}
              >
                <span className="vm-avatar" style={avatarStyle(ct.email || ct.name)} aria-hidden="true">{initial(ct.name, ct.email)}</span>
                <span className="vm-contact-main">
                  <span className="vm-contact-name">{ct.name || ct.email}</span>
                  {ct.name && <span className="vm-contact-email">{ct.email}</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
