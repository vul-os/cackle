// Small pure helpers shared between the quick-facts bar and the ticket
// selection panel, so "how much is left" and "what does this cost" are
// computed identically in both places instead of drifting apart.

export { formatMoney } from '@/lib/money';

/** Ticket types that should actually be offered to a visitor right now. */
export function visibleTicketTypes(ticketTypes = []) {
    return ticketTypes.filter((t) => t.status !== 'hidden');
}

/** Remaining stock for one ticket type — never negative. */
export function remainingFor(ticketType) {
    return Math.max(0, (ticketType.quantity_total ?? 0) - (ticketType.quantity_sold ?? 0));
}

/**
 * Aggregate availability across all visible ticket types:
 *   - no types published yet            -> { state: 'unpublished' }
 *   - every type sold out                -> { state: 'sold_out' }
 *   - <= 20 tickets left across the event -> { state: 'low', remaining }
 *   - otherwise                          -> { state: 'available', remaining }
 */
export function availabilitySummary(ticketTypes = []) {
    const visible = visibleTicketTypes(ticketTypes);
    if (visible.length === 0) return { state: 'unpublished', remaining: 0 };

    const remaining = visible.reduce((sum, t) => sum + remainingFor(t), 0);
    if (remaining <= 0) return { state: 'sold_out', remaining: 0 };
    if (remaining <= 20) return { state: 'low', remaining };
    return { state: 'available', remaining };
}

/** Lowest ticket price currently on offer, in minor units, or null if nothing is published. */
export function priceFromMinor(ticketTypes = []) {
    const visible = visibleTicketTypes(ticketTypes);
    if (visible.length === 0) return null;
    return visible.reduce((min, t) => (min === null || t.price_minor < min ? t.price_minor : min), null);
}
