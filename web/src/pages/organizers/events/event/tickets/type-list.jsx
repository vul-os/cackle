import React, { memo, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Edit2, Trash2, Ticket } from 'lucide-react';

function formatMoney(cents, currency = 'ZAR') {
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format((cents || 0) / 100);
    } catch {
        return `${((cents || 0) / 100).toFixed(2)} ${currency}`;
    }
}

const TicketTypeItem = memo(({ ticketType, currency, onEdit, onDelete }) => {
    const handleEdit = useCallback(() => onEdit(ticketType), [ticketType, onEdit]);
    const handleDelete = useCallback(() => onDelete(ticketType.id), [ticketType.id, onDelete]);

    const available = (ticketType.quantity_total ?? 0) - (ticketType.quantity_sold ?? 0);

    return (
        <div className="rounded-lg border border-border p-4 transition-colors hover:border-primary/40">
            <div className="flex items-center justify-between">
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                        <h3 className="truncate font-medium">{ticketType.name}</h3>
                        {available <= 0 && <Badge variant="destructive">Sold out</Badge>}
                    </div>
                    <p className="text-sm text-muted-foreground">
                        {formatMoney(ticketType.price_cents, currency)} · {ticketType.quantity_sold ?? 0}/{ticketType.quantity_total ?? 0} sold
                    </p>
                    {ticketType.description && <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{ticketType.description}</p>}
                </div>
                <div className="ml-4 flex shrink-0 gap-2">
                    <Button variant="ghost" size="sm" onClick={handleEdit}>
                        <Edit2 className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="sm" onClick={handleDelete}>
                        <Trash2 className="h-4 w-4" />
                    </Button>
                </div>
            </div>
        </div>
    );
});
TicketTypeItem.displayName = 'TicketTypeItem';

const TicketTypeList = memo(({ ticketTypes, currency, onEdit, onDelete }) => {
    if (ticketTypes.length === 0) {
        return (
            <div className="flex flex-col items-center gap-2 py-12 text-center text-muted-foreground">
                <Ticket className="h-8 w-8" />
                <p>No ticket types yet. Create one to start selling.</p>
            </div>
        );
    }

    return (
        <div className="grid gap-4">
            {ticketTypes.map((tt) => (
                <TicketTypeItem key={tt.id} ticketType={tt} currency={currency} onEdit={onEdit} onDelete={onDelete} />
            ))}
        </div>
    );
});
TicketTypeList.displayName = 'TicketTypeList';

export default TicketTypeList;
