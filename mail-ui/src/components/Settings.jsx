import Icon from './Icon.jsx'

/** A segmented control. */
function Seg({ value, onChange, options }) {
  return (
    <div className="vm-segctl" role="radiogroup">
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={value === o.value}
          className={'vm-seg' + (value === o.value ? ' vm-on' : '')} onClick={() => onChange(o.value)}>
          {o.icon && <Icon name={o.icon} />} {o.label}
        </button>
      ))}
    </div>
  )
}

/**
 * <Settings/> — preferences panel (density, reading-pane, theme, shortcuts,
 * threading, signature). Renders inside the right side panel.
 */
export default function Settings({ settings, onChange, onClose }) {
  const set = (patch) => onChange?.(patch)
  return (
    <div className="vm-settings">
      <header className="vm-panel-head">
        <h2><Icon name="settings" className="vm-icon" /> Settings</h2>
        <button type="button" className="vm-iconbtn" aria-label="Close settings" onClick={onClose}><Icon name="close" /></button>
      </header>

      <div className="vm-panel-body">
        <div className="vm-set-row">
          <label className="vm-set-label">Density</label>
          <Seg value={settings.density} onChange={(v) => set({ density: v })}
            options={[{ value: 'comfortable', label: 'Comfortable' }, { value: 'compact', label: 'Compact' }]} />
        </div>

        <div className="vm-set-row">
          <label className="vm-set-label">Reading pane</label>
          <Seg value={settings.readingPane} onChange={(v) => set({ readingPane: v })}
            options={[{ value: 'right', label: 'Right' }, { value: 'bottom', label: 'Bottom' }, { value: 'off', label: 'No split' }]} />
        </div>

        <div className="vm-set-row">
          <label className="vm-set-label">Theme</label>
          <Seg value={settings.theme} onChange={(v) => set({ theme: v })}
            options={[{ value: 'dark', label: 'Dark', icon: 'moon' }, { value: 'light', label: 'Light', icon: 'sun' }]} />
        </div>

        <div className="vm-set-row vm-set-inline">
          <label className="vm-set-label" htmlFor="vm-set-threaded">Conversation view</label>
          <Toggle id="vm-set-threaded" checked={settings.threaded} onChange={(v) => set({ threaded: v })} />
        </div>

        <div className="vm-set-row vm-set-inline">
          <label className="vm-set-label" htmlFor="vm-set-shortcuts">Keyboard shortcuts</label>
          <Toggle id="vm-set-shortcuts" checked={settings.shortcuts} onChange={(v) => set({ shortcuts: v })} />
        </div>

        <div className="vm-set-row">
          <label className="vm-set-label" htmlFor="vm-set-sig">Signature</label>
          <textarea id="vm-set-sig" className="vm-set-textarea" value={settings.signature}
            placeholder="Appended to new messages…" rows={4}
            onChange={(e) => set({ signature: e.target.value })} />
        </div>
      </div>
    </div>
  )
}

function Toggle({ id, checked, onChange }) {
  return (
    <button id={id} type="button" role="switch" aria-checked={checked}
      className={'vm-toggle' + (checked ? ' vm-on' : '')} onClick={() => onChange(!checked)}>
      <span className="vm-toggle-knob" />
    </button>
  )
}
