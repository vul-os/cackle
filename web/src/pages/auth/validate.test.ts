import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import {
    isEmailShaped,
    emailError,
    requiredError,
    newPasswordError,
    confirmPasswordError,
    MIN_PASSWORD_LENGTH,
} from './validate.ts';

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..', '..', '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');

test('MIN_PASSWORD_LENGTH matches the Go source it has to agree with', () => {
    // If internal/auth/service.go's floor ever moves, this pins the UI to
    // notice rather than silently blocking (or silently allowing) something
    // the server disagrees with.
    const service = read('internal/auth/service.go');
    assert.match(service, /MinPasswordLength = 8/, 'internal/auth/service.go no longer sets MinPasswordLength = 8');
    assert.equal(MIN_PASSWORD_LENGTH, 8);
});

test('isEmailShaped accepts an ordinary address and rejects the obvious non-shapes', () => {
    assert.equal(isEmailShaped('venue@example.com'), true);
    assert.equal(isEmailShaped('  venue@example.com  '), true);
    for (const bad of ['', 'venue', 'venue@', '@example.com', 'venue example.com', 'venue@example']) {
        assert.equal(isEmailShaped(bad), false, `expected "${bad}" to be rejected`);
    }
});

test('emailError says something distinct for empty vs malformed vs fine', () => {
    assert.match(emailError(''), /enter/i);
    assert.match(emailError('   '), /enter/i);
    assert.match(emailError('not-an-email'), /look like/i);
    assert.equal(emailError('venue@example.com'), null);
});

test('requiredError names the field it is missing', () => {
    assert.equal(requiredError('', 'your name'), 'Enter your name.');
    assert.equal(requiredError('   ', 'your name'), 'Enter your name.');
    assert.equal(requiredError('Nomvula', 'your name'), null);
});

test('newPasswordError enforces the same floor the server does', () => {
    assert.match(newPasswordError(''), /enter a password/i);
    assert.match(newPasswordError('short1'), /at least 8/i);
    assert.equal(newPasswordError('eightplus'), null);
});

test('confirmPasswordError distinguishes empty from mismatched', () => {
    assert.match(confirmPasswordError('eightplus', ''), /again/i);
    assert.match(confirmPasswordError('eightplus', 'different'), /match/i);
    assert.equal(confirmPasswordError('eightplus', 'eightplus'), null);
});
