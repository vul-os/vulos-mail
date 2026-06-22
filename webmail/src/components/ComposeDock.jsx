import { useEffect, useRef, useState } from "react";
import Icon from "./Icon.jsx";
import { useToast } from "./Toasts.jsx";
import { sanitizeHTML, fmtBytes } from "../lib/util.js";
import { readDraft, writeDraft, clearDraft } from "./Mail.jsx";

function readBase64(file) {
  return new Promise((res, rej) => {
    const r = new FileReader();
    r.onload = () => res(String(r.result).split(",")[1] || "");
    r.onerror = rej; r.readAsDataURL(file);
  });
}

export default function ComposeDock({ jmap, settings, pre = {}, onClose, onReopen }) {
  const toast = useToast();
  const richRef = useRef(null);
  const toRef = useRef(null);
  const subjRef = useRef(null);
  const fileRef = useRef(null);
  const draftT = useRef(null);
  const sentRef = useRef(false);
  const [minimized, setMinimized] = useState(false);
  const [atts, setAtts] = useState([]);
  const [status, setStatus] = useState("");
  const [linkbar, setLinkbar] = useState(false);
  const [dragging, setDragging] = useState(false);
  const savedRange = useRef(null);
  const linkInputRef = useRef(null);

  // Seed the composer body once on mount (parity with the vanilla SPA).
  useEffect(() => {
    const rich = richRef.current;
    if (pre.draft) {
      if (toRef.current) toRef.current.value = pre.draft.to || "";
      if (subjRef.current) subjRef.current.value = pre.draft.subject || "";
      rich.innerHTML = sanitizeHTML(pre.draft.html || "");
    } else {
      if (toRef.current) toRef.current.value = pre.to || "";
      if (subjRef.current) subjRef.current.value = pre.subject || "";
      const sig = settings && settings.signature ? "\n\n" + settings.signature : "";
      rich.innerText = (pre.text || "") + sig;
    }
    // Focus: recipient if empty, else the body.
    setTimeout(() => { (pre.to || pre.draft ? rich : toRef.current)?.focus(); }, 0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function snapshot() {
    return { to: toRef.current.value, subject: subjRef.current.value, html: richRef.current.innerHTML, at: Date.now() };
  }
  function scheduleDraft() {
    clearTimeout(draftT.current);
    draftT.current = setTimeout(() => {
      if (!sentRef.current) {
        writeDraft(jmap.user, snapshot());
        setStatus("Draft saved");
        setTimeout(() => setStatus((s) => (s === "Draft saved" ? "" : s)), 1500);
      }
    }, 800);
  }

  function close(discard) {
    clearTimeout(draftT.current);
    const rich = richRef.current;
    if (discard) clearDraft(jmap.user);
    else if (!sentRef.current && (rich.innerText.trim() || toRef.current.value.trim() || subjRef.current.value.trim())) {
      writeDraft(jmap.user, snapshot());
    }
    onClose();
  }

  // Formatting toolbar (execCommand is deprecated but the only cross-browser way
  // for contenteditable rich text without a framework dependency).
  function fmt(cmd) { richRef.current.focus(); document.execCommand(cmd, false, null); scheduleDraft(); }

  function openLinkbar() {
    const sel = window.getSelection();
    savedRange.current = (sel && sel.rangeCount && richRef.current.contains(sel.anchorNode)) ? sel.getRangeAt(0).cloneRange() : null;
    setLinkbar(true);
    setTimeout(() => linkInputRef.current?.focus(), 0);
  }
  function applyLink() {
    let url = linkInputRef.current.value.trim();
    if (!url) { setLinkbar(false); return; }
    if (!/^(https?:|mailto:)/i.test(url)) url = "https://" + url;
    richRef.current.focus();
    if (savedRange.current) { const s = window.getSelection(); s.removeAllRanges(); s.addRange(savedRange.current); }
    if (window.getSelection().isCollapsed) document.execCommand("insertText", false, url);
    document.execCommand("createLink", false, url);
    richRef.current.querySelectorAll("a").forEach((a) => { a.target = "_blank"; a.rel = "noopener noreferrer"; });
    setLinkbar(false); scheduleDraft();
  }

  // Paste hardening: sanitize HTML pastes; plain text inserts as-is.
  function onPaste(e) {
    const dt = e.clipboardData; if (!dt) return;
    e.preventDefault();
    const html = dt.getData("text/html");
    if (html) document.execCommand("insertHTML", false, sanitizeHTML(html));
    else document.execCommand("insertText", false, dt.getData("text/plain"));
    scheduleDraft();
  }

  async function ingestFiles(files) {
    const next = [];
    for (const f of files) {
      const data = await readBase64(f);
      next.push({ name: f.name, type: f.type || "application/octet-stream", data, size: f.size });
    }
    setAtts((a) => [...a, ...next]);
  }

  const dragDepth = useRef(0);

  function doSend() {
    const to = toRef.current.value.trim();
    if (!to) { toRef.current.focus(); return; }
    const rich = richRef.current;
    const payload = {
      to: to.split(",").map((s) => s.trim()).filter(Boolean),
      subject: subjRef.current.value,
      text: rich.innerText,
      html: rich.innerHTML.trim() ? rich.innerHTML : "",
      attachments: atts.map((a) => ({ name: a.name, type: a.type, data: a.data })),
    };
    const reopenPre = { draft: { to: toRef.current.value, subject: subjRef.current.value, html: rich.innerHTML } };
    sentRef.current = true; clearTimeout(draftT.current); clearDraft(jmap.user);
    close(true);
    let cancelled = false;
    const timer = setTimeout(async () => {
      if (cancelled) return;
      try { await jmap.send(payload); toast("Sent"); }
      catch (ex) { toast("Send failed: " + ex.message); onReopen(reopenPre); }
    }, 5000);
    toast("Sending…", () => { cancelled = true; clearTimeout(timer); onReopen(reopenPre); });
  }

  return (
    <div
      className={"compose" + (minimized ? " min" : "") + (dragging ? " dragging" : "")}
      role="dialog" aria-label="Compose message"
      onKeyDown={(e) => { if (e.key === "Escape") { e.preventDefault(); close(false); } }}
      onDragEnter={(e) => { e.preventDefault(); if (dragDepth.current++ === 0) setDragging(true); }}
      onDragOver={(e) => e.preventDefault()}
      onDragLeave={(e) => { e.preventDefault(); if (--dragDepth.current <= 0) { dragDepth.current = 0; setDragging(false); } }}
      onDrop={async (e) => { e.preventDefault(); dragDepth.current = 0; setDragging(false); if (e.dataTransfer?.files.length) await ingestFiles(e.dataTransfer.files); }}
    >
      <header className="compose-head" onClick={(e) => { if (e.target.closest(".close,.min")) return; setMinimized((m) => !m); }}>
        <span>New message</span>
        <div className="compose-head-actions">
          <button className="iconbtn min" title="Minimize" onClick={() => setMinimized((m) => !m)}>
            <Icon body='<path d="M5 12h14"/>' />
          </button>
          <button className="iconbtn close" title="Discard (Esc)" onClick={() => close(true)}>
            <Icon body='<path d="M18 6 6 18M6 6l12 12"/>' />
          </button>
        </div>
      </header>
      <div className="compose-body">
        <label className="crow"><span>To</span><input ref={toRef} className="c-to" type="text" placeholder="recipient@domain" onInput={scheduleDraft} /></label>
        <label className="crow"><span>Subject</span><input ref={subjRef} className="c-subj" type="text" placeholder="Subject" onInput={scheduleDraft} /></label>
        <div ref={richRef} className="c-rich" contentEditable role="textbox" aria-multiline="true"
          aria-label="Message body" data-ph="Write something…" suppressContentEditableWarning
          onInput={scheduleDraft} onPaste={onPaste}
          onKeyDown={(e) => { if ((e.metaKey || e.ctrlKey) && e.key === "Enter") doSend(); }} />
        <div className="c-atts">
          {atts.map((a, i) => (
            <span key={i} className="att-chip">
              <Icon body='<path d="M21 11.5 12.5 20a4 4 0 0 1-6-6l8-8a2.5 2.5 0 0 1 4 4l-8 8a1 1 0 0 1-1.5-1.5L17 11"/>' />
              <span className="nm">{a.name}</span><span className="sz">{fmtBytes(a.size)}</span>
              <span className="rm" onClick={() => setAtts((xs) => xs.filter((_, j) => j !== i))}>✕</span>
            </span>
          ))}
        </div>
      </div>
      <footer className="compose-foot">
        <button className="btn btn-primary c-send" title="Send  ⌘↵" onClick={doSend}>
          <Icon body='<path d="M22 2 11 13"/><path d="M22 2 15 22l-4-9-9-4 20-7z"/>' />
          Send
        </button>
        <div className="c-tools">
          <button className="ctool" data-fmt="bold" title="Bold (⌘B)" onMouseDown={(e) => { e.preventDefault(); fmt("bold"); }}><b>B</b></button>
          <button className="ctool" data-fmt="italic" title="Italic (⌘I)" onMouseDown={(e) => { e.preventDefault(); fmt("italic"); }}><i>I</i></button>
          <button className="ctool" data-fmt="insertUnorderedList" title="Bulleted list" onMouseDown={(e) => { e.preventDefault(); fmt("insertUnorderedList"); }}>≔</button>
          <button className="ctool" data-link title="Insert link" onMouseDown={(e) => { e.preventDefault(); openLinkbar(); }}>
            <Icon body='<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>' />
          </button>
          <button className="ctool" data-attach title="Attach files" onClick={() => fileRef.current.click()}>
            <Icon body='<path d="M21 11.5 12.5 20a4 4 0 0 1-6-6l8-8a2.5 2.5 0 0 1 4 4l-8 8a1 1 0 0 1-1.5-1.5L17 11"/>' />
          </button>
          <input ref={fileRef} type="file" className="c-file" multiple hidden
            onChange={async () => { await ingestFiles(fileRef.current.files); fileRef.current.value = ""; }} />
        </div>
        <span className="c-status">{status}</span>
      </footer>
      {linkbar && (
        <div className="c-linkbar">
          <input ref={linkInputRef} type="url" placeholder="https://… or mailto:…" aria-label="Link URL"
            onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); applyLink(); } else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); setLinkbar(false); } }} />
          <button className="btn btn-primary" type="button" onClick={applyLink}>Add link</button>
          <button className="btn btn-ghost" type="button" onClick={() => setLinkbar(false)}>Cancel</button>
        </div>
      )}
      {dragging && <div className="compose-drop">Drop files to attach</div>}
    </div>
  );
}
