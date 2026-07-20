import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { Plus, Minus, Ticket } from 'lucide-react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogTrigger } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/use-toast';

function formatMoney(cents, currency = 'ZAR') {
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format((cents || 0) / 100);
    } catch {
        return `${((cents || 0) / 100).toFixed(2)} ${currency}`;
    }
}

const TicketSelection = ({ event, ticketTypes = [] }) => {
    const { addItem } = useCart();
    const navigate = useNavigate();
    const [open, setOpen] = useState(false);
    const [quantities, setQuantities] = useState({});

    const available = ticketTypes.filter((t) => t.status !== 'hidden');
    const remaining = (t) => Math.max(0, (t.quantity_total ?? 0) - (t.quantity_sold ?? 0));

    const updateQuantity = (id, delta, max) => {
        setQuantities((prev) => {
            const next = Math.max(0, Math.min(max, (prev[id] || 0) + delta));
            return { ...prev, [id]: next };
        });
    };

    const total = available.reduce((sum, t) => sum + (quantities[t.id] || 0) * t.price_cents, 0);
    const totalQuantity = Object.values(quantities).reduce((sum, q) => sum + q, 0);

    const handleAddToCart = () => {
        available.forEach((t) => {
            const qty = quantities[t.id] || 0;
            if (qty > 0) addItem(t, event, qty);
        });
        setQuantities({});
        setOpen(false);
        toast({ title: 'Added to cart', description: `${totalQuantity} ticket${totalQuantity === 1 ? '' : 's'} added.` });
        navigate('/cart');
    };

    if (available.length === 0) {
        return (
            <Button size="lg" disabled className="rounded-full px-8 text-lg">
                <Ticket className="mr-2 h-5 w-5" />
                No tickets available
            </Button>
        );
    }

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                <Button size="lg" className="rounded-full px-8 text-lg shadow-lg">
                    <Ticket className="mr-2 h-5 w-5" />
                    Get Tickets
                </Button>
            </DialogTrigger>

            <DialogContent className="sm:max-w-md">
                <DialogHeader>
                    <DialogTitle>Select tickets</DialogTitle>
                </DialogHeader>

                <div className="max-h-[400px] space-y-3 overflow-y-auto">
                    {available.map((t) => {
                        const left = remaining(t);
                        const qty = quantities[t.id] || 0;
                        const soldOut = left <= 0;
                        return (
                            <div key={t.id} className="flex items-center justify-between rounded-lg border border-border bg-muted/40 p-4">
                                <div className="min-w-0 flex-1 pr-4">
                                    <h3 className="font-medium">{t.name}</h3>
                                    <p className="text-sm text-muted-foreground">{formatMoney(t.price_cents, event?.currency)}</p>
                                    {t.description && <p className="mt-1 text-xs text-muted-foreground">{t.description}</p>}
                                    {soldOut ? (
                                        <p className="mt-1 text-xs font-medium text-destructive">Sold out</p>
                                    ) : (
                                        left <= 10 && <p className="mt-1 text-xs font-medium text-warning">Only {left} left</p>
                                    )}
                                </div>
                                <div className="flex items-center gap-2">
                                    <Button
                                        size="icon"
                                        variant="outline"
                                        className="h-8 w-8"
                                        onClick={() => updateQuantity(t.id, -1, left)}
                                        disabled={qty === 0}
                                    >
                                        <Minus className="h-4 w-4" />
                                    </Button>
                                    <span className="w-6 text-center">{qty}</span>
                                    <Button
                                        size="icon"
                                        variant="outline"
                                        className="h-8 w-8"
                                        onClick={() => updateQuantity(t.id, 1, left)}
                                        disabled={soldOut || qty >= Math.min(left, 10)}
                                    >
                                        <Plus className="h-4 w-4" />
                                    </Button>
                                </div>
                            </div>
                        );
                    })}
                </div>

                <div className="border-t border-border pt-4">
                    <div className="flex justify-between font-medium">
                        <span>Total</span>
                        <span>{formatMoney(total, event?.currency)}</span>
                    </div>
                </div>

                <DialogFooter>
                    <Button onClick={handleAddToCart} disabled={totalQuantity === 0} className="w-full">
                        Add {totalQuantity > 0 ? `${totalQuantity} ` : ''}to Cart
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
};

export default TicketSelection;
