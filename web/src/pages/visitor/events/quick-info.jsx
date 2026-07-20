import React from 'react';
import { Calendar, Clock, MapPin, Tag, Ticket } from 'lucide-react';
import { availabilitySummary, priceFromMinor, formatMoney } from './ticket-utils';

const QuickFact = ({ icon: Icon, label, value, tone }) => (
    <div className="flex items-center gap-3">
        <div
            className={
                'flex h-11 w-11 shrink-0 items-center justify-center rounded-full ' +
                (tone === 'warning'
                    ? 'bg-warning/15 text-warning'
                    : tone === 'destructive'
                      ? 'bg-destructive/10 text-destructive'
                      : 'bg-primary/10 text-primary')
            }
        >
            <Icon className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="min-w-0">
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
            <p className="truncate font-semibold text-foreground" title={typeof value === 'string' ? value : undefined}>
                {value}
            </p>
        </div>
    </div>
);

const formatDate = (date) =>
    date ? new Date(date).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' }) : 'Date TBA';

const formatTimeRange = (start, end) => {
    const opts = { hour: 'numeric', minute: '2-digit' };
    const startStr = start ? new Date(start).toLocaleTimeString(undefined, opts) : '';
    const endStr = end ? new Date(end).toLocaleTimeString(undefined, opts) : '';
    return [startStr, endStr].filter(Boolean).join(' – ') || 'Time TBA';
};

function availabilityFact(ticketTypes) {
    const summary = availabilitySummary(ticketTypes);
    switch (summary.state) {
        case 'unpublished':
            return { value: 'Tickets TBA', tone: 'default' };
        case 'sold_out':
            return { value: 'Sold out', tone: 'destructive' };
        case 'low':
            return { value: `Only ${summary.remaining} left`, tone: 'warning' };
        default:
            return { value: 'Available', tone: 'default' };
    }
}

/**
 * Quick-facts bar: date, time, venue, price-from, and availability at a
 * glance — pure information, no call-to-action (that lives in the sticky
 * ticket panel). Renders on a single row on desktop and wraps to a 2-column
 * grid on narrow screens so it never crowds out the page below the hero.
 */
const EventQuickInfo = ({ event, ticketTypes = [] }) => {
    const price = priceFromMinor(ticketTypes);
    const availability = availabilityFact(ticketTypes);

    return (
        <div className="grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2 lg:grid-cols-5">
            <QuickFact icon={Calendar} label="Date" value={formatDate(event.starts_at)} />
            <QuickFact icon={Clock} label="Time" value={formatTimeRange(event.starts_at, event.ends_at)} />
            <QuickFact icon={MapPin} label="Venue" value={event.venue_name || 'Venue TBA'} />
            <QuickFact
                icon={Tag}
                label="Price from"
                value={price === null ? 'TBA' : price === 0 ? 'Free' : formatMoney(price, event.currency)}
            />
            <QuickFact icon={Ticket} label="Availability" value={availability.value} tone={availability.tone} />
        </div>
    );
};

export default EventQuickInfo;
