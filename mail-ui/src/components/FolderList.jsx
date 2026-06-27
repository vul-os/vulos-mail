import Icon from './Icon.jsx'

export const STARRED_FOLDER = '__starred'

/** Order in which special folders appear (Gmail-like). */
const SPECIAL_ORDER = ['inbox', 'starred', 'sent', 'drafts', 'archive', 'trash', 'junk']

/** Classify a mailbox into a special kind (or 'label') by special-use + name. */
export function classifyFolder(f) {
  const attrs = (f.attributes || f.Attributes || []).map((a) => String(a).toLowerCase())
  const name = String(f.name ?? f.path ?? '').toLowerCase()
  const has = (s) => attrs.includes('\\' + s) || name === s || name.endsWith('/' + s)
  if (name === 'inbox' || attrs.includes('\\inbox')) return 'inbox'
  if (has('sent') || name.includes('sent')) return 'sent'
  if (has('drafts') || name.includes('draft')) return 'drafts'
  if (has('trash') || name.includes('trash') || name.includes('deleted') || name === 'bin') return 'trash'
  if (has('archive') || name.includes('archive')) return 'archive'
  if (has('junk') || name.includes('junk') || name.includes('spam')) return 'junk'
  return 'label'
}

const ICON_FOR = {
  inbox: 'inbox', starred: 'star', sent: 'send', drafts: 'draft',
  archive: 'archive', trash: 'trash', junk: 'tag', label: 'tag',
}

const LABEL_FOR = {
  inbox: 'Inbox', starred: 'Starred', sent: 'Sent', drafts: 'Drafts',
  archive: 'Archive', trash: 'Trash', junk: 'Spam',
}

/**
 * <FolderList/> — left rail. Special folders (with unread counts) first, then
 * user labels. Collapsible to an icon-only rail.
 */
export default function FolderList({
  folders = [], current, onSelect, onCompose, me,
  collapsed = false, onToggleCollapse, starredCount = 0,
  onOpenPanel, onOpenHelp,
}) {
  // Bucket real folders by kind, keep first match per special kind.
  const specials = {}
  const labels = []
  for (const f of folders) {
    const path = f.path ?? f.name ?? f.id
    const kind = classifyFolder(f)
    const entry = {
      path, kind,
      label: LABEL_FOR[kind] ?? (f.name ?? f.path ?? path),
      unread: f.unread ?? f.unseen ?? f.UnreadCount ?? 0,
    }
    if (kind === 'label') labels.push(entry)
    else if (!specials[kind]) specials[kind] = entry
  }

  // Inject the virtual Starred view between Inbox and Sent.
  specials.starred = { path: STARRED_FOLDER, kind: 'starred', label: 'Starred', unread: 0, count: starredCount }

  const ordered = SPECIAL_ORDER.map((k) => specials[k]).filter(Boolean)

  const renderItem = (it) => {
    const active = it.path === current
    return (
      <li key={it.path}>
        <button
          type="button"
          className={'vm-folder' + (active ? ' vm-active' : '')}
          aria-current={active ? 'true' : undefined}
          onClick={() => onSelect?.(it.path)}
          title={it.label}
        >
          <Icon name={ICON_FOR[it.kind] || 'tag'} className="vm-icon" />
          <span className="vm-folder-name">{it.label}</span>
          {it.unread > 0 && <span className="vm-folder-count">{it.unread}</span>}
        </button>
      </li>
    )
  }

  return (
    <nav className={'vm-sidebar' + (collapsed ? ' vm-collapsed' : '')} aria-label="Mailboxes">
      <div className="vm-brand">
        <button
          type="button"
          className="vm-iconbtn vm-rail-toggle"
          aria-label={collapsed ? 'Expand menu' : 'Collapse menu'}
          onClick={onToggleCollapse}
        >
          <Icon name="menu" />
        </button>
        <span className="vm-brand-text">
          <Icon name="mail" className="vm-icon vm-brand-mark" />
          <span>Vulos Mail</span>
        </span>
      </div>

      <button type="button" className="vm-compose-btn" onClick={onCompose} title="Compose">
        <Icon name="pencil" />
        <span className="vm-compose-label">Compose</span>
      </button>

      <ul className="vm-folders">
        {ordered.map(renderItem)}
        {labels.length > 0 && (
          <li className="vm-folder-section" aria-hidden="true"><span>Labels</span></li>
        )}
        {labels.map(renderItem)}
      </ul>

      {/* Mobile-only: Calendar / Contacts / Settings / Shortcuts otherwise live
          in the far-right rail, which is hidden ≤768px. */}
      {(onOpenPanel || onOpenHelp) && (
        <ul className="vm-folders vm-drawer-extra" aria-label="Tools">
          <li className="vm-folder-section" aria-hidden="true"><span>More</span></li>
          {onOpenPanel && (
            <>
              <li>
                <button type="button" className="vm-folder" onClick={() => onOpenPanel('calendar')} title="Calendar">
                  <Icon name="calendar" className="vm-icon" /><span className="vm-folder-name">Calendar</span>
                </button>
              </li>
              <li>
                <button type="button" className="vm-folder" onClick={() => onOpenPanel('contacts')} title="Contacts">
                  <Icon name="users" className="vm-icon" /><span className="vm-folder-name">Contacts</span>
                </button>
              </li>
              <li>
                <button type="button" className="vm-folder" onClick={() => onOpenPanel('settings')} title="Settings">
                  <Icon name="settings" className="vm-icon" /><span className="vm-folder-name">Settings</span>
                </button>
              </li>
            </>
          )}
          {onOpenHelp && (
            <li>
              <button type="button" className="vm-folder" onClick={onOpenHelp} title="Keyboard shortcuts">
                <Icon name="keyboard" className="vm-icon" /><span className="vm-folder-name">Shortcuts</span>
              </button>
            </li>
          )}
        </ul>
      )}

      {me?.email && (
        <div className="vm-sidebar-foot" title={me.email}>
          <span className="vm-me-avatar" aria-hidden="true">{(me.email[0] || '?').toUpperCase()}</span>
          <span className="vm-me">{me.email}</span>
        </div>
      )}
    </nav>
  )
}
