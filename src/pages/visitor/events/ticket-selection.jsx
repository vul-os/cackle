// components/TicketSelection.jsx
import React, { useState } from 'react';
import { useCart } from '@/context/cart';
import { Plus, Minus, Ticket } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";

const TicketSelection = ({ ticketTypes }) => {
  const { addItem } = useCart();
  const [open, setOpen] = useState(false);
  const [quantities, setQuantities] = useState(
    ticketTypes.reduce((acc, ticket) => ({ ...acc, [ticket.id]: 0 }), {})
  );

  const updateQuantity = (ticketId, delta) => {
    setQuantities(prev => ({
      ...prev,
      [ticketId]: Math.max(0, prev[ticketId] + delta)
    }));
  };

  const total = ticketTypes.reduce((sum, ticket) => 
    sum + (ticket.price * quantities[ticket.id]), 0
  );

  const handleAddToCart = () => {
    Object.entries(quantities).forEach(([ticketId, quantity]) => {
      if (quantity > 0) {
        const ticketType = ticketTypes.find(tt => tt.id === ticketId);
        addItem(ticketType, quantity);
      }
    });
    
    // Reset quantities
    setQuantities(ticketTypes.reduce((acc, ticket) => ({ ...acc, [ticket.id]: 0 }), {}));
    // Close the dialog
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button className="bg-gradient-to-r from-[#880424] to-[#660318] hover:from-[#990525] hover:to-[#770419] text-lg px-8 py-6 shadow-lg hover:shadow-xl transition-all text-white rounded-full">
          <Ticket className="h-5 w-5 mr-2" />
          Get Tickets
        </Button>
      </DialogTrigger>
      
      <DialogContent className="sm:max-w-md bg-gray-900 text-white">
        <DialogHeader>
          <DialogTitle>Select Tickets</DialogTitle>
        </DialogHeader>
        
        <div className="space-y-4">
          {ticketTypes.map(ticket => (
            <div key={ticket.id} className="flex items-center justify-between p-4 border border-white/10 rounded-lg bg-white/5">
              <div>
                <h3 className="font-medium text-white">{ticket.name}</h3>
                <p className="text-sm text-gray-300">${ticket.price.toFixed(2)}</p>
                {ticket.description && (
                  <p className="text-sm text-gray-400">{ticket.description}</p>
                )}
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => updateQuantity(ticket.id, -1)}
                  disabled={quantities[ticket.id] === 0}
                  className="text-white border-white/20"
                >
                  <Minus className="h-4 w-4" />
                </Button>
                <span className="w-8 text-center text-white">{quantities[ticket.id]}</span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => updateQuantity(ticket.id, 1)}
                  disabled={quantities[ticket.id] >= ticket.quantity_total}
                  className="text-white border-white/20"
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>

        <div className="mt-4 pt-4 border-t border-white/10">
          <div className="flex justify-between font-medium text-white">
            <span>Total:</span>
            <span>${total.toFixed(2)}</span>
          </div>
        </div>

        <DialogFooter>
          <Button
            onClick={handleAddToCart}
            disabled={total === 0}
            className="w-full bg-gradient-to-r from-[#880424] to-[#660318] hover:from-[#990525] hover:to-[#770419]"
          >
            Add to Cart (${total.toFixed(2)})
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default TicketSelection;