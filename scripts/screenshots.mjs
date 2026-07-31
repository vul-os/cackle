#!/usr/bin/env node
/**
 * Cackle — Playwright screenshotter
 *
 * Captures every major surface in BOTH themes at BOTH viewports:
 *
 *   docs/screenshots/<surface>-<theme>.png          desktop, 1440×900 @2x
 *   docs/screenshots/<surface>-<theme>-mobile.png   mobile,   390×844 @2x
 *
 * The DESKTOP names are deliberately bare — they are the historical names and
 * site/index.html, docs/*.md and README.md all reference them by name and
 * theme-swap them at runtime, so renaming them breaks the landing page and the
 * docs at once. Mobile is therefore a SUFFIX on the existing name rather than a
 * new naming scheme, and the suffix goes LAST so `<surface>-<theme>` stays the
 * stable stem both the site's theme-swapper and its viewport-swapper build on.
 *
 * It then copies the flagship shot to docs/screenshots/hero.png and writes a
 * generated docs/screenshots/README.md index. The one exception is
 * `landing` (see SURFACES below): it captures the FULL scrollable page, not
 * just the viewport, so the flagship hero.png shot actually shows the
 * demo events listed beneath the marketing hero, not just the hero alone.
 *
 * Pipeline:
 *   1. Build web/ (vite build -> web/dist), unless already built.
 *   2. Build the Go binary with the frontend embedded (mirrors `make build`:
 *      copy web/dist -> cmd/cackle/dist, `go build -tags embed_frontend`).
 *   3. Boot it with --demo on port 8087, in-memory DB, zero setup required.
 *      The binary is rebuilt from the CURRENT tree on every run — a stale
 *      binary left over from an earlier session photographs an app nobody has
 *      any more, and looks entirely plausible while doing it.
 *   4. Log in as the seeded demo organiser (see DEMO_EMAIL/DEMO_PASSWORD
 *      below) via the real sign-in form, so organiser-only surfaces render
 *      with real session cookies — no guessing at localStorage token keys.
 *   5. Discover real demo IDs (event slug, ticket id) from the running API
 *      instead of hardcoding them, so this script survives seed-data changes.
 *   6. Shoot every surface below, light then dark, tolerating any single
 *      surface failing (never let one bad route sink the whole run).
 *
 * If the app cannot boot at all (build fails, binary never becomes
 * reachable), this writes an explanatory docs/screenshots/README.md and
 * EXITS 0 — a broken --demo mode must never fail unrelated CI.
 *
 * Usage:
 *   npm run screenshots
 *   BASE_URL=https://cackle.example.com npm run screenshots   (skip local boot)
 *
 * Prerequisites (local mode):
 *   go 1.25, node 22, `npx playwright install chromium`
 *
 * Coordination note for whoever owns internal/demo:
 *   This script assumes --demo seeds an organiser account reachable at
 *   DEMO_EMAIL / DEMO_PASSWORD below (overridable via CACKLE_DEMO_EMAIL /
 *   CACKLE_DEMO_PASSWORD) that owns at least one published event with at
 *   least one ticket type and one issued ticket, so ticket-qr / event-editor
 *   / ticket-types / attendees / stats have something real to render. If
 *   that contract doesn't match yet, the affected surfaces are still
 *   attempted and marked "best-effort" in the generated README rather than
 *   failing the run.
 */

