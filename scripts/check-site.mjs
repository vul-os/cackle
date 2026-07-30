#!/usr/bin/env node
/**
 * check-site.mjs — the gate site/ never had.
 *
 * `docs:check` proves the chapters are mirrored and their links resolve, and
 * the screenshotter proves the captures render. Nothing proved the two PAGES:
 * that they load clean, that they fetch nothing from a third party, and — the
 * one that actually bites — that every relative path resolves BOTH at `./` in
 * this repo AND under the deployed base path, because
 * vulos-static/scripts/collect-repo-landings.mjs copies site/ verbatim into
 * `public/projects/cackle/` and only renames the entry file to landing.html.
 * A path that works in one shape and not the other ships broken and silent.
 *
 * There is a third shape, and it is the one that actually gets served: the
 * COLLECTED tree. If a sibling vulos-static checkout has already collected
 * this repo, `--collected` (or its presence, when `--all-shapes` is passed)
 * serves that directory as the wrapper route does and checks it too — which is
 * the only way to catch a link the collector rewrote into a 404.
 *
 * # Two checks here are about claims, not code
 *
 * This product's defining honesty is that a cross-gate double-scan is
 * DETECTED, never PREVENTED — two gates partitioned from each other cannot
 * coordinate, and no merge rule invents coordination. A venue that reads
 * "prevented" on a marketing page plans its staffing around a guarantee that
 * cannot exist. So the landing page is asserted to STATE the limit, and
 * asserted NOT to contain any phrasing that upgrades it. That is a gate on the
 * copy, in the same file that gates the markup, because the copy is the part a
 * customer actually acts on.
 *
 * Usage:
 *   node scripts/check-site.mjs              both path shapes, both pages
 *   node scripts/check-site.mjs --collected  also check the collected tree
 *   node scripts/check-site.mjs --quiet      only failures
 */
