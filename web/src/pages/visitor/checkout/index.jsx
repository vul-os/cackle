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
import BillingForm from './billing-form';
import OrderSummary from './order-summary';
import PaymentRedirectPage from './redirect';
import CheckoutSteps from './steps';
import { TAP_BUTTON } from '@/pages/visitor/ui-scale';

// Deliberately permissive: this is a client-side sanity check that catches
// an empty box and a missing @, not an attempt to decide what a valid
// address looks like. The server is the authority; this exists so the buyer
// finds out before the round trip.
const looksLikeEmail = (value) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());

const Shell = ({ children }) => (
    <div className="flex min-h-screen flex-col bg-background">
        <Header />
        <main id="main" className="flex-1 pt-16">
            {children}
        </main>
        <Footer />
    </div>
);

const CheckoutPage = () => {
    const navigate = useNavigate();
    const { eventId } = useParams();
    const { itemsByEvent, eventTotal, clearEvent } = useCart();
    const { user } = useAuth();

    const items = itemsByEvent[eventId] || [];
    const event = items[0]?.event;

    const [isProcessing, setIsProcessing] = useState(false);
    const [redirectUrl, setRedirectUrl] = useState(null);
    const [billingDetails, setBillingDetails] = useState({ name: user?.name || '', email: user?.email || '' });
    const [errors, setErrors] = useState({});

    const handleInputChange = (e) => {
        const { name, value } = e.target;
        setBillingDetails((prev) => ({ ...prev, [name]: value }));
        // Clear the field's error as soon as it is touched — leaving it up
        // while someone is fixing it reads as "still wrong".
        setErrors((prev) => (prev[name] ? { ...prev, [name]: undefined } : prev));
    };

    const validate = () => {
        const next = {};
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
            const firstBad = ['name', 'email'].find((k) => document.getElementById(k) && !billingDetails[k].trim().length) || 'name';
            document.getElementById(firstBad)?.focus();
            document.getElementById(firstBad)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
            return;
        }

        setIsProcessing(true);
        try {
            const result = await ordersApi.create({
                event_id: eventId,
                items: items.map((i) => ({ ticket_type_id: i.ticket_type_id, quantity: i.quantity })),
                buyer: { name: billingDetails.name, email: billingDetails.email },
            });

            clearEvent(eventId);

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
            toast({ title: 'Checkout failed', description: err.message || 'Please try again.', variant: 'destructive' });
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
                                <Button className={TAP_BUTTON} asChild>
                                    <Link to="/cart">Back to cart</Link>
                                </Button>
                                <Button variant="outline" className={TAP_BUTTON} asChild>
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
                    <Button variant="ghost" className={`-ml-2 ${TAP_BUTTON}`} asChild>
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
                        total={eventTotal(eventId)}
                        isProcessing={isProcessing}
                        onCheckout={handleCheckout}
                    />
                </div>
            </div>
        </Shell>
    );
};

export default CheckoutPage;
