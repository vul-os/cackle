// Accessibility gate: every audited colour pair, measured against the real
// stylesheet, in both themes.
//
// This is the test SlipScan did not have. Its audit found tertiary text at
// 2.33:1 — a defect no amount of looking catches and one line of arithmetic
// does. So the arithmetic runs on every `npm test`, over `src/index.css`
// itself rather than a copy of the palette, and a token nudged into
// illegibility fails the build with the measured number attached.
//
// Structured so a failure is immediately actionable: one subtest per theme
// per pair, named for the situation a user is in, reporting the ratio it got
// and the one it needed.

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import {
    parseHsl,
    hexToRgb,
    composite,
    contrastRatio,
    relativeLuminance,
    round2,
    meets,
} from './contrast.js';
import { readThemeTokens, token } from './theme-tokens.js';
import { PAIRS, GATE_PAIRS, measurePair, measureLiteralPair, verdictTokensMatch } from './contrast-audit.js';

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(here, '..', 'index.css'), 'utf8');
const themes = readThemeTokens(css);

// ── The maths itself ────────────────────────────────────────────────────────
// A contrast gate is only worth its assertions if its arithmetic is right, so
// the arithmetic is pinned against values that can be checked by hand.

test('contrast maths', async (t) => {
    await t.test('black on white is 21:1 and white on black is the same', () => {
        assert.equal(round2(contrastRatio('0 0% 0%', '0 0% 100%')), 21);
        assert.equal(round2(contrastRatio('0 0% 100%', '0 0% 0%')), 21);
    });

    await t.test('a colour against itself is 1:1', () => {
        assert.equal(round2(contrastRatio('38 91% 55%', '38 91% 55%')), 1);
    });

    await t.test('relative luminance matches WCAG reference points', () => {
        assert.equal(round2(relativeLuminance('0 0% 100%')), 1);
        assert.equal(relativeLuminance('0 0% 0%'), 0);
    });

    await t.test('parses HSL tokens with and without alpha', () => {
        assert.deepEqual(parseHsl('38 91% 55%'), { h: 38, s: 0.91, l: 0.55, a: 1 });
        assert.deepEqual(parseHsl('0 0% 100% / 0.6'), { h: 0, s: 0, l: 1, a: 0.6 });
    });

    await t.test('refuses to score an unparseable token rather than defaulting it', () => {
        // A gate that silently scores garbage as black has stopped being a gate.
        assert.throws(() => parseHsl('rebeccapurple'), /cannot parse HSL/);
        assert.throws(() => hexToRgb('#12345'), /cannot parse hex/);
        assert.throws(() => token(themes.light, '--no-such-token'), /is not defined/);
    });

    await t.test('the brand token really is #F5A623', () => {
        // The suite's design survey recorded cackle's accent as #F5A623 and
        // site/assets/style.css declares it. The app's token must BE that
        // colour, not a near-miss nobody measured.
        const brand = parseHsl(token(themes.light, '--brand'));
        const { r, g, b } = hexToRgb('#F5A623');
        const asRgb = composite(token(themes.light, '--brand'), '0 0% 100%');
        // Allow 1/255 per channel for the HSL<->RGB round trip.
        assert.ok(Math.abs(asRgb.r - r) <= 1, `brand red ${asRgb.r} vs ${r}`);
        assert.ok(Math.abs(asRgb.g - g) <= 1, `brand green ${asRgb.g} vs ${g}`);
        assert.ok(Math.abs(asRgb.b - b) <= 1, `brand blue ${asRgb.b} vs ${b}`);
        assert.equal(brand.a, 1);
    });

    await t.test('compositing alpha changes the answer (and is therefore not optional)', () => {
        // --sidebar-muted-foreground is white at 60% over the near-black
        // sidebar. Scored raw it looks like pure white; scored honestly it is
        // a full 10 points of ratio lower.
        const raw = contrastRatio('0 0% 100%', '240 12% 6%');
        const composited = contrastRatio('0 0% 100% / 0.6', '240 12% 6%');
        assert.ok(raw > 17, `opaque white measured ${round2(raw)}`);
        assert.ok(composited < 8, `60% white measured ${round2(composited)}`);
    });
});

// ── The stylesheet parse ────────────────────────────────────────────────────

