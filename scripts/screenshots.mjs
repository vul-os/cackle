#!/usr/bin/env node
/**
 * Cackle — Playwright screenshotter
 *
 * Captures every major surface at 1440×900 @2x (retina) into
 * docs/screenshots/<surface>-<theme>.png, in BOTH light and dark, then copies
 * the flagship shot to docs/screenshots/hero.png and writes a generated
 * docs/screenshots/README.md index.
 *
 * Pipeline:
 *   1. Build web/ (vite build -> web/dist), unless already built.
 *   2. Build the Go binary with the frontend embedded (mirrors `make build`:
 *      copy web/dist -> cmd/cackle/dist, `go build -tags embed_frontend`).
 *   3. Boot it with --demo on port 8087, in-memory DB, zero setup required.
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
import { mkdirSync, writeFileSync, existsSync, copyFileSync, rmSync, cpSync, readFileSync } from 'node:fs';
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

const VIEWPORT = { width: 1440, height: 900 };
const THEME_STORAGE_KEYS = ['cackle-ui-theme', 'vite-ui-theme']; // shadcn ThemeProvider storageKey — cover both the current and legacy default

const HERO_SURFACE = 'landing'; // the homepage — first thing anyone sees
const HERO_THEME = 'light';

// ── Surfaces to capture ───────────────────────────────────────────────────────
// `path` is a best-effort guess based on the current route scaffold; `auth`
// means "log in as the demo organiser first"; `discover` resolves a real
// dynamic path from the running API before falling back to `path`.
const SURFACES = [
  { name: 'landing', path: '/', description: 'Landing page' },
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

/** Build web/dist (if missing) and the cackle binary with it embedded. */
function buildBinary() {
  if (!existsSync(path.join(ROOT, 'web', 'dist', 'index.html'))) {
    console.log('  building frontend (web/dist) …');
    execSync('npm ci', { cwd: path.join(ROOT, 'web'), stdio: 'pipe' });
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
  const ctx = { eventSlug: null, eventId: null, ticketId: null };
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

async function makeThemeContext(browser, theme) {
  const ctx = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 2,
    colorScheme: theme,
    locale: 'en-US',
  });
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

async function capture(page, surface, theme, discoveryCtx) {
  console.log(`  → [${theme}] ${surface.name} — ${surface.description}`);
  let url = `${BASE}${surface.path}`;
  if (surface.discover) {
    const resolved = await surface.discover(discoveryCtx).catch(() => null);
    if (resolved) url = `${BASE}${resolved}`;
  }

  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 20_000 });
    if (surface.waitFor) {
      await page.waitForSelector(surface.waitFor, { timeout: 8_000 }).catch(() => {});
    } else {
      await page.waitForLoadState('networkidle', { timeout: 8_000 }).catch(() => {});
    }
    await page.waitForTimeout(surface.settleMs || 800);

    const outPath = path.join(OUT, `${surface.name}-${theme}.png`);
    await page.screenshot({ path: outPath, fullPage: false });
    console.log(`     saved ${path.relative(ROOT, outPath)}`);
    return { name: surface.name, theme, status: 'ok', url };
  } catch (err) {
    console.warn(`     FAILED: ${err.message}`);
    return { name: surface.name, theme, status: 'failed', error: err.message, url };
  }
}

async function main() {
  mkdirSync(OUT, { recursive: true });
  const usingExternal = Boolean(EXTERNAL_URL);

  console.log('\nCackle screenshotter');
  console.log(`  target      : ${BASE}${usingExternal ? ' (external)' : ' (local --demo)'}`);
  console.log(`  output      : ${path.relative(ROOT, OUT)}/`);
  console.log(`  viewport    : 1440×900 @2x (retina), light + dark\n`);

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

  for (const theme of ['light', 'dark']) {
    const context = await makeThemeContext(browser, theme);
    if (SURFACES.some((s) => s.grantCamera)) {
      await context.grantPermissions(['camera']).catch(() => {});
    }
    const page = await context.newPage();
    page.on('console', () => {});
    page.on('pageerror', () => {});

    let loggedIn = false;
    if (SURFACES.some((s) => s.auth)) {
      try {
        await loginViaUI(page);
        loggedIn = true;
        if (!discoveryCtx.ticketId) {
          discoveryCtx.ticketId = await discoverTicket(page);
        }
      } catch (err) {
        console.warn(`  [${theme}] demo login failed (${err.message}) — auth-gated surfaces will show the sign-in page instead`);
      }
    }

    for (const surface of SURFACES) {
      if (surface.auth && !loggedIn) {
        results.push({ name: surface.name, theme, status: 'skipped', error: 'demo login unavailable' });
        continue;
      }
      results.push(await capture(page, surface, theme, discoveryCtx));
    }

    await context.close();
  }

  await browser.close();
  stopServer();
  rmSync(path.join(ROOT, 'cackle-screenshots-bin'), { force: true });

  const ok = results.filter((r) => r.status === 'ok');
  const failed = results.filter((r) => r.status === 'failed');
  const skipped = results.filter((r) => r.status === 'skipped');
  console.log(`\nDone — ${ok.length} captured, ${failed.length} failed, ${skipped.length} skipped`);

  // Identical-capture guard. "N captured, 0 failed" once meant 13 byte-identical
  // copies of the sign-in page, because a silent auth failure sent every
  // auth-gated route to the same redirect. A count is not evidence the shots
  // differ, so check, and say so loudly when they don't.
  const byDigest = new Map();
  for (const r of ok) {
    const file = path.join(OUT, `${r.name}-${r.theme}.png`);
    if (!existsSync(file)) continue;
    const digest = createHash('sha256').update(readFileSync(file)).digest('hex');
    if (!byDigest.has(digest)) byDigest.set(digest, []);
    byDigest.get(digest).push(`${r.name}-${r.theme}`);
  }
  const collisions = [...byDigest.values()].filter((names) => names.length > 1);
  if (collisions.length) {
    console.warn('\n  WARNING: identical screenshots detected — these surfaces rendered the same page:');
    for (const names of collisions) console.warn(`    ${names.join('  ==  ')}`);
    console.warn('  Usually a route that does not exist, or an auth-gated route silently redirecting.');
  } else if (ok.length) {
    console.log('  all captures are distinct');
  }

  // Hero: the single most representative shot, copied to the gallery top.
  const heroSrc = path.join(OUT, `${HERO_SURFACE}-${HERO_THEME}.png`);
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

  const notes = [
    '# docs/screenshots',
    '',
    'Generated by `npm run screenshots` (scripts/screenshots.mjs).',
    'Every surface is captured in **light and dark** at retina (1440×900 @2x)',
    'against `./cackle --demo`. `hero.png` is a copy of the flagship shot',
    `(${HERO_SURFACE}, ${HERO_THEME}) — the Cackle homepage.`,
    '',
    '| File | Surface | Status |',
    '|------|---------|--------|',
    ...results.map((r) => {
      const desc = SURFACES.find((s) => s.name === r.name)?.description ?? r.name;
      const status = r.status === 'ok' ? 'captured' : r.status === 'skipped' ? 'skipped (needs live instance)' : 'failed';
      return `| ${r.name}-${r.theme}.png | ${desc} | ${status} |`;
    }),
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
