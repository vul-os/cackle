import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft, Ticket, Coins, ShieldCheck, BarChart3, Gauge, type LucideIcon } from 'lucide-react';
import { events as eventsApi } from '@/lib/api';
import { Money } from '@/components/ui/money';
import { TicketTypeBreakdown, RatioMeter } from './stats-charts';
import type { EventStats } from '@/lib/api-types';

// Icon + label on one row, the value on the card's full inner width below it.
//
// Beside the icon the value column was 39px wide at 768, and `break-words` duly
// broke the event's revenue into "R 1" / "0,800.00" — a figure that reads as a
// different number. Money may only ever break at a separator, never inside a
// figure, so the fix is to give the figure room (full card width, and a 3-up
// row only once the content column is wide enough for one) rather than to let
// it break.
interface StatTileProps {
    icon: LucideIcon;
    label: string;
    value: React.ReactNode;
    sub?: React.ReactNode;
}

const StatTile = ({ icon: Icon, label, value, sub }: StatTileProps) => (
    <Card>
        <CardContent className="flex flex-col gap-3 p-6">
            <div className="flex items-center gap-3">
                <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary-emphasis">
                    <Icon className="h-5 w-5" aria-hidden="true" />
                </div>
                <p className="min-w-0 text-sm text-muted-foreground">{label}</p>
            </div>
            <div className="min-w-0">
                {/* Plain counts (Sold, Admitted) render in the font's default
                    proportional figures — a lone hero number needs no column
                    to align against (dataviz: tabular-nums is for columns).
                    Revenue is the one exception: it's a <Money> element, which
                    always carries its own `tnum` — that's money.tsx's own
                    call (a currency amount that could tick upward while read
                    must not visibly reflow), not a choice made here. */}
                <p className="text-2xl font-bold leading-tight md:text-xl xl:text-2xl">{value}</p>
                {sub && <p className="text-xs text-muted-foreground">{sub}</p>}
            </div>
        </CardContent>
    </Card>
);

interface EventStatsPageState {
    stats: EventStats | null;
    currency: string;
    eventTitle: string;
    loading: boolean;
    error: string | null;
}

const EventStatsPage = () => {
    const { id } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState<EventStatsPageState>({ stats: null, currency: '', eventTitle: '', loading: true, error: null });

    const fetchStats = useCallback(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        Promise.all([eventsApi.stats(id ?? ''), eventsApi.get(id ?? '')])
            .then(([statsData, eventData]) => {
                if (cancelled) return;
                setState({
                    stats: statsData.stats,
                    currency: eventData.event.currency || '',
                    eventTitle: eventData.event.title || '',
                    loading: false,
                    error: null,
                });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ stats: null, currency: '', eventTitle: '', loading: false, error: err?.message || 'Could not load stats.' });
            });
        return () => {
            cancelled = true;
        };
    }, [id]);

    useEffect(() => fetchStats(), [fetchStats]);

    const { stats, currency, eventTitle, loading, error } = state;
    const byType = stats?.by_type ?? [];

    const totalCapacity = useMemo(
        () => (stats?.by_type ?? []).reduce((sum, t) => sum + (t.quantity_total ?? 0), 0),
        [stats],
    );

    const admissionRate = useMemo(() => {
        if (!stats || !stats.sold) return null;
        return Math.round(((stats.admitted ?? 0) / stats.sold) * 100);
    }, [stats]);

    return (
        <div className="mx-auto max-w-4xl">
            {/* h-11 below `sm` keeps this a real 44px touch target on a phone;
                sm:h-9 returns it to the compact ghost-button height once
                there's a pointer, not a thumb, doing the tapping. */}
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${id}`)} className="mb-6 h-11 sm:h-9">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <div className="mb-8 flex items-center gap-3">
                <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 text-primary-emphasis">
                    <Gauge className="h-6 w-6" aria-hidden="true" />
                </div>
                <div className="min-w-0">
                    <h1 className="font-display text-display-sm font-bold leading-tight">Analytics</h1>
                    {eventTitle && <p className="truncate text-sm text-muted-foreground">{eventTitle}</p>}
                </div>
            </div>

            {loading && (
                <div className="space-y-6" role="status" aria-label="Loading stats">
                    {/* same breakpoints as the loaded grid, so the skeleton does
                        not reflow into a different shape the moment data lands */}
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                        {[0, 1, 2].map((i) => (
                            <Skeleton key={i} className="h-24" />
                        ))}
                    </div>
                    <Skeleton className="h-56" />
                    <Skeleton className="h-40" />
                </div>
            )}

            {!loading && error && <ErrorState description={error} onRetry={fetchStats} />}

            {!loading && !error && stats && (
                <div className="space-y-6">
                    {/* 3-up only from xl. With the console sidebar in place a
                        768px window leaves ~470px of content and a 1280px one
                        ~896px (the max-w-4xl cap); a third of the former cannot
                        hold a revenue figure at any legible size, so the row
                        goes 1-up / 2-up / 3-up as the column earns it. */}
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                        <StatTile icon={Ticket} label="Sold" value={stats.sold ?? 0} sub={totalCapacity ? `of ${totalCapacity} capacity` : undefined} />
                        <StatTile icon={Coins} label="Revenue" value={<Money minor={stats.revenue_minor ?? 0} currency={currency} />} />
                        <StatTile
                            icon={ShieldCheck}
                            label="Admitted"
                            value={stats.admitted ?? 0}
                            sub={admissionRate !== null ? `${admissionRate}% of sold` : 'No sales yet'}
                        />
                    </div>

                    <Card>
                        <CardHeader>
                            <CardTitle className="flex items-center gap-2 text-base">
                                <BarChart3 className="h-4 w-4" aria-hidden="true" />
                                Tickets by type
                            </CardTitle>
                            <CardDescription>Sold vs. capacity and revenue, per ticket type.</CardDescription>
                        </CardHeader>
                        <CardContent>
                            {byType.length === 0 ? (
                                <EmptyState
                                    icon={Ticket}
                                    title="No ticket types yet"
                                    description="Add a ticket type to this event to see sales broken down here."
                                    className="border-none bg-transparent p-0 py-6"
                                />
                            ) : (
                                <TicketTypeBreakdown byType={byType} currency={currency} />
                            )}
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader>
                            <CardTitle className="flex items-center gap-2 text-base">
                                <Gauge className="h-4 w-4" aria-hidden="true" />
                                Sales &amp; admission
                            </CardTitle>
                            <CardDescription>How full the event is, and how many sold tickets have actually walked through the gate.</CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-5">
                            <RatioMeter
                                label="Capacity filled"
                                value={stats.sold ?? 0}
                                of={totalCapacity}
                                emptyLabel="No ticket types with a capacity set yet."
                            />
                            <RatioMeter
                                label="Admitted at the gate"
                                value={stats.admitted ?? 0}
                                of={stats.sold ?? 0}
                                emptyLabel="No tickets sold yet — nothing to admit."
                            />
                        </CardContent>
                    </Card>
                </div>
            )}
        </div>
    );
};

export default EventStatsPage;
