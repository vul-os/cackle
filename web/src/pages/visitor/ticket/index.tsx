import React, { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { format } from 'date-fns';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import { Button } from '@/components/ui/button';
import { ErrorState } from '@/components/ui/error-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Calendar, MapPin, User, Armchair, Printer, Download, Ban, ChevronLeft, Sun, WifiOff, type LucideIcon } from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import { tickets as ticketsApi } from '@/lib/api';
import PrintStyles from '@/pages/visitor/tickets/printing/print-styles';
import { Separator } from '@/components/ui/separator';
import { humanError } from '@/pages/visitor/errors';
import type { Ticket as TicketRecord } from '@/lib/api-types';

// ── This page is read at a door ───────────────────────────────────────────
//
// Not at a desk. Someone is holding a phone up one-handed, in a queue, in
// bad light, possibly with no signal, while a steward two feet away needs
// to read the ticket type off it. Everything here is arranged for that:
//
//   - The QR comes FIRST on a phone. It is the thing being held up, so it
//     must not be below the fold, and it scales with the viewport instead
//     of sitting at a fixed 220px on every screen.
//   - The QR plate is white with black modules in BOTH themes. A scanner
//     reads reflectance, not brand; inverting it in dark mode would be a
//     tasteful way to stop the ticket working.
//   - Error correction stays at level H, which is what survives a
//     fingerprint, a cracked screen and a glare band across the code.
//   - The details a steward calls out — type, name, seat, serial — are set
//     large enough to read at arm's length rather than as form captions.

const STATUS_LABEL: Record<string, string> = {
    void: 'This ticket has been voided and cannot be used for entry.',
    refunded: 'This ticket has been refunded and cannot be used for entry.',
};

function formatDate(dateString: string | null | undefined): string {
    if (!dateString) return 'Date TBA';
    try {
        return format(new Date(dateString), 'EEEE, d MMMM yyyy');
    } catch {
        return 'Date TBA';
    }
}

function formatTime(dateString: string | null | undefined): string {
    if (!dateString) return '';
    try {
        return format(new Date(dateString), 'HH:mm');
    } catch {
        return '';
    }
}

const Shell = ({ children }: { children: React.ReactNode }) => (
    <div className="flex min-h-screen flex-col bg-background">
        <Header />
        <PrintStyles />
        <main id="main" className="flex-1 px-4 pb-16 pt-24">
            <div className="mx-auto max-w-3xl">{children}</div>
        </main>
        <div className="print:hidden">
            <Footer />
        </div>
    </div>
);

function TicketSkeleton() {
    return (
        <div className="space-y-6" role="status" aria-label="Loading ticket">
            <Skeleton className="h-10 w-40" />
            <div className="overflow-hidden rounded-3xl border border-border">
                <Skeleton className="h-32 w-full rounded-none" />
                <div className="flex flex-col items-center gap-8 bg-card p-6 sm:flex-row sm:p-8">
                    <Skeleton className="h-[280px] w-[280px] max-w-full rounded-2xl" />
                    <div className="w-full flex-1 space-y-4">
                        <Skeleton className="h-6 w-2/3" />
                        <Skeleton className="h-6 w-1/2" />
                        <Skeleton className="h-6 w-1/3" />
                    </div>
                </div>
            </div>
        </div>
    );
}

/**
 * `seat` rides on the decoded capability token (internal/tickets/capability.go),
 * not on the Ticket row itself — see printing/layout.tsx's TicketLayoutTicket
 * for the same distinction.
 */
type PageTicket = TicketRecord & { seat?: string };

interface TicketPageState {
    ticket: PageTicket | null;
    loading: boolean;
    error: string | null;
}

export default function TicketPage() {
    const { id } = useParams();
    const [state, setState] = useState<TicketPageState>({ ticket: null, loading: true, error: null });
    const [reloadToken, setReloadToken] = useState(0);

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        ticketsApi
            .get(id ?? '')
            .then((data) => {
                if (cancelled) return;
                setState({ ticket: data.ticket, loading: false, error: null });
            })
            .catch((err: unknown) => {
                if (cancelled) return;
                setState({ ticket: null, loading: false, error: err instanceof Error ? err.message : 'Ticket not found' });
            });
        return () => {
            cancelled = true;
        };
    }, [id, reloadToken]);

    const { ticket, loading, error } = state;

    return (
        <Shell>
            {loading && <TicketSkeleton />}

            {!loading && (error || !ticket) && (
                <ErrorState
                    title="We can't show this ticket"
                    description={humanError(
                        error,
                        "This ticket doesn't exist, or it belongs to a different account. Check the link, or find it under My tickets.",
                    )}
                    onRetry={() => setReloadToken((n) => n + 1)}
                />
            )}

            {!loading && !error && ticket && <TicketCard ticket={ticket} />}
        </Shell>
    );
}

/** A label/value pair sized to be read from a couple of feet away. */
const GateFact = ({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: React.ReactNode }) => (
    <div className="flex items-start gap-3">
        <Icon className="mt-1 h-5 w-5 shrink-0 text-primary-emphasis" aria-hidden="true" />
        <div className="min-w-0">
            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
            <p className="text-lg font-bold leading-snug text-foreground sm:text-xl">{value}</p>
        </div>
    </div>
);

