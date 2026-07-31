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
// The theme rule's own floors, and the same argument again. A `className=`
// extractor that silently stops matching — a JSX refactor, a prop rename, a
// regex edited in a hurry — reports zero theme violations while looking at
// nothing at all, which is indistinguishable from success.
//
// The denominator is deliberately the count of colour-TAKING classes rather
// than the count of suspicious ones. The suspicious counts are the wrong
// floor precisely because the work succeeding drives them toward zero: there
// are two `dark:` variants left in the whole app, and a floor of two proves
// nothing about whether anything was read. "How many `bg-`/`text-`/`border-`
// classes did you look at and clear" does not shrink as the tree gets
// cleaner. Set at roughly 60% of the counts observed when this landed
// (~1880 classes, ~1970 regions), so ordinary churn does not trip them and a
// collapsed extractor does.
const MIN_STYLE_REGIONS = 1200;
const MIN_COLOUR_CLASSES = 1150;

const CLAIMS_FILE = join(appDir, 'components', 'honesty', 'claims.js');
const STRIP_FILE = join(appDir, 'components', 'honesty', 'honesty-strip.jsx');
const ROUTES_FILE = join(appDir, 'routes.jsx');
const TAILWIND_CONFIG = join(repoRoot, 'web', 'tailwind.config.js');
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
/**
 * sentences splits source text into the unit every negation rule in this file
 * is scoped to.
 *
 * Whitespace is collapsed BEFORE splitting, and the reason is specific:
 * check-site.mjs reads rendered innerText, where a sentence is one line; this
 * file reads source, where JSX line-wraps mid-sentence. Splitting on newlines
 * flagged the app's own honest ledger entry — "It is not a backup, and it
 * does not / prevent a double admission either." — because the wrap put the
 * negator on the previous line. Joining first restores the sentence scope the
 * rule is actually about.
 */
function sentences(text) {
  return text.replace(/\s+/g, ' ').split(/(?<=[.!?])\s+/);
}

