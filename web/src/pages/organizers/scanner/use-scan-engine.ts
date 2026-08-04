import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { verifyWithRing, type KeyRing } from '@/lib/capability';
import { recordScan, getTally, wasAdmitted, getPendingSync, markSynced, type AdmissionRecord } from '@/lib/scan-store';
import { scan as scanApi } from '@/lib/api';
import { useOnline } from '@/lib/use-online';
import { uuid } from '@/lib/utils';
import { decideAdmission } from '@/lib/scan-decision';

const DEVICE_ID_KEY = 'cackle_device_id';

export function getDeviceId(): string {
    let id = localStorage.getItem(DEVICE_ID_KEY);
    if (!id) {
        id = uuid();
        localStorage.setItem(DEVICE_ID_KEY, id);
    }
    return id;
}

/**
 * Core offline scan logic: verify a capability token against the event's
 * pinned key ring, dedupe locally, persist the append-only scan record, and
 * keep a live tally. Nothing here requires the network — sync is a
 * best-effort background step layered on top.
 *
 * `ticketIndex` mirrors the Go side's DecideWithBundle (internal/scan/
 * admission.go): it is the scan bundle's `ticket_index` — the set of
 * ticket IDs currently valid (issued, not void, not refunded) for this
 * event as of when the bundle was downloaded. A ticket whose signature
 * verifies but whose id is ABSENT from an AUTHORITATIVE ticketIndex is
 * treated as invalid — this is what catches a ticket refunded after
 * issuance, which a signature alone can never reveal.
 *
 * `ticketIndexPresent` says whether the index is authoritative, and it is
 * the crux: a server-built bundle always sets it, so an EMPTY authoritative
 * index means "admit nothing" (every ticket revoked, or none issued) — not
 * "no data". Only a legacy bundle without the flag falls back to
 * signature-only checking. Inferring this from length alone would silently
 * re-admit every physically-held ticket for a fully-cancelled event.
 * See docs/OFFLINE-GATES.md.
 *
 * Like the rest of this hook, this check is purely local: ticketIndex is
 * whatever was cached in IndexedDB alongside the rest of the bundle, so a
 * ticket refunded after the bundle was downloaded is still admitted here
 * until the gate re-pulls a fresh bundle — an inherent limitation of
 * offline operation, not a bug.
 *
 * `admittedIndex` is the bundle's `admitted_index`: the tickets the server
 * already had an admission recorded for when this bundle was built. It is the
 * ONLY channel by which this device learns that a ticket was admitted at a
 * DIFFERENT gate, and it mirrors Go's DecideWithBundle. Unlike ticketIndex it
 * needs no "present" flag — empty and absent both mean "this bundle knows of
 * nobody already inside", and both correctly leave the answer to the local log.
 *
 * Dedupe is per DEVICE, and it FAILS CLOSED. A second scan of the same
 * ticket on this scanner is refused here, offline, immediately. Two offline
 * scanners at two entrances cannot see each other, so the same ticket
 * presented at both IS admitted at both — that is not prevented and cannot be,
 * because preventing it needs coordination they do not have. What narrows the
 * window is re-pulling the bundle (admittedIndex above) and syncing; what
 * catches what still slipped through is
 * GET /api/events/:id/admission-conflicts after the fact.
 * docs/OFFLINE-GATES.md spells out exactly what that does and does not stop.
 * If the local store errors instead of answering, the scan is recorded
 * 'invalid' and refused rather than admitted — never guess in the admitting
 * direction.
 */
/** The extra shape a scan can resolve to when the local write itself fails —
 * a superset AdmissionRecord could never represent, since it never made it
 * into the store. */
type ScanFailure = {
    id: string;
    event_id: string;
    ticket_id: null;
    result: 'invalid';
    note: string;
    holder_name: null;
    at: number;
};

export type ScanResult = (AdmissionRecord & { at: number }) | ScanFailure;

interface UseScanEngineArgs {
    eventId: string | null | undefined;
    keyRing: KeyRing;
    ticketIndex: string[] | null | undefined;
    ticketIndexPresent: boolean;
    admittedIndex: string[] | null | undefined;
    gateId?: string | null;
}

