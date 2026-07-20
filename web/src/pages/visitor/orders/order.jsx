import React, { useEffect, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ErrorState } from '@/components/ui/error-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Check, ChevronLeft, ChevronRight, Calendar, MapPin, Ticket, Clock, XCircle, RotateCcw, Hash } from 'lucide-react';
import Header from '@/pages/visitor/header';
import { orders as ordersApi, events as eventsApi, tickets as ticketsApi } from '@/lib/api';
import { formatMoney } from '@/lib/money';

function formatWhen(iso) {
    if (!iso) return null;
    try {
        return new Date(iso).toLocaleString(undefined, {
            weekday: 'short',
            month: 'short',
            day: 'numeric',
            year: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        });
    } catch {
        return null;
    }
}

function statusColor(status) {
    switch (status) {
        case 'paid':
            return 'bg-success/15 text-success';
        case 'pending':
            return 'bg-warning/15 text-warning-foreground';
        case 'failed':
        case 'cancelled':
            return 'bg-destructive/15 text-destructive';
        case 'refunded':
            return 'bg-muted text-muted-foreground';
        default:
            return 'bg-muted text-muted-foreground';
    }
}

// Order status is a small state machine, not a free-form log — the API
// exposes only `status` plus `created_at`/`paid_at` timestamps, so the
// timeline below is derived, not fetched. It's still accurate: every
// terminal status implies exactly one path through "placed → ...".
function buildTimeline(order) {
    const steps = [{ key: 'created', label: 'Order placed', at: order.created_at, tone: 'done' }];
    switch (order.status) {
        case 'paid':
            steps.push({ key: 'paid', label: 'Payment confirmed', at: order.paid_at, tone: 'done' });
            break;
        case 'pending':
            steps.push({ key: 'pending', label: 'Awaiting payment', at: null, tone: 'current' });
            break;
        case 'failed':
            steps.push({ key: 'failed', label: 'Payment failed', at: null, tone: 'error' });
            break;
        case 'refunded':
            steps.push({ key: 'paid', label: 'Payment confirmed', at: order.paid_at, tone: 'done' });
            steps.push({ key: 'refunded', label: 'Refunded', at: null, tone: 'error' });
            break;
        case 'cancelled':
            steps.push({ key: 'cancelled', label: 'Cancelled', at: null, tone: 'error' });
            break;
        default:
            break;
    }
    return steps;
}

const TIMELINE_ICON = { done: Check, current: Clock, error: XCircle };
const TIMELINE_DOT_CLASS = {
    done: 'border-success bg-success text-success-foreground',
    current: 'border-warning bg-warning text-warning-foreground',
    error: 'border-destructive bg-destructive text-destructive-foreground',
};

function OrderTimeline({ order }) {
    const steps = buildTimeline(order);
    return (
        <ol className="space-y-0">
            {steps.map((step, i) => {
                const Icon = TIMELINE_ICON[step.tone];
                const when = formatWhen(step.at);
                return (
                    <li key={step.key} className="relative flex gap-4 pb-6 last:pb-0">
                        {i < steps.length - 1 && (
                            <span className="absolute left-[15px] top-8 h-full w-px bg-border" aria-hidden="true" />
                        )}
                        <span
                            className={`z-10 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 ${TIMELINE_DOT_CLASS[step.tone]}`}
                        >
                            <Icon className="h-4 w-4" aria-hidden="true" />
                        </span>
                        <div className="pt-1">
                            <p className="text-sm font-semibold text-foreground">{step.label}</p>
                            {when && <p className="text-xs text-muted-foreground">{when}</p>}
                        </div>
                    </li>
                );
            })}
        </ol>
    );
}

function OrderDetailSkeleton() {
    return (
        <div className="mx-auto max-w-4xl space-y-6 p-4" role="status" aria-label="Loading order">
            <Skeleton className="h-8 w-64" />
            <Skeleton className="h-40 w-full rounded-xl" />
            <Skeleton className="h-64 w-full rounded-xl" />
        </div>
    );
}

