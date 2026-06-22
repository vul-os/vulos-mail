import Icon from "./Icon.jsx";
import { useToast } from "./Toasts.jsx";
import {
  STAR, SYS_ICONS, kw, fromName, fromAddr, addrList, avatarColor, initials,
  fmtFull, fmtBytes, renderEmailHTML, labelName,
} from "../lib/util.js";

function ActBtn({ act, title, body, cls = "", onClick }) {
  return (
    <button className={"iconbtn " + cls} data-act={act} title={title} onClick={onClick}>
      <Icon body={body} />
    </button>
  );
}

export default function ReadPane({ jmap, msg, busy, labels, reading, onBack, onArchive, onTrash, onToggleStar, onMarkUnread, onReply }) {
  const toast = useToast();

  async function downloadAtt(id, n, name) {
    try {
      const blob = await jmap.download(`/api/webmail/attachment?id=${encodeURIComponent(id)}&n=${n}`);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a"); a.href = url; a.download = name || "attachment"; a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1500);
    } catch (ex) { toast(ex.message); }
  }

  const empty = !reading || (!msg && !busy);

  return (
    <section className="read-pane" id="read-pane">
      <header className="read-mobilebar" id="read-mobilebar">
        <button className="iconbtn" id="read-back" title="Back to list" aria-label="Back to list" onClick={onBack}>
          <Icon body='<path d="M19 12H5"/><path d="m12 19-7-7 7-7"/>' />
        </button>
        <span className="read-mobilebar-title">{msg && !msg.__error ? (msg.subject || "(no subject)") : "Conversation"}</span>
      </header>

      {empty && (
        <div className="read-empty" id="read-empty">
          <span className="logo logo-xl" aria-hidden="true" />
          <p>Select a conversation</p>
          <p className="hint"><kbd>j</kbd><kbd>k</kbd> move · <kbd>Enter</kbd> open · <kbd>c</kbd> compose · <kbd>?</kbd> shortcuts</p>
        </div>
      )}

      {reading && (busy || msg) && (
        <article className="read" id="read">
          <div className="read-actions">
            <ActBtn act="back" title="Back (u)" body='<path d="M19 12H5"/><path d="m12 19-7-7 7-7"/>' onClick={onBack} />
            <ActBtn act="archive" title="Archive (e)" body={SYS_ICONS.archive} onClick={onArchive} />
            <ActBtn act="trash" title="Delete (#)" body={SYS_ICONS.trash} onClick={onTrash} />
            <ActBtn act="star" title={msg && kw(msg, "$flagged") ? "Unstar (s)" : "Star (s)"} body={`<path d="${STAR}"/>`} cls={msg && kw(msg, "$flagged") ? "on" : ""} onClick={onToggleStar} />
            <ActBtn act="unread" title="Mark unread" body='<path d="M22 6 12 13 2 6"/><rect x="2" y="4" width="20" height="16" rx="2"/>' onClick={onMarkUnread} />
            <ActBtn act="reply" title="Reply (r)" body='<path d="M9 17 4 12l5-5"/><path d="M20 18v-2a4 4 0 0 0-4-4H4"/>' onClick={onReply} />
          </div>

          {busy && !msg && (
            <>
              <div className="sk-line" style={{ width: "55%" }} />
              <div className="sk-line" style={{ width: "85%" }} />
              <div className="sk-line" style={{ width: "70%" }} />
            </>
          )}

          {msg && msg.__error && (
            <div className="read-empty"><p>{msg.__error}</p></div>
          )}

          {msg && !msg.__error && (
            <>
              <h1 className="read-subject">{msg.subject || "(no subject)"}</h1>
              <div className="read-labels">
                {Object.keys(msg.mailboxIds || {}).map((id) => (
                  <span key={id} className="chip">{labelName(labels, id)}</span>
                ))}
              </div>
              <div className="msg">
                <div className="msg-head">
                  <div className="avatar" style={{ background: avatarColor(fromAddr(msg)) }}>{initials(fromName(msg))}</div>
                  <div className="msg-meta">
                    <div className="msg-from">{fromName(msg)}</div>
                    <div className="msg-addr">{fromAddr(msg)}</div>
                    <div className="msg-to">to {addrList(msg.to) || "you"}</div>
                  </div>
                  <div className="msg-date">{fmtFull(msg.receivedAt)}</div>
                </div>
                {/* XSS-inert: renderEmailHTML escapes plain text or sanitizes HTML
                    to a safe subset — script/onerror never survive. */}
                <div className="msg-body" dangerouslySetInnerHTML={{ __html: renderEmailHTML(msg) }} />
                {msg.attachments && msg.attachments.length > 0 && (
                  <div className="read-atts">
                    {msg.attachments.map((a, i) => (
                      <div key={i} className="read-att" data-att={i} onClick={() => downloadAtt(msg.id, i, a.name)}>
                        <Icon body='<path d="M21 11.5 12.5 20a4 4 0 0 1-6-6l8-8a2.5 2.5 0 0 1 4 4l-8 8a1 1 0 0 1-1.5-1.5L17 11"/>' />
                        <span className="nm">{a.name || "attachment"}</span>
                        <span className="sz">{fmtBytes(a.size)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </>
          )}
        </article>
      )}
    </section>
  );
}
