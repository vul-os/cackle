import React, { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { format } from 'date-fns';
import Header from '@/pages/visitor/header';
import { Button } from '@/components/ui/button';
import { ErrorState } from '@/components/ui/error-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Calendar, MapPin, User, Armchair, Printer, Download, Ban, ChevronLeft } from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import { tickets as ticketsApi } from '@/lib/api';
import PrintStyles from '@/pages/visitor/tickets/printing/print-styles';

const STATUS_LABEL = {
    void: 'This ticket has been voided and cannot be used for entry.',
    refunded: 'This ticket has been refunded and cannot be used for entry.',
};

function formatDate(dateString) {
    if (!dateString) return 'Date TBA';
    try {
        return format(new Date(dateString), 'EEEE, d MMMM yyyy');
    } catch {
        return 'Date TBA';
    }
}

function formatTime(dateString) {
    if (!dateString) return '';
    try {
        return format(new Date(dateString), 'HH:mm');
    } catch {
        return '';
    }
}

function TicketSkeleton() {
    return (
        <div className="space-y-6" role="status" aria-label="Loading ticket">
            <Skeleton className="h-10 w-40" />
            <div className="overflow-hidden rounded-3xl border border-border">
                <Skeleton className="h-28 w-full rounded-none" />
                <div className="flex flex-col gap-8 bg-card p-8 sm:flex-row">
                    <div className="flex-[3] space-y-4">
                        <Skeleton className="h-5 w-2/3" />
                        <Skeleton className="h-5 w-1/2" />
                        <Skeleton className="h-5 w-1/3" />
                    </div>
                    <div className="flex flex-1 items-center justify-center">
                        <Skeleton className="h-[220px] w-[220px] rounded-2xl" />
                    </div>
                </div>
            </div>
        </div>
    );
}

export default function TicketPage() {
    const { id } = useParams();
    const [state, setState] = useState({ ticket: null, loading: true, error: null });
    const [reloadToken, setReloadToken] = useState(0);

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        ticketsApi
            .get(id)
            .then((data) => {
                if (cancelled) return;
                setState({ ticket: data?.ticket ?? data, loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ ticket: null, loading: false, error: err.message || 'Ticket not found' });
            });
        return () => {
            cancelled = true;
        };
    }, [id, reloadToken]);

    const { ticket, loading, error } = state;

    return (
        <>
            <Header />
            <PrintStyles />
            <main className="mx-auto max-w-3xl p-4 pb-24 pt-24">
                {loading && <TicketSkeleton />}

                {!loading && (error || !ticket) && (
                    <ErrorState
                        title="Couldn't load this ticket"
                        description={error || 'This ticket does not exist, or is not yours.'}
                        onRetry={() => setReloadToken((n) => n + 1)}
                    />
                )}

                {!loading && !error && ticket && <TicketCard ticket={ticket} />}
            </main>
        </>
    );
}

