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
 * # The overflow check measures elements, not the root
 *
 * This file used to assert `documentElement.scrollWidth <= clientWidth` and had
 * printed `ok` at every breakpoint for its entire life. It could not do
 * otherwise: index.html's `<section>`s carry `overflow:hidden`, and a clipped
 * subtree adds nothing to any ancestor's scroll area — a 900px element planted
 * inside `section#top` at 390px, and a 160-character unbreakable token 905px
 * past the right edge, both passed. `scrollWidth` is also blind to leftward and
 * `position:fixed` overflow by construction. measureOverflow() compares every
 * rendered element's border box against the viewport instead, forgiving only
 * genuine scroll containers, and overflowGateSelfTest() plants and removes both
 * of those shapes on every run so an `ok` here means something again.
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
import { existsSync, readFileSync } from 'node:fs';
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

// Layout is theme-dependent here: `:root[data-theme=light]` restyles chips and
// the docs rail, and docs.html re-renders the current chapter on a theme
// change. Measuring one theme measures half the page.
const THEMES = ['dark', 'light'];

// The overflow scan's coverage floor, per page, per measurement. The landing
// renders ~730 elements and docs.html ~250 on its last chapter; a selector that
// silently stopped matching, or a page that never finished rendering, would
// report zero offenders out of zero elements and pass. A floor is what makes
// "no element overflows" mean "many elements were looked at and none did".
const MIN_ELEMENTS_MEASURED = { landing: 400, docs: 150 };

// Same idea, for the per-breakpoint image scan below: the landing page has a
// steady 10 <img> elements (chrome logos + the gallery + the 7 themeshots),
// present at every breakpoint and theme. docs.html's own shell markup has
// only 2 (the header tile logo and the footer Vulos logo — everything else
// on that page is chapter content, which docs.html's rail-vs-chapter-table
// check and docs:links already gate, and which varies chapter to chapter: the
// breakpoint sweep below runs on whichever chapter the earlier per-chapter
// walk left the page on, and "changelog" — its last chapter — embeds none).
// A selector that stopped matching (or a chapter that never finished
// rendering) would examine zero images and report zero broken — this floor
// is what makes that impossible to pass silently.
const IMAGE_FLOOR = { landing: 8, docs: 2 };

// A run that dies early must FAIL, not pass by doing nothing. Every shape
// contributes a fixed number of assertions, so a short count is itself a bug.
// Per page: requests, >=400, console, self-contained, images, the overflow
// coverage floor, the image coverage floor, and — per breakpoint per theme —
// one overflow assertion, one "no failed requests", one "no response >= 400"
// and one "every <img> decoded" assertion.
//
// The last three of those four are re-runs of checks that also run once,
// early, at the page's initial viewport. That earlier run is not redundant
// with these: the page swaps <img src> to a per-breakpoint capture file (see
// index.html's setShot()/narrow listener), AFTER the early run already
// fired, and nothing re-asserted the three request/image checks again until
// this loop existed — so a capture missing for exactly one breakpoint (its
// `-mobile` sibling, say) was invisible to "no failed requests" / "no
// response >= 400" / "every <img> decoded" for this file's entire life.
// Proven: moving site/screenshots/attendees-dark-mobile.png aside and
// running this file unmodified still printed "check-site: 57 checks
// passed". Re-running per breakpoint (watermarked against what "no failed
// requests" / "no response >= 400" already reported, so nothing is
// double-counted) is what closes that gap.
const CHECKS_PER_PAGE = 7 + BREAKPOINTS.length * THEMES.length * 4;
const CLAIM_CHECKS = 2;                          // landing only, per shape
const DOCS_CHECKS = 1;                           // docs.html only: rail vs chapter table
const SELF_TESTS = 3;                            // claim gate, brand-mark provenance, overflow gate — all proving they still bite
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