export default function OrderPage() {
    const { id } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState({ order: null, loading: true, error: null });
    const [reloadToken, setReloadToken] = useState(0);
    // GET /api/orders/{id} returns bare order_id/ticket_type_id foreign keys,
    // no nested event/ticket-type — enrich client-side against the public
    // event endpoint, which also carries ticket_types (id -> name).
    const [event, setEvent] = useState(null);
    const [myTickets, setMyTickets] = useState(null);

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        ordersApi
            .get(id)
            .then((data) => {
                if (cancelled) return;
                setState({ order: data?.order ?? data, loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ order: null, loading: false, error: err.message || 'Order not found.' });
            });
        return () => {
            cancelled = true;
        };
    }, [id, reloadToken]);

    const { order, loading, error } = state;
    const items = order?.items ?? order?.order_items ?? [];

    useEffect(() => {
        if (!order?.event_id) return;
        let cancelled = false;
        eventsApi
            .get(order.event_id)
            .then((data) => {
                if (cancelled) return;
                // GET /api/events/{id} responds { event, ticket_types, issuer_keys }
                // — flatten into one object with ticket_types attached so
                // callers can read event.title and look up ticket type names
                // from the same fetch.
                const ev = data?.event ?? data;
                setEvent(ev ? { ...ev, ticket_types: data?.ticket_types ?? [] } : null);
            })
            .catch(() => {
                if (!cancelled) setEvent(null);
            });
        return () => {
            cancelled = true;
        };
    }, [order?.event_id]);

    // Issued tickets for this order — GET /api/tickets has no order filter,
    // so fetch the buyer's whole list and filter client-side by order_id.
    // Only paid orders have tickets to find; skip the request otherwise.
    useEffect(() => {
        if (!order || order.status !== 'paid') {
            setMyTickets(order && order.status === 'paid' ? undefined : []);
            return;
        }
        let cancelled = false;
        ticketsApi
            .list()
            .then((data) => {
                if (cancelled) return;
                const list = Array.isArray(data) ? data : (data?.tickets ?? []);
                setMyTickets(list.filter((t) => t.order_id === order.id));
            })
            .catch(() => {
                if (!cancelled) setMyTickets([]);
            });
        return () => {
            cancelled = true;
        };
    }, [order]);

    const ticketTypeName = (ticketTypeId) => event?.ticket_types?.find((t) => t.id === ticketTypeId)?.name ?? 'Ticket';

    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main className="pt-24">
                <div className="mx-auto max-w-4xl px-4 py-4">
                    <Button variant="ghost" onClick={() => navigate('/orders')} className="flex items-center gap-2">
                        <ChevronLeft className="h-4 w-4" aria-hidden="true" />
                        Back to orders
                    </Button>
                </div>

                {loading && <OrderDetailSkeleton />}

                {!loading && (error || !order) && (
                    <div className="mx-auto max-w-2xl p-4">
                        <ErrorState
                            title="Order not found"
                            description={error || "We couldn't find that order."}
                            onRetry={() => setReloadToken((n) => n + 1)}
                        />
                    </div>
                )}

                {!loading && !error && order && (
                    <div className="mx-auto max-w-4xl space-y-6 p-4">
                        <Card>
                            <CardHeader className="space-y-1">
                                <div className="flex flex-wrap items-center justify-between gap-2">
                                    <CardTitle className="text-2xl">Order confirmation</CardTitle>
                                    <span className={`inline-flex items-center rounded-full px-3 py-1 text-sm font-medium ${statusColor(order.status)}`}>
                                        {order.status === 'paid' && <Check className="mr-1.5 h-4 w-4" aria-hidden="true" />}
                                        {order.status === 'refunded' && <RotateCcw className="mr-1.5 h-4 w-4" aria-hidden="true" />}
                                        {order.status}
                                    </span>
                                </div>
                                <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
                                    <Hash className="h-3.5 w-3.5" aria-hidden="true" />
                                    {order.id}
                                </p>
                            </CardHeader>

                            <CardContent className="space-y-6">
                                {event && (
                                    <div className="space-y-2 rounded-lg bg-muted/50 p-4">
                                        <p className="font-display text-lg font-bold">{event.title}</p>
                                        <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
                                            {event.starts_at && (
                                                <span className="flex items-center gap-1.5">
                                                    <Calendar className="h-4 w-4" aria-hidden="true" />
                                                    {new Date(event.starts_at).toLocaleDateString(undefined, {
                                                        weekday: 'short',
                                                        month: 'short',
                                                        day: 'numeric',
                                                        year: 'numeric',
                                                    })}
                                                </span>
                                            )}
                                            {event.venue_name && (
                                                <span className="flex items-center gap-1.5">
                                                    <MapPin className="h-4 w-4" aria-hidden="true" />
                                                    {event.venue_name}
                                                </span>
                                            )}
                                        </div>
                                    </div>
                                )}

                                <div className="space-y-4">
                                    <h3 className="text-lg font-semibold">Order summary</h3>
                                    <div className="divide-y divide-border">
                                        {items.map((item) => (
                                            <div key={item.id} className="flex justify-between gap-4 py-4">
                                                <div className="min-w-0 space-y-1">
                                                    <p className="truncate font-medium">{ticketTypeName(item.ticket_type_id)}</p>
                                                    <p className="text-sm text-muted-foreground">
                                                        {item.quantity} × {formatMoney(item.unit_price_minor, order.currency)}
                                                    </p>
                                                </div>
                                                <p className="shrink-0 font-medium">
                                                    {formatMoney((item.unit_price_minor ?? 0) * item.quantity, order.currency)}
                                                </p>
                                            </div>
                                        ))}
                                    </div>

                                    <div className="space-y-2 border-t border-border pt-4">
                                        <div className="flex items-center justify-between text-sm">
                                            <span className="text-muted-foreground">Subtotal</span>
                                            <span>{formatMoney(order.subtotal_minor, order.currency)}</span>
                                        </div>
                                        {order.fee_minor > 0 && (
                                            <div className="flex items-center justify-between text-sm">
                                                <span className="text-muted-foreground">Fees</span>
                                                <span>{formatMoney(order.fee_minor, order.currency)}</span>
                                            </div>
                                        )}
                                        <div className="flex items-center justify-between border-t border-border pt-2">
                                            <span className="font-semibold">Total</span>
                                            <span className="text-lg font-semibold">{formatMoney(order.total_minor, order.currency)}</span>
                                        </div>
                                    </div>
                                </div>

                                <div className="space-y-2 rounded-lg bg-muted/50 p-4 text-sm">
                                    <p>
                                        <span className="text-muted-foreground">Buyer:</span> {order.buyer_name}
                                    </p>
                                    <p>
                                        <span className="text-muted-foreground">Email:</span> {order.buyer_email}
                                    </p>
                                    {order.provider && (
                                        <p>
                                            <span className="text-muted-foreground">Payment via:</span> {order.provider}
                                        </p>
                                    )}
                                    {order.provider_ref && (
                                        <p className="flex items-center gap-1.5">
                                            <span className="text-muted-foreground">Payment reference:</span>
                                            <span className="font-mono text-xs">{order.provider_ref}</span>
                                        </p>
                                    )}
                                </div>
                            </CardContent>
                        </Card>

                        <Card>
                            <CardHeader>
                                <CardTitle className="text-lg">Order status</CardTitle>
                            </CardHeader>
                            <CardContent>
                                <OrderTimeline order={order} />
                            </CardContent>
                        </Card>

                        {order.status === 'paid' && (
                            <Card>
                                <CardHeader className="flex flex-row items-center justify-between">
                                    <CardTitle className="text-lg">Your tickets</CardTitle>
                                    {Array.isArray(myTickets) && myTickets.length > 0 && (
                                        <Button variant="outline" size="sm" onClick={() => navigate('/tickets')}>
                                            View all tickets
                                        </Button>
                                    )}
                                </CardHeader>
                                <CardContent>
                                    {myTickets === undefined && (
                                        <div className="space-y-2">
                                            <Skeleton className="h-14 w-full" />
                                            <Skeleton className="h-14 w-full" />
                                        </div>
                                    )}
                                    {Array.isArray(myTickets) && myTickets.length === 0 && (
                                        <p className="text-sm text-muted-foreground">
                                            No tickets found for this order yet — they may still be issuing.
                                        </p>
                                    )}
                                    {Array.isArray(myTickets) && myTickets.length > 0 && (
                                        <ul className="divide-y divide-border">
                                            {myTickets.map((t) => (
                                                <li key={t.id}>
                                                    <Link
                                                        to={`/ticket/${t.id}`}
                                                        className="flex items-center justify-between gap-4 py-3 transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                                                    >
                                                        <span className="flex min-w-0 items-center gap-3">
                                                            <Ticket className="h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
                                                            <span className="min-w-0">
                                                                <span className="block truncate text-sm font-medium">
                                                                    {t.ticket_type_name || ticketTypeName(t.ticket_type_id)} — {t.holder_name}
                                                                </span>
                                                                <span className="block font-mono text-xs text-muted-foreground">#{t.serial}</span>
                                                            </span>
                                                        </span>
                                                        <span className="flex shrink-0 items-center gap-2">
                                                            {t.status && t.status !== 'valid' && (
                                                                <Badge variant="destructive" className="text-[10px]">
                                                                    {t.status}
                                                                </Badge>
                                                            )}
                                                            <ChevronRight className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                                                        </span>
                                                    </Link>
                                                </li>
                                            ))}
                                        </ul>
                                    )}
                                </CardContent>
                            </Card>
                        )}
                    </div>
                )}
            </main>
        </div>
    );
}
