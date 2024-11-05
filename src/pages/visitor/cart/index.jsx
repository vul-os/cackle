import React, { useContext, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { AuthContext } from '@/context/use-auth';
import { Button } from "@/components/ui/button";
import { Minus, Plus, Trash2, ArrowLeft, Clock, MapPin } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import Header from '@/pages/visitor/header';
import { format } from 'date-fns';

const CartPage = () => {
  const { items, itemsByEvent, total, updateQuantity, removeItem } = useCart();
  const { user, loading } = useContext(AuthContext);
  const navigate = useNavigate();
  const [showAuthDialog, setShowAuthDialog] = useState(false);

  const handleCheckout = () => {
    if (!user) {
      setShowAuthDialog(true);
    } else {
      navigate('/checkout');
    }
  };

  const formatDateTime = (dateString) => {
    return format(new Date(dateString), 'EEE, MMM d, yyyy h:mm a');
  };

  if (items.length === 0) {
    return (
      <>
        <Header />
        <div className="min-h-screen bg-gray-50 dark:bg-slate-900 pt-24">
          <div className="container mx-auto px-4 py-8">
            <div className="max-w-2xl mx-auto text-center">
              <h1 className="text-2xl font-bold mb-4 dark:text-white">Your Cart is Empty</h1>
              <p className="text-gray-600 dark:text-slate-400 mb-8">
                Looks like you haven't added any tickets to your cart yet.
              </p>
              <Button
                onClick={() => navigate('/')}
                className="bg-[#FF4848] text-white hover:bg-red-600"
              >
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
      <div className="min-h-screen bg-gray-50 dark:bg-slate-900 pt-24">
        <div className="container mx-auto px-4 py-8">
          <div className="max-w-4xl mx-auto">
            <h1 className="text-3xl font-bold mb-8 dark:text-white">Shopping Cart</h1>
            
            <div className="space-y-6">
              {Object.entries(itemsByEvent).map(([eventId, eventItems]) => {
                const event = eventItems[0].event;
                
                return (
                  <div key={eventId} className="bg-white dark:bg-slate-800 rounded-lg shadow-sm">
                    {/* Event Header */}
                    <div className="p-4 border-b border-gray-200 dark:border-slate-700">
                      <h2 className="text-xl font-semibold dark:text-white mb-2">
                        {event.title}
                      </h2>
                      <div className="space-y-1 text-sm text-gray-500 dark:text-slate-400">
                        <div className="flex items-center gap-2">
                          <Clock className="h-4 w-4" />
                          <span>{formatDateTime(event.start_time)}</span>
                        </div>
                        {event.venue_name && (
                          <div className="flex items-center gap-2">
                            <MapPin className="h-4 w-4" />
                            <span>{event.venue_name}</span>
                          </div>
                        )}
                      </div>
                    </div>

                    {/* Event Items */}
                    {eventItems.map((item) => (
                      <div 
                        key={`${item.ticket_type_id}-${item.ticket_type.name}`}
                        className="flex items-center gap-4 p-4 border-b border-gray-200 dark:border-slate-700 last:border-0"
                      >
                        <div className="flex-1">
                          <h3 className="font-medium dark:text-white">{item.ticket_type.name}</h3>
                          <p className="text-sm text-gray-500 dark:text-slate-400">
                            ${item.unit_price.toFixed(2)} each
                          </p>
                        </div>
                        
                        <div className="flex items-center gap-3">
                          <div className="flex items-center gap-2">
                            <Button
                              variant="outline"
                              size="icon"
                              onClick={() => updateQuantity(item.ticket_type_id, item.quantity - 1)}
                              disabled={item.quantity <= 1}
                              className="h-8 w-8"
                            >
                              <Minus className="h-4 w-4" />
                            </Button>
                            <span className="w-8 text-center dark:text-white">
                              {item.quantity}
                            </span>
                            <Button
                              variant="outline"
                              size="icon"
                              onClick={() => updateQuantity(item.ticket_type_id, item.quantity + 1)}
                              className="h-8 w-8"
                            >
                              <Plus className="h-4 w-4" />
                            </Button>
                          </div>
                          
                          <div className="w-24 text-right font-medium dark:text-white">
                            ${(item.quantity * item.unit_price).toFixed(2)}
                          </div>
                          
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => removeItem(item.ticket_type_id)}
                            className="text-gray-400 hover:text-red-600"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                );
              })}
              
              {/* Cart Summary */}
              <div className="bg-white dark:bg-slate-800 rounded-lg shadow-sm p-4">
                <div className="flex justify-between items-center">
                  <div>
                    <p className="text-lg font-medium dark:text-white">Total</p>
                    <p className="text-sm text-gray-500 dark:text-slate-400">
                      {items.reduce((acc, item) => acc + item.quantity, 0)} items
                    </p>
                  </div>
                  <div className="text-2xl font-bold dark:text-white">
                    ${total.toFixed(2)}
                  </div>
                </div>
                
                <div className="mt-6 flex gap-4">
                  <Button
                    variant="outline"
                    onClick={() => navigate('/')}
                    className="flex-1"
                  >
                    Continue Shopping
                  </Button>
                  <Button
                    onClick={handleCheckout}
                    className="flex-1 bg-[#FF4848] text-white hover:bg-red-600"
                    disabled={loading}
                  >
                    {loading ? 'Loading...' : 'Checkout'}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <Dialog open={showAuthDialog} onOpenChange={setShowAuthDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Authentication Required</DialogTitle>
            <DialogDescription>
              You need to be logged in to proceed with checkout.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => setShowAuthDialog(false)}
            >
              Cancel
            </Button>
            <Button
              onClick={() => {
                setShowAuthDialog(false);
                navigate('/login');
              }}
              className="bg-[#FF4848] text-white hover:bg-red-600"
            >
              Login
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default CartPage;