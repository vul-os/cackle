import React, { useEffect, useState } from 'react';
import Header from '@/pages/visitor/header';
import { Card, CardContent } from '@/components/ui/card';
import { AlertCircle, Ticket } from 'lucide-react';
import TicketFilters from './ticket-filters';
import PrintableTicket from './printing/layout';
import EventInformation from './event-infomation';
import { usePrintTicket } from './printing/use-print-ticket';
import { PrintTicketButtons, PrintAllButton } from './printing/print-buttons';
import { tickets as ticketsApi } from '@/lib/api';

export default function TicketsListPage() {
    const [state, setState] = useState({ tickets: [], loading: true, error: null });
    const [selectedEvent, setSelectedEvent] = useState('all');
    const [selectedTicketType, setSelectedTicketType] = useState('all');

    const { isPrinting, printSingleTicket, printAllTickets } = usePrintTicket();

    useEffect(() => {
        let cancelled = false;
        ticketsApi
            .list()
            .then((data) => {
                if (cancelled) return;
                const list = Array.isArray(data) ? data : (data?.tickets ?? []);
                setState({ tickets: list, loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ tickets: [], loading: false, error: err.message || 'Could not load your tickets.' });
            });
        return () => {
            cancelled = true;
        };
    }, []);

    const { tickets, loading, error } = state;

    // GET /api/tickets returns a flat, best-effort-decorated ticket: no
    // nested `event`/`ticket_type` objects, just event_id/event_title/
    // event_venue_name/event_starts_at and ticket_type_id/ticket_type_name
    // alongside the ticket's own fields. Build the small view-model objects
    // the printable-ticket components expect from those flat fields.
    const eventOf = (t) => ({
        id: t.event_id,
        title: t.event_title || 'Untitled event',
        venue_name: t.event_venue_name,
        starts_at: t.event_starts_at,
    });
    const typeOf = (t) => ({ id: t.ticket_type_id, name: t.ticket_type_name || 'Ticket' });

    const events = tickets.reduce((acc, t) => {
        if (t.event_id && !acc.find((e) => e.id === t.event_id)) acc.push(eventOf(t));
        return acc;
    }, []);
    const ticketTypes = tickets.reduce((acc, t) => {
        if (t.ticket_type_id && !acc.find((tt) => tt.id === t.ticket_type_id)) acc.push(typeOf(t));
        return acc;
    }, []);

    const filteredTickets = tickets.filter((t) => {
        const eventMatch = selectedEvent === 'all' || t.event_id === selectedEvent;
        const typeMatch = selectedTicketType === 'all' || t.ticket_type_id === selectedTicketType;
        return eventMatch && typeMatch;
    });

    const groupedTickets = filteredTickets.reduce((acc, ticket) => {
        const eventId = ticket.event_id ?? 'unknown';
        const typeId = ticket.ticket_type_id ?? 'unknown';
        acc[eventId] ||= { event: eventOf(ticket), ticketTypes: {} };
        acc[eventId].ticketTypes[typeId] ||= { type: typeOf(ticket), tickets: [] };
        acc[eventId].ticketTypes[typeId].tickets.push(ticket);
        return acc;
    }, {});

    if (loading) {
        return (
            <>
                <Header />
                <div className="flex min-h-[400px] items-center justify-center pt-16">
                    <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-foreground" />
                </div>
            </>
        );
    }

    if (error) {
        return (
            <>
                <Header />
                <Card className="mx-auto mt-24 max-w-2xl">
                    <CardContent className="pt-6">
                        <div className="flex flex-col items-center space-y-4 text-center">
                            <AlertCircle className="h-12 w-12 text-destructive" />
                            <h2 className="text-2xl font-semibold">Couldn&apos;t load tickets</h2>
                            <p className="text-muted-foreground">{error}</p>
                        </div>
                    </CardContent>
                </Card>
            </>
        );
    }

    if (tickets.length === 0) {
        return (
            <>
                <Header />
                <Card className="mx-auto mt-24 max-w-2xl">
                    <CardContent className="pt-6">
                        <div className="flex flex-col items-center space-y-4 text-center">
                            <Ticket className="h-12 w-12 text-muted-foreground" />
                            <h2 className="text-2xl font-semibold">No tickets yet</h2>
                            <p className="text-muted-foreground">Tickets from paid orders will show up here.</p>
                        </div>
                    </CardContent>
                </Card>
            </>
        );
    }

    return (
        <>
            <Header />
            <main className="mx-auto max-w-6xl p-4 pt-24">
                <div className="mb-6">
                    <div className="mb-4 flex items-center justify-between">
                        <h1 className="font-display text-3xl font-bold">My Tickets</h1>
                        <PrintAllButton onPrintAll={() => printAllTickets(filteredTickets)} isPrinting={isPrinting} ticketsCount={filteredTickets.length} />
                    </div>

                    <TicketFilters
                        selectedEvent={selectedEvent}
                        setSelectedEvent={setSelectedEvent}
                        selectedTicketType={selectedTicketType}
                        setSelectedTicketType={setSelectedTicketType}
                        events={events}
                        ticketTypes={ticketTypes}
                    />
                </div>

                {Object.values(groupedTickets).map(({ event, ticketTypes: byType }) => (
                    <div key={event.id} className="mb-8">
                        <h2 className="mb-4 text-2xl font-semibold">{event.title}</h2>

                        {Object.values(byType).map(({ type, tickets: typeTickets }) => (
                            <div key={type.id} className="mb-6">
                                <h3 className="mb-3 border-l-4 border-primary pl-4 text-xl font-medium">
                                    {type.name} ({typeTickets.length})
                                </h3>
                                <div className="grid grid-cols-1 gap-6">
                                    {typeTickets.map((ticket) => (
                                        <Card key={ticket.id} className="relative">
                                            <CardContent className="p-6">
                                                <PrintableTicket ticket={ticket} event={event} type={type} />
                                                <PrintTicketButtons ticketId={ticket.id} onPrint={() => printSingleTicket(ticket.id)} isPrinting={isPrinting} />
                                            </CardContent>
                                        </Card>
                                    ))}
                                </div>
                            </div>
                        ))}

                        <EventInformation event={event} />
                    </div>
                ))}
            </main>
        </>
    );
}
