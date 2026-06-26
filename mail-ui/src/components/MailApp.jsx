import { useCallback, useEffect, useMemo, useState } from 'react'
import { createMailClient, FLAG_SEEN, FLAG_FLAGGED } from '../api.js'
import FolderList from './FolderList.jsx'
import MessageList from './MessageList.jsx'
import MessageView from './MessageView.jsx'
import Compose from './Compose.jsx'
import '../index.css'

/**
 * <MailApp/> — full three-pane webmail, wired to the lilmail /v1 API.
 *
 * @param {object} props
 * @param {string} [props.baseUrl='/v1'] - API base URL (same-origin by default)
 * @param {object} [props.client]        - pre-built client (overrides baseUrl; for tests)
 * @param {(draft) => (void|Promise<void>)} [props.onSend] - overrides the
 *   default send (POST /v1/messages); useful when the host routes mail itself.
 * @param {(err) => void} [props.onAuthError] - called on 401 responses
 */
export default function MailApp({ baseUrl = '/v1', client: clientProp, onSend, onAuthError }) {
  const client = useMemo(
    () => clientProp ?? createMailClient({ baseUrl }),
    [clientProp, baseUrl],
  )

  // Default send goes straight to /v1; a host may override via onSend.
  const sendDraft = useMemo(
    () => onSend ?? ((draft) => client.sendMessage(draft)),
    [onSend, client],
  )

  const [me, setMe] = useState(null)
  const [folders, setFolders] = useState([])
  const [folder, setFolder] = useState('INBOX')
  const [messages, setMessages] = useState([])
  const [selected, setSelected] = useState(null)   // full Email
  const [query, setQuery] = useState('')
  const [listLoading, setListLoading] = useState(true)
  const [viewLoading, setViewLoading] = useState(false)
  const [listError, setListError] = useState('')
  const [composing, setComposing] = useState(false)
  const [pane, setPane] = useState('list')          // mobile pane: folders | list | read

  const handleError = useCallback((e) => {
    if (e?.status === 401) onAuthError?.(e)
    return e?.message || 'Something went wrong'
  }, [onAuthError])

  // Bootstrap: identity + folders.
  useEffect(() => {
    let live = true
    client.me().then((m) => live && setMe(m)).catch(() => {})
    client.listFolders().then((f) => live && setFolders(f)).catch(() => {})
    return () => { live = false }
  }, [client])

  // Load message list whenever folder or query changes.
  const loadList = useCallback(async () => {
    setListLoading(true)
    setListError('')
    try {
      const msgs = query
        ? await client.search(query, { folder })
        : await client.listMessages({ folder })
      setMessages(msgs)
    } catch (e) {
      setListError(handleError(e))
      setMessages([])
    } finally {
      setListLoading(false)
    }
  }, [client, folder, query, handleError])

  useEffect(() => { loadList() }, [loadList])

  const openMessage = useCallback(async (msg) => {
    setSelected(msg)               // optimistic: show summary immediately
    setPane('read')
    setViewLoading(true)
    try {
      const full = await client.getMessage(msg.id, { folder })
      setSelected(full)
      // Mark read on open if it wasn't.
      if (!(full.flags || []).includes(FLAG_SEEN)) {
        client.setFlag(full.id, FLAG_SEEN, true, { folder }).catch(() => {})
        setSelected((s) => s && { ...s, flags: [...new Set([...(s.flags || []), FLAG_SEEN])] })
        setMessages((list) => list.map((m) =>
          m.id === full.id ? { ...m, flags: [...new Set([...(m.flags || []), FLAG_SEEN])] } : m))
      }
    } catch (e) {
      setListError(handleError(e))
    } finally {
      setViewLoading(false)
    }
  }, [client, folder, handleError])

  const toggleStar = useCallback((msg, next) => {
    const patch = (m) => {
      const flags = new Set(m.flags || [])
      if (next) flags.add(FLAG_FLAGGED); else flags.delete(FLAG_FLAGGED)
      return { ...m, flags: [...flags] }
    }
    setMessages((list) => list.map((m) => (m.id === msg.id ? patch(m) : m)))
    setSelected((s) => (s && s.id === msg.id ? patch(s) : s))
    client.setFlag(msg.id, FLAG_FLAGGED, next, { folder }).catch((e) => {
      handleError(e)
      loadList()  // resync on failure
    })
  }, [client, folder, handleError, loadList])

  const deleteSelected = useCallback(() => {
    if (!selected) return
    const id = selected.id
    setMessages((list) => list.filter((m) => m.id !== id))
    setSelected(null)
    setPane('list')
    client.deleteMessage(id, { folder }).catch((e) => {
      handleError(e)
      loadList()
    })
  }, [client, folder, selected, handleError, loadList])

  const selectFolder = useCallback((f) => {
    setFolder(f)
    setQuery('')
    setSelected(null)
    setPane('list')
  }, [])

  return (
    <div className="vm-app" data-pane={pane}>
      <FolderList
        folders={folders}
        current={folder}
        me={me}
        onSelect={selectFolder}
        onCompose={() => setComposing(true)}
      />

      <MessageList
        messages={messages}
        selectedId={selected?.id ?? null}
        loading={listLoading}
        error={listError}
        query={query}
        onSearch={setQuery}
        onSelect={openMessage}
        onToggleStar={toggleStar}
      />

      <MessageView
        message={selected}
        loading={viewLoading}
        onToggleStar={(next) => selected && toggleStar(selected, next)}
        onDelete={deleteSelected}
        onReply={() => setComposing(true)}
        onBack={() => { setSelected(null); setPane('list') }}
      />

      {composing && (
        <Compose
          initial={selected ? { to: selected.from, subject: 'Re: ' + (selected.subject || '') } : {}}
          onSend={sendDraft}
          onClose={() => setComposing(false)}
        />
      )}
    </div>
  )
}
