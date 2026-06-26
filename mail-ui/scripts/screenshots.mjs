/**
 * @vulos/mail-ui Playwright screenshotter
 *
 * Boots the shared mail UI against a MOCK /v1 backend (the standalone demo in
 * src/demo, driven by src/demo/mockClient.js — seeded fixtures, no live IMAP /
 * SMTP / CalDAV / CardDAV) and captures the key screens to docs/screenshots/.
 *
 * Modelled on lilmail's scripts/screenshots.mjs, but the "server" here is just
 * the built demo SPA served over a tiny static file server — zero credentials,
 * zero setup.
 *
 *   npm run screenshots          # build the demo + capture all screens
 *   BASE_URL=http://localhost:5173 MAILUI_EXTERNAL=1 npm run screenshots
 *
 * Environment variables:
 *   BASE_URL          URL of an already-running demo (with MAILUI_EXTERNAL=1).
 *   MAILUI_EXTERNAL   "1" → don't build/serve; screenshot BASE_URL directly.
 */

import { chromium } from 'playwright'
import { execFileSync } from 'child_process'
import { createServer } from 'http'
import { readFile, mkdir } from 'fs/promises'
import { existsSync } from 'fs'
import { resolve, dirname, join, extname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const ROOT = resolve(__dirname, '..')
const DIST = resolve(ROOT, 'dist')
const OUT_DIR = resolve(ROOT, 'docs', 'screenshots')
const EXTERNAL = process.env.MAILUI_EXTERNAL === '1'

const MIME = {
  '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
  '.png': 'image/png', '.svg': 'image/svg+xml', '.json': 'application/json',
  '.ico': 'image/x-icon', '.woff2': 'font/woff2', '.map': 'application/json',
}

// ── Build the demo SPA (mock-backed) if needed ────────────────────────────
function ensureBuilt() {
  if (existsSync(join(DIST, 'index.html'))) return
  console.log('Building demo SPA (npm run build)…')
  execFileSync('npm', ['run', 'build'], { cwd: ROOT, stdio: 'inherit' })
}

// ── Minimal static file server for dist/ ──────────────────────────────────
function startStaticServer() {
  const server = createServer(async (req, res) => {
    try {
      let p = decodeURIComponent(new URL(req.url, 'http://x').pathname)
      if (p === '/' || p.endsWith('/')) p += 'index.html'
      let file = join(DIST, p)
      if (!file.startsWith(DIST)) { res.writeHead(403).end(); return }
      if (!existsSync(file)) file = join(DIST, 'index.html') // SPA fallback
      const body = await readFile(file)
      res.writeHead(200, { 'Content-Type': MIME[extname(file)] || 'application/octet-stream' })
      res.end(body)
    } catch {
      res.writeHead(404).end('not found')
    }
  })
  return new Promise((res) => {
    server.listen(0, '127.0.0.1', () => res({ server, port: server.address().port }))
  })
}

async function shot(page, name, description) {
  const path = resolve(OUT_DIR, `${name}.png`)
  await page.screenshot({ path, fullPage: false })
  console.log(`  [ok] ${name}.png — ${description}`)
}

async function main() {
  await mkdir(OUT_DIR, { recursive: true })

  let baseUrl = process.env.BASE_URL || ''
  let httpServer = null
  if (!EXTERNAL) {
    ensureBuilt()
    const { server, port } = await startStaticServer()
    httpServer = server
    baseUrl = `http://127.0.0.1:${port}`
    console.log(`Serving demo from ${DIST} at ${baseUrl}`)
  } else {
    console.log(`Using external demo at ${baseUrl}`)
  }

  const browser = await chromium.launch({ headless: true })
  const context = await browser.newContext({
    viewport: { width: 1366, height: 860 },
    colorScheme: 'dark',
    deviceScaleFactor: 2,
  })
  const page = await context.newPage()

  const settle = (ms = 350) => page.waitForTimeout(ms)

  try {
    await page.goto(baseUrl, { waitUntil: 'networkidle' })
    await page.waitForSelector('.vm-row', { timeout: 10000 })
    await settle()

    // ── Inbox: three-pane list (reading pane empty placeholder).
    console.log('\nCapturing: inbox (three-pane)')
    await shot(page, 'inbox', 'Three-pane inbox (rail | list | reading pane)')
    await shot(page, 'mail', 'Three-pane inbox (alias)')

    // ── Thread: open the multi-message roadmap conversation.
    console.log('Capturing: thread')
    await page.getByText('Product roadmap Q3 — feedback welcome').first().click()
    await page.waitForSelector('.vm-msg-body', { timeout: 5000 }).catch(() => {})
    await settle()
    await shot(page, 'thread', 'Conversation view (collapsible thread, latest expanded)')
    await shot(page, 'hero', 'Hero — open conversation in the three-pane view')

    // ── Search: query + results + active-query chip.
    console.log('Capturing: search')
    const search = page.getByLabel('Search mail')
    await search.click()
    await search.fill('roadmap')
    await search.press('Enter')
    await settle()
    await shot(page, 'search', 'Search results with active-query chip')
    await search.fill('')
    await page.getByLabel('Clear search').click().catch(() => {})
    await settle(200)

    // ── Compose: docked composer.
    console.log('Capturing: compose')
    await page.getByRole('button', { name: 'Compose' }).first().click()
    await page.waitForSelector('.vm-compose', { timeout: 5000 })
    await page.getByLabel('Subject').fill('Lunch on Thursday?')
    await page.locator('.vm-ctext').click()
    await page.keyboard.type('Hi Alice — are you free for lunch on Thursday to talk roadmap?')
    // Type a partial recipient last so the autocomplete dropdown reads cleanly.
    await page.getByLabel('To', { exact: true }).click()
    await page.keyboard.type('ali')
    await settle(500)
    await shot(page, 'compose', 'Docked compose with contact autocomplete + rich text')
    await page.getByLabel('Close').first().click().catch(() => {})
    await settle(200)

    // ── Calendar side panel.
    console.log('Capturing: calendar')
    await page.getByRole('button', { name: 'Calendar' }).click()
    await page.waitForSelector('.vm-cal-grid', { timeout: 5000 })
    await settle()
    await shot(page, 'calendar', 'Calendar month view (side panel)')
    await page.getByRole('button', { name: 'Calendar' }).click()

    // ── Contacts side panel.
    console.log('Capturing: contacts')
    await page.getByRole('button', { name: 'Contacts' }).click()
    await page.waitForSelector('.vm-contact-list', { timeout: 5000 })
    await settle()
    await shot(page, 'contacts', 'Contacts list (side panel)')
    await page.getByRole('button', { name: 'Contacts' }).click()

    // ── Settings side panel (light theme preview included).
    console.log('Capturing: settings')
    await page.getByRole('button', { name: 'Settings' }).click()
    await page.waitForSelector('.vm-settings', { timeout: 5000 })
    await settle()
    await shot(page, 'settings', 'Settings: density, reading pane, theme, signature')

    // ── Mobile: single-pane inbox at 390px (reload resets panel state).
    console.log('Capturing: mobile')
    await page.setViewportSize({ width: 390, height: 844 })
    await page.reload({ waitUntil: 'networkidle' })
    await page.waitForSelector('.vm-row', { timeout: 5000 })
    await settle()
    await shot(page, 'mobile', 'Mobile single-pane inbox (≤768px flow)')
  } finally {
    await browser.close()
    if (httpServer) httpServer.close()
  }

  console.log(`\nDone. Screenshots written to ${OUT_DIR}`)
}

main().catch((err) => {
  console.error('\nScreenshotter failed:', err.message)
  process.exit(1)
})
