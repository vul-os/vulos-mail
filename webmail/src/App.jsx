import { useCallback, useEffect, useRef, useState } from "react";
import { MailApp } from "@vulos/mail-ui";
import Login from "./components/Login.jsx";

// Thin webmail shell: sign-in + mount of the shared @vulos/mail-ui <MailApp/>.
//
// The mail surface talks exclusively to the lilmail /v1 JSON API for ALL mail
// data — folders, messages, search, flags, delete AND send (POST /v1/messages).
// In the standalone deployment vulos-mail reverse-proxies /v1 to a lilmail engine
// pointed at its own IMAP/SMTP (see cmd/vulos-mail/main.go). Auth is a server-side
// session: POST /api/webmail/login mints an HttpOnly cookie that the mail-ui's
// /v1 requests ride (credentials: 'include'); the proxy brokers the cookie's
// credentials to the engine. The browser never holds the password.
export default function App() {
  const [ready, setReady] = useState(false); // initial session probe done
  const [authed, setAuthed] = useState(false);
  const bootedRef = useRef(false);

  // Probe for an existing session on first mount (the cookie, if any, is sent
  // automatically). /v1/me returns 200 when signed in, 401 otherwise.
  useEffect(() => {
    if (bootedRef.current) return;
    bootedRef.current = true;
    (async () => {
      try {
        const r = await fetch("/v1/me", { credentials: "include" });
        if (r.ok) setAuthed(true);
      } catch {
        /* offline / engine down — fall through to the login screen */
      }
      setReady(true);
    })();
  }, []);

  const onLogout = useCallback(async () => {
    try {
      await fetch("/api/webmail/logout", { method: "POST", credentials: "include" });
    } catch {
      /* best-effort — reload clears in-memory state regardless */
    }
    location.reload();
  }, []);

  // Avoid a flash of the login screen while probing the session.
  if (!ready) return null;

  // No onSend override: the mail-ui sends via POST /v1/messages, which the server
  // proxies to the lilmail engine (which submits over SMTP back to vulos-mail).
  return authed
    ? <MailApp baseUrl="/v1" onAuthError={onLogout} />
    : <Login onLogin={() => setAuthed(true)} />;
}
