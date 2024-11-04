import React from 'react';
import { ShoppingCart, Minus, Plus, X } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { Link } from 'react-router-dom';
import { useCart } from '@/context/cart';

const CartDropdown = ({ isMobile = false }) => {
  const { items, itemCount, total, updateQuantity, removeItem } = useCart();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button 
          variant="ghost"
          size={isMobile ? "sm" : "default"}
          className="relative text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100 hover:bg-gray-100 dark:hover:bg-slate-800"
        >
          <ShoppingCart className="h-5 w-5" />
          {itemCount > 0 && (
            <span className="absolute -top-2 -right-2 bg-[#FF4848] text-white text-xs rounded-full w-5 h-5 flex items-center justify-center">
              {itemCount}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      
      <DropdownMenuContent align="end" className="w-96 bg-white dark:bg-slate-900 border border-gray-200 dark:border-slate-800">
        <div className="p-4">
          <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-slate-100">Shopping Cart</h2>
          
          {items.length > 0 ? (
            <>
              <div className="space-y-4 max-h-[60vh] overflow-y-auto">
                {items.map(item => (
                  <div 
                    // Using combination of id and ticket type for extra uniqueness
                    key={`${item.id}-${item.ticket_type.name}`} 
                    className="flex flex-col gap-2 pb-4 border-b border-gray-200 dark:border-slate-800"
                  >
                    <div className="flex justify-between items-start">
                      <div>
                        <h3 className="font-medium text-gray-900 dark:text-slate-100">
                          {item.ticket_type.name}
                        </h3>
                        <p className="text-sm text-gray-500 dark:text-slate-400">
                          ${item.unit_price.toFixed(2)} each
                        </p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200"
                        onClick={() => removeItem(item.id)}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                    
                    <div className="flex items-center gap-2">
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-8 w-8 border-gray-200 dark:border-slate-700"
                        onClick={() => updateQuantity(item.id, item.quantity - 1)}
                      >
                        <Minus className="h-4 w-4" />
                      </Button>
                      <span className="w-8 text-center text-gray-900 dark:text-slate-100">
                        {item.quantity}
                      </span>
                      <Button
                        variant="outline"
                        size="icon"
                        className="h-8 w-8 border-gray-200 dark:border-slate-700"
                        onClick={() => updateQuantity(item.id, item.quantity + 1)}
                      >
                        <Plus className="h-4 w-4" />
                      </Button>
                      <span className="ml-auto font-medium text-gray-900 dark:text-slate-100">
                        ${item.subtotal.toFixed(2)}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
              
              <div className="border-t border-gray-200 dark:border-slate-800 mt-4 pt-4">
                <div className="flex justify-between font-semibold text-gray-900 dark:text-slate-100">
                  <span>Total:</span>
                  <span>${total.toFixed(2)}</span>
                </div>
              </div>
              
              <Button 
                className="w-full mt-4 bg-[#FF4848] hover:bg-red-600 text-white"
                asChild
              >
                <Link to="/checkout">
                  Proceed to Checkout
                </Link>
              </Button>
            </>
          ) : (
            <div className="text-center text-gray-500 dark:text-slate-400 py-8">
              Your cart is empty
            </div>
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default CartDropdown;