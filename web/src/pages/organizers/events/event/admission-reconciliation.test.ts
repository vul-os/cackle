import { test } from 'node:test';
import assert from 'node:assert/strict';
import { summarize, orderClaims, gateLabel, NEVER_PREVENTED_NOTE } from './admission-reconciliation.ts';

// Fixture mirroring the server's own end-to-end test
// (TestAdmissionConflicts_TwoPartitionedGatesSurfacedAfterSync): gate North's
// device-A admits first, gate South's device-B admits 90s later, both saw the
// same ticket, neither could have known about the other.
function twoGateConflict() {
    return {
        ticket_id: 'ticket-shared',
        devices: 2,
        extra_admissions: 1,
        claims: [
            { device_id: 'device-B', gate_id: 'South', scanned_at: '2026-01-01T00:01:30Z', result: 'admitted', server_result: 'duplicate' },
            { device_id: 'device-A', gate_id: 'North', scanned_at: '2026-01-01T00:00:00Z', result: 'admitted' },
        ],
    };
}

test('summarize: no conflicts is zero everywhere, never "undefined"', () => {
    const s = summarize({ conflicts: [], extra_admissions: 0, complete: true, caveat: 'caveat text', algebra: 'a', engine: 'e' });
    assert.equal(s.extraAdmissions, 0);
    assert.equal(s.ticketsAffected, 0);
    assert.deepEqual(s.gates, []);
    assert.equal(s.complete, true);
});

test('summarize: the two-gate scenario attributes the extra admission to the LATER gate only', () => {
    const s = summarize({
        conflicts: [twoGateConflict()],
        extra_admissions: 1,
        complete: true,
        caveat: 'caveat text',
        algebra: 'dmtap-v1',
        engine: 'substrate-1.0',
    });
    assert.equal(s.extraAdmissions, 1);
    assert.equal(s.ticketsAffected, 1);
    // North was first (legitimate); South is where the extra body got in.
    assert.deepEqual(s.gates, [{ gate: 'South', count: 1 }]);
    assert.equal(s.algebra, 'dmtap-v1');
    assert.equal(s.engine, 'substrate-1.0');
});

test('summarize: aggregates extra admissions across multiple conflicts at the same gate', () => {
    const second = {
        ticket_id: 'ticket-2',
        devices: 2,
        extra_admissions: 1,
        claims: [
            { device_id: 'device-C', gate_id: 'East', scanned_at: '2026-01-01T00:00:00Z', result: 'admitted' },
            { device_id: 'device-D', gate_id: 'South', scanned_at: '2026-01-01T00:05:00Z', result: 'admitted', server_result: 'duplicate' },
        ],
    };
    const s = summarize({ conflicts: [twoGateConflict(), second], extra_admissions: 2, complete: true, caveat: 'x' });
    assert.equal(s.extraAdmissions, 2);
    assert.equal(s.ticketsAffected, 2);
    assert.deepEqual(s.gates, [{ gate: 'South', count: 2 }]);
});

test('summarize: three-way conflict counts every claim after the first as extra', () => {
    const threeWay = {
        ticket_id: 'ticket-3',
        devices: 3,
        extra_admissions: 2,
        claims: [
            { device_id: 'd1', gate_id: 'North', scanned_at: '2026-01-01T00:00:00Z', result: 'admitted' },
            { device_id: 'd2', gate_id: 'South', scanned_at: '2026-01-01T00:01:00Z', result: 'admitted', server_result: 'duplicate' },
            { device_id: 'd3', gate_id: 'East', scanned_at: '2026-01-01T00:02:00Z', result: 'admitted', server_result: 'duplicate' },
        ],
    };
    const s = summarize({ conflicts: [threeWay], extra_admissions: 2, complete: true, caveat: 'x' });
    assert.equal(s.extraAdmissions, 2);
    assert.deepEqual(
        s.gates.sort((a, b) => a.gate.localeCompare(b.gate)),
        [{ gate: 'East', count: 1 }, { gate: 'South', count: 1 }],
    );
});

test('summarize: ties break alphabetically so the order is stable', () => {
    const conflict = {
        ticket_id: 't',
        devices: 3,
        extra_admissions: 2,
        claims: [
            { device_id: 'd1', gate_id: 'North', scanned_at: '2026-01-01T00:00:00Z', result: 'admitted' },
            { device_id: 'd2', gate_id: 'Zeta', scanned_at: '2026-01-01T00:01:00Z', result: 'admitted', server_result: 'duplicate' },
            { device_id: 'd3', gate_id: 'Alpha', scanned_at: '2026-01-01T00:02:00Z', result: 'admitted', server_result: 'duplicate' },
        ],
    };
    const s = summarize({ conflicts: [conflict], extra_admissions: 2, complete: true, caveat: 'x' });
    assert.deepEqual(s.gates, [{ gate: 'Alpha', count: 1 }, { gate: 'Zeta', count: 1 }]);
});

test('summarize: a missing/blank caveat falls back to the never-prevented note, never to silence', () => {
    const withNoCaveat = summarize({ conflicts: [], extra_admissions: 0, complete: true, caveat: '' });
    assert.equal(withNoCaveat.caveat, NEVER_PREVENTED_NOTE);

    const malformed = summarize(null);
    assert.equal(malformed.caveat, NEVER_PREVENTED_NOTE);
    assert.equal(malformed.extraAdmissions, 0);
    assert.equal(malformed.ticketsAffected, 0);
});

test('summarize: an incomplete report says so rather than defaulting to "done"', () => {
    const s = summarize({ conflicts: [], extra_admissions: 0, complete: false, caveat: 'x' });
    assert.equal(s.complete, false);
});

test('orderClaims: marks the earliest claim as not-extra and every later one as extra', () => {
    const ordered = orderClaims(twoGateConflict());
    assert.equal(ordered.length, 2);
    assert.equal(ordered[0].device_id, 'device-A');
    assert.equal(ordered[0].extra, false);
    assert.equal(ordered[0].downgraded, false);
    assert.equal(ordered[1].device_id, 'device-B');
    assert.equal(ordered[1].extra, true);
});

test('orderClaims: downgraded is only true when server_result is present and differs from result', () => {
    const [winner, loser] = orderClaims(twoGateConflict());
    assert.equal(winner.downgraded, false, 'the winning claim was not rewritten by the server');
    assert.equal(loser.downgraded, true, 'the losing claim was downgraded to duplicate server-side');

    // A claim that carries server_result equal to its own reported result
    // (possible if a caller echoes it back) must not read as downgraded.
    const echoed = orderClaims({
        claims: [{ device_id: 'd1', gate_id: 'North', scanned_at: '2026-01-01T00:00:00Z', result: 'admitted', server_result: 'admitted' }],
    });
    assert.equal(echoed[0].downgraded, false);
});

test('orderClaims: tolerates a conflict with no claims array', () => {
    assert.deepEqual(orderClaims({}), []);
    assert.deepEqual(orderClaims(null), []);
});

test('gateLabel: falls back to a short device id when gate_id is absent', () => {
    assert.equal(gateLabel({ gate_id: 'North', device_id: 'device-A' }), 'North');
    assert.equal(gateLabel({ device_id: 'abcdefgh12345' }), 'Unlabelled gate (device abcdefgh)');
    assert.equal(gateLabel({}), 'Unlabelled gate (device unknown)');
});
