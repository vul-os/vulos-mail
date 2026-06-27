import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createMailClient, FLAG_SEEN, FLAG_FLAGGED, ApiError } from '../api.js'
import FolderList, { STARRED_FOLDER, classifyFolder } from './FolderList.jsx'
import MessageList from './MessageList.jsx'
import MessageView from './MessageView.jsx'
import Compose from './Compose.jsx'
import Settings from './Settings.jsx'
import Calendar from './Calendar.jsx'
import Contacts from './Contacts.jsx'
import ShortcutsHelp from './ShortcutsHelp.jsx'
import Icon from './Icon.jsx'
import { groupThreads } from './threading.js'
import { useSettings } from './useSettings.js'
import { useKeyboard } from './useKeyboard.js'
import { quoteReply, quoteForward, replyAllCc } from './reply.js'
import '../index.css'

let composeSeq = 0

// How long an undoable destructive action can be reversed before it commits.
const UNDO_MS = 6000

/**
 * <MailApp/> — full Gmail-class webmail, wired to the lilmail /v1 API.
 *
 * @param {object} props
 * @param {string} [props.baseUrl='/v1']
 * @param {object} [props.client]   - pre-built client (overrides baseUrl; tests/demo)
 * @param {(draft)=>(void|Promise<void>)} [props.onSend] - override default send
 * @param {(err)=>void} [props.onAuthError]
 */
