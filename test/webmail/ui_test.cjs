// Automated webmail UI test: drives the real SPA in headless Chrome and asserts
// DOM behavior across every surface. Exits non-zero if any check fails.
//
//   BASE_URL                 webmail origin (default http://127.0.0.1:18080)
//   PUPPETEER_EXECUTABLE_PATH  Chrome/Chromium binary
//   VULOS_USER / VULOS_PW    login credentials
// Use the full puppeteer (bundled Chrome) when available — that's the Docker
// runner — else puppeteer-core with an explicit Chrome path on the host.
let puppeteer, bundled = true;
try { puppeteer = require("puppeteer"); } catch { puppeteer = require("puppeteer-core"); bundled = false; }

const BASE = process.env.BASE_URL || "http://127.0.0.1:18080";
const CHROME = process.env.PUPPETEER_EXECUTABLE_PATH ||
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const USER = process.env.VULOS_USER || "alice@vulos.to";
const PW = process.env.VULOS_PW || "pw";

const results = [];
const check = (name, ok, detail = "") => {
  results.push(ok);
  console.log(`  [${ok ? "PASS" : "FAIL"}] ${name}${detail ? " — " + detail : ""}`);
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

(async () => {
  const launchOpts = { headless: "new", args: ["--no-sandbox", "--disable-dev-shm-usage"] };
  if (!bundled) launchOpts.executablePath = CHROME;
  const browser = await puppeteer.launch(launchOpts);
  const page = await browser.newPage();
  await page.setViewport({ width: 1440, height: 900 });
  const pageErrors = [];
  page.on("pageerror", (e) => pageErrors.push(e.message));

  await page.goto(BASE, { waitUntil: "networkidle0" });

  // ---- login ----
  await page.waitForSelector("#login-user", { timeout: 8000 });
  await page.type("#login-user", USER);
  await page.type("#login-pass", PW);
  await page.click("#login-btn");
  await page.waitForFunction(() => { const a = document.querySelector("#app"); return a && !a.hidden; }, { timeout: 10000 });
  await page.waitForSelector(".row", { timeout: 10000 }).catch(() => {});
  check("login → app visible", await page.$("#app") != null);

  // ---- brand logo: the new vulos-mail.png mark is served, used as the
  //      brand mark, and set as the favicon ----
  const logo = await page.evaluate(async () => {
    const el = document.querySelector(".logo");
    const bg = el ? getComputedStyle(el).backgroundImage : "";
    const fav = !!document.querySelector('link[rel="icon"][href*="vulos-mail.png"]');
    let png = false;
    try {
      const r = await fetch("./vulos-mail.png");
      if (r.ok) { const b = new Uint8Array(await r.arrayBuffer()); png = b[0] === 0x89 && b[1] === 0x50 && b[2] === 0x4e && b[3] === 0x47; }
    } catch {}
    return { el: !!el, bg: bg.includes("vulos-mail.png"), fav, png };
  });
  check("Vulos Mail logo renders + is the favicon", logo.el && logo.bg && logo.fav && logo.png, JSON.stringify(logo));

  // ---- inbox list ----
  const rowCount = await page.$$eval(".row", (r) => r.length);
  check("inbox rows rendered", rowCount >= 1, `${rowCount} rows`);
  check("every row has a gradient avatar", (await page.$$eval(".row-avatar", (a) => a.length)) === rowCount);
  check("unread dots present", (await page.$$eval(".row-unreaddot", (d) => d.length)) >= 1);
  check("list count shown", (await page.$eval("#list-count", (e) => e.textContent)).length > 0);

  // ---- open + read ----
  await page.click(".row");
  await page.waitForSelector(".read-subject", { timeout: 5000 });
  check("reading pane shows subject", await page.$(".read-subject") != null);
  check("reading pane shows sender + body", (await page.$(".msg-from")) != null && (await page.$(".msg-body")) != null);

  // ---- XSS: a hostile email (script/img-onerror in sender, subject, body) must
  //      render as inert escaped text — no script execution, no injected nodes ----
  const xssClicked = await page.evaluate(() => {
    const r = [...document.querySelectorAll(".row")].find((e) => e.textContent.includes("XSSPROBE"));
    if (r) r.click();
    return !!r;
  });
  await page.waitForSelector(".read-subject", { timeout: 4000 }).catch(() => {});
  await sleep(500);
  const pwned = await page.evaluate(() => window.__pwn === 1);
  const injectedNodes = await page.evaluate(() =>
    document.querySelectorAll(".read-subject script, .read-subject img, .msg-body script, .msg-body img, .msg-from script, .msg-from img").length);
  check("XSS email renders inert (no script/onerror execution)", xssClicked && !pwned, pwned ? "PAYLOAD EXECUTED" : "");
  check("email HTML is escaped (no injected script/img nodes)", injectedNodes === 0, `${injectedNodes} injected nodes`);

  // ---- star toggle ----
  await sleep(400); // let any list re-render (from opening the XSS message) settle
  const starOn = async () => page.$$eval(".row .star.on", (s) => s.length);
  const beforeStar = await starOn();
  await page.hover(".row");
  await sleep(150);
  await page.click(".row .star");
  await sleep(300);
  check("star toggles on a row", (await starOn()) !== beforeStar);

  // ---- search filters ----
  await page.click("#search");
  await page.type("#search", "zzz-no-such-subject-xyz");
  await sleep(400);
  check("search narrows the list", (await page.$$eval(".row", (r) => r.length)) < rowCount);
  await page.click("#search", { clickCount: 3 });
  await page.keyboard.press("Backspace");
  await page.keyboard.press("Escape");
  await sleep(300);

  // ---- command palette (⌘K) ----
  await page.keyboard.down("Meta");
  await page.keyboard.press("KeyK");
  await page.keyboard.up("Meta");
  let cmdkOpen = await page.waitForFunction(() => { const o = document.querySelector("#cmdk"); return o && !o.hidden; }, { timeout: 3000 }).then(() => true).catch(() => false);
  if (!cmdkOpen) { // fall back to Ctrl+K
    await page.keyboard.down("Control"); await page.keyboard.press("KeyK"); await page.keyboard.up("Control");
    cmdkOpen = await page.waitForFunction(() => { const o = document.querySelector("#cmdk"); return o && !o.hidden; }, { timeout: 3000 }).then(() => true).catch(() => false);
  }
  check("command palette opens (⌘K)", cmdkOpen);
  check("command palette lists commands", (await page.$$eval(".cmdk-item", (i) => i.length)) >= 1);
  await page.keyboard.press("Escape");
  await sleep(200);

  // ---- compose ----
  await page.click("#compose-btn");
  await page.waitForSelector(".compose .c-to", { timeout: 3000 });
  await page.type(".compose .c-to", "bob@example.com");
  await page.type(".compose .c-subj", "hello from ui test");
  check("compose dock opens with rich body + send + tools",
    (await page.$(".compose .c-rich")) != null && (await page.$(".compose .c-send")) != null && (await page.$(".compose .ctool")) != null);
  await page.click(".compose .close");
  await sleep(200);

  // helper: close an overlay via its close control and wait until hidden.
  const closeOverlay = async (overlayId, closeSel) => {
    await page.click(closeSel).catch(() => {});
    await page.waitForFunction((id) => { const o = document.querySelector(id); return !o || o.hidden; }, { timeout: 3000 }, overlayId).catch(() => {});
    await sleep(150);
  };

  // ---- contacts ----
  await page.click("#contacts-btn");
  await page.waitForSelector("#contact-form", { timeout: 3000 });
  // Wait for the async contact fetch to settle before adding (else it overwrites).
  await page.waitForFunction(() => { const l = document.querySelector("#contacts-list"); return l && !l.textContent.includes("Loading"); }, { timeout: 5000 }).catch(() => {});
  await page.type("#contact-name", "Test Person");
  await page.type("#contact-email", "tp@example.com");
  await page.focus("#contact-email");
  await page.keyboard.press("Enter"); // submit the form
  const contactOk = await page.waitForFunction(() => [...document.querySelectorAll(".contact")].some((c) => c.textContent.includes("tp@example.com")), { timeout: 5000 }).then(() => true).catch(() => false);
  check("contact added and listed", contactOk);
  await closeOverlay("#contacts", "#contacts-close");

  // ---- calendar (month grid + add) ----
  await page.click("#calendar-btn");
  await page.waitForSelector(".cal-cell", { timeout: 4000 });
  check("calendar month grid renders", (await page.$$eval(".cal-cell", (c) => c.length)) === 42, "42 day cells");
  await page.click("#cal-new");
  await page.waitForSelector("#event-title", { timeout: 2000 });
  await page.type("#event-title", "UI Test Event");
  await page.click("#event-form button[type=submit]");
  await sleep(600);
  await page.click("#cal-view button[data-view=agenda]");
  const evtOk = await page.waitForFunction(() => document.body.innerText.includes("UI Test Event"), { timeout: 5000 }).then(() => true).catch(() => false);
  check("calendar event added (visible in agenda)", evtOk);
  await closeOverlay("#calendar", "#calendar-close");

  // ---- settings ----
  await page.click("#settings-btn");
  await page.waitForSelector("#set-save", { timeout: 3000 });
  check("settings panel opens", (await page.$("#set-sig")) != null && (await page.$("#set-vac")) != null);
  await closeOverlay("#settings", "#set-cancel");

  // ---- multi-select bulk bar ----
  await page.hover(".row");
  await page.click(".row .pick");
  await page.waitForSelector("#selbar", { timeout: 3000 }).catch(() => {});
  check("multi-select shows bulk action bar", (await page.$("#selbar")) != null);

  check("no uncaught JS errors during the run", pageErrors.length === 0, pageErrors.slice(0, 3).join(" | "));

  // ---- self-serve free signup (fresh page): create an account, solve the
  //      Altcha proof-of-work in-browser, and land signed-in ----
  const sp = await browser.newPage();
  await sp.goto(BASE, { waitUntil: "networkidle0" });
  await sp.waitForSelector("#show-signup", { timeout: 8000 });
  await sp.click("#show-signup");
  await sp.waitForFunction(() => { const f = document.querySelector("#signup-form"); return f && !f.hidden; }, { timeout: 3000 });
  await sp.type("#signup-handle", "freebie");
  await sp.type("#signup-pass", "supersecret1");
  await sp.click("#signup-btn");
  const signedUp = await sp.waitForFunction(() => { const a = document.querySelector("#app"); return a && !a.hidden; }, { timeout: 30000 }).then(() => true).catch(() => false);
  check("self-serve free signup creates an account + signs in", signedUp);
  await sp.close();

  await browser.close();
  const passed = results.filter(Boolean).length;
  console.log(`\n=== ${passed}/${results.length} webmail UI checks passed ===`);
  process.exit(passed === results.length ? 0 : 1);
})().catch((e) => { console.error("FATAL:", e); process.exit(1); });
