#!/usr/bin/env node
/**
 * make-og-card.mjs — render site/assets/og-card.png (1200×630).
 *
 * The landing page's og:image. It is generated rather than hand-drawn so it
 * cannot drift from the page: the headline, the colours and the typeface below
 * are the same ones site/index.html declares, and regenerating after a
 * rebrand is one command instead of a trip through a design tool.
 *
 * Self-contained like the page it advertises — the font is the woff2 this repo
 * already vendors, inlined as a data: URI so the render needs no network.
 *
 * The small brand tile in the card header is PARSED out of brand/logo.svg at
 * run time (not re-declared as a second literal) — this script used to draw
 * its own line/path with different coordinates than the approved mark, which
 * meant every og-card.png shipped a subtly different glyph than the actual
 * favicon. See commit history: fixed alongside the rest of the suite's
 * generator-hardening pass.
 *
 * Usage: node scripts/make-og-card.mjs
 */
import { chromium } from 'playwright';
import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const fontB64 = readFileSync(join(repoRoot, 'site', 'assets', 'fonts', 'figtree-latin.woff2')).toString('base64');
const out = join(repoRoot, 'site', 'assets', 'og-card.png');

// Parse the approved mark's line + path out of brand/logo.svg. The tile's
// background rect (colour/rx) is drawn separately by the card's own CSS
// (`.tile{background:#FF4848;border-radius:15px}`) to match the card's own
// tile-corner-radius convention, so we only need the two inner glyph shapes.
const brandSvg = readFileSync(join(repoRoot, 'brand', 'logo.svg'), 'utf8');
const lineMatch = brandSvg.match(/<line\b[^>]*\/>/);
const pathMatch = brandSvg.match(/<path\b[^>]*\/>/);
if (!lineMatch || !pathMatch) {
  throw new Error('make-og-card: could not find the mark\'s <line>/<path> in brand/logo.svg');
}
const markMarkup = `${lineMatch[0]}\n      ${pathMatch[0]}`;

const html = `<!doctype html><meta charset="utf-8"><style>
@font-face{font-family:'Figtree';font-weight:300 900;font-display:block;
  src:url(data:font/woff2;base64,${fontB64}) format('woff2')}
*{box-sizing:border-box;margin:0}
body{width:1200px;height:630px;background:#0B0B0D;color:#F7F6F4;overflow:hidden;
  font-family:'Figtree',sans-serif;position:relative;
  background-image:
    radial-gradient(700px 420px at 88% 2%,rgba(245,166,35,.20),transparent 62%),
    radial-gradient(560px 380px at 2% 96%,rgba(255,72,72,.13),transparent 58%);}
.grain{position:absolute;inset:0;opacity:.035;
  background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='140' height='140'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='3'/%3E%3C/filter%3E%3Crect width='140' height='140' filter='url(%23n)'/%3E%3C/svg%3E")}
.in{position:absolute;inset:0;padding:74px 78px;display:flex;flex-direction:column}
.brand{display:flex;align-items:center;gap:16px;font-size:31px;font-weight:800;letter-spacing:-.03em}
.tile{width:52px;height:52px;border-radius:15px;background:#FF4848;display:grid;place-items:center}
.tile svg{width:33px;height:33px}
h1{margin-top:auto;font-size:82px;line-height:1.01;letter-spacing:-.045em;font-weight:800;max-width:19ch}
.amber{color:#F5A623}
.perf{height:3px;margin:38px 0 26px;width:210px;
  background-image:repeating-linear-gradient(90deg,#F5A623 0 10px,transparent 10px 20px)}
p{font-size:27px;color:#A6A3AD;max-width:40ch;line-height:1.42}
.tag{position:absolute;right:78px;bottom:74px;font-size:19px;color:#7C7A85;font-weight:600}
</style>
<div class="grain"></div>
<div class="in">
  <div class="brand">
    <span class="tile"><svg viewBox="0 0 128 128" fill="none">
      ${markMarkup}
    </svg></span><span>Cackle<span class="amber">.</span></span>
  </div>
  <h1>The wifi dies.<br>The gate <span class="amber">keeps working</span>.</h1>
  <div class="perf"></div>
  <p>Events and ticketing in one binary you host yourself. A ticket is a signed capability, not a database lookup.</p>
  <div class="tag">vulos.org/projects/cackle</div>
</div>`;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1200, height: 630 }, deviceScaleFactor: 1 });
await page.setContent(html, { waitUntil: 'load' });
await page.evaluate(() => document.fonts.ready);
await page.screenshot({ path: out });
await browser.close();
console.log(`og-card: wrote ${out} (1200×630)`);
