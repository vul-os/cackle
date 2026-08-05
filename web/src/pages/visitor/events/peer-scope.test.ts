// The invariant behind borrowed listings, held two ways.
//
// Part one is the pure logic in ./peer-scope.ts, exercised directly.
//
// Part two is a SOURCE-TEXT guard over ./peer-events.tsx, and it is deliberate.
// The rule "a peer event must never look like it is sold by this host" is not
// a value a unit test can read off a return: it is the absence of a whole
// class of UI. A component that grew an "Add to cart" button, a price, a
// ticket picker or a route into checkout would still render, still pass every
// behavioural assertion about the props it does use, and quietly take money
// for a ticket this node cannot issue. So the test reads the file and asserts
// those things are not in it. The backend holds the same line from the other
// end — the JSON shape has no price/currency/ticket_type field and a Go test
// asserts that — and this is the client half of the same fence.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { peerOrgId, renderablePeerRows, usablePeerRows } from './peer-scope.ts';

const here = dirname(fileURLToPath(import.meta.url));
const componentSource = readFileSync(join(here, 'peer-events.tsx'), 'utf8');

/**
 * The component with its comments removed.
 *
 * The rule is about what the component RENDERS and DOES, not about what its
 * prose explains. The file header has to be able to say "there is no cart and
 * no checkout here" and "there is no directory" — those sentences are the
 * explanation of the rule, and a guard that punishes a file for documenting
 * itself teaches the next person to delete the documentation. So the matchers
 * below run over code only. `stripComments` is self-tested at the bottom of
 * this file, against a planted defect in each family, so a passing run is
 * evidence it actually looked rather than evidence it stripped everything.
 */
