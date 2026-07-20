import React from 'react';
import { Calendar, MapPin } from 'lucide-react';
import TicketSelection from './ticket-selection';

const QuickInfoItem = ({ icon, title, subtitle }) => (
    <div className="flex items-center gap-4">
        <div className="rounded-full bg-primary/10 p-3 text-primary">{icon}</div>
        <div className="min-w-0">
            <p className="truncate font-semibold text-foreground">{title}</p>
            <p className="truncate text-muted-foreground">{subtitle}</p>
        </div>
    </div>
);

const formatDate = (date) =>
    date
        ? new Date(date).toLocaleDateString(undefined, { weekday: 'short', month: 'long', day: 'numeric', year: 'numeric' })
        : 'Date TBA';

const formatTimeRange = (start, end) => {
    const opts = { hour: 'numeric', minute: '2-digit' };
    const startStr = start ? new Date(start).toLocaleTimeString(undefined, opts) : '';
    const endStr = end ? new Date(end).toLocaleTimeString(undefined, opts) : '';
    return [startStr, endStr].filter(Boolean).join(' – ') || 'Time TBA';
};

const EventQuickInfo = ({ event, ticketTypes }) => {
    return (
        <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
            <QuickInfoItem icon={<Calendar className="h-6 w-6" />} title={formatDate(event.starts_at)} subtitle={formatTimeRange(event.starts_at, event.ends_at)} />
            <QuickInfoItem icon={<MapPin className="h-6 w-6" />} title={event.venue_name || 'Venue TBA'} subtitle={event.address || 'View on map'} />
            <div className="flex items-center md:justify-end">
                <TicketSelection ticketTypes={ticketTypes} event={event} />
            </div>
        </div>
    );
};

export default EventQuickInfo;
