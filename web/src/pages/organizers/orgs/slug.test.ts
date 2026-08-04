// The org-slug preview must agree with internal/orgs.slugify, because the
// form shows the user the address they are about to get. A preview that
// disagrees with the server is worse than no preview: it is a promise the
// product then breaks in public.
//
// Every case below is also asserted against the Go implementation in
// internal/orgs.TestSlugifyMatchesTheFrontendPreview, reading this same
// table, so the two can't drift apart silently — one of the two suites
// goes red the moment they do.

import test from 'node:test';
import assert from 'node:assert/strict';

import { slugifyOrgName, MAX_ORG_SLUG_LENGTH } from './slug.ts';

// SHARED_CASES is the contract. Keep it in sync with the Go table.
export const SHARED_CASES = [
    ['The Old Biscuit Mill', 'the-old-biscuit-mill'],
    ['Neon Nights', 'neon-nights'],
    ['  Harbour   Live  ', 'harbour-live'],
    ['HARBOUR-LIVE', 'harbour-live'],
    ['harbour--live', 'harbour-live'],
    ['Rocking the Daisies!', 'rocking-the-daisies'],
    ["St Mary's Church Hall", 'st-mary-s-church-hall'],
    ['Café del Mar', 'caf-del-mar'],
    ['-leading-and-trailing-', 'leading-and-trailing'],
    ['Venue 42', 'venue-42'],
    ['2026', '2026'],
    ['!!! ???', ''],
    ['', ''],
    ['   ', ''],
    ['---', ''],
];

test('slugifyOrgName matches the server rule on every shared case', () => {
    for (const [input, want] of SHARED_CASES) {
        assert.equal(slugifyOrgName(input), want, `input ${JSON.stringify(input)}`);
    }
});

test('slugifyOrgName is idempotent — normalising a slug again changes nothing', () => {
    for (const [input] of SHARED_CASES) {
        const once = slugifyOrgName(input);
        assert.equal(slugifyOrgName(once), once, `input ${JSON.stringify(input)}`);
    }
});

test('slugifyOrgName caps at the server length limit and never ends in a hyphen', () => {
    // A name long enough that the cut lands mid-word, and one where the cut
    // lands exactly on a hyphen — the case a naive slice() leaves trailing.
    const long = 'a'.repeat(80);
    assert.equal(slugifyOrgName(long).length, MAX_ORG_SLUG_LENGTH);

    const hyphenAtBoundary = `${'a'.repeat(MAX_ORG_SLUG_LENGTH - 1)} bbbb`;
    const got = slugifyOrgName(hyphenAtBoundary);
    assert.ok(got.length <= MAX_ORG_SLUG_LENGTH, `length ${got.length}`);
    assert.ok(!got.endsWith('-'), `slug ended in a hyphen: ${got}`);
    assert.ok(!got.startsWith('-'), `slug started with a hyphen: ${got}`);
});

test('slugifyOrgName tolerates non-string input rather than throwing', () => {
    // The form binds this to a react-hook-form field that is undefined on
    // first render.
    for (const input of [undefined, null]) {
        assert.equal(slugifyOrgName(input), '');
    }
});

test('slugifyOrgName output only ever contains a-z, 0-9 and inner hyphens', () => {
    const inputs = [
        ...SHARED_CASES.map(([i]) => i),
        'Ünïcödé Vénüé',
        'tabs\tand\nnewlines',
        '<script>alert(1)</script>',
        'a/b\\c?d#e&f=g',
        '100% Pure',
    ];
    for (const input of inputs) {
        const got = slugifyOrgName(input);
        assert.match(got, /^$|^[a-z0-9]+(-[a-z0-9]+)*$/, `input ${JSON.stringify(input)} -> ${JSON.stringify(got)}`);
    }
});
