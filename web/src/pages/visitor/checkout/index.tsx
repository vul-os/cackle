import React, { useState } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { useAuth } from '@/context/use-auth';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import { Button } from '@/components/ui/button';
import { ArrowLeft, ShoppingCart } from 'lucide-react';
import { EmptyState } from '@/components/ui/empty-state';
import { toast } from '@/components/ui/use-toast';
import { orders as ordersApi, payments as paymentsApi } from '@/lib/api';
import BillingForm, { type BillingDetails, type BillingErrors } from './billing-form';
import OrderSummary, { type OrderIssue } from './order-summary';
import PaymentRedirectPage from './redirect';
import CheckoutSteps from './steps';

// Deliberately permissive: this is a client-side sanity check that catches
// an empty box and a missing @, not an attempt to decide what a valid
// address looks like. The server is the authority; this exists so the buyer
// finds out before the round trip.
const looksLikeEmail = (value: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());

// Checkout runs against LIVE inventory (internal/orders.Service.Create):
// quantity caps, sales windows and per-order limits are all real, and a
// buyer sitting on this page for a few minutes — or racing someone else for
// the last few tickets — can genuinely hit them. The API's error message is
// the raw sentinel text from internal/orders/orders.go (e.g. "orders: sold
// out: …"), which is meant for a log line, not a buyer. This maps the known
// shapes to a state that says what happened and gives a real way forward —
// everything else falls through to the generic toast below, unclassified
// rather than guessed at.
function classifyOrderError(message: string | null | undefined): OrderIssue | null {
    const m = (message || '').toLowerCase();
    if (m.includes('sold out')) {
        return {
            title: 'Sold out',
            description:
                "Someone else bought the last of these while you were checking out. Nothing has been charged — head back to your cart to pick a different ticket type or quantity.",
        };
    }
    if (m.includes('max per order')) {
        return {
            title: 'Too many for one order',
            description:
                'This ticket type limits how many you can buy in a single order. Go back to your cart and lower the quantity.',
        };
    }
    if (m.includes('not currently on sale') || m.includes('is not published')) {
        return {
            title: 'No longer available',
            description:
                'Sales for this have closed, or the event was taken down since you added it. Nothing has been charged — check your cart for what is still on sale.',
        };
    }
    return null;
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

const BILLING_FIELDS = ['name', 'email'] as const;

const CheckoutPage = () => {
    const navigate = useNavigate();
    const { eventId } = useParams();
    const { itemsByEvent, eventTotal, clearEvent } = useCart();
    const { user } = useAuth();

    const items = itemsByEvent[eventId ?? ''] || [];
    const event = items[0]?.event;

    const [isProcessing, setIsProcessing] = useState(false);
    const [redirectUrl, setRedirectUrl] = useState<string | null>(null);
    const [billingDetails, setBillingDetails] = useState<BillingDetails>({ name: user?.name || '', email: user?.email || '' });
    const [errors, setErrors] = useState<BillingErrors>({});
    // A classified inventory failure (sold out / window closed / per-order
    // cap) rendered inline in the order summary — see classifyOrderError.
    // Anything unclassified stays a toast rather than a guessed-at state.
    const [orderIssue, setOrderIssue] = useState<OrderIssue | null>(null);

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const name = e.target.name as keyof BillingDetails;
        const { value } = e.target;
        setBillingDetails((prev) => ({ ...prev, [name]: value }));
        // Clear the field's error as soon as it is touched — leaving it up
        // while someone is fixing it reads as "still wrong".
        setErrors((prev) => (prev[name] ? { ...prev, [name]: undefined } : prev));
    };

    const validate = () => {
        const next: BillingErrors = {};
        if (!billingDetails.name.trim()) next.name = 'We need a name to put on the ticket.';
        if (!billingDetails.email.trim()) next.email = 'We need an email to send the ticket to.';
        else if (!looksLikeEmail(billingDetails.email)) next.email = "That doesn't look like an email address.";
        setErrors(next);
        return Object.keys(next).length === 0;
    };

    const handleCheckout = async () => {
        if (!validate()) {
            // Move focus to the first bad field rather than only colouring
            // it: on a phone the summary and the form are far apart.
            const firstBad = BILLING_FIELDS.find((k) => document.getElementById(k) && !billingDetails[k].trim().length) || 'name';
            document.getElementById(firstBad)?.focus();
            document.getElementById(firstBad)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
            return;
        }

        const id = eventId ?? '';
        setOrderIssue(null);
        setIsProcessing(true);
        try {
            const result = await ordersApi.create({
                event_id: id,
                items: items.map((i) => ({ ticket_type_id: i.ticket_type_id, quantity: i.quantity })),
                buyer: { name: billingDetails.name, email: billingDetails.email },
            });

            clearEvent(id);

            if (result?.payment?.redirect_url) {
                setRedirectUrl(result.payment.redirect_url);
            } else if (result?.order?.id) {
                // No redirect: either the provider settles inline (e.g.
                // --demo's stub) or needs out-of-band confirmation (manual,
                // an invoice-style crypto provider). Either way, an order's
                // id IS its provider reference (see internal/orders' Order
                // doc comment) — poll verify once so an inline provider's
                // settlement is reflected immediately rather than leaving
                // the order stuck showing "pending" until something else
                // happens to call verify. A provider that isn't confirmed
                // yet (manual/invoice) rejects this with "not confirmed",
                // which is expected, not an error — the order page still
                // shows its real (pending) status either way.
                try {
                    await paymentsApi.verify(result.order.id);
                } catch {
                    // Not settled yet — normal for manual/invoice-style
                    // providers; nothing to do here.
                }
                navigate(`/order/${result.order.id}`);
            } else {
                navigate('/orders');
            }
        } catch (err) {
            const message = err instanceof Error ? err.message : undefined;
            const known = classifyOrderError(message);
            if (known) {
                // A real, expected state — not a toast that vanishes before a
                // phone screen has scrolled back up to see it.
                setOrderIssue(known);
            } else {
                toast({ title: 'Checkout failed', description: message || 'Please try again.', variant: 'destructive' });
            }
        } finally {
            setIsProcessing(false);
        }
    };

    if (redirectUrl) {
        return <PaymentRedirectPage redirectUrl={redirectUrl} />;
    }

    if (!event) {
        return (
            <Shell>
                <div className="mx-auto max-w-xl px-4 py-16 sm:py-24">
                    <EmptyState
                        icon={ShoppingCart}
                        title="Nothing to check out"
                        description="This event isn't in your cart any more — it may have been cleared, or already paid for."
                        action={
                            <div className="flex flex-wrap justify-center gap-3">
                                <Button asChild>
                                    <Link to="/cart">Back to cart</Link>
                                </Button>
                                <Button variant="outline" asChild>
                                    <Link to="/orders">See my orders</Link>
                                </Button>
                            </div>
                        }
                    />
                </div>
            </Shell>
        );
    }

    return (
        <Shell>
            <div className="mx-auto max-w-5xl px-4 py-8 sm:py-12">
                <CheckoutSteps current="details" className="mb-10" />

                <div className="mb-8">
                    <Button variant="ghost" className="-ml-2" asChild>
                        <Link to="/cart">
                            <ArrowLeft className="mr-2 h-4 w-4" aria-hidden="true" />
                            Back to cart
                        </Link>
                    </Button>
                    <h1 className="mt-3 font-display text-display-sm font-extrabold tracking-tight sm:text-display-md">
                        Check out
                    </h1>
                </div>

                <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[1fr_380px] lg:gap-8">
                    <BillingForm billingDetails={billingDetails} handleInputChange={handleInputChange} errors={errors} />
                    <OrderSummary
                        event={event}
                        items={items}
                        total={eventTotal(eventId ?? '')}
                        isProcessing={isProcessing}
                        onCheckout={handleCheckout}
                        issue={orderIssue}
                    />
                </div>
            </div>
        </Shell>
    );
};

export default CheckoutPage;
