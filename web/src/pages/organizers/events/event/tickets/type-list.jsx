import React, { memo, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { EmptyState } from '@/components/ui/empty-state';
import { Edit2, Trash2, Ticket } from 'lucide-react';
import { formatMoney } from '@/lib/money';

function saleWindowLabel(tt) {
    if (!tt.sales_start || !tt.sales_end) return null;
    const now = Date.now();
    const start = new Date(tt.sales_start).getTime();
    const end = new Date(tt.sales_end).getTime();
    if (now < start) return `Sales open ${new Date(tt.sales_start).toLocaleDateString()}`;
    if (now > end) return `Sales closed ${new Date(tt.sales_end).toLocaleDateString()}`;
    return `Sales close ${new Date(tt.sales_end).toLocaleDateString()}`;
}

const TicketTypeItem = memo(({ ticketType, currency, onEdit, onDelete }) => {
    const handleEdit = useCallback(() => onEdit(ticketType), [ticketType, onEdit]);
    const handleDelete = useCallback(() => onDelete(ticketType.id), [ticketType.id, onDelete]);

    const total = ticketType.quantity_total ?? 0;
    const sold = ticketType.quantity_sold ?? 0;
    const remaining = Math.max(0, total - sold);
    const pctSold = total > 0 ? Math.min(100, Math.round((sold / total) * 100)) : 0;
    const soldOut = total > 0 && remaining <= 0;
    const windowLabel = saleWindowLabel(ticketType);

    return (
        <div className="rounded-lg border border-border p-4 transition-colors hover:border-primary/40">
            <div className="flex items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                        <h3 className="truncate font-medium">{ticketType.name}</h3>
                        {soldOut && <Badge variant="destructive">Sold out</Badge>}
                        {windowLabel && (
                            <Badge variant="outline" className="font-normal">
                                {windowLabel}
                            </Badge>
                        )}
                    </div>
                    <p className="text-sm text-muted-foreground">
                        {formatMoney(ticketType.price_minor, currency)} · max {ticketType.max_per_order ?? 10} per order
                    </p>
                    {ticketType.description && <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{ticketType.description}</p>}

                    <div className="mt-3 max-w-xs space-y-1">
                        <div className="flex justify-between text-xs text-muted-foreground">
                            <span>
                                {sold} sold{total > 0 ? ` of ${total}` : ''}
                            </span>
                            <span>{remaining} left</span>
                        </div>
                        <Progress value={pctSold} className="h-1.5" />
                    </div>
                </div>
                <div className="flex shrink-0 gap-2">
                    <Button variant="ghost" size="sm" onClick={handleEdit} aria-label={`Edit ${ticketType.name}`}>
                        <Edit2 className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="sm" onClick={handleDelete} aria-label={`Delete ${ticketType.name}`}>
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
        return <EmptyState icon={Ticket} title="No ticket types yet" description="Create one to start selling." />;
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