import { createServer } from 'node:http';
import { readFile, stat } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { dirname, join, extname, normalize, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const siteDir = join(repoRoot, 'site');
const quiet = process.argv.includes('--quiet');
const wantCollected = process.argv.includes('--collected');

// Playwright is already a devDependency of this repo's root package (the
// screenshotter uses it), so this is a zero-install addition.
const require = createRequire(join(repoRoot, 'package.json'));
let chromium;
try {
  ({ chromium } = require('playwright'));
} catch {
  console.error('check-site: playwright is not installed. Run `npm install` at the repo root, then re-run this.');
  process.exit(1);
}

// The deployed shape. collect-repo-landings.mjs writes public/projects/<slug>/,
// which Astro serves at /projects/cackle/. Root is the repo shape.
const SHAPES = [
  { name: 'repo root', base: '/', dir: siteDir, entry: 'index.html' },
  { name: 'deployed', base: '/projects/cackle/', dir: siteDir, entry: 'index.html' },
];

// The collected tree, if a sibling vulos-static has produced one. Its entry is
// landing.html — the collector removes index.html so it cannot shadow the
// Astro route — so this shape also proves the rename did not orphan a link.
const collectedDir = resolve(repoRoot, '..', 'vulos-static', 'public', 'projects', 'cackle');
if (existsSync(join(collectedDir, 'landing.html'))) {
  if (wantCollected) {
    SHAPES.push({ name: 'collected', base: '/projects/cackle/', dir: collectedDir, entry: 'landing.html' });
  } else if (!quiet) {
    console.log('check-site: a collected tree exists — pass --collected to check it too.\n');
  }
} else if (wantCollected) {
  console.error(`check-site: --collected passed but no landing.html at ${collectedDir}.\n` +
    '  Run `node scripts/collect-repo-landings.mjs` in the vulos-static checkout first.');
  process.exit(1);
}

const BREAKPOINTS = [360, 768, 1440];

// A run that dies early must FAIL, not pass by doing nothing. Every shape
// contributes a fixed number of assertions, so a short count is itself a bug.
const CHECKS_PER_PAGE = 5 + BREAKPOINTS.length; // requests, >=400, console, self-contained, images + breakpoints
const CLAIM_CHECKS = 2;                          // landing only, per shape
const DOCS_CHECKS = 1;                           // docs.html only: rail vs chapter table
const SELF_TESTS = 1;                            // the claim gate proving it still bites
const EXPECTED_CHECKS =
  SELF_TESTS + SHAPES.length * (2 * CHECKS_PER_PAGE + CLAIM_CHECKS + DOCS_CHECKS);

const MIME = {
  '.html': 'text/html; charset=utf-8', '.md': 'text/markdown; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8', '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8', '.json': 'application/json',
  '.svg': 'image/svg+xml', '.png': 'image/png', '.jpg': 'image/jpeg',
  '.woff2': 'font/woff2', '.woff': 'font/woff', '.ico': 'image/x-icon',
  '.webmanifest': 'application/manifest+json', '.txt': 'text/plain; charset=utf-8',
};

/** Serve `dir` under `base`, 404ing anything outside it — as the real route does. */
function serve(base, dir) {
  const server = createServer(async (req, res) => {
    const path = decodeURIComponent(req.url.split('?')[0].split('#')[0]);
    if (!path.startsWith(base)) { res.writeHead(404); res.end('outside base'); return; }
    const file = normalize(join(dir, path.slice(base.length - 1)));
    if (!file.startsWith(normalize(dir))) { res.writeHead(403); res.end(); return; }
    try {
      const s = await stat(file);
      const target = s.isDirectory() ? join(file, 'index.html') : file;
      res.writeHead(200, { 'content-type': MIME[extname(target)] || 'application/octet-stream' });
      res.end(await readFile(target));
    } catch {
      res.writeHead(404, { 'content-type': 'text/plain' });
      res.end('404 ' + path);
    }
  });
  return new Promise((r) => server.listen(0, '127.0.0.1', () => r(server)));
}

/**
 * Subresource origins this page actually reaches for. <a href> is navigation
 * and rel=canonical/og:url are metadata a scraper reads — neither is a fetch,
 * and both are REQUIRED to be absolute, so counting them would make the
 * self-containment rule impossible to satisfy.
 */
function collectExternal() {
  const out = new Set();
  const FETCHED = /^(stylesheet|icon|apple-touch-icon|manifest|preload|prefetch|modulepreload|mask-icon)$/i;
  const push = (v) => {
    let u; try { u = new URL(v, location.href); } catch { return; }
    if (u.origin !== location.origin && u.protocol !== 'data:' && u.protocol !== 'blob:') out.add(u.origin + '  <- ' + v);
  };
  for (const el of document.querySelectorAll('[src]')) push(el.getAttribute('src'));
  for (const el of document.querySelectorAll('[srcset]')) {
    el.getAttribute('srcset').split(',').forEach((s) => push(s.trim().split(/\s+/)[0]));
  }
  for (const el of document.querySelectorAll('link[href]')) {
    if ((el.getAttribute('rel') || '').split(/\s+/).some((r) => FETCHED.test(r))) push(el.getAttribute('href'));
  }
  for (const sheet of document.styleSheets) {
    let rules; try { rules = sheet.cssRules; } catch { continue; }
    const scan = (rs) => {
      for (const r of rs) {
        if (r.cssRules) scan(r.cssRules);
        for (const m of (r.cssText || '').matchAll(/url\((['"]?)([^'")]+)\1\)/g)) push(m[2]);
      }
    };
    scan(rules);
  }
  return [...out];
}

/** Every <img> that is in the DOM must have actually decoded. */
function brokenImages() {
  return [...document.querySelectorAll('img')]
    .filter((i) => !i.complete || i.naturalWidth === 0)
    .map((i) => i.getAttribute('src') || '(no src)');
}

/**
 * Phrasings that would upgrade "detected" into "prevented". Matched against
 * the rendered TEXT, so a claim is caught wherever it is written — body copy,
 * a status chip, a heading.
 */
const FORBIDDEN_CLAIMS = [
  /prevents?\s+(?:a\s+|the\s+)?(?:cross-gate\s+)?(?:double|duplicate)[- ]?(?:scan|admission|entry)/i,
  /(?:double|duplicate)[- ]?(?:scan|admission|entry)\s+(?:is\s+)?prevented/i,
  /(?:stops?|blocks?)\s+(?:a\s+|the\s+)?(?:cross-gate\s+)?(?:double|duplicate)[- ]?(?:scan|admission|entry)/i,
  /guarantees?\s+(?:that\s+)?(?:no|nobody)\s+(?:one\s+)?(?:can\s+)?(?:enters?|gets? in)\s+twice/i,
  /impossible\s+to\s+(?:use|scan|reuse)\s+(?:a\s+)?ticket\s+twice/i,
];

/**
 * The page is REQUIRED to discuss prevention — it has to name the limit to
 * warn anyone about it — so a bare regex hit is not evidence of a false claim.
 * "double-scan is detected, not prevented" and a status row reading
 * "Cross-gate double-scan prevented · Never · two partitioned gates cannot
 * coordinate" both match the patterns above and are exactly the copy we want.
 *
 * What separates an honest mention from a claim is a NEGATOR, and the scope
 * that has to hold it is the SENTENCE the match sits in — not a character
 * window around it. A window was tried first and was worse than useless: at
 * ±200 characters, "Cackle prevents cross-gate double-scan" planted in the
 * hero was forgiven by the word "doesn't" three sentences later, in unrelated
 * copy. A guard that can be satisfied by a neighbour is not a guard.
 *
 * Sentence scope holds because the page's honest phrasings each carry their
 * own denial: "…prevented · Never · Two partitioned gates cannot coordinate…"
 * and "…it does not prevent a double admission either." Both are proven by
 * claimGateSelfTest below, alongside a bald claim that must still fail.
 */
const NEGATOR = /\b(?:not|never|cannot|can't|no|nobody|neither|nor|without|impossible|refus\w*|doesn't|won't|isn't|instead of|rather than)\b/i;

function unnegatedClaims(text) {
  const hits = [];
  // Split on sentence enders, keeping enough context that a label and the
  // clause explaining it stay together (they are not separated by a period).
  const sentences = text.split(/(?<=[.!?])\s+/);
  for (const sentence of sentences) {
    if (NEGATOR.test(sentence)) continue;
    for (const re of FORBIDDEN_CLAIMS) {
      const m = sentence.match(re);
      if (m) hits.push(`${re} → "${m[0]}"`);
    }
  }
  return hits;
}

/**
 * A guard that cannot fire is worse than no guard, because it reports success.
 * These two strings prove the claim gate still bites and still forgives: the
 * first must be caught, the second (the page's own honest phrasing) must not.
 */
function claimGateSelfTest() {
  const bad = [];
  // Must be caught: bald claims, with honest copy elsewhere in the same blob,
  // so a neighbouring denial cannot rescue them.
  const mustCatch = [
    'Cackle prevents cross-gate double-scan across every gate you run.',
    'Every duplicate entry is prevented. Cackle does not hold your money.',
    'Cackle stops double admission at the door. It never phones home.',
  ];
  for (const s of mustCatch) {
    if (unnegatedClaims(s).length === 0) bad.push(`NOT caught: "${s}"`);
  }
  // Must pass: the exact shapes this page actually uses.
  const mustPass = [
    'Cross-gate double-scan is detected, not prevented.',
    'Cross-gate double-scan prevented Never Two partitioned gates cannot coordinate at the moment of a scan, so both admit.',
    'It is not a backup, and it does not prevent a double admission either.',
  ];
  for (const s of mustPass) {
    if (unnegatedClaims(s).length > 0) bad.push(`wrongly flagged: "${s}"`);
  }
  return bad;
}

async function main() {
  if (!existsSync(siteDir)) { console.error(`check-site: no site/ at ${siteDir}`); process.exit(1); }
  const browser = await chromium.launch();
  const problems = [];
  let ran = 0;
  const log = (...a) => { if (!quiet) console.log(...a); };
  const note = (ok, where, msg) => {
    ran++;
    if (ok) log(`  ok    ${msg}`);
    else { problems.push(`${where}: ${msg}`); console.log(`  FAIL  ${msg}`); }
  };

  // Before anything is served: prove the claim gate can still fail. A guard
  // that silently stopped matching would let every later "ok" mean nothing.
  log('claim gate');
  const selfTest = claimGateSelfTest();
  note(selfTest.length === 0, 'claim gate',
    `catches a prevention claim and forgives an honest denial${selfTest.length ? ' — ' + selfTest.join('; ') : ''}`);

  for (const shape of SHAPES) {
    const server = await serve(shape.base, shape.dir);
    const origin = `http://127.0.0.1:${server.address().port}`;
    for (const page of [shape.entry, 'docs.html']) {
      const url = `${origin}${shape.base}${page}`;
      const where = `${page} @ ${shape.name}`;
      log(`\n${page}  (${shape.name}: ${shape.base})`);

      const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
      const pg = await ctx.newPage();
      const failed = [], bad = [], errors = [];
      pg.on('requestfailed', (r) => failed.push(`${r.url()} — ${r.failure()?.errorText}`));
      pg.on('response', (r) => { if (r.status() >= 400) bad.push(`${r.status()} ${r.url()}`); });
      pg.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
      pg.on('pageerror', (e) => errors.push('pageerror: ' + e.message));

      await pg.goto(url, { waitUntil: 'networkidle' });

      // Walk every interactive surface, in both themes. All of it loads
      // lazily, so a broken capture path or a dead handler only shows up if
      // something actually asks for it.
      //
      // The gallery is stepped one tab at a time and AWAITED. Clicking through
      // it in a tight loop reassigns <img src> while the previous capture is
      // still in flight, which cancels that request — a real net::ERR_ABORTED
      // that says nothing about whether the file exists. Waiting for each
      // image to settle is what makes "no failed requests" mean what it says.
      const galCount = await pg.$$eval('#galTabs button', (b) => b.length);
      const stepGallery = async () => {
        for (let i = 0; i < galCount; i++) {
          await pg.evaluate((n) => document.querySelectorAll('#galTabs button')[n].click(), i);
          await pg.waitForFunction(() => {
            const im = document.getElementById('galImg');
            return im && im.complete && im.naturalWidth > 0;
          }, null, { timeout: 10000 }).catch(() => errors.push(`gallery image ${i + 1} never decoded`));
        }
      };
      if (galCount) await stepGallery();
      await pg.evaluate(() => {
        for (let i = 0; i < 10; i++) document.getElementById('scanBtn')?.click();
        document.getElementById('swWifi')?.click();
        document.getElementById('swServer')?.click();
        document.getElementById('scanBtn')?.click();
        const t = document.getElementById('themeBtn'); if (t) t.click();
      });
      await pg.waitForTimeout(700);
      if (galCount) await stepGallery();   // every capture again, other theme

      // Every docs chapter, including the built-in one, has to render.
      if (page === 'docs.html') {
        // The rail and the chapter table are two hand-maintained lists of the
        // same thing, so they drift: `host-pages` was mirrored, given a DOCS
        // entry, and never linked — reachable only by typing the hash, and
        // invisible to a walk that iterates the rail. Compare them first, or
        // every chapter check below silently skips whatever is missing.
        const { rail, docs } = await pg.evaluate(() => ({
          rail: [...document.querySelectorAll('.docs-nav a[data-slug]')].map((a) => a.dataset.slug),
          docs: (typeof DOCS !== 'undefined' ? DOCS : []).map((d) => d.slug),
        }));
        const missing = docs.filter((s) => !rail.includes(s));
        const extra = rail.filter((s) => !docs.includes(s));
        note(missing.length === 0 && extra.length === 0, where,
          `every chapter is linked in the rail${missing.length ? ' — unlinked: ' + missing.join(', ') : ''}` +
          `${extra.length ? ' — links a chapter that does not exist: ' + extra.join(', ') : ''}`);

        const slugs = rail;
        for (const slug of slugs) {
          await pg.evaluate((s) => { location.hash = '#' + s; }, slug);
          await pg.waitForFunction(() => {
            const c = document.getElementById('content');
            return c && c.textContent.trim() !== 'Loading…' && c.textContent.length > 200;
          }, null, { timeout: 8000 }).catch(() => errors.push(`docs chapter "${slug}" never rendered`));
        }
      }
      await pg.evaluate(() => { const t = document.getElementById('themeBtn'); if (t) t.click(); });
      await pg.waitForTimeout(900);

      note(failed.length === 0, where, `no failed requests${failed.length ? ' — ' + failed.join('; ') : ''}`);
      note(bad.length === 0, where, `no response >= 400${bad.length ? ' — ' + bad.join('; ') : ''}`);
      note(errors.length === 0, where, `no console or page error${errors.length ? ' — ' + errors.join('; ') : ''}`);

      const ext = await pg.evaluate(collectExternal);
      note(ext.length === 0, where, `self-contained: no external subresource origin${ext.length ? ' — ' + ext.join('; ') : ''}`);

      const broken = await pg.evaluate(brokenImages);
      note(broken.length === 0, where, `every <img> decoded${broken.length ? ' — ' + broken.join('; ') : ''}`);

      // The claims gate — landing page only; docs.html is a viewer shell whose
      // prose comes from docs/, which has its own link/mirror gate.
      if (page === shape.entry) {
        const text = await pg.evaluate(() => document.body.innerText.replace(/\s+/g, ' '));
        const hits = unnegatedClaims(text);
        note(hits.length === 0, where,
          `never claims cross-gate double-scan is prevented${hits.length ? ' — matched ' + hits.join('; ') : ''}`);
        const states = /detected,?\s+not\s+prevented/i.test(text) || /both gates will open/i.test(text);
        note(states, where, 'states the double-scan limit in plain words');
      }

      for (const w of BREAKPOINTS) {
        await pg.setViewportSize({ width: w, height: 900 });
        await pg.waitForTimeout(280);
        const m = await pg.evaluate(() => ({
          sw: document.documentElement.scrollWidth,
          cw: document.documentElement.clientWidth,
        }));
        note(m.sw <= m.cw, where, `no horizontal scroll @${w} (scrollWidth ${m.sw} <= clientWidth ${m.cw})`);
      }
      await ctx.close();
    }
    server.close();
  }
  await browser.close();

  if (ran !== EXPECTED_CHECKS) {
    problems.push(`only ${ran} of ${EXPECTED_CHECKS} checks ran — the run did not complete`);
  }
  if (problems.length) {
    console.error(`\ncheck-site: ${problems.length} problem${problems.length === 1 ? '' : 's'}:\n` +
      problems.map((p) => `  - ${p}`).join('\n'));
    process.exit(1);
  }
  console.log(`\ncheck-site: ${ran} checks passed — ${relative(repoRoot, siteDir)}/ loads clean, ` +
    `fetches nothing off-origin, resolves at ${SHAPES.map((s) => s.name).join(' + ')}, ` +
    'and never claims a double-scan guarantee it cannot keep.');
}

main().catch((e) => { console.error('check-site:', e); process.exit(1); });
