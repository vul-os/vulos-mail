// app.js — vulos-mail webmail controller. Vanilla JS, no build step.
(function () {
  "use strict";
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const el = (t, c, h) => { const e = document.createElement(t); if (c) e.className = c; if (h != null) e.innerHTML = h; return e; };

  const jmap = new JMAP("");
  const S = {
    labels: [], current: "inbox", currentName: "Inbox",
    rows: [],        // [{id, msg, thread}]
    sel: -1,         // index in rows
    openId: null,
    filter: "",
    selected: new Set(),
    settings: { signature: "", vacation: { enabled: false } },
  };

  const SYS_ICONS = {
    inbox: '<path d="M3 12h5l2 3h4l2-3h5"/><path d="M5 5h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2z"/>',
    sent: '<path d="M22 2 11 13"/><path d="M22 2 15 22l-4-9-9-4 20-7z"/>',
    drafts: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z"/>',
    trash: '<path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>',
    spam: '<path d="M12 2 2 7v6c0 5 4 8 10 9 6-1 10-4 10-9V7L12 2z"/><path d="M12 8v4"/><path d="M12 16h.01"/>',
    archive: '<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M10 12h4"/>',
    star: '<path d="m12 3 2.9 6 6.6.9-4.8 4.6 1.2 6.5L12 18l-5.9 3 1.2-6.5L2.5 9.9 9 9z"/>',
    important: '<path d="M3 5h13l4 7-4 7H3z"/>',
    snoozed: '<circle cx="12" cy="13" r="8"/><path d="M12 9v4l2 2"/><path d="M5 3 2 6"/><path d="m22 6-3-3"/>',
    _: '<path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/>',
  };
  const ORDER = ["inbox", "star", "important", "snoozed", "sent", "drafts", "archive", "spam", "trash"];

  // ── auth ──────────────────────────────────────────────────────────
  async function boot() {
    const saved = sessionStorage.getItem("vulos-mail.auth");
    if (saved) {
      const { u, p } = JSON.parse(saved);
      jmap.setAuth(u, p);
      try { await jmap.session(); return start(); } catch { sessionStorage.removeItem("vulos-mail.auth"); }
    }
    showLogin();
  }
  function showLogin() {
    $("#app").hidden = true; $("#login").hidden = false;
    $("#login-user").focus();
  }
  $("#login-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const u = $("#login-user").value.trim(), p = $("#login-pass").value;
    const btn = $("#login-btn"), err = $("#login-err");
    btn.disabled = true; btn.textContent = "Signing in…"; err.hidden = true;
    jmap.setAuth(u, p);
    try {
      await jmap.session();
      sessionStorage.setItem("vulos-mail.auth", JSON.stringify({ u, p }));
      start();
    } catch (ex) {
      err.textContent = ex.message; err.hidden = false;
      btn.disabled = false; btn.textContent = "Sign in";
    }
  });
  $("#logout").addEventListener("click", () => { sessionStorage.removeItem("vulos-mail.auth"); location.reload(); });

  // ── self-serve signup (free account) ──────────────────────────────────
  const showSignupView = () => { $("#login-form").hidden = true; $("#show-signup").parentElement.hidden = true; $("#signup-form").hidden = false; $("#signup-handle").focus(); };
  const showLoginView = () => { $("#signup-form").hidden = true; $("#login-form").hidden = false; $("#show-signup").parentElement.hidden = false; };
  $("#show-signup").addEventListener("click", (e) => { e.preventDefault(); showSignupView(); });
  $("#show-login").addEventListener("click", (e) => { e.preventDefault(); showLoginView(); });

  // Solve an Altcha proof-of-work challenge: find n with SHA-256(salt+n)==challenge.
  async function solveAltcha(ch) {
    const enc = new TextEncoder();
    for (let n = 0; n <= ch.maxnumber; n++) {
      const buf = await crypto.subtle.digest("SHA-256", enc.encode(ch.salt + n));
      const hex = [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
      if (hex === ch.challenge) {
        return btoa(JSON.stringify({ algorithm: ch.algorithm, challenge: ch.challenge, number: n, salt: ch.salt, signature: ch.signature }));
      }
    }
    throw new Error("could not solve challenge");
  }

  $("#signup-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const handle = $("#signup-handle").value.trim().toLowerCase(), pass = $("#signup-pass").value;
    const btn = $("#signup-btn"), err = $("#signup-err");
    err.hidden = true; btn.disabled = true; btn.textContent = "Creating account…";
    try {
      const ch = await fetch("/api/signup/challenge").then((r) => { if (!r.ok) throw new Error("signup unavailable"); return r.json(); });
      const solution = await solveAltcha(ch);
      const res = await fetch("/api/signup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ handle, password: pass, solution }) });
      if (!res.ok) { const j = await res.json().catch(() => ({})); throw new Error(j.error || "could not create account"); }
      const { address } = await res.json();
      // Seamlessly sign in to the new account.
      showLoginView();
      $("#login-user").value = address; $("#login-pass").value = pass;
      $("#login-form").requestSubmit();
    } catch (ex) {
      err.textContent = ex.message; err.hidden = false;
      btn.disabled = false; btn.textContent = "Create free account";
    }
  });

  function start() {
    $("#login").hidden = true; $("#app").hidden = false;
    $("#me").textContent = jmap.user || "";
    jmap.getSettings().then((s) => { if (s) S.settings = s; }).catch(() => {});
    loadLabels().then(() => selectLabel("inbox", "Inbox"));
    startPush();
  }

  // ── live updates (SSE) ────────────────────────────────────────────
  let pushReloadT, pushES;
  async function startPush() {
    try {
      if (pushES) { pushES.close(); pushES = null; } // never leak a prior stream
      const token = await jmap.pushToken();
      const es = new EventSource("/api/webmail/changes?token=" + encodeURIComponent(token));
      pushES = es;
      es.onmessage = () => {
        // Debounced refresh of the current view + label counts. The open message
        // and selection are preserved by loadList/renderList.
        clearTimeout(pushReloadT);
        pushReloadT = setTimeout(() => { loadList(); loadLabels(); }, 400);
      };
      es.onerror = () => {}; // EventSource auto-reconnects (server sends retry:)
    } catch { /* push unavailable; manual refresh still works */ }
  }

  // ── settings ──────────────────────────────────────────────────────
  function openSettings() {
    const s = S.settings || {};
    $("#set-sig").value = s.signature || "";
    const v = s.vacation || {};
    $("#set-vac").checked = !!v.enabled;
    $("#set-vac-subj").value = v.subject || "";
    $("#set-vac-body").value = v.body || "";
    $("#set-vac-fields").classList.toggle("on", !!v.enabled);
    $("#settings").hidden = false;
  }
  function closeSettings() { $("#settings").hidden = true; }
  $("#settings-btn").addEventListener("click", openSettings);
  $("#set-cancel").addEventListener("click", closeSettings);
  $("#settings").addEventListener("click", (e) => { if (e.target.id === "settings") closeSettings(); });
  $("#set-vac").addEventListener("change", (e) => $("#set-vac-fields").classList.toggle("on", e.target.checked));
  $("#set-save").addEventListener("click", async () => {
    const s = {
      signature: $("#set-sig").value,
      vacation: { enabled: $("#set-vac").checked, subject: $("#set-vac-subj").value, body: $("#set-vac-body").value },
    };
    const btn = $("#set-save"); btn.disabled = true; btn.textContent = "Saving…";
    try { await jmap.saveSettings(s); S.settings = s; closeSettings(); toast("Settings saved"); }
    catch (ex) { toast(ex.message); }
    finally { btn.disabled = false; btn.textContent = "Save"; }
  });

  // ── labels ────────────────────────────────────────────────────────
  async function loadLabels() {
    const r = await jmap.mailboxes();
    S.labels = (r.list || []).sort((a, b) => {
      const ia = ORDER.indexOf(a.role || a.id), ib = ORDER.indexOf(b.role || b.id);
      return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) || a.name.localeCompare(b.name);
    });
    renderLabels();
  }
  function renderLabels() {
    const ul = $("#labels"); ul.innerHTML = "";
    for (const m of S.labels) {
      const key = m.role || m.id;
      const li = el("li", "label" + (m.id === S.current ? " active" : "") + (m.unreadEmails > 0 ? " has-unread" : ""));
      li.innerHTML =
        `<svg viewBox="0 0 24 24" class="label-ic">${SYS_ICONS[key] || SYS_ICONS._}</svg>` +
        `<span class="label-name">${esc(m.name)}</span>` +
        `<span class="label-count">${m.unreadEmails > 0 ? m.unreadEmails : (m.totalEmails || "")}</span>`;
      li.onclick = () => selectLabel(m.id, m.name);
      ul.appendChild(li);
    }
  }

  async function selectLabel(id, name) {
    S.current = id; S.currentName = name; S.sel = -1; S.openId = null; S.filter = "";
    S.selected.clear(); renderSelbar();
    $("#search").value = "";
    $("#list-title").textContent = name;
    $("#app").classList.remove("reading");
    showRead(false);
    renderLabels();
    await loadList();
  }

  // ── list ──────────────────────────────────────────────────────────
  async function loadList() {
    const ml = $("#msglist"); ml.innerHTML = skeleton(8);
    $("#list-empty").hidden = true;
    try {
      const q = await jmap.query(S.current);
      const ids = q.ids || [];
      $("#list-count").textContent = ids.length ? ids.length + " conversation" + (ids.length > 1 ? "s" : "") : "";
      if (!ids.length) { ml.innerHTML = ""; empty("No mail here."); return; }
      const g = await jmap.emails(ids, ["id", "threadId", "from", "to", "subject", "receivedAt", "keywords", "mailboxIds", "preview", "size"]);
      // preserve query order
      const byId = Object.fromEntries((g.list || []).map((m) => [m.id, m]));
      S.rows = ids.map((id) => byId[id]).filter(Boolean).map((m) => ({ id: m.id, msg: m }));
      ml.classList.add("animate");
      renderList();
      setTimeout(() => ml.classList.remove("animate"), 800);
    } catch (ex) { ml.innerHTML = ""; empty("Couldn't load: " + ex.message); }
  }

  function visibleRows() {
    if (!S.filter) return S.rows;
    const f = S.filter.toLowerCase();
    return S.rows.filter(({ msg }) =>
      (msg.subject || "").toLowerCase().includes(f) ||
      (fromName(msg) + " " + fromAddr(msg)).toLowerCase().includes(f) ||
      (msg.preview || "").toLowerCase().includes(f));
  }

  function renderList() {
    const ml = $("#msglist"); ml.innerHTML = "";
    const rows = visibleRows();
    if (!rows.length) { empty(S.filter ? "No matches." : "No mail here."); return; }
    $("#list-empty").hidden = true;
    rows.forEach((row, i) => {
      const m = row.msg;
      const unread = !kw(m, "$seen");
      const starred = kw(m, "$flagged");
      const picked = S.selected.has(row.id);
      const li = el("li", "row" + (unread ? " unread" : "") + (m.id === S.openId ? " active" : "") + (picked ? " picked" : ""));
      li.dataset.i = i; li.dataset.id = row.id;
      li.style.animationDelay = Math.min(i, 14) * 24 + "ms";
      const nm = fromName(m);
      li.innerHTML =
        (unread ? '<span class="row-unreaddot"></span>' : "") +
        `<span class="pick" data-pick title="Select"><svg viewBox="0 0 24 24" class="ic" style="width:12px;height:12px;stroke-width:2.6"><path d="M20 6 9 17l-5-5"/></svg></span>` +
        `<div class="row-avatar" style="background:${avatarColor(fromAddr(m))}">${esc(initials(nm))}</div>` +
        `<div class="row-main">
           <div class="row-top"><span class="row-from">${esc(nm)}</span>
             <svg viewBox="0 0 24 24" class="star${starred ? " on" : ""}" data-star title="Star"><path d="${STAR}"/></svg>
             <span class="row-date">${fmtDate(m.receivedAt)}</span></div>
           <div class="row-subj">${esc(m.subject || "(no subject)")}</div>
           <div class="row-snip">${esc(m.preview || "")}</div>
         </div>`;
      li.onclick = (e) => {
        if (e.target.closest("[data-star]")) { toggleStar(row); e.stopPropagation(); return; }
        if (e.target.closest("[data-pick]")) { toggleSel(row); e.stopPropagation(); return; }
        openRow(i);
      };
      ml.appendChild(li);
    });
  }

  function empty(t) { const e = $("#list-empty"); e.textContent = t; e.hidden = false; }

  // ── contacts ──────────────────────────────────────────────────────
  let contactsCache = [];
  async function openContacts() {
    $("#contacts").hidden = false;
    $("#contacts-list").innerHTML = '<div class="contacts-empty">Loading…</div>';
    try { contactsCache = await jmap.contacts(); } catch { contactsCache = []; }
    renderContacts("");
    $("#contact-name").focus();
  }
  function renderContacts(f) {
    const ff = (f || "").toLowerCase();
    const list = contactsCache.filter((c) => !ff || (c.name + " " + c.email).toLowerCase().includes(ff))
      .sort((a, b) => (a.name || a.email).localeCompare(b.name || b.email));
    const box = $("#contacts-list");
    if (!list.length) { box.innerHTML = `<div class="contacts-empty">${contactsCache.length ? "No matches." : "No contacts yet. Add one above."}</div>`; return; }
    box.innerHTML = "";
    for (const c of list) {
      const nm = c.name || c.email.split("@")[0];
      const row = el("div", "contact",
        `<div class="avatar" style="background:${avatarColor(c.email)}">${esc(initials(nm))}</div>` +
        `<div class="contact-meta"><div class="contact-name">${esc(nm)}</div><div class="contact-email">${esc(c.email)}</div></div>` +
        `<div class="contact-acts">
           <button class="iconbtn" data-mail title="Compose"><svg viewBox="0 0 24 24" class="ic"><path d="M4 4h16v16H4z"/><path d="m22 6-10 7L2 6"/></svg></button>
           <button class="iconbtn" data-del title="Delete"><svg viewBox="0 0 24 24" class="ic"><path d="M3 6h18"/><path d="M8 6V4h8v2m1 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/></svg></button>
         </div>`);
      row.querySelector("[data-mail]").onclick = () => { $("#contacts").hidden = true; openCompose({ to: c.email }); };
      row.querySelector("[data-del]").onclick = async () => { await jmap.delContact(c.id); contactsCache = contactsCache.filter((x) => x.id !== c.id); renderContacts($("#contact-search").value); toast("Contact deleted"); };
      box.appendChild(row);
    }
  }
  $("#contacts-btn").addEventListener("click", openContacts);
  $("#contacts-close").addEventListener("click", () => ($("#contacts").hidden = true));
  $("#contacts").addEventListener("click", (e) => { if (e.target.id === "contacts") $("#contacts").hidden = true; });
  $("#contact-search").addEventListener("input", (e) => renderContacts(e.target.value));
  $("#contact-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const name = $("#contact-name").value.trim(), email = $("#contact-email").value.trim();
    if (!email) return;
    try {
      const r = await jmap.addContact({ name, email });
      contactsCache.push({ id: r.id, name, email });
      $("#contact-name").value = ""; $("#contact-email").value = "";
      renderContacts($("#contact-search").value); toast("Contact added");
    } catch (ex) { toast(ex.message); }
  });

  // ── calendar ──────────────────────────────────────────────────────
  let eventsCache = [];
  let calMonth = new Date();          // first-of-month being viewed
  let calView = "month";              // "month" | "agenda"
  let editingEvent = null;            // event under edit in the popover, or null
  const WD = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const dayKey = (d) => d.getFullYear() + "-" + (d.getMonth() + 1) + "-" + d.getDate();
  const sameDay = (a, b) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();

  async function openCalendar() {
    $("#calendar").hidden = false;
    calMonth = new Date();
    try { eventsCache = await jmap.events(); } catch { eventsCache = []; }
    renderCalendar();
  }
  function setView(v) {
    calView = v;
    $$("#cal-view button").forEach((b) => b.classList.toggle("on", b.dataset.view === v));
    $("#cal-grid").hidden = v !== "month";
    $("#cal-weekdays").hidden = v !== "month";
    $("#calendar-list").hidden = v !== "agenda";
    renderCalendar();
  }
  function renderCalendar() {
    $("#cal-title").textContent = calMonth.toLocaleDateString([], { month: "long", year: "numeric" });
    calView === "month" ? renderMonth() : renderAgenda();
  }
  function eventsByDay() {
    const map = {};
    for (const ev of eventsCache) (map[dayKey(new Date(ev.start))] ||= []).push(ev);
    for (const k in map) map[k].sort((a, b) => new Date(a.start) - new Date(b.start));
    return map;
  }
  function renderMonth() {
    const wd = $("#cal-weekdays"); wd.innerHTML = WD.map((d) => `<span>${d}</span>`).join("");
    const grid = $("#cal-grid"); grid.innerHTML = "";
    const byDay = eventsByDay();
    const today = new Date();
    const y = calMonth.getFullYear(), m = calMonth.getMonth();
    const start = new Date(y, m, 1); start.setDate(1 - start.getDay());
    for (let i = 0; i < 42; i++) {
      const d = new Date(start); d.setDate(start.getDate() + i);
      const cell = el("div", "cal-cell" + (d.getMonth() !== m ? " other" : "") + (sameDay(d, today) ? " today" : ""));
      cell.appendChild(el("span", "cal-daynum", String(d.getDate())));
      const evs = byDay[dayKey(d)] || [];
      const shown = evs.length > 3 ? 2 : evs.length; // leave room for "+N more"
      evs.slice(0, shown).forEach((ev) => {
        const t = new Date(ev.start).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
        const pill = el("div", "cal-pill", `<span class="pt">${t}</span><span class="pn">${esc(ev.summary || "(untitled)")}</span>`);
        pill.title = t + "  " + (ev.summary || "(untitled)");
        pill.onclick = (e) => { e.stopPropagation(); openEventPop(ev); };
        cell.appendChild(pill);
      });
      if (evs.length > shown) {
        const more = el("div", "cal-more", "+" + (evs.length - shown) + " more");
        more.onclick = (e) => { e.stopPropagation(); openDayPop(d, evs); };
        cell.appendChild(more);
      }
      cell.onclick = () => openEventPop(null, d);
      grid.appendChild(cell);
    }
  }
  // Day-detail popover (from "+N more"): lists that day's events; keeps month view.
  function openDayPop(day, evs) {
    const card = $("#calendar .cal-card");
    const pop = el("div", "cal-pop");
    const form = el("form", "", `<div class="cal-pop-title">${esc(day.toLocaleDateString([], { weekday: "long", month: "long", day: "numeric" }))}</div>`);
    form.addEventListener("submit", (e) => e.preventDefault());
    evs.forEach((ev) => {
      const t = new Date(ev.start).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
      const row = el("div", "cal-ev", `<span class="cal-time">${t}</span><span class="cal-dot"></span><span class="cal-title">${esc(ev.summary || "(untitled)")}</span>`);
      row.onclick = () => { pop.remove(); openEventPop(ev); };
      form.appendChild(row);
    });
    const add = el("button", "btn btn-ghost", "+ Add event"); add.type = "button";
    add.style.marginTop = "8px";
    add.onclick = () => { pop.remove(); openEventPop(null, day); };
    form.appendChild(add);
    pop.appendChild(form);
    pop.addEventListener("click", (e) => { if (e.target === pop) pop.remove(); });
    card.appendChild(pop);
  }
  function renderAgenda() {
    const box = $("#calendar-list");
    const evs = eventsCache.slice().sort((a, b) => new Date(a.start) - new Date(b.start));
    if (!evs.length) { box.innerHTML = '<div class="contacts-empty">No events yet. Click a day or + Event to add one.</div>'; return; }
    box.innerHTML = ""; let lastDay = "";
    for (const ev of evs) {
      const d = new Date(ev.start);
      const day = d.toLocaleDateString([], { weekday: "long", month: "short", day: "numeric" });
      if (day !== lastDay) { box.appendChild(el("div", "cal-day", esc(day))); lastDay = day; }
      const row = el("div", "cal-ev",
        `<span class="cal-time">${d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}</span>` +
        `<span class="cal-dot"></span><span class="cal-title">${esc(ev.summary || "(untitled)")}</span>`);
      row.onclick = () => openEventPop(ev);
      box.appendChild(row);
    }
  }
  // local datetime-local string from a Date
  function dtLocal(d) {
    const p = (n) => String(n).padStart(2, "0");
    return d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate()) + "T" + p(d.getHours()) + ":" + p(d.getMinutes());
  }
  function openEventPop(ev, day) {
    editingEvent = ev;
    $("#cal-pop").hidden = false;
    $("#cal-pop-head").textContent = ev ? "Edit event" : "New event";
    $("#cal-pop-save").textContent = ev ? "Save" : "Add";
    $("#cal-pop-del").hidden = !ev;
    $("#event-title").value = ev ? (ev.summary || "") : "";
    let when;
    if (ev) when = new Date(ev.start);
    else { when = new Date(day || calMonth); when.setHours(9, 0, 0, 0); }
    $("#event-when").value = dtLocal(when);
    $("#event-title").focus();
  }
  function closeEventPop() { $("#cal-pop").hidden = true; editingEvent = null; }

  $("#calendar-btn").addEventListener("click", openCalendar);
  $("#calendar-close").addEventListener("click", () => ($("#calendar").hidden = true));
  $("#calendar").addEventListener("click", (e) => { if (e.target.id === "calendar") $("#calendar").hidden = true; });
  $("#cal-prev").addEventListener("click", () => { calMonth = new Date(calMonth.getFullYear(), calMonth.getMonth() - 1, 1); renderCalendar(); });
  $("#cal-next").addEventListener("click", () => { calMonth = new Date(calMonth.getFullYear(), calMonth.getMonth() + 1, 1); renderCalendar(); });
  $("#cal-today").addEventListener("click", () => { calMonth = new Date(); renderCalendar(); });
  $$("#cal-view button").forEach((b) => b.addEventListener("click", () => setView(b.dataset.view)));
  $("#cal-new").addEventListener("click", () => openEventPop(null, new Date()));
  $("#cal-pop-cancel").addEventListener("click", closeEventPop);
  $("#cal-pop").addEventListener("click", (e) => { if (e.target.id === "cal-pop") closeEventPop(); });
  $("#cal-pop-del").addEventListener("click", async () => {
    if (!editingEvent) return;
    const id = editingEvent.id;
    await jmap.delEvent(id);
    eventsCache = eventsCache.filter((x) => x.id !== id);
    closeEventPop(); renderCalendar(); toast("Event deleted");
  });
  $("#event-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const summary = $("#event-title").value.trim();
    const when = $("#event-when").value;
    if (!summary) return;
    const start = when ? new Date(when).toISOString() : new Date().toISOString();
    const old = editingEvent;
    try {
      // Edit = re-add THEN delete the old (the API has no update verb). Add-first
      // means a failed add never destroys the original; worst case is a transient
      // duplicate if the delete then fails (recoverable), not data loss.
      const r = await jmap.addEvent({ summary, start, end: "" });
      eventsCache.push({ id: r.id, summary, start, end: "" });
      if (old) {
        await jmap.delEvent(old.id);
        eventsCache = eventsCache.filter((x) => x.id !== old.id);
      }
      closeEventPop(); renderCalendar(); toast(old ? "Event saved" : "Event added");
    } catch (ex) { renderCalendar(); toast(ex.message); }
  });

  // ── multi-select ──────────────────────────────────────────────────
  function toggleSel(row) {
    if (S.selected.has(row.id)) S.selected.delete(row.id); else S.selected.add(row.id);
    const li = $(`.row[data-id="${cssId(row.id)}"]`);
    if (li) li.classList.toggle("picked", S.selected.has(row.id));
    renderSelbar();
  }
  function clearSel() { S.selected.clear(); renderSelbar(); $$(".row.picked").forEach((r) => r.classList.remove("picked")); }
  function selRows() { return S.rows.filter((r) => S.selected.has(r.id)); }
  function renderSelbar() {
    let bar = $("#selbar");
    if (S.selected.size === 0) { if (bar) bar.remove(); return; }
    if (!bar) { bar = el("div", "selbar"); bar.id = "selbar"; $("#list-pane").insertBefore(bar, $("#msglist")); }
    bar.innerHTML =
      `<span class="selcount"><b>${S.selected.size}</b> selected</span>` +
      selAct("read", "Mark read", '<path d="M22 6 12 13 2 6"/><rect x="2" y="4" width="20" height="16" rx="2"/>') +
      selAct("star", "Star", `<path d="${STAR}"/>`) +
      selAct("archive", "Archive", SYS_ICONS.archive) +
      selAct("trash", "Delete", SYS_ICONS.trash) +
      selAct("clear", "Clear", '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>');
    bar.querySelector("[data-s=read]").onclick = () => bulk("read");
    bar.querySelector("[data-s=star]").onclick = () => bulk("star");
    bar.querySelector("[data-s=archive]").onclick = () => bulk("archive");
    bar.querySelector("[data-s=trash]").onclick = () => bulk("trash");
    bar.querySelector("[data-s=clear]").onclick = clearSel;
  }
  // `body` is full inner-SVG markup (one or more <path>/<rect> elements).
  const selAct = (s, t, body) => `<button class="iconbtn" data-s="${s}" title="${t}"><svg viewBox="0 0 24 24" class="ic">${body}</svg></button>`;

  async function bulk(kind) {
    const rows = selRows(); if (!rows.length) return;
    const ids = rows.map((r) => r.id);
    if (kind === "read") { rows.forEach((r) => { setKw(r.msg, "$seen", true); }); await setMany(ids, { "keywords/$seen": true }); clearSel(); renderList(); bumpUnread(); return; }
    if (kind === "star") { rows.forEach((r) => setKw(r.msg, "$flagged", true)); await setMany(ids, { "keywords/$flagged": true }); clearSel(); renderList(); toast(ids.length + " starred"); return; }
    if (kind === "archive") {
      rows.forEach((r) => removeRowQuiet(r)); clearSel(); renderList();
      await setMany(ids, { "mailboxIds/inbox": null });
      toast(ids.length + " archived", async () => { await setMany(ids, { "mailboxIds/inbox": true }); loadList(); });
    }
    if (kind === "trash") {
      rows.forEach((r) => removeRowQuiet(r)); clearSel(); renderList();
      await setMany(ids, { "mailboxIds/inbox": null, "mailboxIds/trash": true });
      toast(ids.length + " deleted", async () => { await setMany(ids, { "mailboxIds/inbox": true, "mailboxIds/trash": null }); loadList(); });
    }
  }
  function removeRowQuiet(row) { S.rows = S.rows.filter((r) => r.id !== row.id); if (S.openId === row.id) backToList(); }
  async function setMany(ids, patch) { const u = {}; ids.forEach((id) => (u[id] = patch)); try { await jmap.set(u); } catch {} }
  function cssId(id) { return String(id).replace(/"/g, '\\"'); }

  // ── open / read ───────────────────────────────────────────────────
  async function openRow(i) {
    const rows = visibleRows();
    if (i < 0 || i >= rows.length) return;
    S.sel = i;
    const row = rows[i];
    S.openId = row.id;
    $("#app").classList.add("reading");
    highlightRow();
    showRead(true);
    $("#read").innerHTML = '<div class="read-actions"></div>' + skeleton(3);
    try {
      const g = await jmap.emails([row.id], ["id", "threadId", "from", "to", "cc", "subject", "receivedAt", "keywords", "mailboxIds", "bodyValues", "attachments", "preview", "size"]);
      const m = (g.list || [])[0];
      if (!m) return;
      row.msg = { ...row.msg, ...m };
      renderRead(row);
      if (!kw(row.msg, "$seen")) markSeen(row, true);
    } catch (ex) { $("#read").innerHTML = `<div class="read-empty"><p>${esc(ex.message)}</p></div>`; }
  }

  function renderRead(row) {
    const m = row.msg;
    const starred = kw(m, "$flagged");
    const body = bodyText(m);
    const labels = (Object.keys(m.mailboxIds || {})).map((id) => labelName(id));
    $("#read").innerHTML = `
      <div class="read-actions">
        ${actBtn("back", "Back (u)", '<path d="M19 12H5"/><path d="m12 19-7-7 7-7"/>')}
        ${actBtn("archive", "Archive (e)", SYS_ICONS.archive)}
        ${actBtn("trash", "Delete (#)", SYS_ICONS.trash)}
        ${actBtn("star", starred ? "Unstar (s)" : "Star (s)", STAR, starred ? "on" : "")}
        ${actBtn("unread", "Mark unread", '<path d="M22 6 12 13 2 6"/><rect x="2" y="4" width="20" height="16" rx="2"/>')}
        ${actBtn("reply", "Reply (r)", '<path d="M9 17 4 12l5-5"/><path d="M20 18v-2a4 4 0 0 0-4-4H4"/>')}
      </div>
      <h1 class="read-subject">${esc(m.subject || "(no subject)")}</h1>
      <div class="read-labels">${labels.map((l) => `<span class="chip">${esc(l)}</span>`).join("")}</div>
      <div class="msg">
        <div class="msg-head">
          <div class="avatar" style="background:${avatarColor(fromAddr(m))}">${esc(initials(fromName(m)))}</div>
          <div class="msg-meta">
            <div class="msg-from">${esc(fromName(m))}</div>
            <div class="msg-addr">${esc(fromAddr(m))}</div>
            <div class="msg-to">to ${esc(addrList(m.to) || "you")}</div>
          </div>
          <div class="msg-date">${fmtFull(m.receivedAt)}</div>
        </div>
        <div class="msg-body">${fmtBody(body)}</div>
        ${attsHTML(m.attachments)}
      </div>`;
    $("#read [data-act=back]").onclick = backToList;
    $("#read [data-act=archive]").onclick = () => { archive(row); };
    $("#read [data-act=trash]").onclick = () => { trash(row); };
    $("#read [data-act=star]").onclick = () => toggleStar(row);
    $("#read [data-act=unread]").onclick = () => { markSeen(row, false); backToList(); };
    $("#read [data-act=reply]").onclick = () => replyTo(row);
    $$("#read [data-att]").forEach((e) => {
      const i = +e.dataset.att;
      e.onclick = () => downloadAtt(row.id, i, (m.attachments[i] || {}).name);
    });
  }
  function attsHTML(atts) {
    if (!atts || !atts.length) return "";
    return `<div class="read-atts">` + atts.map((a, i) =>
      `<div class="read-att" data-att="${i}"><svg viewBox="0 0 24 24" class="ic"><path d="M21 11.5 12.5 20a4 4 0 0 1-6-6l8-8a2.5 2.5 0 0 1 4 4l-8 8a1 1 0 0 1-1.5-1.5L17 11"/></svg><span class="nm">${esc(a.name || "attachment")}</span><span class="sz">${fmtBytes(a.size)}</span></div>`).join("") + `</div>`;
  }
  async function downloadAtt(id, n, name) {
    try {
      const blob = await jmap.download(`/api/webmail/attachment?id=${encodeURIComponent(id)}&n=${n}`);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a"); a.href = url; a.download = name || "attachment"; a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1500);
    } catch (ex) { toast(ex.message); }
  }
  const actBtn = (act, title, path, cls = "") =>
    `<button class="iconbtn ${cls}" data-act="${act}" title="${title}"><svg viewBox="0 0 24 24" class="ic"><path d="${path}"/></svg></button>`;

  function showRead(on) { $("#read").hidden = !on; $("#read-empty").hidden = on; }
  function backToList() { S.openId = null; $("#app").classList.remove("reading"); showRead(false); renderList(); $("#msglist").focus(); }

  // ── actions ───────────────────────────────────────────────────────
  async function markSeen(row, seen) {
    setKw(row.msg, "$seen", seen);
    renderRowInPlace(row); bumpUnread();
    try { await jmap.set({ [row.id]: { ["keywords/$seen"]: seen ? true : null } }); } catch {}
  }
  async function toggleStar(row) {
    const on = !kw(row.msg, "$flagged");
    setKw(row.msg, "$flagged", on);
    renderRowInPlace(row);
    if (S.openId === row.id) renderRead(row);
    try { await jmap.set({ [row.id]: { ["keywords/$flagged"]: on ? true : null } }); } catch {}
    toast(on ? "Starred" : "Unstarred");
  }
  async function archive(row) {
    const id = row.id;
    if (S.current === "inbox") removeRow(row);
    try { await jmap.set({ [id]: { ["mailboxIds/inbox"]: null } }); } catch {}
    toast("Archived", async () => { await jmap.set({ [id]: { ["mailboxIds/inbox"]: true } }); loadList(); });
  }
  async function trash(row) {
    const id = row.id;
    removeRow(row);
    try { await jmap.set({ [id]: { ["mailboxIds/inbox"]: null, ["mailboxIds/trash"]: true } }); } catch {}
    toast("Deleted", async () => { await jmap.set({ [id]: { ["mailboxIds/inbox"]: true, ["mailboxIds/trash"]: null } }); loadList(); });
  }
  function removeRow(row) {
    S.rows = S.rows.filter((r) => r.id !== row.id);
    if (S.openId === row.id) backToList();
    renderList();
  }

  // ── compose ───────────────────────────────────────────────────────
  $("#compose-btn").addEventListener("click", () => openCompose());
  function openCompose(pre = {}) {
    const node = $("#compose-tpl").content.firstElementChild.cloneNode(true);
    $("#compose-dock").appendChild(node);
    const rich = node.querySelector(".c-rich");
    const atts = []; // {name,type,data(base64),size}
    if (pre.to) node.querySelector(".c-to").value = pre.to;
    if (pre.subject) node.querySelector(".c-subj").value = pre.subject;
    const sig = S.settings && S.settings.signature ? "\n\n" + S.settings.signature : "";
    rich.innerText = (pre.text || "") + sig;

    const head = node.querySelector(".compose-head");
    head.onclick = (e) => { if (e.target.closest(".close,.min")) return; node.classList.toggle("min"); };
    node.querySelector(".close").onclick = () => node.remove();
    node.querySelector(".min").onclick = () => node.classList.toggle("min");

    // formatting toolbar
    node.querySelectorAll(".ctool[data-fmt]").forEach((b) => b.addEventListener("mousedown", (e) => {
      e.preventDefault(); rich.focus(); document.execCommand(b.dataset.fmt, false, null);
    }));
    node.querySelector("[data-link]").addEventListener("mousedown", (e) => {
      e.preventDefault(); rich.focus();
      const url = prompt("Link URL:", "https://");
      if (url) document.execCommand("createLink", false, url);
    });

    // attachments
    const file = node.querySelector(".c-file");
    node.querySelector("[data-attach]").onclick = () => file.click();
    const ingestFiles = async (files) => {
      for (const f of files) {
        const data = await readBase64(f);
        atts.push({ name: f.name, type: f.type || "application/octet-stream", data, size: f.size });
      }
      renderAtts();
    };
    file.addEventListener("change", async () => { await ingestFiles(file.files); file.value = ""; });
    // Drag-and-drop attachments onto the compose window.
    let dragDepth = 0;
    node.addEventListener("dragenter", (e) => { e.preventDefault(); if (dragDepth++ === 0) showDrop(true); });
    node.addEventListener("dragover", (e) => e.preventDefault());
    node.addEventListener("dragleave", (e) => { e.preventDefault(); if (--dragDepth <= 0) { dragDepth = 0; showDrop(false); } });
    node.addEventListener("drop", async (e) => {
      e.preventDefault(); dragDepth = 0; showDrop(false);
      if (e.dataTransfer && e.dataTransfer.files.length) await ingestFiles(e.dataTransfer.files);
    });
    function showDrop(on) {
      node.classList.toggle("dragging", on);
      let d = node.querySelector(".compose-drop");
      if (on && !d) { d = el("div", "compose-drop", "Drop files to attach"); node.appendChild(d); }
      else if (!on && d) d.remove();
    }
    function renderAtts() {
      const box = node.querySelector(".c-atts"); box.innerHTML = "";
      atts.forEach((a, i) => {
        const chip = el("span", "att-chip",
          `<svg viewBox="0 0 24 24" class="ic"><path d="M21 11.5 12.5 20a4 4 0 0 1-6-6l8-8a2.5 2.5 0 0 1 4 4l-8 8a1 1 0 0 1-1.5-1.5L17 11"/></svg>` +
          `<span class="nm">${esc(a.name)}</span><span class="sz">${fmtBytes(a.size)}</span><span class="rm">✕</span>`);
        chip.querySelector(".rm").onclick = () => { atts.splice(i, 1); renderAtts(); };
        box.appendChild(chip);
      });
    }

    const send = node.querySelector(".c-send");
    const doSend = async () => {
      const to = node.querySelector(".c-to").value.trim();
      if (!to) { node.querySelector(".c-to").focus(); return; }
      send.disabled = true; node.querySelector(".c-status").textContent = "Sending…";
      try {
        await jmap.send({
          to: to.split(",").map((s) => s.trim()).filter(Boolean),
          subject: node.querySelector(".c-subj").value,
          text: rich.innerText,
          html: rich.innerHTML.trim() ? rich.innerHTML : "",
          attachments: atts.map((a) => ({ name: a.name, type: a.type, data: a.data })),
        });
        node.remove(); toast("Sent");
      } catch (ex) { node.querySelector(".c-status").textContent = ex.message; send.disabled = false; }
    };
    send.onclick = doSend;
    rich.addEventListener("keydown", (e) => { if ((e.metaKey || e.ctrlKey) && e.key === "Enter") doSend(); });
    (pre.to ? rich : node.querySelector(".c-to")).focus();
    return node;
  }
  function replyTo(row) {
    const m = row.msg;
    openCompose({
      to: fromAddr(m),
      subject: /^re:/i.test(m.subject || "") ? m.subject : "Re: " + (m.subject || ""),
      text: "\n\n———\nOn " + fmtFull(m.receivedAt) + ", " + fromName(m) + " wrote:\n> " + (bodyText(m).split("\n").join("\n> ")),
    });
  }
  function readBase64(file) {
    return new Promise((res, rej) => {
      const r = new FileReader();
      r.onload = () => res(String(r.result).split(",")[1] || "");
      r.onerror = rej; r.readAsDataURL(file);
    });
  }

  // ── keyboard ──────────────────────────────────────────────────────
  const SHORTCUTS = [
    ["c", "Compose"], ["/", "Search"], ["j / k", "Next / previous"], ["Enter", "Open"],
    ["u", "Back to list"], ["e", "Archive"], ["#", "Delete"], ["s", "Star"],
    ["r", "Reply"], ["g i", "Go to Inbox"], ["x", "Select"], ["⌘ K", "Command palette"],
    ["?", "This help"], ["Esc", "Close"],
  ];
  let gPending = false;
  document.addEventListener("keydown", (e) => {
    if ($("#login").hidden === false) return;
    // Command palette: works everywhere, even while typing.
    if ((e.metaKey || e.ctrlKey) && (e.key === "k" || e.key === "K")) { e.preventDefault(); openCmdk(); return; }
    if (!$("#cmdk").hidden) { cmdkKey(e); return; }
    const typing = /^(input|textarea)$/i.test(e.target.tagName) || e.target.isContentEditable;
    if (typing) { if (e.key === "Escape") e.target.blur(); return; }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === "x") { if (S.sel < 0) { S.sel = 0; highlightRow(); } const r = visibleRows()[S.sel]; if (r) { toggleSel(r); move(1); } return; }
    const rows = visibleRows();
    if (gPending) { gPending = false; if (e.key === "i") selectLabel("inbox", "Inbox"); return; }
    switch (e.key) {
      case "j": move(1); break;
      case "k": move(-1); break;
      case "Enter": if (S.sel >= 0) openRow(S.sel); break;
      case "u": backToList(); break;
      case "c": e.preventDefault(); openCompose(); break;
      case "/": e.preventDefault(); $("#search").focus(); break;
      case "g": gPending = true; setTimeout(() => (gPending = false), 700); break;
      case "e": if (rows[S.sel]) archive(rows[S.sel]); break;
      case "#": if (rows[S.sel]) trash(rows[S.sel]); break;
      case "s": if (rows[S.sel]) toggleStar(rows[S.sel]); break;
      case "r": if (rows[S.sel]) replyTo(rows[S.sel]); break;
      case "?": toggleShortcuts(true); break;
      case "Escape": {
        const dyn = document.querySelector("#calendar .cal-pop:not(#cal-pop)");
        if (dyn) { dyn.remove(); break; }                       // day-detail popover first
        if (!$("#cal-pop").hidden) { $("#cal-pop").hidden = true; break; }
        $("#contacts").hidden = true; $("#settings").hidden = true; $("#calendar").hidden = true; toggleShortcuts(false); break;
      }
    }
  });
  function move(d) {
    const rows = visibleRows(); if (!rows.length) return;
    S.sel = Math.max(0, Math.min(rows.length - 1, (S.sel < 0 ? 0 : S.sel + d)));
    highlightRow(true);
  }
  function highlightRow(scroll) {
    const vr = visibleRows();
    $$(".row").forEach((r) => r.classList.toggle("active", +r.dataset.i === S.sel || (vr[S.sel] && r.dataset.id === S.openId)));
    const cur = $(`.row[data-i="${S.sel}"]`);
    if (cur) { cur.classList.add("active"); if (scroll) cur.scrollIntoView({ block: "nearest" }); }
  }

  // ── search / refresh ──────────────────────────────────────────────
  let st;
  $("#search").addEventListener("input", (e) => { clearTimeout(st); st = setTimeout(() => { S.filter = e.target.value.trim(); S.sel = -1; renderList(); }, 120); });
  $("#refresh").addEventListener("click", () => { const r = $("#refresh"); r.classList.add("spin"); loadList().then(() => loadLabels()).finally(() => setTimeout(() => r.classList.remove("spin"), 400)); });

  function toggleShortcuts(on) {
    const o = $("#shortcuts");
    if (on && o.hidden) {
      $("#sc-grid").innerHTML = SHORTCUTS.map(([k, d]) => `<div class="sc"><span>${d}</span><kbd>${k}</kbd></div>`).join("");
      o.hidden = false;
    } else if (!on) o.hidden = true;
  }
  $("#sc-close").onclick = () => toggleShortcuts(false);
  $("#shortcuts").onclick = (e) => { if (e.target.id === "shortcuts") toggleShortcuts(false); };

  // ── command palette (⌘K) ──────────────────────────────────────────
  let cmdkIdx = 0, cmdkCmds = [];
  function commands() {
    const c = [
      { label: "Compose new message", k: "c", ic: '<path d="M4 20h4L18.5 9.5a2.1 2.1 0 0 0-3-3L5 17v3z"/><path d="m13.5 6.5 3 3"/>', run: () => openCompose() },
      { label: "Search mail", k: "/", ic: '<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>', run: () => $("#search").focus() },
      { label: "Refresh", ic: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>', run: () => $("#refresh").click() },
      { label: "Mark all as read", ic: SYS_ICONS.inbox, run: markAllRead },
      { label: "Contacts", ic: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.9"/>', run: openContacts },
      { label: "Calendar", ic: '<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>', run: openCalendar },
      { label: "Settings", ic: '<circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 0 0-.1-1l2-1.5-2-3.4-2.3 1a7 7 0 0 0-1.7-1l-.4-2.6h-4l-.4 2.6a7 7 0 0 0-1.7 1l-2.3-1-2 3.4L5 11a7 7 0 0 0 0 2l-2 1.5 2 3.4 2.3-1a7 7 0 0 0 1.7 1l.4 2.6h4l.4-2.6a7 7 0 0 0 1.7-1l2.3 1 2-3.4-2-1.5a7 7 0 0 0 .1-1z"/>', run: openSettings },
      { label: "Keyboard shortcuts", k: "?", ic: '<circle cx="12" cy="12" r="10"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 2.5-3 4"/><path d="M12 17h.01"/>', run: () => toggleShortcuts(true) },
      { label: "Sign out", ic: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>', run: () => $("#logout").click() },
    ];
    for (const m of S.labels) c.push({ label: "Go to " + m.name, ic: SYS_ICONS[m.role || m.id] || SYS_ICONS._, run: () => selectLabel(m.id, m.name) });
    return c;
  }
  function openCmdk() { const o = $("#cmdk"); o.hidden = false; const inp = $("#cmdk-in"); inp.value = ""; cmdkIdx = 0; renderCmdk(""); inp.focus(); }
  function closeCmdk() { $("#cmdk").hidden = true; }
  function renderCmdk(f) {
    const all = commands(); const ff = f.trim().toLowerCase();
    cmdkCmds = ff ? all.filter((c) => c.label.toLowerCase().includes(ff)) : all;
    cmdkIdx = Math.max(0, Math.min(cmdkIdx, cmdkCmds.length - 1));
    const list = $("#cmdk-list");
    list.innerHTML = cmdkCmds.length
      ? cmdkCmds.map((c, i) => `<div class="cmdk-item${i === cmdkIdx ? " on" : ""}" data-i="${i}"><svg viewBox="0 0 24 24" class="ic">${c.ic}</svg><span>${esc(c.label)}</span>${c.k ? `<span class="k">${c.k}</span>` : ""}</div>`).join("")
      : '<div class="cmdk-empty">No matching commands</div>';
    $$("#cmdk-list .cmdk-item").forEach((e) => (e.onclick = () => runCmdk(+e.dataset.i)));
  }
  function runCmdk(i) { const c = cmdkCmds[i]; closeCmdk(); if (c) setTimeout(c.run, 0); }
  function cmdkKey(e) {
    if (e.key === "Escape") { e.preventDefault(); closeCmdk(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); cmdkIdx = Math.min(cmdkIdx + 1, cmdkCmds.length - 1); renderCmdk($("#cmdk-in").value); scrollCmdk(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); cmdkIdx = Math.max(cmdkIdx - 1, 0); renderCmdk($("#cmdk-in").value); scrollCmdk(); }
    else if (e.key === "Enter") { e.preventDefault(); runCmdk(cmdkIdx); }
  }
  function scrollCmdk() { const e = $("#cmdk-list .cmdk-item.on"); if (e) e.scrollIntoView({ block: "nearest" }); }
  $("#cmdk-in").addEventListener("input", (e) => { cmdkIdx = 0; renderCmdk(e.target.value); });
  $("#cmdk").addEventListener("click", (e) => { if (e.target.id === "cmdk") closeCmdk(); });
  async function markAllRead() {
    const ids = S.rows.filter((r) => !kw(r.msg, "$seen")).map((r) => r.id);
    if (!ids.length) { toast("Nothing unread"); return; }
    S.rows.forEach((r) => setKw(r.msg, "$seen", true)); renderList();
    await setMany(ids, { "keywords/$seen": true }); bumpUnread(); toast("Marked " + ids.length + " read");
  }

  // ── helpers ───────────────────────────────────────────────────────
  const STAR = "m12 3 2.9 6 6.6.9-4.8 4.6 1.2 6.5L12 18l-5.9 3 1.2-6.5L2.5 9.9 9 9z";
  function kw(m, k) { return !!(m.keywords && m.keywords[k]); }
  function setKw(m, k, v) { m.keywords = m.keywords || {}; if (v) m.keywords[k] = true; else delete m.keywords[k]; }
  function renderRowInPlace() { const rows = visibleRows(); renderList(); highlightRow(); void rows; }
  function bumpUnread() { /* recompute label unread counts lazily */ loadLabels().catch(() => {}); }
  function from(m) { return (m.from && m.from[0]) || {}; }
  function fromName(m) { const f = from(m); return f.name || (f.email ? f.email.split("@")[0] : "(unknown)"); }
  function fromAddr(m) { return from(m).email || ""; }
  function addrList(a) { return (a || []).map((x) => x.name || x.email).join(", "); }
  function labelName(id) { const m = S.labels.find((l) => l.id === id); return m ? m.name : id; }
  function bodyText(m) {
    if (m.bodyValues) { const v = Object.values(m.bodyValues)[0]; if (v && v.value) return v.value; }
    return m.preview || "";
  }
  function initials(n) {
    const p = (n || "").replace(/[^\p{L}\p{N}\s]/gu, " ").trim().split(/\s+/).filter(Boolean);
    return ((p[0] || "")[0] || "?").toUpperCase() + (p[1] ? p[1][0].toUpperCase() : "");
  }
  function avatarColor(seed) {
    let h = 0; for (const c of seed) h = (h * 31 + c.charCodeAt(0)) >>> 0;
    const hues = [[15, 106, 108], [201, 106, 255], [45, 212, 191], [245, 158, 11]];
    const [r, g, b] = hues[h % hues.length];
    return `linear-gradient(135deg,rgb(${r},${g},${b}),rgb(${(r + 40) % 256},${(g + 30) % 256},${(b + 50) % 256}))`;
  }
  function esc(s) { return (s == null ? "" : String(s)).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c])); }
  function linkify(s) { return s.replace(/(https?:\/\/[^\s<]+)/g, '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>'); }
  // Escape, linkify per line, and wrap runs of ">"-quoted lines in a blockquote.
  function fmtBody(body) {
    const lines = (body || "").split("\n");
    let out = "", inQ = false;
    for (const ln of lines) {
      const q = /^\s*>/.test(ln);
      if (q && !inQ) { out += "<blockquote>"; inQ = true; }
      else if (!q && inQ) { out += "</blockquote>"; inQ = false; }
      out += linkify(esc(q ? ln.replace(/^\s*>\s?/, "") : ln)) + "\n";
    }
    if (inQ) out += "</blockquote>";
    return out;
  }
  function skeleton(n) { return Array(n).fill('<li class="skeleton"><div class="sk-line" style="width:40%"></div><div class="sk-line" style="width:80%"></div><div class="sk-line" style="width:60%"></div></li>').join(""); }
  function fmtDate(iso) {
    if (!iso) return ""; const d = new Date(iso), now = new Date();
    if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
    if (d.getFullYear() === now.getFullYear()) return d.toLocaleDateString([], { month: "short", day: "numeric" });
    return d.toLocaleDateString([], { year: "2-digit", month: "short", day: "numeric" });
  }
  function fmtFull(iso) { return iso ? new Date(iso).toLocaleString([], { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }) : ""; }
  function fmtBytes(n) { if (!n) return ""; if (n < 1024) return n + " B"; if (n < 1048576) return (n / 1024).toFixed(0) + " KB"; return (n / 1048576).toFixed(1) + " MB"; }
  // Stacked toasts: each is its own node that auto-expires, so rapid actions
  // don't clobber each other (and an Undo isn't lost to the next action).
  function toast(t, undoFn) {
    const stack = $("#toast-stack");
    const e = el("div", "toast", esc(t) + (undoFn ? '<span class="undo">Undo</span>' : ""));
    stack.appendChild(e);
    requestAnimationFrame(() => e.classList.add("show"));
    const dismiss = () => { e.classList.remove("show"); setTimeout(() => e.remove(), 280); };
    if (undoFn) e.querySelector(".undo").onclick = () => { undoFn(); dismiss(); };
    setTimeout(dismiss, undoFn ? 6000 : 2400);
  }

  boot();
})();