function TicketCard({ ticket }) {
    // GET /api/tickets/{id} returns a flat, decorated ticket — event_title /
    // event_venue_name / event_starts_at / ticket_type_name alongside the
    // ticket's own fields, not nested `event`/`ticket_type` objects.
    const eventTitle = ticket.event_title || 'Untitled event';
    const venue = ticket.event_venue_name || 'Venue TBA';
    const ticketTypeName = ticket.ticket_type_name || 'General admission';
    const status = ticket.status && ticket.status !== 'valid' ? ticket.status : null;
    const isVoid = Boolean(status);

    return (
        <>
            <div className="mb-6 flex flex-wrap items-center justify-between gap-3 print:hidden">
                <Button variant="ghost" asChild>
                    <Link to="/tickets">
                        <ChevronLeft className="mr-2 h-4 w-4" aria-hidden="true" />
                        Back to tickets
                    </Link>
                </Button>
                <div className="flex gap-2">
                    <Button variant="outline" onClick={() => window.print()}>
                        <Printer className="mr-2 h-4 w-4" aria-hidden="true" />
                        Print
                    </Button>
                    <Button variant="outline" asChild>
                        <a href={ticketsApi.pdfUrl(ticket.id)} target="_blank" rel="noopener noreferrer">
                            <Download className="mr-2 h-4 w-4" aria-hidden="true" />
                            PDF
                        </a>
                    </Button>
                </div>
            </div>

            <div
                className={`print-ticket relative overflow-hidden rounded-3xl border shadow-floating ${
                    isVoid ? 'border-destructive/50' : 'border-border'
                }`}
            >
                {isVoid && (
                    <div
                        aria-hidden="true"
                        className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center overflow-hidden"
                    >
                        <span className="-rotate-[18deg] select-none whitespace-nowrap rounded-lg border-4 border-destructive/80 px-6 py-1 text-4xl font-black uppercase tracking-widest text-destructive/80 sm:px-10 sm:py-2 sm:text-6xl">
                            {status}
                        </span>
                    </div>
                )}

                {isVoid && (
                    <div
                        role="alert"
                        className="print-keep-color flex items-center justify-center gap-2 bg-destructive px-6 py-3 text-center text-sm font-extrabold uppercase tracking-wide text-destructive-foreground sm:text-base"
                    >
                        <Ban className="h-5 w-5 shrink-0" aria-hidden="true" />
                        {STATUS_LABEL[status] ?? `This ticket is ${status} and cannot be used for entry.`}
                    </div>
                )}

                <div className="print-keep-color bg-foreground px-6 py-7 text-background sm:px-10 sm:py-9">
                    <p className="text-xs font-bold uppercase tracking-widest text-background/70">{ticketTypeName}</p>
                    <h1 className="mt-1 font-display text-3xl font-extrabold leading-tight tracking-tight text-background sm:text-4xl">
                        {eventTitle}
                    </h1>
                    <div className="mt-5 flex flex-col gap-2 text-sm font-semibold text-background/90 sm:flex-row sm:flex-wrap sm:gap-x-8">
                        <span className="flex items-center gap-2">
                            <Calendar className="h-4 w-4 shrink-0" aria-hidden="true" />
                            {formatDate(ticket.event_starts_at)}
                            {ticket.event_starts_at && ` · ${formatTime(ticket.event_starts_at)}`}
                        </span>
                        <span className="flex items-center gap-2">
                            <MapPin className="h-4 w-4 shrink-0" aria-hidden="true" />
                            {venue}
                        </span>
                    </div>
                </div>

                <div className="flex flex-col-reverse bg-card px-6 py-8 sm:flex-row sm:items-center sm:gap-10 sm:px-10">
                    <div className="mt-8 flex-[3] space-y-5 sm:mt-0">
                        <div className="flex items-start gap-3">
                            <User className="mt-0.5 h-6 w-6 shrink-0 text-primary" aria-hidden="true" />
                            <div>
                                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Ticket holder</p>
                                <p className="text-xl font-bold text-foreground sm:text-2xl">{ticket.holder_name || 'Ticket holder'}</p>
                            </div>
                        </div>

                        {ticket.seat && (
                            <div className="flex items-center gap-3">
                                <Armchair className="h-6 w-6 shrink-0 text-primary" aria-hidden="true" />
                                <div>
                                    <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Seat</p>
                                    <p className="text-lg font-bold text-foreground">{ticket.seat}</p>
                                </div>
                            </div>
                        )}

                        <div className="border-t border-dashed border-border pt-4">
                            <p className="font-mono text-sm font-medium text-foreground">#{ticket.serial}</p>
                            {!isVoid && (
                                <p className="mt-2 text-sm text-muted-foreground">
                                    Present this QR code at the gate. It verifies offline — no signal required.
                                </p>
                            )}
                        </div>
                    </div>

                    <div className="flex flex-1 flex-col items-center justify-center gap-3 border-b border-dashed border-border pb-8 sm:border-b-0 sm:border-l sm:pb-0 sm:pl-10">
                        <div
                            className={`print-keep-color print-qr rounded-2xl bg-white p-5 shadow-soft ring-1 ring-black/5 ${
                                isVoid ? 'opacity-60 grayscale' : ''
                            }`}
                            aria-label={isVoid ? `Ticket QR code, ${status}, not valid for entry` : 'Ticket QR code — present this at the gate'}
                        >
                            {ticket.capability ? (
                                <QRCodeSVG value={ticket.capability} size={220} level="H" />
                            ) : (
                                <div className="flex h-[220px] w-[220px] items-center justify-center text-center text-sm text-gray-500">
                                    No capability issued for this ticket.
                                </div>
                            )}
                        </div>
                    </div>
                </div>
            </div>
        </>
    );
}
