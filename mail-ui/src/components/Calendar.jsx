import { useCallback, useEffect, useMemo, useState } from 'react'
import { createMailClient } from '../api.js'
import Icon from './Icon.jsx'
import '../index.css'

const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
const MONTHS = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]

const sameDay = (a, b) =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()

/** Build a 6×7 grid of Date cells (Monday-first) covering the month of `anchor`. */
function monthGrid(anchor) {
  const first = new Date(anchor.getFullYear(), anchor.getMonth(), 1)
  const offset = (first.getDay() + 6) % 7 // Mon=0
  const gridStart = new Date(first)
  gridStart.setDate(1 - offset)
  return Array.from({ length: 42 }, (_, i) => {
    const d = new Date(gridStart)
    d.setDate(gridStart.getDate() + i)
    return d
  })
}

function fmtTime(iso) {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

/**
 * <Calendar/> — month + agenda views over the /v1 calendar API.
 *
 * @param {object} props
 * @param {string} [props.baseUrl='/v1']
 * @param {object} [props.client]        - pre-built client (overrides baseUrl)
 * @param {(err) => void} [props.onAuthError]
 */
export default function Calendar({ baseUrl = '/v1', client: clientProp, onAuthError }) {
  const client = useMemo(() => clientProp ?? createMailClient({ baseUrl }), [clientProp, baseUrl])

  const [anchor, setAnchor] = useState(() => new Date())
  const [view, setView] = useState('month') // 'month' | 'agenda'
  const [events, setEvents] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const handleError = useCallback((e) => {
    if (e?.status === 401) onAuthError?.(e)
    return e?.message || 'Could not load calendar'
  }, [onAuthError])

  // Range = whole grid (covers leading/trailing days of adjacent months).
  const [rangeStart, rangeEnd] = useMemo(() => {
    const grid = monthGrid(anchor)
    const start = grid[0]
    const end = new Date(grid[41])
    end.setDate(end.getDate() + 1)
    return [start, end]
  }, [anchor])

  useEffect(() => {
    let live = true
    setLoading(true)
    setError('')
    client.listEvents({ start: rangeStart, end: rangeEnd })
      .then((evs) => { if (live) setEvents(evs) })
      .catch((e) => { if (live) { setError(handleError(e)); setEvents([]) } })
      .finally(() => { if (live) setLoading(false) })
    return () => { live = false }
  }, [client, rangeStart, rangeEnd, handleError])

  const eventsByDay = useMemo(() => {
    const map = new Map()
    for (const ev of events) {
      const d = new Date(ev.start)
      if (Number.isNaN(d.getTime())) continue
      const key = d.toDateString()
      if (!map.has(key)) map.set(key, [])
      map.get(key).push(ev)
    }
    return map
  }, [events])

  const agenda = useMemo(
    () => [...events].sort((a, b) => new Date(a.start) - new Date(b.start)),
    [events],
  )

  const step = (delta) => setAnchor((a) => new Date(a.getFullYear(), a.getMonth() + delta, 1))
  const today = new Date()
  const grid = monthGrid(anchor)

  return (
    <div className="vm-cal">
      <header className="vm-cal-head">
        <div className="vm-cal-nav">
          <button type="button" className="vm-iconbtn" aria-label="Previous month" onClick={() => step(-1)}>
            <Icon name="prev" />
          </button>
          <button type="button" className="vm-btn vm-btn-ghost vm-cal-today" onClick={() => setAnchor(new Date())}>
            Today
          </button>
          <button type="button" className="vm-iconbtn" aria-label="Next month" onClick={() => step(1)}>
            <Icon name="next" />
          </button>
          <h2 className="vm-cal-title">{MONTHS[anchor.getMonth()]} {anchor.getFullYear()}</h2>
        </div>
        <div className="vm-cal-views" role="tablist" aria-label="Calendar view">
          <button type="button" role="tab" aria-selected={view === 'month'}
            className={'vm-seg' + (view === 'month' ? ' vm-on' : '')} onClick={() => setView('month')}>
            <Icon name="grid" /> Month
          </button>
          <button type="button" role="tab" aria-selected={view === 'agenda'}
            className={'vm-seg' + (view === 'agenda' ? ' vm-on' : '')} onClick={() => setView('agenda')}>
            <Icon name="list" /> Agenda
          </button>
        </div>
      </header>

      {error && <div className="vm-error" role="alert">{error}</div>}

      {view === 'month' ? (
        <div className="vm-cal-grid" aria-busy={loading}>
          {WEEKDAYS.map((w) => <div key={w} className="vm-cal-dow">{w}</div>)}
          {grid.map((d) => {
            const dayEvents = eventsByDay.get(d.toDateString()) || []
            const muted = d.getMonth() !== anchor.getMonth()
            return (
              <div key={d.toISOString()} className={'vm-cal-cell' + (muted ? ' vm-muted' : '')}>
                <span className={'vm-cal-num' + (sameDay(d, today) ? ' vm-today' : '')}>{d.getDate()}</span>
                <div className="vm-cal-evs">
                  {dayEvents.slice(0, 3).map((ev, i) => (
                    <span key={ev.uid || i} className="vm-cal-ev" title={ev.summary}>
                      {!ev.allDay && <em>{fmtTime(ev.start)}</em>} {ev.summary || '(busy)'}
                    </span>
                  ))}
                  {dayEvents.length > 3 && <span className="vm-cal-more">+{dayEvents.length - 3} more</span>}
                </div>
              </div>
            )
          })}
        </div>
      ) : (
        <div className="vm-agenda" aria-busy={loading}>
          {agenda.length === 0 ? (
            <div className="vm-empty">{loading ? 'Loading…' : 'No events this month'}</div>
          ) : (
            <ul className="vm-agenda-list">
              {agenda.map((ev, i) => (
                <li key={ev.uid || i} className="vm-agenda-row">
                  <div className="vm-agenda-when">
                    <span className="vm-agenda-date">{new Date(ev.start).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</span>
                    <span className="vm-agenda-time">{ev.allDay ? 'All day' : fmtTime(ev.start)}</span>
                  </div>
                  <div className="vm-agenda-main">
                    <span className="vm-agenda-sum">{ev.summary || '(no title)'}</span>
                    {ev.location && <span className="vm-agenda-loc">{ev.location}</span>}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}
