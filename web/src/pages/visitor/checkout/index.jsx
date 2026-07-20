import React, { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { useAuth } from '@/context/use-auth';
import Header from '@/pages/visitor/header';
import { Button } from '@/components/ui/button';
import { ArrowLeft, AlertCircle } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';
import { orders as ordersApi } from '@/lib/api';
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
                // No redirect (e.g. free order, or a provider that settles inline)
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
                <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background px-4 pt-16 text-center">
                    <AlertCircle className="h-10 w-10 text-muted-foreground" />
                    <h1 className="text-xl font-semibold">Nothing to check out</h1>
                    <p className="text-muted-foreground">This event isn&apos;t in your cart (anymore).</p>
                    <Button onClick={() => navigate('/cart')}>Back to cart</Button>
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
