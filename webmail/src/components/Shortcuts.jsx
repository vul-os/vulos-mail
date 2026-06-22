const SHORTCUTS = [
  ["c", "Compose"], ["/", "Search"], ["j / k", "Next / previous"], ["Enter", "Open"],
  ["u", "Back to list"], ["e", "Archive"], ["#", "Delete"], ["s", "Star"],
  ["r", "Reply"], ["g i", "Go to Inbox"], ["x", "Select"], ["⌘ K", "Command palette"],
  ["?", "This help"], ["Esc", "Close"],
];

export default function Shortcuts({ onClose }) {
  return (
    <div className="overlay" id="shortcuts" onClick={(e) => { if (e.target.id === "shortcuts") onClose(); }}>
      <div className="shortcuts-card">
        <h2>Keyboard shortcuts</h2>
        <div className="sc-grid" id="sc-grid">
          {SHORTCUTS.map(([k, d]) => (
            <div key={d} className="sc"><span>{d}</span><kbd>{k}</kbd></div>
          ))}
        </div>
        <button className="btn btn-ghost" id="sc-close" onClick={onClose}>Close</button>
      </div>
    </div>
  );
}
