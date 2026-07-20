import React, { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { useAuth } from '@/context/use-auth';
import Header from '@/pages/visitor/header';
import { Button } from '@/components/ui/button';
import { ArrowLeft, ShoppingCart } from 'lucide-react';
import { EmptyState } from '@/components/ui/empty-state';
import { toast } from '@/components/ui/use-toast';
import { orders as ordersApi, payments as paymentsApi } from '@/lib/api';
import BillingForm from './billing-form';
import OrderSummary from './order-summary';
import PaymentRedirectPage from './redirect';

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

    const handleInputChange = (e) => {
        const { name, value } = e.target;
        setBillingDetails((prev) => ({ ...prev, [name]: value }));
    };

    const handleCheckout = async () => {
        if (!billingDetails.name.trim() || !billingDetails.email.trim()) {
            toast({ title: 'Missing details', description: 'Please fill in your name and email.', variant: 'destructive' });
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
            <>
                <Header />
                <div className="min-h-screen bg-background px-4 pt-24">
                    <div className="mx-auto max-w-lg">
                        <EmptyState
                            icon={ShoppingCart}
                            title="Nothing to check out"
                            description="This event isn't in your cart (anymore)."
                            action={<Button onClick={() => navigate('/cart')}>Back to cart</Button>}
                        />
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
                    <div className="mx-auto max-w-6xl">
                        <Button variant="ghost" onClick={() => navigate('/cart')} className="mb-6">
                            <ArrowLeft className="mr-2 h-4 w-4" />
                            Back to Cart
                        </Button>

                        <div className="grid grid-cols-1 gap-8 md:grid-cols-2">
                            <BillingForm billingDetails={billingDetails} handleInputChange={handleInputChange} />
                            <OrderSummary
                                event={event}
                                items={items}
                                total={eventTotal(eventId)}
                                isProcessing={isProcessing}
                                onCheckout={handleCheckout}
                            />
                        </div>
                    </div>
                </div>
            </div>
        </>
    );
};

export default CheckoutPage;
