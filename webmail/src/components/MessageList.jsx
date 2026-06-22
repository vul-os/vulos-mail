import { useEffect } from "react";
import Icon from "./Icon.jsx";
import { STAR, SYS_ICONS, kw, fromName, fromAddr, avatarColor, initials, fmtDate } from "../lib/util.js";

function Row({ row, i, sel, openId, picked, onOpen, onStar, onPick }) {
  const m = row.msg;
  const unread = !kw(m, "$seen");
  const starred = kw(m, "$flagged");
  const nm = fromName(m);
  const threadCount = m.__threadCount;
  return (
    <li
      className={"row" + (unread ? " unread" : "") + (m.id === openId ? " active" : "") + (i === sel ? " active" : "") + (picked ? " picked" : "")}
      data-i={i} data-id={row.id}
      style={{ animationDelay: Math.min(i, 14) * 24 + "ms" }}
      onClick={(e) => {
        if (e.target.closest("[data-star]")) { e.stopPropagation(); onStar(row); return; }
        if (e.target.closest("[data-pick]")) { e.stopPropagation(); onPick(row); return; }
        onOpen(i);
      }}
    >
      {unread && <span className="row-unreaddot" />}
      <span className="pick" data-pick title="Select">
        <Icon body='<path d="M20 6 9 17l-5-5"/>' style={{ width: 12, height: 12, strokeWidth: 2.6 }} />
      </span>
      <div className="row-avatar" style={{ background: avatarColor(fromAddr(m)) }}>{initials(nm)}</div>
      <div className="row-main">
        <div className="row-top">
          <span className="row-from">{nm}</span>
          {threadCount > 1 && <span className="row-thread" title={threadCount + " messages"}>{threadCount}</span>}
          <svg viewBox="0 0 24 24" className={"star" + (starred ? " on" : "")} data-star title="Star">
            <path d={STAR} />
          </svg>
          <span className="row-date">{fmtDate(m.receivedAt)}</span>
        </div>
        <div className="row-subj">{m.subject || "(no subject)"}</div>
        <div className="row-snip">{m.preview || ""}</div>
      </div>
    </li>
  );
}

function Selbar({ count, onBulk, onClear }) {
  const act = (s, t, body) => (
    <button className="iconbtn" data-s={s} title={t} onClick={() => (s === "clear" ? onClear() : onBulk(s))}>
      <Icon body={body} />
    </button>
  );
  return (
    <div className="selbar" id="selbar">
      <span className="selcount"><b>{count}</b> selected</span>
      {act("read", "Mark read", '<path d="M22 6 12 13 2 6"/><rect x="2" y="4" width="20" height="16" rx="2"/>')}
      {act("star", "Star", `<path d="${STAR}"/>`)}
      {act("archive", "Archive", SYS_ICONS.archive)}
      {act("trash", "Delete", SYS_ICONS.trash)}
      {act("clear", "Clear", '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>')}
    </div>
  );
}

export default function MessageList({
  title, rows, total, loading, loadingMore, filter, sel, openId, selected,
  searchRef, listElRef, onSearch, onRefresh, onOpenRow, onToggleStar, onToggleSel, onClearSel, onBulk, onLoadMore,
}) {
  const count = filter ? rows.length : (total || rows.length);

  // Infinite scroll: load the next page when near the bottom.
  useEffect(() => {
    const elr = listElRef.current;
    if (!elr) return;
    const onScroll = () => {
      if (elr.scrollTop + elr.clientHeight >= elr.scrollHeight - 400) onLoadMore();
    };
    elr.addEventListener("scroll", onScroll);
    return () => elr.removeEventListener("scroll", onScroll);
  }, [listElRef, onLoadMore]);

  return (
    <section className="list-pane" id="list-pane">
      <header className="topbar">
        <div className="search">
          <Icon body='<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>' />
          <input id="search" ref={searchRef} type="search" placeholder="Search mail   /" spellCheck={false}
            onChange={(e) => onSearch(e.target.value.trim())} />
          <kbd className="search-kbd">/</kbd>
        </div>
        <button className="iconbtn" id="refresh" title="Refresh (g i)" onClick={onRefresh}>
          <Icon body='<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>' />
        </button>
      </header>
      <div className="list-head">
        <h1 id="list-title">{title}</h1>
        <span className="list-count" id="list-count">
          {count ? count + " conversation" + (count > 1 ? "s" : "") : ""}
        </span>
      </div>

      {selected.size > 0 && <Selbar count={selected.size} onBulk={onBulk} onClear={onClearSel} />}

      <ol className="msglist" id="msglist" ref={listElRef} tabIndex={0} aria-label="Messages">
        {loading ? (
          Array.from({ length: 8 }).map((_, i) => (
            <li key={i} className="skeleton">
              <div className="sk-line" style={{ width: "40%" }} />
              <div className="sk-line" style={{ width: "80%" }} />
              <div className="sk-line" style={{ width: "60%" }} />
            </li>
          ))
        ) : (
          rows.map((row, i) => (
            <Row key={row.id} row={row} i={i} sel={sel} openId={openId}
              picked={selected.has(row.id)}
              onOpen={onOpenRow} onStar={onToggleStar} onPick={onToggleSel} />
          ))
        )}
        {loadingMore && (
          <li className="skeleton"><div className="sk-line" style={{ width: "50%" }} /></li>
        )}
      </ol>

      {!loading && rows.length === 0 && (
        <div className="list-empty" id="list-empty">{filter ? "No matches." : "No mail here."}</div>
      )}
    </section>
  );
}
