import React from 'react';
import { format } from 'date-fns';
import { Clock, MapPin, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { formatMoney } from '@/lib/money';

const OrderSummary = ({ event, items, total, isProcessing, onCheckout }) => {
    return (
        <Card>
            <CardHeader>
                <CardTitle>Order Summary</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <h3 className="mb-2 font-medium">{event.title}</h3>
                    <div className="mb-3 space-y-1 text-sm text-muted-foreground">
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
                    {items.map((item) => (
                        <div key={item.ticket_type_id} className="mb-2 flex justify-between text-sm">
                            <span>
                                {item.quantity}x {item.ticket_type.name}
                            </span>
                            <span>{formatMoney(item.quantity * item.ticket_type.price_minor, event.currency)}</span>
                        </div>
                    ))}
                </div>
            </CardContent>
            <CardFooter className="flex-col space-y-4">
                <div className="flex w-full items-center justify-between text-lg font-medium">
                    <span>Total</span>
                    <span>{formatMoney(total, event.currency)}</span>
                </div>
                <Button className="w-full" onClick={onCheckout} disabled={isProcessing}>
                    {isProcessing ? (
                        <div className="flex items-center gap-2">
                            <Loader2 className="h-4 w-4 animate-spin" />
                            Processing...
                        </div>
                    ) : (
                        `Pay ${formatMoney(total, event.currency)}`
                    )}
                </Button>
            </CardFooter>
        </Card>
    );
};

export default OrderSummary;
