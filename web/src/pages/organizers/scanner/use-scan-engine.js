import { useCallback, useEffect, useRef, useState } from 'react';
import { verifyWithRing } from '@/lib/capability';
import { recordScan, getTally, wasAdmitted, getPendingSync, markSynced } from '@/lib/scan-store';
import { scan as scanApi } from '@/lib/api';
import { useOnline } from '@/lib/use-online';
import { uuid } from '@/lib/utils';

const DEVICE_ID_KEY = 'cackle_device_id';

export function getDeviceId() {
    let id = localStorage.getItem(DEVICE_ID_KEY);
    if (!id) {
        id = uuid();
        localStorage.setItem(DEVICE_ID_KEY, id);
    }
    return id;
}

function describeError(err) {
    switch (err?.code) {
        case 'malformed':
            return 'Malformed ticket';
        case 'unsupported_version':
            return 'Unsupported ticket version';
        case 'bad_signature':
            return 'Invalid signature — tampered or wrong event';
        case 'not_yet_valid':
            return 'Ticket not valid yet';
        case 'expired':
            return 'Ticket expired';
        case 'unknown_kid':
            return 'Unknown signing key';
        default:
            return err?.message || 'Invalid ticket';
    }
}

/**
 * Core offline scan logic: verify a capability token against the event's
 * pinned key ring, dedupe locally, persist the append-only scan record, and
 * keep a live tally. Nothing here requires the network — sync is a
 * best-effort background step layered on top.
 */
export function useScanEngine({ eventId, keyRing, gateId }) {
    const online = useOnline();
    const [tally, setTally] = useState({ admitted: 0, duplicate: 0, invalid: 0, wrong_event: 0, total: 0 });
    const [pendingCount, setPendingCount] = useState(0);
    const [lastResult, setLastResult] = useState(null);
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
        async (token) => {
            if (!eventId || busy.current) return;
            busy.current = true;
            try {
                let payload;
                let error;
                try {
                    payload = verifyWithRing(token, keyRing, new Date());
                } catch (err) {
                    error = err;
                }

                let result;
                let note = null;
                let holderName = null;
                let ticketId = null;

                if (error) {
                    result = 'invalid';
                    note = describeError(error);
                } else if (payload.eid !== eventId) {
                    result = 'wrong_event';
                    note = 'Ticket belongs to a different event';
                    ticketId = payload.tid;
                    holderName = payload.nm;
                } else {
                    ticketId = payload.tid;
                    holderName = payload.nm;
                    const already = await wasAdmitted(eventId, payload.tid);
                    result = already ? 'duplicate' : 'admitted';
                }

                const record = await recordScan({
                    eventId,
                    ticketId,
                    deviceId: deviceId.current,
                    gateId: gateId || 'default',
                    result,
                    note,
                    holderName,
                });

                setLastResult({ ...record, at: Date.now() });
                await refreshCounts();
                if (navigator.onLine) syncNow();
            } finally {
                setTimeout(() => {
                    busy.current = false;
                }, 400);
            }
        },
        [eventId, keyRing, gateId, refreshCounts, syncNow],
    );

    return { online, tally, pendingCount, lastResult, isSyncing, syncNow, handleDecode, deviceId: deviceId.current };
}

export default useScanEngine;
