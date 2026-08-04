// The sign-in provider row must be impossible to render on a box where no
// provider is configured.
//
// The failure this guards against is specific and it is a real one in this
// codebase's history: shipping UI for a capability the binary does not have.
// A "Sign in with Google" button on a stock Cackle is a control that cannot
// work, and a person who presses it concludes the product is broken rather
// than that a setting is missing.
//
// Two halves are checked. The pure list-shaping function is exercised
// directly. The rendering rule — "no providers, no markup" — is checked
// against the SOURCE of the component and its two callers, the same
// technique operator-reset.test.js uses, because a claim about what a page
// does not render is a claim about the file, not about one rendered state.

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { signInProvidersFrom, ssoMessageFor, SSO_MESSAGES } from './sso.ts';

const here = dirname(fileURLToPath(import.meta.url));
const read = (name) => readFileSync(join(here, name), 'utf8');

// ── the list-shaping half ───────────────────────────────────────────────────

test('an unconfigured box yields no providers', () => {
    assert.deepEqual(signInProvidersFrom({ providers: [] }), []);
});

test('every non-answer collapses to no providers', () => {
    for (const payload of [null, undefined, {}, { providers: null }, { providers: 'google' }, { providers: {} }]) {
        assert.deepEqual(signInProvidersFrom(payload), [], `payload ${JSON.stringify(payload)} produced buttons`);
    }
});

test('an entry with no usable start path is not a button', () => {
    const shaped = signInProvidersFrom({
        providers: [
            { id: 'google', label: 'Google' }, // no start_path
            { id: '', label: 'Nameless', start_path: '/api/auth/oauth//start' },
            { id: 'elsewhere', label: 'Elsewhere', start_path: 'https://accounts.example/authorize' },
            { start_path: '/api/auth/oauth/x/start' }, // no id
        ],
    });
    assert.deepEqual(shaped, []);
});

test('a configured provider becomes exactly one button pointed at this box', () => {
    const shaped = signInProvidersFrom({
        providers: [{ id: 'google', label: 'Google', start_path: '/api/auth/oauth/google/start' }],
    });
    assert.equal(shaped.length, 1);
    assert.equal(shaped[0].id, 'google');
    assert.equal(shaped[0].label, 'Google');
    assert.equal(shaped[0].startPath, '/api/auth/oauth/google/start');
    assert.ok(shaped[0].startPath.startsWith('/'), 'the button must navigate to a path on this box');
});

test('a provider with no label falls back to its id rather than to nothing', () => {
    const [only] = signInProvidersFrom({ providers: [{ id: 'google', start_path: '/api/auth/oauth/google/start' }] });
    assert.equal(only.label, 'google');
});

// ── the message half ────────────────────────────────────────────────────────

test('a successful return shows no notice', () => {
    assert.equal(ssoMessageFor('ok'), null);
    assert.equal(ssoMessageFor(null), null);
    assert.equal(ssoMessageFor(''), null);
});

test('an unrecognised reason gets the generic message, never an invented one', () => {
    assert.equal(ssoMessageFor('something-new'), SSO_MESSAGES.failed);
});

test('every message points the person back at the password form', () => {
    for (const [reason, message] of Object.entries(SSO_MESSAGES)) {
        assert.ok(message.length > 0, `${reason} has no message`);
        assert.ok(/^[A-Z]/.test(message), `${reason} does not read like a sentence: ${message}`);
        // "state" is the retry case; the rest must name the fallback that
        // always works, because the fallback always working is the point.
        if (reason !== 'state') {
            assert.match(message, /password/i, `${reason} does not offer the password form: ${message}`);
        }
    }
});

test('the unverified message explains the refusal without blaming the person', () => {
    const m = SSO_MESSAGES.unverified;
    assert.match(m, /confirm/i);
    assert.match(m, /did not sign you in/i);
});

// ── the rendering rule, read off the source ─────────────────────────────────

test('the button row returns nothing when there are no providers', () => {
    const src = read('sso-buttons.tsx');
    assert.match(
        src,
        /if \(providers\.length === 0\) return null;/,
        'sso-buttons.tsx no longer short-circuits on an empty provider list — a button could render on a box with nothing configured',
    );
    // The list starts EMPTY and is only ever filled from the server's answer.
    // (The `(?:<...>)?` tolerates the TS generic — `useState<SsoProvider[]>([])`.)
    assert.match(src, /useState(?:<[^>]*>)?\(\[\]\)/, 'the provider list no longer starts empty');
    // A failed request must not become a rendered button.
    assert.match(src, /\.catch\(\(\) => \{[\s\S]*?setProviders\(\[\]\)/, 'a failed providers request no longer clears the list');
});

test('the app holds no absolute URL to any sign-in provider', () => {
    for (const name of ['sso.ts', 'sso-buttons.tsx', 'signin.jsx', 'signup.jsx']) {
        const src = read(name);
        const absolute = [...src.matchAll(/https?:\/\/[^\s"'`)<>]+/g)].map((m) => m[0]);
        assert.deepEqual(absolute, [], `${name} names an absolute URL: ${absolute.join(', ')}`);
    }
});

test('both auth pages keep the password form and mount the row below it', () => {
    for (const name of ['signin.jsx', 'signup.jsx']) {
        const src = read(name);
        assert.match(src, /<SsoButtons\b/, `${name} does not mount the provider row`);
        assert.match(src, /type="password"/, `${name} lost its password field`);
        assert.ok(
            src.indexOf('type="password"') < src.indexOf('<SsoButtons'),
            `${name} puts the provider row above the password form — the password form is the one that works offline`,
        );
    }
});
