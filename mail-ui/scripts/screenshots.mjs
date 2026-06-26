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
    viewport: { width: 1280, height: 800 },
    colorScheme: 'dark',
    deviceScaleFactor: 2,
  })
  const page = await context.newPage()

  try {
    await page.goto(baseUrl, { waitUntil: 'networkidle' })

    // ── Mail (default tab): open the first message → three-pane reading view.
    console.log('\nCapturing: mail (three-pane)')
    await page.waitForSelector('.vm-app', { timeout: 10000 })
    const firstRow = await page.$('.vm-row')
    if (firstRow) {
      await firstRow.click()
      await page.waitForSelector('.vm-msg-body', { timeout: 5000 }).catch(() => {})
      await page.waitForTimeout(300)
    }
    await shot(page, 'mail', 'Three-pane mail view (folders | list | reading pane)')
    await shot(page, 'hero', 'Hero — message open in the three-pane view')

    // ── Calendar tab: month grid.
    console.log('\nCapturing: calendar')
    await page.getByRole('button', { name: 'calendar' }).click()
    await page.waitForSelector('.vm-cal-grid', { timeout: 5000 })
    await page.waitForTimeout(300)
    await shot(page, 'calendar', 'Calendar month view')

    // ── Contacts tab: list.
    console.log('\nCapturing: contacts')
    await page.getByRole('button', { name: 'contacts' }).click()
    await page.waitForSelector('.vm-contact-list', { timeout: 5000 })
    await page.waitForTimeout(300)
    await shot(page, 'contacts', 'Contacts list')
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
