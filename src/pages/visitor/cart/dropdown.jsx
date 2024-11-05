import React, { useMemo, useCallback } from 'react';
import { ShoppingCart, Minus, Plus, X, Clock, MapPin } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { Link } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { format } from 'date-fns';

const CartDropdown = ({ isMobile = false }) => {
  const { 
    items, 
    itemsByEvent, 
    itemCount, 
    total, 
    updateQuantity, 
    removeItem, 
    isLoading,
    syncInProgress 
  } = useCart();

  // Memoized handlers to prevent unnecessary re-renders
  const handleRemoveItem = useCallback(async (ticketTypeId) => {
    try {
      console.log('Removing item:', ticketTypeId);
      await removeItem(ticketTypeId);
    } catch (error) {
      console.error('Failed to remove item:', error);
      // You might want to show an error toast here
    }
  }, [removeItem]);

  const handleUpdateQuantity = useCallback(async (ticketTypeId, newQuantity) => {
    try {
      await updateQuantity(ticketTypeId, newQuantity);
    } catch (error) {
      console.error('Failed to update quantity:', error);
      // You might want to show an error toast here
    }
  }, [updateQuantity]);

  // Calculate total items count directly from items array for reliability
  const totalItems = useMemo(() => {
    return items.reduce((sum, item) => sum + (item.quantity || 0), 0);
  }, [items]);

  const formatDateTime = (dateString) => {
    if (!dateString) return '';
    try {
      return format(new Date(dateString), 'EEE, MMM d, h:mm a');
    } catch (error) {
      console.error('Error formatting date:', error);
      return 'Date TBD';
    }
  };

  const renderEventDetails = (event) => {
    if (!event) return null;

    return (
      <div className="mb-3">
        <h3 className="font-medium text-gray-900 dark:text-slate-100">
          {event.title || 'Untitled Event'}
        </h3>
        <div className="mt-1 space-y-1">
          {event.start_time && (
            <div className="flex items-center gap-1.5 text-sm text-gray-500 dark:text-slate-400">
              <Clock className="h-3.5 w-3.5" />
              <span>{formatDateTime(event.start_time)}</span>
            </div>
          )}
          {event.venue_name && (
            <div className="flex items-center gap-1.5 text-sm text-gray-500 dark:text-slate-400">
              <MapPin className="h-3.5 w-3.5" />
              <span>{event.venue_name}</span>
            </div>
          )}
        </div>
      </div>
    );
  };

  const renderCartContent = () => {
    if (isLoading || syncInProgress) {
      return (
        <div className="text-center text-gray-500 dark:text-slate-400 py-8">
          {isLoading ? 'Loading cart...' : 'Updating cart...'}
        </div>
      );
    }

    if (!items.length) {
      return (
        <div className="text-center text-gray-500 dark:text-slate-400 py-8">
          Your cart is empty
        </div>
      );
    }

    return (
      <>
        <div className="space-y-6 max-h-[60vh] overflow-y-auto">
          {Object.entries(itemsByEvent || {}).map(([eventId, eventItems]) => {
            if (!eventItems?.[0]?.event) return null;
            const event = eventItems[0].event;
            
            return (
              <div 
                key={eventId}
                className="pb-4 border-b border-gray-200 dark:border-slate-800 last:border-b-0"
              >
                {renderEventDetails(event)}

                {/* Event Items */}
                <div className="space-y-3">
                  {eventItems.map(item => {
                    if (!item.ticket_type_id) {
                      console.warn('Item missing ticket_type_id:', item);
                      return null;
                    }

                    return (
                      <div 
                        key={`${item.ticket_type_id}-${item.ticket_type?.name}`}
                        className="flex flex-col gap-2"
                      >
                        <div className="flex justify-between items-start">
                          <div>
                            <p className="font-medium text-gray-900 dark:text-slate-100">
                              {item.ticket_type?.name || 'Unnamed Ticket'}
                            </p>
                            <p className="text-sm text-gray-500 dark:text-slate-400">
                              ${(item.unit_price || 0).toFixed(2)} each
                            </p>
                          </div>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200"
                            onClick={() => handleRemoveItem(item.ticket_type_id)}
                            disabled={syncInProgress}
                          >
                            <X className="h-4 w-4" />
                          </Button>
                        </div>
                        
                        <div className="flex items-center gap-2">
                          <Button
                            variant="outline"
                            size="icon"
                            className="h-8 w-8 border-gray-200 dark:border-slate-700"
                            onClick={() => handleUpdateQuantity(item.ticket_type_id, (item.quantity || 0) - 1)}
                            disabled={!item.quantity || item.quantity <= 1 || syncInProgress}
                          >
                            <Minus className="h-4 w-4" />
                          </Button>
                          <span className="w-8 text-center text-gray-900 dark:text-slate-100">
                            {item.quantity || 0}
                          </span>
                          <Button
                            variant="outline"
                            size="icon"
                            className="h-8 w-8 border-gray-200 dark:border-slate-700"
                            onClick={() => handleUpdateQuantity(item.ticket_type_id, (item.quantity || 0) + 1)}
                            disabled={syncInProgress}
                          >
                            <Plus className="h-4 w-4" />
                          </Button>
                          <span className="ml-auto font-medium text-gray-900 dark:text-slate-100">
                            ${((item.quantity * (item.unit_price || 0)) || 0).toFixed(2)}
                          </span>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            );
          })}
        </div>
        
        <div className="border-t border-gray-200 dark:border-slate-800 mt-4 pt-4">
          <div className="flex justify-between font-semibold text-gray-900 dark:text-slate-100">
            <span>Total:</span>
            <span>${(total || 0).toFixed(2)}</span>
          </div>
        </div>
        
        <div className="flex gap-2 mt-4">
          <Button 
            className="flex-1 bg-[#FF4848] hover:bg-red-600 text-white"
            asChild
            disabled={syncInProgress || !items.length}
          >
            <Link to="/cart">Checkout</Link>
          </Button>
        </div>
      </>
    );
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button 
          variant="ghost"
          size={isMobile ? "sm" : "default"}
          className="relative text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100 hover:bg-gray-100 dark:hover:bg-slate-800"
          disabled={syncInProgress}
        >
          <ShoppingCart className="h-5 w-5" />
          {totalItems > 0 && (
            <span className="absolute -top-2 -right-2 bg-[#FF4848] text-white text-xs rounded-full w-5 h-5 flex items-center justify-center">
              {totalItems}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      
      <DropdownMenuContent 
        align="end" 
        className="w-96 bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800"
      >
        <div className="p-4">
          <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-slate-100">
            Shopping Cart
          </h2>
          {renderCartContent()}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default CartDropdown;