import Icon from './Icon.jsx'

/** Map a few well-known folder names to icons; default to a folder glyph. */
function iconFor(name = '') {
  const n = name.toLowerCase()
  if (n === 'inbox') return 'inbox'
  if (n.includes('sent')) return 'send'
  if (n.includes('trash') || n.includes('deleted')) return 'trash'
  if (n.includes('archive')) return 'archive'
  if (n.includes('draft')) return 'edit'
  if (n.includes('star') || n.includes('flag')) return 'star'
  return 'folder'
}

/**
 * <FolderList/> — left pane. Renders MailboxInfo[] from /v1/folders.
 *
 * @param {object} props
 * @param {Array}  props.folders  - MailboxInfo[] (name/path + optional counts)
 * @param {string} props.current  - active folder path
 * @param {(folder: string) => void} props.onSelect
 * @param {() => void} [props.onCompose]
 * @param {{email?: string}} [props.me]
 */
export default function FolderList({ folders = [], current, onSelect, onCompose, me }) {
  return (
    <nav className="vm-sidebar" aria-label="Mailboxes">
      <div className="vm-brand">
        <Icon name="mail" className="vm-icon vm-brand-mark" />
        <span>Vulos Mail</span>
      </div>

      <button type="button" className="vm-compose-btn" onClick={onCompose}>
        <Icon name="edit" /> Compose
      </button>

      <ul className="vm-folders">
        {folders.map((f) => {
          const path = f.path ?? f.name ?? f.id
          const label = f.name ?? f.path ?? path
          const unread = f.unread ?? f.unseen
          return (
            <li key={path}>
              <button
                type="button"
                className={'vm-folder' + (path === current ? ' vm-active' : '')}
                aria-current={path === current ? 'true' : undefined}
                onClick={() => onSelect?.(path)}
              >
                <Icon name={iconFor(label)} className="vm-icon" />
                <span className="vm-folder-name">{label}</span>
                {unread > 0 && <span className="vm-folder-count">{unread}</span>}
              </button>
            </li>
          )
        })}
      </ul>

      {me?.email && (
        <div className="vm-sidebar-foot">
          <span className="vm-me" title={me.email}>{me.email}</span>
        </div>
      )}
    </nav>
  )
}
