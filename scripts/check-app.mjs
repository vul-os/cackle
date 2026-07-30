#!/usr/bin/env node
/**
 * check-app.mjs — the gate `web/src/` never had.
 *
 * # Why this file exists
 *
 * `scripts/check-site.mjs` gates `site/`: it asserts the landing page STATES
 * that a cross-gate double-scan is detected and never prevented, and asserts
 * that nothing on the page upgrades that into a promise. It is a good gate and
 * it works. It also gates exactly the wrong half of the product.
 *
 * `site/` is a marketing page a customer reads once. `web/src/` is the copy
 * that ships INSIDE the binary — what the organiser reads while staffing a
 * door and what the buyer reads while paying. That half was gated by nothing,
 * and the difference showed: `site/index.html` got the honesty exactly right,
 * while `web/src/pages/organizers/pricing.jsx` shipped an invented fee
 * ("Our Fee (0.85%)"), an invented competitive claim, hardcoded ZAR rates for
 * one processor named as though it were the default, and no build-status
 * notice at all. Nothing caught any of it, because nothing was looking.
 *
 * This file looks. It holds `web/src/` to the same standard, and it is a
 * SOURCE-TEXT gate rather than a rendered one on purpose: the console pages
 * live behind auth and behind an org the demo seed has to create, so a
 * headless walk reaches the storefront and stops. Reading the source reaches
 * all 145 files, including copy that is written but not yet linked.
 *
 * # What it asserts
 *
 *  1. The two load-bearing claims exist, verbatim, in one place
 *     (`web/src/components/honesty/claims.js`) and still say what they must.
 *  2. Those claims reach every route — proven structurally, by parsing
 *     `routes.jsx` and showing every `<Route>` descends from a layout that
 *     renders them. A claim mounted per page is a claim that drifts per page.
 *  3. Nothing anywhere in `web/src/` upgrades "detected" into "prevented".
 *  4. No fabricated fee, price or superlative marketing claim.
 *  5. No processor, country or currency presented as the privileged default.
 *  6. No absolute external URL that the app FETCHES at run time.
 *  7. The "there is no billing system in this software" claim is re-verified
 *     against the Go source every run, so if billing ever lands the copy is
 *     forced to change with it.
 *  8. No claim that Cackle discovers organisers, events or peers on its own —
 *     no directory, no proximity search, no global or central feed. There is
 *     no discovery mechanism anywhere in the stack; every peer an operator
 *     sees was enrolled by key, out of band.
 *
 * # Fail-closed
 *
 * Every check contributes to a fixed expected count, and the file glob has a
 * floor. A gate that examines nothing and exits 0 is worse than no gate: it
 * launders an unchecked tree into a green tick. If this run examines fewer
 * files than the floor, or runs fewer checks than expected, it FAILS.
 *
 * Usage:
 *   node scripts/check-app.mjs
 *   node scripts/check-app.mjs --quiet     only failures
 */
