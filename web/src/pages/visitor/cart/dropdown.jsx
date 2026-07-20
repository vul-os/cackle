import React, { useMemo } from 'react';
import { ShoppingCart, Clock, MapPin } from 'lucide-react';
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Button } from '@/components/ui/button';
import { Link } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { format } from 'date-fns';

function formatMoney(cents, currency = 'ZAR') {
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format((cents || 0) / 100);
    } catch {
        return `${((cents || 0) / 100).toFixed(2)} ${currency}`;
    }
}

const CartDropdown = ({ isMobile = false }) => {
    const { itemsByEvent, itemCount, total } = useCart();

    const groups = useMemo(() => Object.entries(itemsByEvent), [itemsByEvent]);

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <Button variant="ghost" size={isMobile ? 'sm' : 'default'} className="relative">
                    <ShoppingCart className="h-5 w-5" />
                    {itemCount > 0 && (
                        <span className="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full bg-primary text-xs text-primary-foreground">
                            {itemCount}
                        </span>
                    )}
                </Button>
            </DropdownMenuTrigger>

            <DropdownMenuContent align="end" className="w-96 p-4">
                <h2 className="mb-4 text-lg font-semibold">Shopping Cart</h2>

                {groups.length === 0 ? (
                    <p className="py-8 text-center text-muted-foreground">Your cart is empty</p>
                ) : (
                    <>
                        <div className="max-h-[60vh] space-y-6 overflow-y-auto">
                            {groups.map(([eventId, items]) => {
                                const event = items[0].event;
                                return (
                                    <div key={eventId} className="border-b border-border pb-4 last:border-0">
                                        <h3 className="font-medium">{event.title}</h3>
                                        <div className="mt-1 space-y-1">
                                            {event.starts_at && (
                                                <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                                                    <Clock className="h-3.5 w-3.5" />
                                                    <span>{format(new Date(event.starts_at), 'EEE, MMM d, h:mm a')}</span>
                                                </div>
                                            )}
                                            {event.venue_name && (
                                                <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                                                    <MapPin className="h-3.5 w-3.5" />
                                                    <span>{event.venue_name}</span>
                                                </div>
                                            )}
                                        </div>

                                        <div className="mt-3 space-y-2">
                                            {items.map((item) => (
                                                <div key={item.ticket_type_id} className="flex justify-between text-sm">
                                                    <span>
                                                        {item.quantity}x {item.ticket_type.name}
                                                    </span>
                                                    <span className="font-medium">
                                                        {formatMoney(item.quantity * item.ticket_type.price_cents, event.currency)}
                                                    </span>
                                                </div>
                                            ))}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>

                        <div className="mt-4 flex justify-between border-t border-border pt-4 font-semibold">
                            <span>Total</span>
                            <span>{formatMoney(total)}</span>
                        </div>

                        <Button className="mt-4 w-full" asChild>
                            <Link to="/cart">View Cart</Link>
                        </Button>
                    </>
                )}
            </DropdownMenuContent>
        </DropdownMenu>
    );
};

export default CartDropdown;
