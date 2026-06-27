import Icon from './Icon.jsx'

/** A segmented control. */
function Seg({ value, onChange, options, ariaLabel }) {
  return (
    <div className="vm-segctl" role="radiogroup" aria-label={ariaLabel}>
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={value === o.value}
          className={'vm-seg' + (value === o.value ? ' vm-on' : '')} onClick={() => onChange(o.value)}>
          {o.icon && <Icon name={o.icon} />} {o.label}
        </button>
      ))}
    </div>
  )
}

/** A labelled settings section with an optional description. */
function Section({ title, children }) {
  return (
    <section className="vm-set-section">
      <h3 className="vm-set-section-title">{title}</h3>
      <div className="vm-set-group">{children}</div>
    </section>
  )
}

/**
 * <Settings/> — preferences panel. Appearance, layout, reading and composing
 * preferences (persisted locally via useSettings). Renders inside the right side
 * panel. A host app can inject server-backed account controls via `extra` (e.g.
 * the standalone webmail's account / connection / change-password surface).
 *
 * @param {object} props
 * @param {object} props.settings
 * @param {(patch)=>void} props.onChange
 * @param {()=>void} [props.onClose]
 * @param {import('react').ReactNode} [props.extra] - host-supplied section(s),
 *   rendered first (above the built-in preferences).
 */
export default function Settings({ settings, onChange, onClose, extra }) {
  const set = (patch) => onChange?.(patch)
  return (
    <div className="vm-settings">
      <header className="vm-panel-head">
        <h2><Icon name="settings" className="vm-icon" /> Settings</h2>
        <button type="button" className="vm-iconbtn" aria-label="Close settings" onClick={onClose}><Icon name="close" /></button>
      </header>

      <div className="vm-panel-body">
        {extra}

        <Section title="Appearance">
          <div className="vm-set-row">
            <label className="vm-set-label">Theme</label>
            <Seg value={settings.theme} onChange={(v) => set({ theme: v })} ariaLabel="Theme"
              options={[
                { value: 'system', label: 'Auto', icon: 'contrast' },
                { value: 'dark', label: 'Dark', icon: 'moon' },
                { value: 'light', label: 'Light', icon: 'sun' },
              ]} />
            <p className="vm-set-desc">Auto follows your operating system’s light or dark setting.</p>
          </div>

          <div className="vm-set-row">
            <label className="vm-set-label">Density</label>
            <Seg value={settings.density} onChange={(v) => set({ density: v })} ariaLabel="Density"
              options={[{ value: 'comfortable', label: 'Comfortable' }, { value: 'compact', label: 'Compact' }]} />
          </div>
        </Section>

        <Section title="Layout">
          <div className="vm-set-row">
            <label className="vm-set-label">Reading pane</label>
            <Seg value={settings.readingPane} onChange={(v) => set({ readingPane: v })} ariaLabel="Reading pane"
              options={[{ value: 'right', label: 'Right' }, { value: 'bottom', label: 'Bottom' }, { value: 'off', label: 'No split' }]} />
            <p className="vm-set-desc">Where an opened conversation appears next to the message list.</p>
          </div>

          <div className="vm-set-row vm-set-inline">
            <span className="vm-set-line">
              <label className="vm-set-label" htmlFor="vm-set-threaded">Conversation view</label>
              <span className="vm-set-desc">Group replies into a single thread.</span>
            </span>
            <Toggle id="vm-set-threaded" checked={settings.threaded} onChange={(v) => set({ threaded: v })} />
          </div>
        </Section>

        <Section title="Reading &amp; shortcuts">
          <div className="vm-set-row vm-set-inline">
            <span className="vm-set-line">
              <label className="vm-set-label" htmlFor="vm-set-shortcuts">Keyboard shortcuts</label>
              <span className="vm-set-desc">Gmail-style keys: j/k, e, #, r, c… Press ? for the full list.</span>
            </span>
            <Toggle id="vm-set-shortcuts" checked={settings.shortcuts} onChange={(v) => set({ shortcuts: v })} />
          </div>
        </Section>

        <Section title="Composing">
          <div className="vm-set-row">
            <label className="vm-set-label" htmlFor="vm-set-sig">Signature</label>
            <textarea id="vm-set-sig" className="vm-set-textarea" value={settings.signature}
              placeholder="Appended to new messages…" rows={4}
              onChange={(e) => set({ signature: e.target.value })} />
            <p className="vm-set-desc">Added to the bottom of new messages and replies.</p>
          </div>
        </Section>
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
