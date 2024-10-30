import React, { memo, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Edit2, Trash2 } from 'lucide-react';

const TicketTypeItem = memo(({ ticketType, onEdit, onDelete }) => {
  const handleEdit = useCallback(() => {
    onEdit(ticketType);
  }, [ticketType, onEdit]);

  const handleDelete = useCallback(() => {
    onDelete(ticketType.id);
  }, [ticketType.id, onDelete]);

  const availableTickets = ticketType.quantity_total - (ticketType.quantity_sold || 0);

  return (
    <div className="p-4 border rounded-lg hover:border-gray-300 transition-colors">
      <div className="flex items-center justify-between">
        <div className="flex-1 min-w-0">
          <h3 className="font-medium truncate">{ticketType.name}</h3>
          <p className="text-sm text-gray-500">
            Price: ${ticketType.price.toFixed(2)} • Available: {availableTickets}
          </p>
          {ticketType.description && (
            <p className="text-sm text-gray-600 mt-1 line-clamp-2">{ticketType.description}</p>
          )}
        </div>
        <div className="flex gap-2 flex-shrink-0 ml-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleEdit}
            className="hover:bg-gray-100"
          >
            <Edit2 className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleDelete}
            className="hover:bg-gray-100"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
});

// Add display name for better debugging
TicketTypeItem.displayName = 'TicketTypeItem';

const TicketTypeList = memo(({ ticketTypes, onEdit, onDelete }) => {
  if (ticketTypes.length === 0) {
    return (
      <div className="text-center py-8 text-gray-500">
        No ticket types created yet. Click "Create Ticket Type" to get started.
      </div>
    );
  }

  return (
    <div className="grid gap-4">
      {ticketTypes.map((ticketType) => (
        <TicketTypeItem
          key={ticketType.id}
          ticketType={ticketType}
          onEdit={onEdit}
          onDelete={onDelete}
        />
      ))}
    </div>
  );
});

// Add display name for better debugging
TicketTypeList.displayName = 'TicketTypeList';

export default TicketTypeList;