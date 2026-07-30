// The invariant behind borrowed listings, held two ways.
//
// Part one is the pure logic in ./peer-scope.js, exercised directly.
//
// Part two is a SOURCE-TEXT guard over ./peer-events.jsx, and it is deliberate.
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

import { peerOrgId, usablePeerRows } from './peer-scope.js';

const here = dirname(fileURLToPath(import.meta.url));
const componentSource = readFileSync(join(here, 'peer-events.jsx'), 'utf8');

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
function stripComments(src) {
    return src
        .replace(/\/\*[\s\S]*?\*\//g, ' ') // /* … */ and JSX {/* … */}
        .replace(/(^|[^:])\/\/[^\n]*/g, '$1'); // // … but not the // in a URL scheme
}

const component = stripComments(componentSource);

// ── whose listings ──────────────────────────────────────────────────────────

test('a single-organisation box shows that organisation\'s borrowed listings', () => {
    assert.equal(peerOrgId({ organisations: [{ id: 'org-1', name: 'The Bijou', slug: 'the-bijou' }], multi_org: false }), 'org-1');
});

test('a narrowed listing shows the narrowed organisation\'s, not the box\'s first', () => {
    const host = {
        organisations: [
            { id: 'org-1', name: 'The Bijou', slug: 'the-bijou' },
            { id: 'org-2', name: 'Night Market', slug: 'night-market' },
        ],
        multi_org: true,
        org: { id: 'org-2', name: 'Night Market', slug: 'night-market' },
    };
    assert.equal(peerOrgId(host), 'org-2');
});

test('a multi-organisation box with no narrowing shows none — one operator\'s enrolments are not another\'s', () => {
    const host = {
        organisations: [
            { id: 'org-1', name: 'The Bijou', slug: 'the-bijou' },
            { id: 'org-2', name: 'Night Market', slug: 'night-market' },
        ],
        multi_org: true,
    };
    assert.equal(peerOrgId(host), null);
});

test('a missing or malformed host envelope shows none rather than guessing', () => {
    for (const host of [undefined, null, {}, { organisations: null }, { organisations: [] }, { organisations: [{}] }, { org: {} }]) {
        assert.equal(peerOrgId(host), null, `expected null for ${JSON.stringify(host)}`);
    }
});

// ── which rows may be rendered ──────────────────────────────────────────────

const row = (over = {}) => ({
    external: true,
    publisher: 'Neighbour Hall',
    publisher_key: 'a1b2c3',
    url: 'https://their-box.example/h/spring-fair',
    title: 'Spring Fair',
    notice: 'Hosted by another organiser. Tickets are sold on their own site, not here.',
    ...over,
});

test('a row that does not declare itself external is DROPPED, not rendered', () => {
    const kept = usablePeerRows({ events: [row(), row({ external: false }), row({ external: undefined }), row({ external: 'true' })] });
    assert.equal(kept.length, 1);
    assert.equal(kept[0].title, 'Spring Fair');
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
        assert.equal(re.test(component), false, `peer-events.jsx must not contain ${re}`);
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
        assert.equal(re.test(component), false, `peer-events.jsx must not contain ${re}`);
    }
});

test('the source guard would catch the defects it exists to catch', () => {
    // A guard that cannot fire is worse than no guard, because it reports
    // success. Plant one defect from each family into the real source and
    // require a hit; then confirm the same text sitting in a COMMENT is
    // forgiven, which is the whole reason stripComments exists.
    const planted = [
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
    // that could appear — peers_included is false on the wire today.
    assert.match(component, /if \(events\.length === 0\) return null;/);
});