function unnegatedClaims(text, patterns = FORBIDDEN_CLAIMS) {
  const hits = [];
  for (const sentence of sentences(text)) {
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
//
// # The plural hole
//
// These patterns ended in `\bfee\b`, and `\b` does not fire between "fee" and
// "fees" — so every one of them was blind to the plural. That is not a
// hypothetical: `web/src/pages/organizers/payouts/index.jsx` shipped a column
// described as "platform fees", implying Cackle takes a cut of every sale. It
// takes none — `fee_minor` is hardcoded to 0 throughout internal/orders and
// no code path ever sets it — and the gate said "no fabricated fee, price or
// superlative". A PAGE AGENT found it by reading. Planting "Platform fees are
// deducted from every payout." on pricing.jsx and re-running reproduced the
// pass exactly.
//
// Fixed the same way the identifier matcher below was: by enumerating the
// shapes the thing actually takes. English fees pluralise, possess and
// hyphenate — "platform fees", "Cackle's fee", "service-fee" — so all three
// are here, in every pattern that names a fee, a commission or a cut.
//
// # Two scoping rules, at two different scopes, for two different reasons
//
// NEGATION is scoped to the SENTENCE, because a denial governs a sentence.
// The app is REQUIRED to talk about fees — it cannot tell an organiser it
// charges nothing without using the word — so "there is no platform fee" and
// "Cackle takes no cut of any sale" have to pass. Both are real, shipped
// copy; the second is the sentence the payouts page was corrected TO. A gate
// that fires on the honest denial is worse than no gate, because the next
// person to hit it turns it off.
//
// ATTRIBUTION is scoped to the NOUN PHRASE — roughly 40 characters — and not
// to the sentence, which is the more interesting half. What Cackle's payment
// PROCESSOR charges is the organiser's own arrangement, is unknowable to
// Cackle, and is text this product genuinely needs to be able to write. But
// who charges a fee is decided by the words immediately in front of it, not
// by anything three clauses away: at sentence scope, "Cackle's fee is
// separate from your provider's fee" would be forgiven by the word "provider"
// at the far end of it. That is the same ±200-character mistake check-site.mjs
// records making with negation, arriving from the other direction.
const MONEY_NEGATOR =
  /\b(?:not|never|cannot|can't|no|none|nothing|nobody|neither|nor|without|zero|free|refus\w*|doesn't|won't|isn't|instead of|rather than)\b|\b0\s*%/i;

/** Someone other than Cackle, close enough in front of a fee to own it. */
const THIRD_PARTY_FEE = /\b(?:processor|provider|gateway|acquirer|issuer|bank|adapter)s?(?:'s|’s)?\b[^.]{0,24}$/i;

// "fee", "fees", "fee's" — and the separator that precedes it, which allows a
// space or a hyphen but NOT an underscore. `platform_fee` is an identifier,
// belongs to FEE_IDENTIFIER below, and appears legitimately in this app's own
// prose: pricing.jsx documents the very grep that proves no such identifier
// exists, and that grep names all four of them.
const FEE = "fee(?:s|'s|’s)?";
const FEE_OWNER = "our|platform|service|application|cackle(?:'s|’s)?";

const FABRICATED_MONEY = [
  // "Our Fee (0.85%)", "our fee", "platform fees", "Cackle's fee", "service-fee"
  { re: new RegExp(`\\b(?:${FEE_OWNER})[-\\s]+${FEE}\\b`, 'i'), why: 'names a fee Cackle charges — there is none' },
  { re: /\b(?:OUR|PLATFORM|SERVICE|APPLICATION)_FEES?(?:_(?:RATE|BPS|AMOUNT))?\b/, why: 'a fee-rate constant for a fee that does not exist' },
  // "we take 0.85%", "a 2% commission", "0.85% cut", "2% fees"
  { re: new RegExp(`\\b\\d+(?:\\.\\d+)?\\s*%\\s*(?:${FEE}|commissions?|cuts?|of every|of each)\\b`, 'i'), why: 'states a percentage Cackle takes' },
  { re: new RegExp(`\\b${FEE}\\s*\\(\\s*\\d+(?:\\.\\d+)?\\s*%`, 'i'), why: 'a labelled percentage fee line item' },
  // Superlatives, and NOT negatable — see MONEY_NEGATABLE below.
  { re: /\b(?:the\s+)?(?:lowest|cheapest|best|most\s+affordable|market[- ]leading)\s+(?:fees?|prices?|rates?|pricing|commissions?)\b/i, why: 'unsubstantiated competitive claim about price', negatable: false },
  { re: /\bin the (?:market|industry)\b/i, why: 'unsubstantiated competitive positioning', negatable: false },
  { re: /\b(?:industry[- ]leading|world[- ]class|unbeatable|second to none|the (?:only|number one) (?:ticketing|platform))\b/i, why: 'unsubstantiated superlative', negatable: false },
];

/**
 * unnegatedMoney applies both scoping rules and returns the surviving hits.
 *
 * `negatable: false` on the three superlative patterns is the same lesson
 * this file already records about "impossible" in the claim matcher, hit
 * again from a new direction: "second to none" CONTAINS the word "none", so
 * making it negatable would hand that pattern its own escape hatch and it
 * could never fire. And nothing is lost — no honest sentence in this app
 * needs to deny being the cheapest. A fee, by contrast, is denied in writing
 * on two shipped pages.
 */
function unnegatedMoney(text) {
  const hits = [];
  for (const sentence of sentences(text)) {
    const negated = MONEY_NEGATOR.test(sentence);
    for (const { re, why, negatable = true } of FABRICATED_MONEY) {
      if (negated && negatable) continue;
      const m = sentence.match(re);
      if (!m) continue;
      // Attribution, at noun-phrase scope: is somebody other than Cackle
      // standing immediately in front of this fee?
      if (negatable && THIRD_PARTY_FEE.test(sentence.slice(Math.max(0, m.index - 40), m.index))) continue;
      hits.push({ hit: m[0], why });
    }
  }
  return hits;
}

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

// ── the theme matcher ───────────────────────────────────────────────────────
//
// The owner's complaint, in full: "fix cards, deep css classes stick to theme".
//
// Every colour in this app is supposed to come from a semantic token, so it
// flips when the theme does. `web/src/index.css` defines those tokens twice —
// once under `:root`, once under `.dark` — and `lib/contrast.test.js` measures
// both sets. All of that is worth nothing to a component that writes
// `bg-gray-100`, because a raw palette class is the same colour in both
// themes by construction. The measured, audited, theme-aware palette simply
// does not apply to it.
//
// Nothing was looking for that. Before this matcher existed, planting
// `bg-gray-100 text-gray-500 dark:bg-gray-800` and `style={{color:'#6b7280'}}`
// on a page and running the gate produced "25 checks passed".
//
// Three shapes are theme-blind, and all three are here:
//
//   1. a RAW PALETTE class — `bg-white`, `text-gray-500`, `from-black/70`.
//      Fixed colours from Tailwind's default ramp. The `.dark` block cannot
//      reach them.
//   2. a COLOUR LITERAL — `#880424`, `rgba(10,8,10,0.75)`, `hsl(0 0% 50%)` —
//      written into a `className` or a `style`. Same problem, spelled out.
//   3. a `dark:` COLOUR PATCH — `dark:bg-gray-800`, `dark:text-foreground`.
//      This one looks like the fix and is the tell. If a colour needs a
//      `dark:` override it was the wrong colour: a semantic token already
//      flips, so patching one is redundant, and patching a raw palette class
//      just means shipping two hardcoded colours instead of one.
//
// The false-positive side is where this rule earns or loses its keep, because
// three legitimate cases look exactly like violations from a distance:
//
//   * `hsl(var(--chart-3))` and `text-[hsl(var(--brand))]` are TOKEN
//     REFERENCES that merely happen to be spelled with a colour function.
//     They are the correct thing. Flagging them would push authors toward
//     hardcoding, which is the opposite of the point.
//   * `dark:` on a non-colour utility — `dark:prose-invert`, `dark:opacity-40`,
//     `dark:border-2`, `dark:shadow-none` — is genuinely theme-specific
//     TREATMENT, not a second hardcoded colour.
//   * a QR plate really does have to be white. A camera reads dark modules on
//     a light ground; a themed plate is an unscannable ticket. That is a
//     physical constraint, not a styling preference, and it is what THEME_ALLOW
//     below is for.
//
// The `--verdict-*` floods and the `--gate-*`/`--on-media-*` tokens are the
// same kind of case and need no exception at all: they are already tokens, so
// `bg-verdict-duplicate` and `text-gate-ink` never match anything here.

const RAW_PALETTE =
  'white|black|slate|gray|grey|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose';

// Utilities that take a colour. `border`/`divide`/`ring` carry their side and
// offset suffixes so `border-t-gray-200` and `ring-offset-white` are seen.
// Deliberately NOT a list of every utility: `p-4` and `text-lg` share prefixes
// with nothing here, and a utility that cannot take a colour cannot be a
// theme-blind colour.
const COLOUR_UTIL =
  'bg|text|border(?:-[xytlrbse])?|ring(?:-offset)?|from|via|to|fill|stroke|decoration|outline|divide(?:-[xy])?|placeholder|caret|accent|shadow';

// A Tailwind variant chain: `hover:`, `dark:`, `sm:group-hover:`,
// `[&>svg]:`, `aria-[invalid=true]:` …
const VARIANT_CHAIN = '(?:[a-z][a-z0-9-]*(?:\\[[^\\]]*\\])?:)*';

// A class token has to START at a token boundary and END at one, or
// `bg-red-500-legacy` and `to-blackboard` would both be reported as colours.
const TOKEN_OPEN = '(?:^|[\\s"\'`{(,])';
const TOKEN_CLOSE = '(?=$|[\\s"\'`})\\],;])';

/** `bg-white`, `dark:hover:text-gray-400/70`, `from-black/70`. */
const RAW_PALETTE_CLASS = new RegExp(
  `${TOKEN_OPEN}(${VARIANT_CHAIN}(?:${COLOUR_UTIL})-(?:${RAW_PALETTE})(?:-(?:50|[1-9]00|950))?(?:\\/(?:\\d{1,3}|\\[[^\\]]*\\]))?)${TOKEN_CLOSE}`,
  'g',
);

/** `text-[#880424]`, `bg-[rgb(10,8,10)]` — but NOT `text-[hsl(var(--brand))]`. */
const ARBITRARY_COLOUR_CLASS = new RegExp(
  `${TOKEN_OPEN}(${VARIANT_CHAIN}(?:${COLOUR_UTIL})-\\[(?:#[0-9a-fA-F]{3,8}|(?:rgba?|hsla?)\\([^\\]]*\\))\\](?:\\/\\d{1,3})?)${TOKEN_CLOSE}`,
  'g',
);

/**
 * Every class token that TAKES a colour, whatever it was given — `bg-card`,
 * `text-sm`, `border-b`, `from-black/70`. This is the coverage denominator:
 * the count of things the rule looked at and decided were fine, which is what
 * separates "nothing is wrong" from "nothing was examined".
 */
const COLOUR_UTIL_CLASS = new RegExp(
  `${TOKEN_OPEN}(${VARIANT_CHAIN}(?:${COLOUR_UTIL})-[a-zA-Z0-9[(#][^\\s"'\`}\\],;]*)${TOKEN_CLOSE}`,
  'g',
);

// Non-global twins of the two class matchers. `.test()` on a /g/ regex
// advances its own lastIndex and therefore returns a DIFFERENT answer on
// alternate calls for the same input — a guard that is right every other
// time. The `dark:` pass below asks yes/no questions, so it asks these.
const RAW_PALETTE_CLASS_ONE = new RegExp(RAW_PALETTE_CLASS.source);
const ARBITRARY_COLOUR_CLASS_ONE = new RegExp(ARBITRARY_COLOUR_CLASS.source);

/** Every `dark:`-variant utility in the source, colour or not. */
const DARK_VARIANT = new RegExp(
  `${TOKEN_OPEN}((?:[a-z][a-z0-9-]*(?:\\[[^\\]]*\\])?:)*dark:${VARIANT_CHAIN}[a-z][a-zA-Z0-9/[\\]().#_-]*)${TOKEN_CLOSE}`,
  'g',
);

/**
 * A colour literal, anywhere. `hsl(var(--x))` is excluded by the matcher
 * itself rather than by a later filter, because the exclusion IS the rule:
 * a colour function whose argument is a custom property is a token reference,
 * and a token reference is the correct thing to write.
 */
const COLOUR_LITERAL = /#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{1,5})?\b|(?:rgba?|hsla?)\([^)]*\)/g;
const NAMED_CSS_COLOUR = new RegExp(`\\b(?:${RAW_PALETTE}|silver|maroon|navy|olive|teal|aqua|fuchsia)\\b`, 'g');

/**
 * THEME_ALLOW — the only colours permitted to ignore the theme, each with the
 * reason it is not a styling choice.
 *
 * Scoped to a FILE and to specific CLASSES, never to a whole file. That is
 * deliberate: both entries below sit in files that also carry `text-gray-500`
 * and `ring-black/10` for the plate's caption and hairline, and neither of
 * those is physically required by anything. A file-level exemption would
 * launder them through with the one class that has a real reason.
 *
 * An entry that stops suppressing anything FAILS the run rather than sitting
 * there, so this list can only shrink by being noticed.
 */
const THEME_ALLOW = [
  {
    file: 'web/src/pages/visitor/ticket/index.jsx',
    classes: ['bg-white'],
    why:
      'the QR plate behind the ticket code. A QR reader needs dark modules on a light ground with real ' +
      'reflectance contrast; on the dark theme a themed plate is a ticket that will not scan at the door. ' +
      'This is the physical requirement of the medium, not a styling preference — which is why the class is ' +
      'paired with `print-keep-color` for exactly the same reason on paper.',
  },
  {
    file: 'web/src/pages/visitor/tickets/printing/layout.jsx',
    classes: ['bg-white'],
    why: 'the same QR plate on the printable ticket sheet, for the same physical scanning reason.',
  },
];

/** Strip comments so a class named in prose is not read as a class in use. */
function stripComments(src) {
  // The `[^:]` guard is the same one hasLineComment() encodes: a URL scheme's
  // `//` is always preceded by its own `:`, and a real comment marker never is.
  return src.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/([^:]|^)\/\/[^\n]*/gm, '$1');
}

/**
 * Every `className=` / `class=` / `style=` value in the source, with the
 * attribute that carried it.
 *
 * Attribute-scoped rather than whole-file, because that is where the literal
 * rule can be applied without ambiguity: `#abc` in a URL fragment or a git sha
 * is not a colour, and `'white'` is only a colour when something is being
 * painted with it. Quote- and brace-aware, so `className={cn('a', x ? 'b' :
 * 'c')}` and a template literal holding `${expr}` are captured whole instead
 * of being cut at their first inner brace.
 */
function styleRegions(src) {
  const out = [];
  const re = /\b(className|class|style)\s*=\s*/g;
  let m;
  while ((m = re.exec(src)) !== null) {
    let i = re.lastIndex;
    while (i < src.length && /\s/.test(src[i])) i++;
    const opener = src[i];
    if (opener === '"' || opener === "'") {
      const end = src.indexOf(opener, i + 1);
      if (end === -1) continue;
      out.push({ attr: m[1], value: src.slice(i + 1, end) });
      re.lastIndex = end + 1;
    } else if (opener === '{') {
      let depth = 0, quote = null, j = i;
      for (; j < src.length; j++) {
        const ch = src[j];
        if (quote) {
          if (ch === quote && src[j - 1] !== '\\') quote = null;
          continue;
        }
        if (ch === '"' || ch === "'" || ch === '`') { quote = ch; continue; }
        if (ch === '{') depth++;
        else if (ch === '}' && --depth === 0) break;
      }
      if (j >= src.length) continue;
      out.push({ attr: m[1], value: src.slice(i + 1, j) });
      re.lastIndex = j + 1;
    }
  }
  return out;
}

/**
 * SEMANTIC_ROOTS is read from tailwind.config.js rather than restated here,
 * so the `dark:` rule keeps up with the palette by construction. It is used
 * for one job only: telling `dark:bg-card` (a redundant patch on a token that
 * already flips) apart from `dark:border-2` (a width) and `dark:prose-invert`
 * (a treatment).
 *
 * Purely numeric keys are dropped, and they matter: `brand: { 2: … }` and
 * `chart: { 1: … }` contribute the keys "1".."8", which would otherwise make
 * the legitimate `dark:border-2` look like a colour named 2.
 */
function semanticColourRoots(configSrc) {
  const block = configSrc.match(/colors:\s*\{([\s\S]*?)\n {12}\},/);
  if (!block) return [];
  const keys = new Set();
  for (const m of block[1].matchAll(/^\s*'?([A-Za-z0-9][A-Za-z0-9-]*)'?:\s*(?:'|\{)/gm)) {
    if (/^\d+$/.test(m[1]) || m[1] === 'DEFAULT') continue;
    keys.add(m[1]);
  }
  return [...keys];
}

/**
 * themeViolations classifies one file and returns both the hits and the
 * counts, so a run that examined nothing cannot report zero problems without
 * also reporting that it looked at nothing.
 *
 * @param {string} relPath repo-relative, for allowlist matching and messages
 * @param {string} src
 * @param {string[]} roots semantic colour roots from the Tailwind config
 */
function themeViolations(relPath, src, roots) {
  // Fail-closed. With an empty root list the `dark:` alternation collapses to
  // `(?:…|)`, which matches the empty string, and every `dark:` utility in the
  // app becomes a violation. A gate that cannot read the palette must say so,
  // not guess.
  if (!roots.length) throw new Error('themeViolations: no semantic colour roots — the Tailwind palette could not be read');
  const allowed = new Set(THEME_ALLOW.filter((a) => a.file === relPath).flatMap((a) => a.classes));
  const hits = [];
  const suppressed = new Set();
  let regionsScanned = 0, darkClassified = 0, literalsClassified = 0, classesClassified = 0;

  const code = stripComments(src);
  const bare = (cls) => cls.slice(cls.lastIndexOf(':') + 1).split('/')[0];

  for (const m of code.matchAll(COLOUR_UTIL_CLASS)) classesClassified++;

  for (const re of [RAW_PALETTE_CLASS, ARBITRARY_COLOUR_CLASS]) {
    re.lastIndex = 0;
    for (const m of code.matchAll(re)) {
      const cls = m[1];
      // `text-[hsl(var(--brand))]` reaches ARBITRARY_COLOUR_CLASS because it
      // is spelled with a colour function, and it is the CORRECT thing to
      // write: an arbitrary value wrapping a custom property is a token
      // reference. The exclusion belongs here rather than in the pattern,
      // where making the alternation reject `var(` costs more legibility
      // than one honest line of prose.
      if (/var\(\s*--/.test(cls)) continue;
      if (allowed.has(bare(cls))) { suppressed.add(bare(cls)); continue; }
      hits.push(`theme-blind class \`${cls}\``);
    }
  }

  DARK_VARIANT.lastIndex = 0;
  const darkColourRe = new RegExp(`^(?:${COLOUR_UTIL})-(?:${RAW_PALETTE}|${roots.join('|')})(?:[-/]|$)`);
  for (const m of code.matchAll(DARK_VARIANT)) {
    darkClassified++;
    const util = m[1].slice(m[1].lastIndexOf('dark:') + 5);
    // Only the ones already reported above are skipped; a `dark:` patch on a
    // SEMANTIC token is invisible to those matchers and is the whole reason
    // this third pass exists.
    if (RAW_PALETTE_CLASS_ONE.test(` ${util} `) || ARBITRARY_COLOUR_CLASS_ONE.test(` ${util} `)) continue;
    if (darkColourRe.test(util)) hits.push(`\`${m[1]}\` patches a colour with a theme variant — use the token`);
  }

  for (const { attr, value } of styleRegions(src)) {
    regionsScanned++;
    COLOUR_LITERAL.lastIndex = 0;
    for (const lit of value.match(COLOUR_LITERAL) || []) {
      literalsClassified++;
      if (/var\(\s*--/.test(lit)) continue; // a token reference, not a literal
      hits.push(`colour literal \`${lit}\` in a ${attr}`);
    }
    // Named CSS colours are only unambiguous inside a real style declaration,
    // so this runs on `style` alone. `whiteSpace: 'nowrap'` survives it: the
    // word boundary refuses to end a match in the middle of an identifier.
    if (attr !== 'style') continue;
    for (const named of value.match(NAMED_CSS_COLOUR) || []) {
      literalsClassified++;
      hits.push(`named colour \`${named}\` in a style`);
    }
  }

  return { hits, suppressed, regionsScanned, darkClassified, literalsClassified, classesClassified };
}

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
    'Our Fee (0.85%)',
    'const OUR_FEE_RATE = 0.0085;',
    'The lowest fees in the market',
    'We take a 2% commission on every ticket.',
    'The industry-leading ticketing platform.',
    // The plural family. Every one of these walked straight through the
    // predecessor, whose patterns all ended in `\bfee\b`.
    'Platform fees are deducted from every payout.',
    'Gross sales, platform fees, and net payable per event.',
    'Service fees apply to each ticket sold.',
    'Our fees are among the lowest anywhere.',
    'Application fees are withheld at settlement.',
    "Cackle's fees are billed monthly.",
    'Cackle’s fee is 1.5% of face value.',
    'A service-fee is added at checkout.',
    'Fees (2.5%) are shown at checkout.',
    'We keep 2% fees on each sale.',
    'const PLATFORM_FEES_BPS = 85;',
    'APPLICATION_FEE_AMOUNT',
    // "second to none" carries the word "none". If the superlative patterns
    // were negatable it would forgive itself and could never fire — the same
    // trap "impossible" sets for the claim matcher.
    'Cackle is second to none.',
  ];
  for (const s of mustCatch) if (unnegatedMoney(s).length === 0) bad.push(`money gate did NOT catch: "${s}"`);

  // The forgive side, and it is the load-bearing one. The app CANNOT tell an
  // organiser it charges nothing without writing the word "fee", so every
  // denial below is real or near-real shipped copy — the second and third are
  // what the payouts page and the pricing page actually say today.
  const mustPass = [
    'Cackle charges you nothing. There is no billing system in this software.',
    'Cackle takes 0% of what you sell.',
    'No per-ticket cut, no percentage, no monthly plan.',
    'Ticket price', 'Total',
    'Gross sales, fees, and net payable per event. Cackle takes no cut of any sale.',
    'Cackle has no fees.',
    'There is no platform fee.',
    'There are no platform fees, no service fees and no application fees.',
    '"platform fees" would read as a cut Cackle takes — it takes none.',
    'Cackle charges no fee of any kind.',
    'Our fee is zero.',
    'There is no fee anywhere in this software.',
    // What a PROCESSOR charges is the organiser's own arrangement and is
    // honest text this product needs to be able to write. Attribution is
    // judged at noun-phrase scope: the third party has to be standing in
    // front of the fee, not merely somewhere in the sentence.
    'Fee breakdown is your provider’s business, not ours.',
    'Whatever your payment provider’s service fee comes to is between you and them.',
    'The processor fees on that sale were deducted before payout.',
    'Your bank’s fees are not visible to Cackle.',
    'This column exists for whatever your payment processor may ever deduct.',
    // A legitimate identifier grep, which pricing.jsx really does print. The
    // underscore forms belong to FEE_IDENTIFIER and are matched only in
    // uppercase, so this documentation of their ABSENCE is not a claim.
    "grep -rn 'platform_fee|service_fee|our_fee|application_fee|commission' internal/ cmd/",
  ];
  for (const s of mustPass) {
    const hits = unnegatedMoney(s);
    if (hits.length) bad.push(`money gate wrongly flagged: "${s}" (${hits.map((h) => h.hit).join(', ')})`);
  }

  // Attribution must not be a sentence-wide escape hatch: naming a provider
  // anywhere in a sentence cannot launder a fee Cackle attributes to itself.
  const both = "Cackle's fee is separate from your provider's fee.";
  if (unnegatedMoney(both).length === 0) {
    bad.push(`money gate let a Cackle fee through because a provider was named elsewhere in the sentence: "${both}"`);
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
 * The theme matcher's self-test.
 *
 * Weighted deliberately toward the FORGIVE side. Every other matcher in this
 * file guards copy, where a false positive is an annoyance; this one guards
 * the styling of every component in the app, where a false positive is worse
 * than a false negative — it teaches whoever hits it that the gate is noise,
 * and the next person routes around it. The four must-pass cases below are
 * the exact three shapes the rule is most likely to get wrong (a token
 * reference spelled with a colour function, a `dark:` that is treatment
 * rather than colour, a physically-required white) plus the identifier trap
 * `whiteSpace`, and each of them is a thing this app really writes.
 */
function themeMatcherSelfTest(roots) {
  const bad = [];
  const F = 'web/src/pages/fixture.jsx';
  const run = (src, file = F) => themeViolations(file, src, roots).hits;

  if (!roots.length) return ['theme gate could not read any colour root out of web/tailwind.config.js'];

  const mustCatch = [
    ['raw palette fill', '<div className="rounded bg-gray-100 p-2" />'],
    ['raw palette ink', '<p className="text-gray-500">x</p>'],
    ['bare white', '<div className="bg-white" />'],
    ['gradient stop', '<div className="bg-gradient-to-t from-black/70 to-transparent" />'],
    ['variant chain', '<a className="sm:hover:text-slate-800">x</a>'],
    ['border side', '<div className="border-l-4 border-red-600" />'],
    ['ring + outline', '<div className="ring-black/10 outline-white" />'],
    ['dark: patch on a raw palette', '<div className="bg-slate-100 dark:bg-black/20" />'],
    ['dark: patch on a semantic token', '<div className="bg-card dark:bg-card" />'],
    ['arbitrary hex in a class', '<code className="dark:text-[#880424]">x</code>'],
    ['hex in an inline style', "<div style={{ color: '#6b7280' }} />"],
    ['rgba in an inline style', "<div style={{ backgroundImage: `linear-gradient(rgba(10,8,10,0.75), url(${bg}))` }} />"],
    ['literal hsl in an inline style', "<div style={{ color: 'hsl(0 0% 50%)' }} />"],
    ['named colour in a style string', `const html = '<div style="border:3px solid white"></div>';`],
    // The plate exemption is per FILE and per CLASS. Written in a file that
    // has no entry, the very same class is still a violation — otherwise the
    // allowlist would be a phrase anyone could copy.
    ['an allowlisted class in a file with no entry', '<div className="bg-white p-4" />'],
  ];
  for (const [label, src] of mustCatch) {
    if (run(src).length === 0) bad.push(`theme gate did NOT catch ${label}: ${src}`);
  }

  const mustPass = [
    // Token references. The whole point of the rule is to produce these.
    ['semantic tokens', '<div className="bg-card text-card-foreground border-border" />'],
    ['brand + emphasis', '<span className="bg-primary text-primary-foreground" /><b className="text-primary-emphasis" />'],
    // The verdict palette is the scanner's three answers and is untouchable.
    // It must never be inconvenient to use correctly.
    ['verdict tokens', '<div className="bg-verdict-duplicate text-verdict-ink" /><div className="bg-verdict-admit" />'],
    ['gate + media tokens', '<div className="bg-gate text-gate-ink border-gate-line" /><h1 className="text-media-ink" />'],
    ['token reference in a class', '<span className="text-[hsl(var(--brand))] bg-[hsl(var(--brand-2))]" />'],
    ['token reference in a style', "<div style={{ background: 'hsl(var(--primary))' }} />"],
    ['chart series from tokens', 'const seriesColor = (i) => `hsl(var(--chart-${i + 1}))`;'],
    ['custom property set to a token', `<div style={{ '--notch': 'var(--sidebar-background)' }} />`],
    // `dark:` that is genuinely theme-specific treatment, not a colour.
    ['dark:prose-invert', '<div className="prose dark:prose-invert" />'],
    ['dark: opacity / width / shadow', '<div className="dark:opacity-40 dark:border-2 dark:shadow-none dark:shadow-elevated" />'],
    ['dark: mix-blend', '<img className="opacity-90 dark:opacity-70 dark:mix-blend-luminosity" />'],
    // Utilities that merely share a prefix with a colour utility.
    ['non-colour utilities', '<p className="text-sm text-balance border-b divide-y to-95% from-0%" />'],
    ['transparent / current / inherit', '<div className="bg-transparent border-transparent fill-current stroke-current text-inherit" />'],
    // The identifier trap: a CSS property whose NAME contains a colour word.
    ['whiteSpace is not the colour white', "<td style={{ whiteSpace: 'nowrap' }} />"],
    // Not a colour: a URL fragment and a short hash both start with '#'.
    ['a href fragment is not a hex colour', '<a href="/docs#faq">FAQ</a>'],
    // Prose naming a class is documentation, not a class in use — the same
    // one-directional comment rule the URL matcher uses.
    ['a class named in a comment', '// this used to be bg-gray-100 and text-white before the palette landed'],
    ['a class named in a block comment', '/* replaced bg-slate-50 with bg-muted */'],
  ];
  for (const [label, src] of mustPass) {
    const hits = run(src);
    if (hits.length) bad.push(`theme gate wrongly flagged ${label}: ${hits.join('; ')}`);
  }

  // The allowlist really does suppress, in the file it names.
  const plate = '<div className="print-keep-color bg-white p-4 ring-1 ring-black/10" />';
  const inPlateFile = run(plate, THEME_ALLOW[0].file);
  if (inPlateFile.some((h) => h.includes('bg-white'))) {
    bad.push(`theme gate ignored its own allowlist: ${inPlateFile.join('; ')}`);
  }
  // …and suppresses ONLY the class it names. The hairline on that same plate
  // is not physically required by anything and must still be reported.
  if (!inPlateFile.some((h) => h.includes('ring-black'))) {
    bad.push('the plate allowlist leaked past the class it names — ring-black/10 went unreported');
  }
  return bad;
}

/**
 * Allowlist hygiene. Three ways an exemption list rots, all fatal here:
 * an entry with no stated reason, an entry naming a file that no longer
 * exists, and an entry that no longer suppresses anything. The last is the
 * one that matters most — a stale exemption is how a list that was once
 * three justified cases becomes a place to put things.
 */
function themeAllowlistProblems(counted) {
  const bad = [];
  for (const entry of THEME_ALLOW) {
    const where = `${entry.file} [${entry.classes.join(', ')}]`;
    if (!entry.classes?.length) bad.push(`${where}: exempts no specific class — a file-wide exemption is not allowed`);
    if (!entry.why || entry.why.length < 40) bad.push(`${where}: no stated reason`);
    if (!existsSync(join(repoRoot, entry.file))) bad.push(`${where}: names a file that does not exist`);
    for (const cls of entry.classes || []) {
      if (!counted.get(entry.file)?.has(cls)) {
        bad.push(`${where}: \`${cls}\` no longer appears there — delete this entry, it is now exempting nothing`);
      }
    }
  }
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

  const tailwindSrc = readFileSyncSafe(TAILWIND_CONFIG) || '';
  const colourRoots = semanticColourRoots(tailwindSrc);
  const themeBad = themeMatcherSelfTest(colourRoots);
  note(themeBad.length === 0,
    `theme matcher catches what it must and forgives what it must, over ${colourRoots.length} colour roots ` +
      `read from web/tailwind.config.js${fail('problems', themeBad)}`);

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

  const upgraded = [], discovered = [], money = [], privileged = [], remote = [], themeBlind = [];
  const shipped = [], testFiles = [];
  let urlsClassified = 0;
  // Theme coverage. Counted so that "no violations" is always accompanied by
  // evidence that something was examined — the failure mode this file's
  // header names, applied to the newest rule in it.
  let styleRegionsScanned = 0, darkVariantsClassified = 0, colourLiteralsClassified = 0, colourClassesClassified = 0;
  const themeFilesScanned = [];
  const allowlistUsed = new Map();
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
    for (const { hit, why } of unnegatedMoney(src)) money.push(`${rel(file)}: "${hit}" — ${why}`);
    for (const { re, why } of PRIVILEGED_DEFAULT) {
      const m = src.match(re);
      if (m) privileged.push(`${rel(file)}: "${m[0]}" — ${why}`);
    }
    // The URL rule is about what the SHIPPED bundle reaches for, so `*.test.js`
    // is out of scope: `node --test` runs those files, vite never bundles them,
    // and their fixtures are deliberately full of unroutable URLs. Excluded
    // files are counted below so a tree that renamed itself into the exclusion
    // shows up as a collapse rather than as a clean run.
    // The theme rule is about what RENDERS, so it runs over every shipped
    // .jsx/.js — component, page and lib alike — and skips `*.test.js` for
    // the same reason the URL rule does: those files are fixtures, several
    // of them deliberately contain the defects they assert against, and
    // vite never bundles them.
    if (!isTest(file)) {
      const t = themeViolations(rel(file), src, colourRoots);
      themeFilesScanned.push(file);
      styleRegionsScanned += t.regionsScanned;
      darkVariantsClassified += t.darkClassified;
      colourLiteralsClassified += t.literalsClassified;
      colourClassesClassified += t.classesClassified;
      if (t.suppressed.size) allowlistUsed.set(rel(file), t.suppressed);
      for (const hit of t.hits) themeBlind.push(`${rel(file)}: ${hit}`);
    }

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

  // 4b ── the theme rule. See "the theme matcher" above.
  log('\ntheme');
  note(
    themeFilesScanned.length >= MIN_FILES_SCANNED - 20 &&
      styleRegionsScanned >= MIN_STYLE_REGIONS &&
      colourClassesClassified >= MIN_COLOUR_CLASSES,
    `${colourClassesClassified} colour-taking classes (floor ${MIN_COLOUR_CLASSES}) and ` +
      `${styleRegionsScanned} className/style regions (floor ${MIN_STYLE_REGIONS}) classified across ` +
      `${themeFilesScanned.length} files; of those, ${darkVariantsClassified} carried a dark: variant and ` +
      `${colourLiteralsClassified} held a colour function or literal — ` +
      'a run that examined nothing cannot report zero theme violations');
  note(themeBlind.length === 0,
    `every colour comes from a semantic token and flips with the theme` +
      fail(`${themeBlind.length} theme-blind`, themeBlind));
  // Printed with the failure rather than left in this file's header, because
  // whoever hits this is looking at terminal output, not at the gate's
  // source, and the wrong reflex — relaxing the matcher or adding an
  // allowlist entry — is the one that is easiest to reach for.
  if (themeBlind.length) {
    console.log(
      '\n  Fix the COLOUR, not this check. The token for each case:\n' +
      '    surfaces / type / rules   bg-card · bg-background · bg-muted · text-foreground ·\n' +
      '                              text-muted-foreground · border-border\n' +
      '    brand                     bg-primary + text-primary-foreground · text-primary-emphasis\n' +
      '                              (brand as INK — bg-primary under small text is 3.34:1)\n' +
      '    over a photograph         text-media-ink · from-media-ground/90 · border-media-ink/20\n' +
      '    the scan screen           bg-gate · text-gate-ink · text-gate-ink-muted · border-gate-line\n' +
      '    a modal / drawer wash     bg-scrim\n' +
      '    elevation                 shadow-soft · shadow-elevated · shadow-floating ·\n' +
      '                              shadow-docked (casts upward, for a bottom-docked bar)\n' +
      '                              never a hardcoded rgba() box-shadow — the ramp is themed\n' +
      '  THEME_ALLOW in this file is for colour a physical device requires, not for\n' +
      '  colour that is inconvenient to tokenise. It is checked for staleness every run.\n',
    );
  }
  const allowProblems = themeAllowlistProblems(allowlistUsed);
  note(allowProblems.length === 0,
    `all ${THEME_ALLOW.length} theme exemptions still apply, name a class rather than a file, and say why` +
      fail('stale or unjustified', allowProblems));

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

  const EXPECTED_CHECKS = 29;
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
