import AppsAndBots from "@vulos/apps-ui";
import "@vulos/apps-ui/styles.css";

// AppsAndBotsSurface — the Mail product's Apps & Bots manage place, built on the
// shared @vulos/apps-ui <AppsAndBots mode="product"/>. Reachable from Settings →
// "Apps & bots" (which navigates to /apps).
//
// Auth: the appsplatform management API on the vulos-mail server is authed with
// the SAME HttpOnly webmail session cookie the rest of the webmail uses (not a
// bearer token). The lib is tokens-first, so we inject a fetcher that adds
// credentials:"include" — the session cookie rides along and the server's Admin
// adapter resolves the signed-in mailbox as the owner. No app token is ever held
// in the browser.
const cookieFetcher = (input, init = {}) =>
  fetch(input, { ...init, credentials: "include" });

export default function AppsAndBotsSurface() {
  return (
    <div className="vm-apps-page">
      <header className="vm-apps-topbar">
        <a className="vm-apps-back" href="/" aria-label="Back to mail">
          ← Back to Mail
        </a>
      </header>
      <main className="vm-apps-main">
        <AppsAndBots
          mode="product"
          product="mail"
          baseUrl=""
          basePath="/api/apps"
          theme="light"
          fetcher={cookieFetcher}
          title="Mail — Apps & Bots"
          subtitle="Install and manage apps & bots that read and act on your mail via the lilmail /v1 API."
        />
      </main>
    </div>
  );
}