import { chromium } from 'playwright';
import { mkdirSync, writeFileSync, existsSync, copyFileSync, rmSync, cpSync, readFileSync, statSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { spawn, execSync } from 'node:child_process';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');
const OUT = path.join(ROOT, 'docs', 'screenshots');

const EXTERNAL_URL = process.env.BASE_URL;
const LOCAL_PORT = 8087;
const LOCAL_BASE = `http://127.0.0.1:${LOCAL_PORT}`;
const BASE = EXTERNAL_URL ?? LOCAL_BASE;

const DEMO_EMAIL = process.env.CACKLE_DEMO_EMAIL || 'demo@cackle.events';
const DEMO_PASSWORD = process.env.CACKLE_DEMO_PASSWORD || 'demo1234';

// The app's ThemeProvider reads `cackle-ui-theme`. `vite-ui-theme` is the
// shadcn template default this app was scaffolded from and is written too, so a
// stray legacy reader cannot flip a set back to light — getting this key wrong
// silently produced two *light* sets for an earlier run, with no error anywhere.
const THEME_STORAGE_KEYS = ['cackle-ui-theme', 'vite-ui-theme'];

// Both viewports every surface is shot at. `suffix` is appended AFTER the
// theme, so desktop keeps its historical `<surface>-<theme>.png` name (which
// site/index.html, README.md and docs/*.md all reference literally) and mobile
// is unambiguously `<surface>-<theme>-mobile.png`.
//
// 390×844 is the iPhone 12/13/14 logical viewport — the narrowest width the
// design spec requires this app to work at, and the one the gate scanner is
// actually held at. `isMobile` turns on meta-viewport handling and touch, so
// the capture exercises the same code path a phone does rather than a merely
// narrow desktop window.
const VIEWPORTS = [
  { name: 'desktop', suffix: '', width: 1440, height: 900, isMobile: false, dsf: 2 },
  { name: 'mobile', suffix: '-mobile', width: 390, height: 844, isMobile: true, dsf: 2 },
];

/** The one place a capture's filename is decided. */
function shotName(surface, theme, viewport) {
  return `${surface}-${theme}${viewport.suffix}.png`;
}

const HERO_SURFACE = 'landing'; // the homepage — first thing anyone sees
const HERO_THEME = 'light';

// ── Surfaces to capture ───────────────────────────────────────────────────────
// `path` is a best-effort guess based on the current route scaffold; `auth`
// means "log in as the demo organiser first"; `discover` resolves a real
// dynamic path from the running API before falling back to `path`.
const SURFACES = [
  {
    name: 'landing',
    path: '/',
    description: 'Landing page',
    // This is the flagship shot (copied to hero.png below) — a visitor's
    // very first view of the product. A viewport-only capture only ever
    // showed the marketing hero above the fold; fullPage captures the hero
    // AND the real featured/upcoming events listing beneath it (sourced
    // live from GET /api/events), so the one screenshot most people will
    // ever see actually shows what's on, not just a tagline.
    fullPage: true,
  },
  {
    name: 'event-browse',
    path: '/events',
    description: 'Browse published events',
  },
  {
    name: 'event-detail',
    path: '/events/demo',
    description: 'Event detail — public',
    discover: async (ctx) => (ctx.eventSlug ? `/events/${ctx.eventSlug}` : null),
  },
  {
    name: 'checkout',
    path: '/checkout',
    auth: true,
    seedCart: true,
    description: 'Checkout',
    discover: async (ctx) => (ctx.eventId ? `/checkout/${ctx.eventId}` : null),
  },
  {
    name: 'my-tickets',
    path: '/tickets',
    auth: true,
    description: "Visitor's tickets",
  },
  {
    name: 'ticket-qr',
    path: '/ticket/demo',
    auth: true,
    description: 'Single ticket — offline-verifiable QR capability',
    discover: async (ctx) => (ctx.ticketId ? `/ticket/${ctx.ticketId}` : null),
  },
  {
    name: 'organiser-home',
    path: '/admin',
    auth: true,
    description: 'Organiser home / dashboard',
  },
  {
    name: 'event-editor',
    path: '/admin/events/demo',
    auth: true,
    description: 'Event editor',
    discover: async (ctx) => (ctx.eventId ? `/admin/events/${ctx.eventId}` : null),
  },
  {
    name: 'ticket-types',
    path: '/admin/events/demo/tickets',
    auth: true,
    description: 'Ticket types for an event',
    discover: async (ctx) => (ctx.eventId ? `/admin/events/${ctx.eventId}/tickets` : null),
  },
  {
    name: 'attendees',
    path: '/admin/events/demo/attendees',
    auth: true,
    description: 'Attendee list',
    discover: async (ctx) => (ctx.eventId ? `/admin/events/${ctx.eventId}/attendees` : null),
  },
  {
    name: 'scanner',
    path: '/admin/scanner',
    auth: true,
    description: 'Offline gate scanner — the flagship surface',
    settleMs: 1500,
    grantCamera: true,
  },
  {
    name: 'stats',
    path: '/admin/events/demo/stats',
    auth: true,
    description: 'Event stats (sold / revenue / admitted)',
    discover: async (ctx) => (ctx.eventId ? `/admin/events/${ctx.eventId}/stats` : null),
  },
  {
    name: 'settings',
    path: '/admin/settings',
    auth: true,
    description: 'Organiser settings',
  },
];

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

async function waitForHTTP(url, maxMs = 45_000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const r = await fetch(url, { signal: AbortSignal.timeout(1_500) });
      if (r.status < 600) return true;
    } catch {
      /* not up yet */
    }
    await sleep(500);
  }
  return false;
}

