import React from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { MapPin, Ban, Armchair, User } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { formatDate, formatTime } from '../date-utils';

const STATUS_LABEL = {
    void: 'Void — not valid for entry',
    refunded: 'Refunded — not valid for entry',
};

/**
 * Wallet-style ticket card, used both on screen (tickets list) and on paper
 * (print/PDF). print-ticket / print-keep-color / print-qr are consumed by
 * ../printing/print-styles.jsx's @media print rules.
 */
export default function TicketLayout({ ticket, event, type }) {
    const status = ticket.status && ticket.status !== 'valid' ? ticket.status : null;
    const isVoid = Boolean(status);

    return (
        <div data-ticket-id={ticket.id} className="print-ticket relative">
            <div
                className={`overflow-hidden rounded-2xl border shadow-elevated ${
                    isVoid ? 'border-destructive/40' : 'border-border'
                }`}
            >
                <div className="print-keep-color flex items-start justify-between gap-4 bg-foreground px-6 py-5 text-background">
                    <div className="min-w-0">
                        <p className="text-xs font-bold uppercase tracking-wider text-background/70">
                            {formatDate(event.starts_at)} · {formatTime(event.starts_at)}
                        </p>
                        <h3 className="mt-1 truncate font-display text-xl font-bold leading-tight text-background sm:text-2xl">{event.title}</h3>
                    </div>
                    <Badge className="shrink-0 border-transparent bg-primary text-primary-foreground">{type.name}</Badge>
                </div>

                {isVoid && (
                    <div className="print-keep-color flex items-center gap-2 bg-destructive px-6 py-2.5 text-sm font-bold uppercase tracking-wide text-destructive-foreground">
                        <Ban className="h-4 w-4 shrink-0" aria-hidden="true" />
                        {STATUS_LABEL[status] ?? `${status} — not valid for entry`}
                    </div>
                )}

                <div className="flex flex-col gap-6 bg-card px-6 py-6 sm:flex-row sm:items-center">
                    <div className="flex-[3] space-y-4">
                        <div className="flex items-start gap-3">
                            <MapPin className="mt-0.5 h-5 w-5 shrink-0 text-primary" aria-hidden="true" />
                            <div>
                                <div className="text-sm font-semibold text-foreground">{event.venue_name || 'Venue TBA'}</div>
                                {event.address && <div className="text-sm text-muted-foreground">{event.address}</div>}
                            </div>
                        </div>
                        {ticket.holder_name && (
                            <div className="flex items-center gap-3">
                                <User className="h-5 w-5 shrink-0 text-primary" aria-hidden="true" />
                                <div className="text-sm font-semibold text-foreground">{ticket.holder_name}</div>
                            </div>
                        )}
                        {ticket.seat && (
                            <div className="flex items-center gap-3">
                                <Armchair className="h-5 w-5 shrink-0 text-primary" aria-hidden="true" />
                                <div className="text-sm font-semibold text-foreground">Seat {ticket.seat}</div>
                            </div>
                        )}
                        <div className="border-t border-dashed border-border pt-3 font-mono text-xs text-muted-foreground">
                            #{ticket.serial}
                        </div>
                    </div>

                    <div className="flex flex-1 flex-col items-center justify-center gap-3 border-t border-dashed border-border pt-6 sm:border-l sm:border-t-0 sm:pl-6 sm:pt-0">
                        <div
                            className={`print-keep-color print-qr rounded-2xl bg-white p-3 shadow-soft ring-1 ring-black/5 ${
                                isVoid ? 'opacity-60 grayscale' : ''
                            }`}
                            aria-label={isVoid ? `QR code, ${status}, not valid for entry` : 'Entry QR code'}
                        >
                            {ticket.capability ? (
                                <QRCodeSVG value={ticket.capability} size={140} level="H" />
                            ) : (
                                <div className="flex h-[140px] w-[140px] items-center justify-center text-center text-xs text-gray-500">
                                    No capability issued
                                </div>
                            )}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}
