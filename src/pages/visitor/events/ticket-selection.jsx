import React, { useState } from 'react';
import { useCart } from '@/context/use-cart';
import { Plus, Minus, Ticket, ChevronDown } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";

const TICKET_TYPES = {
  GENERAL: 'general',
  VIP: 'vip',
  EARLY_BIRD: 'early-bird',
  GROUP: 'group',
  STUDENT: 'student'
};

const TicketSelection = ({ event }) => {
  const { addItem } = useCart();
  const [open, setOpen] = useState(false);
  const [selectedType, setSelectedType] = useState("all");
  
  const ticketTypes = [
    {
      id: "1",
      name: "General Admission",
      type: TICKET_TYPES.GENERAL,
      price: 99.99,
      quantity_total: 1000,
      description: "Standard event access"
    },
    {
      id: "2",
      name: "VIP Pass",
      type: TICKET_TYPES.VIP,
      price: 299.99,
      quantity_total: 100,
      description: "Premium access with exclusive perks"
    },
    {
      id: "3",
      name: "Early Bird Special",
      type: TICKET_TYPES.EARLY_BIRD,
      price: 79.99,
      quantity_total: 200,
      description: "Limited time discount pricing"
    },
    {
      id: "4",
      name: "Group Package (5+ people)",
      type: TICKET_TYPES.GROUP,
      price: 89.99,
      quantity_total: 50,
      description: "Discounted rate for groups"
    },
    {
      id: "5",
      name: "Student Ticket",
      type: TICKET_TYPES.STUDENT,
      price: 49.99,
      quantity_total: 300,
      description: "Valid student ID required"
    }
  ];

  const [quantities, setQuantities] = useState(
    ticketTypes.reduce((acc, ticket) => ({ ...acc, [ticket.id]: 0 }), {})
  );

  const uniqueTypes = ["all", ...new Set(ticketTypes.map(ticket => ticket.type))];

  const filteredTickets = selectedType === "all" 
    ? ticketTypes 
    : ticketTypes.filter(ticket => ticket.type === selectedType);

  const updateQuantity = (ticketId, delta) => {
    setQuantities(prev => ({
      ...prev,
      [ticketId]: Math.max(0, prev[ticketId] + delta)
    }));
  };

  const total = ticketTypes.reduce((sum, ticket) => 
    sum + (ticket.price * quantities[ticket.id]), 0
  );

  const formatTicketType = (type) => {
    if (type === 'all') return 'All Tickets';
    return type.split('-')
      .map(word => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ');
  };

  const handleAddToCart = () => {
    Object.entries(quantities).forEach(([ticketId, quantity]) => {
      if (quantity > 0) {
        const ticketType = ticketTypes.find(tt => tt.id === ticketId);
        if (ticketType) {
          addItem(ticketType, event, quantity);
        }
      }
    });
    setQuantities(ticketTypes.reduce((acc, ticket) => ({ ...acc, [ticket.id]: 0 }), {}));
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
      
      <DialogContent className="sm:max-w-md bg-white dark:bg-gray-900 text-gray-900 dark:text-white">
        <DialogHeader>
          <DialogTitle>Select Tickets</DialogTitle>
        </DialogHeader>

        <div className="mb-4">
          <Select value={selectedType} onValueChange={setSelectedType}>
            <SelectTrigger className="w-full bg-gray-100 dark:bg-gray-800 border-gray-200 dark:border-white/10">
              <SelectValue placeholder="Select ticket type">
                {formatTicketType(selectedType)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent className="bg-white dark:bg-gray-800 border-gray-200 dark:border-white/10">
              {uniqueTypes.map(type => (
                <SelectItem 
                  key={type} 
                  value={type}
                  className="hover:bg-gray-100 dark:hover:bg-gray-700 focus:bg-gray-100 dark:focus:bg-gray-700 cursor-pointer"
                >
                  {formatTicketType(type)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        
        <div className="space-y-4 max-h-[400px] overflow-y-auto">
          {filteredTickets.map(ticket => (
            <div 
              key={ticket.id} 
              className="flex items-center justify-between p-4 border border-gray-200 dark:border-white/10 rounded-lg bg-gray-50 dark:bg-white/5"
            >
              <div>
                <h3 className="font-medium text-gray-900 dark:text-white">
                  {ticket.name}
                </h3>
                <p className="text-sm text-gray-600 dark:text-gray-300">
                  R{ticket.price.toFixed(2)}
                </p>
                {ticket.description && (
                  <p className="text-sm text-gray-500 dark:text-gray-400">
                    {ticket.description}
                  </p>
                )}
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => updateQuantity(ticket.id, -1)}
                  disabled={quantities[ticket.id] === 0}
                  className="border-gray-300 dark:border-white/20"
                >
                  <Minus className="h-4 w-4" />
                </Button>
                <span className="w-8 text-center">
                  {quantities[ticket.id]}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => updateQuantity(ticket.id, 1)}
                  disabled={quantities[ticket.id] >= ticket.quantity_total}
                  className="border-gray-300 dark:border-white/20"
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>

        <div className="mt-4 pt-4 border-t border-gray-200 dark:border-white/10">
          <div className="flex justify-between font-medium">
            <span>Total:</span>
            <span>R{total.toFixed(2)}</span>
          </div>
        </div>

        <DialogFooter>
          <Button
            onClick={handleAddToCart}
            disabled={total === 0}
            className="w-full bg-gradient-to-r from-[#880424] to-[#660318] hover:from-[#990525] hover:to-[#770419] text-white"
          >
            Add to Cart (R{total.toFixed(2)})
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default TicketSelection;