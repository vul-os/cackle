// Prints the measured WCAG contrast of every audited colour pair, in both
// themes, from the real src/index.css.
//
//   node scripts/contrast-report.mjs          # table
//   node scripts/contrast-report.mjs --json   # machine-readable
//
// This is the EVIDENCE side of the accessibility work. The gate side is
// src/lib/contrast.test.js, which asserts the same rows from the same
// definitions (src/lib/contrast-audit.js) and fails the suite on a
// regression. Exits 1 if anything is below threshold, so it is safe to wire
// into CI as a check rather than only a report.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { readThemeTokens } from '../src/lib/theme-tokens.js';
import { auditTheme, GATE_PAIRS, measureLiteralPair } from '../src/lib/contrast-audit.js';
import { round2 } from '../src/lib/contrast.js';

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(here, '..', 'src', 'index.css'), 'utf8');
const { light, dark } = readThemeTokens(css);

const lightRows = auditTheme(light);
const darkRows = auditTheme(dark);
const gateRows = GATE_PAIRS.map(measureLiteralPair);

if (process.argv.includes('--json')) {
    console.log(JSON.stringify({ light: lightRows, dark: darkRows, gate: gateRows }, null, 2));
} else {
    const nameWidth = Math.max(...lightRows.map((r) => r.name.length), ...gateRows.map((r) => r.name.length));

    console.log('\nCackle — measured contrast (WCAG 2.1)\n');
    console.log(
        `${'pair'.padEnd(nameWidth)}  ${'light'.padStart(7)}  ${'dark'.padStart(7)}  ${'min'.padStart(5)}`,
    );
    console.log('-'.repeat(nameWidth + 26));

    for (let i = 0; i < lightRows.length; i++) {
        const l = lightRows[i];
        const d = darkRows[i];
        const mark = l.pass && d.pass ? ' ' : '!';
        console.log(
            `${l.name.padEnd(nameWidth)}  ${(round2(l.ratio) + (l.pass ? '' : '*')).padStart(7)}  ` +
                `${(round2(d.ratio) + (d.pass ? '' : '*')).padStart(7)}  ${String(l.threshold).padStart(5)} ${mark}`,
        );
    }

    console.log('\nGate scan surface (fixed in both themes)\n');
    console.log(`${'pair'.padEnd(nameWidth)}  ${'ratio'.padStart(7)}  ${'min'.padStart(5)}`);
    console.log('-'.repeat(nameWidth + 17));
    for (const g of gateRows) {
        console.log(
            `${g.name.padEnd(nameWidth)}  ${(round2(g.ratio) + (g.pass ? '' : '*')).padStart(7)}  ` +
                `${String(g.threshold).padStart(5)} ${g.pass ? ' ' : '!'}`,
        );
    }

    const all = [...lightRows, ...darkRows, ...gateRows];
    const failures = all.filter((r) => !r.pass);
    const worst = all.reduce((a, b) => (a.ratio < b.ratio ? a : b));
    console.log(
        `\n${all.length} measurements, ${failures.length} below threshold. ` +
            `Lowest: ${worst.name} at ${round2(worst.ratio)}:1 (min ${worst.threshold}).`,
    );
    for (const f of failures) {
        console.log(`  FAIL ${f.name}: ${round2(f.ratio)}:1 < ${f.threshold} (${f.fg} on ${f.bg})`);
    }
}

const failed = [...lightRows, ...darkRows, ...gateRows].some((r) => !r.pass);
process.exit(failed ? 1 : 0);
