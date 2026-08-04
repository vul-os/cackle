// These tests exist because the thing they guard was silently broken for
// the entire life of the feature: the team page created an invite, threw
// the token away, and toasted "Invite sent" when Cackle contains no mail
// code at all. Nobody could accept an invite, so nobody could be added as
// a scanner, so nobody could staff a door.
//
// So they are deliberately not just "does the string concatenate". They
// pin the link to the route the app really declares, and they read the
// shipped copy back and fail if it starts claiming delivery again.

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { buildInviteUrl, copyTextToClipboard, INVITE_ACCEPT_PATH, INVITE_TOKEN_PARAM } from './invite-link.js';

const here = dirname(fileURLToPath(import.meta.url));
const webSrc = join(here, '..', '..', '..');
const read = (p) => readFileSync(join(webSrc, p), 'utf8');

test('buildInviteUrl produces the URL an invited person opens', () => {
    assert.equal(
        buildInviteUrl('https://tickets.example.org', 'abc123'),
        'https://tickets.example.org/accept-invite?token=abc123',
    );
});

test('buildInviteUrl works for a LAN origin with a port — the venue-laptop case', () => {
    assert.equal(
        buildInviteUrl('http://192.168.1.20:8080', 'abc123'),
        'http://192.168.1.20:8080/accept-invite?token=abc123',
    );
});

test('buildInviteUrl does not double the slash on a trailing-slash origin', () => {
    assert.equal(buildInviteUrl('https://example.org/', 'abc123'), 'https://example.org/accept-invite?token=abc123');
    assert.equal(buildInviteUrl('https://example.org///', 'abc123'), 'https://example.org/accept-invite?token=abc123');
});

test('buildInviteUrl percent-encodes a token that would otherwise corrupt the query', () => {
    // Today's tokens are base64url and survive untouched; this is the
    // guard for the day the token alphabet changes underneath us.
    assert.equal(
        buildInviteUrl('https://example.org', 'a+b/c&d=e'),
        'https://example.org/accept-invite?token=a%2Bb%2Fc%26d%3De',
    );
    const url = new URL(buildInviteUrl('https://example.org', 'a+b/c&d=e'));
    assert.equal(url.searchParams.get('token'), 'a+b/c&d=e', 'the token must survive a round trip through the URL');
});

test('buildInviteUrl refuses to build a link with no token', () => {
    // A tokenless link renders "Missing invite link" for the recipient and
    // looks identical to a working one to the sender. Fail loudly instead.
    assert.throws(() => buildInviteUrl('https://example.org', ''), TypeError);
    assert.throws(() => buildInviteUrl('https://example.org', '   '), TypeError);
    assert.throws(() => buildInviteUrl('https://example.org', undefined), TypeError);
    assert.throws(() => buildInviteUrl('https://example.org', null), TypeError);
    assert.throws(() => buildInviteUrl('', 'abc123'), TypeError);
});

test('the invite link points at a route the app actually declares', () => {
    // If someone renames or removes the accept route, every invite link
    // ever generated becomes a 404 and this goes red instead.
    const routes = read('routes.tsx');
    assert.match(
        routes,
        new RegExp(`path=["']${INVITE_ACCEPT_PATH}["']`),
        `routes.tsx declares no ${INVITE_ACCEPT_PATH} route, so every invite link is dead`,
    );

    const acceptPage = read('pages/organizers/invite-accept.tsx');
    assert.match(
        acceptPage,
        new RegExp(`searchParams\\.get\\(['"]${INVITE_TOKEN_PARAM}['"]\\)`),
        `the accept page does not read the "${INVITE_TOKEN_PARAM}" query parameter the link uses`,
    );
});

test('an invite link opened signed-out survives the sign-in detour with its token', () => {
    // Found by driving this for real: the link bounced through /login and
    // came back WITHOUT ?token=, so the invitee was told their link was
    // missing its code and could not join. Two independent causes, both
    // asserted here because either one alone reproduces it.
    //
    // 1. The global 401 handler recorded window.location.pathname only.
    //    useAuthRedirect prefers that router state over the path
    //    ProtectedRoute persists, so a bare pathname does not merely lose
    //    the token — it overrides the copy that still had it.
    const auth = read('context/use-auth.tsx');
    assert.match(
        auth,
        /window\.location\.pathname \+ window\.location\.search/,
        'the 401 redirect drops the query string, so /accept-invite?token=… comes back tokenless',
    );

    // 2. AuthProvider's boot probe passes {skipAuthRedirect:true} because
    //    an anonymous visitor's 401 is normal. `auth.me` took no
    //    arguments and silently dropped it, so every signed-out page load
    //    fired the 401 redirect in the first place.
    const api = read('lib/api.ts');
    assert.match(
        api,
        /me:\s*\(opts[^)]*\)\s*=>\s*request\('\/auth\/me',\s*\{[^}]*\.\.\.opts/,
        'auth.me does not forward its options, so skipAuthRedirect is ignored on every anonymous page load',
    );

    // 3. ProtectedRoute's persisted copy must keep the search too.
    const guard = read('components/auth/protected-route.jsx');
    assert.match(guard, /location\.pathname \+ location\.search/, 'ProtectedRoute persists a tokenless path');

    // 4. /accept-invite must be a path the 401 handler recognises as
    //    protected, or it never records a return-to at all.
    assert.match(auth, /PROTECTED_PREFIXES = \[[^\]]*'\/accept-invite'/, 'the 401 handler does not treat /accept-invite as protected');
});

