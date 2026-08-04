// The gate's four verdicts, as the door staff experience them.
//
// Separated from the view so the copy and the semantics can be asserted in a
// test (`verdict.test.js`) rather than only read off a JSX tree. What a gate
// says when it refuses somebody is product surface, not decoration.
//
// # Three rules encoded here
//
// 1. COLOUR IS NEVER THE ONLY SIGNAL. Every verdict carries a distinct icon, a
//    distinct word, and a distinct haptic pattern. Roughly 1 in 12 men has a
//    red/green colour vision deficiency, and a gate is exactly the situation —
//    glanced at, in bad light, at speed — where a colour-only signal fails.
//
// 2. NOTHING CLAIMS PREVENTION. A duplicate is reported as an observation
//    ("already scanned"), never as a block, because two partitioned gates
//    cannot be stopped from admitting the same ticket. The `where` field
//    exists so the operator learns which gate saw it — this one, or another —
//    since that is the difference between a person trying it twice at one door
//    and a genuine cross-gate double-entry.
//
// 3. THE VERB IS WHAT THE STAFF MEMBER DOES. "Let them in" / "Do not let them
//    in" beats "valid" / "invalid" when the reader has three seconds and a
//    queue behind the person in front of them.

import { ShieldCheck, Ban, CopyCheck, MapPinOff } from 'lucide-react';

/**
 * VERDICTS maps a scan result to everything the gate screen needs.
 *
 * `vibrate` is a navigator.vibrate pattern in ms. The patterns are
 * deliberately unalike in RHYTHM, not just length: a single short pulse for
 * admit, a long buzz for refusal, a double tap for a duplicate. In a loud
 * venue with the screen glare-washed, the phone in your hand is sometimes the
 * only channel that gets through.
 */
export const VERDICTS = {
    admitted: {
        label: 'LET THEM IN',
        icon: ShieldCheck,
        surface: 'bg-verdict-admit',
        vibrate: [70],
        tone: 'admit',
    },
    duplicate: {
        label: 'ALREADY SCANNED',
        icon: CopyCheck,
        surface: 'bg-verdict-duplicate',
        vibrate: [40, 80, 40],
        tone: 'duplicate',
    },
    invalid: {
        label: 'DO NOT LET THEM IN',
        icon: Ban,
        surface: 'bg-verdict-reject',
        vibrate: [400],
        tone: 'reject',
    },
    wrong_event: {
        label: 'WRONG EVENT',
        icon: MapPinOff,
        surface: 'bg-verdict-reject',
        vibrate: [200, 100, 200],
        tone: 'reject',
    },
};

type VerdictKind = keyof typeof VERDICTS;

function isVerdictKind(value: string): value is VerdictKind {
    return value in VERDICTS;
}

/**
 * verdictFor returns the presentation for a result, falling back to the
 * refusal treatment for anything unrecognised.
 *
 * The fallback direction matters and is the same fail-closed instinct as the
 * rest of the scan path: a result string this build does not know about is
 * shown as a refusal, never as an admission. A future server that adds a
 * verdict will make old gates refuse rather than wave people through.
 */
export function verdictFor(result: string) {
    return isVerdictKind(result) ? VERDICTS[result] : VERDICTS.invalid;
}

/**
 * How long a verdict stays on screen before the camera comes back.
 *
 * An admission clears fast because the queue is moving and the answer is
 * "yes". Everything else lingers, because a refusal needs to be read, and
 * because the natural reflex on a refusal is to scan again immediately — which
 * would bury the reason under a second identical flood.
 *
 */
export function verdictHoldMs(result: string) {
    return result === 'admitted' ? 1100 : 2400;
}

/**
 * describeDuplicate turns a duplicate's note into the question the operator
 * actually has: was this the same door, or a different one?
 *
 * The distinction is the whole reason cross-gate reconciliation exists. A
 * repeat at THIS gate is somebody trying it twice at one door and being caught
 * in the act. A ticket already admitted at ANOTHER gate means the two gates
 * could not see each other and a person may already be inside — which is
 * detected here and, unavoidably, not prevented.
 *
 * @returns {{ where: 'this-gate'|'other-gate'|'unknown', detail: string }}
 */
export function describeDuplicate(note: string | null) {
    if (typeof note === 'string' && /another gate/i.test(note)) {
        return {
            where: 'other-gate',
            detail: 'Another gate already admitted this ticket. Someone may already be inside on it.',
        };
    }
    if (typeof note === 'string' && /this gate/i.test(note)) {
        return {
            where: 'this-gate',
            detail: 'This gate already admitted this ticket.',
        };
    }
    return { where: 'unknown', detail: note || 'This ticket has already been scanned.' };
}
