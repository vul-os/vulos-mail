import { useCallback, useEffect, useRef, useState } from "react";
import { JMAP } from "./lib/jmap.js";
import Login from "./components/Login.jsx";
import Mail from "./components/Mail.jsx";

// Single shared JMAP client (same instance shared across login + app, exactly
// like the vanilla SPA's module-scoped `jmap`).
const jmap = new JMAP("");

export default function App() {
  const [ready, setReady] = useState(false);   // initial session restore done
  const [authed, setAuthed] = useState(false);
  const bootedRef = useRef(false);

  // Restore a saved session on first mount.
  useEffect(() => {
    if (bootedRef.current) return;
    bootedRef.current = true;
    (async () => {
      const saved = sessionStorage.getItem("vulos-mail.auth");
      if (saved) {
        try {
          const { u, p } = JSON.parse(saved);
          jmap.setAuth(u, p);
          await jmap.session();
          setAuthed(true);
        } catch {
          sessionStorage.removeItem("vulos-mail.auth");
        }
      }
      setReady(true);
    })();
  }, []);

  const onLogin = useCallback((u, p) => {
    sessionStorage.setItem("vulos-mail.auth", JSON.stringify({ u, p }));
    setAuthed(true);
  }, []);

  const onLogout = useCallback(() => {
    sessionStorage.removeItem("vulos-mail.auth");
    location.reload();
  }, []);

  // Avoid a flash of the login screen while restoring a session.
  if (!ready) return null;

  return authed
    ? <Mail jmap={jmap} onLogout={onLogout} />
    : <Login jmap={jmap} onLogin={onLogin} />;
}