function writeUnbootableReadme(reason) {
  mkdirSync(OUT, { recursive: true });
  const notes = [
    '# docs/screenshots',
    '',
    '**Screenshots were not generated.**',
    '',
    `\`scripts/screenshots.mjs\` could not get Cackle running in demo mode: ${reason}`,
    '',
    'To generate screenshots once the app boots:',
    '',
    '```sh',
    'make build      # builds web/ and the cackle binary with the UI embedded',
    './cackle --demo --addr :8087',
    'npm run screenshots',
    '```',
    '',
    'This file is written automatically (and CI exits 0 despite the failure)',
    'so a broken demo mode never blocks unrelated CI — see scripts/screenshots.mjs.',
    '',
  ].join('\n');
  writeFileSync(path.join(OUT, 'README.md'), notes + '\n');
}

// Wall-clock start, used by the freshness guard at the end: every capture this
// run claims to have taken must have an mtime at or after this.
const RUN_STARTED = Date.now();

let serverProc = null;

function stopServer() {
  if (serverProc) {
    try {
      serverProc.kill('SIGTERM');
    } catch {
      /* already dead */
    }
    serverProc = null;
  }
}

/**
 * Build web/dist and the cackle binary with it embedded.
 *
 * The frontend is rebuilt EVERY run, not just when web/dist is missing. A
 * left-over dist from an earlier session produces a binary that boots happily
 * and photographs an app that no longer exists — the resulting images look
 * entirely plausible and are simply wrong. Set CACKLE_SHOTS_REUSE_DIST=1 to
 * reuse an existing build when you know it is current (iterating on this
 * script, say); it is opt-in precisely because the failure is silent.
 */
function buildBinary() {
  const distIndex = path.join(ROOT, 'web', 'dist', 'index.html');
  const reuse = process.env.CACKLE_SHOTS_REUSE_DIST === '1' && existsSync(distIndex);
  if (reuse) {
    console.log('  reusing existing web/dist (CACKLE_SHOTS_REUSE_DIST=1) — captures are only as fresh as it is');
  } else {
    console.log('  building frontend (web/dist) …');
    if (!existsSync(path.join(ROOT, 'web', 'node_modules'))) {
      execSync('npm ci', { cwd: path.join(ROOT, 'web'), stdio: 'pipe' });
    }
    execSync('npm run build', { cwd: path.join(ROOT, 'web'), stdio: 'pipe' });
  }

  console.log('  building cackle binary (frontend embedded) …');
  const embedDir = path.join(ROOT, 'cmd', 'cackle', 'dist');
  rmSync(embedDir, { recursive: true, force: true });
  cpSync(path.join(ROOT, 'web', 'dist'), embedDir, { recursive: true });
  try {
    execSync(
      `go build -tags embed_frontend -o "${path.join(ROOT, 'cackle-screenshots-bin')}" ./cmd/cackle`,
      { cwd: ROOT, stdio: 'pipe', env: { ...process.env, CGO_ENABLED: '0' } },
    );
  } finally {
    rmSync(embedDir, { recursive: true, force: true });
  }
}

