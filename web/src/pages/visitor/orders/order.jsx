import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Check, ChevronLeft, AlertCircle } from 'lucide-react';
import Header from '@/pages/visitor/header';
import { orders as ordersApi } from '@/lib/api';

function formatMoney(cents, currency = 'ZAR') {
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format((cents || 0) / 100);
    } catch {
        return `${((cents || 0) / 100).toFixed(2)} ${currency}`;
    }
}

function statusColor(status) {
    switch (status) {
        case 'paid':
            return 'bg-success/15 text-success';
        case 'pending':
            return 'bg-warning/15 text-warning-foreground';
        case 'failed':
            return 'bg-destructive/15 text-destructive';
        default:
            return 'bg-muted text-muted-foreground';
    }
}

export default function OrderPage() {
    const { id } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState({ order: null, loading: true, error: null });

    useEffect(() => {
        let cancelled = false;
        ordersApi
            .get(id)
            .then((data) => {
                if (cancelled) return;
                setState({ order: data?.order ?? data, loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ order: null, loading: false, error: err.message || 'Order not found.' });
            });
        return () => {
            cancelled = true;
        };
    }, [id]);

    const { order, loading, error } = state;
    const items = order?.items ?? order?.order_items ?? [];

    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main className="pt-24">
                <div className="mx-auto max-w-4xl px-4 py-4">
                    <Button variant="outline" onClick={() => navigate('/orders')} className="flex items-center gap-2">
                        <ChevronLeft className="h-4 w-4" />
                        Back to Orders
                    </Button>
                </div>

                {loading ? (
                    <div className="flex min-h-[400px] items-center justify-center">
                        <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-foreground" />
                    </div>
                ) : error || !order ? (
                    <Card className="mx-auto mt-8 max-w-2xl">
                        <CardContent className="pt-6">
                            <div className="flex flex-col items-center space-y-4 text-center">
                                <AlertCircle className="h-12 w-12 text-destructive" />
                                <div className="space-y-2">
                                    <h2 className="text-2xl font-semibold">Order not found</h2>
                                    <p className="text-muted-foreground">{error || "We couldn't find that order."}</p>
                                </div>
                            </div>
                        </CardContent>
                    </Card>
                ) : (
                    <div className="mx-auto max-w-4xl space-y-6 p-4">
                        <Card>
                            <CardHeader className="space-y-1">
                                <div className="flex items-center justify-between">
                                    <CardTitle className="text-2xl">Order Confirmation</CardTitle>
                                    <span className={`inline-flex items-center rounded-full px-3 py-1 text-sm font-medium ${statusColor(order.status)}`}>
                                        {order.status === 'paid' && <Check className="mr-2 h-4 w-4" />}
                                        {order.status}
                                    </span>
                                </div>
                                <p className="text-sm text-muted-foreground">Order ID: {order.id}</p>
                            </CardHeader>

                            <CardContent className="space-y-6">
                                <div className="space-y-4">
                                    <h3 className="text-lg font-semibold">Order Summary</h3>
                                    <div className="divide-y divide-border">
                                        {items.map((item) => (
                                            <div key={item.id} className="flex justify-between py-4">
                                                <div className="space-y-1">
                                                    <p className="font-medium">{item.ticket_type?.name ?? 'Ticket'}</p>
                                                    <p className="text-sm text-muted-foreground">Quantity: {item.quantity}</p>
                                                </div>
                                                <p className="font-medium">
                                                    {formatMoney((item.unit_price_cents ?? 0) * item.quantity, order.currency)}
                                                </p>
                                            </div>
                                        ))}
                                    </div>

                                    <div className="space-y-2 border-t border-border pt-4">
                                        <div className="flex items-center justify-between text-sm">
                                            <span className="text-muted-foreground">Subtotal</span>
                                            <span>{formatMoney(order.subtotal_cents, order.currency)}</span>
                                        </div>
                                        {order.fee_cents > 0 && (
                                            <div className="flex items-center justify-between text-sm">
                                                <span className="text-muted-foreground">Fees</span>
                                                <span>{formatMoney(order.fee_cents, order.currency)}</span>
                                            </div>
                                        )}
                                        <div className="flex items-center justify-between border-t border-border pt-2">
                                            <span className="font-semibold">Total</span>
                                            <span className="text-lg font-semibold">{formatMoney(order.total_cents, order.currency)}</span>
                                        </div>
                                    </div>
                                </div>

                                <div className="space-y-2 rounded-lg bg-muted/50 p-4 text-sm">
                                    <p>
                                        <span className="text-muted-foreground">Buyer:</span> {order.buyer_name}
                                    </p>
                                    <p>
                                        <span className="text-muted-foreground">Email:</span> {order.buyer_email}
                                    </p>
                                    {order.provider && (
                                        <p>
                                            <span className="text-muted-foreground">Payment via:</span> {order.provider}
                                        </p>
                                    )}
                                </div>

                                {order.status === 'paid' && (
                                    <div className="flex justify-end">
                                        <Button variant="outline" onClick={() => navigate('/tickets')}>
                                            View My Tickets
                                        </Button>
                                    </div>
                                )}
                            </CardContent>
                        </Card>
                    </div>
                )}
            </main>
        </div>
    );
}
