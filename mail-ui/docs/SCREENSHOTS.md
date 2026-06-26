# Screenshots

The `@vulos/mail-ui` gallery is captured by a Playwright screenshotter that
boots the shared components against a **mock `/v1` backend** — the standalone
demo in `src/demo/` driven by `src/demo/mockClient.js` (seeded fixtures). No live
IMAP / SMTP / CalDAV / CardDAV server and **no credentials** are required.

## Gallery

| File | Description | Status |
|------|-------------|--------|
| `docs/screenshots/hero.png` | Three-pane mail view with a message open (hero) | Real — mock `/v1` |
| `docs/screenshots/mail.png` | `<MailApp/>` three-pane (folders \| list \| reading pane) | Real — mock `/v1` |
| `docs/screenshots/calendar.png` | `<Calendar/>` month view with seeded events | Real — mock `/v1` |
| `docs/screenshots/contacts.png` | `<Contacts/>` list | Real — mock `/v1` |

## Regenerate

```bash
npm install            # first time (installs Playwright)
npm run screenshots    # builds the demo SPA + captures all screens
```

This will:
1. Build the standalone demo SPA (`npm run build` → `dist/`) if not already built.
2. Serve `dist/` over a tiny in-process static server (no extra deps).
3. Launch headless Chromium (1280×800, dark, 2× DPI) and capture the Mail,
   Calendar, and Contacts tabs to `docs/screenshots/`.

Capture an already-running demo instead (e.g. `npm run dev`):

```bash
BASE_URL=http://localhost:5173 MAILUI_EXTERNAL=1 npm run screenshots
```

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Node.js | 18+ | Runs Vite + the Playwright script |
| Playwright Chromium | — | `npm install` pulls the `playwright` package; run `npx playwright install chromium` once if the browser is missing |

## Reproducibility

The demo seed lives in `src/demo/mockClient.js`. Calendar event dates are
relative to `Date.now()` at build time, so the visible month tracks the current
date; message and contact content are stable.
