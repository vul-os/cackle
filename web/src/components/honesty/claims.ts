/**
 * claims.js — the sentences this product is not allowed to get wrong.
 *
 * `site/index.html` states the build status and the cross-gate limit exactly
 * right, and `scripts/check-site.mjs` has gated that copy since the day it was
 * written. The React app — the copy that actually ships INSIDE the binary —
 * had neither the statements nor the gate. That asymmetry is how a fabricated
 * pricing page survived in `web/src/` while the marketing site next to it was
 * scrupulously honest.
 *
 * So the two load-bearing claims live here, once, as exported strings, and
 * `scripts/check-app.mjs` asserts them from this file outward: that the text
 * still says what it must, that a layout actually renders it, and that nothing
 * anywhere in `web/src/` upgrades it into a promise the software cannot keep.
 *
 * Editing the strings below is a claim change. The gate will tell you.
 */

/**
 * Cackle is not production software and the app has to say so where a person
 * making a decision will read it. `site/index.html` says it above the fold;
 * this is the app's equivalent.
 */
export const BUILD_STATUS_LABEL = 'Experimental — not production-ready.';

export const BUILD_STATUS_DETAIL =
    'This is a working build, not a finished product. Do not run a real gate or take real money on it without reading the docs first.';

/**
 * THE safety claim. A venue staffs its doors around this sentence.
 *
 * Two gates that are both offline are partitioned from each other; there is no
 * merge rule that invents coordination they never had. The second door WILL
 * open. Cackle's contribution is that you find out — afterwards, from the
 * reconciliation report — not that it is stopped. Anything stronger than that
 * is a lie with a queue of people behind it.
 */
export const CROSS_GATE_LIMIT_LABEL = 'Detected, never prevented.';

export const CROSS_GATE_LIMIT_DETAIL =
    'Two doors that are both offline cannot talk to each other. If someone tries the same ticket at both, Cackle will show you it happened — after the gates sync. It cannot stop it at the second door.';

/**
 * Cackle takes no cut. There is no billing, metering, invoicing or fee-split
 * code anywhere in this repository — verified by grep, not by memory. Whatever
 * a payment processor charges is between the organiser and that processor.
 */
export const NO_FEE_STATEMENT =
    'Cackle charges you nothing. There is no billing system in this software and no fee is taken from a ticket sale.';