import { readFileSync, existsSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const appDir = join(repoRoot, 'web', 'src');
const quiet = process.argv.includes('--quiet');

// The floor. `web/src` holds 151 .js/.jsx files today; a glob that silently
// stops matching, a moved directory or a botched refactor all show up as a
// collapse in this number rather than as a green run over an empty list.
const MIN_FILES_SCANNED = 100;
// routes.jsx declares 36 paths. Same reasoning: a parser that stops finding
// routes must fail, not report that all zero of them are covered.
const MIN_ROUTES = 30;

const CLAIMS_FILE = join(appDir, 'components', 'honesty', 'claims.js');
const STRIP_FILE = join(appDir, 'components', 'honesty', 'honesty-strip.jsx');
const ROUTES_FILE = join(appDir, 'routes.jsx');
const LAYOUTS = [
  join(appDir, 'components', 'layout', 'blank-layout.jsx'),
  join(appDir, 'components', 'layout', 'main-layout.jsx'),
];
const LAYOUT_COMPONENTS = ['BlankLayout', 'MainLayout'];

// ── the claim matchers ──────────────────────────────────────────────────────
//
// Deliberately the same family as check-site.mjs's. They are duplicated here
// rather than imported because that file is a sibling gate with its own
// owner, and a shared import would make one gate's refactor able to silently
// disarm the other. The duplication is not left to drift: `siblingParity()`
// below re-reads check-site.mjs and fails if it grows a pattern this file
// lacks.

const FORBIDDEN_CLAIMS = [
  /prevents?\s+(?:a\s+|the\s+)?(?:cross-gate\s+)?(?:double|duplicate)[- ]?(?:scan|admission|entry)/i,
  /(?:double|duplicate)[- ]?(?:scan|admission|entry)\s+(?:is\s+)?prevented/i,
  /(?:stops?|blocks?)\s+(?:a\s+|the\s+)?(?:cross-gate\s+)?(?:double|duplicate)[- ]?(?:scan|admission|entry)/i,
  /guarantees?\s+(?:that\s+)?(?:no|nobody)\s+(?:one\s+)?(?:can\s+)?(?:enters?|gets? in)\s+twice/i,
  /impossible\s+to\s+(?:use|scan|reuse)\s+(?:a\s+)?ticket\s+twice/i,
];

/**
 * The app is REQUIRED to discuss prevention — it cannot warn anyone about a
 * limit it never names — so a bare regex hit is not evidence of a false
 * claim. What separates an honest mention from a promise is a NEGATOR, and
 * the scope that holds it is the SENTENCE the match sits in. check-site.mjs
 * records why a character window was tried first and abandoned: at ±200
 * characters a planted claim in the hero was forgiven by the word "doesn't"
 * three sentences away, in unrelated copy. A guard a neighbour can satisfy is
 * not a guard.
 *
 * One deliberate divergence from check-site.mjs: "impossible" is NOT a negator
 * here. It is on that file's list, which quietly kills its own fifth pattern —
 * "impossible to use a ticket twice" is a forbidden claim that contains its
 * own escape hatch, so that pattern can never fire there. Proven by the
 * self-test below, which hands the matcher exactly that sentence and requires
 * a catch. Dropping the word costs nothing: no honest sentence in this app
 * needs "impossible" to negate a prevention claim, because every honest
 * sentence about the limit already says "cannot", "never" or "not".
 */
const NEGATOR = /\b(?:not|never|cannot|can't|no|nobody|neither|nor|without|refus\w*|doesn't|won't|isn't|instead of|rather than)\b/i;

// `patterns` defaults to FORBIDDEN_CLAIMS so every existing call site keeps
// working unchanged; DISCOVERY_CLAIMS below reuses the same sentence-scoped
// negation logic instead of duplicating it, because the reasoning — a bare
// regex hit is not evidence of a false claim, a NEGATOR in the same sentence
// is what tells an honest mention from a promise apart — applies to both.
function unnegatedClaims(text, patterns = FORBIDDEN_CLAIMS) {
  const hits = [];
  // Whitespace is collapsed BEFORE splitting, and the reason is specific:
  // check-site.mjs reads rendered innerText, where a sentence is one line;
  // this file reads source, where JSX line-wraps mid-sentence. Splitting on
  // newlines flagged the app's own honest ledger entry — "It is not a backup,
  // and it does not / prevent a double admission either." — because the
  // wrap put the negator on the previous line. Joining first restores the
  // sentence scope the rule is actually about.
  const flat = text.replace(/\s+/g, ' ');
  for (const sentence of flat.split(/(?<=[.!?])\s+/)) {
    if (NEGATOR.test(sentence)) continue;
    for (const re of patterns) {
      const m = sentence.match(re);
      if (m) hits.push(`${m[0]}`);
    }
  }
  return hits;
}

// ── the discovery matchers ──────────────────────────────────────────────────
//
// There is no discovery mechanism anywhere in the stack — no directory, no
// DHT, no rendezvous, no geo index, no search — and none is planned. Peer
// event feeds only show organisers a box operator explicitly enrolled by key,
// out of band. `web/src/pages/visitor/events/peer-scope.test.js` already
// holds this line for `peer-events.jsx`, the one component that renders
// borrowed listings, but nothing held it repo-wide, so a discovery claim
// anywhere else — a marketing blurb on an unrelated settings page, an empty
// state that got creative — would ship unnoticed.
//
// Scoped narrowly on purpose. "near you", "search" and "directory" are all
// ordinary words with innocent uses this app actually has — "events near the
// top of the page", "search these events" (the local search box on a single
// organiser's own listing), a venue's own address field. Each pattern below
// requires the SPECIFIC combination that only means proximity-based or
// central-index discovery, never a bare word alone.
const DISCOVERY_CLAIMS = [
  /\bdiscover(?:s|ing|y)?\s+(?:organisers?|organizers?|events?|peers?)\b/i,
  /\b(?:organisers?|organizers?|events?)\s+(?:near|nearby)\s+you\b/i,
  /\bevents?\s+nearby\b/i,
  /\bnear\s+you\b/i,
  /\bin\s+your\s+area\b/i,
  /\bbrowse\s+(?:organisers?|organizers?|(?:the\s+)?directory)\b/i,
  /\bfind\s+(?:more\s+)?organisers?\b/i,
  /\b(?:global|central|public)\s+(?:feed|directory|index)\b/i,
  /\b(?:organiser|organizer)\s+directory\b/i,
  /\bdirectory\s+of\s+organisers?\b/i,
  /\bsearch\s+(?:for\s+)?(?:organisers?|organizers?|peers?|the\s+directory)\b/i,
];

// ── the money matchers ──────────────────────────────────────────────────────
//
// Cackle has no billing code (check 7 re-proves that against the Go source
// every run), so ANY fee it appears to charge is fabricated by definition.
// These target the shapes the deleted page actually used, plus the generic
// marketing superlative that has no place in a product that is not finished.

const FABRICATED_MONEY = [
  // "Our Fee (0.85%)", "our fee", "platform fee", "service fee"
  { re: /\b(?:our|platform|service|cackle(?:'s|’s)?)\s+fee\b/i, why: 'names a fee Cackle charges — there is none' },
  { re: /\b(?:OUR|PLATFORM|SERVICE)_FEE(?:_RATE)?\b/, why: 'a fee-rate constant for a fee that does not exist' },
  // "we take 0.85%", "a 2% commission", "0.85% cut"
  { re: /\b\d+(?:\.\d+)?\s*%\s*(?:fee|commission|cut|of every|of each)\b/i, why: 'states a percentage Cackle takes' },
  { re: /\bfee\s*\(\s*\d+(?:\.\d+)?\s*%/i, why: 'a labelled percentage fee line item' },
  // Superlatives. Not hedged, not defensible, and unnecessary in either case.
  { re: /\b(?:the\s+)?(?:lowest|cheapest|best|most\s+affordable|market[- ]leading)\s+(?:fees?|prices?|rates?|pricing)\b/i, why: 'unsubstantiated competitive claim about price' },
  { re: /\bin the (?:market|industry)\b/i, why: 'unsubstantiated competitive positioning' },
  { re: /\b(?:industry[- ]leading|world[- ]class|unbeatable|second to none|the (?:only|number one) (?:ticketing|platform))\b/i, why: 'unsubstantiated superlative' },
];

// ── billing-identifier matcher ──────────────────────────────────────────────
//
// Check 7 (the "backing" section below) re-reads the Go source every run and
// fails if it finds a fee-collection identifier, because Cackle has no
// billing code and the app tells organisers so in writing. The regex this
// replaced was `/\b(?:platform_?[Ff]ee|service_?[Ff]ee|application_?[Ff]ee|
// our_?[Ff]ee|[Cc]ommission[Rr]ate)\b/` with no `i` flag: every alternative
// but the commission one is anchored to a lowercase first letter, so it
// missed the exact form EXPORTED Go identifiers take — `PlatformFee`,
// `ApplicationFeeAmount`, `PLATFORM_FEE` — which is precisely the realistic
// case, because a field or constant meant to be read from another package
// has to be exported, and exported means capitalised.
//
// Go identifiers take four shapes: snake_case, SCREAMING_SNAKE, PascalCase
// (exported) and camelCase (unexported). One alternative is built per shape
// per prefix below, and each is anchored so "Fee"/"FEE"/"fee" starts its own
// identifier segment and does not continue into more lowercase letters. That
// second anchor is doing the real work: it is what separates
// `PlatformFeeBps` (segment ends at "Fee", "B" opens a new one — a catch)
// from `PlatformFeedback` ("d" continues the same run — not a fee) and from
// `coffee`/`feed`/`feeds` (no qualifying prefix, and "fee" is mid-word, not
// a segment, in every one of them). The old trailing `\b` could not draw
// that distinction: `\b` fires on any word/non-word transition, not on a
// naming-convention segment start, so it passed straight through the
// difference between "FeeBps" and "Feedback".
const FEE_PREFIXES = ['platform', 'service', 'application', 'our'];
const feeIdentifierAlternatives = FEE_PREFIXES.flatMap((w) => {
  const Cap = w[0].toUpperCase() + w.slice(1);
  const UPPER = w.toUpperCase();
  return [
    `${w}_fee(?:_\\w+)?`,       // platform_fee, platform_fee_bps
    `${UPPER}_FEE(?:_\\w+)?`,   // PLATFORM_FEE, PLATFORM_FEE_BPS
    `${Cap}Fee(?:[A-Z]\\w*)?`,  // PlatformFee, PlatformFeeBps (exported)
    `${w}Fee(?:[A-Z]\\w*)?`,    // applicationFee, applicationFeeAmount (unexported)
  ];
});
// Commission is spelled with "...Rate", not "...Fee", but takes the same
// four shapes.
feeIdentifierAlternatives.push(
  'commission_rate(?:_\\w+)?',
  'COMMISSION_RATE(?:_\\w+)?',
  'CommissionRate(?:[A-Z]\\w*)?',
  'commissionRate(?:[A-Z]\\w*)?',
);
const FEE_IDENTIFIER = new RegExp(`\\b(?:${feeIdentifierAlternatives.join('|')})\\b`);

// ── privileged-default matchers ─────────────────────────────────────────────
//
// README: "there is no privileged country, currency, or processor", and
// cmd/cackle/main.go backs that up — `manual` is always registered, every
// other adapter is opt-in per deployment. Copy that names one processor,
// country or tax regime as the way Cackle works contradicts both.

const PROCESSORS = 'paystack|stripe|payfast|payshap|btcpay|lnbits|flutterwave|mpesa|square|adyen|razorpay';
const PRIVILEGED_DEFAULT = [
  { re: new RegExp(`\\bpowered by\\s+(?:${PROCESSORS})\\b`, 'i'), why: 'names one processor as the product\'s payment rail' },
  { re: new RegExp(`\\b(?:payment|paid|pay|card|checkout)\\s+(?:via|with|through|using|by)\\s+(?:${PROCESSORS})\\b`, 'i'), why: 'presents one processor as how buyers pay' },
  { re: new RegExp(`\\b(?:${PROCESSORS})\\s+is\\s+(?:the\\s+)?(?:default|built[- ]in)\\b`, 'i'), why: 'names a non-default processor as the default' },
  { re: /\bSouth African[- ](?:issued|cards?)\b/i, why: 'a privileged country in payment copy' },
  { re: /\bVAT\s*\(?\s*1[45]\s*%/i, why: 'one jurisdiction\'s tax rate hardcoded into copy' },
  { re: /\bPayShap\b/, why: 'a single-country instant-payment rail named in product copy' },
];

// ── external-fetch matcher ──────────────────────────────────────────────────
//
// §8 of the design spec: "No external fetches. None." The gate runs with no
// internet, so a page that phones home is a lie about the product.
//
// The distinction check-site.mjs draws applies here too and for the same
// reason: an <a href>, a window.open() and a markdown [link](url) are
// NAVIGATION — the user chose to leave — and an SVG xmlns is an identifier
// that is never dereferenced. None of those is the page fetching a third
// party on the user's behalf. Everything else is.
//
// The word boundary in front of `href` is load-bearing and was added because
// this gate's own self-test caught it missing: without it, `const tileHref =
// "https://tile…"` ends in the characters `href = "` and a tile URL walked
// straight through the navigation exemption. A rule that any identifier can
// satisfy by ending in the right five letters is not a rule.
const NAVIGATION_CONTEXT = [
  /(?:^|[\s<{(=])href\s*=\s*["'{]?\s*$/i, // <a href="…", attribution='… <a href="…'
  /window\.open\(\s*[`'"]?\s*$/i,
  /\]\(\s*$/, // markdown [text](url)
  /xmlns(?::\w+)?\s*=\s*["']\s*$/i,
  /location\.href\s*=\s*[`'"]\s*$/i,
];

// ── file collection ─────────────────────────────────────────────────────────

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry.startsWith('.')) continue;
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.(?:js|jsx)$/.test(entry)) out.push(p);
  }
  return out;
}

const rel = (p) => relative(repoRoot, p).split(sep).join('/');
const isTest = (p) => /\.test\.jsx?$/.test(p);

// ── structural checks ───────────────────────────────────────────────────────

/**
 * Parse routes.jsx and report which declared paths descend from a layout that
 * renders the honesty strip. Brace- and quote-aware, because
 * `element={<LandingPage />}` contains a `>` that a naive `[^>]*` split on.
 */
function routeCoverage(src) {
  const stack = [];
  const covered = [];
  const orphaned = [];
  let i = 0;
  while (i < src.length) {
    if (src.startsWith('</Route>', i)) { stack.pop(); i += 8; continue; }
    if (src.startsWith('<Route', i) && !/[A-Za-z0-9_]/.test(src[i + 6] || ' ')) {
      let j = i + 6, depth = 0, quote = null, end = -1;
      while (j < src.length) {
        const c = src[j];
        if (quote) { if (c === quote) quote = null; }
        else if (c === '"' || c === "'" || c === '`') quote = c;
        else if (c === '{') depth++;
        else if (c === '}') depth--;
        else if (c === '>' && depth === 0) { end = j; break; }
        j++;
      }
      if (end === -1) break; // malformed — the count floor below will catch it
      const attrs = src.slice(i + 6, end);
      const selfClosing = src[end - 1] === '/';
      const isLayout = LAYOUT_COMPONENTS.some((c) => new RegExp(`element=\\{\\s*<${c}\\b`).test(attrs));
      const pathMatch = attrs.match(/\bpath\s*=\s*"([^"]*)"/);
      const inLayout = isLayout || stack.some((f) => f);
      if (pathMatch) (inLayout ? covered : orphaned).push(pathMatch[1]);
      if (!selfClosing) stack.push(isLayout || stack.some((f) => f));
      i = end + 1;
      continue;
    }
    i++;
  }
  return { covered, orphaned };
}

/**
 * The sibling gate must not grow a prevention pattern this one lacks. Read
 * check-site.mjs's FORBIDDEN_CLAIMS source text and compare literal for
 * literal — read-only, so the two gates stay in step without either owning
 * the other.
 */
function siblingParity() {
  const src = readFileSyncSafe(join(repoRoot, 'scripts', 'check-site.mjs'));
  if (!src) return ['scripts/check-site.mjs is missing — the site gate it mirrors is gone'];
  const block = src.match(/const FORBIDDEN_CLAIMS = \[([\s\S]*?)\n\];/);
  if (!block) return ['scripts/check-site.mjs no longer declares FORBIDDEN_CLAIMS — the site claim gate was removed or renamed'];
  const theirs = [...block[1].matchAll(/^\s*(\/.*\/[gimsuy]*),\s*$/gm)].map((m) => m[1]);
  if (theirs.length === 0) return ['scripts/check-site.mjs declares an EMPTY FORBIDDEN_CLAIMS — the site claim gate matches nothing'];
  const mine = new Set(FORBIDDEN_CLAIMS.map((r) => r.toString()));
  const missing = theirs.filter((t) => !mine.has(t));
  return missing.map((t) => `check-site.mjs gates a pattern this gate does not: ${t}`);
}

function readFileSyncSafe(p) {
  try { return readFileSync(p, 'utf8'); } catch { return null; }
}

// ── self-tests ──────────────────────────────────────────────────────────────
//
// A guard that cannot fire is worse than no guard, because it reports success.
// Every matcher family below is handed something it MUST catch and something
// it MUST forgive, before a single real file is read.

function claimMatcherSelfTest() {
  const bad = [];
  const mustCatch = [
    'Cackle prevents cross-gate double-scan across every gate you run.',
    'Every duplicate entry is prevented. Cackle does not hold your money.',
    'Cackle stops double admission at the door. It never phones home.',
    'It is impossible to use a ticket twice.',
  ];
  for (const s of mustCatch) if (unnegatedClaims(s).length === 0) bad.push(`claim gate did NOT catch: "${s}"`);
  const mustPass = [
    'Detected, never prevented.',
    'Two doors that are both offline cannot talk to each other. It cannot stop it at the second door.',
    'Cross-gate double-scan is detected, not prevented.',
    'A second scan on the same scanner is refused at the door; nothing prevents a double admission across two offline gates.',
  ];
  for (const s of mustPass) if (unnegatedClaims(s).length > 0) bad.push(`claim gate wrongly flagged: "${s}"`);
  return bad;
}

function discoveryMatcherSelfTest() {
  const bad = [];
  const mustCatch = [
    'Discover organisers near you.',
    'Browse the organiser directory to find your next event.',
    'Find more organisers in your area.',
    'Search for organisers near you.',
    'A global feed of every event on the network.',
  ];
  for (const s of mustCatch) if (unnegatedClaims(s, DISCOVERY_CLAIMS).length === 0) bad.push(`discovery gate did NOT catch: "${s}"`);
  // Every one of these uses a word the matcher keys on — "organiser",
  // "near", "search", "directory" — in a sense that is not discovery.
  const mustPass = [
    'Paste the address and key an organiser gives you.',
    'Events near the top of the page are starting soonest.',
    'Search these events by name or date.',
    'Venue: 45 Bree Street, Cape Town.',
    'Cackle does not discover organisers on its own; you paste in a key someone gives you out of band.',
    'There is no directory, no DHT, no rendezvous, no geo index and no search.',
  ];
  for (const s of mustPass) if (unnegatedClaims(s, DISCOVERY_CLAIMS).length > 0) bad.push(`discovery gate wrongly flagged: "${s}"`);
  return bad;
}

function moneyMatcherSelfTest() {
  const bad = [];
  const mustCatch = [
    ['Our Fee (0.85%)', /fee\s*\(/i],
    ['const OUR_FEE_RATE = 0.0085;', /_FEE/],
    ['The lowest fees in the market', /lowest/i],
    ['We take a 2% commission on every ticket.', /%/],
    ['The industry-leading ticketing platform.', /industry/i],
  ];
  for (const [s] of mustCatch) {
    if (!FABRICATED_MONEY.some(({ re }) => re.test(s))) bad.push(`money gate did NOT catch: "${s}"`);
  }
  const mustPass = [
    'Cackle charges you nothing. There is no billing system in this software.',
    'Cackle takes 0% of what you sell.',
    'No per-ticket cut, no percentage, no monthly plan.',
    'Ticket price', 'Total', 'Fee breakdown is your provider’s business, not ours.',
  ];
  for (const s of mustPass) {
    const hit = FABRICATED_MONEY.find(({ re }) => re.test(s));
    if (hit) bad.push(`money gate wrongly flagged: "${s}" (${hit.re})`);
  }
  return bad;
}

function feeIdentifierMatcherSelfTest() {
  const bad = [];
  // Real shapes an exported or unexported Go identifier actually takes —
  // the exact forms the unanchored, non-`i` predecessor could not catch.
  const mustCatch = [
    'PlatformFee', 'ServiceFee', 'ApplicationFee', 'PlatformFeeBps',
    'applicationFeeAmount', 'PLATFORM_FEE', 'platform_fee', 'OUR_FEE',
    'our_fee', 'CommissionRate', 'commissionRate', 'COMMISSION_RATE',
    'PLATFORM_FEE_BPS', 'ApplicationFeeAmount',
  ];
  for (const s of mustCatch) if (!FEE_IDENTIFIER.test(s)) bad.push(`fee-identifier gate did NOT catch: "${s}"`);
  // "fee" is a substring of every one of these, and none of them is a fee.
  // Each either lacks a qualifying prefix or is the SAME lowercase run
  // continuing past where a real "Fee" segment would end — the distinction
  // a blanket case-insensitive `\bfee\b` cannot draw.
  const mustPass = [
    'feed', 'feeds', 'feedback', 'coffee', 'Feedback',
    'PlatformFeedback', 'applicationFeedback', 'ApplicationFeeder',
    'FeeCollector', // a real word, but not one of the four gated prefixes
    'Cackle charges you nothing. There is no billing system in this software.',
    'no fee is charged here',
  ];
  for (const s of mustPass) if (FEE_IDENTIFIER.test(s)) bad.push(`fee-identifier gate wrongly flagged: "${s}"`);
  return bad;
}

function processorMatcherSelfTest() {
  const bad = [];
  const mustCatch = [
    'Powered by Paystack',
    'Find an event and buy a ticket — card payment via Paystack.',
    'South African-issued cards',
    'Include VAT (15%)',
    'Stripe is the default provider.',
  ];
  for (const s of mustCatch) {
    if (!PRIVILEGED_DEFAULT.some(({ re }) => re.test(s))) bad.push(`processor gate did NOT catch: "${s}"`);
  }
  const mustPass = [
    'manual is the always-on default: no API key, works anywhere.',
    'Optional adapters (Stripe, Paystack, BTCPay and more) are off by default.',
    'The provider bank list shape is not pinned down in the docs.',
    'South African Rand',
  ];
  for (const s of mustPass) {
    const hit = PRIVILEGED_DEFAULT.find(({ re }) => re.test(s));
    if (hit) bad.push(`processor gate wrongly flagged: "${s}" (${hit.re})`);
  }
  return bad;
}

function urlMatcherSelfTest() {
  const bad = [];
  const nav = [
    'href="https://vulos.org"',
    "window.open(`https://www.google.com/maps/search/?api=1&query=${lat},${lng}`, '_blank')",
    '[README](https://github.com/vul-os/cackle) in the repository',
    'xmlns="http://www.w3.org/2000/svg"',
    "attribution='&copy; <a href=\"https://www.openstreetmap.org/copyright\">OpenStreetMap</a>'",
    '// this block used to fetch https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png on every render',
    'const directionsHref = hasCoords ? `https://www.openstreetmap.org/?mlat=${lat}` : null;\n<a href={directionsHref}>Open in maps</a>',
    // A real comment can still name two schemed URLs — the FIRST url's own
    // "//" must not be what makes this a comment; the leading "// " is.
    '// migrated from https://old.example.com/a to https://old.example.com/b, both gone now',
  ];
  for (const s of nav) if (externalFetches(s).length) bad.push(`url gate wrongly flagged navigation: "${s}"`);
  const fetches = [
    'url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"',
    "fetch('https://api.example.com/v1/track')",
    '<img src="https://img.shields.io/badge/build-passing.svg" />',
    "import('https://cdn.jsdelivr.net/npm/mermaid/+esm')",
    // A bound name only escapes if an href actually consumes it. This one is
    // bound and never hrefed, so the variable is no hiding place.
    'const tileHref = "https://tile.example.com/{z}/{x}/{y}.png";\n<TileLayer url={tileHref} />',
    "fetch('https://api.example.com/v1/track') // harmless, honest",
    // The defect this guards: a prior URL's own "https://" scheme on the
    // same line used to be mistaken for a "//" comment marker, so the
    // SECOND url on a line — the one actually reaching `fetch` — escaped
    // entirely. Realistic JSX shape: a legitimate <a href> followed by a
    // real fetch call on the same line.
    'const ok = "https://a.example.com"; fetch("https://evil.example.com/track");',
    '<a href="https://vulos.org">Home</a> && fetch("https://evil.example.com/track")',
  ];
  for (const s of fetches) if (!externalFetches(s).length) bad.push(`url gate did NOT catch a fetch: "${s}"`);
  return bad;
}

/**
 * True if `linePrefix` contains a real `//` line-comment marker — one that is
 * NOT the two slashes of a URL scheme (`http://`, `https://`). A scheme's
 * `//` is always immediately preceded by the scheme's own `:`; a comment
 * marker never is, so that single character of lookbehind is enough to tell
 * them apart without a real tokenizer.
 */
function hasLineComment(linePrefix) {
  let idx = -1;
  while ((idx = linePrefix.indexOf('//', idx + 1)) !== -1) {
    if (linePrefix[idx - 1] !== ':') return true;
  }
  return false;
}

/** Every absolute URL in `src` that is not plainly navigation or an identifier. */
function externalFetches(src) {
  const out = [];
  for (const m of src.matchAll(/https?:\/\/[^\s"'`)<>]+/g)) {
    const before = src.slice(Math.max(0, m.index - 100), m.index);
    if (NAVIGATION_CONTEXT.some((re) => re.test(before))) continue;

    // A URL written into a comment is documentation, not a request — and the
    // comments that matter most here are the ones explaining why a fetch was
    // REMOVED, which would otherwise be punished for naming what they killed.
    // Deliberately one-directional: the comment marker has to come BEFORE the
    // URL on its own line, so `fetch('https://x') // fine` is still caught.
    //
    // `linePrefix.includes('//')` used to be the whole check, and it was
    // hollow: a URL's OWN scheme contains "//", so the first URL on a line
    // poisons every match after it. `const ok = "https://a"; fetch("https:
    // //evil")` had its `fetch` call walk straight through the exemption,
    // because by the time the second URL was reached, `linePrefix` already
    // contained the first URL's "https://". hasLineComment() below looks for
    // a `//` that is NOT the two slashes of a scheme instead: a scheme's `//`
    // is always immediately preceded by the `:` of `http:`/`https:`, and a
    // real `//` comment marker never is.
    const lineStart = src.lastIndexOf('\n', m.index) + 1;
    const linePrefix = src.slice(lineStart, m.index);
    if (hasLineComment(linePrefix) || /^\s*[*]/.test(linePrefix)) continue;

    // A URL bound to a name that the same file then hands to an `href={}` is
    // navigation that happens to travel through a variable — the shape
    // `const directionsHref = ...; <a href={directionsHref}>`. Proving the
    // binding is actually consumed by an href is what keeps this from being
    // a rename-away escape hatch: `url="…"` on a tile layer is bound too, and
    // is still caught, because nothing hrefs it.
    const bound = before.match(/\b([A-Za-z_$][\w$]*)\s*=\s*[^;=]{0,90}$/);
    if (bound && new RegExp(`href\\s*=\\s*\\{[^}]*\\b${bound[1]}\\b`).test(src)) continue;

    out.push(m[0]);
  }
  return out;
}

// ── the run ─────────────────────────────────────────────────────────────────

function main() {
  if (!existsSync(appDir)) { console.error(`check-app: no web/src at ${appDir}`); process.exit(1); }

  const problems = [];
  let ran = 0;
  const log = (...a) => { if (!quiet) console.log(...a); };
  const note = (ok, msg) => {
    ran++;
    if (ok) log(`  ok    ${msg}`);
    else { problems.push(msg); console.log(`  FAIL  ${msg}`); }
  };
  const fail = (where, list) => (list.length ? ` — ${where}: ${list.join('; ')}` : '');

  // 1 ── the matchers themselves, before anything real is read.
  log('matcher self-tests');
  for (const [label, run] of [
    ['claim', claimMatcherSelfTest],
    ['discovery', discoveryMatcherSelfTest],
    ['fabricated-money', moneyMatcherSelfTest],
    ['fee-identifier', feeIdentifierMatcherSelfTest],
    ['privileged-processor', processorMatcherSelfTest],
    ['external-url', urlMatcherSelfTest],
  ]) {
    const bad = run();
    note(bad.length === 0, `${label} matcher catches what it must and forgives what it must${fail('problems', bad)}`);
  }

  const parity = siblingParity();
  note(parity.length === 0, `every prevention pattern check-site.mjs gates is gated here too${fail('drift', parity)}`);

  // 2 ── the claims module: one place, still saying the right thing.
  log('\nclaims module');
  const claims = readFileSyncSafe(CLAIMS_FILE);
  note(claims !== null, `${rel(CLAIMS_FILE)} exists — the app's claims have one home`);
  const claimText = claims || '';
  const constant = (name) => (claimText.match(new RegExp(`export const ${name}\\s*=\\s*([\\s\\S]*?);\\n`)) || [])[1] || '';
  const status = constant('BUILD_STATUS_LABEL') + ' ' + constant('BUILD_STATUS_DETAIL');
  const limit = constant('CROSS_GATE_LIMIT_LABEL') + ' ' + constant('CROSS_GATE_LIMIT_DETAIL');
  const noFee = constant('NO_FEE_STATEMENT');

  note(/experimental/i.test(status) && /not production[- ]ready/i.test(status),
    `the build status still says experimental and not production-ready${status.trim() ? '' : ' — the constants are empty or missing'}`);
  note(/detected,?\s+never\s+prevented/i.test(limit),
    'the cross-gate limit is still stated as DETECTED, NEVER PREVENTED');
  note(/(?:cannot|can(?:'|’)t|can not)\s+stop/i.test(limit) && /second door/i.test(limit),
    'the cross-gate limit still names the actual failure — it cannot stop the second door');
  note(/no billing/i.test(noFee) && /(?:charges you nothing|no fee)/i.test(noFee),
    'the no-fee statement still says Cackle charges nothing and has no billing system');

  // 3 ── those claims reach every route, structurally.
  log('\nreach');
  const strip = readFileSyncSafe(STRIP_FILE) || '';
  const rendered = ['BUILD_STATUS_LABEL', 'BUILD_STATUS_DETAIL', 'CROSS_GATE_LIMIT_LABEL', 'CROSS_GATE_LIMIT_DETAIL']
    .filter((c) => !new RegExp(`\\{\\s*${c}\\s*\\}`).test(strip));
  note(rendered.length === 0, `the honesty strip renders every claim constant${fail('never rendered', rendered)}`);

  const unmounted = LAYOUTS.filter((p) => {
    const src = readFileSyncSafe(p) || '';
    return !/from '\.\.\/honesty\/honesty-strip'/.test(src) || !/<HonestyStrip\b/.test(src);
  }).map(rel);
  note(unmounted.length === 0, `both layouts mount <HonestyStrip>${fail('missing', unmounted)}`);

  const routesSrc = readFileSyncSafe(ROUTES_FILE) || '';
  const { covered, orphaned } = routeCoverage(routesSrc);
  note(covered.length >= MIN_ROUTES,
    `${covered.length} routes examined (floor ${MIN_ROUTES}) — an unparsed routes.jsx cannot pass by finding nothing`);
  note(orphaned.length === 0,
    `every declared route descends from a layout that states both claims${fail('outside any layout', orphaned)}`);

  // 4 ── the whole app's copy.
  log('\ncopy');
  const files = walk(appDir);
  note(files.length >= MIN_FILES_SCANNED,
    `${files.length} source files found under web/src (floor ${MIN_FILES_SCANNED}) — an emptied glob cannot pass by scanning nothing`);

  const upgraded = [], discovered = [], money = [], privileged = [], remote = [];
  const shipped = [], testFiles = [];
  let urlsClassified = 0;
  for (const file of files) {
    const src = readFileSync(file, 'utf8');
    for (const hit of unnegatedClaims(src)) upgraded.push(`${rel(file)}: "${hit}"`);
    // Test files are exempt from THIS matcher only (the other three run over
    // every file unchanged). `peer-scope.test.js` plants the literal phrase
    // "Discover organisers near you" — both as regex source and as a
    // planted-defect fixture — to prove its OWN local guard over
    // peer-events.jsx fires; that string never ships (vite does not bundle
    // `*.test.js`) and is proof the guard works, not evidence of a claim.
    if (!isTest(file)) for (const hit of unnegatedClaims(src, DISCOVERY_CLAIMS)) discovered.push(`${rel(file)}: "${hit}"`);
    for (const { re, why } of FABRICATED_MONEY) {
      const m = src.match(re);
      if (m) money.push(`${rel(file)}: "${m[0]}" — ${why}`);
    }
    for (const { re, why } of PRIVILEGED_DEFAULT) {
      const m = src.match(re);
      if (m) privileged.push(`${rel(file)}: "${m[0]}" — ${why}`);
    }
    // The URL rule is about what the SHIPPED bundle reaches for, so `*.test.js`
    // is out of scope: `node --test` runs those files, vite never bundles them,
    // and their fixtures are deliberately full of unroutable URLs. Excluded
    // files are counted below so a tree that renamed itself into the exclusion
    // shows up as a collapse rather than as a clean run.
    if (isTest(file)) { testFiles.push(file); continue; }
    shipped.push(file);
    urlsClassified += [...src.matchAll(/https?:\/\/[^\s"'`)<>]+/g)].length;
    for (const u of externalFetches(src)) remote.push(`${rel(file)}: ${u}`);
  }

  note(upgraded.length === 0,
    `${files.length} files: nothing upgrades "detected" into "prevented"${fail('claims', upgraded)}`);
  note(discovered.length === 0,
    `${files.length} files: no discovery, directory or proximity claim — Cackle finds no peers on its own${fail('found', discovered)}`);
  note(money.length === 0,
    `${files.length} files: no fabricated fee, price or superlative${fail('found', money)}`);
  note(privileged.length === 0,
    `${files.length} files: no privileged processor, country or tax rate${fail('found', privileged)}`);
  note(shipped.length >= MIN_FILES_SCANNED - testFiles.length && testFiles.length <= 20,
    `${shipped.length} shipped files carry the URL rule, ${testFiles.length} test files excluded from it`);
  note(remote.length === 0,
    `${urlsClassified} absolute URLs classified across ${shipped.length} shipped files, none fetched at run time${fail('fetched', remote)}`);

  // 5 ── the no-billing claim, re-proven against the Go source.
  //
  // The app now tells organisers, in writing, that there is no billing system
  // in this software. That is true today. This makes it stay true, or makes
  // whoever changes it change the copy in the same commit.
  log('\nbacking');
  const goFiles = [join(repoRoot, 'internal'), join(repoRoot, 'cmd')]
    .filter(existsSync)
    .flatMap((d) => walkGo(d));
  const billing = [];
  for (const f of goFiles) {
    const m = readFileSync(f, 'utf8').match(FEE_IDENTIFIER);
    if (m) billing.push(`${rel(f)}: ${m[0]}`);
  }
  note(goFiles.length >= 50, `${goFiles.length} Go files examined for fee-collection code (floor 50)`);
  note(billing.length === 0,
    `no fee-collection identifier in the Go source — "no billing system" is still true${fail('found', billing)}`);

  const EXPECTED_CHECKS = 25;
  if (ran !== EXPECTED_CHECKS) {
    problems.push(`only ${ran} of ${EXPECTED_CHECKS} checks ran — the run did not complete`);
    console.log(`  FAIL  only ${ran} of ${EXPECTED_CHECKS} checks ran`);
  }

  if (problems.length) {
    console.error(`\ncheck-app: ${problems.length} problem${problems.length === 1 ? '' : 's'}:\n` +
      problems.map((p) => `  - ${p}`).join('\n'));
    process.exit(1);
  }
  console.log(`\ncheck-app: ${ran} checks passed — ${files.length} app source files and ${covered.length} routes examined. ` +
    'The app states its experimental status and the cross-gate limit on every route, ' +
    'never upgrades "detected" into "prevented", invents no fee or superlative, ' +
    'privileges no processor, and fetches nothing off-origin.');
}

function walkGo(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry.startsWith('.')) continue;
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) walkGo(p, out);
    else if (entry.endsWith('.go')) out.push(p);
  }
  return out;
}

main();