/**
 * Horizontal overflow, measured per element rather than from the root.
 *
 * This replaced `documentElement.scrollWidth <= clientWidth`, which was hollow
 * on this site and provably so. Two independent reasons:
 *
 *  1. site/index.html wraps its content in `<section>`s that carry
 *     `overflow:hidden` (they contain decorative blobs). A clipped subtree
 *     contributes nothing to any ancestor's scroll area, so a 900px element
 *     planted inside `section#top` at a 390px viewport left the root reporting
 *     `scrollWidth 390 <= clientWidth 390` — `ok`. The most realistic defect of
 *     all, an unbreakable 160-character token in a paragraph, rendered 905px
 *     past the right edge and also printed `ok`.
 *  2. `scrollWidth` cannot express LEFTWARD overflow at all. An element at
 *     `left:-400px` is off-screen for every reader and adds nothing to
 *     scrollWidth in an LTR document, so no root-based comparison can ever see
 *     it. Same for `position:fixed`, which is outside the scroll area entirely.
 *
 * So: every rendered element's border box is compared against the viewport, in
 * both directions. Two rules make it honest rather than merely noisy:
 *
 *  - ONLY a genuine scroll container (`overflow-x: auto|scroll`) forgives. Such
 *    a container is an intentional, user-reachable sideways scroller — the
 *    <pre> holding a long shell command, the box holding a base64 signature —
 *    and its content is MEANT to be wider than the viewport. `hidden` and
 *    `clip` are NOT forgiven: they silently destroy content, which is the bug
 *    class this gate exists to find, not an affordance the reader can act on.
 *  - The ancestor walk STOPS BEFORE `<body>`. body carries `overflow-x:hidden`
 *    deliberately, so that a stray element cannot give a real reader a
 *    horizontal scrollbar. Walking into it would mark every element on the page
 *    "legitimately clipped" and rebuild exactly the hollow check being removed.
 *
 * `examined` is returned so the caller can assert a floor: a scan that looked
 * at nothing must fail, not report that all zero elements behaved.
 */
function measureOverflow() {
  const TOL = 0.5; // sub-pixel layout noise, not a real overhang
  const vw = document.documentElement.clientWidth;
  const name = (el) => el.tagName.toLowerCase() +
    (el.id ? '#' + el.id : '') +
    (typeof el.className === 'string' && el.className.trim()
      ? '.' + el.className.trim().split(/\s+/).slice(0, 2).join('.') : '');

  const offenders = [];
  let examined = 0;
  for (const el of document.querySelectorAll('body *')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) continue; // not rendered — nothing to overflow
    examined++;
    if (r.right - vw <= TOL && -r.left <= TOL) continue;

    let scroller = null;
    for (let a = el.parentElement; a && a !== document.body; a = a.parentElement) {
      const ox = getComputedStyle(a).overflowX;
      if (ox === 'auto' || ox === 'scroll') { scroller = name(a); break; }
    }
    if (scroller) continue;

    offenders.push(`${name(el)} spans ${Math.round(r.left)}…${Math.round(r.right)} ` +
      `outside viewport 0…${vw}`);
  }
  return { vw, examined, offenders };
}

/**
 * Proves the overflow scan above can go red, and goes green again when the
 * cause is removed — both directions, on the real page, every run. Without
 * this the scan's "ok" would be worth exactly what the old one's was.
 *
 * The plants are chosen to be the shapes the OLD check could not see: one
 * inside a clipping `<section>`, and one pushed off the left edge.
 */
