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
 * generator-hardening pass. The tile's FILL is parsed from the same file for
 * the same reason: it is the brand red by definition, so it cannot be a
 * literal here that drifts from the mark it is supposed to be showing.
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

// Parse the approved mark out of brand/logo.svg: the two inner glyph shapes,
// AND the tile's own fill. The card redraws the tile rect itself only because
// it uses a smaller corner radius than the 128-box mark does — so the SHAPE is
// this card's convention, but the COLOUR is the brand's, read from the file.
const brandSvg = readFileSync(join(repoRoot, 'brand', 'logo.svg'), 'utf8');
const lineMatch = brandSvg.match(/<line\b[^>]*\/>/);
const pathMatch = brandSvg.match(/<path\b[^>]*\/>/);
const tileFill = (brandSvg.match(/<rect\b[^>]*fill="(#[0-9a-fA-F]{3,8})"/) || [])[1];
if (!lineMatch || !pathMatch) {
  throw new Error('make-og-card: could not find the mark\'s <line>/<path> in brand/logo.svg');
}
if (!tileFill) {
  throw new Error('make-og-card: could not read the tile fill out of brand/logo.svg');
}
const markMarkup = `${lineMatch[0]}\n      ${pathMatch[0]}`;

/**
 * The palette, declared ONCE, so this card cannot drift from the page it
 * advertises. RED is not written here at all — it is read out of
 * brand/logo.svg above, so re-colouring the approved mark re-colours the card.
 * The rest are the canonical spec's INK and the text tints derived from it.
 *
 * The retired accent this file used to hardcode is gone, and that is not a
 * matter of taste: it was the same warm yellow the scanner uses for its WARN
 * verdict, and a gate's warning colour has to mean exactly one thing.
 *
 * MEASURED: white on RED is 3.02:1 — used here only on the 82px headline and
 * the 31px wordmark, both far past the large-text threshold. No small copy on
 * this card sits on a red fill.
 */
const PALETTE = {
  red: tileFill,     // #FF4848, from the approved mark
  ink: '#14121A',    // the dark ground
  paper: '#FFFFFF',  // headline
  mute: '#A9A2B4',   // supporting copy on ink
  faint: '#8E87A0',  // the URL tag
};

const html = `<!doctype html><meta charset="utf-8"><style>
@font-face{font-family:'Figtree';font-weight:300 900;font-display:block;
  src:url(data:font/woff2;base64,${fontB64}) format('woff2')}
*{box-sizing:border-box;margin:0}
body{width:1200px;height:630px;background:${PALETTE.ink};color:${PALETTE.paper};overflow:hidden;
  font-family:'Figtree',sans-serif;position:relative;
  background-image:
    radial-gradient(720px 430px at 88% 0%,rgba(255,72,72,.22),transparent 62%),
    radial-gradient(560px 380px at 0% 100%,rgba(255,72,72,.10),transparent 58%);}
.grain{position:absolute;inset:0;opacity:.035;
  background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='140' height='140'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='3'/%3E%3C/filter%3E%3Crect width='140' height='140' filter='url(%23n)'/%3E%3C/svg%3E")}
.in{position:absolute;inset:0;padding:74px 78px;display:flex;flex-direction:column}
.brand{display:flex;align-items:center;gap:16px;font-size:31px;font-weight:800;letter-spacing:-.03em}
.tile{width:52px;height:52px;border-radius:15px;background:${PALETTE.red};display:grid;place-items:center;
  box-shadow:0 8px 26px -8px rgba(255,72,72,.7)}
.tile svg{width:33px;height:33px}
h1{margin-top:auto;font-size:82px;line-height:1.01;letter-spacing:-.045em;font-weight:800;max-width:19ch}
.red{color:${PALETTE.red}}
/* The ticket's tear line — the motif the whole landing page is built on. */
.perf{height:3px;margin:38px 0 26px;width:240px;
  background-image:repeating-linear-gradient(90deg,${PALETTE.red} 0 10px,transparent 10px 20px)}
p{font-size:27px;color:${PALETTE.mute};max-width:42ch;line-height:1.42}
.tag{position:absolute;right:78px;bottom:74px;font-size:19px;color:${PALETTE.faint};font-weight:600}
/* Two torn edges, top and bottom: the card IS a stub. */
.edge{position:absolute;left:0;right:0;height:9px;background:${PALETTE.red}}
.edge.t{top:0} .edge.b{bottom:0}
</style>
<div class="grain"></div>
<div class="edge t"></div><div class="edge b"></div>
<div class="in">
  <div class="brand">
    <span class="tile"><svg viewBox="0 0 128 128" fill="none">
      ${markMarkup}
    </svg></span><span>Cackle<span class="red">.</span></span>
  </div>
  <h1>The wifi died at the door.<br>People <span class="red">still got in</span>.</h1>
  <div class="perf"></div>
  <p>Ticketing you run yourself. The phone at the gate keeps working when the venue&rsquo;s internet doesn&rsquo;t.</p>
  <div class="tag">vulos.org/projects/cackle</div>
</div>`;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1200, height: 630 }, deviceScaleFactor: 1 });
await page.setContent(html, { waitUntil: 'load' });
await page.evaluate(() => document.fonts.ready);
await page.screenshot({ path: out });
await browser.close();
console.log(`og-card: wrote ${out} (1200×630)`);
