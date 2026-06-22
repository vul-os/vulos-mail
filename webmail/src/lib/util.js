// util.js — pure helpers ported 1:1 from the vanilla SPA (webmail/app.js).
// Formatting, avatar colors, keyword helpers, and HTML sanitization.

export const STAR = "m12 3 2.9 6 6.6.9-4.8 4.6 1.2 6.5L12 18l-5.9 3 1.2-6.5L2.5 9.9 9 9z";

export const SYS_ICONS = {
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
export const ORDER = ["inbox", "star", "important", "snoozed", "sent", "drafts", "archive", "spam", "trash"];

export function kw(m, k) { return !!(m && m.keywords && m.keywords[k]); }
export function from(m) { return (m && m.from && m.from[0]) || {}; }
export function fromName(m) { const f = from(m); return f.name || (f.email ? f.email.split("@")[0] : "(unknown)"); }
export function fromAddr(m) { return from(m).email || ""; }
export function addrList(a) { return (a || []).map((x) => x.name || x.email).join(", "); }

export function bodyText(m) {
  if (m && m.bodyValues) { const v = Object.values(m.bodyValues)[0]; if (v && v.value) return v.value; }
  return (m && m.preview) || "";
}

export function initials(n) {
  const p = (n || "").replace(/[^\p{L}\p{N}\s]/gu, " ").trim().split(/\s+/).filter(Boolean);
  return ((p[0] || "")[0] || "?").toUpperCase() + (p[1] ? p[1][0].toUpperCase() : "");
}

export function avatarColor(seed) {
  let h = 0; for (const c of seed || "") h = (h * 31 + c.charCodeAt(0)) >>> 0;
  const hues = [[15, 106, 108], [201, 106, 255], [45, 212, 191], [245, 158, 11]];
  const [r, g, b] = hues[h % hues.length];
  return `linear-gradient(135deg,rgb(${r},${g},${b}),rgb(${(r + 40) % 256},${(g + 30) % 256},${(b + 50) % 256}))`;
}

export function esc(s) {
  return (s == null ? "" : String(s)).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
}

// Sanitize HTML to a safe subset: drop scripts/styles/event handlers, keep only
// inline-formatting + links/lists. Used for paste into the composer AND for
// rendering remote HTML email bodies (see renderEmailHTML).
const ALLOW = new Set(["B", "STRONG", "I", "EM", "U", "A", "BR", "P", "DIV", "SPAN", "UL", "OL", "LI", "BLOCKQUOTE", "CODE", "PRE", "H1", "H2", "H3", "H4", "TABLE", "THEAD", "TBODY", "TR", "TD", "TH", "HR"]);
export function sanitizeHTML(html) {
  const tpl = document.createElement("template");
  tpl.innerHTML = html;
  const walk = (node) => {
    [...node.childNodes].forEach((n) => {
      if (n.nodeType === 1) {
        const tag = n.tagName;
        if (tag === "SCRIPT" || tag === "STYLE" || tag === "IFRAME" || tag === "OBJECT" || tag === "EMBED" || tag === "LINK" || tag === "META") { n.remove(); return; }
        if (!ALLOW.has(tag)) {
          walk(n);
          const parent = n.parentNode;
          while (n.firstChild) parent.insertBefore(n.firstChild, n);
          n.remove();
          return;
        }
        [...n.attributes].forEach((a) => {
          const name = a.name.toLowerCase();
          if (tag === "A" && name === "href" && /^(https?:|mailto:)/i.test(a.value.trim())) return;
          n.removeAttribute(a.name);
        });
        if (tag === "A") { n.setAttribute("target", "_blank"); n.setAttribute("rel", "noopener noreferrer"); }
        walk(n);
      } else if (n.nodeType !== 3) { n.remove(); }
    });
  };
  walk(tpl.content);
  return tpl.innerHTML;
}

export function linkify(s) {
  return s.replace(/(https?:\/\/[^\s<]+)/g, '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>');
}

// Escape, linkify per line, wrap runs of ">"-quoted lines in a blockquote.
// This is the XSS-inert plain-text renderer (escapes everything first).
export function fmtBody(body) {
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

// Decide how to render an email body. If the value is HTML (type text/html),
// sanitize it to an inert safe subset; otherwise treat as plain text. Either
// path is XSS-inert: no script/onerror survives.
export function renderEmailHTML(m) {
  let value = "", isHTML = false;
  if (m && m.bodyValues) {
    const v = Object.values(m.bodyValues)[0];
    if (v && v.value) {
      value = v.value;
      // Respect an explicit isHTML flag; only auto-detect when unspecified.
      isHTML = v.isHTML === true ? true
        : v.isHTML === false ? false
        : /<\/?[a-z][\s\S]*>/i.test(value);
    }
  }
  if (!value) value = (m && m.preview) || "";
  return isHTML ? sanitizeHTML(value) : fmtBody(value);
}

export function fmtDate(iso) {
  if (!iso) return "";
  const d = new Date(iso), now = new Date();
  if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  if (d.getFullYear() === now.getFullYear()) return d.toLocaleDateString([], { month: "short", day: "numeric" });
  return d.toLocaleDateString([], { year: "2-digit", month: "short", day: "numeric" });
}
export function fmtFull(iso) {
  return iso ? new Date(iso).toLocaleString([], { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }) : "";
}
export function fmtBytes(n) {
  if (!n) return "";
  if (n < 1024) return n + " B";
  if (n < 1048576) return (n / 1024).toFixed(0) + " KB";
  return (n / 1048576).toFixed(1) + " MB";
}

export function labelName(labels, id) { const m = labels.find((l) => l.id === id); return m ? m.name : id; }
