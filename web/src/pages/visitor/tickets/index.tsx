import React, { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import { Ticket } from 'lucide-react';
import { humanError } from '@/pages/visitor/errors';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonList } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import TicketFilters from './ticket-filters';
import PrintableTicket from './printing/layout';
import EventInformation from './event-infomation';
import PrintStyles from './printing/print-styles';
import { usePrintTicket } from './printing/use-print-ticket';
import { PrintTicketButtons, PrintAllButton } from './printing/print-buttons';
import { tickets as ticketsApi } from '@/lib/api';
import type { Ticket as TicketRecord } from '@/lib/api-types';

interface TicketFiltersState {
    event: string;
    ticketType: string;
    status: string;
    time: string;
    search: string;
}

const DEFAULT_FILTERS: TicketFiltersState = { event: 'all', ticketType: 'all', status: 'all', time: 'all', search: '' };

interface TicketsState {
    tickets: TicketRecord[];
    loading: boolean;
    error: string | null;
}

/** The view-model this page derives from GET /api/tickets' flat, decorated rows — see eventOf/typeOf below. */
interface TicketEventViewModel {
    id: string;
    title: string;
    venue_name?: string | undefined;
    starts_at?: string | undefined;
}

interface TicketTypeViewModel {
    id: string;
    name: string;
}

interface GroupedByType {
    type: TicketTypeViewModel;
    tickets: TicketRecord[];
}

interface GroupedByEvent {
    event: TicketEventViewModel;
    ticketTypes: Record<string, GroupedByType>;
}

