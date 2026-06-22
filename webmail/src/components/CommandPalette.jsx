import { useEffect, useRef, useState } from "react";
import Icon from "./Icon.jsx";

export default function CommandPalette({ commands, onClose }) {
  const [q, setQ] = useState("");
  const [idx, setIdx] = useState(0);
  const inputRef = useRef(null);
  const listRef = useRef(null);

  useEffect(() => { inputRef.current?.focus(); }, []);

  const ff = q.trim().toLowerCase();
  const filtered = ff ? commands.filter((c) => c.label.toLowerCase().includes(ff)) : commands;
  const clamped = Math.max(0, Math.min(idx, filtered.length - 1));

  function run(i) {
    const c = filtered[i];
    onClose();
    if (c) setTimeout(c.run, 0);
  }

  function onKey(e) {
    if (e.key === "Escape") { e.preventDefault(); onClose(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); setIdx((x) => Math.min(x + 1, filtered.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setIdx((x) => Math.max(x - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); run(clamped); }
  }

  useEffect(() => {
    const on = listRef.current?.querySelector(".cmdk-item.on");
    if (on) on.scrollIntoView({ block: "nearest" });
  }, [clamped]);

  return (
    <div className="cmdk" id="cmdk" onClick={(e) => { if (e.target.id === "cmdk") onClose(); }}>
      <div className="cmdk-box">
        <input className="cmdk-in" id="cmdk-in" ref={inputRef} placeholder="Type a command or jump to a label…"
          spellCheck={false} value={q} onChange={(e) => { setQ(e.target.value); setIdx(0); }} onKeyDown={onKey} />
        <div className="cmdk-list" id="cmdk-list" ref={listRef}>
          {filtered.length ? filtered.map((c, i) => (
            <div key={c.label} className={"cmdk-item" + (i === clamped ? " on" : "")} data-i={i}
              onMouseEnter={() => setIdx(i)} onClick={() => run(i)}>
              <Icon body={c.ic} />
              <span>{c.label}</span>
              {c.k && <span className="k">{c.k}</span>}
            </div>
          )) : <div className="cmdk-empty">No matching commands</div>}
        </div>
      </div>
    </div>
  );
}
