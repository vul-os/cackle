import React from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { format } from 'date-fns';
import { useCart } from '@/context/use-cart';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Separator } from '@/components/ui/separator';
import { Money } from '@/components/ui/money';
import { Minus, Plus, Trash2, ArrowLeft, Clock, MapPin, ShoppingCart, ArrowRight, Lock } from 'lucide-react';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import CheckoutSteps from '@/pages/visitor/checkout/steps';
import { REDIRECT_STORAGE_KEY } from '@/pages/auth/auth-redirect';

function whenLabel(iso: string | null | undefined): string | null {
    if (!iso) return null;
    try {
        return format(new Date(iso), 'EEE, d MMM yyyy · HH:mm');
    } catch {
        return null;
    }
}

const Shell = ({ children }: { children: React.ReactNode }) => (
    <div className="flex min-h-screen flex-col bg-background">
        <Header />
        <main id="main" className="flex-1 pt-16">
            {children}
        </main>
        <Footer />
    </div>
);

const CartPage = () => {
    const { itemsByEvent, itemCount, updateQuantity, removeItem, eventTotal } = useCart();
    const { user, loading } = useAuth();
    const navigate = useNavigate();

    const groups = Object.entries(itemsByEvent);

    const handleCheckout = (eventId: string) => {
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
            <Shell>
                <div className="mx-auto max-w-xl px-4 py-16 sm:py-24">
                    <EmptyState
                        icon={ShoppingCart}
                        title="Your cart is empty"
                        description="Nothing in here yet. Have a look at what's on — most events are a couple of taps away."
                        action={
                            <Button asChild>
                                <Link to="/events">
                                    What&apos;s on
                                    <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                                </Link>
                            </Button>
                        }
                    />
                </div>
            </Shell>
        );
    }

    return (
        <Shell>
            <div className="mx-auto max-w-3xl px-4 py-8 sm:py-12">
                <CheckoutSteps current="cart" className="mb-10" />

                <h1 className="font-display text-display-sm font-extrabold tracking-tight sm:text-display-md">Your cart</h1>
                <p className="mt-1.5 text-sm text-muted-foreground">
                    {itemCount} ticket{itemCount === 1 ? '' : 's'}
                    {groups.length > 1 ? ` across ${groups.length} events` : ''}
                </p>

                <div className="mt-8 space-y-8">
                    {groups.map(([eventId, eventItems]) => {
                        // A group only exists in itemsByEvent because at least
                        // one item was pushed into it, so index 0 is always
                        // populated.
                        const first = eventItems[0];
                        if (!first) return null;
                        const event = first.event;
                        const subtotal = eventTotal(eventId);
                        const when = whenLabel(event.starts_at);

                        return (
                            // One stub per event, because one event is one
                            // order — the seam near the bottom is where the
                            // ticket half ends and the pay-for-it half begins.
                            <section
                                key={eventId}
                                className="ticket-stub overflow-hidden rounded-2xl border border-border bg-card shadow-soft"
                                style={{ '--notch': 'var(--background)', '--notch-y': 'calc(100% - 5.5rem)' } as React.CSSProperties}
                                aria-labelledby={`cart-event-${eventId}`}
                            >
                                <header className="border-b border-border px-5 py-4 sm:px-6">
                                    <h2 id={`cart-event-${eventId}`} className="font-display text-lg font-bold leading-snug tracking-tight">
                                        {event.title}
                                    </h2>
                                    <div className="mt-2 flex flex-col gap-1 text-sm text-muted-foreground sm:flex-row sm:flex-wrap sm:gap-x-5">
                                        {when && (
                                            <span className="flex items-center gap-2">
                                                <Clock className="h-4 w-4 shrink-0" aria-hidden="true" />
                                                {when}
                                            </span>
                                        )}
                                        {event.venue_name && (
                                            <span className="flex items-center gap-2">
                                                <MapPin className="h-4 w-4 shrink-0" aria-hidden="true" />
                                                <span className="truncate">{event.venue_name}</span>
                                            </span>
                                        )}
                                    </div>
                                </header>

                                <ul className="divide-y divide-border">
                                    {eventItems.map((item) => (
                                        <li key={item.ticket_type_id} className="px-5 py-4 sm:px-6">
                                            {/* Stacks below sm. Four controls
                                                and a price on one 390px row is
                                                how a stepper ends up at 32px
                                                wide and a name ends up
                                                truncated to nothing. */}
                                            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
                                                <div className="min-w-0 flex-1">
                                                    <p className="font-semibold leading-snug text-foreground">{item.ticket_type.name}</p>
                                                    <p className="mt-0.5 text-sm text-muted-foreground">
                                                        <Money minor={item.ticket_type.price_minor} currency={event.currency} /> each
                                                    </p>
                                                </div>

                                                <div className="flex items-center justify-between gap-3 sm:justify-end">
                                                    <div className="flex items-center gap-1">
                                                        <Button
                                                            variant="outline"
                                                            size="icon"
                                                            onClick={() => updateQuantity(item.ticket_type_id, item.quantity - 1)}
                                                            aria-label={`One fewer ${item.ticket_type.name}`}
                                                        >
                                                            <Minus className="h-4 w-4" aria-hidden="true" />
                                                        </Button>
                                                        <span
                                                            className="w-9 text-center text-base font-semibold tabular-nums"
                                                            aria-live="polite"
                                                            aria-atomic="true"
                                                        >
                                                            {item.quantity}
                                                            <span className="sr-only"> {item.ticket_type.name}</span>
                                                        </span>
                                                        <Button
                                                            variant="outline"
                                                            size="icon"
                                                            onClick={() => updateQuantity(item.ticket_type_id, item.quantity + 1)}
                                                            aria-label={`One more ${item.ticket_type.name}`}
                                                        >
                                                            <Plus className="h-4 w-4" aria-hidden="true" />
                                                        </Button>
                                                    </div>

                                                    <div className="flex items-center gap-1">
                                                        <span className="w-24 text-right font-semibold tabular-nums">
                                                            <Money
                                                                minor={item.quantity * item.ticket_type.price_minor}
                                                                currency={event.currency}
                                                            />
                                                        </span>
                                                        <Button
                                                            variant="ghost"
                                                            size="icon"
                                                            className="text-muted-foreground hover:text-destructive"
                                                            onClick={() => removeItem(item.ticket_type_id)}
                                                            aria-label={`Remove ${item.ticket_type.name} from cart`}
                                                        >
                                                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                                                        </Button>
                                                    </div>
                                                </div>
                                            </div>
                                        </li>
                                    ))}
                                </ul>

                                <div className="px-5 sm:px-6">
                                    <Separator variant="perforated" style={{ '--notch': 'var(--background)' } as React.CSSProperties} />
                                </div>

                                <div className="flex flex-col gap-4 px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
                                    <div>
                                        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Subtotal</p>
                                        <p className="font-display text-2xl font-bold tabular-nums">
                                            <Money minor={subtotal} currency={event.currency} />
                                        </p>
                                    </div>
                                    <Button
                                        size="lg"
                                        className="w-full sm:w-auto"
                                        onClick={() => handleCheckout(eventId)}
                                        disabled={loading}
                                    >
                                        {user ? 'Check out' : 'Sign in to check out'}
                                        <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                                    </Button>
                                </div>
                            </section>
                        );
                    })}
                </div>

                {groups.length > 1 && (
                    <p className="mt-6 rounded-lg border border-border bg-muted/40 px-4 py-3 text-sm text-muted-foreground">
                        Each event is its own order, paid to its own organiser — so you check out one event at a time.
                    </p>
                )}

                <div className="mt-8 flex flex-wrap items-center justify-between gap-4">
                    <Button variant="outline" asChild>
                        <Link to="/events">
                            <ArrowLeft className="mr-2 h-4 w-4" aria-hidden="true" />
                            Keep browsing
                        </Link>
                    </Button>
                    <p className="flex items-center gap-2 text-xs text-muted-foreground">
                        <Lock className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                        Your cart lives in this browser only — nothing is sent anywhere until you check out.
                    </p>
                </div>
            </div>
        </Shell>
    );
};

export default CartPage;
