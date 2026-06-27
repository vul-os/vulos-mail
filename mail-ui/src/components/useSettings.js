/**
 * useSettings.js — localStorage-persisted UI preferences (density, reading-pane
 * position, theme, keyboard-shortcuts toggle, signature, threading).
 */
import { useCallback, useEffect, useState } from 'react'

const KEY = 'vulos-mail.settings.v1'

export const DEFAULT_SETTINGS = {
  density: 'comfortable',     // 'comfortable' | 'compact'
  readingPane: 'right',       // 'right' | 'bottom' | 'off'
  theme: 'system',           // 'system' (follow OS) | 'dark' | 'light'
  shortcuts: true,
  threaded: true,
  signature: '',
}

function load() {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULT_SETTINGS }
    return { ...DEFAULT_SETTINGS, ...JSON.parse(raw) }
  } catch {
    return { ...DEFAULT_SETTINGS }
  }
}

export function useSettings() {
  const [settings, setSettings] = useState(load)

  useEffect(() => {
    try { localStorage.setItem(KEY, JSON.stringify(settings)) } catch { /* ignore */ }
  }, [settings])

  const set = useCallback((patch) => {
    setSettings((s) => ({ ...s, ...(typeof patch === 'function' ? patch(s) : patch) }))
  }, [])

  return [settings, set]
}