test('theme tokens are read from the shipped stylesheet', async (t) => {
    await t.test('both theme scopes are present and populated', () => {
        assert.ok(Object.keys(themes.light).length > 25);
        assert.ok(Object.keys(themes.dark).length > 25);
    });

    await t.test('dark inherits every token it does not override', () => {
        // .dark only redefines what differs. Measuring the block alone would
        // test a palette no browser ever renders — and would skip exactly the
        // inherited tokens where an unreadable value could hide.
        assert.equal(themes.dark['--brand'], themes.light['--brand']);
        assert.equal(themes.dark['--sidebar-background'], themes.light['--sidebar-background']);
        assert.notEqual(themes.dark['--background'], themes.light['--background']);
    });

    await t.test('a token mentioned only in a comment is not read as a declaration', () => {
        // The :root block's prose names --primary-emphasis and --brand-2
        // several times; comment stripping is what keeps those from being
        // parsed as live values.
        assert.equal(themes.light['--primary-emphasis'], '30 92% 32%');
    });
});

// ── The audit ───────────────────────────────────────────────────────────────

for (const themeName of ['light', 'dark']) {
    test(`WCAG AA — ${themeName} theme`, async (t) => {
        for (const pair of PAIRS) {
            await t.test(`${pair.name} clears ${pair.threshold}:1`, () => {
                const result = measurePair(themes[themeName], pair);
                assert.ok(
                    result.pass,
                    `${pair.name} (${themeName}): ${round2(result.ratio)}:1, needs ${pair.threshold}:1 ` +
                        `— ${pair.fg} on ${pair.bg}`,
                );
            });
        }
    });
}

test('WCAG AA — gate scan surface', async (t) => {
    for (const pair of GATE_PAIRS) {
        await t.test(`${pair.name} clears ${pair.threshold}:1`, () => {
            const result = measureLiteralPair(pair);
            assert.ok(
                result.pass,
                `${pair.name}: ${round2(result.ratio)}:1, needs ${pair.threshold}:1`,
            );
        });
    }

    await t.test('the audited verdict fills are the ones index.css actually ships', () => {
        // GATE_PAIRS holds literals because .gate-surface is theme-independent.
        // That duplication is only safe while it is checked.
        const mismatches = verdictTokensMatch(themes.light);
        assert.deepEqual(
            mismatches,
            [],
            'verdict tokens drifted from the audit: ' + JSON.stringify(mismatches),
        );
        assert.deepEqual(verdictTokensMatch(themes.dark), []);
    });
});

// ── The rules the palette is built on ───────────────────────────────────────

test('palette invariants', async (t) => {
    await t.test('the brand fill is NOT usable as ink on a light surface', () => {
        // This is the measurement the whole --primary-emphasis split exists
        // for. Asserting it (rather than only asserting the fix) means that if
        // somebody ever "simplifies" the two tokens back into one, this test
        // explains why they cannot.
        const ratio = contrastRatio(token(themes.light, '--primary'), token(themes.light, '--background'));
        assert.ok(
            !meets(ratio, 4.5),
            `--primary on --background measured ${round2(ratio)}:1; if this now passes, ` +
                'the brand colour changed and --primary-emphasis should be revisited',
        );
    });

    await t.test('brand ink and the focus ring are legible in both themes', () => {
        for (const themeName of ['light', 'dark']) {
            const scope = themes[themeName];
            assert.ok(meets(contrastRatio(token(scope, '--primary-emphasis'), token(scope, '--background')), 4.5));
            assert.ok(meets(contrastRatio(token(scope, '--ring'), token(scope, '--background')), 3));
        }
    });

    await t.test('every verdict flood carries its text at AA in both themes', () => {
        for (const themeName of ['light', 'dark']) {
            const scope = themes[themeName];
            for (const v of ['--verdict-admit', '--verdict-reject', '--verdict-duplicate']) {
                const ratio = contrastRatio(token(scope, '--verdict-ink'), token(scope, v));
                assert.ok(meets(ratio, 4.5), `${v} (${themeName}) carries ink at ${round2(ratio)}:1`);
            }
        }
    });

    await t.test('the verdict palette does not shift between themes', () => {
        // A gate runs the same screen at dusk and at noon. The answer it gives
        // must not change shade because of a theme toggle nobody touched.
        for (const v of ['--verdict-admit', '--verdict-reject', '--verdict-duplicate', '--verdict-ink']) {
            assert.equal(token(themes.light, v), token(themes.dark, v), `${v} differs between themes`);
        }
    });

    await t.test('"already scanned" is held clear of the brand amber', () => {
        // Both are warm and mid-hue; the duplicate state must never read as
        // brand chrome. Compared as hue distance, which is the axis that
        // actually confuses them — contrast ratio would not catch this.
        const brandHue = parseHsl(token(themes.light, '--brand')).h;
        const dupHue = parseHsl(token(themes.light, '--verdict-duplicate')).h;
        assert.ok(Math.abs(brandHue - dupHue) >= 8, `brand h=${brandHue}, duplicate h=${dupHue}`);
    });
});