async function overflowGateSelfTest(pg) {
  const bad = [];
  const baseline = await pg.evaluate(measureOverflow);
  if (baseline.offenders.length !== 0) {
    bad.push(`baseline is not clean (${baseline.offenders.join('; ')}) — a plant proves nothing here`);
  }
  if (baseline.examined < MIN_ELEMENTS_MEASURED.landing) {
    bad.push(`baseline examined only ${baseline.examined} elements`);
  }

  const plants = [
    ['inside a clipping section', () => {
      const host = document.querySelector('section') || document.body;
      const d = document.createElement('div');
      d.id = 'overflow-gate-selftest';
      d.style.cssText = 'width:900px;height:24px';
      host.appendChild(d);
    }],
    ['pushed off the left edge', () => {
      const d = document.createElement('div');
      d.id = 'overflow-gate-selftest';
      d.style.cssText = 'position:absolute;left:-400px;top:0;width:300px;height:24px';
      document.body.appendChild(d);
    }],
  ];
  for (const [label, plant] of plants) {
    await pg.evaluate(plant);
    const red = await pg.evaluate(measureOverflow);
    const named = red.offenders.some((o) => o.includes('#overflow-gate-selftest'));
    if (!named) bad.push(`a planted element ${label} was NOT caught, or was not named`);
    await pg.evaluate(() => document.getElementById('overflow-gate-selftest')?.remove());
    const green = await pg.evaluate(measureOverflow);
    if (green.offenders.length !== 0) {
      bad.push(`removing the plant (${label}) did not clear the scan: ${green.offenders.join('; ')}`);
    }
  }
  return bad;
}

/** Force the page into `want` and wait for it to settle. Returns what it got. */
async function setTheme(pg, want) {
  const current = () => pg.evaluate(() => document.documentElement.getAttribute('data-theme'));
  if (await current() !== want) {
    await pg.evaluate(() => document.getElementById('themeBtn')?.click());
    await pg.waitForTimeout(700); // docs.html re-renders the open chapter on a theme change
  }
  return current();
}

/**
 * Wait for every <img> currently in the DOM to finish loading — successfully
 * or not; `.complete` goes true either way, `brokenImages()` is what tells
 * them apart.
 *
 * This exists because of the same footgun the gallery-stepping code above
 * already names: writing a new `src` while the old request for that element
 * is still in flight cancels it with a genuine `net::ERR_ABORTED`, which is
 * indistinguishable from a real failure to a listener that isn't told which
 * requests were superseded on purpose. The breakpoint sweep below changes
 * viewport AND theme in the same iteration — both of which reassign `<img
 * src>` (see index.html's setShot()) — so without this wait, the NEXT
 * iteration's reassignment routinely aborted the PREVIOUS iteration's still-
 * settling request, and "no failed requests @<bp> <theme>" failed on
 * `checkout-light.png` / `attendees-light.png` / … that were never actually
 * missing, only still loading. Waiting here first is what makes that
 * assertion mean "a request for a file this page asked for did not
 * complete", not "a request this page itself superseded got cancelled".
 */
async function waitForImagesSettled(pg, timeout = 8000) {
  await pg
    .waitForFunction(() => [...document.querySelectorAll('img')].every((i) => i.complete), null, { timeout })
    .catch(() => {});
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

/**
 * Every script that draws the brand mark (site/assets/og-card.png's tile,
 * currently the only one) must PARSE brand/logo.svg's <line>/<path> at run
 * time rather than re-declaring the coordinates as a literal — a hardcoded
 * copy already drifted here once (a stale line y-position and a different
 * smile curve, found and fixed alongside this check). This is a static
 * source-text check, not a render, so it fails closed even if playwright
 * never launches for the rest of the file.
 */
function brandMarkProvenanceCheck() {
  const bad = [];
  const brandSvg = readFileSyncSafe(join(repoRoot, 'brand', 'logo.svg'));
  if (!brandSvg) return ['brand/logo.svg is missing — cannot verify anything derives from it'];
  const linePayload = (brandSvg.match(/<line\b([^>]*)\/>/) || [])[1];
  const pathPayload = (brandSvg.match(/<path\b([^>]*)\/>/) || [])[1];
  if (!linePayload || !pathPayload) return ['brand/logo.svg has no recognisable <line>/<path> mark to check against'];

  const generators = [{ file: join(repoRoot, 'scripts', 'make-og-card.mjs'), label: 'scripts/make-og-card.mjs' }];
  let checked = 0;
  for (const { file, label } of generators) {
    const src = readFileSyncSafe(file);
    if (!src) { bad.push(`${label} is missing`); continue; }
    checked++;
    if (!/brand.*logo\.svg/.test(src)) bad.push(`${label} never reads brand/logo.svg`);
    // The mark's own d="..."/coordinate payload must not appear as a second
    // literal in the generator's source — that would mean it was re-declared
    // rather than parsed out at run time.
    const dMatch = pathPayload.match(/d="([^"]+)"/);
    if (dMatch && src.includes(dMatch[1])) bad.push(`${label} hardcodes the mark's path data instead of parsing it`);
  }
  if (checked === 0) bad.push('found zero brand-mark generators to check — this gate is checking nothing');
  return bad;
}

