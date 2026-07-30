import React from 'react';
import { format } from 'date-fns';
import { Clock, MapPin, Loader2, Lock } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Money } from '@/components/ui/money';

function whenLabel(iso) {
    if (!iso) return null;
    try {
        return format(new Date(iso), 'EEE, d MMM yyyy · HH:mm');
    } catch {
        return null;
    }
}

const OrderSummary = ({ event, items, total, isProcessing, onCheckout }) => {
    const when = whenLabel(event.starts_at);
    const ticketCount = items.reduce((n, i) => n + i.quantity, 0);

    return (
        <Card className="lg:sticky lg:top-24">
            <CardHeader className="pb-4">
                <CardTitle>Order summary</CardTitle>
            </CardHeader>

            <CardContent className="space-y-5">
                <div>
                    <h3 className="font-display text-base font-bold leading-snug tracking-tight">{event.title}</h3>
                    <div className="mt-2 space-y-1 text-sm text-muted-foreground">
                        {when && (
                            <p className="flex items-center gap-2">
                                <Clock className="h-4 w-4 shrink-0" aria-hidden="true" />
                                {when}
                            </p>
                        )}
                        {event.venue_name && (
                            <p className="flex items-center gap-2">
                                <MapPin className="h-4 w-4 shrink-0" aria-hidden="true" />
                                <span className="truncate">{event.venue_name}</span>
                            </p>
                        )}
                    </div>
                </div>

                <ul className="space-y-2 border-t border-border pt-4">
                    {items.map((item) => (
                        <li key={item.ticket_type_id} className="flex items-baseline justify-between gap-3 text-sm">
                            <span className="min-w-0">
                                <span className="tabular-nums">{item.quantity}</span> × {item.ticket_type.name}
                            </span>
                            <span className="shrink-0 tabular-nums">
                                <Money minor={item.quantity * item.ticket_type.price_minor} currency={event.currency} />
                            </span>
                        </li>
                    ))}
                </ul>
            </CardContent>

            <CardFooter className="flex-col items-stretch gap-4">
                {/* The tear line: above it is what you are buying, below it
                    is what it costs and the button that commits you. */}
                <Separator variant="perforated" style={{ '--notch': 'var(--card)' }} />

                <div className="flex items-baseline justify-between">
                    <span className="text-base font-semibold">Total</span>
                    <span className="font-display text-2xl font-bold tabular-nums">
                        <Money minor={total} currency={event.currency} />
                    </span>
                </div>

                <Button className="w-full" size="lg" onClick={onCheckout} disabled={isProcessing}>
                    {isProcessing ? (
                        <>
                            <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                            Placing your order…
                        </>
                    ) : (
                        <span className="flex items-center gap-1.5">
                            Pay <Money minor={total} currency={event.currency} />
                        </span>
                    )}
                </Button>

                {/* Says what happens next, and does not overstate who is
                    holding the money. Cackle never does. */}
                <p className="flex items-start gap-2 text-xs leading-relaxed text-muted-foreground">
                    <Lock className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                    <span>
                        You pay the organiser directly, however they take payment — Cackle never holds your money.{' '}
                        {ticketCount === 1 ? 'Your ticket' : `All ${ticketCount} tickets`} appear under My tickets as soon as
                        the payment clears.
                    </span>
                </p>
            </CardFooter>
        </Card>
    );
};

export default OrderSummary;
