import React from 'react';
import { format } from 'date-fns';
import { Clock, MapPin, Loader2 } from 'lucide-react';
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

const OrderSummary = ({ itemsByEvent, total, isProcessing, onCheckout }) => {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Order Summary</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {Object.entries(itemsByEvent).map(([eventId, eventItems]) => {
          const event = eventItems[0].event;
          return (
            <div key={eventId} className="border-b border-gray-200 dark:border-slate-700 pb-4 last:border-0">
              <h3 className="font-medium dark:text-white mb-2">{event.title}</h3>
              <div className="text-sm text-gray-500 dark:text-slate-400 space-y-1 mb-3">
                <div className="flex items-center gap-2">
                  <Clock className="h-4 w-4" />
                  <span>{format(new Date(event.start_time), 'EEE, MMM d, yyyy h:mm a')}</span>
                </div>
                {event.venue_name && (
                  <div className="flex items-center gap-2">
                    <MapPin className="h-4 w-4" />
                    <span>{event.venue_name}</span>
                  </div>
                )}
              </div>
              {eventItems.map((item) => (
                <div 
                  key={item.ticket_type_id}
                  className="flex justify-between text-sm mb-2"
                >
                  <span className="dark:text-white">
                    {item.quantity}x {item.ticket_type.name}
                  </span>
                  <span className="dark:text-white">
                    R{(item.quantity * item.unit_price).toFixed(2)}
                  </span>
                </div>
              ))}
            </div>
          );
        })}
      </CardContent>
      <CardFooter className="flex-col space-y-4">
        <div className="w-full flex justify-between items-center text-lg font-medium">
          <span>Total</span>
          <span>R{total.toFixed(2)}</span>
        </div>
        <Button
          className="w-full bg-[#FF4848] text-white hover:bg-red-600"
          onClick={onCheckout}
          disabled={isProcessing}
        >
          {isProcessing ? (
            <div className="flex items-center gap-2">
              <Loader2 className="h-4 w-4 animate-spin" />
              Processing...
            </div>
          ) : (
            `Pay R${total.toFixed(2)}`
          )}
        </Button>
      </CardFooter>
    </Card>
  );
};

export default OrderSummary;