function readFileSyncSafe(p) {
  try { return readFileSync(p, 'utf8'); } catch { return null; }
}

async function main() {
  if (!existsSync(siteDir)) { console.error(`check-site: no site/ at ${siteDir}`); process.exit(1); }
  const browser = await chromium.launch();
  const problems = [];
  let ran = 0;
  let totalMeasured = 0; // element boxes compared against the viewport, all pages/viewports/themes
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

  log('brand mark provenance');
  const markProblems = brandMarkProvenanceCheck();
  note(markProblems.length === 0, 'brand mark provenance',
    `every mark-drawing generator parses brand/logo.svg instead of re-declaring it${markProblems.length ? ' — ' + markProblems.join('; ') : ''}`);

  // Before any page is measured: prove the overflow scan still bites, on the
  // real landing page, at the narrowest viewport it is asserted at. The check
  // this replaced printed `ok` at every breakpoint for the entire life of the
  // file, so nothing here is trusted until it has been shown to fail on demand.
  log('overflow gate');
  {
    const srv = await serve(SHAPES[0].base, SHAPES[0].dir);
    const ctx = await browser.newContext({ viewport: { width: BREAKPOINTS[0], height: 900 } });
    const pg = await ctx.newPage();
    await pg.goto(`http://127.0.0.1:${srv.address().port}${SHAPES[0].base}${SHAPES[0].entry}`,
      { waitUntil: 'networkidle' });
    await pg.setViewportSize({ width: BREAKPOINTS[0], height: 900 });
    await pg.waitForTimeout(300);
    const overflowProblems = await overflowGateSelfTest(pg);
    note(overflowProblems.length === 0, 'overflow gate',
      `catches a planted element a clipping section hides and one off the left edge, and clears when both are removed` +
      `${overflowProblems.length ? ' — ' + overflowProblems.join('; ') : ''}`);
    await ctx.close();
    srv.close();
  }

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
      const chapterExt = new Set(); // external origins seen in any docs chapter, not just the last one visited
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

        // collectExternal() below only sees whatever chapter is CURRENTLY in
        // the DOM — and the loop's last iteration leaves that on the last
        // slug in `rail`. Without accumulating per-chapter, an external
        // fetch in every OTHER chapter is invisible: a shields.io badge
        // shipped in site/docs/overview.md once passed this exact check for
        // that reason, discovered only by hand. Collect after every chapter
        // renders, not just after the walk ends on whichever slug is last.
        const slugs = rail;
        for (const slug of slugs) {
          await pg.evaluate((s) => { location.hash = '#' + s; }, slug);
          await pg.waitForFunction(() => {
            const c = document.getElementById('content');
            return c && c.textContent.trim() !== 'Loading…' && c.textContent.length > 200;
          }, null, { timeout: 8000 }).catch(() => errors.push(`docs chapter "${slug}" never rendered`));
          for (const u of await pg.evaluate(collectExternal)) chapterExt.add(u);
        }
      }
      await pg.evaluate(() => { const t = document.getElementById('themeBtn'); if (t) t.click(); });
      await pg.waitForTimeout(900);

      note(failed.length === 0, where, `no failed requests${failed.length ? ' — ' + failed.join('; ') : ''}`);
      note(bad.length === 0, where, `no response >= 400${bad.length ? ' — ' + bad.join('; ') : ''}`);
      note(errors.length === 0, where, `no console or page error${errors.length ? ' — ' + errors.join('; ') : ''}`);

      const ext = new Set([...(await pg.evaluate(collectExternal)), ...chapterExt]);
      note(ext.size === 0, where, `self-contained: no external subresource origin${ext.size ? ' — ' + [...ext].join('; ') : ''}`);

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

      const floor = page === shape.entry ? MIN_ELEMENTS_MEASURED.landing : MIN_ELEMENTS_MEASURED.docs;
      const imageFloor = page === shape.entry ? IMAGE_FLOOR.landing : IMAGE_FLOOR.docs;
      let leastExamined = Infinity;
      let leastImagesExamined = Infinity;
      // Watermarks into `failed`/`bad` (see CHECKS_PER_PAGE's comment above):
      // each breakpoint asserts only what accrued SINCE the last one, so a
      // stale failure from an earlier viewport isn't reported six times over.
      let failedWatermark = failed.length;
      let badWatermark = bad.length;
      for (const w of BREAKPOINTS) {
        await pg.setViewportSize({ width: w, height: 900 });
        // Crossing the 700px breakpoint reassigns every themeshot's <img src>
        // (index.html's narrow listener) on its own, independently of the
        // theme loop below — settle THIS wave before the theme loop below
        // fires a second one, or the two reassignments race and abort each
        // other's still-in-flight requests.
        await waitForImagesSettled(pg);
        for (const t of THEMES) {
          const got = await setTheme(pg, t);
          await pg.waitForTimeout(280);
          await waitForImagesSettled(pg); // let this combo's own src swaps finish before judging them

          const m = await pg.evaluate(measureOverflow);
          leastExamined = Math.min(leastExamined, m.examined);
          totalMeasured += m.examined;
          note(m.offenders.length === 0 && got === t, where,
            `nothing overflows @${w} ${t} (${m.examined} elements measured against a ${m.vw}px viewport)` +
            `${got !== t ? ` — page would not switch to ${t}, it is ${got}` : ''}` +
            `${m.offenders.length ? ' — ' + m.offenders.slice(0, 6).join('; ') +
              (m.offenders.length > 6 ? ` (+${m.offenders.length - 6} more)` : '') : ''}`);

          const newFailed = failed.slice(failedWatermark);
          failedWatermark = failed.length;
          note(newFailed.length === 0, where,
            `no failed requests @${w} ${t}${newFailed.length ? ' — ' + newFailed.join('; ') : ''}`);

          const newBad = bad.slice(badWatermark);
          badWatermark = bad.length;
          note(newBad.length === 0, where,
            `no response >= 400 @${w} ${t}${newBad.length ? ' — ' + newBad.join('; ') : ''}`);

          const imgs = await pg.evaluate(() => document.querySelectorAll('img').length);
          const brokenAtBp = await pg.evaluate(brokenImages);
          leastImagesExamined = Math.min(leastImagesExamined, imgs);
          note(brokenAtBp.length === 0, where,
            `every <img> decoded @${w} ${t} (${imgs} images)${brokenAtBp.length ? ' — ' + brokenAtBp.join('; ') : ''}`);
        }
      }
      // A scan that examined nothing would report no offenders and pass.
      note(leastExamined >= floor, where,
        `every overflow scan measured at least ${floor} elements (thinnest was ${leastExamined})`);
      note(leastImagesExamined >= imageFloor, where,
        `every breakpoint's image scan examined at least ${imageFloor} <img> elements (thinnest was ${leastImagesExamined})`);
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
    `keeps ${totalMeasured} element boxes inside the viewport at ` +
    `${BREAKPOINTS.join('/')}px in ${THEMES.join(' + ')}, ` +
    'and never claims a double-scan guarantee it cannot keep.');
}

main().catch((e) => { console.error('check-site:', e); process.exit(1); });
