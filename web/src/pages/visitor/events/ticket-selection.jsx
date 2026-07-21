import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { Minus, Plus, Ticket } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/use-toast';
import { EmptyState } from '@/components/ui/empty-state';
import { cn } from '@/lib/utils';
import { visibleTicketTypes, remainingFor, priceFromMinor, formatMoney } from './ticket-utils';

/**
 * The event page's primary conversion surface: an inline (not modal) ticket
 * selector with per-type quantity steppers, meant to sit in a sticky sidebar
 * next to the event description on desktop and flow inline on mobile. See
 * `MobileStickyCta` below for the small-screen "jump to tickets" affordance.
 */
const TicketSelection = ({ event, ticketTypes = [], className }) => {
    const { addItem } = useCart();
    const navigate = useNavigate();
    const [quantities, setQuantities] = useState({});

    const available = visibleTicketTypes(ticketTypes);

    const updateQuantity = (id, delta, max) => {
        setQuantities((prev) => {
            const next = Math.max(0, Math.min(max, (prev[id] || 0) + delta));
            return { ...prev, [id]: next };
        });
    };

    const total = available.reduce((sum, t) => sum + (quantities[t.id] || 0) * t.price_minor, 0);
    const totalQuantity = Object.values(quantities).reduce((sum, q) => sum + q, 0);

    const handleAddToCart = () => {
        available.forEach((t) => {
            const qty = quantities[t.id] || 0;
            if (qty > 0) addItem(t, event, qty);
        });
        setQuantities({});
        toast({ title: 'Added to cart', description: `${totalQuantity} ticket${totalQuantity === 1 ? '' : 's'} added.` });
        navigate('/cart');
    };

    return (
        <Card id="ticket-panel" className={cn('overflow-hidden', className)}>
            <CardHeader className="pb-5">
                <CardTitle className="flex items-center gap-2 font-display text-lg">
                    <Ticket className="h-4 w-4 text-primary" aria-hidden="true" />
                    Tickets
                </CardTitle>
            </CardHeader>
            {/* Perforated tear-line under the header — this panel IS the ticket
                stub the visitor is about to buy. --notch matches the card
                surface so the punched circles read as cut through it. */}
            <div className="ticket-perforation mx-6" style={{ '--notch': 'var(--card)' }} aria-hidden="true" />
            <CardContent className="space-y-4 pt-6">
                {available.length === 0 ? (
                    <EmptyState
                        icon={Ticket}
                        title="No tickets available"
                        description="Ticket sales haven't opened for this event yet — check back soon."
                        className="border-none bg-transparent p-0 py-6"
                    />
                ) : (
                    <>
                        <div className="max-h-[420px] space-y-3 overflow-y-auto pr-1">
                            {available.map((t) => {
                                const left = remainingFor(t);
                                const qty = quantities[t.id] || 0;
                                const soldOut = left <= 0;
                                return (
                                    <div
                                        key={t.id}
                                        className={cn(
                                            'rounded-lg border border-border bg-muted/40 p-4 transition-opacity',
                                            soldOut && 'opacity-60',
                                        )}
                                    >
                                        <div className="flex items-start justify-between gap-3">
                                            <div className="min-w-0 flex-1">
                                                <h3 className="font-medium leading-tight">{t.name}</h3>
                                                <p className="mt-0.5 text-sm text-muted-foreground">{formatMoney(t.price_minor, event?.currency)}</p>
                                                {t.description && <p className="mt-1 text-xs text-muted-foreground">{t.description}</p>}
                                                {soldOut ? (
                                                    <p className="mt-1.5 text-xs font-semibold text-destructive">Sold out</p>
                                                ) : (
                                                    left <= 10 && <p className="mt-1.5 text-xs font-semibold text-warning">Only {left} left</p>
                                                )}
                                            </div>
                                            <div className="flex shrink-0 items-center gap-2">
                                                <Button
                                                    type="button"
                                                    size="icon"
                                                    variant="outline"
                                                    className="h-9 w-9"
                                                    onClick={() => updateQuantity(t.id, -1, left)}
                                                    disabled={qty === 0}
                                                    aria-label={`Fewer ${t.name} tickets`}
                                                >
                                                    <Minus className="h-4 w-4" />
                                                </Button>
                                                <span className="w-6 text-center tabular-nums" aria-live="polite" aria-atomic="true">
                                                    {qty}
                                                </span>
                                                <Button
                                                    type="button"
                                                    size="icon"
                                                    variant="outline"
                                                    className="h-9 w-9"
                                                    onClick={() => updateQuantity(t.id, 1, left)}
                                                    disabled={soldOut || qty >= Math.min(left, 10)}
                                                    aria-label={`More ${t.name} tickets`}
                                                >
                                                    <Plus className="h-4 w-4" />
                                                </Button>
                                            </div>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>

                        <div className="flex items-center justify-between border-t border-border pt-4 font-medium">
                            <span>Total</span>
                            <span data-testid="ticket-total">{formatMoney(total, event?.currency)}</span>
                        </div>

                        <Button onClick={handleAddToCart} disabled={totalQuantity === 0} className="w-full" size="lg">
                            <Ticket className="mr-2 h-4 w-4" />
                            {totalQuantity > 0 ? `Add ${totalQuantity} to cart` : 'Select tickets'}
                        </Button>
                    </>
                )}
            </CardContent>
        </Card>
    );
};

/**
 * Small-screen-only sticky CTA bar fixed to the bottom of the viewport. It
 * doesn't duplicate the selector — tapping it just scrolls the real ticket
 * panel into view, so there's exactly one place quantities are chosen.
 */
export const MobileStickyCta = ({ event, ticketTypes = [] }) => {
    const available = visibleTicketTypes(ticketTypes);
    const price = priceFromMinor(ticketTypes);
    const soldOut = available.length > 0 && available.every((t) => remainingFor(t) <= 0);

    const scrollToTickets = () => {
        document.getElementById('ticket-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    };

    return (
        <div
            className="fixed inset-x-0 bottom-0 z-40 border-t border-border bg-background/95 p-4 pb-[calc(env(safe-area-inset-bottom)+1rem)] shadow-[0_-4px_16px_rgba(0,0,0,0.08)] backdrop-blur-md lg:hidden"
            data-testid="mobile-ticket-cta"
        >
            <div className="mx-auto flex max-w-6xl items-center justify-between gap-4">
                <div className="min-w-0">
                    <p className="text-xs text-muted-foreground">{soldOut ? 'Sold out' : 'From'}</p>
                    {!soldOut && (
                        <p className="truncate font-display text-lg font-bold">
                            {price === null ? 'TBA' : price === 0 ? 'Free' : formatMoney(price, event?.currency)}
                        </p>
                    )}
                </div>
                <Button size="lg" className="shrink-0" onClick={scrollToTickets} disabled={available.length === 0}>
                    <Ticket className="mr-2 h-4 w-4" />
                    {available.length === 0 ? 'Unavailable' : 'Get tickets'}
                </Button>
            </div>
        </div>
    );
};

export default TicketSelection;