export default function TicketsListPage() {
    const [state, setState] = useState<TicketsState>({ tickets: [], loading: true, error: null });
    const [filters, setFilters] = useState<TicketFiltersState>(DEFAULT_FILTERS);

    const { isPrinting, printTarget, printSingleTicket, printAllTickets } = usePrintTicket();

    const [reloadToken, setReloadToken] = useState(0);
    const load = () => setReloadToken((n) => n + 1);

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        ticketsApi
            .list()
            .then((data) => {
                if (cancelled) return;
                setState({ tickets: data.tickets ?? [], loading: false, error: null });
            })
            .catch((err: unknown) => {
                if (cancelled) return;
                setState({ tickets: [], loading: false, error: err instanceof Error ? err.message : 'Could not load your tickets.' });
            });
        return () => {
            cancelled = true;
        };
    }, [reloadToken]);

    const { tickets, loading, error } = state;

    // GET /api/tickets returns a flat, best-effort-decorated ticket: no
    // nested `event`/`ticket_type` objects, just event_id/event_title/
    // event_venue_name/event_starts_at and ticket_type_id/ticket_type_name
    // alongside the ticket's own fields. Build the small view-model objects
    // the printable-ticket components expect from those flat fields.
    const eventOf = (t: TicketRecord): TicketEventViewModel => ({
        id: t.event_id,
        title: t.event_title || 'Untitled event',
        venue_name: t.event_venue_name,
        starts_at: t.event_starts_at,
    });
    const typeOf = (t: TicketRecord): TicketTypeViewModel => ({ id: t.ticket_type_id, name: t.ticket_type_name || 'Ticket' });

    const events = useMemo(
        () =>
            tickets.reduce<TicketEventViewModel[]>((acc, t) => {
                if (t.event_id && !acc.find((e) => e.id === t.event_id)) acc.push(eventOf(t));
                return acc;
            }, []),
        [tickets],
    );
    const ticketTypes = useMemo(
        () =>
            tickets.reduce<TicketTypeViewModel[]>((acc, t) => {
                if (t.ticket_type_id && !acc.find((tt) => tt.id === t.ticket_type_id)) acc.push(typeOf(t));
                return acc;
            }, []),
        [tickets],
    );

    const filteredTickets = useMemo(() => {
        const now = Date.now();
        const q = filters.search.trim().toLowerCase();
        return tickets.filter((t) => {
            if (filters.event !== 'all' && t.event_id !== filters.event) return false;
            if (filters.ticketType !== 'all' && t.ticket_type_id !== filters.ticketType) return false;
            const status = t.status || 'valid';
            if (filters.status !== 'all' && status !== filters.status) return false;
            if (filters.time !== 'all' && t.event_starts_at) {
                const startsAt = new Date(t.event_starts_at).getTime();
                if (!Number.isNaN(startsAt)) {
                    if (filters.time === 'upcoming' && startsAt < now) return false;
                    if (filters.time === 'past' && startsAt >= now) return false;
                }
            }
            if (q) {
                const haystack = `${t.event_title || ''} ${t.event_venue_name || ''} ${t.ticket_type_name || ''}`.toLowerCase();
                if (!haystack.includes(q)) return false;
            }
            return true;
        });
    }, [tickets, filters]);

    const groupedTickets = useMemo(
        () =>
            filteredTickets.reduce<Record<string, GroupedByEvent>>((acc, ticket) => {
                const eventId = ticket.event_id ?? 'unknown';
                const typeId = ticket.ticket_type_id ?? 'unknown';
                acc[eventId] ||= { event: eventOf(ticket), ticketTypes: {} };
                acc[eventId].ticketTypes[typeId] ||= { type: typeOf(ticket), tickets: [] };
                acc[eventId].ticketTypes[typeId].tickets.push(ticket);
                return acc;
            }, {}),
        [filteredTickets],
    );

    const totalShown = filteredTickets.length;

    return (
        <div className="flex min-h-screen flex-col bg-background">
            <Header />
            <PrintStyles />
            <main id="main" className="mx-auto w-full max-w-6xl flex-1 p-4 pb-16 pt-24">
                <div className="mb-6 print:hidden">
                    <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
                        <div>
                            <h1 className="font-display text-display-sm font-extrabold tracking-tight sm:text-display-md">
                                My tickets
                            </h1>
                            <p className="mt-1.5 text-sm text-muted-foreground">
                                {loading
                                    ? 'Loading your tickets…'
                                    : tickets.length === 0
                                      ? 'Every ticket from a paid order shows up here.'
                                      : `${totalShown} of ${tickets.length} ticket${tickets.length === 1 ? '' : 's'}. Each one works at the door with no signal.`}
                            </p>
                        </div>
                        {!loading && !error && tickets.length > 0 && (
                            <PrintAllButton onPrintAll={printAllTickets} isPrinting={isPrinting} ticketsCount={filteredTickets.length} />
                        )}
                    </div>

                    {!loading && !error && tickets.length > 0 && (
                        <TicketFilters
                            search={filters.search}
                            setSearch={(search) => setFilters((f) => ({ ...f, search }))}
                            selectedEvent={filters.event}
                            setSelectedEvent={(event) => setFilters((f) => ({ ...f, event }))}
                            selectedTicketType={filters.ticketType}
                            setSelectedTicketType={(ticketType) => setFilters((f) => ({ ...f, ticketType }))}
                            selectedStatus={filters.status}
                            setSelectedStatus={(status) => setFilters((f) => ({ ...f, status }))}
                            selectedTime={filters.time}
                            setSelectedTime={(time) => setFilters((f) => ({ ...f, time }))}
                            events={events}
                            ticketTypes={ticketTypes}
                        />
                    )}
                </div>

                {loading && <SkeletonList rows={3} />}

                {!loading && error && (
                    <ErrorState
                        title="We couldn't load your tickets"
                        description={humanError(error, 'Something went wrong on the way to the server. Check your connection and try again.')}
                        onRetry={load}
                    />
                )}

                {!loading && !error && tickets.length === 0 && (
                    <EmptyState
                        icon={Ticket}
                        title="No tickets yet"
                        description="Once an order is paid, its tickets appear here — one QR code each, ready for the door."
                        action={
                            <Button asChild>
                                <Link to="/events">What&apos;s on</Link>
                            </Button>
                        }
                    />
                )}

                {!loading && !error && tickets.length > 0 && filteredTickets.length === 0 && (
                    <EmptyState
                        icon={Ticket}
                        title="No tickets match your filters"
                        description="Try clearing a filter or your search."
                        action={
                            <Button variant="outline" onClick={() => setFilters(DEFAULT_FILTERS)}>
                                Clear filters
                            </Button>
                        }
                    />
                )}

                {!loading &&
                    !error &&
                    Object.values(groupedTickets).map(({ event, ticketTypes: byType }) => (
                        <section key={event.id} className="mb-12" aria-labelledby={`tickets-event-${event.id}`}>
                            <h2
                                id={`tickets-event-${event.id}`}
                                className="mb-5 font-display text-2xl font-bold tracking-tight sm:text-3xl"
                            >
                                {event.title}
                            </h2>

                            {Object.values(byType).map(({ type, tickets: typeTickets }) => (
                                <div key={type.id} className="mb-8">
                                    <h3 className="mb-4 flex items-baseline gap-2 border-l-4 border-primary pl-4 text-lg font-bold tracking-tight">
                                        {type.name}
                                        <span className="text-sm font-medium tabular-nums text-muted-foreground">
                                            × {typeTickets.length}
                                        </span>
                                    </h3>
                                    <div className="grid grid-cols-1 gap-6">
                                        {typeTickets.map((ticket) => {
                                            const hiddenWhilePrinting =
                                                printTarget && printTarget !== 'all' && printTarget !== ticket.id;
                                            return (
                                                <div key={ticket.id} className={hiddenWhilePrinting ? 'print:hidden' : ''}>
                                                    <PrintableTicket ticket={ticket} event={event} type={type} />
                                                    <PrintTicketButtons
                                                        ticketId={ticket.id}
                                                        onPrint={() => printSingleTicket(ticket.id)}
                                                        isPrinting={isPrinting}
                                                    />
                                                </div>
                                            );
                                        })}
                                    </div>
                                </div>
                            ))}

                            <EventInformation event={event} />
                        </section>
                    ))}
            </main>
            <div className="print:hidden">
                <Footer />
            </div>
        </div>
    );
}
