// The forgot-password page told users "Reset email sent — check your
// inbox" for the whole life of this product, while the repository
// contained no mail code of any kind. Anyone who believed it waited for a
// message that was never coming and stayed locked out.
//
// These tests guard the replacement: the page must name the real path
// (the operator, and the exact command), and must never claim delivery
// again.

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { resetCommandFor, RESET_SUBCOMMAND } from './operator-reset.ts';

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..', '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');

test('resetCommandFor builds the command an operator actually runs', () => {
    assert.equal(resetCommandFor('venue@example.com'), "cackle reset-password -email 'venue@example.com'");
});

test('resetCommandFor stays sensible before anything is typed', () => {
    assert.equal(resetCommandFor(''), "cackle reset-password -email 'their@email.address'");
    assert.equal(resetCommandFor('   '), "cackle reset-password -email 'their@email.address'");
    assert.equal(resetCommandFor(undefined), "cackle reset-password -email 'their@email.address'");
});

test('resetCommandFor quotes the address so the command cannot mean something else', () => {
    assert.equal(resetCommandFor("o'brien@example.com"), `cackle reset-password -email 'o'\\''brien@example.com'`);
    assert.equal(resetCommandFor('a b@example.com'), "cackle reset-password -email 'a b@example.com'");
    assert.equal(resetCommandFor('$(whoami)@example.com'), "cackle reset-password -email '$(whoami)@example.com'");
});

test('the command names a subcommand the binary really implements', () => {
    // If the Go side is renamed, this page starts telling people to run
    // something that does not exist. Go red instead.
    const cli = read('cmd/cackle/main.go');
    assert.match(cli, /"reset-password":\s*resetPasswordCmd/, 'cmd/cackle registers no reset-password subcommand');

    const impl = read('cmd/cackle/resetpassword.go');
    const [, flagName] = RESET_SUBCOMMAND.match(/^cackle (\S+)$/);
    assert.equal(flagName, 'reset-password');
    assert.match(impl, /fs\.StringVar\(&email, "email"/, 'the subcommand does not take -email');
    assert.match(impl, /update-password\?token=/, 'the subcommand does not print an /update-password link');
});

test('the reset link the CLI prints lands on a route the app declares', () => {
    const routes = read('web/src/routes.tsx');
    assert.match(routes, /path="\/update-password"/, 'no /update-password route — every printed reset link is dead');

    const page = read('web/src/pages/auth/update-password.tsx');
    assert.match(page, /searchParams\.get\('token'\)/, 'the update-password page does not read the token from the URL');
});

test('nothing in the reset flow claims an email was sent', () => {
    const files = {
        'web/src/pages/auth/forgot-password.tsx': read('web/src/pages/auth/forgot-password.tsx'),
        'web/src/pages/auth/update-password.tsx': read('web/src/pages/auth/update-password.tsx'),
        'web/src/pages/auth/operator-reset.ts': read('web/src/pages/auth/operator-reset.ts'),
    };
    const forbidden = [
        /reset email sent/i,
        /check your inbox/i,
        /on its way/i,
        /we'?(ve| have) (sent|emailed)/i,
        /sending reset link/i,
        /send reset link/i,
    ];
    for (const [name, source] of Object.entries(files)) {
        for (const pattern of forbidden) {
            assert.equal(pattern.test(source), false, `${name} claims a reset email was sent (matched ${pattern})`);
        }
    }
});

test('the forgot-password page states the truth and gives a real path', () => {
    const page = read('web/src/pages/auth/forgot-password.tsx');
    assert.match(page, /does not send email/i, 'the page must say plainly that no email is coming');
    assert.match(page, /resetCommandFor\(email\)/, 'the page must show the operator command for the address typed');
    // And it must no longer fire the API call whose token nobody can read.
    assert.equal(/requestPasswordReset/.test(page), false, 'the page still calls the reset API, which mints a token nothing delivers');
});
