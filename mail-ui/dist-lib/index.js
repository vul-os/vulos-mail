import { jsx as t, jsxs as c, Fragment as cn } from "react/jsx-runtime";
import { useState as C, useRef as Oe, useEffect as _e, useMemo as ke, useId as da, useCallback as Y } from "react";
import { FLAG_FLAGGED as We, FLAG_SEEN as Ue, createMailClient as mn, ApiError as fa } from "./api.js";
const Tn = {
  inbox: "M22 12h-6l-2 3h-4l-2-3H2 M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z",
  send: "M22 2 11 13 M22 2 15 22l-4-9-9-4 20-7z",
  star: "M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z",
  trash: "M3 6h18 M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2",
  search: "M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z M21 21l-4.3-4.3",
  folder: "M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z",
  mail: "M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z M22 6l-10 7L2 6",
  mailopen: "M21 14V8.5a2 2 0 0 0-.9-1.67l-7-4.66a2 2 0 0 0-2.2 0l-7 4.66A2 2 0 0 0 3 8.5V14 M3 12l8.4 5.6a2 2 0 0 0 2.2 0L22 12 M3 21h18",
  edit: "M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7 M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z",
  pencil: "M12 20h9 M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z",
  back: "M19 12H5 M12 19l-7-7 7-7",
  close: "M18 6 6 18 M6 6l12 12",
  paperclip: "M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48",
  archive: "M21 8v13H3V8 M1 3h22v5H1z M10 12h4",
  calendar: "M8 2v4 M16 2v4 M3 10h18 M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z",
  users: "M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2 M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8 M23 21v-2a4 4 0 0 0-3-3.87 M16 3.13a4 4 0 0 1 0 7.75",
  prev: "M15 18l-6-6 6-6",
  next: "M9 18l6-6-6-6",
  plus: "M12 5v14 M5 12h14",
  list: "M8 6h13 M8 12h13 M8 18h13 M3 6h.01 M3 12h.01 M3 18h.01",
  grid: "M3 3h7v7H3z M14 3h7v7h-7z M14 14h7v7h-7z M3 14h7v7H3z",
  reply: "M9 17l-5-5 5-5 M4 12h11a5 5 0 0 1 5 5v1",
  replyall: "M7 17l-5-5 5-5 M12 17l-5-5 5-5 M7 12h8a5 5 0 0 1 5 5v1",
  forward: "M15 17l5-5-5-5 M20 12H9a5 5 0 0 0-5 5v1",
  more: "M12 13a1 1 0 1 0 0-2 1 1 0 0 0 0 2 M12 6a1 1 0 1 0 0-2 1 1 0 0 0 0 2 M12 20a1 1 0 1 0 0-2 1 1 0 0 0 0 2",
  check: "M20 6 9 17l-5-5",
  square: "M4 4h16a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z",
  checksquare: "M9 11l3 3L22 4 M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11",
  minus: "M5 12h14",
  expand: "M15 3h6v6 M9 21H3v-6 M21 3l-7 7 M3 21l7-7",
  collapse: "M4 14h6v6 M20 10h-6V4 M14 10l7-7 M3 21l7-7",
  settings: "M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6 M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z",
  keyboard: "M4 5h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2z M6 9h.01 M10 9h.01 M14 9h.01 M18 9h.01 M6 13h.01 M18 13h.01 M8 17h8",
  menu: "M3 6h18 M3 12h18 M3 18h18",
  bold: "M6 4h8a4 4 0 0 1 0 8H6z M6 12h9a4 4 0 0 1 0 8H6z",
  italic: "M19 4h-9 M14 20H5 M15 4 9 20",
  ul: "M8 6h13 M8 12h13 M8 18h13 M3 6h.01 M3 12h.01 M3 18h.01",
  ol: "M10 6h11 M10 12h11 M10 18h11 M4 6h1v4 M4 10h2 M6 18H4l2-2.5a1 1 0 0 0-1.5-1.3",
  link: "M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1 M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1",
  chevdown: "M6 9l6 6 6-6",
  chevup: "M18 15l-6-6-6 6",
  sun: "M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10 M12 1v2 M12 21v2 M4.2 4.2l1.4 1.4 M18.4 18.4l1.4 1.4 M1 12h2 M21 12h2 M4.2 19.8l1.4-1.4 M18.4 5.6l1.4-1.4",
  moon: "M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z",
  refresh: "M23 4v6h-6 M1 20v-6h6 M3.51 9a9 9 0 0 1 14.85-3.36L23 10 M1 14l4.64 4.36A9 9 0 0 0 20.49 15",
  draft: "M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z M14 2v6h6 M12 18v-6 M9 15h6",
  trashrestore: "M3 6h18 M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6 M9 11l3-3 3 3 M12 8v8",
  tag: "M20.59 13.41 13.42 20.6a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z M7 7h.01",
  attach: "M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48",
  print: "M6 9V2h12v7 M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2 M6 14h12v8H6z",
  clock: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18 M12 7v5l3 3",
  mappin: "M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z M12 13a3 3 0 1 0 0-6 3 3 0 0 0 0 6",
  contrast: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z M12 3v18",
  key: "M9.5 14.5a5 5 0 1 0 0-10 5 5 0 0 0 0 10z M13 11l8-8 M18 6l3 3",
  server: "M4 4h16a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z M4 14h16a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-2a2 2 0 0 1 2-2z M6 7h.01 M6 17h.01",
  copy: "M20 9h-9a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2v-9a2 2 0 0 0-2-2z M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1",
  logout: "M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4 M16 17l5-5-5-5 M21 12H9",
  info: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z M12 11v5 M12 8h.01",
  eye: "M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z",
  eyeoff: "M9.9 4.24A9.12 9.12 0 0 1 12 4c6.5 0 10 7 10 7a13.2 13.2 0 0 1-1.67 2.68 M6.61 6.61A13.5 13.5 0 0 0 2 12s3.5 7 10 7a9.12 9.12 0 0 0 3.39-.61 M1 1l22 22 M9.17 9.17a3 3 0 0 0 4.24 4.24",
  user: "M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2 M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z"
};
function p({ name: e, className: a = "vm-icon", fill: s = "none", ...i }) {
  const u = Tn[e] || Tn.mail;
  return /* @__PURE__ */ t("svg", { className: a, viewBox: "0 0 24 24", fill: s, "aria-hidden": "true", ...i, children: u.split(" M").map((o, g) => /* @__PURE__ */ t("path", { d: g === 0 ? o : "M" + o }, g)) });
}
const wt = "__starred", ha = ["inbox", "starred", "sent", "drafts", "archive", "trash", "junk"];
function sn(e) {
  const a = (e.attributes || e.Attributes || []).map((u) => String(u).toLowerCase()), s = String(e.name ?? e.path ?? "").toLowerCase(), i = (u) => a.includes("\\" + u) || s === u || s.endsWith("/" + u);
  return s === "inbox" || a.includes("\\inbox") ? "inbox" : i("sent") || s.includes("sent") ? "sent" : i("drafts") || s.includes("draft") ? "drafts" : i("trash") || s.includes("trash") || s.includes("deleted") || s === "bin" ? "trash" : i("archive") || s.includes("archive") ? "archive" : i("junk") || s.includes("junk") || s.includes("spam") ? "junk" : "label";
}
const pa = {
  inbox: "inbox",
  starred: "star",
  sent: "send",
  drafts: "draft",
  archive: "archive",
  trash: "trash",
  junk: "tag",
  label: "tag"
}, va = {
  inbox: "Inbox",
  starred: "Starred",
  sent: "Sent",
  drafts: "Drafts",
  archive: "Archive",
  trash: "Trash",
  junk: "Spam"
};
function ba({
  folders: e = [],
  current: a,
  onSelect: s,
  onCompose: i,
  me: u,
  collapsed: o = !1,
  onToggleCollapse: g,
  starredCount: d = 0,
  onOpenPanel: m,
  onOpenHelp: E
}) {
  const S = {}, k = [];
  for (const v of e) {
    const z = v.path ?? v.name ?? v.id, B = sn(v), U = {
      path: z,
      kind: B,
      label: va[B] ?? v.name ?? v.path ?? z,
      unread: v.unread ?? v.unseen ?? v.UnreadCount ?? 0
    };
    B === "label" ? k.push(U) : S[B] || (S[B] = U);
  }
  S.starred = { path: wt, kind: "starred", label: "Starred", unread: 0, count: d };
  const R = ha.map((v) => S[v]).filter(Boolean), A = (v) => {
    const z = v.path === a;
    return /* @__PURE__ */ t("li", { children: /* @__PURE__ */ c(
      "button",
      {
        type: "button",
        className: "vm-folder" + (z ? " vm-active" : ""),
        "aria-current": z ? "true" : void 0,
        onClick: () => s == null ? void 0 : s(v.path),
        title: v.label,
        children: [
          /* @__PURE__ */ t(p, { name: pa[v.kind] || "tag", className: "vm-icon" }),
          /* @__PURE__ */ t("span", { className: "vm-folder-name", children: v.label }),
          v.unread > 0 && /* @__PURE__ */ t("span", { className: "vm-folder-count", children: v.unread })
        ]
      }
    ) }, v.path);
  };
  return /* @__PURE__ */ c("nav", { className: "vm-sidebar" + (o ? " vm-collapsed" : ""), "aria-label": "Mailboxes", children: [
    /* @__PURE__ */ c("div", { className: "vm-brand", children: [
      /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-iconbtn vm-rail-toggle",
          "aria-label": o ? "Expand menu" : "Collapse menu",
          onClick: g,
          children: /* @__PURE__ */ t(p, { name: "menu" })
        }
      ),
      /* @__PURE__ */ c("span", { className: "vm-brand-text", children: [
        /* @__PURE__ */ t(p, { name: "mail", className: "vm-icon vm-brand-mark" }),
        /* @__PURE__ */ t("span", { children: "Vulos Mail" })
      ] })
    ] }),
    /* @__PURE__ */ c("button", { type: "button", className: "vm-compose-btn", onClick: i, title: "Compose", children: [
      /* @__PURE__ */ t(p, { name: "pencil" }),
      /* @__PURE__ */ t("span", { className: "vm-compose-label", children: "Compose" })
    ] }),
    /* @__PURE__ */ c("ul", { className: "vm-folders", children: [
      R.map(A),
      k.length > 0 && /* @__PURE__ */ t("li", { className: "vm-folder-section", "aria-hidden": "true", children: /* @__PURE__ */ t("span", { children: "Labels" }) }),
      k.map(A)
    ] }),
    (m || E) && /* @__PURE__ */ c("ul", { className: "vm-folders vm-drawer-extra", "aria-label": "Tools", children: [
      /* @__PURE__ */ t("li", { className: "vm-folder-section", "aria-hidden": "true", children: /* @__PURE__ */ t("span", { children: "More" }) }),
      m && /* @__PURE__ */ c(cn, { children: [
        /* @__PURE__ */ t("li", { children: /* @__PURE__ */ c("button", { type: "button", className: "vm-folder", onClick: () => m("calendar"), title: "Calendar", children: [
          /* @__PURE__ */ t(p, { name: "calendar", className: "vm-icon" }),
          /* @__PURE__ */ t("span", { className: "vm-folder-name", children: "Calendar" })
        ] }) }),
        /* @__PURE__ */ t("li", { children: /* @__PURE__ */ c("button", { type: "button", className: "vm-folder", onClick: () => m("contacts"), title: "Contacts", children: [
          /* @__PURE__ */ t(p, { name: "users", className: "vm-icon" }),
          /* @__PURE__ */ t("span", { className: "vm-folder-name", children: "Contacts" })
        ] }) }),
        /* @__PURE__ */ t("li", { children: /* @__PURE__ */ c("button", { type: "button", className: "vm-folder", onClick: () => m("settings"), title: "Settings", children: [
          /* @__PURE__ */ t(p, { name: "settings", className: "vm-icon" }),
          /* @__PURE__ */ t("span", { className: "vm-folder-name", children: "Settings" })
        ] }) })
      ] }),
      E && /* @__PURE__ */ t("li", { children: /* @__PURE__ */ c("button", { type: "button", className: "vm-folder", onClick: E, title: "Keyboard shortcuts", children: [
        /* @__PURE__ */ t(p, { name: "keyboard", className: "vm-icon" }),
        /* @__PURE__ */ t("span", { className: "vm-folder-name", children: "Shortcuts" })
      ] }) })
    ] }),
    (u == null ? void 0 : u.email) && /* @__PURE__ */ c("div", { className: "vm-sidebar-foot", title: u.email, children: [
      /* @__PURE__ */ t("span", { className: "vm-me-avatar", "aria-hidden": "true", children: (u.email[0] || "?").toUpperCase() }),
      /* @__PURE__ */ t("span", { className: "vm-me", children: u.email })
    ] })
  ] });
}
function rn(e = "", a = "") {
  const s = (e || a).trim();
  if (!s) return "?";
  const i = s.split(/\s+/).filter(Boolean);
  return i.length >= 2 && /[a-z]/i.test(i[1][0]) ? (i[0][0] + i[1][0]).toUpperCase() : s[0].toUpperCase();
}
function ga(e = "") {
  let a = 0;
  for (let s = 0; s < e.length; s++) a = (a * 31 + e.charCodeAt(s)) % 360;
  return a;
}
function Ut(e = "") {
  const a = ga(e.toLowerCase());
  return {
    background: `hsl(${a} 42% 38%)`,
    color: `hsl(${a} 60% 92%)`
  };
}
function Na(e) {
  const a = new Date(e);
  if (Number.isNaN(a.getTime())) return "";
  const s = /* @__PURE__ */ new Date();
  return a.toDateString() === s.toDateString() ? a.toLocaleTimeString(void 0, { hour: "numeric", minute: "2-digit" }) : a.getFullYear() === s.getFullYear() ? a.toLocaleDateString(void 0, { month: "short", day: "numeric" }) : a.toLocaleDateString(void 0, { year: "2-digit", month: "numeric", day: "numeric" });
}
function jt(e) {
  const a = new Date(e);
  return Number.isNaN(a.getTime()) ? "" : a.toLocaleString(void 0, {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit"
  });
}
function Mn(e = "") {
  return e.split(/[,;]/).map((a) => a.trim()).filter(Boolean);
}
function ya({
  threads: e = [],
  selectedId: a,
  focusId: s,
  selection: i,
  onToggleSelect: u,
  onSelectRange: o,
  onSelectAll: g,
  onOpen: d,
  onToggleStar: m,
  onArchive: E,
  onDelete: S,
  onToggleRead: k,
  onRefresh: R,
  onCompose: A,
  loading: v,
  error: z,
  onRetry: B,
  query: U = "",
  onSearch: D,
  onClearSearch: w,
  canArchive: F = !0,
  folder: $ = "INBOX",
  searchRef: W,
  onMenu: b
}) {
  const [L, K] = C(U), X = Oe(null), Ee = Oe(null), J = Oe(null);
  _e(() => {
    K(U);
  }, [U]);
  const ee = i ? i.size : 0, ge = e.length > 0 && ee === e.length;
  function Ne(T) {
    T.preventDefault(), D == null || D(L.trim());
  }
  const le = (T) => (oe) => {
    oe.stopPropagation(), T == null || T(oe);
  };
  function ie(T, oe, ne) {
    if (T.stopPropagation(), T.shiftKey && J.current != null && o) {
      const G = Math.min(J.current, oe), y = Math.max(J.current, oe);
      o(e.slice(G, y + 1).map((Se) => Se.id));
    } else
      u == null || u(ne);
    J.current = oe;
  }
  return /* @__PURE__ */ c("section", { className: "vm-list", "aria-label": "Messages", children: [
    /* @__PURE__ */ c("div", { className: "vm-topbar", children: [
      /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-menu-btn", "aria-label": "Menu", onClick: b, children: /* @__PURE__ */ t(p, { name: "menu" }) }),
      /* @__PURE__ */ c("form", { className: "vm-search", onSubmit: Ne, role: "search", children: [
        /* @__PURE__ */ t(p, { name: "search", className: "vm-icon" }),
        /* @__PURE__ */ t(
          "input",
          {
            ref: (T) => {
              Ee.current = T, W && (W.current = T);
            },
            type: "search",
            value: L,
            placeholder: "Search mail",
            onChange: (T) => K(T.target.value),
            "aria-label": "Search mail"
          }
        ),
        L && /* @__PURE__ */ t(
          "button",
          {
            type: "button",
            className: "vm-search-clear",
            "aria-label": "Clear search",
            onClick: () => {
              K(""), w == null || w();
            },
            children: /* @__PURE__ */ t(p, { name: "close" })
          }
        )
      ] })
    ] }),
    /* @__PURE__ */ c("div", { className: "vm-toolbar", children: [
      /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-checkbox" + (ge ? " vm-on" : ee > 0 ? " vm-some" : ""),
          role: "checkbox",
          "aria-checked": ge ? "true" : ee > 0 ? "mixed" : "false",
          "aria-label": ge ? "Deselect all" : "Select all",
          onClick: () => g == null ? void 0 : g(!ge),
          children: /* @__PURE__ */ t(p, { name: ee > 0 && !ge ? "minus" : "check" })
        }
      ),
      ee > 0 ? /* @__PURE__ */ c("div", { className: "vm-bulk", role: "toolbar", "aria-label": "Bulk actions", children: [
        /* @__PURE__ */ c("span", { className: "vm-bulk-count", children: [
          ee,
          " selected"
        ] }),
        F && /* @__PURE__ */ t(
          "button",
          {
            type: "button",
            className: "vm-iconbtn",
            "aria-label": "Archive selected",
            title: "Archive",
            onClick: () => E == null ? void 0 : E(null),
            children: /* @__PURE__ */ t(p, { name: "archive" })
          }
        ),
        /* @__PURE__ */ t(
          "button",
          {
            type: "button",
            className: "vm-iconbtn vm-danger",
            "aria-label": "Delete selected",
            title: "Delete",
            onClick: () => S == null ? void 0 : S(null),
            children: /* @__PURE__ */ t(p, { name: "trash" })
          }
        ),
        /* @__PURE__ */ t(
          "button",
          {
            type: "button",
            className: "vm-iconbtn",
            "aria-label": "Mark read",
            title: "Mark read",
            onClick: () => k == null ? void 0 : k(null, !0),
            children: /* @__PURE__ */ t(p, { name: "mailopen" })
          }
        ),
        /* @__PURE__ */ t(
          "button",
          {
            type: "button",
            className: "vm-iconbtn",
            "aria-label": "Mark unread",
            title: "Mark unread",
            onClick: () => k == null ? void 0 : k(null, !1),
            children: /* @__PURE__ */ t(p, { name: "mail" })
          }
        ),
        /* @__PURE__ */ t(
          "button",
          {
            type: "button",
            className: "vm-iconbtn",
            "aria-label": "Star selected",
            title: "Star",
            onClick: () => m == null ? void 0 : m(null, !0),
            children: /* @__PURE__ */ t(p, { name: "star" })
          }
        )
      ] }) : /* @__PURE__ */ c(cn, { children: [
        U && /* @__PURE__ */ c("span", { className: "vm-query-chip", children: [
          /* @__PURE__ */ t(p, { name: "search" }),
          " ",
          U,
          /* @__PURE__ */ t("button", { type: "button", "aria-label": "Clear search", onClick: w, children: /* @__PURE__ */ t(p, { name: "close" }) })
        ] }),
        /* @__PURE__ */ t("span", { className: "vm-spacer" }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Refresh", title: "Refresh", onClick: R, children: /* @__PURE__ */ t(p, { name: "refresh" }) })
      ] })
    ] }),
    z ? /* @__PURE__ */ c("div", { className: "vm-empty vm-state", role: "alert", children: [
      /* @__PURE__ */ t(p, { name: "refresh", className: "vm-empty-icon" }),
      /* @__PURE__ */ t("p", { children: z }),
      /* @__PURE__ */ t("button", { type: "button", className: "vm-btn vm-btn-ghost", onClick: B, children: "Retry" })
    ] }) : v ? /* @__PURE__ */ t("ul", { className: "vm-rows", children: Array.from({ length: 9 }).map((T, oe) => /* @__PURE__ */ c("li", { className: "vm-skeleton", "aria-hidden": "true", children: [
      /* @__PURE__ */ t("div", { className: "vm-sk-avatar" }),
      /* @__PURE__ */ c("div", { className: "vm-sk-lines", children: [
        /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "38%" } }),
        /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "72%" } })
      ] })
    ] }, oe)) }) : e.length === 0 ? /* @__PURE__ */ c("div", { className: "vm-empty vm-state", children: [
      /* @__PURE__ */ t(p, { name: U ? "search" : "inbox", className: "vm-empty-icon" }),
      /* @__PURE__ */ t("p", { children: U ? "No results" : Ta($) }),
      !U && A && /* @__PURE__ */ c("button", { type: "button", className: "vm-btn vm-btn-primary", onClick: A, children: [
        /* @__PURE__ */ t(p, { name: "pencil" }),
        " Compose"
      ] })
    ] }) : /* @__PURE__ */ t("ul", { className: "vm-rows", ref: X, children: e.map((T, oe) => {
      const ne = i == null ? void 0 : i.has(T.id), G = T.fromName || T.from || "(unknown)";
      return /* @__PURE__ */ t("li", { children: /* @__PURE__ */ c(
        "div",
        {
          className: "vm-row" + (T.id === a ? " vm-active" : "") + (T.id === s ? " vm-focus" : "") + (T.unread ? " vm-unread" : "") + (ne ? " vm-selected" : ""),
          role: "button",
          tabIndex: 0,
          "aria-label": `${G}: ${T.subject || "(no subject)"}`,
          onClick: () => d == null ? void 0 : d(T),
          onKeyDown: (y) => {
            (y.key === "Enter" || y.key === " ") && (y.preventDefault(), d == null || d(T));
          },
          children: [
            /* @__PURE__ */ t(
              "button",
              {
                type: "button",
                className: "vm-checkbox vm-row-check" + (ne ? " vm-on" : ""),
                role: "checkbox",
                "aria-checked": ne ? "true" : "false",
                "aria-label": ne ? "Deselect" : "Select",
                onClick: (y) => ie(y, oe, T.id),
                children: /* @__PURE__ */ t(p, { name: "check" })
              }
            ),
            /* @__PURE__ */ t(
              "button",
              {
                type: "button",
                className: "vm-star" + (T.starred ? " vm-on" : ""),
                "aria-label": T.starred ? "Unstar" : "Star",
                "aria-pressed": T.starred,
                onClick: le(() => m == null ? void 0 : m(T, !T.starred)),
                children: /* @__PURE__ */ t(p, { name: "star", fill: T.starred ? "currentColor" : "none" })
              }
            ),
            /* @__PURE__ */ t("span", { className: "vm-avatar", style: Ut(T.from || G), "aria-hidden": "true", children: rn(T.fromName, T.from) }),
            /* @__PURE__ */ c("span", { className: "vm-row-main", children: [
              /* @__PURE__ */ c("span", { className: "vm-row-top", children: [
                /* @__PURE__ */ c("span", { className: "vm-row-from", children: [
                  G,
                  T.count > 1 && /* @__PURE__ */ t("span", { className: "vm-thread-count", children: T.count })
                ] }),
                /* @__PURE__ */ t("span", { className: "vm-row-date", children: Na(T.date) })
              ] }),
              /* @__PURE__ */ c("span", { className: "vm-row-line", children: [
                /* @__PURE__ */ t("span", { className: "vm-row-subj", children: T.subject || "(no subject)" }),
                T.hasAttachments && /* @__PURE__ */ t(p, { name: "paperclip", className: "vm-attach-dot" })
              ] }),
              /* @__PURE__ */ t("span", { className: "vm-row-snip", children: T.preview })
            ] }),
            /* @__PURE__ */ c("span", { className: "vm-row-actions", children: [
              F && /* @__PURE__ */ t(
                "button",
                {
                  type: "button",
                  className: "vm-iconbtn",
                  "aria-label": "Archive",
                  title: "Archive",
                  onClick: le(() => E == null ? void 0 : E(T)),
                  children: /* @__PURE__ */ t(p, { name: "archive" })
                }
              ),
              /* @__PURE__ */ t(
                "button",
                {
                  type: "button",
                  className: "vm-iconbtn vm-danger",
                  "aria-label": "Delete",
                  title: "Delete",
                  onClick: le(() => S == null ? void 0 : S(T)),
                  children: /* @__PURE__ */ t(p, { name: "trash" })
                }
              ),
              /* @__PURE__ */ t(
                "button",
                {
                  type: "button",
                  className: "vm-iconbtn",
                  "aria-label": T.unread ? "Mark read" : "Mark unread",
                  title: T.unread ? "Mark read" : "Mark unread",
                  onClick: le(() => k == null ? void 0 : k(T, T.unread)),
                  children: /* @__PURE__ */ t(p, { name: T.unread ? "mailopen" : "mail" })
                }
              )
            ] })
          ]
        }
      ) }, T.id);
    }) })
  ] });
}
function Ta(e) {
  const a = String(e || "").toLowerCase();
  return a === "__starred" ? "No starred conversations" : a.includes("sent") ? "No sent messages" : a.includes("draft") ? "No drafts" : a.includes("trash") || a.includes("deleted") ? "Trash is empty" : a.includes("archive") ? "Nothing archived" : "Nothing here — inbox zero";
}
/*! @license DOMPurify 3.4.11 | (c) Cure53 and other contributors | Released under the Apache license 2.0 and Mozilla Public License 2.0 | github.com/cure53/DOMPurify/blob/3.4.11/LICENSE */
function _n(e, a) {
  (a == null || a > e.length) && (a = e.length);
  for (var s = 0, i = Array(a); s < a; s++) i[s] = e[s];
  return i;
}
function Ma(e) {
  if (Array.isArray(e)) return e;
}
function _a(e, a) {
  var s = e == null ? null : typeof Symbol < "u" && e[Symbol.iterator] || e["@@iterator"];
  if (s != null) {
    var i, u, o, g, d = [], m = !0, E = !1;
    try {
      if (o = (s = s.call(e)).next, a !== 0) for (; !(m = (i = o.call(s)).done) && (d.push(i.value), d.length !== a); m = !0) ;
    } catch (S) {
      E = !0, u = S;
    } finally {
      try {
        if (!m && s.return != null && (g = s.return(), Object(g) !== g)) return;
      } finally {
        if (E) throw u;
      }
    }
    return d;
  }
}
function Ea() {
  throw new TypeError(`Invalid attempt to destructure non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`);
}
function Sa(e, a) {
  return Ma(e) || _a(e, a) || Aa(e, a) || Ea();
}
function Aa(e, a) {
  if (e) {
    if (typeof e == "string") return _n(e, a);
    var s = {}.toString.call(e).slice(8, -1);
    return s === "Object" && e.constructor && (s = e.constructor.name), s === "Map" || s === "Set" ? Array.from(e) : s === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(s) ? _n(e, a) : void 0;
  }
}
const Vn = Object.entries, En = Object.setPrototypeOf, wa = Object.isFrozen, ka = Object.getPrototypeOf, Ca = Object.getOwnPropertyDescriptor;
let pe = Object.freeze, ve = Object.seal, bt = Object.create, Yn = typeof Reflect < "u" && Reflect, ln = Yn.apply, on = Yn.construct;
pe || (pe = function(a) {
  return a;
});
ve || (ve = function(a) {
  return a;
});
ln || (ln = function(a, s) {
  for (var i = arguments.length, u = new Array(i > 2 ? i - 2 : 0), o = 2; o < i; o++)
    u[o - 2] = arguments[o];
  return a.apply(s, u);
});
on || (on = function(a) {
  for (var s = arguments.length, i = new Array(s > 1 ? s - 1 : 0), u = 1; u < s; u++)
    i[u - 1] = arguments[u];
  return new a(...i);
});
const _t = se(Array.prototype.forEach), Da = se(Array.prototype.lastIndexOf), Sn = se(Array.prototype.pop), vt = se(Array.prototype.push), La = se(Array.prototype.splice), qe = Array.isArray, At = se(String.prototype.toLowerCase), Kt = se(String.prototype.toString), An = se(String.prototype.match), Et = se(String.prototype.replace), wn = se(String.prototype.indexOf), Oa = se(String.prototype.trim), Ra = se(Number.prototype.toString), Ia = se(Boolean.prototype.toString), kn = typeof BigInt > "u" ? null : se(BigInt.prototype.toString), Cn = typeof Symbol > "u" ? null : se(Symbol.prototype.toString), ue = se(Object.prototype.hasOwnProperty), St = se(Object.prototype.toString), he = se(RegExp.prototype.test), rt = xa(TypeError);
function se(e) {
  return function(a) {
    a instanceof RegExp && (a.lastIndex = 0);
    for (var s = arguments.length, i = new Array(s > 1 ? s - 1 : 0), u = 1; u < s; u++)
      i[u - 1] = arguments[u];
    return ln(e, a, i);
  };
}
function xa(e) {
  return function() {
    for (var a = arguments.length, s = new Array(a), i = 0; i < a; i++)
      s[i] = arguments[i];
    return on(e, s);
  };
}
function H(e, a) {
  let s = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : At;
  if (En && En(e, null), !qe(a))
    return e;
  let i = a.length;
  for (; i--; ) {
    let u = a[i];
    if (typeof u == "string") {
      const o = s(u);
      o !== u && (wa(a) || (a[i] = o), u = o);
    }
    e[u] = !0;
  }
  return e;
}
function Pa(e) {
  for (let a = 0; a < e.length; a++)
    ue(e, a) || (e[a] = null);
  return e;
}
function Me(e) {
  const a = bt(null);
  for (const i of Vn(e)) {
    var s = Sa(i, 2);
    const u = s[0], o = s[1];
    ue(e, u) && (qe(o) ? a[u] = Pa(o) : o && typeof o == "object" && o.constructor === Object ? a[u] = Me(o) : a[u] = o);
  }
  return a;
}
function Fa(e) {
  switch (typeof e) {
    case "string":
      return e;
    case "number":
      return Ra(e);
    case "boolean":
      return Ia(e);
    case "bigint":
      return kn ? kn(e) : "0";
    case "symbol":
      return Cn ? Cn(e) : "Symbol()";
    case "undefined":
      return St(e);
    case "function":
    case "object": {
      if (e === null)
        return St(e);
      const a = e, s = ze(a, "toString");
      if (typeof s == "function") {
        const i = s(a);
        return typeof i == "string" ? i : St(i);
      }
      return St(e);
    }
    default:
      return St(e);
  }
}
function ze(e, a) {
  for (; e !== null; ) {
    const i = Ca(e, a);
    if (i) {
      if (i.get)
        return se(i.get);
      if (typeof i.value == "function")
        return se(i.value);
    }
    e = ka(e);
  }
  function s() {
    return null;
  }
  return s;
}
function Ha(e) {
  try {
    return he(e, ""), !0;
  } catch {
    return !1;
  }
}
const Dn = pe(["a", "abbr", "acronym", "address", "area", "article", "aside", "audio", "b", "bdi", "bdo", "big", "blink", "blockquote", "body", "br", "button", "canvas", "caption", "center", "cite", "code", "col", "colgroup", "content", "data", "datalist", "dd", "decorator", "del", "details", "dfn", "dialog", "dir", "div", "dl", "dt", "element", "em", "fieldset", "figcaption", "figure", "font", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr", "html", "i", "img", "input", "ins", "kbd", "label", "legend", "li", "main", "map", "mark", "marquee", "menu", "menuitem", "meter", "nav", "nobr", "ol", "optgroup", "option", "output", "p", "picture", "pre", "progress", "q", "rp", "rt", "ruby", "s", "samp", "search", "section", "select", "shadow", "slot", "small", "source", "spacer", "span", "strike", "strong", "style", "sub", "summary", "sup", "table", "tbody", "td", "template", "textarea", "tfoot", "th", "thead", "time", "tr", "track", "tt", "u", "ul", "var", "video", "wbr"]), Xt = pe(["svg", "a", "altglyph", "altglyphdef", "altglyphitem", "animatecolor", "animatemotion", "animatetransform", "circle", "clippath", "defs", "desc", "ellipse", "enterkeyhint", "exportparts", "filter", "font", "g", "glyph", "glyphref", "hkern", "image", "inputmode", "line", "lineargradient", "marker", "mask", "metadata", "mpath", "part", "path", "pattern", "polygon", "polyline", "radialgradient", "rect", "stop", "style", "switch", "symbol", "text", "textpath", "title", "tref", "tspan", "view", "vkern"]), qt = pe(["feBlend", "feColorMatrix", "feComponentTransfer", "feComposite", "feConvolveMatrix", "feDiffuseLighting", "feDisplacementMap", "feDistantLight", "feDropShadow", "feFlood", "feFuncA", "feFuncB", "feFuncG", "feFuncR", "feGaussianBlur", "feImage", "feMerge", "feMergeNode", "feMorphology", "feOffset", "fePointLight", "feSpecularLighting", "feSpotLight", "feTile", "feTurbulence"]), za = pe(["animate", "color-profile", "cursor", "discard", "font-face", "font-face-format", "font-face-name", "font-face-src", "font-face-uri", "foreignobject", "hatch", "hatchpath", "mesh", "meshgradient", "meshpatch", "meshrow", "missing-glyph", "script", "set", "solidcolor", "unknown", "use"]), Jt = pe(["math", "menclose", "merror", "mfenced", "mfrac", "mglyph", "mi", "mlabeledtr", "mmultiscripts", "mn", "mo", "mover", "mpadded", "mphantom", "mroot", "mrow", "ms", "mspace", "msqrt", "mstyle", "msub", "msup", "msubsup", "mtable", "mtd", "mtext", "mtr", "munder", "munderover", "mprescripts"]), Ua = pe(["maction", "maligngroup", "malignmark", "mlongdiv", "mscarries", "mscarry", "msgroup", "mstack", "msline", "msrow", "semantics", "annotation", "annotation-xml", "mprescripts", "none"]), Ln = pe(["#text"]), On = pe(["accept", "action", "align", "alt", "autocapitalize", "autocomplete", "autopictureinpicture", "autoplay", "background", "bgcolor", "border", "capture", "cellpadding", "cellspacing", "checked", "cite", "class", "clear", "color", "cols", "colspan", "command", "commandfor", "controls", "controlslist", "coords", "crossorigin", "datetime", "decoding", "default", "dir", "disabled", "disablepictureinpicture", "disableremoteplayback", "download", "draggable", "enctype", "enterkeyhint", "exportparts", "face", "for", "headers", "height", "hidden", "high", "href", "hreflang", "id", "inert", "inputmode", "integrity", "ismap", "kind", "label", "lang", "list", "loading", "loop", "low", "max", "maxlength", "media", "method", "min", "minlength", "multiple", "muted", "name", "nonce", "noshade", "novalidate", "nowrap", "open", "optimum", "part", "pattern", "placeholder", "playsinline", "popover", "popovertarget", "popovertargetaction", "poster", "preload", "pubdate", "radiogroup", "readonly", "rel", "required", "rev", "reversed", "role", "rows", "rowspan", "spellcheck", "scope", "selected", "shape", "size", "sizes", "slot", "span", "srclang", "start", "src", "srcset", "step", "style", "summary", "tabindex", "title", "translate", "type", "usemap", "valign", "value", "width", "wrap", "xmlns"]), Zt = pe(["accent-height", "accumulate", "additive", "alignment-baseline", "amplitude", "ascent", "attributename", "attributetype", "azimuth", "basefrequency", "baseline-shift", "begin", "bias", "by", "class", "clip", "clippathunits", "clip-path", "clip-rule", "color", "color-interpolation", "color-interpolation-filters", "color-profile", "color-rendering", "cx", "cy", "d", "dx", "dy", "diffuseconstant", "direction", "display", "divisor", "dur", "edgemode", "elevation", "end", "exponent", "fill", "fill-opacity", "fill-rule", "filter", "filterunits", "flood-color", "flood-opacity", "font-family", "font-size", "font-size-adjust", "font-stretch", "font-style", "font-variant", "font-weight", "fx", "fy", "g1", "g2", "glyph-name", "glyphref", "gradientunits", "gradienttransform", "height", "href", "id", "image-rendering", "in", "in2", "intercept", "k", "k1", "k2", "k3", "k4", "kerning", "keypoints", "keysplines", "keytimes", "lang", "lengthadjust", "letter-spacing", "kernelmatrix", "kernelunitlength", "lighting-color", "local", "marker-end", "marker-mid", "marker-start", "markerheight", "markerunits", "markerwidth", "maskcontentunits", "maskunits", "max", "mask", "mask-type", "media", "method", "mode", "min", "name", "numoctaves", "offset", "operator", "opacity", "order", "orient", "orientation", "origin", "overflow", "paint-order", "path", "pathlength", "patterncontentunits", "patterntransform", "patternunits", "points", "preservealpha", "preserveaspectratio", "primitiveunits", "r", "rx", "ry", "radius", "refx", "refy", "repeatcount", "repeatdur", "restart", "result", "rotate", "scale", "seed", "shape-rendering", "slope", "specularconstant", "specularexponent", "spreadmethod", "startoffset", "stddeviation", "stitchtiles", "stop-color", "stop-opacity", "stroke-dasharray", "stroke-dashoffset", "stroke-linecap", "stroke-linejoin", "stroke-miterlimit", "stroke-opacity", "stroke", "stroke-width", "style", "surfacescale", "systemlanguage", "tabindex", "tablevalues", "targetx", "targety", "transform", "transform-origin", "text-anchor", "text-decoration", "text-rendering", "textlength", "type", "u1", "u2", "unicode", "values", "viewbox", "visibility", "version", "vert-adv-y", "vert-origin-x", "vert-origin-y", "width", "word-spacing", "wrap", "writing-mode", "xchannelselector", "ychannelselector", "x", "x1", "x2", "xmlns", "y", "y1", "y2", "z", "zoomandpan"]), Rn = pe(["accent", "accentunder", "align", "bevelled", "close", "columnalign", "columnlines", "columnspacing", "columnspan", "denomalign", "depth", "dir", "display", "displaystyle", "encoding", "fence", "frame", "height", "href", "id", "largeop", "length", "linethickness", "lquote", "lspace", "mathbackground", "mathcolor", "mathsize", "mathvariant", "maxsize", "minsize", "movablelimits", "notation", "numalign", "open", "rowalign", "rowlines", "rowspacing", "rowspan", "rspace", "rquote", "scriptlevel", "scriptminsize", "scriptsizemultiplier", "selection", "separator", "separators", "stretchy", "subscriptshift", "supscriptshift", "symmetric", "voffset", "width", "xmlns"]), Ft = pe(["xlink:href", "xml:id", "xlink:title", "xml:space", "xmlns:xlink"]), ja = ve(/{{[\w\W]*|^[\w\W]*}}/g), Ba = ve(/<%[\w\W]*|^[\w\W]*%>/g), Ga = ve(/\${[\w\W]*/g), Wa = ve(/^data-[\-\w.\u00B7-\uFFFF]+$/), Va = ve(/^aria-[\-\w]+$/), In = ve(
  /^(?:(?:(?:f|ht)tps?|mailto|tel|callto|sms|cid|xmpp|matrix):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i
  // eslint-disable-line no-useless-escape
), Ya = ve(/^(?:\w+script|data):/i), $a = ve(
  /[\u0000-\u0020\u00A0\u1680\u180E\u2000-\u2029\u205F\u3000]/g
  // eslint-disable-line no-control-regex
), Ka = ve(/^html$/i), Xa = ve(/^[a-z][.\w]*(-[.\w]+)+$/i), xn = ve(/<[/\w!]/g), qa = ve(/<[/\w]/g), Ja = ve(/<\/no(script|embed|frames)/i), Za = ve(/\/>/i), He = {
  element: 1,
  attribute: 2,
  text: 3,
  cdataSection: 4,
  entityReference: 5,
  // Deprecated
  entityNode: 6,
  // Deprecated
  processingInstruction: 7,
  comment: 8,
  document: 9,
  documentType: 10,
  documentFragment: 11,
  notation: 12
  // Deprecated
}, Qa = function() {
  return typeof window > "u" ? null : window;
}, es = function(a, s) {
  if (typeof a != "object" || typeof a.createPolicy != "function")
    return null;
  let i = null;
  const u = "data-tt-policy-suffix";
  s && s.hasAttribute(u) && (i = s.getAttribute(u));
  const o = "dompurify" + (i ? "#" + i : "");
  try {
    return a.createPolicy(o, {
      createHTML(g) {
        return g;
      },
      createScriptURL(g) {
        return g;
      }
    });
  } catch {
    return console.warn("TrustedTypes policy " + o + " could not be created."), null;
  }
}, Pn = function() {
  return {
    afterSanitizeAttributes: [],
    afterSanitizeElements: [],
    afterSanitizeShadowDOM: [],
    beforeSanitizeAttributes: [],
    beforeSanitizeElements: [],
    beforeSanitizeShadowDOM: [],
    uponSanitizeAttribute: [],
    uponSanitizeElement: [],
    uponSanitizeShadowNode: []
  };
}, Ke = function(a, s, i, u) {
  return ue(a, s) && qe(a[s]) ? H(u.base ? Me(u.base) : {}, a[s], u.transform) : i;
};
function $n() {
  let e = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : Qa();
  const a = (h) => $n(h);
  if (a.version = "3.4.11", a.removed = [], !e || !e.document || e.document.nodeType !== He.document || !e.Element)
    return a.isSupported = !1, a;
  let s = e.document;
  const i = s, u = i.currentScript;
  e.DocumentFragment;
  const o = e.HTMLTemplateElement, g = e.Node, d = e.Element, m = e.NodeFilter, E = e.NamedNodeMap;
  E === void 0 && (e.NamedNodeMap || e.MozNamedAttrMap), e.HTMLFormElement;
  const S = e.DOMParser, k = e.trustedTypes, R = d.prototype, A = ze(R, "cloneNode"), v = ze(R, "remove"), z = ze(R, "nextSibling"), B = ze(R, "childNodes"), U = ze(R, "parentNode"), D = ze(R, "shadowRoot"), w = ze(R, "attributes"), F = g && g.prototype ? ze(g.prototype, "nodeType") : null, $ = g && g.prototype ? ze(g.prototype, "nodeName") : null;
  if (typeof o == "function") {
    const h = s.createElement("template");
    h.content && h.content.ownerDocument && (s = h.content.ownerDocument);
  }
  let W, b = "", L, K = !1, X = 0;
  const Ee = function() {
    if (X > 0)
      throw rt('A configured TRUSTED_TYPES_POLICY callback (createHTML or createScriptURL) must not call DOMPurify.sanitize, as that causes infinite recursion. Do not pass a policy whose callbacks wrap DOMPurify as TRUSTED_TYPES_POLICY; see the "DOMPurify and Trusted Types" section of the README.');
  }, J = function(n) {
    Ee(), X++;
    try {
      return W.createHTML(n);
    } finally {
      X--;
    }
  }, ee = function(n) {
    Ee(), X++;
    try {
      return W.createScriptURL(n);
    } finally {
      X--;
    }
  }, ge = function() {
    return K || (L = es(k, u), K = !0), L;
  }, Ne = s, le = Ne.implementation, ie = Ne.createNodeIterator, T = Ne.createDocumentFragment, oe = Ne.getElementsByTagName, ne = i.importNode;
  let G = Pn();
  a.isSupported = typeof Vn == "function" && typeof U == "function" && le && le.createHTMLDocument !== void 0;
  const y = ja, Se = Ba, ye = Ga, lt = Wa, it = Va, Ve = Ya, Re = $a, ot = Xa;
  let kt = In, q = null;
  const Ct = H({}, [...Dn, ...Xt, ...qt, ...Jt, ...Ln]);
  let Z = null;
  const Dt = H({}, [...On, ...Zt, ...Rn, ...Ft]);
  let Q = Object.seal(bt(null, {
    tagNameCheck: {
      writable: !0,
      configurable: !1,
      enumerable: !0,
      value: null
    },
    attributeNameCheck: {
      writable: !0,
      configurable: !1,
      enumerable: !0,
      value: null
    },
    allowCustomizedBuiltInElements: {
      writable: !0,
      configurable: !1,
      enumerable: !0,
      value: !1
    }
  })), Pe = null, be = null;
  const Ce = Object.seal(bt(null, {
    tagCheck: {
      writable: !0,
      configurable: !1,
      enumerable: !0,
      value: null
    },
    attributeCheck: {
      writable: !0,
      configurable: !1,
      enumerable: !0,
      value: null
    }
  }));
  let Je = !0, Ze = !0, Qe = !1, re = !0, V = !1, et = !0, de = !1, Ye = !1, gt = null, tt = null, De = !1, Ie = !1, ct = !1, je = !1, nt = !0, mt = !1;
  const Lt = "user-content-";
  let Nt = !0, Ae = !1, Fe = {}, ce = null;
  const yt = H({}, [
    "annotation-xml",
    "audio",
    "colgroup",
    "desc",
    "foreignobject",
    "head",
    "iframe",
    "math",
    "mi",
    "mn",
    "mo",
    "ms",
    "mtext",
    "noembed",
    "noframes",
    "noscript",
    "plaintext",
    "script",
    // <selectedcontent> mirrors the selected <option>'s subtree, cloned by
    // the UA (customizable <select>) — including any on* handlers — and the
    // engine re-mirrors synchronously whenever a removal changes which
    // option/selectedcontent is current, even inside DOMPurify's inert
    // DOMParser document. Hoisting its children on removal re-inserts a fresh
    // mirror target ahead of the walk, which the engine refills, looping
    // forever (DoS) and amplifying output. Dropping its content on removal
    // (rather than hoisting) breaks that cascade; the content is a duplicate
    // of the option, which is sanitized on its own. See campaign-3 F1/F6.
    "selectedcontent",
    "style",
    "svg",
    "template",
    "thead",
    "title",
    "video",
    "xmp"
  ]);
  let Ot = null;
  const Rt = H({}, ["audio", "video", "img", "source", "image", "track"]);
  let at = null;
  const ut = H({}, ["alt", "class", "for", "id", "label", "name", "pattern", "placeholder", "role", "summary", "title", "value", "style", "xmlns"]), dt = "http://www.w3.org/1998/Math/MathML", ft = "http://www.w3.org/2000/svg", we = "http://www.w3.org/1999/xhtml";
  let r = we, N = !1, _ = null;
  const P = H({}, [dt, ft, we], Kt), I = pe(["mi", "mo", "mn", "ms", "mtext"]);
  let x = H({}, I);
  const fe = pe(["annotation-xml"]);
  let Be = H({}, fe);
  const Jn = H({}, ["title", "style", "font", "a", "script"]);
  let Tt = null;
  const Zn = ["application/xhtml+xml", "text/html"], Qn = "text/html";
  let te = null, ht = null;
  const ea = s.createElement("form"), un = function(n) {
    return n instanceof RegExp || n instanceof Function;
  }, Wt = function() {
    let n = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    if (ht && ht === n)
      return;
    (!n || typeof n != "object") && (n = {}), n = Me(n), Tt = // eslint-disable-next-line unicorn/prefer-includes
    Zn.indexOf(n.PARSER_MEDIA_TYPE) === -1 ? Qn : n.PARSER_MEDIA_TYPE, te = Tt === "application/xhtml+xml" ? Kt : At, q = Ke(n, "ALLOWED_TAGS", Ct, {
      transform: te
    }), Z = Ke(n, "ALLOWED_ATTR", Dt, {
      transform: te
    }), _ = Ke(n, "ALLOWED_NAMESPACES", P, {
      transform: Kt
    }), at = Ke(n, "ADD_URI_SAFE_ATTR", ut, {
      transform: te,
      base: ut
    }), Ot = Ke(n, "ADD_DATA_URI_TAGS", Rt, {
      transform: te,
      base: Rt
    }), ce = Ke(n, "FORBID_CONTENTS", yt, {
      transform: te
    }), Pe = Ke(n, "FORBID_TAGS", Me({}), {
      transform: te
    }), be = Ke(n, "FORBID_ATTR", Me({}), {
      transform: te
    }), Fe = ue(n, "USE_PROFILES") ? n.USE_PROFILES && typeof n.USE_PROFILES == "object" ? Me(n.USE_PROFILES) : n.USE_PROFILES : !1, Je = n.ALLOW_ARIA_ATTR !== !1, Ze = n.ALLOW_DATA_ATTR !== !1, Qe = n.ALLOW_UNKNOWN_PROTOCOLS || !1, re = n.ALLOW_SELF_CLOSE_IN_ATTR !== !1, V = n.SAFE_FOR_TEMPLATES || !1, et = n.SAFE_FOR_XML !== !1, de = n.WHOLE_DOCUMENT || !1, Ie = n.RETURN_DOM || !1, ct = n.RETURN_DOM_FRAGMENT || !1, je = n.RETURN_TRUSTED_TYPE || !1, De = n.FORCE_BODY || !1, nt = n.SANITIZE_DOM !== !1, mt = n.SANITIZE_NAMED_PROPS || !1, Nt = n.KEEP_CONTENT !== !1, Ae = n.IN_PLACE || !1, kt = Ha(n.ALLOWED_URI_REGEXP) ? n.ALLOWED_URI_REGEXP : In, r = typeof n.NAMESPACE == "string" ? n.NAMESPACE : we, x = ue(n, "MATHML_TEXT_INTEGRATION_POINTS") && n.MATHML_TEXT_INTEGRATION_POINTS && typeof n.MATHML_TEXT_INTEGRATION_POINTS == "object" ? Me(n.MATHML_TEXT_INTEGRATION_POINTS) : H({}, I), Be = ue(n, "HTML_INTEGRATION_POINTS") && n.HTML_INTEGRATION_POINTS && typeof n.HTML_INTEGRATION_POINTS == "object" ? Me(n.HTML_INTEGRATION_POINTS) : H({}, fe);
    const l = ue(n, "CUSTOM_ELEMENT_HANDLING") && n.CUSTOM_ELEMENT_HANDLING && typeof n.CUSTOM_ELEMENT_HANDLING == "object" ? Me(n.CUSTOM_ELEMENT_HANDLING) : bt(null);
    if (Q = bt(null), ue(l, "tagNameCheck") && un(l.tagNameCheck) && (Q.tagNameCheck = l.tagNameCheck), ue(l, "attributeNameCheck") && un(l.attributeNameCheck) && (Q.attributeNameCheck = l.attributeNameCheck), ue(l, "allowCustomizedBuiltInElements") && typeof l.allowCustomizedBuiltInElements == "boolean" && (Q.allowCustomizedBuiltInElements = l.allowCustomizedBuiltInElements), ve(Q), V && (Ze = !1), ct && (Ie = !0), Fe && (q = H({}, Ln), Z = bt(null), Fe.html === !0 && (H(q, Dn), H(Z, On)), Fe.svg === !0 && (H(q, Xt), H(Z, Zt), H(Z, Ft)), Fe.svgFilters === !0 && (H(q, qt), H(Z, Zt), H(Z, Ft)), Fe.mathMl === !0 && (H(q, Jt), H(Z, Rn), H(Z, Ft))), Ce.tagCheck = null, Ce.attributeCheck = null, ue(n, "ADD_TAGS") && (typeof n.ADD_TAGS == "function" ? Ce.tagCheck = n.ADD_TAGS : qe(n.ADD_TAGS) && (q === Ct && (q = Me(q)), H(q, n.ADD_TAGS, te))), ue(n, "ADD_ATTR") && (typeof n.ADD_ATTR == "function" ? Ce.attributeCheck = n.ADD_ATTR : qe(n.ADD_ATTR) && (Z === Dt && (Z = Me(Z)), H(Z, n.ADD_ATTR, te))), ue(n, "ADD_URI_SAFE_ATTR") && qe(n.ADD_URI_SAFE_ATTR) && H(at, n.ADD_URI_SAFE_ATTR, te), ue(n, "FORBID_CONTENTS") && qe(n.FORBID_CONTENTS) && (ce === yt && (ce = Me(ce)), H(ce, n.FORBID_CONTENTS, te)), ue(n, "ADD_FORBID_CONTENTS") && qe(n.ADD_FORBID_CONTENTS) && (ce === yt && (ce = Me(ce)), H(ce, n.ADD_FORBID_CONTENTS, te)), Nt && (q["#text"] = !0), de && H(q, ["html", "head", "body"]), q.table && (H(q, ["tbody"]), delete Pe.tbody), n.TRUSTED_TYPES_POLICY) {
      if (typeof n.TRUSTED_TYPES_POLICY.createHTML != "function")
        throw rt('TRUSTED_TYPES_POLICY configuration option must provide a "createHTML" hook.');
      if (typeof n.TRUSTED_TYPES_POLICY.createScriptURL != "function")
        throw rt('TRUSTED_TYPES_POLICY configuration option must provide a "createScriptURL" hook.');
      const f = W;
      W = n.TRUSTED_TYPES_POLICY;
      try {
        b = J("");
      } catch (M) {
        throw W = f, M;
      }
    } else n.TRUSTED_TYPES_POLICY === null ? (W = void 0, b = "") : (W === void 0 && (W = ge()), W && typeof b == "string" && (b = J("")));
    pe && pe(n), ht = n;
  }, dn = H({}, [...Xt, ...qt, ...za]), fn = H({}, [...Jt, ...Ua]), ta = function(n, l, f) {
    return l.namespaceURI === we ? n === "svg" : l.namespaceURI === dt ? n === "svg" && (f === "annotation-xml" || x[f]) : !!dn[n];
  }, na = function(n, l, f) {
    return l.namespaceURI === we ? n === "math" : l.namespaceURI === ft ? n === "math" && Be[f] : !!fn[n];
  }, aa = function(n, l, f) {
    return l.namespaceURI === ft && !Be[f] || l.namespaceURI === dt && !x[f] ? !1 : !fn[n] && (Jn[n] || !dn[n]);
  }, sa = function(n) {
    let l = U(n);
    (!l || !l.tagName) && (l = {
      namespaceURI: r,
      tagName: "template"
    });
    const f = At(n.tagName), M = At(l.tagName);
    return _[n.namespaceURI] ? n.namespaceURI === ft ? ta(f, l, M) : n.namespaceURI === dt ? na(f, l, M) : n.namespaceURI === we ? aa(f, l, M) : !!(Tt === "application/xhtml+xml" && _[n.namespaceURI]) : !1;
  }, $e = function(n) {
    vt(a.removed, {
      element: n
    });
    try {
      U(n).removeChild(n);
    } catch {
      if (v(n), !U(n))
        throw rt("a node selected for removal could not be detached from its tree and cannot be safely returned; refusing to sanitize in place");
    }
  }, hn = function(n) {
    const l = B(n);
    if (l) {
      const M = [];
      _t(l, (O) => {
        vt(M, O);
      }), _t(M, (O) => {
        try {
          v(O);
        } catch {
        }
      });
    }
    const f = w(n);
    if (f)
      for (let M = f.length - 1; M >= 0; --M) {
        const O = f[M], j = O && O.name;
        if (typeof j == "string")
          try {
            n.removeAttribute(j);
          } catch {
          }
      }
  }, st = function(n, l) {
    try {
      vt(a.removed, {
        attribute: l.getAttributeNode(n),
        from: l
      });
    } catch {
      vt(a.removed, {
        attribute: null,
        from: l
      });
    }
    if (l.removeAttribute(n), n === "is")
      if (Ie || ct)
        try {
          $e(l);
        } catch {
        }
      else
        try {
          l.setAttribute(n, "");
        } catch {
        }
  }, ra = function(n) {
    const l = w(n);
    if (l)
      for (let f = l.length - 1; f >= 0; --f) {
        const M = l[f], O = M && M.name;
        if (!(typeof O != "string" || Z[te(O)]))
          try {
            n.removeAttribute(O);
          } catch {
          }
      }
  }, la = function(n) {
    const l = [n];
    for (; l.length > 0; ) {
      const f = l.pop();
      (F ? F(f) : f.nodeType) === He.element && ra(f);
      const O = B(f);
      if (O)
        for (let j = O.length - 1; j >= 0; --j)
          l.push(O[j]);
    }
  }, pn = function(n) {
    let l = null, f = null;
    if (De)
      n = "<remove></remove>" + n;
    else {
      const j = An(n, /^[\r\n\t ]+/);
      f = j && j[0];
    }
    Tt === "application/xhtml+xml" && r === we && (n = '<html xmlns="http://www.w3.org/1999/xhtml"><head></head><body>' + n + "</body></html>");
    const M = W ? J(n) : n;
    if (r === we)
      try {
        l = new S().parseFromString(M, Tt);
      } catch {
      }
    if (!l || !l.documentElement) {
      l = le.createDocument(r, "template", null);
      try {
        l.documentElement.innerHTML = N ? b : M;
      } catch {
      }
    }
    const O = l.body || l.documentElement;
    return n && f && O.insertBefore(s.createTextNode(f), O.childNodes[0] || null), r === we ? oe.call(l, de ? "html" : "body")[0] : de ? l.documentElement : O;
  }, vn = function(n) {
    return ie.call(
      n.ownerDocument || n,
      n,
      // eslint-disable-next-line no-bitwise
      m.SHOW_ELEMENT | m.SHOW_COMMENT | m.SHOW_TEXT | m.SHOW_PROCESSING_INSTRUCTION | m.SHOW_CDATA_SECTION,
      null
    );
  }, It = function(n) {
    return n = Et(n, y, " "), n = Et(n, Se, " "), n = Et(n, ye, " "), n;
  }, Vt = function(n) {
    var l;
    n.normalize();
    const f = ie.call(
      n.ownerDocument || n,
      n,
      // eslint-disable-next-line no-bitwise
      m.SHOW_TEXT | m.SHOW_COMMENT | m.SHOW_CDATA_SECTION | m.SHOW_PROCESSING_INSTRUCTION,
      null
    );
    let M = f.nextNode();
    for (; M; )
      M.data = It(M.data), M = f.nextNode();
    const O = (l = n.querySelectorAll) === null || l === void 0 ? void 0 : l.call(n, "template");
    O && _t(O, (j) => {
      pt(j.content) && Vt(j.content);
    });
  }, xt = function(n) {
    const l = $ ? $(n) : null;
    return typeof l != "string" || te(l) !== "form" ? !1 : typeof n.nodeName != "string" || typeof n.textContent != "string" || typeof n.removeChild != "function" || // Realm-safe NamedNodeMap detection: equality against the cached
    // prototype getter. Clobbered .attributes (e.g. <input name="attributes">)
    // makes the direct read diverge from the cached read; a clean form
    // (same-realm OR foreign-realm) has both reads pointing at the same
    // canonical NamedNodeMap.
    n.attributes !== w(n) || typeof n.removeAttribute != "function" || typeof n.setAttribute != "function" || typeof n.namespaceURI != "string" || typeof n.insertBefore != "function" || typeof n.hasChildNodes != "function" || // NodeType clobbering probe. Cached Node.prototype.nodeType getter
    // returns the integer 1 for any Element regardless of realm; direct
    // read on a clobbered form (e.g. <input name="nodeType">) returns
    // the named child element. Cheap addition — nodeType is read from
    // an internal slot, no serialization cost — and removes a residual
    // clobbering surface used by several mXSS / PI / comment branches
    // in _sanitizeElements that compare currentNode.nodeType directly.
    n.nodeType !== F(n) || // HTMLFormElement has [LegacyOverrideBuiltIns]: a descendant named
    // "childNodes" shadows the prototype getter. Direct reads of
    // form.childNodes from a clobbered form return the named child
    // instead of the real NodeList, so any walk that reads it directly
    // skips the form's real children. Compare the direct read to the
    // cached Node.prototype getter — when the form's named-property
    // getter intercepts the read, the two values differ and we flag
    // the form. This catches every clobbering child type (input,
    // select, etc.) regardless of whether the named child happens to
    // carry a numeric .length, which a typeof-based probe would miss
    // (e.g. HTMLSelectElement.length is a defined unsigned-long).
    n.childNodes !== B(n);
  }, pt = function(n) {
    if (!F || typeof n != "object" || n === null)
      return !1;
    try {
      return F(n) === He.documentFragment;
    } catch {
      return !1;
    }
  }, Mt = function(n) {
    if (!F || typeof n != "object" || n === null)
      return !1;
    try {
      return typeof F(n) == "number";
    } catch {
      return !1;
    }
  };
  function Ge(h, n, l) {
    h.length !== 0 && _t(h, (f) => {
      f.call(a, n, l, ht);
    });
  }
  const ia = function(n, l) {
    return !!(et && n.hasChildNodes() && !Mt(n.firstElementChild) && he(xn, n.textContent) && he(xn, n.innerHTML) || et && n.namespaceURI === we && l === "style" && Mt(n.firstElementChild) || n.nodeType === He.processingInstruction || et && n.nodeType === He.comment && he(qa, n.data));
  }, oa = function(n, l) {
    if (!Pe[l] && Nn(l) && (Q.tagNameCheck instanceof RegExp && he(Q.tagNameCheck, l) || Q.tagNameCheck instanceof Function && Q.tagNameCheck(l)))
      return !1;
    if (Nt && !ce[l]) {
      const f = U(n), M = B(n);
      if (M && f) {
        const O = M.length;
        for (let j = O - 1; j >= 0; --j) {
          const me = Ae ? M[j] : A(M[j], !0);
          f.insertBefore(me, z(n));
        }
      }
    }
    return $e(n), !0;
  }, bn = function(n) {
    if (Ge(G.beforeSanitizeElements, n, null), xt(n))
      return $e(n), !0;
    const l = te($ ? $(n) : n.nodeName);
    if (Ge(G.uponSanitizeElement, n, {
      tagName: l,
      allowedTags: q
    }), ia(n, l))
      return $e(n), !0;
    if (Pe[l] || !(Ce.tagCheck instanceof Function && Ce.tagCheck(l)) && !q[l])
      return oa(n, l);
    if ((F ? F(n) : n.nodeType) === He.element && !sa(n) || (l === "noscript" || l === "noembed" || l === "noframes") && he(Ja, n.innerHTML))
      return $e(n), !0;
    if (V && n.nodeType === He.text) {
      const M = It(n.textContent);
      n.textContent !== M && (vt(a.removed, {
        element: n.cloneNode()
      }), n.textContent = M);
    }
    return Ge(G.afterSanitizeElements, n, null), !1;
  }, gn = function(n, l, f) {
    if (be[l] || nt && (l === "id" || l === "name") && (f in s || f in ea))
      return !1;
    const M = Z[l] || Ce.attributeCheck instanceof Function && Ce.attributeCheck(l, n);
    if (!(Ze && he(lt, l))) {
      if (!(Je && he(it, l))) {
        if (M) {
          if (!at[l]) {
            if (!he(kt, Et(f, Re, ""))) {
              if (!((l === "src" || l === "xlink:href" || l === "href") && n !== "script" && wn(f, "data:") === 0 && Ot[n])) {
                if (!(Qe && !he(Ve, Et(f, Re, "")))) {
                  if (f)
                    return !1;
                }
              }
            }
          }
        } else if (
          // First condition does a very basic check if a) it's basically a valid custom element tagname AND
          // b) if the tagName passes whatever the user has configured for CUSTOM_ELEMENT_HANDLING.tagNameCheck
          // and c) if the attribute name passes whatever the user has configured for CUSTOM_ELEMENT_HANDLING.attributeNameCheck
          !(Nn(n) && (Q.tagNameCheck instanceof RegExp && he(Q.tagNameCheck, n) || Q.tagNameCheck instanceof Function && Q.tagNameCheck(n)) && (Q.attributeNameCheck instanceof RegExp && he(Q.attributeNameCheck, l) || Q.attributeNameCheck instanceof Function && Q.attributeNameCheck(l, n)) || // Alternative, second condition checks if it's an `is`-attribute, AND
          // the value passes whatever the user has configured for CUSTOM_ELEMENT_HANDLING.tagNameCheck
          l === "is" && Q.allowCustomizedBuiltInElements && (Q.tagNameCheck instanceof RegExp && he(Q.tagNameCheck, f) || Q.tagNameCheck instanceof Function && Q.tagNameCheck(f)))
        ) return !1;
      }
    }
    return !0;
  }, ca = H({}, ["annotation-xml", "color-profile", "font-face", "font-face-format", "font-face-name", "font-face-src", "font-face-uri", "missing-glyph"]), Nn = function(n) {
    return !ca[At(n)] && he(ot, n);
  }, ma = function(n, l, f, M) {
    if (W && typeof k == "object" && typeof k.getAttributeType == "function" && !f)
      switch (k.getAttributeType(n, l)) {
        case "TrustedHTML":
          return J(M);
        case "TrustedScriptURL":
          return ee(M);
      }
    return M;
  }, ua = function(n, l, f, M) {
    try {
      f ? n.setAttributeNS(f, l, M) : n.setAttribute(l, M), xt(n) ? $e(n) : Sn(a.removed);
    } catch {
      st(l, n);
    }
  }, yn = function(n) {
    Ge(G.beforeSanitizeAttributes, n, null);
    const l = n.attributes;
    if (!l || xt(n))
      return;
    const f = {
      attrName: "",
      attrValue: "",
      keepAttr: !0,
      allowedAttributes: Z,
      forceKeepAttr: void 0
    };
    let M = l.length;
    const O = te(n.nodeName);
    for (; M--; ) {
      const j = l[M], me = j.name, ae = j.namespaceURI, Le = j.value, xe = te(me), $t = Le;
      let Te = me === "value" ? $t : Oa($t);
      if (f.attrName = xe, f.attrValue = Te, f.keepAttr = !0, f.forceKeepAttr = void 0, Ge(G.uponSanitizeAttribute, n, f), Te = f.attrValue, mt && (xe === "id" || xe === "name") && wn(Te, Lt) !== 0 && (st(me, n), Te = Lt + Te), et && he(/((--!?|])>)|<\/(style|script|title|xmp|textarea|noscript|iframe|noembed|noframes)/i, Te)) {
        st(me, n);
        continue;
      }
      if (xe === "attributename" && An(Te, "href")) {
        st(me, n);
        continue;
      }
      if (!f.forceKeepAttr) {
        if (!f.keepAttr) {
          st(me, n);
          continue;
        }
        if (!re && he(Za, Te)) {
          st(me, n);
          continue;
        }
        if (V && (Te = It(Te)), !gn(O, xe, Te)) {
          st(me, n);
          continue;
        }
        Te = ma(O, xe, ae, Te), Te !== $t && ua(n, me, ae, Te);
      }
    }
    Ge(G.afterSanitizeAttributes, n, null);
  }, Pt = function(n) {
    let l = null;
    const f = vn(n);
    for (Ge(G.beforeSanitizeShadowDOM, n, null); l = f.nextNode(); )
      if (Ge(G.uponSanitizeShadowNode, l, null), bn(l), yn(l), pt(l.content) && Pt(l.content), (F ? F(l) : l.nodeType) === He.element) {
        const O = D(l);
        pt(O) && (Yt(O), Pt(O));
      }
    Ge(G.afterSanitizeShadowDOM, n, null);
  }, Yt = function(n) {
    const l = [{
      node: n,
      shadow: null
    }];
    for (; l.length > 0; ) {
      const f = l.pop();
      if (f.shadow) {
        Pt(f.shadow);
        continue;
      }
      const M = f.node, j = (F ? F(M) : M.nodeType) === He.element, me = B(M);
      if (me)
        for (let ae = me.length - 1; ae >= 0; --ae)
          l.push({
            node: me[ae],
            shadow: null
          });
      if (j) {
        const ae = $ ? $(M) : null;
        if (typeof ae == "string" && te(ae) === "template") {
          const Le = M.content;
          pt(Le) && l.push({
            node: Le,
            shadow: null
          });
        }
      }
      if (j) {
        const ae = D(M);
        pt(ae) && l.push({
          node: null,
          shadow: ae
        }, {
          node: ae,
          shadow: null
        });
      }
    }
  };
  return a.sanitize = function(h) {
    let n = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {}, l = null, f = null, M = null, O = null;
    if (N = !h, N && (h = "<!-->"), typeof h != "string" && !Mt(h) && (h = Fa(h), typeof h != "string"))
      throw rt("dirty is not a string, aborting");
    if (!a.isSupported)
      return h;
    Ye ? (q = gt, Z = tt) : Wt(n), (G.uponSanitizeElement.length > 0 || G.uponSanitizeAttribute.length > 0) && (q = Me(q)), G.uponSanitizeAttribute.length > 0 && (Z = Me(Z)), a.removed = [];
    const j = Ae && typeof h != "string" && Mt(h);
    if (j) {
      const Le = $ ? $(h) : h.nodeName;
      if (typeof Le == "string") {
        const xe = te(Le);
        if (!q[xe] || Pe[xe])
          throw rt("root node is forbidden and cannot be sanitized in-place");
      }
      if (xt(h))
        throw rt("root node is clobbered and cannot be sanitized in-place");
      try {
        Yt(h);
      } catch (xe) {
        throw hn(h), xe;
      }
    } else if (Mt(h))
      l = pn("<!---->"), f = l.ownerDocument.importNode(h, !0), f.nodeType === He.element && f.nodeName === "BODY" || f.nodeName === "HTML" ? l = f : l.appendChild(f), Yt(f);
    else {
      if (!Ie && !V && !de && // eslint-disable-next-line unicorn/prefer-includes
      h.indexOf("<") === -1)
        return W && je ? J(h) : h;
      if (l = pn(h), !l)
        return Ie ? null : je ? b : "";
    }
    l && De && $e(l.firstChild);
    const me = vn(j ? h : l);
    try {
      for (; M = me.nextNode(); )
        bn(M), yn(M), pt(M.content) && Pt(M.content);
    } catch (Le) {
      throw j && hn(h), Le;
    }
    if (j)
      return _t(a.removed, (Le) => {
        Le.element && la(Le.element);
      }), V && Vt(h), h;
    if (Ie) {
      if (V && Vt(l), ct)
        for (O = T.call(l.ownerDocument); l.firstChild; )
          O.appendChild(l.firstChild);
      else
        O = l;
      return (Z.shadowroot || Z.shadowrootmode) && (O = ne.call(i, O, !0)), O;
    }
    let ae = de ? l.outerHTML : l.innerHTML;
    return de && q["!doctype"] && l.ownerDocument && l.ownerDocument.doctype && l.ownerDocument.doctype.name && he(Ka, l.ownerDocument.doctype.name) && (ae = "<!DOCTYPE " + l.ownerDocument.doctype.name + `>
` + ae), V && (ae = It(ae)), W && je ? J(ae) : ae;
  }, a.setConfig = function() {
    let h = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    Wt(h), Ye = !0, gt = q, tt = Z;
  }, a.clearConfig = function() {
    ht = null, Ye = !1, gt = null, tt = null, W = L, b = "";
  }, a.isValidAttribute = function(h, n, l) {
    ht || Wt({});
    const f = te(h), M = te(n);
    return gn(f, M, l);
  }, a.addHook = function(h, n) {
    typeof n == "function" && ue(G, h) && vt(G[h], n);
  }, a.removeHook = function(h, n) {
    if (ue(G, h)) {
      if (n !== void 0) {
        const l = Da(G[h], n);
        return l === -1 ? void 0 : La(G[h], l, 1)[0];
      }
      return Sn(G[h]);
    }
  }, a.removeHooks = function(h) {
    ue(G, h) && (G[h] = []);
  }, a.removeAllHooks = function() {
    G = Pn();
  }, a;
}
var Bt = $n();
const ts = [
  "onerror",
  "onload",
  "onclick",
  "onmouseover",
  "onfocus",
  "onblur",
  "onchange",
  "onsubmit",
  "onkeydown",
  "onkeyup",
  "onkeypress",
  "onanimationstart"
], ns = {
  USE_PROFILES: { html: !0 },
  FORBID_TAGS: ["script", "iframe", "object", "embed", "form", "input", "button", "style", "link", "meta", "base"],
  FORBID_ATTR: ts,
  ALLOW_DATA_ATTR: !1
};
let Fn = !1;
function as() {
  Fn || typeof Bt.addHook != "function" || (Bt.addHook("afterSanitizeAttributes", (e) => {
    e.tagName === "A" && e.getAttribute("href") && (e.setAttribute("target", "_blank"), e.setAttribute("rel", "noopener noreferrer"));
  }), Fn = !0);
}
function Kn(e) {
  return as(), Bt.sanitize(e ?? "", ns);
}
function ss(e) {
  return Bt.sanitize(e ?? "", { ALLOWED_TAGS: [], ALLOWED_ATTR: [] });
}
const Hn = (e, a) => Array.isArray(e == null ? void 0 : e.flags) && e.flags.includes(a);
function rs({
  thread: e,
  fullById: a = {},
  onNeedBody: s,
  loading: i,
  error: u,
  onToggleStar: o,
  onArchive: g,
  onDelete: d,
  onReply: m,
  onReplyAll: E,
  onForward: S,
  onBack: k,
  canArchive: R = !0
}) {
  const A = (e == null ? void 0 : e.messages) ?? [], v = A.length ? A[A.length - 1].id : null, [z, B] = C(() => /* @__PURE__ */ new Set());
  _e(() => {
    if (v == null) {
      B(/* @__PURE__ */ new Set());
      return;
    }
    B(/* @__PURE__ */ new Set([v])), s == null || s(v);
  }, [e == null ? void 0 : e.id, v]);
  const U = (w) => {
    B((F) => {
      const $ = new Set(F);
      return $.has(w) ? $.delete(w) : ($.add(w), s == null || s(w)), $;
    });
  };
  if (i && !e)
    return /* @__PURE__ */ t("section", { className: "vm-read", children: /* @__PURE__ */ c("div", { className: "vm-read-inner", children: [
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "60%", height: 16 } }),
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "90%" } }),
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "82%" } }),
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "88%" } })
    ] }) });
  if (u) return /* @__PURE__ */ t("section", { className: "vm-read", children: /* @__PURE__ */ t("div", { className: "vm-empty vm-state", role: "alert", children: u }) });
  if (!e)
    return /* @__PURE__ */ t("section", { className: "vm-read", children: /* @__PURE__ */ c("div", { className: "vm-empty vm-state", children: [
      /* @__PURE__ */ t(p, { name: "mailopen", className: "vm-empty-icon" }),
      /* @__PURE__ */ t("p", { children: "Select a conversation to read" })
    ] }) });
  const D = e.starred;
  return /* @__PURE__ */ c("section", { className: "vm-read", "aria-label": "Conversation", children: [
    /* @__PURE__ */ c("div", { className: "vm-read-actions", children: [
      /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-back", onClick: k, "aria-label": "Back to list", children: /* @__PURE__ */ t(p, { name: "back" }) }),
      R && /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Archive", title: "Archive", onClick: g, children: /* @__PURE__ */ t(p, { name: "archive" }) }),
      /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-danger", "aria-label": "Delete", title: "Delete", onClick: d, children: /* @__PURE__ */ t(p, { name: "trash" }) }),
      /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-iconbtn" + (D ? " vm-on" : ""),
          "aria-label": D ? "Unstar" : "Star",
          "aria-pressed": D,
          onClick: () => o == null ? void 0 : o(!D),
          children: /* @__PURE__ */ t(p, { name: "star", fill: D ? "currentColor" : "none" })
        }
      ),
      /* @__PURE__ */ t("span", { className: "vm-spacer" }),
      /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-iconbtn",
          "aria-label": "Reply",
          title: "Reply",
          onClick: () => m == null ? void 0 : m(e.latest),
          children: /* @__PURE__ */ t(p, { name: "reply" })
        }
      ),
      /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-iconbtn",
          "aria-label": "Reply all",
          title: "Reply all",
          onClick: () => E == null ? void 0 : E(e.latest),
          children: /* @__PURE__ */ t(p, { name: "replyall" })
        }
      ),
      /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-iconbtn",
          "aria-label": "Forward",
          title: "Forward",
          onClick: () => S == null ? void 0 : S(e.latest),
          children: /* @__PURE__ */ t(p, { name: "forward" })
        }
      )
    ] }),
    /* @__PURE__ */ c("div", { className: "vm-read-inner", children: [
      /* @__PURE__ */ c("div", { className: "vm-read-headline", children: [
        /* @__PURE__ */ t("h1", { className: "vm-read-subject", children: e.subject || "(no subject)" }),
        e.count > 1 && /* @__PURE__ */ t("span", { className: "vm-read-count", children: e.count })
      ] }),
      /* @__PURE__ */ c("div", { className: "vm-mobile-actions", role: "toolbar", "aria-label": "Conversation actions", children: [
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Reply", onClick: () => m == null ? void 0 : m(e.latest), children: /* @__PURE__ */ t(p, { name: "reply" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Reply all", onClick: () => E == null ? void 0 : E(e.latest), children: /* @__PURE__ */ t(p, { name: "replyall" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Forward", onClick: () => S == null ? void 0 : S(e.latest), children: /* @__PURE__ */ t(p, { name: "forward" }) }),
        R && /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Archive", onClick: g, children: /* @__PURE__ */ t(p, { name: "archive" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-danger", "aria-label": "Delete", onClick: d, children: /* @__PURE__ */ t(p, { name: "trash" }) })
      ] }),
      /* @__PURE__ */ t("div", { className: "vm-thread", children: A.map((w, F) => {
        const $ = z.has(w.id), W = F === A.length - 1;
        return /* @__PURE__ */ t(
          ls,
          {
            summary: w,
            full: a[w.id],
            open: $,
            last: W,
            onToggle: () => U(w.id),
            onReply: () => m == null ? void 0 : m(a[w.id] || w),
            onReplyAll: () => E == null ? void 0 : E(a[w.id] || w),
            onForward: () => S == null ? void 0 : S(a[w.id] || w)
          },
          w.id
        );
      }) }, e.id)
    ] })
  ] });
}
function ls({ summary: e, full: a, open: s, last: i, onToggle: u, onReply: o, onReplyAll: g, onForward: d }) {
  const m = a || e, E = ke(() => m != null && m.html ? Kn(m.html) : "", [m == null ? void 0 : m.html]), S = Hn(m, We), k = !Hn(e, Ue), R = m.fromName || m.from || "(unknown)", A = a || m.html || m.body;
  return s ? /* @__PURE__ */ c("article", { className: "vm-msg", children: [
    /* @__PURE__ */ c("header", { className: "vm-msg-head", onClick: i ? void 0 : u, role: i ? void 0 : "button", children: [
      /* @__PURE__ */ t("span", { className: "vm-avatar", style: Ut(m.from || R), "aria-hidden": "true", children: rn(m.fromName, m.from) }),
      /* @__PURE__ */ c("span", { className: "vm-msg-meta", children: [
        /* @__PURE__ */ c("span", { className: "vm-msg-fromline", children: [
          /* @__PURE__ */ t("span", { className: "vm-msg-from", children: R }),
          /* @__PURE__ */ c("span", { className: "vm-msg-addr", children: [
            "<",
            m.from,
            ">"
          ] })
        ] }),
        m.to && /* @__PURE__ */ c("span", { className: "vm-msg-to", children: [
          "to ",
          m.to,
          m.cc ? `, ${m.cc}` : ""
        ] })
      ] }),
      /* @__PURE__ */ t("time", { className: "vm-msg-date", children: jt(m.date) })
    ] }),
    A ? E ? /* @__PURE__ */ t("div", { className: "vm-msg-body", dangerouslySetInnerHTML: { __html: E } }) : /* @__PURE__ */ t("div", { className: "vm-msg-body vm-plain", children: m.body || "" }) : /* @__PURE__ */ c("div", { className: "vm-msg-body", children: [
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "90%" } }),
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "80%" } })
    ] }),
    Array.isArray(m.attachments) && m.attachments.length > 0 && /* @__PURE__ */ t("div", { className: "vm-attach-list", children: m.attachments.map((v, z) => /* @__PURE__ */ c("span", { className: "vm-attach-chip", title: "Download not yet available over /v1", children: [
      /* @__PURE__ */ t(p, { name: "attach" }),
      " ",
      /* @__PURE__ */ t("span", { className: "vm-attach-name", children: v.filename || v.Filename || "attachment" }),
      v.size || v.Size ? /* @__PURE__ */ t("span", { className: "vm-attach-size", children: is(v.size || v.Size) }) : null
    ] }, v.id || z)) }),
    /* @__PURE__ */ c("footer", { className: "vm-msg-foot", children: [
      /* @__PURE__ */ c("button", { type: "button", className: "vm-btn vm-btn-ghost", onClick: o, children: [
        /* @__PURE__ */ t(p, { name: "reply" }),
        " Reply"
      ] }),
      /* @__PURE__ */ c("button", { type: "button", className: "vm-btn vm-btn-ghost", onClick: g, children: [
        /* @__PURE__ */ t(p, { name: "replyall" }),
        " Reply all"
      ] }),
      /* @__PURE__ */ c("button", { type: "button", className: "vm-btn vm-btn-ghost", onClick: d, children: [
        /* @__PURE__ */ t(p, { name: "forward" }),
        " Forward"
      ] })
    ] })
  ] }) : /* @__PURE__ */ t("article", { className: "vm-msg vm-collapsed" + (k ? " vm-unread" : ""), children: /* @__PURE__ */ c("button", { type: "button", className: "vm-msg-head", onClick: u, "aria-expanded": "false", children: [
    /* @__PURE__ */ t("span", { className: "vm-avatar vm-avatar-sm", style: Ut(m.from || R), "aria-hidden": "true", children: rn(m.fromName, m.from) }),
    /* @__PURE__ */ c("span", { className: "vm-msg-meta", children: [
      /* @__PURE__ */ t("span", { className: "vm-msg-from", children: R }),
      /* @__PURE__ */ t("span", { className: "vm-msg-collapsed-snip", children: e.preview || m.preview })
    ] }),
    S && /* @__PURE__ */ t(p, { name: "star", className: "vm-msg-star", fill: "currentColor" }),
    /* @__PURE__ */ t("time", { className: "vm-msg-date", children: jt(m.date) })
  ] }) });
}
function is(e) {
  return e ? e < 1024 ? e + " B" : e < 1024 * 1024 ? (e / 1024).toFixed(0) + " KB" : (e / 1024 / 1024).toFixed(1) + " MB" : "";
}
function os({
  initial: e = {},
  onSend: a,
  onClose: s,
  onSaveDraft: i,
  onContactSearch: u,
  signature: o = ""
}) {
  const [g, d] = C(e.to ?? ""), [m, E] = C(e.cc ?? ""), [S, k] = C(e.bcc ?? ""), [R, A] = C(e.subject ?? ""), [v, z] = C(!!(e.cc || e.bcc)), [B, U] = C(!1), [D, w] = C(!1), [F, $] = C(!1), [W, b] = C(""), [L, K] = C(""), X = Oe(null), Ee = Oe(null), J = Oe(null), ee = Oe(null), ge = Oe(!1);
  _e(() => {
    if (!X.current) return;
    const y = o ? `<br><br><div class="vm-sig">${zn(o)}</div>` : "";
    X.current.innerHTML = (e.html ?? (e.body ? zn(e.body) : "")) + y;
  }, []), _e(() => {
    var y;
    (y = Ee.current) == null || y.focus();
  }, []);
  function Ne() {
    var Se;
    const y = ((Se = X.current) == null ? void 0 : Se.innerHTML) ?? "";
    return {
      to: g,
      cc: m,
      bcc: S,
      subject: R,
      html: y,
      text: ss(y),
      inReplyTo: e.inReplyTo,
      references: e.references
    };
  }
  const le = () => {
    ge.current = !0, i && (clearTimeout(ee.current), ee.current = setTimeout(async () => {
      const y = Ne();
      if (!(!y.to && !y.subject && !y.text.trim()))
        try {
          await i(y), b((/* @__PURE__ */ new Date()).toLocaleTimeString(void 0, { hour: "numeric", minute: "2-digit" })), ge.current = !1;
        } catch {
        }
    }, 1200));
  };
  _e(() => {
    le();
  }, [g, m, S, R]), _e(() => () => clearTimeout(ee.current), []);
  async function ie() {
    if (a) {
      if (K(""), !g.trim()) {
        K("Add at least one recipient");
        return;
      }
      $(!0);
      try {
        await a(Ne()), s == null || s();
      } catch (y) {
        K((y == null ? void 0 : y.message) || "Failed to send"), $(!1);
      }
    }
  }
  function T() {
    (W || ge.current) && !window.confirm("Discard this draft?") || s == null || s();
  }
  function oe(y) {
    var Ve, Re;
    if (y.key === "Escape") {
      (Re = (Ve = y.nativeEvent) == null ? void 0 : Ve.stopImmediatePropagation) == null || Re.call(Ve), y.stopPropagation(), s == null || s();
      return;
    }
    if (y.key !== "Tab" || !D || !J.current) return;
    const Se = J.current.querySelectorAll(
      'button:not([disabled]), [href], input:not([disabled]), textarea, [contenteditable="true"], [tabindex]:not([tabindex="-1"])'
    ), ye = Array.from(Se).filter((ot) => ot.offsetParent !== null || ot === document.activeElement);
    if (!ye.length) return;
    const lt = ye[0], it = ye[ye.length - 1];
    y.shiftKey && document.activeElement === lt ? (y.preventDefault(), it.focus()) : !y.shiftKey && document.activeElement === it && (y.preventDefault(), lt.focus());
  }
  const ne = (y, Se) => {
    var ye;
    (ye = X.current) == null || ye.focus();
    try {
      document.execCommand(y, !1, Se);
    } catch {
    }
    le();
  }, G = () => {
    const y = window.prompt("Link URL");
    y && ne("createLink", y);
  };
  return B ? /* @__PURE__ */ t("div", { className: "vm-compose-dock vm-min", children: /* @__PURE__ */ c("div", { className: "vm-compose-bar", children: [
    /* @__PURE__ */ t("button", { type: "button", className: "vm-compose-bar-title", onClick: () => U(!1), children: /* @__PURE__ */ t("span", { className: "vm-compose-title", children: R || "New message" }) }),
    /* @__PURE__ */ c("span", { className: "vm-compose-bar-actions", children: [
      /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Restore", title: "Restore", onClick: () => U(!1), children: /* @__PURE__ */ t(p, { name: "chevup" }) }),
      /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Close", title: "Close", onClick: () => s == null ? void 0 : s(), children: /* @__PURE__ */ t(p, { name: "close" }) })
    ] })
  ] }) }) : /* @__PURE__ */ t("div", { ref: J, className: "vm-compose-dock" + (D ? " vm-max" : ""), role: "dialog", "aria-modal": D ? "true" : void 0, "aria-label": "Compose message", onKeyDown: oe, children: /* @__PURE__ */ c("div", { className: "vm-compose", children: [
    /* @__PURE__ */ c("header", { className: "vm-compose-head", children: [
      /* @__PURE__ */ t("span", { className: "vm-compose-title", children: R || "New message" }),
      /* @__PURE__ */ c("span", { className: "vm-compose-bar-actions", children: [
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Minimise", onClick: () => U(!0), children: /* @__PURE__ */ t(p, { name: "minus" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": D ? "Restore" : "Full screen", onClick: () => w((y) => !y), children: /* @__PURE__ */ t(p, { name: D ? "collapse" : "expand" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Close", onClick: s, children: /* @__PURE__ */ t(p, { name: "close" }) })
      ] })
    ] }),
    /* @__PURE__ */ c("div", { className: "vm-compose-body", children: [
      /* @__PURE__ */ t(Qt, { label: "To", value: g, setValue: d, inputRef: Ee, onContactSearch: u, onChange: le, children: /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-cc-toggle",
          "aria-expanded": v,
          onClick: () => z((y) => !y),
          children: v ? "Hide" : "Cc Bcc"
        }
      ) }),
      v && /* @__PURE__ */ c(cn, { children: [
        /* @__PURE__ */ t(Qt, { label: "Cc", value: m, setValue: E, onContactSearch: u, onChange: le }),
        /* @__PURE__ */ t(Qt, { label: "Bcc", value: S, setValue: k, onContactSearch: u, onChange: le })
      ] }),
      /* @__PURE__ */ t("label", { className: "vm-crow", children: /* @__PURE__ */ t(
        "input",
        {
          className: "vm-subject",
          type: "text",
          value: R,
          placeholder: "Subject",
          onChange: (y) => A(y.target.value),
          "aria-label": "Subject"
        }
      ) }),
      /* @__PURE__ */ t(
        "div",
        {
          ref: X,
          className: "vm-ctext",
          contentEditable: !0,
          suppressContentEditableWarning: !0,
          role: "textbox",
          "aria-multiline": "true",
          "aria-label": "Message body",
          "data-placeholder": "Write your message…",
          onInput: le
        }
      )
    ] }),
    L && /* @__PURE__ */ t("div", { className: "vm-error", role: "alert", children: L }),
    /* @__PURE__ */ c("footer", { className: "vm-compose-foot", children: [
      /* @__PURE__ */ c("button", { type: "button", className: "vm-btn vm-btn-primary", onClick: ie, disabled: F || !a, children: [
        /* @__PURE__ */ t(p, { name: "send" }),
        " ",
        F ? "Sending…" : "Send"
      ] }),
      /* @__PURE__ */ c("div", { className: "vm-fmt", role: "toolbar", "aria-label": "Formatting", children: [
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Bold", title: "Bold", onMouseDown: (y) => y.preventDefault(), onClick: () => ne("bold"), children: /* @__PURE__ */ t(p, { name: "bold" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Italic", title: "Italic", onMouseDown: (y) => y.preventDefault(), onClick: () => ne("italic"), children: /* @__PURE__ */ t(p, { name: "italic" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Bulleted list", title: "Bulleted list", onMouseDown: (y) => y.preventDefault(), onClick: () => ne("insertUnorderedList"), children: /* @__PURE__ */ t(p, { name: "ul" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Numbered list", title: "Numbered list", onMouseDown: (y) => y.preventDefault(), onClick: () => ne("insertOrderedList"), children: /* @__PURE__ */ t(p, { name: "ol" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm", "aria-label": "Insert link", title: "Insert link", onMouseDown: (y) => y.preventDefault(), onClick: G, children: /* @__PURE__ */ t(p, { name: "link" }) })
      ] }),
      /* @__PURE__ */ t("span", { className: "vm-spacer" }),
      /* @__PURE__ */ t(
        "button",
        {
          type: "button",
          className: "vm-iconbtn vm-sm vm-attach-btn",
          "aria-label": "Attach files (coming soon)",
          title: "Attachments are not yet available over /v1",
          disabled: !0,
          children: /* @__PURE__ */ t(p, { name: "attach" })
        }
      ),
      W && /* @__PURE__ */ c("span", { className: "vm-note", children: [
        "Saved ",
        W
      ] }),
      /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn vm-sm vm-danger", "aria-label": "Discard draft", title: "Discard", onClick: T, children: /* @__PURE__ */ t(p, { name: "trash" }) })
    ] })
  ] }) });
}
function Qt({ label: e, value: a, setValue: s, inputRef: i, onContactSearch: u, onChange: o, children: g }) {
  const [d, m] = C(!1), [E, S] = C([]), [k, R] = C(0), A = Oe(null), v = da(), z = (D) => {
    const w = a.split(",");
    w[w.length - 1] = " " + D, s(w.join(",").replace(/^\s+/, "") + ", "), m(!1), S([]), o == null || o();
  };
  function B(D) {
    const w = D.target.value;
    s(w), o == null || o();
    const F = w.split(",").pop().trim();
    if (clearTimeout(A.current), !u || F.length < 1) {
      m(!1), S([]);
      return;
    }
    A.current = setTimeout(async () => {
      try {
        const $ = await u(F);
        S(($ || []).slice(0, 6)), R(0), m(($ || []).length > 0);
      } catch {
        S([]), m(!1);
      }
    }, 160);
  }
  function U(D) {
    d && (D.key === "ArrowDown" ? (D.preventDefault(), R((w) => Math.min(w + 1, E.length - 1))) : D.key === "ArrowUp" ? (D.preventDefault(), R((w) => Math.max(w - 1, 0))) : D.key === "Enter" && E[k] ? (D.preventDefault(), z(E[k].email)) : D.key === "Escape" && m(!1));
  }
  return /* @__PURE__ */ c("div", { className: "vm-crow vm-recip", children: [
    /* @__PURE__ */ t("span", { className: "vm-crow-label", children: e }),
    /* @__PURE__ */ c("div", { className: "vm-recip-wrap", children: [
      /* @__PURE__ */ t(
        "input",
        {
          ref: i,
          type: "text",
          value: a,
          onChange: B,
          onKeyDown: U,
          onBlur: () => setTimeout(() => m(!1), 120),
          "aria-label": e,
          "aria-autocomplete": "list",
          "aria-expanded": d,
          "aria-controls": v,
          autoComplete: "off"
        }
      ),
      d && /* @__PURE__ */ t("ul", { className: "vm-autocomplete", id: v, role: "listbox", children: E.map((D, w) => /* @__PURE__ */ c(
        "li",
        {
          role: "option",
          "aria-selected": w === k,
          className: "vm-ac-item" + (w === k ? " vm-on" : ""),
          onMouseDown: (F) => {
            F.preventDefault(), z(D.email);
          },
          onMouseEnter: () => R(w),
          children: [
            /* @__PURE__ */ t("span", { className: "vm-ac-name", children: D.name || D.email }),
            D.name && /* @__PURE__ */ t("span", { className: "vm-ac-email", children: D.email })
          ]
        },
        D.email + w
      )) })
    ] }),
    g
  ] });
}
function zn(e = "") {
  return e.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/\n/g, "<br>");
}
function en({ value: e, onChange: a, options: s, ariaLabel: i }) {
  return /* @__PURE__ */ t("div", { className: "vm-segctl", role: "radiogroup", "aria-label": i, children: s.map((u) => /* @__PURE__ */ c(
    "button",
    {
      type: "button",
      role: "radio",
      "aria-checked": e === u.value,
      className: "vm-seg" + (e === u.value ? " vm-on" : ""),
      onClick: () => a(u.value),
      children: [
        u.icon && /* @__PURE__ */ t(p, { name: u.icon }),
        " ",
        u.label
      ]
    },
    u.value
  )) });
}
function Ht({ title: e, children: a }) {
  return /* @__PURE__ */ c("section", { className: "vm-set-section", children: [
    /* @__PURE__ */ t("h3", { className: "vm-set-section-title", children: e }),
    /* @__PURE__ */ t("div", { className: "vm-set-group", children: a })
  ] });
}
function cs({ settings: e, onChange: a, onClose: s, extra: i }) {
  const u = (o) => a == null ? void 0 : a(o);
  return /* @__PURE__ */ c("div", { className: "vm-settings", children: [
    /* @__PURE__ */ c("header", { className: "vm-panel-head", children: [
      /* @__PURE__ */ c("h2", { children: [
        /* @__PURE__ */ t(p, { name: "settings", className: "vm-icon" }),
        " Settings"
      ] }),
      /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Close settings", onClick: s, children: /* @__PURE__ */ t(p, { name: "close" }) })
    ] }),
    /* @__PURE__ */ c("div", { className: "vm-panel-body", children: [
      i,
      /* @__PURE__ */ c(Ht, { title: "Appearance", children: [
        /* @__PURE__ */ c("div", { className: "vm-set-row", children: [
          /* @__PURE__ */ t("label", { className: "vm-set-label", children: "Theme" }),
          /* @__PURE__ */ t(
            en,
            {
              value: e.theme,
              onChange: (o) => u({ theme: o }),
              ariaLabel: "Theme",
              options: [
                { value: "system", label: "Auto", icon: "contrast" },
                { value: "dark", label: "Dark", icon: "moon" },
                { value: "light", label: "Light", icon: "sun" }
              ]
            }
          ),
          /* @__PURE__ */ t("p", { className: "vm-set-desc", children: "Auto follows your operating system’s light or dark setting." })
        ] }),
        /* @__PURE__ */ c("div", { className: "vm-set-row", children: [
          /* @__PURE__ */ t("label", { className: "vm-set-label", children: "Density" }),
          /* @__PURE__ */ t(
            en,
            {
              value: e.density,
              onChange: (o) => u({ density: o }),
              ariaLabel: "Density",
              options: [{ value: "comfortable", label: "Comfortable" }, { value: "compact", label: "Compact" }]
            }
          )
        ] })
      ] }),
      /* @__PURE__ */ c(Ht, { title: "Layout", children: [
        /* @__PURE__ */ c("div", { className: "vm-set-row", children: [
          /* @__PURE__ */ t("label", { className: "vm-set-label", children: "Reading pane" }),
          /* @__PURE__ */ t(
            en,
            {
              value: e.readingPane,
              onChange: (o) => u({ readingPane: o }),
              ariaLabel: "Reading pane",
              options: [{ value: "right", label: "Right" }, { value: "bottom", label: "Bottom" }, { value: "off", label: "No split" }]
            }
          ),
          /* @__PURE__ */ t("p", { className: "vm-set-desc", children: "Where an opened conversation appears next to the message list." })
        ] }),
        /* @__PURE__ */ c("div", { className: "vm-set-row vm-set-inline", children: [
          /* @__PURE__ */ c("span", { className: "vm-set-line", children: [
            /* @__PURE__ */ t("label", { className: "vm-set-label", htmlFor: "vm-set-threaded", children: "Conversation view" }),
            /* @__PURE__ */ t("span", { className: "vm-set-desc", children: "Group replies into a single thread." })
          ] }),
          /* @__PURE__ */ t(Un, { id: "vm-set-threaded", checked: e.threaded, onChange: (o) => u({ threaded: o }) })
        ] })
      ] }),
      /* @__PURE__ */ t(Ht, { title: "Reading & shortcuts", children: /* @__PURE__ */ c("div", { className: "vm-set-row vm-set-inline", children: [
        /* @__PURE__ */ c("span", { className: "vm-set-line", children: [
          /* @__PURE__ */ t("label", { className: "vm-set-label", htmlFor: "vm-set-shortcuts", children: "Keyboard shortcuts" }),
          /* @__PURE__ */ t("span", { className: "vm-set-desc", children: "Gmail-style keys: j/k, e, #, r, c… Press ? for the full list." })
        ] }),
        /* @__PURE__ */ t(Un, { id: "vm-set-shortcuts", checked: e.shortcuts, onChange: (o) => u({ shortcuts: o }) })
      ] }) }),
      /* @__PURE__ */ t(Ht, { title: "Composing", children: /* @__PURE__ */ c("div", { className: "vm-set-row", children: [
        /* @__PURE__ */ t("label", { className: "vm-set-label", htmlFor: "vm-set-sig", children: "Signature" }),
        /* @__PURE__ */ t(
          "textarea",
          {
            id: "vm-set-sig",
            className: "vm-set-textarea",
            value: e.signature,
            placeholder: "Appended to new messages…",
            rows: 4,
            onChange: (o) => u({ signature: o.target.value })
          }
        ),
        /* @__PURE__ */ t("p", { className: "vm-set-desc", children: "Added to the bottom of new messages and replies." })
      ] }) })
    ] })
  ] });
}
function Un({ id: e, checked: a, onChange: s }) {
  return /* @__PURE__ */ t(
    "button",
    {
      id: e,
      type: "button",
      role: "switch",
      "aria-checked": a,
      className: "vm-toggle" + (a ? " vm-on" : ""),
      onClick: () => s(!a),
      children: /* @__PURE__ */ t("span", { className: "vm-toggle-knob" })
    }
  );
}
const ms = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"], us = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December"
], ds = (e, a) => e.getFullYear() === a.getFullYear() && e.getMonth() === a.getMonth() && e.getDate() === a.getDate();
function jn(e) {
  const a = new Date(e.getFullYear(), e.getMonth(), 1), s = (a.getDay() + 6) % 7, i = new Date(a);
  return i.setDate(1 - s), Array.from({ length: 42 }, (u, o) => {
    const g = new Date(i);
    return g.setDate(i.getDate() + o), g;
  });
}
function Bn(e) {
  const a = new Date(e);
  return Number.isNaN(a.getTime()) ? "" : a.toLocaleTimeString(void 0, { hour: "2-digit", minute: "2-digit" });
}
function fs({ baseUrl: e = "/v1", client: a, onAuthError: s, defaultView: i = "month" }) {
  const u = ke(() => a ?? mn({ baseUrl: e }), [a, e]), [o, g] = C(() => /* @__PURE__ */ new Date()), [d, m] = C(i), [E, S] = C([]), [k, R] = C(!0), [A, v] = C(""), z = Y((b) => ((b == null ? void 0 : b.status) === 401 && (s == null || s(b)), (b == null ? void 0 : b.message) || "Could not load calendar"), [s]), [B, U] = ke(() => {
    const b = jn(o), L = b[0], K = new Date(b[41]);
    return K.setDate(K.getDate() + 1), [L, K];
  }, [o]);
  _e(() => {
    let b = !0;
    return R(!0), v(""), u.listEvents({ start: B, end: U }).then((L) => {
      b && S(L);
    }).catch((L) => {
      b && (v(z(L)), S([]));
    }).finally(() => {
      b && R(!1);
    }), () => {
      b = !1;
    };
  }, [u, B, U, z]);
  const D = ke(() => {
    const b = /* @__PURE__ */ new Map();
    for (const L of E) {
      const K = new Date(L.start);
      if (Number.isNaN(K.getTime())) continue;
      const X = K.toDateString();
      b.has(X) || b.set(X, []), b.get(X).push(L);
    }
    return b;
  }, [E]), w = ke(
    () => [...E].sort((b, L) => new Date(b.start) - new Date(L.start)),
    [E]
  ), F = (b) => g((L) => new Date(L.getFullYear(), L.getMonth() + b, 1)), $ = /* @__PURE__ */ new Date(), W = jn(o);
  return /* @__PURE__ */ c("div", { className: "vm-cal", children: [
    /* @__PURE__ */ c("header", { className: "vm-cal-head", children: [
      /* @__PURE__ */ c("div", { className: "vm-cal-nav", children: [
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Previous month", onClick: () => F(-1), children: /* @__PURE__ */ t(p, { name: "prev" }) }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-btn vm-btn-ghost vm-cal-today", onClick: () => g(/* @__PURE__ */ new Date()), children: "Today" }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Next month", onClick: () => F(1), children: /* @__PURE__ */ t(p, { name: "next" }) }),
        /* @__PURE__ */ c("h2", { className: "vm-cal-title", children: [
          us[o.getMonth()],
          " ",
          o.getFullYear()
        ] })
      ] }),
      /* @__PURE__ */ c("div", { className: "vm-cal-views", role: "tablist", "aria-label": "Calendar view", children: [
        /* @__PURE__ */ c(
          "button",
          {
            type: "button",
            role: "tab",
            "aria-selected": d === "month",
            className: "vm-seg" + (d === "month" ? " vm-on" : ""),
            onClick: () => m("month"),
            children: [
              /* @__PURE__ */ t(p, { name: "grid" }),
              " Month"
            ]
          }
        ),
        /* @__PURE__ */ c(
          "button",
          {
            type: "button",
            role: "tab",
            "aria-selected": d === "agenda",
            className: "vm-seg" + (d === "agenda" ? " vm-on" : ""),
            onClick: () => m("agenda"),
            children: [
              /* @__PURE__ */ t(p, { name: "list" }),
              " Agenda"
            ]
          }
        )
      ] })
    ] }),
    A && /* @__PURE__ */ t("div", { className: "vm-error", role: "alert", children: A }),
    d === "month" ? /* @__PURE__ */ c("div", { className: "vm-cal-grid", "aria-busy": k, children: [
      ms.map((b) => /* @__PURE__ */ t("div", { className: "vm-cal-dow", children: b }, b)),
      W.map((b) => {
        const L = D.get(b.toDateString()) || [], K = b.getMonth() !== o.getMonth();
        return /* @__PURE__ */ c("div", { className: "vm-cal-cell" + (K ? " vm-muted" : ""), children: [
          /* @__PURE__ */ t("span", { className: "vm-cal-num" + (ds(b, $) ? " vm-today" : ""), children: b.getDate() }),
          /* @__PURE__ */ c("div", { className: "vm-cal-evs", children: [
            L.slice(0, 3).map((X, Ee) => /* @__PURE__ */ c("span", { className: "vm-cal-ev", title: X.summary, children: [
              !X.allDay && /* @__PURE__ */ t("em", { children: Bn(X.start) }),
              " ",
              X.summary || "(busy)"
            ] }, X.uid || Ee)),
            L.length > 3 && /* @__PURE__ */ c("span", { className: "vm-cal-more", children: [
              "+",
              L.length - 3,
              " more"
            ] })
          ] })
        ] }, b.toISOString());
      })
    ] }) : /* @__PURE__ */ t("div", { className: "vm-agenda", "aria-busy": k, children: w.length === 0 ? /* @__PURE__ */ t("div", { className: "vm-empty", children: k ? "Loading…" : "No events this month" }) : /* @__PURE__ */ t("ul", { className: "vm-agenda-list", children: w.map((b, L) => /* @__PURE__ */ c("li", { className: "vm-agenda-row", children: [
      /* @__PURE__ */ c("div", { className: "vm-agenda-when", children: [
        /* @__PURE__ */ t("span", { className: "vm-agenda-date", children: new Date(b.start).toLocaleDateString(void 0, { month: "short", day: "numeric" }) }),
        /* @__PURE__ */ t("span", { className: "vm-agenda-time", children: b.allDay ? "All day" : Bn(b.start) })
      ] }),
      /* @__PURE__ */ c("div", { className: "vm-agenda-main", children: [
        /* @__PURE__ */ t("span", { className: "vm-agenda-sum", children: b.summary || "(no title)" }),
        b.location && /* @__PURE__ */ t("span", { className: "vm-agenda-loc", children: b.location })
      ] })
    ] }, b.uid || L)) }) })
  ] });
}
const hs = (e = "", a = "") => {
  const s = (e || a).trim();
  return s ? s[0].toUpperCase() : "?";
};
function ps({ baseUrl: e = "/v1", client: a, onSelect: s, onAuthError: i }) {
  const u = ke(() => a ?? mn({ baseUrl: e }), [a, e]), [o, g] = C(""), [d, m] = C([]), [E, S] = C(!0), [k, R] = C(""), A = Y((v) => ((v == null ? void 0 : v.status) === 401 && (i == null || i(v)), (v == null ? void 0 : v.message) || "Could not load contacts"), [i]);
  return _e(() => {
    let v = !0;
    S(!0), R("");
    const z = setTimeout(() => {
      u.listContacts({ q: o }).then((B) => {
        v && m(B);
      }).catch((B) => {
        v && (R(A(B)), m([]));
      }).finally(() => {
        v && S(!1);
      });
    }, o ? 200 : 0);
    return () => {
      v = !1, clearTimeout(z);
    };
  }, [u, o, A]), /* @__PURE__ */ c("div", { className: "vm-contacts", children: [
    /* @__PURE__ */ c("header", { className: "vm-contacts-head", children: [
      /* @__PURE__ */ c("div", { className: "vm-brand", children: [
        /* @__PURE__ */ t(p, { name: "users", className: "vm-icon vm-brand-mark" }),
        /* @__PURE__ */ t("span", { children: "Contacts" })
      ] }),
      /* @__PURE__ */ c("form", { className: "vm-search", role: "search", onSubmit: (v) => v.preventDefault(), children: [
        /* @__PURE__ */ t(p, { name: "search", className: "vm-icon" }),
        /* @__PURE__ */ t(
          "input",
          {
            type: "search",
            value: o,
            placeholder: "Search contacts",
            "aria-label": "Search contacts",
            onChange: (v) => g(v.target.value)
          }
        )
      ] })
    ] }),
    k && /* @__PURE__ */ t("div", { className: "vm-error", role: "alert", children: k }),
    E ? /* @__PURE__ */ t("ul", { className: "vm-rows", children: Array.from({ length: 6 }).map((v, z) => /* @__PURE__ */ c("li", { className: "vm-skeleton", "aria-hidden": "true", children: [
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "40%" } }),
      /* @__PURE__ */ t("div", { className: "vm-sk-line", style: { width: "70%" } })
    ] }, z)) }) : d.length === 0 ? /* @__PURE__ */ t("div", { className: "vm-empty", children: "No contacts" }) : /* @__PURE__ */ t("ul", { className: "vm-contact-list", children: d.map((v, z) => /* @__PURE__ */ t("li", { children: /* @__PURE__ */ c(
      "button",
      {
        type: "button",
        className: "vm-contact-row",
        onClick: () => s == null ? void 0 : s(v),
        disabled: !s,
        children: [
          /* @__PURE__ */ t("span", { className: "vm-avatar", style: Ut(v.email || v.name), "aria-hidden": "true", children: hs(v.name, v.email) }),
          /* @__PURE__ */ c("span", { className: "vm-contact-main", children: [
            /* @__PURE__ */ t("span", { className: "vm-contact-name", children: v.name || v.email }),
            v.name && /* @__PURE__ */ t("span", { className: "vm-contact-email", children: v.email })
          ] })
        ]
      }
    ) }, (v.email || "") + z)) })
  ] });
}
const vs = [
  {
    title: "Navigation",
    items: [
      ["j", "Next conversation"],
      ["k", "Previous conversation"],
      ["Enter / o", "Open conversation"],
      ["u", "Back to list"],
      ["/", "Search"]
    ]
  },
  {
    title: "Actions",
    items: [
      ["e", "Archive"],
      ["#", "Delete"],
      ["s", "Star"],
      ["x", "Select"],
      ["c", "Compose"]
    ]
  },
  {
    title: "Reply",
    items: [
      ["r", "Reply"],
      ["a", "Reply all"],
      ["f", "Forward"],
      ["Esc", "Close"],
      ["?", "This help"]
    ]
  }
];
function bs({ onClose: e }) {
  return /* @__PURE__ */ t(
    "div",
    {
      className: "vm-overlay",
      role: "dialog",
      "aria-modal": "true",
      "aria-label": "Keyboard shortcuts",
      onMouseDown: (a) => {
        a.target === a.currentTarget && (e == null || e());
      },
      children: /* @__PURE__ */ c("div", { className: "vm-help", children: [
        /* @__PURE__ */ c("header", { className: "vm-help-head", children: [
          /* @__PURE__ */ c("h2", { children: [
            /* @__PURE__ */ t(p, { name: "keyboard", className: "vm-icon" }),
            " Keyboard shortcuts"
          ] }),
          /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Close", onClick: e, children: /* @__PURE__ */ t(p, { name: "close" }) })
        ] }),
        /* @__PURE__ */ t("div", { className: "vm-help-grid", children: vs.map((a) => /* @__PURE__ */ c("div", { className: "vm-help-col", children: [
          /* @__PURE__ */ t("h3", { children: a.title }),
          /* @__PURE__ */ t("dl", { children: a.items.map(([s, i]) => /* @__PURE__ */ c("div", { className: "vm-help-row", children: [
            /* @__PURE__ */ t("dt", { children: /* @__PURE__ */ t("kbd", { children: s }) }),
            /* @__PURE__ */ t("dd", { children: i })
          ] }, s)) })
        ] }, a.title)) })
      ] })
    }
  );
}
const Gn = (e, a) => Array.isArray(e.flags) && e.flags.includes(a);
function zt(e) {
  return e.messageId || "uid:" + e.id;
}
function gs(e = "") {
  let a = e, s;
  do
    s = a, a = a.replace(/^\s*(re|fwd|fw|aw)\s*:\s*/i, "");
  while (a !== s);
  return a.trim().toLowerCase();
}
function Ns(e = [], { threaded: a = !0 } = {}) {
  if (!a)
    return e.map((d) => Wn([d])).sort((d, m) => m.ts - d.ts);
  const s = /* @__PURE__ */ new Map(), i = (d) => {
    s.has(d) || s.set(d, d);
    let m = d;
    for (; s.get(m) !== m; ) m = s.get(m);
    for (; s.get(d) !== m; ) {
      const E = s.get(d);
      s.set(d, m), d = E;
    }
    return m;
  }, u = (d, m) => {
    const E = i(d), S = i(m);
    E !== S && s.set(E, S);
  };
  for (const d of e) {
    const m = zt(d);
    i(m);
    const E = [...d.references || [], d.inReplyTo].filter(Boolean);
    let S = m;
    for (const k of E)
      u(S, k), S = k;
  }
  const o = /* @__PURE__ */ new Map();
  for (const d of e) {
    const m = gs(d.subject);
    m && (o.has(m) ? u(zt(d), zt(o.get(m))) : o.set(m, d));
  }
  const g = /* @__PURE__ */ new Map();
  for (const d of e) {
    const m = i(zt(d));
    g.has(m) || g.set(m, []), g.get(m).push(d);
  }
  return [...g.values()].map(Wn).sort((d, m) => m.ts - d.ts);
}
function Wn(e) {
  const a = [...e].sort((g, d) => tn(g) - tn(d)), s = a.find((g) => !g.inReplyTo) || a[0], i = a[a.length - 1], u = [], o = /* @__PURE__ */ new Set();
  for (const g of a) {
    const d = g.fromName || g.from || "", m = (g.from || d).toLowerCase();
    d && !o.has(m) && (o.add(m), u.push({ name: d, email: g.from }));
  }
  return {
    id: i.id,
    // open by the latest message's uid
    ids: a.map((g) => g.id),
    messages: a,
    count: a.length,
    root: s,
    latest: i,
    from: i.from,
    fromName: i.fromName,
    subject: s.subject || i.subject,
    preview: i.preview,
    date: i.date,
    ts: tn(i),
    participants: u,
    hasAttachments: a.some((g) => g.hasAttachments),
    unread: a.some((g) => !Gn(g, Ue)),
    starred: a.some((g) => Gn(g, We))
  };
}
function tn(e) {
  const a = new Date(e.date).getTime();
  return Number.isNaN(a) ? 0 : a;
}
const Xn = "vulos-mail.settings.v1", nn = {
  density: "comfortable",
  // 'comfortable' | 'compact'
  readingPane: "right",
  // 'right' | 'bottom' | 'off'
  theme: "system",
  // 'system' (follow OS) | 'dark' | 'light'
  shortcuts: !0,
  threaded: !0,
  signature: ""
};
function ys() {
  try {
    const e = localStorage.getItem(Xn);
    return e ? { ...nn, ...JSON.parse(e) } : { ...nn };
  } catch {
    return { ...nn };
  }
}
function Ts() {
  const [e, a] = C(ys);
  _e(() => {
    try {
      localStorage.setItem(Xn, JSON.stringify(e));
    } catch {
    }
  }, [e]);
  const s = Y((i) => {
    a((u) => ({ ...u, ...typeof i == "function" ? i(u) : i }));
  }, []);
  return [e, s];
}
function Ms(e) {
  if (e.altKey || e.ctrlKey || e.metaKey) return null;
  switch (e.key) {
    case "j":
      return "next";
    case "k":
      return "prev";
    case "o":
      return "open";
    case "Enter":
      return "open";
    case "u":
      return "back";
    case "e":
      return "archive";
    case "#":
      return "delete";
    case "r":
      return "reply";
    case "a":
      return "replyAll";
    case "f":
      return "forward";
    case "c":
      return "compose";
    case "s":
      return "star";
    case "x":
      return "select";
    case "/":
      return "search";
    case "?":
      return "help";
    case "Escape":
      return "escape";
    default:
      return null;
  }
}
function _s(e) {
  if (!e) return !1;
  const a = e.tagName;
  return a === "INPUT" || a === "TEXTAREA" || a === "SELECT" || e.isContentEditable;
}
function Es(e) {
  var s;
  if (!e) return !1;
  const a = e.tagName;
  return a === "BUTTON" || a === "A" || a === "SUMMARY" || ((s = e.getAttribute) == null ? void 0 : s.call(e, "role")) === "button";
}
function Ss(e, a = !0) {
  _e(() => {
    if (!a) return;
    const s = (i) => {
      const u = _s(i.target), o = Ms(i);
      if (!o || u && o !== "escape" || o === "open" && Es(i.target)) return;
      const g = e[o];
      g && ((o === "search" || o === "help" || o === "delete") && i.preventDefault(), g(i));
    };
    return window.addEventListener("keydown", s), () => window.removeEventListener("keydown", s);
  }, [e, a]);
}
function Gt(e = "") {
  return e.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
function qn(e) {
  return e.html ? Kn(e.html) : Gt(e.body || e.preview || "").replace(/\n/g, "<br>");
}
function As(e) {
  const a = Gt(e.fromName || e.from || "");
  return `<br><br><div class="vm-quote-attr">On ${Gt(jt(e.date))}, ${a} wrote:</div><blockquote class="vm-quote">${qn(e)}</blockquote>`;
}
function ws(e) {
  return `<br><br><div class="vm-quote-attr">${[
    "---------- Forwarded message ----------",
    `From: ${e.fromName ? e.fromName + " <" + e.from + ">" : e.from}`,
    `Date: ${jt(e.date)}`,
    `Subject: ${e.subject || ""}`,
    e.to ? `To: ${e.to}` : ""
  ].filter(Boolean).map(Gt).join("<br>")}</div><blockquote class="vm-quote">${qn(e)}</blockquote>`;
}
function ks(e, a = "") {
  const s = a.toLowerCase(), i = (e.from || "").toLowerCase(), u = /* @__PURE__ */ new Set([s, i]), o = [];
  for (const g of [...Mn(e.to || ""), ...Mn(e.cc || "")]) {
    const d = g.toLowerCase();
    d && !u.has(d) && (u.add(d), o.push(g));
  }
  return o.join(", ");
}
let an = 0;
const Cs = 6e3;
function Rs({ baseUrl: e = "/v1", client: a, onSend: s, onAuthError: i, settingsExtra: u }) {
  var we;
  const o = ke(() => a ?? mn({ baseUrl: e }), [a, e]), g = ke(() => s ?? ((r) => o.sendMessage(r)), [s, o]), [d, m] = Ts(), [E, S] = C(null), [k, R] = C([]), [A, v] = C("INBOX"), [z, B] = C(""), [U, D] = C([]), [w, F] = C(!0), [$, W] = C(""), [b, L] = C(null), [K, X] = C({}), [Ee, J] = C(() => /* @__PURE__ */ new Set()), [ee, ge] = C(-1), [Ne, le] = C([]), [ie, T] = C("none"), [oe, ne] = C(!1), [G, y] = C("list"), [Se, ye] = C(!1), [lt, it] = C(!1), [Ve, Re] = C([]), [ot, kt] = C(!0), q = Oe(null), Ct = Oe(null), [Z, Dt] = C(
    () => typeof matchMedia > "u" || matchMedia("(prefers-color-scheme: dark)").matches
  );
  _e(() => {
    var _;
    if (typeof matchMedia > "u") return;
    const r = matchMedia("(prefers-color-scheme: dark)"), N = (P) => Dt(P.matches);
    return (_ = r.addEventListener) == null || _.call(r, "change", N), () => {
      var P;
      return (P = r.removeEventListener) == null ? void 0 : P.call(r, "change", N);
    };
  }, []);
  const Q = d.theme === "light" || d.theme === "dark" ? d.theme : Z ? "dark" : "light", Pe = Oe(/* @__PURE__ */ new Map());
  _e(() => () => {
    for (const r of Pe.current.values()) clearTimeout(r);
  }, []);
  const be = Y((r) => ((r == null ? void 0 : r.status) === 401 && (i == null || i(r)), (r == null ? void 0 : r.message) || "Something went wrong"), [i]), Ce = Y((r, N = "info") => {
    const _ = ++an;
    Re((P) => [...P, { id: _, text: r, kind: N }]), setTimeout(() => Re((P) => P.filter((I) => I.id !== _)), 3200);
  }, []), Je = Y((r, N, _) => {
    const P = ++an, I = setTimeout(() => {
      Pe.current.delete(P), Re((x) => x.filter((fe) => fe.id !== P)), N();
    }, Cs);
    Pe.current.set(P, I), Re((x) => [...x, {
      id: P,
      text: r,
      kind: "info",
      undo: () => {
        clearTimeout(I), Pe.current.delete(P), Re((fe) => fe.filter((Be) => Be.id !== P)), _();
      }
    }]);
  }, []);
  _e(() => {
    let r = !0;
    return o.me().then((N) => r && S(N)).catch(() => {
    }), o.listFolders().then((N) => r && R(N || [])).catch(() => {
    }), () => {
      r = !1;
    };
  }, [o]);
  const Ze = ke(() => {
    const r = k.find((N) => sn(N) === "archive");
    return r ? r.path ?? r.name : null;
  }, [k]);
  ke(() => {
    const r = k.find((N) => sn(N) === "trash");
    return r ? r.path ?? r.name : null;
  }, [k]);
  const Qe = ot && !!Ze, re = Y(async () => {
    F(!0), W("");
    try {
      let r;
      A === wt ? r = (await o.listMessages({ folder: "INBOX", limit: 200 })).filter((_) => (_.flags || []).includes(We)) : z ? r = await o.search(z, { folder: A === wt ? "INBOX" : A }) : r = await o.listMessages({ folder: A }), D(r || []);
    } catch (r) {
      W(be(r)), D([]);
    } finally {
      F(!1);
    }
  }, [o, A, z, be]);
  _e(() => {
    re();
  }, [re]);
  const V = ke(
    () => Ns(U, { threaded: d.threaded && A !== wt }),
    [U, d.threaded, A]
  ), et = ke(
    () => U.filter((r) => (r.flags || []).includes(We)).length,
    [U]
  );
  _e(() => {
    ee >= V.length && ge(V.length - 1);
  }, [V.length, ee]);
  const de = Y((r, N, _) => {
    const P = new Set(r), I = (x) => {
      if (!P.has(x.id)) return x;
      const fe = new Set(x.flags || []);
      return _ ? fe.add(N) : fe.delete(N), { ...x, flags: [...fe] };
    };
    D((x) => x.map(I)), L((x) => x && { ...x, messages: x.messages.map(I) }), X((x) => {
      const fe = { ...x };
      for (const Be of r) fe[Be] && (fe[Be] = I(fe[Be]));
      return fe;
    });
  }, []), Ye = Y((r) => {
    const N = new Set(r);
    D((_) => _.filter((P) => !N.has(P.id)));
  }, []), gt = Y(async (r) => {
    var N;
    if (!((N = K[r]) != null && N.__full))
      try {
        const _ = await o.getMessage(r, { folder: Xe(A) });
        X((P) => ({ ...P, [r]: { ..._, __full: !0 } })), (_.flags || []).includes(Ue) || (de([r], Ue, !0), o.setFlag(r, Ue, !0, { folder: Xe(A) }).catch(() => {
        }));
      } catch (_) {
        be(_);
      }
  }, [o, A, K, de, be]), tt = Y((r) => {
    L(r), y("read"), ge(V.findIndex((_) => _.id === r.id));
    const N = r.messages.filter((_) => !(_.flags || []).includes(Ue)).map((_) => _.id);
    if (N.length) {
      de(N, Ue, !0);
      for (const _ of N) o.setFlag(_, Ue, !0, { folder: Xe(A) }).catch(() => {
      });
    }
  }, [V, o, A, de]), De = Y((r) => r ? [r] : V.filter((N) => Ee.has(N.id)), [V, Ee]), Ie = Y((r, N) => {
    const _ = De(r);
    for (const P of _)
      if (N)
        de([P.latest.id], We, !0), o.setFlag(P.latest.id, We, !0, { folder: Xe(A) }).catch((I) => {
          be(I), re();
        });
      else {
        const I = P.messages.filter((x) => (x.flags || []).includes(We)).map((x) => x.id);
        de(I, We, !1);
        for (const x of I) o.setFlag(x, We, !1, { folder: Xe(A) }).catch((fe) => {
          be(fe), re();
        });
      }
    r || J(/* @__PURE__ */ new Set());
  }, [De, de, o, A, be, re]), ct = Y((r, N) => {
    const P = De(r).flatMap((I) => I.messages.map((x) => x.id));
    de(P, Ue, N);
    for (const I of P) o.setFlag(I, Ue, N, { folder: Xe(A) }).catch((x) => {
      be(x), re();
    });
    r || J(/* @__PURE__ */ new Set());
  }, [De, de, o, A, be, re]), je = Y((r) => {
    const N = De(r);
    if (!N.length) return;
    const _ = N.flatMap((I) => I.messages.map((x) => x.id)), P = Xe(A);
    Ye(_), b && N.some((I) => I.id === b.id) && (L(null), y("list")), J(/* @__PURE__ */ new Set()), Je(
      `Deleted ${N.length > 1 ? N.length + " conversations" : "conversation"}`,
      () => {
        for (const I of _) o.deleteMessage(I, { folder: P }).catch((x) => {
          be(x), re();
        });
      },
      () => re()
    );
  }, [De, Ye, b, o, A, be, re, Je]), nt = Y((r) => {
    if (!Qe) return;
    const N = De(r);
    if (!N.length) return;
    const _ = N.flatMap((I) => I.messages.map((x) => x.id)), P = Xe(A);
    Ye(_), b && N.some((I) => I.id === b.id) && (L(null), y("list")), J(/* @__PURE__ */ new Set()), Je(
      `Archived ${N.length > 1 ? N.length + " conversations" : "conversation"}`,
      () => {
        Promise.all(_.map((I) => o.moveMessage(I, Ze, { folder: P }))).catch((I) => {
          I instanceof fa && (I.status === 404 || I.status === 405) ? (kt(!1), Ce("Archive is not available on this server", "error")) : be(I), re();
        });
      },
      () => re()
    );
  }, [Qe, De, Ye, b, o, Ze, A, be, re, Ce, Je]), mt = Y((r) => {
    J((N) => {
      const _ = new Set(N);
      return _.has(r) ? _.delete(r) : _.add(r), _;
    });
  }, []), Lt = Y((r) => {
    J((N) => {
      const _ = new Set(N);
      for (const P of r) _.add(P);
      return _;
    });
  }, []), Nt = Y((r) => {
    J(r ? new Set(V.map((N) => N.id)) : /* @__PURE__ */ new Set());
  }, [V]), Ae = Y((r = {}) => {
    le((N) => [...N, { id: ++an, initial: r }]);
  }, []), Fe = Y((r) => le((N) => N.filter((_) => _.id !== r)), []), ce = Y((r, N) => {
    const _ = (r.subject || "").replace(/^\s*(re|fwd?|aw)\s*:\s*/i, "");
    Ae(N === "forward" ? { subject: "Fwd: " + _, html: ws(r) } : {
      to: r.from,
      cc: N === "replyAll" ? ks(r, E == null ? void 0 : E.email) : "",
      subject: "Re: " + _,
      html: As(r),
      inReplyTo: r.messageId,
      references: [...r.references || [], r.messageId].filter(Boolean)
    });
  }, [Ae, E]), yt = Y((r) => {
    v(r), B(""), L(null), J(/* @__PURE__ */ new Set()), y("list"), ye(!1), T("none");
  }, []), Ot = Y((r) => {
    B(r), L(null), y("list");
  }, []), Rt = Y(() => {
    B(""), L(null);
  }, []), at = Y((r) => T((N) => N === r ? "none" : r), []), ut = Y((r) => {
    ge((N) => Math.max(0, Math.min(V.length - 1, N < 0 ? 0 : N + r)));
  }, [V.length]), dt = ke(() => ({
    next: () => ut(1),
    prev: () => ut(-1),
    open: () => {
      const r = V[ee];
      r && tt(r);
    },
    back: () => {
      L(null), y("list");
    },
    archive: () => {
      const r = b || V[ee];
      r && nt(r);
    },
    delete: () => {
      const r = b || V[ee];
      r && je(r);
    },
    star: () => {
      const r = b || V[ee];
      r && Ie(r, !r.starred);
    },
    select: () => {
      const r = V[ee];
      r && mt(r.id);
    },
    reply: () => {
      const r = b;
      r && ce(K[r.latest.id] || r.latest, "reply");
    },
    replyAll: () => {
      const r = b;
      r && ce(K[r.latest.id] || r.latest, "replyAll");
    },
    forward: () => {
      const r = b;
      r && ce(K[r.latest.id] || r.latest, "forward");
    },
    compose: () => Ae(),
    search: () => {
      var r;
      return (r = q.current) == null ? void 0 : r.focus();
    },
    help: () => ne(!0),
    escape: () => {
      oe ? ne(!1) : Ne.length ? Fe(Ne[Ne.length - 1].id) : ie !== "none" ? T("none") : b && (L(null), y("list"));
    }
  }), [V, ee, b, K, ut, tt, nt, je, Ie, mt, ce, Ae, oe, ie, Ne, Fe]);
  Ss(dt, d.shortcuts);
  const ft = Y((r) => o.listContacts({ q: r, limit: 6 }).catch(() => []), [o]);
  return /* @__PURE__ */ c(
    "div",
    {
      ref: Ct,
      className: "vm-app",
      "data-theme": Q,
      "data-density": d.density,
      "data-rp": d.readingPane,
      "data-open": b ? "1" : "0",
      "data-pane": G,
      "data-drawer": Se ? "1" : "0",
      "data-panel-open": ie !== "none" ? "1" : "0",
      children: [
        Se && /* @__PURE__ */ t("div", { className: "vm-scrim", onClick: () => ye(!1), "aria-hidden": "true" }),
        /* @__PURE__ */ t(
          ba,
          {
            folders: k,
            current: A,
            me: E,
            collapsed: lt,
            starredCount: et,
            onToggleCollapse: () => it((r) => !r),
            onSelect: yt,
            onCompose: () => Ae(),
            onOpenPanel: (r) => {
              T(r), ye(!1);
            },
            onOpenHelp: () => {
              ne(!0), ye(!1);
            }
          }
        ),
        /* @__PURE__ */ c("div", { className: "vm-main", children: [
          /* @__PURE__ */ t(
            ya,
            {
              threads: V,
              selectedId: (b == null ? void 0 : b.id) ?? null,
              focusId: ((we = V[ee]) == null ? void 0 : we.id) ?? null,
              selection: Ee,
              onToggleSelect: mt,
              onSelectRange: Lt,
              onSelectAll: Nt,
              onOpen: tt,
              onCompose: () => Ae(),
              onToggleStar: Ie,
              onArchive: nt,
              onDelete: je,
              onToggleRead: ct,
              onRefresh: re,
              loading: w,
              error: $,
              onRetry: re,
              query: z,
              onSearch: Ot,
              onClearSearch: Rt,
              canArchive: Qe,
              folder: A,
              searchRef: q,
              onMenu: () => ye(!0)
            }
          ),
          /* @__PURE__ */ t(
            rs,
            {
              thread: b,
              fullById: K,
              onNeedBody: gt,
              canArchive: Qe,
              onToggleStar: (r) => b && Ie(b, r),
              onArchive: () => b && nt(b),
              onDelete: () => b && je(b),
              onReply: (r) => ce(r, "reply"),
              onReplyAll: (r) => ce(r, "replyAll"),
              onForward: (r) => ce(r, "forward"),
              onBack: () => {
                L(null), y("list");
              }
            }
          )
        ] }),
        /* @__PURE__ */ c("aside", { className: "vm-rightrail", "aria-label": "Side panels", children: [
          /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn" + (ie === "calendar" ? " vm-on" : ""), "aria-label": "Calendar", title: "Calendar", onClick: () => at("calendar"), children: /* @__PURE__ */ t(p, { name: "calendar" }) }),
          /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn" + (ie === "contacts" ? " vm-on" : ""), "aria-label": "Contacts", title: "Contacts", onClick: () => at("contacts"), children: /* @__PURE__ */ t(p, { name: "users" }) }),
          /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn" + (ie === "settings" ? " vm-on" : ""), "aria-label": "Settings", title: "Settings", onClick: () => at("settings"), children: /* @__PURE__ */ t(p, { name: "settings" }) }),
          /* @__PURE__ */ t("span", { className: "vm-spacer" }),
          /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Keyboard shortcuts", title: "Keyboard shortcuts", onClick: () => ne(!0), children: /* @__PURE__ */ t(p, { name: "keyboard" }) })
        ] }),
        ie !== "none" && /* @__PURE__ */ c("aside", { className: "vm-panel", "aria-label": ie, children: [
          ie === "settings" && /* @__PURE__ */ t(cs, { settings: d, onChange: m, onClose: () => T("none"), extra: u }),
          ie === "calendar" && /* @__PURE__ */ c("div", { className: "vm-panel-embed", children: [
            /* @__PURE__ */ c("div", { className: "vm-panel-head", children: [
              /* @__PURE__ */ c("h2", { children: [
                /* @__PURE__ */ t(p, { name: "calendar", className: "vm-icon" }),
                " Calendar"
              ] }),
              /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Close", onClick: () => T("none"), children: /* @__PURE__ */ t(p, { name: "close" }) })
            ] }),
            /* @__PURE__ */ t(fs, { client: o, defaultView: "agenda", onAuthError: i })
          ] }),
          ie === "contacts" && /* @__PURE__ */ c("div", { className: "vm-panel-embed", children: [
            /* @__PURE__ */ c("div", { className: "vm-panel-head", children: [
              /* @__PURE__ */ c("h2", { children: [
                /* @__PURE__ */ t(p, { name: "users", className: "vm-icon" }),
                " Contacts"
              ] }),
              /* @__PURE__ */ t("button", { type: "button", className: "vm-iconbtn", "aria-label": "Close", onClick: () => T("none"), children: /* @__PURE__ */ t(p, { name: "close" }) })
            ] }),
            /* @__PURE__ */ t(ps, { client: o, onSelect: (r) => {
              Ae({ to: r.email }), T("none");
            }, onAuthError: i })
          ] })
        ] }),
        /* @__PURE__ */ t("button", { type: "button", className: "vm-fab", "aria-label": "Compose", onClick: () => Ae(), children: /* @__PURE__ */ t(p, { name: "pencil" }) }),
        /* @__PURE__ */ t("div", { className: "vm-compose-stack", children: Ne.map((r, N) => /* @__PURE__ */ t("div", { className: "vm-compose-slot", style: { "--slot": N }, children: /* @__PURE__ */ t(
          os,
          {
            initial: r.initial,
            signature: d.signature,
            onContactSearch: ft,
            onSaveDraft: (_) => o.saveDraft(_),
            onSend: async (_) => {
              await g(_), Ce("Message sent", "success"), re();
            },
            onClose: () => Fe(r.id)
          }
        ) }, r.id)) }),
        oe && /* @__PURE__ */ t(bs, { onClose: () => ne(!1) }),
        /* @__PURE__ */ t("div", { className: "vm-toasts", "aria-live": "polite", children: Ve.map((r) => /* @__PURE__ */ c("div", { className: "vm-toast vm-toast-" + r.kind, children: [
          /* @__PURE__ */ t("span", { className: "vm-toast-text", children: r.text }),
          r.undo && /* @__PURE__ */ t("button", { type: "button", className: "vm-toast-action", onClick: r.undo, children: "Undo" })
        ] }, r.id)) })
      ]
    }
  );
}
function Xe(e) {
  return e === wt ? "INBOX" : e;
}
export {
  fa as ApiError,
  fs as Calendar,
  os as Compose,
  ps as Contacts,
  nn as DEFAULT_SETTINGS,
  We as FLAG_FLAGGED,
  Ue as FLAG_SEEN,
  ba as FolderList,
  p as Icon,
  Rs as MailApp,
  ya as MessageList,
  rs as MessageView,
  cs as Settings,
  mn as createMailClient,
  Ns as groupThreads,
  Kn as sanitizeEmailHtml,
  ss as stripHtml,
  Ts as useSettings
};
