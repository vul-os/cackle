import React, { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { QRCodeSVG } from 'qrcode.react';
import { format } from 'date-fns';
import Header from '@/pages/visitor/header';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Calendar, MapPin, User, AlertCircle, Printer, Download, Ban } from 'lucide-react';
import { tickets as ticketsApi } from '@/lib/api';

function formatDate(dateString) {
    if (!dateString) return 'TBA';
    try {
        return format(new Date(dateString), 'EEEE, MMMM d, yyyy');
    } catch {
        return 'TBA';
    }
}

function formatTime(dateString) {
    if (!dateString) return '';
    try {
        return format(new Date(dateString), 'h:mm a');
    } catch {
        return '';
    }
}

export default function TicketPage() {
    const { id } = useParams();
    const [state, setState] = useState({ ticket: null, loading: true, error: null });

    useEffect(() => {
        let cancelled = false;
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
    }, [id]);

    const { ticket, loading, error } = state;

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

    if (error || !ticket) {
        return (
            <>
                <Header />
                <Card className="mx-auto mt-24 max-w-2xl">
                    <CardContent className="pt-6">
                        <div className="flex flex-col items-center space-y-4 text-center">
                            <AlertCircle className="h-12 w-12 text-destructive" />
                            <h2 className="text-2xl font-semibold">Couldn&apos;t load this ticket</h2>
                            <p className="text-muted-foreground">{error || 'Ticket not found'}</p>
                        </div>
                    </CardContent>
                </Card>
            </>
        );
    }

    // GET /api/tickets/{id} returns a flat, decorated ticket — event_title /
    // event_venue_name / event_starts_at / ticket_type_name alongside the
    // ticket's own fields, not nested `event`/`ticket_type` objects.
    const event = {
        title: ticket.event_title || 'Untitled event',
        venue_name: ticket.event_venue_name,
        starts_at: ticket.event_starts_at,
    };
    const ticketTypeName = ticket.ticket_type_name || 'General admission';
    const isVoid = ticket.status && ticket.status !== 'valid';

    return (
        <>
            <Header />
            <main className="mx-auto max-w-4xl p-4 pt-24">
                <Card className="mb-6 print:hidden">
                    <CardHeader className="flex flex-row items-center justify-between">
                        <CardTitle>Ticket Details</CardTitle>
                        <div className="flex gap-2">
                            <Button variant="outline" onClick={() => window.print()}>
                                <Printer className="mr-2 h-4 w-4" />
                                Print
                            </Button>
                            <Button variant="outline" asChild>
                                <a href={ticketsApi.pdfUrl(ticket.id)} target="_blank" rel="noopener noreferrer">
                                    <Download className="mr-2 h-4 w-4" />
                                    PDF
                                </a>
                            </Button>
                        </div>
                    </CardHeader>
                </Card>

                <Card id="printable-ticket" className={isVoid ? 'opacity-60' : ''}>
                    <CardContent className="flex flex-col-reverse gap-8 p-6 sm:flex-row">
                        <div className="flex-[3] border-t border-dashed border-border pt-6 sm:border-t-0 sm:border-r sm:pr-8 sm:pt-0">
                            <div className="mb-6 flex items-start justify-between gap-4">
                                <div className="min-w-0">
                                    <h2 className="mb-1 font-display text-2xl font-bold leading-tight">{event.title}</h2>
                                    <p className="text-base font-medium text-primary">{ticketTypeName}</p>
                                </div>
                                <div className="shrink-0 font-mono text-sm text-muted-foreground">#{ticket.serial}</div>
                            </div>

                            <div className="mb-6 space-y-4">
                                <div className="flex items-center gap-3">
                                    <Calendar className="h-5 w-5 shrink-0 text-muted-foreground" />
                                    <div>
                                        <div className="text-sm font-medium">{formatDate(event.starts_at)}</div>
                                        <div className="text-sm text-muted-foreground">{formatTime(event.starts_at)}</div>
                                    </div>
                                </div>
                                <div className="flex items-start gap-3">
                                    <MapPin className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
                                    <div className="text-sm font-medium">{event.venue_name || 'Venue TBA'}</div>
                                </div>
                                <div className="flex items-start gap-3">
                                    <User className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
                                    <div>
                                        <div className="text-sm font-medium">{ticket.holder_name || 'Ticket holder'}</div>
                                        {ticket.seat && <div className="text-sm text-muted-foreground">Seat {ticket.seat}</div>}
                                    </div>
                                </div>
                            </div>

                            {isVoid ? (
                                <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm font-medium text-destructive">
                                    <Ban className="h-4 w-4" />
                                    This ticket is {ticket.status} and cannot be used for entry.
                                </div>
                            ) : (
                                <p className="border-t border-dashed border-border pt-4 text-sm text-muted-foreground">
                                    Present this QR code at the gate. It verifies offline — no signal required.
                                </p>
                            )}
                        </div>

                        <div className="flex flex-1 flex-col items-center justify-center gap-4">
                            <div
                                className={`rounded-2xl bg-white p-4 shadow-md ring-1 ring-black/5 ${isVoid ? 'grayscale' : ''}`}
                                aria-label={isVoid ? 'Ticket QR code (void, not valid for entry)' : 'Ticket QR code — present this at the gate'}
                            >
                                {ticket.capability ? (
                                    <QRCodeSVG value={ticket.capability} size={200} level="H" />
                                ) : (
                                    <div className="flex h-[200px] w-[200px] items-center justify-center text-center text-sm text-muted-foreground">
                                        No capability issued for this ticket.
                                    </div>
                                )}
                            </div>
                            <div className="text-center font-mono text-sm text-muted-foreground">{ticket.serial}</div>
                        </div>
                    </CardContent>
                </Card>
            </main>
        </>
    );
}
