import React, { useMemo } from 'react';
import { ShoppingCart, Clock, MapPin, ArrowRight } from 'lucide-react';
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Button } from '@/components/ui/button';
import { Link } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { format } from 'date-fns';
import { Money } from '@/components/ui/money';
import { Separator } from '@/components/ui/separator';
import { TAP_ICON } from '@/pages/visitor/ui-scale';

// No `isMobile` prop any more: the trigger used to shrink to `size="sm"`
// (32px) on exactly the screens where it needed to be BIGGER. It is one
// control at one size now, 44px on a phone and 36px on a pointer.
const CartDropdown = () => {
    const { itemsByEvent, itemCount, totalsByCurrency } = useCart();

    const groups = useMemo(() => Object.entries(itemsByEvent), [itemsByEvent]);

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <Button
                    variant="ghost"
                    size="icon"
                    className={`relative ${TAP_ICON}`}
                    aria-label={itemCount === 0 ? 'Cart, empty' : `Cart, ${itemCount} ticket${itemCount === 1 ? '' : 's'}`}
                >
                    <ShoppingCart className="h-5 w-5" aria-hidden="true" />
                    {itemCount > 0 && (
                        <span
                            aria-hidden="true"
                            className="absolute right-0.5 top-0.5 flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-primary px-1 text-xs font-bold tabular-nums text-primary-foreground ring-2 ring-background sm:-right-1 sm:-top-1"
                        >
                            {itemCount}
                        </span>
                    )}
                </Button>
            </DropdownMenuTrigger>

            {/* Radix measures the trigger and clamps to the viewport, but the
                panel still has to be narrower than the narrowest phone or the
                collision fix is what is holding the layout up. 20rem = 320px
                clears 390px with both gutters intact. */}
            <DropdownMenuContent align="end" sideOffset={8} className="w-[min(20rem,calc(100vw-2rem))] p-4">
                <h2 className="text-base font-bold tracking-tight">Your cart</h2>

                {groups.length === 0 ? (
                    <div className="py-8 text-center">
                        <ShoppingCart className="mx-auto h-8 w-8 text-muted-foreground/50" aria-hidden="true" />
                        <p className="mt-3 text-sm text-muted-foreground">Nothing in here yet.</p>
                        <Button variant="outline" size="sm" className="mt-4" asChild>
                            <Link to="/events">Browse events</Link>
                        </Button>
                    </div>
                ) : (
                    <>
                        <div className="-mx-1 mt-3 max-h-[50vh] space-y-4 overflow-y-auto px-1">
                            {groups.map(([eventId, items]) => {
                                const event = items[0].event;
                                return (
                                    <div key={eventId} className="border-b border-border pb-4 last:border-0 last:pb-0">
                                        <h3 className="text-sm font-semibold leading-snug">{event.title}</h3>
                                        <div className="mt-1 space-y-0.5">
                                            {event.starts_at && (
                                                <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                                                    <Clock className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                                                    {format(new Date(event.starts_at), 'EEE, d MMM · HH:mm')}
                                                </p>
                                            )}
                                            {event.venue_name && (
                                                <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                                                    <MapPin className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                                                    <span className="truncate">{event.venue_name}</span>
                                                </p>
                                            )}
                                        </div>

                                        <ul className="mt-2.5 space-y-1.5">
                                            {items.map((item) => (
                                                <li key={item.ticket_type_id} className="flex items-baseline justify-between gap-3 text-sm">
                                                    <span className="min-w-0 truncate">
                                                        <span className="tabular-nums">{item.quantity}</span>
                                                        {' × '}
                                                        {item.ticket_type.name}
                                                    </span>
                                                    <span className="shrink-0 font-semibold tabular-nums">
                                                        <Money
                                                            minor={item.quantity * item.ticket_type.price_minor}
                                                            currency={event.currency}
                                                        />
                                                    </span>
                                                </li>
                                            ))}
                                        </ul>
                                    </div>
                                );
                            })}
                        </div>

                        <Separator variant="perforated" className="my-4" style={{ '--notch': 'var(--popover)' }} />

                        {/* One line per currency: a cart spanning events in
                            different currencies has no single meaningful grand
                            total to blend them into (Cackle has no privileged
                            currency). The common case — one event, one
                            currency — renders exactly one line. */}
                        <div className="space-y-1">
                            {Object.entries(totalsByCurrency).map(([currency, minor]) => (
                                <div key={currency} className="flex items-baseline justify-between font-semibold">
                                    <span className="text-sm">Total{Object.keys(totalsByCurrency).length > 1 ? ` (${currency})` : ''}</span>
                                    <span className="tabular-nums">
                                        <Money minor={minor} currency={currency} />
                                    </span>
                                </div>
                            ))}
                        </div>

                        <Button className="mt-4 w-full" asChild>
                            <Link to="/cart">
                                View cart
                                <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                            </Link>
                        </Button>
                    </>
                )}
            </DropdownMenuContent>
        </DropdownMenu>
    );
};

export default CartDropdown;