export default function MailApp({ baseUrl = '/v1', client: clientProp, onSend, onAuthError }) {
  const client = useMemo(() => clientProp ?? createMailClient({ baseUrl }), [clientProp, baseUrl])
  const sendDraft = useMemo(() => onSend ?? ((d) => client.sendMessage(d)), [onSend, client])

  const [settings, setSettings] = useSettings()
  const [me, setMe] = useState(null)
  const [folders, setFolders] = useState([])
  const [folder, setFolder] = useState('INBOX')
  const [query, setQuery] = useState('')
  const [messages, setMessages] = useState([])
  const [listLoading, setListLoading] = useState(true)
  const [listError, setListError] = useState('')

  const [openThread, setOpenThread] = useState(null)
  const [fullById, setFullById] = useState({})
  const [selection, setSelection] = useState(() => new Set())
  const [focusIdx, setFocusIdx] = useState(-1)

  const [composes, setComposes] = useState([])
  const [panel, setPanel] = useState('none')        // none | calendar | contacts | settings
  const [helpOpen, setHelpOpen] = useState(false)
  const [mobilePane, setMobilePane] = useState('list')  // list | read
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [railCollapsed, setRailCollapsed] = useState(false)
  const [toasts, setToasts] = useState([])
  const [moveSupported, setMoveSupported] = useState(true)

  const searchRef = useRef(null)

  // Apply theme to the app root.
  const rootRef = useRef(null)

  // Pending deferred-commit timers for undoable (destructive) actions.
  const undoTimers = useRef(new Map())
  useEffect(() => () => { for (const t of undoTimers.current.values()) clearTimeout(t) }, [])

  const handleError = useCallback((e) => {
    if (e?.status === 401) onAuthError?.(e)
    return e?.message || 'Something went wrong'
  }, [onAuthError])

  const toast = useCallback((text, kind = 'info') => {
    const id = ++composeSeq
    setToasts((t) => [...t, { id, text, kind }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 3200)
  }, [])

  // An undoable toast: `commit` runs when the undo window lapses; `undo` runs if
  // the user clicks Undo first (the destructive server call is deferred until
  // commit, so undoing is a clean re-fetch — nothing was sent to the server).
  const undoable = useCallback((text, commit, undo) => {
    const id = ++composeSeq
    const timer = setTimeout(() => {
      undoTimers.current.delete(id)
      setToasts((t) => t.filter((x) => x.id !== id))
      commit()
    }, UNDO_MS)
    undoTimers.current.set(id, timer)
    setToasts((t) => [...t, {
      id, text, kind: 'info',
      undo: () => {
        clearTimeout(timer)
        undoTimers.current.delete(id)
        setToasts((tt) => tt.filter((x) => x.id !== id))
        undo()
      },
    }])
  }, [])

  // ── Bootstrap ───────────────────────────────────────────────────────────
  useEffect(() => {
    let live = true
    client.me().then((m) => live && setMe(m)).catch(() => {})
    client.listFolders().then((f) => live && setFolders(f || [])).catch(() => {})
    return () => { live = false }
  }, [client])

  // Archive target folder (from special-use); archive hidden when absent.
  const archiveFolder = useMemo(() => {
    const f = folders.find((x) => classifyFolder(x) === 'archive')
    return f ? (f.path ?? f.name) : null
  }, [folders])
  const trashFolder = useMemo(() => {
    const f = folders.find((x) => classifyFolder(x) === 'trash')
    return f ? (f.path ?? f.name) : null
  }, [folders])
  const canArchive = moveSupported && Boolean(archiveFolder)

  // ── List loading ─────────────────────────────────────────────────────────
  const loadList = useCallback(async () => {
    setListLoading(true)
    setListError('')
    try {
      let msgs
      if (folder === STARRED_FOLDER) {
        const all = await client.listMessages({ folder: 'INBOX', limit: 200 })
        msgs = all.filter((m) => (m.flags || []).includes(FLAG_FLAGGED))
      } else if (query) {
        msgs = await client.search(query, { folder: folder === STARRED_FOLDER ? 'INBOX' : folder })
      } else {
        msgs = await client.listMessages({ folder })
      }
      setMessages(msgs || [])
    } catch (e) {
      setListError(handleError(e))
      setMessages([])
    } finally {
      setListLoading(false)
    }
  }, [client, folder, query, handleError])

  useEffect(() => { loadList() }, [loadList])

  const threads = useMemo(
    () => groupThreads(messages, { threaded: settings.threaded && folder !== STARRED_FOLDER }),
    [messages, settings.threaded, folder],
  )
  const starredCount = useMemo(
    () => messages.filter((m) => (m.flags || []).includes(FLAG_FLAGGED)).length,
    [messages],
  )

  // Keep focusIdx in range.
  useEffect(() => { if (focusIdx >= threads.length) setFocusIdx(threads.length - 1) }, [threads.length, focusIdx])

  // ── Message state patching (optimistic) ───────────────────────────────────
  const patchFlags = useCallback((ids, flag, add) => {
    const idSet = new Set(ids)
    const apply = (m) => {
      if (!idSet.has(m.id)) return m
      const f = new Set(m.flags || [])
      if (add) f.add(flag); else f.delete(flag)
      return { ...m, flags: [...f] }
    }
    setMessages((list) => list.map(apply))
    setOpenThread((t) => t ? { ...t, messages: t.messages.map(apply) } : t)
    setFullById((map) => {
      const next = { ...map }
      for (const id of ids) if (next[id]) next[id] = apply(next[id])
      return next
    })
  }, [])

  const removeIds = useCallback((ids) => {
    const idSet = new Set(ids)
    setMessages((list) => list.filter((m) => !idSet.has(m.id)))
  }, [])

  // ── Open / read ────────────────────────────────────────────────────────────
  const needBody = useCallback(async (id) => {
    if (fullById[id]?.__full) return
    try {
      const full = await client.getMessage(id, { folder: starredFolderSrc(folder) })
      setFullById((m) => ({ ...m, [id]: { ...full, __full: true } }))
      if (!(full.flags || []).includes(FLAG_SEEN)) {
        patchFlags([id], FLAG_SEEN, true)
        client.setFlag(id, FLAG_SEEN, true, { folder: starredFolderSrc(folder) }).catch(() => {})
      }
    } catch (e) { handleError(e) }
  }, [client, folder, fullById, patchFlags, handleError])

  const openThreadFn = useCallback((thread) => {
    setOpenThread(thread)
    setMobilePane('read')
    setFocusIdx(threads.findIndex((t) => t.id === thread.id))
    // Mark all unread messages in the thread read (optimistic).
    const unreadIds = thread.messages.filter((m) => !(m.flags || []).includes(FLAG_SEEN)).map((m) => m.id)
    if (unreadIds.length) {
      patchFlags(unreadIds, FLAG_SEEN, true)
      for (const id of unreadIds) client.setFlag(id, FLAG_SEEN, true, { folder: starredFolderSrc(folder) }).catch(() => {})
    }
  }, [threads, client, folder, patchFlags])

  // ── Targets helper: a passed thread, else the current selection ────────────
  const targetsOf = useCallback((thread) => {
    if (thread) return [thread]
    return threads.filter((t) => selection.has(t.id))
  }, [threads, selection])

  // ── Star ───────────────────────────────────────────────────────────────────
  const toggleStar = useCallback((thread, next) => {
    const targets = targetsOf(thread)
    for (const t of targets) {
      if (next) {
        patchFlags([t.latest.id], FLAG_FLAGGED, true)
        client.setFlag(t.latest.id, FLAG_FLAGGED, true, { folder: starredFolderSrc(folder) }).catch((e) => { handleError(e); loadList() })
      } else {
        const flaggedIds = t.messages.filter((m) => (m.flags || []).includes(FLAG_FLAGGED)).map((m) => m.id)
        patchFlags(flaggedIds, FLAG_FLAGGED, false)
        for (const id of flaggedIds) client.setFlag(id, FLAG_FLAGGED, false, { folder: starredFolderSrc(folder) }).catch((e) => { handleError(e); loadList() })
      }
    }
    if (!thread) setSelection(new Set())
  }, [targetsOf, patchFlags, client, folder, handleError, loadList])

  // ── Read / unread ────────────────────────────────────────────────────────
  const toggleRead = useCallback((thread, read) => {
    const targets = targetsOf(thread)
    const ids = targets.flatMap((t) => t.messages.map((m) => m.id))
    patchFlags(ids, FLAG_SEEN, read)
    for (const id of ids) client.setFlag(id, FLAG_SEEN, read, { folder: starredFolderSrc(folder) }).catch((e) => { handleError(e); loadList() })
    if (!thread) setSelection(new Set())
  }, [targetsOf, patchFlags, client, folder, handleError, loadList])

  // ── Delete (to Trash) ──────────────────────────────────────────────────────
  const deleteThreads = useCallback((thread) => {
    const targets = targetsOf(thread)
    if (!targets.length) return
    const ids = targets.flatMap((t) => t.messages.map((m) => m.id))
    const src = starredFolderSrc(folder)
    removeIds(ids)
    if (openThread && targets.some((t) => t.id === openThread.id)) { setOpenThread(null); setMobilePane('list') }
    setSelection(new Set())
    undoable(
      `Deleted ${targets.length > 1 ? targets.length + ' conversations' : 'conversation'}`,
      () => { for (const id of ids) client.deleteMessage(id, { folder: src }).catch((e) => { handleError(e); loadList() }) },
      () => loadList(),
    )
  }, [targetsOf, removeIds, openThread, client, folder, handleError, loadList, undoable])

  // ── Archive (move to Archive) ──────────────────────────────────────────────
  const archiveThreads = useCallback((thread) => {
    if (!canArchive) return
    const targets = targetsOf(thread)
    if (!targets.length) return
    const ids = targets.flatMap((t) => t.messages.map((m) => m.id))
    const src = starredFolderSrc(folder)
    removeIds(ids)
    if (openThread && targets.some((t) => t.id === openThread.id)) { setOpenThread(null); setMobilePane('list') }
    setSelection(new Set())
    undoable(
      `Archived ${targets.length > 1 ? targets.length + ' conversations' : 'conversation'}`,
      () => {
        Promise.all(ids.map((id) => client.moveMessage(id, archiveFolder, { folder: src }))).catch((e) => {
          if (e instanceof ApiError && (e.status === 404 || e.status === 405)) {
            setMoveSupported(false)
            toast('Archive is not available on this server', 'error')
          } else {
            handleError(e)
          }
          loadList()
        })
      },
      () => loadList(),
    )
  }, [canArchive, targetsOf, removeIds, openThread, client, archiveFolder, folder, handleError, loadList, toast, undoable])

  // ── Selection ──────────────────────────────────────────────────────────────
  const toggleSelect = useCallback((id) => {
    setSelection((s) => { const n = new Set(s); n.has(id) ? n.delete(id) : n.add(id); return n })
  }, [])
  const selectRange = useCallback((ids) => {
    setSelection((s) => { const n = new Set(s); for (const id of ids) n.add(id); return n })
  }, [])
  const selectAll = useCallback((on) => {
    setSelection(on ? new Set(threads.map((t) => t.id)) : new Set())
  }, [threads])

  // ── Compose ────────────────────────────────────────────────────────────────
  const openCompose = useCallback((initial = {}) => {
    setComposes((c) => [...c, { id: ++composeSeq, initial }])
  }, [])
  const closeCompose = useCallback((id) => setComposes((c) => c.filter((x) => x.id !== id)), [])

  const replyTo = useCallback((message, mode) => {
    const base = (message.subject || '').replace(/^\s*(re|fwd?|aw)\s*:\s*/i, '')
    if (mode === 'forward') {
      openCompose({ subject: 'Fwd: ' + base, html: quoteForward(message) })
    } else {
      openCompose({
        to: message.from,
        cc: mode === 'replyAll' ? replyAllCc(message, me?.email) : '',
        subject: 'Re: ' + base,
        html: quoteReply(message),
        inReplyTo: message.messageId,
        references: [...(message.references || []), message.messageId].filter(Boolean),
      })
    }
  }, [openCompose, me])

  // ── Folder / search nav ────────────────────────────────────────────────────
  const selectFolder = useCallback((f) => {
    setFolder(f); setQuery(''); setOpenThread(null); setSelection(new Set())
    setMobilePane('list'); setDrawerOpen(false); setPanel('none')
  }, [])
  const runSearch = useCallback((q) => { setQuery(q); setOpenThread(null); setMobilePane('list') }, [])
  const clearSearch = useCallback(() => { setQuery(''); setOpenThread(null) }, [])

  const togglePanel = useCallback((name) => setPanel((p) => (p === name ? 'none' : name)), [])

  // ── Keyboard shortcuts ─────────────────────────────────────────────────────
  const moveFocus = useCallback((delta) => {
    setFocusIdx((i) => {
      const n = Math.max(0, Math.min(threads.length - 1, (i < 0 ? 0 : i + delta)))
      return n
    })
  }, [threads.length])

  const kbHandlers = useMemo(() => ({
    next: () => moveFocus(1),
    prev: () => moveFocus(-1),
    open: () => { const t = threads[focusIdx]; if (t) openThreadFn(t) },
    back: () => { setOpenThread(null); setMobilePane('list') },
    archive: () => { const t = openThread || threads[focusIdx]; if (t) archiveThreads(t) },
    delete: () => { const t = openThread || threads[focusIdx]; if (t) deleteThreads(t) },
    star: () => { const t = openThread || threads[focusIdx]; if (t) toggleStar(t, !t.starred) },
    select: () => { const t = threads[focusIdx]; if (t) toggleSelect(t.id) },
    reply: () => { const t = openThread; if (t) replyTo(fullById[t.latest.id] || t.latest, 'reply') },
    replyAll: () => { const t = openThread; if (t) replyTo(fullById[t.latest.id] || t.latest, 'replyAll') },
    forward: () => { const t = openThread; if (t) replyTo(fullById[t.latest.id] || t.latest, 'forward') },
    compose: () => openCompose(),
    search: () => searchRef.current?.focus(),
    help: () => setHelpOpen(true),
    escape: () => {
      if (helpOpen) setHelpOpen(false)
      else if (composes.length) closeCompose(composes[composes.length - 1].id)
      else if (panel !== 'none') setPanel('none')
      else if (openThread) { setOpenThread(null); setMobilePane('list') }
    },
  }), [threads, focusIdx, openThread, fullById, moveFocus, openThreadFn, archiveThreads, deleteThreads, toggleStar, toggleSelect, replyTo, openCompose, helpOpen, panel, composes, closeCompose])

  useKeyboard(kbHandlers, settings.shortcuts)

  const contactSearch = useCallback((q) => client.listContacts({ q, limit: 6 }).catch(() => []), [client])

  return (
    <div
      ref={rootRef}
      className="vm-app"
      data-theme={settings.theme}
      data-density={settings.density}
      data-rp={settings.readingPane}
      data-open={openThread ? '1' : '0'}
      data-pane={mobilePane}
      data-drawer={drawerOpen ? '1' : '0'}
      data-panel-open={panel !== 'none' ? '1' : '0'}
    >
      {drawerOpen && <div className="vm-scrim" onClick={() => setDrawerOpen(false)} aria-hidden="true" />}

      <FolderList
        folders={folders}
        current={folder}
        me={me}
        collapsed={railCollapsed}
        starredCount={starredCount}
        onToggleCollapse={() => setRailCollapsed((v) => !v)}
        onSelect={selectFolder}
        onCompose={() => openCompose()}
        onOpenPanel={(name) => { setPanel(name); setDrawerOpen(false) }}
        onOpenHelp={() => { setHelpOpen(true); setDrawerOpen(false) }}
      />

      <div className="vm-main">
        <MessageList
          threads={threads}
          selectedId={openThread?.id ?? null}
          focusId={threads[focusIdx]?.id ?? null}
          selection={selection}
          onToggleSelect={toggleSelect}
          onSelectRange={selectRange}
          onSelectAll={selectAll}
          onOpen={openThreadFn}
          onCompose={() => openCompose()}
          onToggleStar={toggleStar}
          onArchive={archiveThreads}
          onDelete={deleteThreads}
          onToggleRead={toggleRead}
          onRefresh={loadList}
          loading={listLoading}
          error={listError}
          onRetry={loadList}
          query={query}
          onSearch={runSearch}
          onClearSearch={clearSearch}
          canArchive={canArchive}
          folder={folder}
          searchRef={searchRef}
          onMenu={() => setDrawerOpen(true)}
        />

        <MessageView
          thread={openThread}
          fullById={fullById}
          onNeedBody={needBody}
          canArchive={canArchive}
          onToggleStar={(next) => openThread && toggleStar(openThread, next)}
          onArchive={() => openThread && archiveThreads(openThread)}
          onDelete={() => openThread && deleteThreads(openThread)}
          onReply={(m) => replyTo(m, 'reply')}
          onReplyAll={(m) => replyTo(m, 'replyAll')}
          onForward={(m) => replyTo(m, 'forward')}
          onBack={() => { setOpenThread(null); setMobilePane('list') }}
        />
      </div>

      {/* Far-right app rail (Gmail-style side panel toggles). */}
      <aside className="vm-rightrail" aria-label="Side panels">
        <button type="button" className={'vm-iconbtn' + (panel === 'calendar' ? ' vm-on' : '')} aria-label="Calendar" title="Calendar" onClick={() => togglePanel('calendar')}><Icon name="calendar" /></button>
        <button type="button" className={'vm-iconbtn' + (panel === 'contacts' ? ' vm-on' : '')} aria-label="Contacts" title="Contacts" onClick={() => togglePanel('contacts')}><Icon name="users" /></button>
        <button type="button" className={'vm-iconbtn' + (panel === 'settings' ? ' vm-on' : '')} aria-label="Settings" title="Settings" onClick={() => togglePanel('settings')}><Icon name="settings" /></button>
        <span className="vm-spacer" />
        <button type="button" className="vm-iconbtn" aria-label="Keyboard shortcuts" title="Keyboard shortcuts" onClick={() => setHelpOpen(true)}><Icon name="keyboard" /></button>
      </aside>

      {panel !== 'none' && (
        <aside className="vm-panel" aria-label={panel}>
          {panel === 'settings' && <Settings settings={settings} onChange={setSettings} onClose={() => setPanel('none')} />}
          {panel === 'calendar' && (
            <div className="vm-panel-embed">
              <div className="vm-panel-head"><h2><Icon name="calendar" className="vm-icon" /> Calendar</h2><button type="button" className="vm-iconbtn" aria-label="Close" onClick={() => setPanel('none')}><Icon name="close" /></button></div>
              <Calendar client={client} defaultView="agenda" onAuthError={onAuthError} />
            </div>
          )}
          {panel === 'contacts' && (
            <div className="vm-panel-embed">
              <div className="vm-panel-head"><h2><Icon name="users" className="vm-icon" /> Contacts</h2><button type="button" className="vm-iconbtn" aria-label="Close" onClick={() => setPanel('none')}><Icon name="close" /></button></div>
              <Contacts client={client} onSelect={(c) => { openCompose({ to: c.email }); setPanel('none') }} onAuthError={onAuthError} />
            </div>
          )}
        </aside>
      )}

      {/* Mobile compose FAB. */}
      <button type="button" className="vm-fab" aria-label="Compose" onClick={() => openCompose()}><Icon name="pencil" /></button>

      <div className="vm-compose-stack">
        {composes.map((c, i) => (
          <div key={c.id} className="vm-compose-slot" style={{ '--slot': i }}>
            <Compose
              initial={c.initial}
              signature={settings.signature}
              onContactSearch={contactSearch}
              onSaveDraft={(d) => client.saveDraft(d)}
              onSend={async (d) => { await sendDraft(d); toast('Message sent', 'success'); loadList() }}
              onClose={() => closeCompose(c.id)}
            />
          </div>
        ))}
      </div>

      {helpOpen && <ShortcutsHelp onClose={() => setHelpOpen(false)} />}

      <div className="vm-toasts" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={'vm-toast vm-toast-' + t.kind}>
            <span className="vm-toast-text">{t.text}</span>
            {t.undo && <button type="button" className="vm-toast-action" onClick={t.undo}>Undo</button>}
          </div>
        ))}
      </div>
    </div>
  )
}

/** Starred is a virtual view over INBOX; map it back to a real source folder. */
function starredFolderSrc(folder) {
  return folder === STARRED_FOLDER ? 'INBOX' : folder
}
