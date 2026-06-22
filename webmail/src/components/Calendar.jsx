import { useEffect, useRef, useState } from "react";
import Icon from "./Icon.jsx";
import { useToast } from "./Toasts.jsx";

const WD = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const dayKey = (d) => d.getFullYear() + "-" + (d.getMonth() + 1) + "-" + d.getDate();
const sameDay = (a, b) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
function dtLocal(d) {
  const p = (n) => String(n).padStart(2, "0");
  return d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate()) + "T" + p(d.getHours()) + ":" + p(d.getMinutes());
}

export default function Calendar({ jmap, onClose }) {
  const toast = useToast();
  const [events, setEvents] = useState([]);
  const [month, setMonth] = useState(() => new Date());
  const [view, setView] = useState("month");
  const [pop, setPop] = useState(null);  // null | { editing, title, when }
  const titleRef = useRef(null);

  useEffect(() => { (async () => { try { setEvents(await jmap.events()); } catch { setEvents([]); } })(); }, [jmap]);
  useEffect(() => { if (pop) setTimeout(() => titleRef.current?.focus(), 0); }, [pop]);

  function openPop(ev, day) {
    let when;
    if (ev) when = new Date(ev.start);
    else { when = new Date(day || month); when.setHours(9, 0, 0, 0); }
    setPop({ editing: ev || null, title: ev ? (ev.summary || "") : "", when: dtLocal(when) });
  }

  async function save(e) {
    e.preventDefault();
    const summary = pop.title.trim();
    if (!summary) return;
    const start = pop.when ? new Date(pop.when).toISOString() : new Date().toISOString();
    const old = pop.editing;
    try {
      const r = await jmap.addEvent({ summary, start, end: "" });
      setEvents((xs) => [...xs, { id: r.id, summary, start, end: "" }]);
      if (old) { await jmap.delEvent(old.id); setEvents((xs) => xs.filter((x) => x.id !== old.id)); }
      setPop(null); toast(old ? "Event saved" : "Event added");
    } catch (ex) { toast(ex.message); }
  }

  async function del() {
    if (!pop?.editing) return;
    const id = pop.editing.id;
    await jmap.delEvent(id);
    setEvents((xs) => xs.filter((x) => x.id !== id));
    setPop(null); toast("Event deleted");
  }

  const byDay = {};
  for (const ev of events) (byDay[dayKey(new Date(ev.start))] ||= []).push(ev);
  for (const k in byDay) byDay[k].sort((a, b) => new Date(a.start) - new Date(b.start));

  const y = month.getFullYear(), m = month.getMonth();
  const start = new Date(y, m, 1); start.setDate(1 - start.getDay());
  const today = new Date();
  const cells = Array.from({ length: 42 }, (_, i) => { const d = new Date(start); d.setDate(start.getDate() + i); return d; });

  const agenda = events.slice().sort((a, b) => new Date(a.start) - new Date(b.start));
  let lastDay = "";

  return (
    <div className="overlay" id="calendar" onClick={(e) => { if (e.target.id === "calendar") onClose(); }}>
      <div className="cal-card">
        <header className="cal-top">
          <div className="cal-nav">
            <button className="iconbtn" id="cal-prev" title="Previous" onClick={() => setMonth(new Date(y, m - 1, 1))}><Icon body='<path d="m15 18-6-6 6-6"/>' /></button>
            <button className="iconbtn" id="cal-next" title="Next" onClick={() => setMonth(new Date(y, m + 1, 1))}><Icon body='<path d="m9 18 6-6-6-6"/>' /></button>
            <button className="btn btn-ghost" id="cal-today" onClick={() => setMonth(new Date())}>Today</button>
            <h2 id="cal-title">{month.toLocaleDateString([], { month: "long", year: "numeric" })}</h2>
          </div>
          <div className="cal-actions">
            <div className="seg" id="cal-view">
              <button data-view="month" className={view === "month" ? "on" : ""} onClick={() => setView("month")}>Month</button>
              <button data-view="agenda" className={view === "agenda" ? "on" : ""} onClick={() => setView("agenda")}>Agenda</button>
            </div>
            <button className="btn btn-primary" id="cal-new" onClick={() => openPop(null, new Date())}>+ Event</button>
            <button className="iconbtn" id="calendar-close" title="Close" onClick={onClose}><Icon body='<path d="M18 6 6 18M6 6l12 12"/>' /></button>
          </div>
        </header>

        <div className="cal-weekdays" id="cal-weekdays" hidden={view !== "month"}>
          {WD.map((d) => <span key={d}>{d}</span>)}
        </div>
        <div className="cal-grid" id="cal-grid" hidden={view !== "month"}>
          {cells.map((d, i) => {
            const evs = byDay[dayKey(d)] || [];
            const shown = evs.length > 3 ? 2 : evs.length;
            return (
              <div key={i} className={"cal-cell" + (d.getMonth() !== m ? " other" : "") + (sameDay(d, today) ? " today" : "")}
                onClick={() => openPop(null, d)}>
                <span className="cal-daynum">{d.getDate()}</span>
                {evs.slice(0, shown).map((ev, j) => {
                  const t = new Date(ev.start).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
                  return (
                    <div key={j} className="cal-pill" title={t + "  " + (ev.summary || "(untitled)")}
                      onClick={(e) => { e.stopPropagation(); openPop(ev); }}>
                      <span className="pt">{t}</span><span className="pn">{ev.summary || "(untitled)"}</span>
                    </div>
                  );
                })}
                {evs.length > shown && <div className="cal-more">+{evs.length - shown} more</div>}
              </div>
            );
          })}
        </div>

        <div className="cal-agenda" id="calendar-list" hidden={view !== "agenda"}>
          {agenda.length === 0 ? (
            <div className="contacts-empty">No events yet. Click a day or + Event to add one.</div>
          ) : (
            agenda.map((ev) => {
              const d = new Date(ev.start);
              const day = d.toLocaleDateString([], { weekday: "long", month: "short", day: "numeric" });
              const head = day !== lastDay ? day : null; lastDay = day;
              return (
                <div key={ev.id}>
                  {head && <div className="cal-day">{head}</div>}
                  <div className="cal-ev" onClick={() => openPop(ev)}>
                    <span className="cal-time">{d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}</span>
                    <span className="cal-dot" /><span className="cal-title">{ev.summary || "(untitled)"}</span>
                  </div>
                </div>
              );
            })
          )}
        </div>

        {pop && (
          <div className="cal-pop" id="cal-pop" onClick={(e) => { if (e.target.id === "cal-pop") setPop(null); }}>
            <form id="event-form" onSubmit={save}>
              <div className="cal-pop-title" id="cal-pop-head">{pop.editing ? "Edit event" : "New event"}</div>
              <input id="event-title" ref={titleRef} type="text" placeholder="Add title" required
                value={pop.title} onChange={(e) => setPop((p) => ({ ...p, title: e.target.value }))} />
              <input id="event-when" type="datetime-local" value={pop.when}
                onChange={(e) => setPop((p) => ({ ...p, when: e.target.value }))} />
              <div className="cal-pop-actions">
                {pop.editing && <button type="button" className="btn btn-ghost" id="cal-pop-del" onClick={del}>Delete</button>}
                <span className="cal-pop-spacer" />
                <button type="button" className="btn btn-ghost" id="cal-pop-cancel" onClick={() => setPop(null)}>Cancel</button>
                <button className="btn btn-primary" type="submit" id="cal-pop-save">{pop.editing ? "Save" : "Add"}</button>
              </div>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