export function useScanEngine({ eventId, keyRing, ticketIndex, ticketIndexPresent, admittedIndex, gateId }: UseScanEngineArgs) {
    const online = useOnline();
    // Always build the set — even when empty — so an authoritative empty
    // index admits nothing. Whether it is consulted at all is gated on
    // ticketIndexPresent below, never on the set being non-empty.
    const ticketIndexSet = useMemo(
        () => new Set(Array.isArray(ticketIndex) ? ticketIndex : []),
        [ticketIndex],
    );
    const admittedIndexSet = useMemo(
        () => new Set(Array.isArray(admittedIndex) ? admittedIndex : []),
        [admittedIndex],
    );
    const [tally, setTally] = useState<Record<string, number>>({ admitted: 0, duplicate: 0, invalid: 0, wrong_event: 0, total: 0 });
    const [pendingCount, setPendingCount] = useState(0);
    const [lastResult, setLastResult] = useState<ScanResult | null>(null);
    const [isSyncing, setIsSyncing] = useState(false);
    const deviceId = useRef(getDeviceId());
    const busy = useRef(false);

    const refreshCounts = useCallback(async () => {
        if (!eventId) return;
        const [t, pending] = await Promise.all([getTally(eventId), getPendingSync(eventId)]);
        setTally(t);
        setPendingCount(pending.length);
    }, [eventId]);

    useEffect(() => {
        refreshCounts();
    }, [refreshCounts]);

    const syncNow = useCallback(async () => {
        if (!eventId || isSyncing) return;
        const pending = await getPendingSync(eventId);
        if (pending.length === 0) return;

        setIsSyncing(true);
        try {
            await scanApi.sync(
                pending.map((p) => ({
                    ticket_id: p.ticket_id,
                    event_id: p.event_id,
                    device_id: p.device_id,
                    gate_id: p.gate_id,
                    scanned_at: p.scanned_at,
                    result: p.result,
                    note: p.note,
                })),
            );
            await markSynced(pending.map((p) => p.id));
            await refreshCounts();
        } catch {
            // stays queued — we'll retry next time we're online
        } finally {
            setIsSyncing(false);
        }
    }, [eventId, isSyncing, refreshCounts]);

    useEffect(() => {
        if (online) syncNow();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [online]);

    const handleDecode = useCallback(
        async (token: string) => {
            if (!eventId || busy.current) return;
            busy.current = true;
            try {
                // The verdict itself lives in lib/scan-decision.js — pure,
                // effects injected, and exhaustively tested there rather than
                // only reachable through a camera. Nothing about the ordering
                // or the fail-closed behaviour changed in moving it.
                const { result, note, ticketId, holderName } = await decideAdmission({
                    token,
                    eventId,
                    keyRing,
                    ticketIndexSet,
                    ticketIndexPresent,
                    admittedIndexSet,
                    verify: verifyWithRing,
                    wasAdmitted,
                });

                let record: AdmissionRecord;
                try {
                    record = await recordScan({
                        eventId,
                        ticketId,
                        deviceId: deviceId.current,
                        gateId: gateId || 'default',
                        result,
                        note,
                        holderName,
                    });
                } catch (err) {
                    // We could not even write the scan down. Show the
                    // operator a refusal rather than silently doing nothing,
                    // which would look identical to "the scanner didn't see
                    // the code" and invites a retry that admits.
                    const message = err instanceof Error ? err.message : String(err);
                    setLastResult({
                        // Still needs a unique id: the result banner is keyed
                        // on it to re-trigger its flash animation.
                        id: uuid(),
                        event_id: eventId,
                        ticket_id: null,
                        result: 'invalid',
                        note: `Could not record this scan: ${message}`,
                        holder_name: null,
                        at: Date.now(),
                    });
                    return;
                }

                setLastResult({ ...record, at: Date.now() });
                await refreshCounts();
                if (navigator.onLine) syncNow();
            } finally {
                setTimeout(() => {
                    busy.current = false;
                }, 400);
            }
        },
        [eventId, keyRing, ticketIndexSet, ticketIndexPresent, admittedIndexSet, gateId, refreshCounts, syncNow],
    );

    return { online, tally, pendingCount, lastResult, isSyncing, syncNow, handleDecode, deviceId: deviceId.current };
}

export default useScanEngine;
