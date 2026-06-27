import { useCallback, useEffect, useRef, useState } from "react";
import { MailApp, Calendar, Contacts } from "@vulos/mail-ui";
import Login from "./components/Login.jsx";
import AccountSettings from "./components/AccountSettings.jsx";

// Which Mail surface to render, derived from the URL path. The Mail product
// exposes Calendar and Contacts as standalone surfaces (mail.vulos.org/calendar
// and /contacts) in addition to the default mailbox. We match on the path
// *suffix* so the same build works whether it is mounted at the origin root
// (mail.vulos.org) or behind the OS app gateway (/app/lilmail/calendar), which
// rewrites the base href but forwards the trailing /calendar|/contacts segment.
function currentSurface() {
  const path = window.location.pathname.replace(/\/+$/, "");
  if (path.endsWith("/calendar")) return "calendar";
  if (path.endsWith("/contacts")) return "contacts";
  return "mail";
}

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

  if (!authed) return <Login onLogin={() => setAuthed(true)} />;

  // All surfaces share the same /v1 → lilmail client/baseUrl and the same
  // server-side session; only the rendered component differs. Calendar and
  // Contacts hit the /v1 calendar/contacts (CalDAV/CardDAV) APIs.
  switch (currentSurface()) {
    case "calendar":
      return <Calendar baseUrl="/v1" onAuthError={onLogout} />;
    case "contacts":
      return <Contacts baseUrl="/v1" onAuthError={onLogout} />;
    default:
      // No onSend override: the mail-ui sends via POST /v1/messages, which the
      // server proxies to the lilmail engine (submits over SMTP back to vulos-mail).
      // settingsExtra injects the standalone self-hoster's account surface
      // (identity, IMAP/SMTP client setup, change password, sign out) into the
      // shared mail UI's Settings panel — backed by /api/webmail/account.
      return <MailApp baseUrl="/v1" onAuthError={onLogout} settingsExtra={<AccountSettings onLogout={onLogout} />} />;
  }
}