async function startLocalServer() {
  buildBinary();

  const bin = path.join(ROOT, 'cackle-screenshots-bin');
  console.log(`  starting cackle --demo on :${LOCAL_PORT} …`);
  serverProc = spawn(bin, ['--demo', '--addr', `:${LOCAL_PORT}`], {
    cwd: ROOT,
    env: { ...process.env, CACKLE_DB: ':memory:' },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  serverProc.stdout.on('data', (d) => process.stdout.write(`  [cackle] ${d}`));
  serverProc.stderr.on('data', (d) => process.stdout.write(`  [cackle] ${d}`));

  const up = await waitForHTTP(`${LOCAL_BASE}/`, 45_000);
  if (!up) throw new Error(`server never became reachable at ${LOCAL_BASE}`);
  console.log(`  server ready at ${LOCAL_BASE}`);
  await sleep(500);
}

/** Discover real demo IDs from the running API so routes aren't hardcoded. */
async function discoverContext() {
  const ctx = { eventSlug: null, eventId: null, ticketId: null, cartSeed: null };
  try {
    const res = await fetch(`${BASE}/api/events`);
    if (res.ok) {
      const data = await res.json();
      const list = Array.isArray(data) ? data : data.events || data.data || [];
      if (list.length > 0) {
        ctx.eventSlug = list[0].slug || list[0].id || null;
        ctx.eventId = list[0].id || null;
      }
    }
    // Seed a realistic single-ticket cart so the checkout surface renders the
    // actual checkout (billing form + order summary) instead of the
    // empty-cart state. Cackle's cart is pure client state in localStorage
    // (cackle_cart_v1) — a list of {ticket_type_id, ticket_type, event,
    // quantity}, exactly what use-cart's ADD_ITEM stores. The PUBLIC storefront
    // response GET /api/events/{slug} carries {event, ticket_types, ...}
    // together (the per-event ticket-types endpoint is auth-gated); read it
    // the same way the storefront event page does, so the seed matches a real
    // add-to-cart and survives seed-data changes.
    if (ctx.eventSlug) {
      const evRes = await fetch(`${BASE}/api/events/${encodeURIComponent(ctx.eventSlug)}`);
      if (evRes.ok) {
        const data = await evRes.json();
        const event = data?.event || data;
        const tts = data?.ticket_types || event?.ticket_types || [];
        const tt = tts.find((t) => t?.status === 'active') || tts[0];
        if (event?.id && tt?.id) {
          ctx.cartSeed = [{ ticket_type_id: tt.id, ticket_type: tt, event, quantity: 1 }];
        }
      }
    }
  } catch {
    /* API not up yet / not built — surfaces fall back to guessed paths */
  }
  return ctx;
}

async function discoverTicket(page) {
  try {
    const res = await page.request.get(`${BASE}/api/tickets`);
    if (res.ok()) {
      const data = await res.json();
      const list = Array.isArray(data) ? data : data.tickets || data.data || [];
      if (list.length > 0) return list[0].id || null;
    }
  } catch {
    /* ignore */
  }
  return null;
}

/** Log in as the demo organiser via the real sign-in form (real cookies, no key-guessing). */
async function loginViaUI(page) {
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded', timeout: 15_000 });
  const email = page.locator('input[type="email"], input[name="email"]').first();
  const password = page.locator('input[type="password"], input[name="password"]').first();
  if ((await email.count()) === 0 || (await password.count()) === 0) {
    throw new Error('sign-in form not found at /login (input[type=email]/[type=password])');
  }
  await email.fill(DEMO_EMAIL);
  await password.fill(DEMO_PASSWORD);

  // Click the real submit button; pressing Enter does not reliably submit
  // every form and previously failed silently.
  const submit = page
    .locator('button[type="submit"], form button:has-text("Sign In")')
    .first();
  if (await submit.count()) {
    await submit.click();
  } else {
    await page.keyboard.press('Enter');
  }

  await page.waitForLoadState('networkidle', { timeout: 10_000 }).catch(() => {});
  await sleep(500);

  // VERIFY. Without this the run happily screenshots 13 copies of the
  // sign-in page and reports "0 failed" — which is exactly what it did.
  const authed = await page.evaluate(async () => {
    try {
      // The app authenticates with a Bearer token from localStorage, not a
      // cookie — verify the way the app itself does.
      const tok = localStorage.getItem('cackle_token');
      if (!tok) return 0;
      const res = await fetch('/api/auth/me', {
        credentials: 'include',
        headers: { Authorization: `Bearer ${tok}` },
      });
      return res.status;
    } catch {
      return 0;
    }
  });
  if (authed !== 200) {
    throw new Error(
      `demo login did not authenticate (GET /api/auth/me -> ${authed}); ` +
        `check DEMO_EMAIL/DEMO_PASSWORD and that the token is sent on same-origin requests`,
    );
  }
}

async function makeThemeContext(browser, theme, viewport) {
  const ctx = await browser.newContext({
    viewport: { width: viewport.width, height: viewport.height },
    deviceScaleFactor: viewport.dsf,
    colorScheme: theme,
    isMobile: viewport.isMobile,
    hasTouch: viewport.isMobile,
    locale: 'en-US',
  });
  // ONE argument. `addInitScript(fn, a, b)` silently drops everything after the
  // first — an entire "dark" sweep once ran against the light theme because of
  // exactly that, with no error and plausible-looking output. Pass an object.
  await ctx.addInitScript(
    ({ keys, t }) => {
      try {
        for (const k of keys) localStorage.setItem(k, t);
      } catch {
        /* localStorage unavailable pre-navigation on about:blank in some engines */
      }
    },
    { keys: THEME_STORAGE_KEYS, t: theme },
  );
  return ctx;
}

/**
 * Walk the page top-to-bottom in viewport-sized steps, then back to the top.
 *
 * A single programmatic `scrollTo(0, bottom)` runs faster than the compositor
 * delivers IntersectionObserver callbacks, so reveal-on-scroll content never
 * becomes visible and the capture is of an empty page — which is a capture
 * artifact, not a bug in the app, and has been mistaken for one before. Lazy
 * `loading="lazy"` images have the same failure mode. Stepping with a wait per
 * step gives both a chance to fire; ending back at 0 means a fullPage shot
 * starts where a reader would.
 */
async function scrollThrough(page) {
  await page.evaluate(async () => {
    const step = Math.max(200, Math.round(window.innerHeight * 0.75));
    const wait = () => new Promise((r) => setTimeout(r, 120));
    for (let y = 0; y < document.body.scrollHeight; y += step) {
      window.scrollTo(0, y);
      await wait();
    }
    window.scrollTo(0, document.body.scrollHeight);
    await wait();
    window.scrollTo(0, 0);
    await wait();
  });
  // Anything that started decoding during the walk gets a moment to finish.
  await page.evaluate(() => Promise.all(
    [...document.images].filter((i) => !i.complete).map((i) => i.decode().catch(() => {})),
  )).catch(() => {});
  await page.waitForTimeout(250);
}

async function capture(page, surface, theme, viewport, discoveryCtx, pageIssues = []) {
  console.log(`  → [${theme}/${viewport.name}] ${surface.name} — ${surface.description}`);
  const issuesBefore = pageIssues.length;
  let url = `${BASE}${surface.path}`;
  if (surface.discover) {
    const resolved = await surface.discover(discoveryCtx).catch(() => null);
    if (resolved) url = `${BASE}${resolved}`;
  }

  try {
    // Seed the client-side cart just for this surface (the page persists
    // across surfaces on the same origin, so a previous surface already put
    // us on the cackle origin where localStorage is writable). Cleared in
    // `finally` so no later surface renders a stray cart-count badge.
    if (surface.seedCart && discoveryCtx.cartSeed) {
      await page
        .evaluate((seed) => localStorage.setItem('cackle_cart_v1', JSON.stringify(seed)), discoveryCtx.cartSeed)
        .catch(() => {});
    }

    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 20_000 });
    if (surface.waitFor) {
      await page.waitForSelector(surface.waitFor, { timeout: 8_000 }).catch(() => {});
    } else {
      await page.waitForLoadState('networkidle', { timeout: 8_000 }).catch(() => {});
    }
    await page.waitForTimeout(surface.settleMs || 800);
    await scrollThrough(page).catch(() => {});

    // Park the pointer in the corner. Playwright's virtual mouse STAYS where it
    // last clicked — which is the sign-in button — and :hover is recomputed at
    // that coordinate on every subsequent page. That put a hovered, lifted card
    // with a red title in the middle of the events grid in every browse capture,
    // at a different card per viewport because the button is at a different
    // coordinate. It looks exactly like a deliberate highlight, and it is not
    // one; it is the harness photographing its own cursor.
    await page.mouse.move(0, 0).catch(() => {});
    await page.waitForTimeout(150);

    // Assert the page actually LAID OUT at the width we asked for. With
    // `isMobile: true`, Chromium honours the page's own <meta name=viewport>;
    // a document without `width=device-width` falls back to a 980px layout
    // viewport and the "390px mobile" capture is really a shrunk 980px desktop
    // one — measured, not assumed: a page with no meta viewport reports
    // innerWidth 980 under exactly this context config. Nothing errors, the
    // file is written, and it looks like a phone until you read the text.
    const laidOutAt = await page.evaluate(() => document.documentElement.clientWidth).catch(() => null);
    const widthOk = laidOutAt !== null && laidOutAt <= viewport.width && laidOutAt >= viewport.width - 24;
    if (!widthOk) {
      console.warn(`     WARNING: laid out at ${laidOutAt}px, expected ~${viewport.width}px (missing <meta name=viewport>?)`);
    }

    const outPath = path.join(OUT, shotName(surface.name, theme, viewport));
    await page.screenshot({ path: outPath, fullPage: Boolean(surface.fullPage) });
    const bytes = statSync(outPath).size;
    console.log(`     saved ${path.relative(ROOT, outPath)} (${(bytes / 1024).toFixed(0)} KB)`);
    return { name: surface.name, theme, viewport: viewport.name, status: 'ok', url, bytes, laidOutAt, widthOk, issues: pageIssues.slice(issuesBefore) };
  } catch (err) {
    console.warn(`     FAILED: ${err.message}`);
    return { name: surface.name, theme, viewport: viewport.name, status: 'failed', error: err.message, url, issues: pageIssues.slice(issuesBefore) };
  } finally {
    if (surface.seedCart) {
      await page.evaluate(() => localStorage.removeItem('cackle_cart_v1')).catch(() => {});
    }
  }
}

