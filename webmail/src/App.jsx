import { useCallback, useEffect, useRef, useState } from "react";
import { MailApp } from "@vulos/mail-ui";
import { JMAP } from "./lib/jmap.js";
import Login from "./components/Login.jsx";

// Auth transport for the app shell (sign-in / sign-up / outbound send). The mail
// surface itself is now the shared @vulos/mail-ui <MailApp/>, which talks to the
// lilmail /v1 JSON API. This file is intentionally thin: shell + auth + mount.
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

  // Compose send: @vulos/mail-ui sends via POST /v1/messages by default, but
  // vulos-mail submits outbound mail over JMAP, so we override onSend to route
  // through jmap.send. The shared <Compose/> passes {to, cc, bcc, subject, text}.
  const onSend = useCallback(async ({ to, cc, bcc, subject, text }) => {
    const list = (s) => (s || "").split(",").map((x) => x.trim()).filter(Boolean);
    await jmap.send({
      to: list(to),
      cc: list(cc),
      bcc: list(bcc),
      subject: subject || "",
      text: text || "",
    });
  }, []);

  // Avoid a flash of the login screen while restoring a session.
  if (!ready) return null;

  return authed
    ? <MailApp baseUrl="/v1" onSend={onSend} onAuthError={onLogout} />
    : <Login jmap={jmap} onLogin={onLogin} />;
}
