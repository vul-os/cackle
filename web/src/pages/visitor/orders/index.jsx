import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Header from '@/pages/visitor/header';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonList } from '@/components/ui/skeleton';
import { ChevronRight, Ticket, Clock, CheckCircle2, XCircle, Layout } from 'lucide-react';
import { orders as ordersApi, events as eventsApi } from '@/lib/api';
import { formatMoney } from '@/lib/money';

const STATUS_STYLE = {
    pending: { className: 'bg-warning/15 text-warning-foreground', icon: Clock },
    paid: { className: 'bg-success/15 text-success', icon: CheckCircle2 },
    failed: { className: 'bg-destructive/15 text-destructive', icon: XCircle },
    refunded: { className: 'bg-muted text-muted-foreground', icon: XCircle },
    cancelled: { className: 'bg-muted text-muted-foreground', icon: XCircle },
};

function formatDate(iso) {
    if (!iso) return '—';
    try {
        return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' });
    } catch {
        return '—';
    }
}

export default function OrdersPage() {
    const navigate = useNavigate();
    const [state, setState] = useState({ orders: [], loading: true, error: null });
    const [reloadToken, setReloadToken] = useState(0);
    // GET /api/orders returns bare orders (event_id, no nested event) — the
    // event title/date shown per row is resolved client-side against the
    // public event-detail endpoint, best-effort, keyed by event_id.
    const [eventsById, setEventsById] = useState({});

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        ordersApi
            .list()
            .then((data) => {
                if (cancelled) return;
                const list = Array.isArray(data) ? data : (data?.orders ?? []);
                setState({ orders: list, loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ orders: [], loading: false, error: err.message || 'Could not load your orders.' });
            });
        return () => {
            cancelled = true;
        };
    }, [reloadToken]);

    useEffect(() => {
        const ids = [...new Set(state.orders.map((o) => o.event_id).filter(Boolean))];
        const missing = ids.filter((id) => !(id in eventsById));
        if (missing.length === 0) return;
        let cancelled = false;
        Promise.allSettled(missing.map((id) => eventsApi.get(id))).then((results) => {
            if (cancelled) return;
            setEventsById((prev) => {
                const next = { ...prev };
                results.forEach((res, i) => {
                    next[missing[i]] = res.status === 'fulfilled' ? (res.value?.event ?? res.value) : null;
                });
                return next;
            });
        });
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [state.orders]);

    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main className="container mx-auto max-w-5xl px-4 pb-16 pt-24">
                <div className="mb-8 flex items-center justify-between">
                    <div>
                        <h1 className="font-display text-3xl font-bold">Your orders</h1>
                        <p className="mt-1 text-muted-foreground">Every order you&apos;ve placed on Cackle.</p>
                    </div>
                    <Button variant="outline" onClick={() => navigate('/tickets')}>
                        <Layout className="mr-2 h-4 w-4" aria-hidden="true" />
                        My tickets
                    </Button>
                </div>

                {state.loading && <SkeletonList rows={3} />}

                {!state.loading && state.error && (
                    <ErrorState description={state.error} onRetry={() => setReloadToken((n) => n + 1)} />
                )}

                {!state.loading && !state.error && state.orders.length === 0 && (
                    <EmptyState
                        icon={Ticket}
                        title="No orders yet"
                        description="Once you buy tickets, they'll show up here."
                        action={<Button onClick={() => navigate('/')}>Browse events</Button>}
                    />
                )}

                {!state.loading && !state.error && state.orders.length > 0 && (
                    <div className="space-y-3">
                        {state.orders.map((order) => {
                            const style = STATUS_STYLE[order.status] ?? STATUS_STYLE.pending;
                            const StatusIcon = style.icon;
                            const event = eventsById[order.event_id];
                            const title = event?.title ?? `Order ${order.id.slice(0, 8)}`;
                            return (
                                <Card
                                    key={order.id}
                                    className="cursor-pointer transition-colors hover:border-primary/40"
                                    role="link"
                                    tabIndex={0}
                                    onClick={() => navigate(`/order/${order.id}`)}
                                    onKeyDown={(e) => {
                                        if (e.key === 'Enter' || e.key === ' ') navigate(`/order/${order.id}`);
                                    }}
                                >
                                    <CardContent className="flex items-center justify-between gap-4 p-5">
                                        <div className="min-w-0">
                                            <p className="truncate font-medium">{title}</p>
                                            <p className="text-sm text-muted-foreground">
                                                {event?.venue_name ? `${event.venue_name} · ` : ''}
                                                {formatDate(order.created_at)}
                                            </p>
                                        </div>
                                        <div className="flex shrink-0 items-center gap-4">
                                            <span
                                                className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${style.className}`}
                                            >
                                                <StatusIcon className="h-3 w-3" aria-hidden="true" />
                                                {order.status}
                                            </span>
                                            <span className="font-medium">{formatMoney(order.total_minor, order.currency)}</span>
                                            <ChevronRight className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                                        </div>
                                    </CardContent>
                                </Card>
                            );
                        })}
                    </div>
                )}
            </main>
        </div>
    );
}
