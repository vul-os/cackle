import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { format } from 'date-fns';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonList } from '@/components/ui/skeleton';
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { ArrowLeft, Receipt, CheckCircle2, XCircle, Loader2 } from 'lucide-react';
import { events as eventsApi, orders as ordersApi } from '@/lib/api';
import { formatMoney } from '@/lib/money';
import { toast } from '@/components/ui/use-toast';

// Every status orders.Order.Status can be (see internal/orders/orders.go).
function statusBadge(status) {
    switch (status) {
        case 'paid':
            return <Badge>Paid</Badge>;
        case 'pending':
            return <Badge variant="secondary">Pending</Badge>;
        case 'failed':
            return <Badge variant="destructive">Failed</Badge>;
        case 'refunded':
            return <Badge variant="destructive">Refunded</Badge>;
        case 'cancelled':
            return <Badge variant="outline">Cancelled</Badge>;
        default:
            return <Badge variant="outline">{status}</Badge>;
    }
}

/**
 * Confirmation dialog shared by the mark-paid and mark-failed actions —
 * both are one-way settlement decisions (mark-paid issues real tickets;
 * mark-failed releases reserved inventory back to sale), so neither should
 * ever fire from a bare click.
 */
const MarkOrderDialog = ({ open, onOpenChange, action, order, currency, isSubmitting, onConfirm }) => {
    const isPaid = action === 'paid';
    return (
        <AlertDialog open={open} onOpenChange={(next) => !isSubmitting && onOpenChange(next)}>
            <AlertDialogContent>
                <AlertDialogHeader>
                    <AlertDialogTitle>{isPaid ? 'Mark this order paid?' : 'Mark this order failed?'}</AlertDialogTitle>
                    <AlertDialogDescription>
                        {isPaid ? (
                            <>
                                This confirms {order ? formatMoney(order.total_minor, currency) : ''} was received for order{' '}
                                {order?.buyer_email ? <strong>{order.buyer_email}</strong> : 'this order'} and issues its tickets
                                immediately. This cannot be undone.
                            </>
                        ) : (
                            <>
                                This records that {order?.buyer_email ? <strong>{order.buyer_email}</strong> : 'this order'} will
                                never be paid and releases the tickets it reserved back to sale. Use this for a buyer who backed
                                out, a duplicate order, or a payment that will never arrive.
                            </>
                        )}
                    </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                    <AlertDialogCancel disabled={isSubmitting}>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                        onClick={(e) => {
                            e.preventDefault();
                            onConfirm();
                        }}
                        disabled={isSubmitting}
                        className={isPaid ? undefined : 'bg-destructive text-destructive-foreground hover:bg-destructive/90'}
                    >
                        {isSubmitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                        {isPaid ? 'Mark paid' : 'Mark failed'}
                    </AlertDialogAction>
                </AlertDialogFooter>
            </AlertDialogContent>
        </AlertDialog>
    );
};

const EventOrdersPage = () => {
    const { id } = useParams();
    const navigate = useNavigate();

    const [event, setEvent] = useState(null);
    const [state, setState] = useState({ orders: [], loading: true, error: null });
    const [dialog, setDialog] = useState(null); // { action: 'paid'|'failed', order }
    const [isSubmitting, setIsSubmitting] = useState(false);

    const fetchOrders = useCallback(() => {
        setState((s) => ({ ...s, loading: true, error: null }));
        Promise.all([eventsApi.get(id), eventsApi.orders(id)])
            .then(([eventData, ordersData]) => {
                setEvent(eventData?.event ?? eventData);
                setState({ orders: ordersData?.orders ?? [], loading: false, error: null });
            })
            .catch((err) => {
                setState({ orders: [], loading: false, error: err.message || 'Could not load orders.' });
            });
    }, [id]);

    useEffect(() => {
        fetchOrders();
    }, [fetchOrders]);

    const currency = event?.currency || '';

    const handleConfirm = async () => {
        if (!dialog) return;
        setIsSubmitting(true);
        try {
            if (dialog.action === 'paid') {
                await ordersApi.markPaid(dialog.order.id);
                toast({ title: 'Order marked paid', description: 'Tickets have been issued.' });
            } else {
                await ordersApi.markFailed(dialog.order.id);
                toast({ title: 'Order marked failed', description: 'Reserved tickets were released back to sale.' });
            }
            setDialog(null);
            fetchOrders();
        } catch (err) {
            toast({ title: 'Could not update order', description: err.message, variant: 'destructive' });
        } finally {
            setIsSubmitting(false);
        }
    };

    return (
        <div className="mx-auto max-w-6xl">
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${id}`)} className="mb-6">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <div className="mb-6 flex items-center gap-3">
                <Receipt className="h-8 w-8 text-primary" />
                <div className="min-w-0">
                    <h1 className="font-display text-3xl font-bold">Orders</h1>
                    {event && <p className="truncate text-sm text-muted-foreground">{event.title}</p>}
                </div>
            </div>

            <Card>
                <CardHeader>
                    <CardTitle>All orders</CardTitle>
                    <CardDescription>
                        Every order placed against this event. Orders paid by bank transfer, cash at the door, or invoice (the
                        <span className="font-mono"> manual</span> payment provider) need to be confirmed here before their
                        tickets are issued.
                    </CardDescription>
                </CardHeader>
                <CardContent>
                    {state.loading && <SkeletonList rows={5} />}

                    {!state.loading && state.error && <ErrorState description={state.error} onRetry={fetchOrders} />}

                    {!state.loading && !state.error && state.orders.length === 0 && (
                        <EmptyState icon={Receipt} title="No orders yet" description="Orders placed for this event will show up here." />
                    )}

                    {!state.loading && !state.error && state.orders.length > 0 && (
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Buyer</TableHead>
                                    <TableHead>Provider</TableHead>
                                    <TableHead>Status</TableHead>
                                    <TableHead className="text-right">Amount</TableHead>
                                    <TableHead>Placed</TableHead>
                                    <TableHead>Marked by</TableHead>
                                    <TableHead className="text-right">Actions</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {state.orders.map((order) => {
                                    const canMark = order.provider === 'manual' && order.status === 'pending';
                                    return (
                                        <TableRow key={order.id}>
                                            <TableCell>
                                                <div className="font-medium">{order.buyer_name || 'Unnamed'}</div>
                                                <div className="text-xs text-muted-foreground">{order.buyer_email}</div>
                                            </TableCell>
                                            <TableCell className="font-mono text-xs">{order.provider}</TableCell>
                                            <TableCell>{statusBadge(order.status)}</TableCell>
                                            <TableCell className="text-right tabular-nums">
                                                {formatMoney(order.total_minor, order.currency || currency)}
                                            </TableCell>
                                            <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                                                {order.created_at ? format(new Date(order.created_at), 'PP p') : '—'}
                                            </TableCell>
                                            <TableCell className="text-sm text-muted-foreground">
                                                {order.marked_by ? (
                                                    <>
                                                        <div>{order.marked_by}</div>
                                                        {order.marked_at && (
                                                            <div className="text-xs">{format(new Date(order.marked_at), 'PP p')}</div>
                                                        )}
                                                    </>
                                                ) : (
                                                    '—'
                                                )}
                                            </TableCell>
                                            <TableCell className="text-right">
                                                {canMark ? (
                                                    <div className="flex justify-end gap-2">
                                                        <Button
                                                            size="sm"
                                                            variant="outline"
                                                            onClick={() => setDialog({ action: 'paid', order })}
                                                        >
                                                            <CheckCircle2 className="mr-1.5 h-3.5 w-3.5" />
                                                            Mark paid
                                                        </Button>
                                                        <Button
                                                            size="sm"
                                                            variant="outline"
                                                            onClick={() => setDialog({ action: 'failed', order })}
                                                        >
                                                            <XCircle className="mr-1.5 h-3.5 w-3.5" />
                                                            Mark failed
                                                        </Button>
                                                    </div>
                                                ) : (
                                                    <span className="text-xs text-muted-foreground">—</span>
                                                )}
                                            </TableCell>
                                        </TableRow>
                                    );
                                })}
                            </TableBody>
                        </Table>
                    )}
                </CardContent>
            </Card>

            <MarkOrderDialog
                open={!!dialog}
                onOpenChange={(next) => !next && setDialog(null)}
                action={dialog?.action}
                order={dialog?.order}
                currency={dialog?.order?.currency || currency}
                isSubmitting={isSubmitting}
                onConfirm={handleConfirm}
            />
        </div>
    );
};

export default EventOrdersPage;
