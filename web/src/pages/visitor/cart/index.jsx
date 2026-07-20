import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Minus, Plus, Trash2, ArrowLeft, Clock, MapPin, ShoppingCart } from 'lucide-react';
import Header from '@/pages/visitor/header';
import { format } from 'date-fns';
import { REDIRECT_STORAGE_KEY } from '@/pages/auth/auth-redirect';

function formatMoney(cents, currency = 'ZAR') {
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format((cents || 0) / 100);
    } catch {
        return `${((cents || 0) / 100).toFixed(2)} ${currency}`;
    }
}

const CartPage = () => {
    const { itemsByEvent, itemCount, updateQuantity, removeItem, eventTotal } = useCart();
    const { user, loading } = useAuth();
    const navigate = useNavigate();

    const events = Object.values(itemsByEvent).map((items) => items[0].event);

    const handleCheckout = (eventId) => {
        if (!user) {
            // ProtectedRoute (and the auth pages' post-login redirect) both
            // key off this localStorage entry, not router state — set it
            // here too so signing in from this prompt returns the buyer to
            // checkout instead of dropping them on the homepage.
            try {
                localStorage.setItem(REDIRECT_STORAGE_KEY, `/checkout/${eventId}`);
            } catch {
                // localStorage unavailable — worst case the buyer lands on
                // the homepage after signing in and re-opens their cart.
            }
            navigate('/login');
            return;
        }
        navigate(`/checkout/${eventId}`);
    };

    if (itemCount === 0) {
        return (
            <>
                <Header />
                <div className="min-h-screen bg-background pt-24">
                    <div className="container mx-auto px-4 py-16">
                        <div className="mx-auto max-w-md text-center">
                            <ShoppingCart className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                            <h1 className="mb-2 text-2xl font-bold">Your cart is empty</h1>
                            <p className="mb-8 text-muted-foreground">Looks like you haven&apos;t added any tickets yet.</p>
                            <Button onClick={() => navigate('/')}>
                                <ArrowLeft className="mr-2 h-4 w-4" />
                                Browse Events
                            </Button>
                        </div>
                    </div>
                </div>
            </>
        );
    }

    return (
        <>
            <Header />
            <div className="min-h-screen bg-background pt-24">
                <div className="container mx-auto px-4 py-8">
                    <div className="mx-auto max-w-4xl">
                        <h1 className="mb-8 text-3xl font-bold">Shopping Cart</h1>

                        <div className="space-y-6">
                            {Object.entries(itemsByEvent).map(([eventId, eventItems]) => {
                                const event = eventItems[0].event;
                                const subtotal = eventTotal(eventId);

                                return (
                                    <div key={eventId} className="rounded-lg border border-border bg-card shadow-sm">
                                        <div className="border-b border-border p-4">
                                            <h2 className="mb-2 text-xl font-semibold">{event.title}</h2>
                                            <div className="space-y-1 text-sm text-muted-foreground">
                                                {event.starts_at && (
                                                    <div className="flex items-center gap-2">
                                                        <Clock className="h-4 w-4" />
                                                        <span>{format(new Date(event.starts_at), 'EEE, MMM d, yyyy h:mm a')}</span>
                                                    </div>
                                                )}
                                                {event.venue_name && (
                                                    <div className="flex items-center gap-2">
                                                        <MapPin className="h-4 w-4" />
                                                        <span>{event.venue_name}</span>
                                                    </div>
                                                )}
                                            </div>
                                        </div>

                                        {eventItems.map((item) => (
                                            <div key={item.ticket_type_id} className="flex items-center gap-4 border-b border-border p-4 last:border-0">
                                                <div className="flex-1">
                                                    <h3 className="font-medium">{item.ticket_type.name}</h3>
                                                    <p className="text-sm text-muted-foreground">
                                                        {formatMoney(item.ticket_type.price_cents, event.currency)} each
                                                    </p>
                                                </div>
                                                <div className="flex items-center gap-3">
                                                    <div className="flex items-center gap-2">
                                                        <Button
                                                            variant="outline"
                                                            size="icon"
                                                            className="h-8 w-8"
                                                            onClick={() => updateQuantity(item.ticket_type_id, item.quantity - 1)}
                                                        >
                                                            <Minus className="h-4 w-4" />
                                                        </Button>
                                                        <span className="w-8 text-center">{item.quantity}</span>
                                                        <Button
                                                            variant="outline"
                                                            size="icon"
                                                            className="h-8 w-8"
                                                            onClick={() => updateQuantity(item.ticket_type_id, item.quantity + 1)}
                                                        >
                                                            <Plus className="h-4 w-4" />
                                                        </Button>
                                                    </div>
                                                    <div className="w-24 text-right font-medium">
                                                        {formatMoney(item.quantity * item.ticket_type.price_cents, event.currency)}
                                                    </div>
                                                    <Button
                                                        variant="ghost"
                                                        size="icon"
                                                        className="text-muted-foreground hover:text-destructive"
                                                        onClick={() => removeItem(item.ticket_type_id)}
                                                    >
                                                        <Trash2 className="h-4 w-4" />
                                                    </Button>
                                                </div>
                                            </div>
                                        ))}

                                        <div className="flex items-center justify-between p-4">
                                            <div>
                                                <p className="text-sm text-muted-foreground">Subtotal</p>
                                                <p className="text-xl font-bold">{formatMoney(subtotal, event.currency)}</p>
                                            </div>
                                            <Button onClick={() => handleCheckout(eventId)} disabled={loading}>
                                                Checkout this event
                                            </Button>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>

                        {events.length > 1 && (
                            <p className="mt-6 text-center text-sm text-muted-foreground">
                                Each event is its own order — check out one event at a time.
                            </p>
                        )}

                        <div className="mt-6">
                            <Button variant="outline" onClick={() => navigate('/')}>
                                <ArrowLeft className="mr-2 h-4 w-4" />
                                Continue Shopping
                            </Button>
                        </div>
                    </div>
                </div>
            </div>
        </>
    );
};

export default CartPage;
