import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Icon from "./Icon.jsx";
import { useToast } from "./Toasts.jsx";
import {
  STAR, SYS_ICONS, ORDER, kw, fromName, fromAddr, avatarColor, initials, esc, fmtDate,
} from "../lib/util.js";
import MessageList from "./MessageList.jsx";
import ReadPane from "./ReadPane.jsx";
import ComposeDock from "./ComposeDock.jsx";
import Settings from "./Settings.jsx";
import Contacts from "./Contacts.jsx";
import Calendar from "./Calendar.jsx";
import CommandPalette from "./CommandPalette.jsx";
import Shortcuts from "./Shortcuts.jsx";

const PAGE = 50; // Email/query window for paginated/infinite-scroll loading.

export default function Mail({ jmap, onLogout }) {
  const toast = useToast();

  const [labels, setLabels] = useState([]);
  const [current, setCurrent] = useState("inbox");
  const [currentName, setCurrentName] = useState("Inbox");
  const [rows, setRows] = useState([]);          // [{id, msg, threadCount}]
  const [total, setTotal] = useState(0);         // server-reported total in mailbox
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [filter, setFilter] = useState("");
  const [sel, setSel] = useState(-1);
  const [openId, setOpenId] = useState(null);
  const [openMsg, setOpenMsg] = useState(null);
  const [readBusy, setReadBusy] = useState(false);
  const [selected, setSelected] = useState(() => new Set());
  const [settings, setSettings] = useState({ signature: "", vacation: { enabled: false } });
  const [reading, setReading] = useState(false);
  // collapseThreads is forwarded to Email/query for forward-compat; the current
  // server ignores it and returns per-message ids (see TODO: server threading).
  const [collapseThreads] = useState(false);

  // Composers (multiple dockable). Each entry: {key, pre}.
  const [composers, setComposers] = useState([]);
  const composerKey = useRef(0);

  // Overlays
  const [showSettings, setShowSettings] = useState(false);
  const [showContacts, setShowContacts] = useState(false);
  const [showCalendar, setShowCalendar] = useState(false);
  const [showCmdk, setShowCmdk] = useState(false);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [showMore, setShowMore] = useState(false);
  const [mobileTab, setMobileTab] = useState("inbox");

  const positionRef = useRef(0);     // pagination cursor
  const hasMoreRef = useRef(false);
  const searchRef = useRef(null);
  const listElRef = useRef(null);

  // ── theme ──────────────────────────────────────────────────────────
  const applyTheme = useCallback((t) => {
    const theme = t === "light" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", theme);
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", theme === "light" ? "#fbfbfa" : "#0c0c0c");
  }, []);
  const setTheme = useCallback((t) => { try { localStorage.setItem("vulos-mail.theme", t); } catch {} applyTheme(t); }, [applyTheme]);
  const theme = (() => { try { return localStorage.getItem("vulos-mail.theme") || "dark"; } catch { return "dark"; } })();

  // ── labels ─────────────────────────────────────────────────────────
  const loadLabels = useCallback(async () => {
    try {
      const r = await jmap.mailboxes();
      const list = (r.list || []).sort((a, b) => {
        const ia = ORDER.indexOf(a.role || a.id), ib = ORDER.indexOf(b.role || b.id);
        return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) || a.name.localeCompare(b.name);
      });
      setLabels(list);
    } catch { /* ignore */ }
  }, [jmap]);

  // ── list ───────────────────────────────────────────────────────────
  const fetchPage = useCallback(async (mailboxId, position) => {
    const q = await jmap.query(mailboxId, { limit: PAGE, position, collapseThreads });
    const ids = q.ids || [];
    const tot = q.total != null ? q.total : ids.length;
    let mapped = [];
    if (ids.length) {
      const g = await jmap.emails(ids, ["id", "threadId", "from", "to", "subject", "receivedAt", "keywords", "mailboxIds", "preview", "size"]);
      const byId = Object.fromEntries((g.list || []).map((m) => [m.id, m]));
      mapped = ids.map((id) => byId[id]).filter(Boolean).map((m) => ({ id: m.id, msg: m }));
    }
    return { mapped, total: tot, count: ids.length };
  }, [jmap, collapseThreads]);

  const loadList = useCallback(async (mailboxId = current) => {
    setLoading(true);
    positionRef.current = 0;
    try {
      const { mapped, total: tot, count } = await fetchPage(mailboxId, 0);
      setRows(mapped);
      setTotal(tot);
      positionRef.current = count;
      hasMoreRef.current = count >= PAGE && positionRef.current < tot;
    } catch (ex) {
      setRows([]); setTotal(0); hasMoreRef.current = false;
      toast("Couldn't load: " + ex.message);
    } finally {
      setLoading(false);
    }
  }, [current, fetchPage, toast]);

  const loadMore = useCallback(async () => {
    if (!hasMoreRef.current || loadingMore || filter) return;
    setLoadingMore(true);
    try {
      const { mapped, total: tot, count } = await fetchPage(current, positionRef.current);
      setRows((prev) => {
        const seen = new Set(prev.map((r) => r.id));
        return [...prev, ...mapped.filter((r) => !seen.has(r.id))];
      });
      setTotal(tot);
      positionRef.current += count;
      hasMoreRef.current = count >= PAGE && positionRef.current < tot;
    } catch { /* ignore */ } finally {
      setLoadingMore(false);
    }
  }, [current, fetchPage, loadingMore, filter]);

  const selectLabel = useCallback((id, name) => {
    setCurrent(id); setCurrentName(name);
    setSel(-1); setOpenId(null); setOpenMsg(null); setFilter("");
    setSelected(new Set());
    setReading(false);
    if (searchRef.current) searchRef.current.value = "";
    const m = labels.find((l) => l.id === id);
    setMobileTab((m && m.role === "inbox") || id === "inbox" ? "inbox" : "labels");
    loadList(id);
  }, [labels, loadList]);

  // Initial boot.
  useEffect(() => {
    (async () => {
      try { const s = await jmap.getSettings(); if (s) setSettings(s); } catch {}
      await loadLabels();
      await loadList("inbox");
      // Offer to resume an unsent draft.
      const d = readDraft(jmap.user);
      if (d && (d.to || d.subject || (d.html && d.html.replace(/<[^>]*>/g, "").trim()))) {
        setTimeout(() => toast("Unsent draft restored", () => openCompose({ draft: d })), 600);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Reload list when thread-collapse toggles.
  useEffect(() => { loadList(current); /* eslint-disable-next-line */ }, [collapseThreads]);

  // ── live updates (SSE) ─────────────────────────────────────────────
  useEffect(() => {
    let es, reloadT;
    (async () => {
      try {
        const token = await jmap.pushToken();
        es = new EventSource("/api/webmail/changes?token=" + encodeURIComponent(token));
        es.onmessage = () => {
          clearTimeout(reloadT);
          reloadT = setTimeout(() => { loadList(); loadLabels(); }, 400);
        };
        es.onerror = () => {};
      } catch { /* push unavailable */ }
    })();
    return () => { if (es) es.close(); clearTimeout(reloadT); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── visible rows (client search filter, ported 1:1) ────────────────
  const visibleRows = useMemo(() => {
    if (!filter) return rows;
    const f = filter.toLowerCase();
    return rows.filter(({ msg }) =>
      (msg.subject || "").toLowerCase().includes(f) ||
      (fromName(msg) + " " + fromAddr(msg)).toLowerCase().includes(f) ||
      (msg.preview || "").toLowerCase().includes(f));
  }, [rows, filter]);

  // ── keyword mutation helpers ───────────────────────────────────────
  const patchRow = useCallback((id, fn) => {
    setRows((prev) => prev.map((r) => (r.id === id ? { ...r, msg: fn({ ...r.msg }) } : r)));
  }, []);
  const removeRow = useCallback((id) => {
    setRows((prev) => prev.filter((r) => r.id !== id));
    setOpenId((cur) => (cur === id ? null : cur));
    if (openId === id) { setReading(false); setOpenMsg(null); }
  }, [openId]);

  const setSeen = useCallback(async (row, seen) => {
    patchRow(row.id, (m) => { m.keywords = { ...(m.keywords || {}) }; if (seen) m.keywords.$seen = true; else delete m.keywords.$seen; return m; });
    try { await jmap.set({ [row.id]: { "keywords/$seen": seen ? true : null } }); } catch {}
    loadLabels();
  }, [jmap, patchRow, loadLabels]);

  const toggleStar = useCallback(async (row) => {
    const on = !kw(row.msg, "$flagged");
    patchRow(row.id, (m) => { m.keywords = { ...(m.keywords || {}) }; if (on) m.keywords.$flagged = true; else delete m.keywords.$flagged; return m; });
    setOpenMsg((cur) => (cur && cur.id === row.id ? { ...cur, keywords: { ...(cur.keywords || {}), ...(on ? { $flagged: true } : {}) } } : cur));
    if (!on) setOpenMsg((cur) => { if (cur && cur.id === row.id) { const k = { ...(cur.keywords || {}) }; delete k.$flagged; return { ...cur, keywords: k }; } return cur; });
    try { await jmap.set({ [row.id]: { "keywords/$flagged": on ? true : null } }); } catch {}
    toast(on ? "Starred" : "Unstarred");
  }, [jmap, patchRow, toast]);

  const archive = useCallback(async (row) => {
    const id = row.id;
    if (current === "inbox") removeRow(id);
    try { await jmap.set({ [id]: { "mailboxIds/inbox": null } }); } catch {}
    toast("Archived", async () => { await jmap.set({ [id]: { "mailboxIds/inbox": true } }); loadList(); });
  }, [jmap, current, removeRow, toast, loadList]);

  const trash = useCallback(async (row) => {
    const id = row.id;
    removeRow(id);
    try { await jmap.set({ [id]: { "mailboxIds/inbox": null, "mailboxIds/trash": true } }); } catch {}
    toast("Deleted", async () => { await jmap.set({ [id]: { "mailboxIds/inbox": true, "mailboxIds/trash": null } }); loadList(); });
  }, [jmap, removeRow, toast, loadList]);

  // ── open / read ────────────────────────────────────────────────────
  const openRow = useCallback(async (i) => {
    const vr = visibleRows;
    if (i < 0 || i >= vr.length) return;
    const row = vr[i];
    setSel(i); setOpenId(row.id); setReading(true); setReadBusy(true); setOpenMsg(null);
    try {
      const g = await jmap.emails([row.id], ["id", "threadId", "from", "to", "cc", "subject", "receivedAt", "keywords", "mailboxIds", "bodyValues", "attachments", "preview", "size"]);
      const m = (g.list || [])[0];
      if (!m) return;
      const merged = { ...row.msg, ...m };
      setOpenMsg(merged);
      patchRow(row.id, (x) => ({ ...x, ...m }));
      if (!kw(merged, "$seen")) setSeen(row, true);
    } catch (ex) {
      setOpenMsg({ __error: ex.message });
    } finally {
      setReadBusy(false);
    }
  }, [visibleRows, jmap, patchRow, setSeen]);

  const backToList = useCallback(() => {
    setOpenId(null); setReading(false); setOpenMsg(null);
    listElRef.current?.focus();
  }, []);

  const replyTo = useCallback((msg) => {
    openCompose({
      to: fromAddr(msg),
      subject: /^re:/i.test(msg.subject || "") ? msg.subject : "Re: " + (msg.subject || ""),
      text: "\n\n———\nOn " + (msg.receivedAt ? new Date(msg.receivedAt).toLocaleString() : "") + ", " + fromName(msg) + " wrote:\n> " +
        ((msg.bodyValues ? (Object.values(msg.bodyValues)[0]?.value || "") : (msg.preview || "")).split("\n").join("\n> ")),
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── compose ────────────────────────────────────────────────────────
  const openCompose = useCallback((pre = {}) => {
    setComposers((cs) => [...cs, { key: ++composerKey.current, pre }]);
  }, []);
  const closeComposer = useCallback((key) => {
    setComposers((cs) => cs.filter((c) => c.key !== key));
  }, []);

  // ── multi-select ───────────────────────────────────────────────────
  const toggleSel = useCallback((row) => {
    setSelected((prev) => { const n = new Set(prev); if (n.has(row.id)) n.delete(row.id); else n.add(row.id); return n; });
  }, []);
  const clearSel = useCallback(() => setSelected(new Set()), []);

  const bulk = useCallback(async (kind) => {
    const ids = [...selected];
    if (!ids.length) return;
    if (kind === "read") {
      setRows((prev) => prev.map((r) => selected.has(r.id) ? { ...r, msg: { ...r.msg, keywords: { ...(r.msg.keywords || {}), $seen: true } } } : r));
      clearSel();
      const u = {}; ids.forEach((id) => (u[id] = { "keywords/$seen": true }));
      try { await jmap.set(u); } catch {}
      loadLabels();
      return;
    }
    if (kind === "star") {
      setRows((prev) => prev.map((r) => selected.has(r.id) ? { ...r, msg: { ...r.msg, keywords: { ...(r.msg.keywords || {}), $flagged: true } } } : r));
      clearSel();
      const u = {}; ids.forEach((id) => (u[id] = { "keywords/$flagged": true }));
      try { await jmap.set(u); } catch {}
      toast(ids.length + " starred");
      return;
    }
    if (kind === "archive") {
      setRows((prev) => prev.filter((r) => !selected.has(r.id))); clearSel();
      const u = {}; ids.forEach((id) => (u[id] = { "mailboxIds/inbox": null }));
      try { await jmap.set(u); } catch {}
      toast(ids.length + " archived", async () => { const un = {}; ids.forEach((id) => (un[id] = { "mailboxIds/inbox": true })); await jmap.set(un); loadList(); });
      return;
    }
    if (kind === "trash") {
      setRows((prev) => prev.filter((r) => !selected.has(r.id))); clearSel();
      const u = {}; ids.forEach((id) => (u[id] = { "mailboxIds/inbox": null, "mailboxIds/trash": true }));
      try { await jmap.set(u); } catch {}
      toast(ids.length + " deleted", async () => { const un = {}; ids.forEach((id) => (un[id] = { "mailboxIds/inbox": true, "mailboxIds/trash": null })); await jmap.set(un); loadList(); });
    }
  }, [selected, clearSel, jmap, toast, loadList, loadLabels]);

  const markAllRead = useCallback(async () => {
    const ids = rows.filter((r) => !kw(r.msg, "$seen")).map((r) => r.id);
    if (!ids.length) { toast("Nothing unread"); return; }
    setRows((prev) => prev.map((r) => ({ ...r, msg: { ...r.msg, keywords: { ...(r.msg.keywords || {}), $seen: true } } })));
    const u = {}; ids.forEach((id) => (u[id] = { "keywords/$seen": true }));
    try { await jmap.set(u); } catch {}
    loadLabels(); toast("Marked " + ids.length + " read");
  }, [rows, jmap, toast, loadLabels]);

  const refresh = useCallback(() => { loadList().then(loadLabels); }, [loadList, loadLabels]);

  // ── keyboard shortcuts ─────────────────────────────────────────────
  const gPending = useRef(false);
  useEffect(() => {
    const onKey = (e) => {
      // Command palette: works everywhere.
      if ((e.metaKey || e.ctrlKey) && (e.key === "k" || e.key === "K")) { e.preventDefault(); setShowCmdk(true); return; }
      if (showCmdk) return; // palette handles its own keys
      const typing = /^(input|textarea)$/i.test(e.target.tagName) || e.target.isContentEditable;
      if (typing) {
        if (e.key === "Escape" && !e.target.closest(".compose")) {
          if (showContacts) { e.preventDefault(); setShowContacts(false); return; }
          if (showSettings) { e.preventDefault(); setShowSettings(false); return; }
          if (showCalendar) { e.preventDefault(); setShowCalendar(false); return; }
          e.target.blur();
        } else if (e.key === "Escape") e.target.blur();
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const vr = visibleRows;
      if (e.key === "x") {
        let idx = sel; if (idx < 0) { idx = 0; setSel(0); }
        const r = vr[idx]; if (r) { toggleSel(r); setSel((s) => Math.min((s < 0 ? 0 : s) + 1, vr.length - 1)); }
        return;
      }
      if (gPending.current) { gPending.current = false; if (e.key === "i") selectLabel("inbox", "Inbox"); return; }
      switch (e.key) {
        case "j": setSel((s) => Math.max(0, Math.min(vr.length - 1, (s < 0 ? 0 : s + 1)))); break;
        case "k": setSel((s) => Math.max(0, Math.min(vr.length - 1, (s < 0 ? 0 : s - 1)))); break;
        case "Enter": if (sel >= 0) openRow(sel); break;
        case "u": backToList(); break;
        case "c": e.preventDefault(); openCompose(); break;
        case "/": e.preventDefault(); searchRef.current?.focus(); break;
        case "g": gPending.current = true; setTimeout(() => (gPending.current = false), 700); break;
        case "e": if (vr[sel]) archive(vr[sel]); break;
        case "#": if (vr[sel]) trash(vr[sel]); break;
        case "s": if (vr[sel]) toggleStar(vr[sel]); break;
        case "r": if (vr[sel]) replyTo(vr[sel].msg); break;
        case "?": setShowShortcuts(true); break;
        case "Escape":
          if (showMore) { setShowMore(false); break; }
          if (showContacts) { setShowContacts(false); break; }
          if (showSettings) { setShowSettings(false); break; }
          if (showCalendar) { setShowCalendar(false); break; }
          setShowShortcuts(false); break;
        default: break;
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [showCmdk, showContacts, showSettings, showCalendar, showMore, visibleRows, sel, selectLabel, openRow, backToList, openCompose, archive, trash, toggleStar, replyTo, toggleSel]);

  // Scroll the selected row into view on j/k navigation.
  useEffect(() => {
    if (sel < 0) return;
    const elr = document.querySelector(`.row[data-i="${sel}"]`);
    if (elr) elr.scrollIntoView({ block: "nearest" });
  }, [sel]);

  // Commands for the palette.
  const commands = useMemo(() => {
    const c = [
      { label: "Compose new message", k: "c", ic: '<path d="M4 20h4L18.5 9.5a2.1 2.1 0 0 0-3-3L5 17v3z"/><path d="m13.5 6.5 3 3"/>', run: () => openCompose() },
      { label: "Search mail", k: "/", ic: '<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>', run: () => searchRef.current?.focus() },
      { label: "Refresh", ic: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>', run: refresh },
      { label: "Mark all as read", ic: SYS_ICONS.inbox, run: markAllRead },
      { label: "Contacts", ic: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.9"/>', run: () => setShowContacts(true) },
      { label: "Calendar", ic: '<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>', run: () => setShowCalendar(true) },
      { label: "Settings", ic: '<circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 0 0-.1-1l2-1.5-2-3.4-2.3 1a7 7 0 0 0-1.7-1l-.4-2.6h-4l-.4 2.6a7 7 0 0 0-1.7 1l-2.3-1-2 3.4L5 11a7 7 0 0 0 0 2l-2 1.5 2 3.4 2.3-1a7 7 0 0 0 1.7 1l.4 2.6h4l.4-2.6a7 7 0 0 0 1.7-1l2.3 1 2-3.4-2-1.5a7 7 0 0 0 .1-1z"/>', run: () => setShowSettings(true) },
      { label: "Keyboard shortcuts", k: "?", ic: '<circle cx="12" cy="12" r="10"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 2.5-3 4"/><path d="M12 17h.01"/>', run: () => setShowShortcuts(true) },
      { label: "Sign out", ic: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>', run: onLogout },
    ];
    for (const m of labels) c.push({ label: "Go to " + m.name, ic: SYS_ICONS[m.role || m.id] || SYS_ICONS._, run: () => selectLabel(m.id, m.name) });
    return c;
  }, [openCompose, refresh, markAllRead, onLogout, labels, selectLabel]);

  return (
    <main id="app" className={"app" + (reading ? " reading" : "")}>
      {/* Sidebar */}
      <nav className="sidebar" id="sidebar">
        <div className="brand brand-sm">
          <span className="logo" aria-hidden="true" />
          <span className="wordmark">Vulos Mail</span>
        </div>
        <button className="btn btn-primary compose-btn" id="compose-btn" title="Compose (c)" onClick={() => openCompose()}>
          <Icon body='<path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-3-3L5 17v3z" /><path d="M13.5 6.5l3 3" />' />
          Compose
        </button>
        <ul className="labels" id="labels">
          {labels.map((m) => {
            const key = m.role || m.id;
            return (
              <li key={m.id}
                className={"label" + (m.id === current ? " active" : "") + (m.unreadEmails > 0 ? " has-unread" : "")}
                onClick={() => selectLabel(m.id, m.name)}>
                <Icon body={SYS_ICONS[key] || SYS_ICONS._} className="label-ic" />
                <span className="label-name">{m.name}</span>
                <span className="label-count">{m.unreadEmails > 0 ? m.unreadEmails : (m.totalEmails || "")}</span>
              </li>
            );
          })}
        </ul>
        <div className="sidebar-foot">
          <div className="me" id="me">{jmap.user || ""}</div>
          <button className="iconbtn" id="calendar-btn" title="Calendar" onClick={() => setShowCalendar(true)}>
            <Icon body='<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>' />
          </button>
          <button className="iconbtn" id="contacts-btn" title="Contacts" onClick={() => setShowContacts(true)}>
            <Icon body='<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.9"/><path d="M16 3.1a4 4 0 0 1 0 7.8"/>' />
          </button>
          <button className="iconbtn" id="settings-btn" title="Settings" onClick={() => setShowSettings(true)}>
            <Icon body='<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1A1.7 1.7 0 0 0 9 19.3a1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 4.7 9a1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>' />
          </button>
          <button className="iconbtn" id="logout" title="Sign out" onClick={onLogout}>
            <Icon body='<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5"/><path d="M21 12H9"/>' />
          </button>
        </div>
      </nav>

      {/* Message list */}
      <MessageList
        title={currentName}
        rows={visibleRows}
        total={total}
        loading={loading}
        loadingMore={loadingMore}
        filter={filter}
        sel={sel}
        openId={openId}
        selected={selected}
        searchRef={searchRef}
        listElRef={listElRef}
        onSearch={setFilter}
        onRefresh={refresh}
        onOpenRow={openRow}
        onToggleStar={toggleStar}
        onToggleSel={toggleSel}
        onClearSel={clearSel}
        onBulk={bulk}
        onLoadMore={loadMore}
      />

      {/* Read pane */}
      <ReadPane
        jmap={jmap}
        msg={openMsg}
        busy={readBusy}
        labels={labels}
        reading={reading}
        onBack={backToList}
        onArchive={() => openMsg && archive({ id: openMsg.id, msg: openMsg })}
        onTrash={() => openMsg && trash({ id: openMsg.id, msg: openMsg })}
        onToggleStar={() => openMsg && toggleStar({ id: openMsg.id, msg: openMsg })}
        onMarkUnread={() => { if (openMsg) { setSeen({ id: openMsg.id, msg: openMsg }, false); backToList(); } }}
        onReply={() => openMsg && replyTo(openMsg)}
      />

      {/* Mobile bottom nav */}
      <nav className="mobilenav" id="mobilenav" aria-label="Primary">
        <button className={"mobilenav-item" + (mobileTab === "inbox" ? " on" : "")} onClick={() => selectLabel("inbox", "Inbox")} title="Inbox">
          <Icon body='<path d="M3 12h5l2 3h4l2-3h5"/><path d="M5 5h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2z"/>' /><span>Inbox</span>
        </button>
        <button className={"mobilenav-item" + (mobileTab === "search" ? " on" : "")} onClick={() => { setReading(false); searchRef.current?.focus(); setMobileTab("search"); }} title="Search">
          <Icon body='<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>' /><span>Search</span>
        </button>
        <button className="mobilenav-item mobilenav-compose" onClick={() => openCompose()} title="Compose">
          <Icon body='<path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-3-3L5 17v3z"/><path d="M13.5 6.5l3 3"/>' /><span>Compose</span>
        </button>
        <button className={"mobilenav-item" + (mobileTab === "labels" ? " on" : "")} onClick={() => setShowMore("folders")} title="Folders">
          <Icon body='<path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/>' /><span>Folders</span>
        </button>
        <button className="mobilenav-item" onClick={() => setShowMore("more")} title="More">
          <Icon body='<circle cx="5" cy="12" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="19" cy="12" r="1.6"/>' /><span>More</span>
        </button>
      </nav>

      {/* Compose dock */}
      <div className="compose-dock" id="compose-dock" aria-live="polite">
        {composers.map((c) => (
          <ComposeDock key={c.key} jmap={jmap} settings={settings} pre={c.pre}
            onClose={() => closeComposer(c.key)} onReopen={openCompose} />
        ))}
      </div>

      {/* Overlays */}
      {showCmdk && <CommandPalette commands={commands} onClose={() => setShowCmdk(false)} />}
      {showShortcuts && <Shortcuts onClose={() => setShowShortcuts(false)} />}
      {showSettings && <Settings jmap={jmap} settings={settings} setSettings={setSettings} theme={theme} setTheme={setTheme} onClose={() => setShowSettings(false)} />}
      {showContacts && <Contacts jmap={jmap} onClose={() => setShowContacts(false)} onCompose={(to) => { setShowContacts(false); openCompose({ to }); }} />}
      {showCalendar && <Calendar jmap={jmap} onClose={() => setShowCalendar(false)} />}

      {/* Mobile sheet */}
      {showMore && (
        <div className="sheet" id="moresheet" onClick={(e) => { if (e.target.id === "moresheet") setShowMore(false); }}>
          <div className="sheet-panel" role="dialog" aria-label="More">
            <div className="sheet-grab" aria-hidden="true" />
            {showMore === "folders" ? (
              <>
                {labels.map((m) => {
                  const key = m.role || m.id;
                  return (
                    <button key={m.id} className="sheet-item" onClick={() => { setShowMore(false); selectLabel(m.id, m.name); }}>
                      <Icon body={SYS_ICONS[key] || SYS_ICONS._} /> {m.name}
                      {m.unreadEmails > 0 && <span className="label-count" style={{ marginLeft: "auto" }}>{m.unreadEmails}</span>}
                    </button>
                  );
                })}
              </>
            ) : (
              <>
                <button className="sheet-item" onClick={() => { setShowMore(false); setShowCalendar(true); }}>
                  <Icon body='<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>' /> Calendar
                </button>
                <button className="sheet-item" onClick={() => { setShowMore(false); setShowContacts(true); }}>
                  <Icon body='<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/>' /> Contacts
                </button>
                <button className="sheet-item" onClick={() => { setShowMore(false); setShowSettings(true); }}>
                  <Icon body='<circle cx="12" cy="12" r="3"/>' /> Settings
                </button>
                <button className="sheet-item" onClick={onLogout}>
                  <Icon body='<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5"/><path d="M21 12H9"/>' /> Sign out
                </button>
              </>
            )}
            <button className="btn btn-ghost btn-block" onClick={() => setShowMore(false)}>Close</button>
          </div>
        </div>
      )}
    </main>
  );
}

// ── single local draft (keyed per account), shared with ComposeDock ─────
const DRAFT_KEY = (user) => "vulos-mail.draft." + (user || "");
export function readDraft(user) { try { return JSON.parse(localStorage.getItem(DRAFT_KEY(user)) || "null"); } catch { return null; } }
export function writeDraft(user, d) { try { localStorage.setItem(DRAFT_KEY(user), JSON.stringify(d)); } catch {} }
export function clearDraft(user) { try { localStorage.removeItem(DRAFT_KEY(user)); } catch {} }
