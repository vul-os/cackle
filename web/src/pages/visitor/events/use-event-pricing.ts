import { useEffect, useState } from 'react';
import { events as eventsApi } from '@/lib/api';
import { visibleTicketTypes, remainingFor } from './ticket-utils';

/** A ref-shaped event this hook can price: a slug or an id, whichever a card has on hand. */
export interface PricedEvent {
    slug?: string | null;
    id?: string | null;
}

/** One event's resolved pricing, or null when it failed to load. */
export interface EventPricing {
    minPriceMinor: number | null;
    soldOut: boolean;
}

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
export function useEventPricing(events: PricedEvent[]): Record<string, EventPricing | null> {
    const [byId, setById] = useState<Record<string, EventPricing | null>>({});

    useEffect(() => {
        const ids = events.map((e) => e.slug || e.id).filter((ref): ref is string => !!ref && !(ref in byId));
        if (ids.length === 0) return;
        let cancelled = false;
        Promise.allSettled(ids.map((ref) => eventsApi.get(ref))).then((results) => {
            if (cancelled) return;
            setById((prev) => {
                const next = { ...prev };
                results.forEach((res, i) => {
                    // `results` was built from `ids.map(...)` above, so index i
                    // is always populated in `ids` too.
                    const ref = ids[i];
                    if (ref === undefined) return;
                    if (res.status !== 'fulfilled') {
                        next[ref] = null;
                        return;
                    }
                    const types = res.value?.ticket_types ?? [];
                    const available = visibleTicketTypes(types);
                    if (available.length === 0) {
                        next[ref] = { minPriceMinor: null, soldOut: false };
                        return;
                    }
                    const soldOut = available.every((t) => remainingFor(t) <= 0);
                    const minPriceMinor = available.reduce(
                        (min: number | null, t) => (min === null || t.price_minor < min ? t.price_minor : min),
                        null,
                    );
                    next[ref] = { minPriceMinor, soldOut };
                });
                return next;
            });
        }).catch((err: unknown) => {
            // Promise.allSettled itself never rejects — every individual
            // fetch's outcome is already captured in `results` above. A
            // rejection here can only mean the .then callback itself threw
            // (a real bug in this file), so this is surfaced rather than
            // silently swallowed; it must not crash the page either, since
            // this hook's whole contract is "a failure degrades to no price
            // shown for that one card, not a broken page".
            if (!cancelled) console.error('useEventPricing: failed to apply fetched pricing', err);
        });
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [events]);

    return byId;
}
