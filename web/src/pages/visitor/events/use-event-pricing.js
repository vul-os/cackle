import { useEffect, useState } from 'react';
import { events as eventsApi } from '@/lib/api';
import { visibleTicketTypes, remainingFor } from './ticket-utils';

/**
 * Best-effort per-event pricing/availability, resolved against the public
 * `GET /api/events/{slug}` endpoint — the public list endpoint carries no
 * pricing (see docs/API.md), so cards that want a "from R120" badge fetch
 * it individually. A failure for one event degrades to "no price shown" for
 * that one card rather than failing the whole page.
 *
 * Returns a map keyed by the same ref passed in (slug, falling back to id):
 * { [ref]: { minPriceMinor, soldOut } | null }
 */
export function useEventPricing(events) {
    const [byId, setById] = useState({});

    useEffect(() => {
        const ids = events.map((e) => e.slug || e.id).filter((ref) => ref && !(ref in byId));
        if (ids.length === 0) return;
        let cancelled = false;
        Promise.allSettled(ids.map((ref) => eventsApi.get(ref))).then((results) => {
            if (cancelled) return;
            setById((prev) => {
                const next = { ...prev };
                results.forEach((res, i) => {
                    if (res.status !== 'fulfilled') {
                        next[ids[i]] = null;
                        return;
                    }
                    const types = res.value?.ticket_types ?? [];
                    const available = visibleTicketTypes(types);
                    if (available.length === 0) {
                        next[ids[i]] = { minPriceMinor: null, soldOut: false };
                        return;
                    }
                    const soldOut = available.every((t) => remainingFor(t) <= 0);
                    const minPriceMinor = available.reduce(
                        (min, t) => (min === null || t.price_minor < min ? t.price_minor : min),
                        null,
                    );
                    next[ids[i]] = { minPriceMinor, soldOut };
                });
                return next;
            });
        });
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [events]);

    return byId;
}
