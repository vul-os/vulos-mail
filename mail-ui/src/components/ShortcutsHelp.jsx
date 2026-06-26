import Icon from './Icon.jsx'

const GROUPS = [
  {
    title: 'Navigation',
    items: [
      ['j', 'Next conversation'],
      ['k', 'Previous conversation'],
      ['Enter / o', 'Open conversation'],
      ['u', 'Back to list'],
      ['/', 'Search'],
    ],
  },
  {
    title: 'Actions',
    items: [
      ['e', 'Archive'],
      ['#', 'Delete'],
      ['s', 'Star'],
      ['x', 'Select'],
      ['c', 'Compose'],
    ],
  },
  {
    title: 'Reply',
    items: [
      ['r', 'Reply'],
      ['a', 'Reply all'],
      ['f', 'Forward'],
      ['Esc', 'Close'],
      ['?', 'This help'],
    ],
  },
]

/** <ShortcutsHelp/> — the `?` keyboard-shortcuts overlay. */
export default function ShortcutsHelp({ onClose }) {
  return (
    <div className="vm-overlay" role="dialog" aria-modal="true" aria-label="Keyboard shortcuts"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose?.() }}>
      <div className="vm-help">
        <header className="vm-help-head">
          <h2><Icon name="keyboard" className="vm-icon" /> Keyboard shortcuts</h2>
          <button type="button" className="vm-iconbtn" aria-label="Close" onClick={onClose}><Icon name="close" /></button>
        </header>
        <div className="vm-help-grid">
          {GROUPS.map((g) => (
            <div key={g.title} className="vm-help-col">
              <h3>{g.title}</h3>
              <dl>
                {g.items.map(([key, desc]) => (
                  <div key={key} className="vm-help-row">
                    <dt><kbd>{key}</kbd></dt>
                    <dd>{desc}</dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
