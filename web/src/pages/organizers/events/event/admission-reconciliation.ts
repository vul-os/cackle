// Pure shaping of GET /api/events/{id}/admission-conflicts into what the
// admissions screen renders. Separated from the JSX for the same reason
// ../../scanner/verdict.ts is separated from scan-view.tsx: what an organiser
// is actually told is product surface, and belongs in something a test can
// read, not only a component tree.
//
// One rule holds this whole screen together, and it is the reason this file
// exists rather than trusting a component to phrase it correctly every time:
//
//   DETECTED, NEVER PREVENTED. Two gates partitioned from each other cannot
//   coordinate, so a cross-gate double admission is discovered after the
//   fact, from logs that have synced, and only ever as completely as those
//   logs have arrived. Nothing derived here may read as a guarantee, and a
//   malformed or stripped-down response must still carry that caveat rather
//   than silently rendering none.

/**
 * NEVER_PREVENTED_NOTE is the fallback caveat, used only if a response
 * somehow arrives without its own `caveat` (a legacy server, a proxy that
 * stripped it, a hand-built test fixture). The screen must never show NO
 * caveat — an empty string reads as nothing to caveat, which is the one
 * reading this product can never allow.
 */
export const NEVER_PREVENTED_NOTE =
    'Two gates that cannot see each other cannot be stopped from admitting the same ticket. This report finds it afterwards; it does not prevent it.';

/**
 * gateLabel returns what to show for a claim's origin: its gate_id, or a
 * short device-id fallback for the rare claim that was never given one.
 *
 * @param {{gate_id?: string, device_id: string}} claim
 */
export function gateLabel(claim) {
    if (claim?.gate_id) return claim.gate_id;
    const device = claim?.device_id ? claim.device_id.slice(0, 8) : 'unknown';
    return `Unlabelled gate (device ${device})`;
}

/**
 * orderClaims returns one conflict's claims in scan order, each annotated
 * with:
 *   - `extra`: true for every claim after the first. The first, earliest
 *     admission is the one legitimate entry; everyone after it at any gate
 *     is a body a signature alone could never have stopped.
 *   - `downgraded`: the server's stored row disagrees with what this device
 *     reported — i.e. this claim lost the reconciliation and was written up
 *     as 'duplicate' even though the device itself admitted.
 *
 * @param {{claims?: Array}} conflict
 */
export function orderClaims(conflict) {
    const claims = [...(conflict?.claims ?? [])].sort(
        (a, b) => new Date(a.scanned_at).getTime() - new Date(b.scanned_at).getTime(),
    );
    return claims.map((claim, i) => ({
        ...claim,
        extra: i > 0,
        downgraded: Boolean(claim.server_result) && claim.server_result !== claim.result,
    }));
}

/**
 * summarize turns the raw admission-conflicts response into the numbers and
 * the gate breakdown the screen leads with.
 *
 * `gates` is every gate that contributed at least one EXTRA admission
 * (never the first, legitimate one), sorted by how many it contributed,
 * descending — "which door(s) does this affect" is the operational
 * question, and an organiser checking in with door staff wants the busiest
 * one first. Ties break alphabetically so the order is stable across
 * re-fetches of the same data.
 *
 * @param {object} response the admission-conflicts JSON body
 */
export function summarize(response) {
    const conflicts = Array.isArray(response?.conflicts) ? response.conflicts : [];
    const gateCounts = new Map();

    for (const conflict of conflicts) {
        for (const claim of orderClaims(conflict)) {
            if (!claim.extra) continue;
            const label = gateLabel(claim);
            gateCounts.set(label, (gateCounts.get(label) ?? 0) + 1);
        }
    }

    const gates = [...gateCounts.entries()]
        .map(([gate, count]) => ({ gate, count }))
        .sort((a, b) => b.count - a.count || a.gate.localeCompare(b.gate));

    return {
        extraAdmissions: response?.extra_admissions ?? 0,
        ticketsAffected: conflicts.length,
        gates,
        complete: response?.complete ?? true,
        caveat: response?.caveat || NEVER_PREVENTED_NOTE,
        algebra: response?.algebra ?? '',
        engine: response?.engine ?? '',
    };
}