function stripComments(src: string): string {
    return src
        .replace(/\/\*[\s\S]*?\*\//g, ' ') // /* … */ and JSX {/* … */}
        .replace(/(^|[^:])\/\/[^\n]*/g, '$1'); // // … but not the // in a URL scheme
}

const component = stripComments(componentSource);

/** One borrowed listing off the wire, in the shape internal/httpapi sends. */
const row = (over = {}) => ({
    external: true,
    publisher: 'Neighbour Hall',
    publisher_key: 'a1b2c3',
    url: 'https://their-box.example/h/spring-fair',
    title: 'Spring Fair',
    notice: 'Hosted by another organiser. Tickets are sold on their own site, not here.',
    ...over,
});

// ── whose listings ──────────────────────────────────────────────────────────

// Every fixture below carries `peers_included: true`, because that is the
// operator saying "show borrowed listings on my page" — see the section after
// next. Without it the answer is null whatever the organisations are, and
// these cases would all pass for the wrong reason.
const shown = (over: Record<string, unknown>) => ({ peers_included: true, ...over });

test('a single-organisation box shows that organisation\'s borrowed listings', () => {
    assert.equal(peerOrgId(shown({ organisations: [{ id: 'org-1', name: 'The Bijou', slug: 'the-bijou' }], multi_org: false })), 'org-1');
});

test('a narrowed listing shows the narrowed organisation\'s, not the box\'s first', () => {
    const host = shown({
        organisations: [
            { id: 'org-1', name: 'The Bijou', slug: 'the-bijou' },
            { id: 'org-2', name: 'Night Market', slug: 'night-market' },
        ],
        multi_org: true,
        org: { id: 'org-2', name: 'Night Market', slug: 'night-market' },
    });
    assert.equal(peerOrgId(host), 'org-2');
});

test('a multi-organisation box with no narrowing shows none — one operator\'s enrolments are not another\'s', () => {
    const host = shown({
        organisations: [
            { id: 'org-1', name: 'The Bijou', slug: 'the-bijou' },
            { id: 'org-2', name: 'Night Market', slug: 'night-market' },
        ],
        multi_org: true,
    });
    assert.equal(peerOrgId(host), null);
});

test('a missing or malformed host envelope shows none rather than guessing', () => {
    const malformed = [{ organisations: null }, { organisations: [] }, { organisations: [{}] }, { org: {} }];
    for (const host of [undefined, null, {}, ...malformed, ...malformed.map(shown)]) {
        assert.equal(peerOrgId(host), null, `expected null for ${JSON.stringify(host)}`);
    }
});

// ── whether this host shows borrowed listings at all ────────────────────────
//
// The second switch. `feed_subscribe` on the server decides whether this box
// PULLS an organiser's programme; `CACKLE_HOST_SCOPE=peers` decides whether
// what was pulled goes on the public page. Both are off by default, so an
// operator who enrolled an organiser and fetched their events publishes
// nothing until they ask for it.

test('a host that has not opted in fetches nobody\'s listings, however many organisations it has', () => {
    const org = { id: 'org-1', name: 'The Bijou', slug: 'the-bijou' };
    assert.equal(peerOrgId({ scope: 'own', organisations: [org], peers_included: false }), null);
    assert.equal(peerOrgId({ scope: 'own', organisations: [org], multi_org: false, org, peers_included: false }), null);
});

// THE test for the join. Rows are present — a real pull happened, the cache is
// full, `usablePeerRows` keeps every one of them — and the operator has not
// opted in. Nothing renders. Without rows in this fixture the assertion would
// pass on an empty cache and prove nothing about the guard.
test('rows in hand and the scope off renders nothing — the case that isolates the guard', () => {
    const data = { events: [row(), row({ title: 'Autumn Fair', url: 'https://their-box.example/h/autumn-fair' })] };
    assert.equal(usablePeerRows(data).length, 2, 'the fixture must carry rows, or this test proves nothing');

    for (const host of [{ scope: 'own', peers_included: false }, { scope: 'single', peers_included: false }]) {
        assert.deepEqual(renderablePeerRows(host, data), [], `expected nothing rendered for ${JSON.stringify(host)}`);
    }
});

test('the same rows with the scope on are rendered', () => {
    const data = { events: [row(), row({ title: 'Autumn Fair', url: 'https://their-box.example/h/autumn-fair' })] };
    const out = renderablePeerRows({ scope: 'peers', peers_included: true }, data);
    assert.equal(out.length, 2);
    assert.deepEqual(out.map((e) => e.title), ['Spring Fair', 'Autumn Fair']);
});

test('an absent or malformed envelope renders nothing, never everything', () => {
    const data = { events: [row()] };
    for (const host of [undefined, null, {}, { scope: 'peers' }, { peers_included: 'true' }, { peers_included: 1 }]) {
        assert.deepEqual(renderablePeerRows(host, data), [], `expected nothing rendered for ${JSON.stringify(host)}`);
    }
});

test('opting in does not lower the bar on which rows may be rendered', () => {
    // The scope says "show borrowed listings"; it does not say "show anything
    // that arrives". A row that fails usablePeerRows is still dropped.
    const data = { events: [row({ external: false }), row({ url: '' }), row()] };
    assert.equal(renderablePeerRows({ peers_included: true }, data).length, 1);
});

// ── which rows may be rendered ──────────────────────────────────────────────

test('a row that does not declare itself external is DROPPED, not rendered', () => {
    const kept = usablePeerRows({ events: [row(), row({ external: false }), row({ external: undefined }), row({ external: 'true' })] });
    assert.equal(kept.length, 1);
    const [only] = kept;
    assert.ok(only);
    assert.equal(only.title, 'Spring Fair');
});

test('a row with no link out is dropped — the link out is the only action there is', () => {
    assert.deepEqual(usablePeerRows({ events: [row({ url: '' }), row({ url: undefined })] }), []);
});

test('a malformed or empty payload yields no rows rather than throwing', () => {
    for (const data of [undefined, null, {}, { events: null }, { events: 'nope' }, { events: [] }]) {
        assert.deepEqual(usablePeerRows(data), [], `expected [] for ${JSON.stringify(data)}`);
    }
});

// ── the invariant: never sold here ──────────────────────────────────────────

test('the borrowed-listing component offers no way to buy anything', () => {
    // Every shape of "this host will sell you this". A borrowed listing has
    // none of them, because this node cannot issue, hold or refund a ticket
    // signed by somebody else's key.
    const forbidden = [
        /\bAdd to cart\b/i,
        /\bBuy\b/i,
        /\bGet tickets?\b/i,
        /\bcheckout\b/i,
        /\buseCart\b/,
        /\bto=["'`]\/(?:cart|checkout)/,
        /\bMoney\b/,
        /event\.(?:price|currency|ticket_type|ticket_types)\b/,
        /\bminPriceMinor\b/,
        /\bsoldOut\b/,
    ];
    for (const re of forbidden) {
        assert.equal(re.test(component), false, `peer-events.tsx must not contain ${re}`);
    }
});

test('a borrowed listing carries its publisher and its notice, verbatim from the payload', () => {
    assert.match(component, /\{event\.publisher\b/, 'the publisher must be rendered');
    assert.match(component, /\{event\.notice\}/, 'the notice must be rendered verbatim, not paraphrased');
});

test('the only action leaves this box, and leaves it safely', () => {
    assert.match(component, /href=\{event\.url\}/, 'the link must go to the publisher\'s own page');
    assert.match(component, /rel="noreferrer noopener"/);
});

test('nothing in the component claims discovery, a directory, or organisers nearby', () => {
    // Cackle finds no peers on its own. There is no directory, no index, no
    // rendezvous and no geo lookup anywhere in the substrate — spec or code.
    // Every organiser shown here was typed in by this box's operator.
    const forbidden = [
        /organisers near you/i,
        /events? nearby/i,
        /near you/i,
        /\bdiscover\b/i,
        /browse organisers/i,
        /\bdirectory\b/i,
        /global feed/i,
        /find (?:more )?organisers/i,
    ];
    for (const re of forbidden) {
        assert.equal(re.test(component), false, `peer-events.tsx must not contain ${re}`);
    }
});

test('the source guard would catch the defects it exists to catch', () => {
    // A guard that cannot fire is worse than no guard, because it reports
    // success. Plant one defect from each family into the real source and
    // require a hit; then confirm the same text sitting in a COMMENT is
    // forgiven, which is the whole reason stripComments exists.
    const planted: Array<[string, RegExp]> = [
        ['<Button>Add to cart</Button>', /\bAdd to cart\b/i],
        ['<Link to="/checkout/1">Get tickets</Link>', /\bGet tickets?\b/i],
        ['<Money minor={event.price} />', /event\.(?:price|currency|ticket_type|ticket_types)\b/],
        ['<p>Discover organisers near you</p>', /organisers near you/i],
        ['<p>Browse the directory</p>', /\bdirectory\b/i],
    ];
    for (const [defect, re] of planted) {
        assert.equal(re.test(stripComments(`${componentSource}\n${defect}\n`)), true, `the guard missed a planted "${defect}"`);
        assert.equal(re.test(stripComments(`${componentSource}\n// ${defect}\n`)), false, `the guard flagged a commented "${defect}"`);
    }
    // And the stripper must not have eaten the file: the things that MUST be
    // present are still findable after stripping.
    assert.match(component, /export default function PeerEvents/);
});

test('the section renders nothing at all when this host carries no borrowed listing', () => {
    // Not an empty state, not a heading, not "no other organisers yet". A
    // visitor to a box with no peers must not learn that peers are a thing
    // that could appear.
    assert.match(component, /if \(events\.length === 0\) return null;/);
});

test('the section is rendered from the GATED rows, never from the raw ones', () => {
    // The guard the component cannot be allowed to walk around. `events` must
    // come from renderablePeerRows, which consults the host envelope; a
    // component that called usablePeerRows directly would render another
    // organiser's programme on a host whose operator never opted in, and every
    // behavioural assertion above would still pass, because the rows
    // themselves are perfectly good rows.
    assert.match(component, /const events = renderablePeerRows\(host, data\);/);
    assert.equal(/\busablePeerRows\b/.test(component), false,
        'peer-events.tsx must not reach past the gate to the ungated rows');
    // And the envelope is read in @/lib/host, not picked apart here.
    assert.equal(/\bpeers_included\b/.test(component), false,
        'peer-events.tsx must not parse the host envelope itself; lib/host.js is the one reader');
});