test('the team page keeps the token it is given instead of discarding it', () => {
    const team = read('pages/organizers/team/index.jsx');
    assert.match(
        team,
        /const created = await orgMembersApi\.invite\(/,
        'the invite response must be captured — discarding it is what made the whole feature unusable',
    );
    assert.match(team, /buildInviteUrl\(window\.location\.origin, token\)/, 'the captured token must become a link');
    assert.match(team, /setNewInvite\(/, 'the link must reach the UI');
});

test('nothing in the invite flow claims an email was sent', () => {
    // Cackle has no SMTP client, no provider SDK and no sender of any
    // kind. Any copy implying delivery is a lie the user will act on by
    // waiting for something that never arrives.
    const files = {
        'pages/organizers/team/index.jsx': read('pages/organizers/team/index.jsx'),
        'pages/organizers/team/invite-link-card.jsx': read('pages/organizers/team/invite-link-card.jsx'),
        'pages/organizers/invite-accept.tsx': read('pages/organizers/invite-accept.tsx'),
    };
    const forbidden = [
        /Invite sent/i,
        /they'?(ll| will) receive/i,
        /check your inbox/i,
        /invite email/i,
        /we'?(ve| have) (sent|emailed)/i,
        /on its way/i,
    ];
    for (const [name, source] of Object.entries(files)) {
        for (const pattern of forbidden) {
            assert.equal(
                pattern.test(source),
                false,
                `${name} claims an email was sent (matched ${pattern}) — Cackle sends no email`,
            );
        }
    }
});

test('the team page states the expiry and the shown-once rule', () => {
    const card = read('pages/organizers/team/invite-link-card.jsx');
    assert.match(card, /stops working on/i, 'the expiry must be stated plainly');
    assert.match(card, /only time the link is shown/i, 'the shown-once rule must be stated');
    assert.match(card, /revoke the invite/i, 'the recovery path for a lost link must be stated');
});

test('the invite token is never written to storage', () => {
    // A bearer credential persisted on a shared venue laptop outlives the
    // person who created it. Component state only.
    for (const p of ['pages/organizers/team/index.jsx', 'pages/organizers/team/invite-link-card.jsx', 'pages/organizers/team/invite-link.js']) {
        const source = read(p);
        assert.equal(/localStorage|sessionStorage|document\.cookie/.test(source), false, `${p} persists the invite token`);
    }
});

test('copyTextToClipboard reports failure instead of lying about it', async () => {
    // The async Clipboard API needs a secure context and a self-hosted
    // Cackle on a venue LAN is usually plain http. A false "Copied!" is
    // how someone ends up pasting nothing into WhatsApp.
    assert.equal(await copyTextToClipboard('', {}, {}), false);
    assert.equal(await copyTextToClipboard('link', undefined, undefined), false);

    const rejecting = { clipboard: { writeText: () => Promise.reject(new Error('not allowed')) } };
    assert.equal(await copyTextToClipboard('link', undefined, rejecting), false, 'a rejected write must not report success');

    let written = null;
    const working = { clipboard: { writeText: (t) => { written = t; return Promise.resolve(); } } };
    assert.equal(await copyTextToClipboard('link', undefined, working), true);
    assert.equal(written, 'link');
});

test('copyTextToClipboard falls back to the legacy path when there is no Clipboard API', async () => {
    const appended = [];
    const el = { style: {}, setAttribute() {}, select() {} };
    const fakeDoc = {
        createElement: () => el,
        body: { appendChild: (n) => appended.push(n), removeChild: () => {} },
        execCommand: () => true,
    };
    assert.equal(await copyTextToClipboard('link', fakeDoc, {}), true);
    assert.equal(appended.length, 1);
    assert.equal(el.value, 'link');

    const failingDoc = { ...fakeDoc, execCommand: () => false };
    assert.equal(await copyTextToClipboard('link', failingDoc, {}), false);
});