async function main() {
  mkdirSync(OUT, { recursive: true });
  const usingExternal = Boolean(EXTERNAL_URL);

  console.log('\nCackle screenshotter');
  console.log(`  target      : ${BASE}${usingExternal ? ' (external)' : ' (local --demo)'}`);
  console.log(`  output      : ${path.relative(ROOT, OUT)}/`);
  console.log(`  viewports   : ${VIEWPORTS.map((v) => `${v.name} ${v.width}×${v.height} @${v.dsf}x`).join(', ')}`);
  console.log(`  themes      : light + dark`);
  console.log(`  captures    : ${SURFACES.length} surfaces × 2 themes × ${VIEWPORTS.length} viewports = ${SURFACES.length * 2 * VIEWPORTS.length}\n`);

  if (!usingExternal) {
    try {
      await startLocalServer();
    } catch (err) {
      console.error(`  could not boot Cackle in demo mode: ${err.message}`);
      stopServer();
      writeUnbootableReadme(err.message);
      process.exit(0); // never break CI over a broken demo mode
      return;
    }
  } else {
    const up = await waitForHTTP(`${BASE}/`, 15_000);
    if (!up) {
      writeUnbootableReadme(`${BASE} is not reachable`);
      process.exit(0);
      return;
    }
  }

  let discoveryCtx = { eventSlug: null, eventId: null, ticketId: null };
  try {
    discoveryCtx = await discoverContext();
  } catch {
    /* fall back to guessed paths */
  }

  const browser = await chromium.launch({ headless: true });
  const results = [];

  const combos = [];
  for (const theme of ['light', 'dark']) for (const viewport of VIEWPORTS) combos.push({ theme, viewport });

  for (const { theme, viewport } of combos) {
    console.log(`\n  ── ${theme} · ${viewport.name} (${viewport.width}×${viewport.height} @${viewport.dsf}x) ──`);
    const context = await makeThemeContext(browser, theme, viewport);
    if (SURFACES.some((s) => s.grantCamera)) {
      await context.grantPermissions(['camera']).catch(() => {});
    }
    const page = await context.newPage();
    // Capture uncaught exceptions and console errors so a screenshot of a page
    // that is actually throwing cannot masquerade as a healthy one. Buffered
    // per page (shared across this theme's surfaces); capture() slices out the
    // window belonging to each surface. Non-error console output is ignored.
    const pageIssues = [];
    page.on('pageerror', (err) => pageIssues.push({ type: 'pageerror', text: String(err?.message ?? err) }));
    page.on('console', (msg) => {
      if (msg.type() === 'error') pageIssues.push({ type: 'console.error', text: msg.text() });
    });

    let loggedIn = false;
    if (SURFACES.some((s) => s.auth)) {
      try {
        await loginViaUI(page);
        loggedIn = true;
        if (!discoveryCtx.ticketId) {
          discoveryCtx.ticketId = await discoverTicket(page);
        }
      } catch (err) {
        console.warn(`  [${theme}/${viewport.name}] demo login failed (${err.message}) — auth-gated surfaces will show the sign-in page instead`);
      }
    }

    for (const surface of SURFACES) {
      if (surface.auth && !loggedIn) {
        results.push({ name: surface.name, theme, viewport: viewport.name, status: 'skipped', error: 'demo login unavailable' });
        continue;
      }
      results.push(await capture(page, surface, theme, viewport, discoveryCtx, pageIssues));
    }

    await context.close();
  }

  await browser.close();
  stopServer();
  rmSync(path.join(ROOT, 'cackle-screenshots-bin'), { force: true });

  const ok = results.filter((r) => r.status === 'ok');
  const failed = results.filter((r) => r.status === 'failed');
  const skipped = results.filter((r) => r.status === 'skipped');
  const expected = SURFACES.length * 2 * VIEWPORTS.length;
  console.log(
    `\nDone — ${ok.length} captured, ${failed.length} failed, ${skipped.length} skipped ` +
      `(expected ${expected} = ${SURFACES.length} surfaces × 2 themes × ${VIEWPORTS.length} viewports)`,
  );

  // Freshness guard. This script exits 0 by design when the app cannot boot, so
  // a green exit is not evidence that anything was written — and a directory
  // full of yesterday's PNGs looks exactly like a directory full of today's.
  // Assert every expected file exists and was written by THIS run.
  const stale = [];
  for (const theme of ['light', 'dark']) {
    for (const viewport of VIEWPORTS) {
      for (const s of SURFACES) {
        const file = path.join(OUT, shotName(s.name, theme, viewport));
        if (!existsSync(file)) { stale.push(`${path.basename(file)} — MISSING`); continue; }
        const st = statSync(file);
        if (st.mtimeMs < RUN_STARTED) stale.push(`${path.basename(file)} — not rewritten by this run`);
        else if (st.size < 5_000) stale.push(`${path.basename(file)} — only ${st.size} bytes`);
      }
    }
  }
  if (stale.length) {
    console.warn(`\n  WARNING: ${stale.length} capture(s) are missing, stale or suspiciously small:`);
    for (const s of stale) console.warn(`    ${s}`);
  } else {
    console.log(`  all ${expected} captures were written by this run and are non-trivial in size`);
  }

  const wrongWidth = ok.filter((r) => r.widthOk === false);
  if (wrongWidth.length) {
    console.warn(`\n  WARNING: ${wrongWidth.length} capture(s) did not lay out at the requested width:`);
    for (const r of wrongWidth) console.warn(`    ${r.name}-${r.theme} [${r.viewport}] — ${r.laidOutAt}px`);
  } else if (ok.length) {
    console.log('  every capture laid out at its requested viewport width');
  }

  // Identical-capture guard. "N captured, 0 failed" once meant 13 byte-identical
  // copies of the sign-in page, because a silent auth failure sent every
  // auth-gated route to the same redirect. A count is not evidence the shots
  // differ, so check, and say so loudly when they don't. It also catches the
  // other silent failure this file has seen: a light/dark pair coming out
  // byte-identical because the theme never actually flipped.
  const byDigest = new Map();
  for (const r of ok) {
    const viewport = VIEWPORTS.find((v) => v.name === r.viewport) ?? VIEWPORTS[0];
    const file = path.join(OUT, shotName(r.name, r.theme, viewport));
    if (!existsSync(file)) continue;
    const digest = createHash('sha256').update(readFileSync(file)).digest('hex');
    if (!byDigest.has(digest)) byDigest.set(digest, []);
    byDigest.get(digest).push(path.basename(file, '.png'));
  }
  const collisions = [...byDigest.values()].filter((names) => names.length > 1);
  if (collisions.length) {
    console.warn('\n  WARNING: identical screenshots detected — these surfaces rendered the same page:');
    for (const names of collisions) console.warn(`    ${names.join('  ==  ')}`);
    console.warn('  Usually a route that does not exist, or an auth-gated route silently redirecting.');
  } else if (ok.length) {
    console.log('  all captures are distinct');
  }

  // JS-health guard. A saved screenshot proves a page painted, not that it ran
  // cleanly — a route can throw uncaught exceptions or log console errors and
  // still produce a plausible-looking image. Surface those so a broken page is
  // never silently shipped as "captured".
  const withIssues = results.filter((r) => r.issues && r.issues.length);
  if (withIssues.length) {
    const total = withIssues.reduce((n, r) => n + r.issues.length, 0);
    console.warn(`\n  WARNING: ${total} console error(s)/uncaught exception(s) across ${withIssues.length} capture(s):`);
    for (const r of withIssues) {
      console.warn(`    [${r.theme}] ${r.name} — ${r.issues.length}:`);
      for (const it of r.issues.slice(0, 3)) {
        console.warn(`        ${it.type}: ${it.text.replace(/\s+/g, ' ').slice(0, 160)}`);
      }
    }
    console.warn('  A clean-looking screenshot of a page that threw is a false positive.');
  } else if (ok.length) {
    console.log('  no console errors or uncaught exceptions during capture');
  }

  // Hero: the single most representative shot, copied to the gallery top.
  // Desktop — README.md renders it at 900px wide in a GitHub page.
  const heroSrc = path.join(OUT, shotName(HERO_SURFACE, HERO_THEME, VIEWPORTS[0]));
  if (existsSync(heroSrc)) {
    copyFileSync(heroSrc, path.join(OUT, 'hero.png'));
    console.log(`  copied ${HERO_SURFACE}-${HERO_THEME}.png -> hero.png`);
  }

  // Mirror into site/screenshots/. The marketing site is published as a
  // standalone static bundle, so it cannot reach up into docs/ — it needs its
  // own copy beside it or every image 404s once deployed.
  const SITE_OUT = path.join(ROOT, 'site', 'screenshots');
  mkdirSync(SITE_OUT, { recursive: true });
  cpSync(OUT, SITE_OUT, { recursive: true });
  console.log(`  mirrored ${OUT} -> ${SITE_OUT}`);

  const statusOf = (name, theme, viewportName) => {
    const r = results.find((x) => x.name === name && x.theme === theme && x.viewport === viewportName);
    if (!r) return '—';
    return r.status === 'ok' ? 'captured' : r.status === 'skipped' ? 'skipped (needs live instance)' : 'failed';
  };

  const notes = [
    '# docs/screenshots',
    '',
    'Generated by `npm run screenshots` (scripts/screenshots.mjs).',
    'Every surface is captured in **light and dark** at **two viewports** against',
    '`./cackle --demo`:',
    '',
    '- `<surface>-<theme>.png` — desktop, 1440×900 @2x',
    '- `<surface>-<theme>-mobile.png` — mobile, 390×844 @2x (touch, meta-viewport)',
    '',
    'The desktop names carry no viewport suffix on purpose: `site/index.html`,',
    '`README.md` and the docs chapters reference them literally, so the bare name',
    'is the stable one and mobile is a suffix on it.',
    '',
    `\`hero.png\` is a copy of the flagship shot (${HERO_SURFACE}, ${HERO_THEME}, desktop) — the Cackle homepage.`,
    '',
    '| Surface | light | dark | light · mobile | dark · mobile |',
    '|---|---|---|---|---|',
    ...SURFACES.map((s) =>
      `| ${s.description} (\`${s.name}\`) | ${statusOf(s.name, 'light', 'desktop')} | ${statusOf(s.name, 'dark', 'desktop')} | ` +
      `${statusOf(s.name, 'light', 'mobile')} | ${statusOf(s.name, 'dark', 'mobile')} |`),
    '',
    'To regenerate: `npm run screenshots`',
    'Against a live instance: `BASE_URL=https://... npm run screenshots`',
    '',
    '## Notes',
    '',
    '- Organiser-gated surfaces (organiser-home, event-editor, ticket-types,',
    '  attendees, scanner, stats, settings) log in via the real sign-in form as',
    `  \`${DEMO_EMAIL}\` (override with \`CACKLE_DEMO_EMAIL\`/\`CACKLE_DEMO_PASSWORD\`).`,
    '  If that account is missing, those surfaces are skipped rather than',
    '  failing the run.',
    '- `event-detail`, `event-editor`, `ticket-types`, `attendees` and `stats`',
    '  resolve a real seeded event id via `GET /api/events` when available,',
    '  falling back to a guessed path otherwise.',
    '',
  ].join('\n');

  writeFileSync(path.join(OUT, 'README.md'), notes + '\n');
  console.log('  wrote docs/screenshots/README.md\n');

  // Never fail CI over screenshot generation — see file header.
  process.exit(0);
}

main().catch((err) => {
  stopServer();
  console.error('Fatal:', err);
  writeUnbootableReadme(String(err.message || err));
  process.exit(0);
});