function TicketCard({ ticket }: { ticket: PageTicket }) {
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
                <Button variant="ghost" className="-ml-2" asChild>
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
                        className="print-keep-color relative z-20 flex items-center justify-center gap-2 bg-destructive px-6 py-3 text-center text-sm font-extrabold uppercase tracking-wide text-destructive-foreground sm:text-base"
                    >
                        <Ban className="h-5 w-5 shrink-0" aria-hidden="true" />
                        {(status && STATUS_LABEL[status]) ?? `This ticket is ${status} and cannot be used for entry.`}
                    </div>
                )}

                {/* The header band is INK with the page ground reversed out of
                    it in both themes: it must read the same at noon and at
                    dusk, exactly like the gate screen it will be held up to. */}
                <div className="print-keep-color bg-foreground px-6 py-7 text-background sm:px-10 sm:py-9">
                    <p className="text-sm font-black uppercase tracking-widest text-background/70">{ticketTypeName}</p>
                    <h1 className="mt-1.5 font-display text-display-sm font-extrabold leading-tight tracking-tight text-background sm:text-display-md">
                        {eventTitle}
                    </h1>
                    <div className="mt-5 flex flex-col gap-2 text-base font-semibold text-background/90 sm:flex-row sm:flex-wrap sm:gap-x-8">
                        <span className="flex items-center gap-2">
                            <Calendar className="h-5 w-5 shrink-0" aria-hidden="true" />
                            {formatDate(ticket.event_starts_at)}
                            {ticket.event_starts_at && ` · ${formatTime(ticket.event_starts_at)}`}
                        </span>
                        <span className="flex items-center gap-2">
                            <MapPin className="h-5 w-5 shrink-0" aria-hidden="true" />
                            {venue}
                        </span>
                    </div>
                </div>

                {/* QR first in the DOM, so it is first on a phone and second
                    on a wide screen (sm:order-2) — the thing you hold up is
                    never below the fold. */}
                <div className="flex flex-col gap-8 bg-card px-6 py-8 sm:flex-row sm:items-center sm:gap-10 sm:px-10">
                    <div className="flex flex-col items-center gap-4 sm:order-2 sm:flex-1">
                        {/* INTENTIONAL raw white, in BOTH themes — not a theme-sweep bug.
                            A scanner reads printed reflectance and the quiet zone around
                            the modules, not a design token; a dark-mode-inverted plate
                            would look "correct" and fail to scan. `text-brand-2` below is
                            the matching fixed-INK text (17.9:1 on white, measured in
                            contrast.test.js) for the one state where this plate has no
                            code to show — it must stay legible on the literal white
                            behind it, not flip to a near-white muted tone in dark mode. */}
                        <div
                            className={`print-keep-color print-qr w-[min(100%,20rem)] rounded-2xl bg-white p-4 shadow-soft ring-1 ring-border sm:p-5 ${
                                isVoid ? 'opacity-60 grayscale' : ''
                            }`}
                            aria-label={
                                isVoid
                                    ? `Ticket QR code, ${status}, not valid for entry`
                                    : 'Ticket QR code — hold this up at the door'
                            }
                        >
                            {ticket.capability ? (
                                // Rendered at 320 and then sized by CSS, so it
                                // is as large as the screen allows instead of
                                // a fixed 220px box on a 1440px monitor and a
                                // 390px phone alike.
                                <QRCodeSVG value={ticket.capability} size={320} level="H" className="h-auto w-full" />
                            ) : (
                                <div className="flex aspect-square w-full items-center justify-center text-center text-sm font-medium text-brand-2/70">
                                    No capability issued for this ticket.
                                </div>
                            )}
                        </div>
                        {!isVoid && (
                            <p className="flex items-center gap-2 text-center text-xs font-medium text-muted-foreground print:hidden">
                                <Sun className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                                Turn your screen brightness up before you reach the door.
                            </p>
                        )}
                    </div>

                    <div className="space-y-5 sm:order-1 sm:flex-1">
                        <GateFact icon={User} label="Ticket holder" value={ticket.holder_name || 'Ticket holder'} />
                        {ticket.seat && <GateFact icon={Armchair} label="Seat" value={ticket.seat} />}

                        {/* The tear-line between "who this is for" and the ticket's own
                            serial/legal footer — the one division on this card that is
                            genuinely a ticket stub, not a generic rule. */}
                        <Separator variant="perforated" style={{ '--notch': 'var(--card)' } as React.CSSProperties} />
                        <div className="pt-3">
                            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Ticket number</p>
                            <p className="mt-0.5 select-all font-mono text-base font-semibold text-foreground">#{ticket.serial}</p>
                            {!isVoid && (
                                <p className="mt-3 flex items-start gap-2 text-sm leading-relaxed text-muted-foreground">
                                    <WifiOff className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                                    <span>
                                        Hold this up at the door. It is checked on the spot — neither you nor the venue needs
                                        a signal for it to work.
                                    </span>
                                </p>
                            )}
                        </div>
                    </div>
                </div>
            </div>
        </>
    );
}
