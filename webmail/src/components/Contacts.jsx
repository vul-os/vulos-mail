import { useEffect, useRef, useState } from "react";
import Icon from "./Icon.jsx";
import { useToast } from "./Toasts.jsx";
import { avatarColor, initials } from "../lib/util.js";

export default function Contacts({ jmap, onClose, onCompose }) {
  const toast = useToast();
  const [list, setList] = useState(null);   // null = loading
  const [search, setSearch] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const nameRef = useRef(null);

  useEffect(() => {
    nameRef.current?.focus();
    (async () => { try { setList(await jmap.contacts()); } catch { setList([]); } })();
  }, [jmap]);

  async function add(e) {
    e.preventDefault();
    const em = email.trim();
    if (!em) return;
    try {
      const r = await jmap.addContact({ name: name.trim(), email: em });
      setList((xs) => [...(xs || []), { id: r.id, name: name.trim(), email: em }]);
      setName(""); setEmail(""); toast("Contact added");
    } catch (ex) { toast(ex.message); }
  }

  async function del(id) {
    await jmap.delContact(id);
    setList((xs) => (xs || []).filter((c) => c.id !== id));
    toast("Contact deleted");
  }

  const ff = search.toLowerCase();
  const shown = (list || []).filter((c) => !ff || (c.name + " " + c.email).toLowerCase().includes(ff))
    .sort((a, b) => (a.name || a.email).localeCompare(b.name || b.email));

  return (
    <div className="overlay" id="contacts" onClick={(e) => { if (e.target.id === "contacts") onClose(); }}>
      <div className="contacts-card">
        <div className="contacts-head">
          <h2>Contacts</h2>
          <button className="iconbtn" id="contacts-close" title="Close" onClick={onClose}>
            <Icon body='<path d="M18 6 6 18M6 6l12 12"/>' />
          </button>
        </div>
        <form id="contact-form" className="contact-add" onSubmit={add}>
          <input id="contact-name" ref={nameRef} type="text" placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <input id="contact-email" type="email" placeholder="email@domain" required value={email} onChange={(e) => setEmail(e.target.value)} />
          <button className="btn btn-primary" type="submit">Add</button>
        </form>
        <input id="contact-search" className="contact-search" type="search" placeholder="Search contacts" value={search} onChange={(e) => setSearch(e.target.value)} />
        <div className="contacts-list" id="contacts-list">
          {list === null ? (
            <div className="contacts-empty">Loading…</div>
          ) : shown.length === 0 ? (
            <div className="contacts-empty">{(list || []).length ? "No matches." : "No contacts yet. Add one above."}</div>
          ) : (
            shown.map((c) => {
              const nm = c.name || c.email.split("@")[0];
              return (
                <div key={c.id} className="contact">
                  <div className="avatar" style={{ background: avatarColor(c.email) }}>{initials(nm)}</div>
                  <div className="contact-meta">
                    <div className="contact-name">{nm}</div>
                    <div className="contact-email">{c.email}</div>
                  </div>
                  <div className="contact-acts">
                    <button className="iconbtn" data-mail title="Compose" onClick={() => onCompose(c.email)}>
                      <Icon body='<path d="M4 4h16v16H4z"/><path d="m22 6-10 7L2 6"/>' />
                    </button>
                    <button className="iconbtn" data-del title="Delete" onClick={() => del(c.id)}>
                      <Icon body='<path d="M3 6h18"/><path d="M8 6V4h8v2m1 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>' />
                    </button>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}
