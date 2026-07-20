import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { format, formatDistanceToNowStrict } from 'date-fns';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { Calendar, Plus, QrCode, Ticket, Building2, Coins, ShieldCheck, MapPin, ArrowRight } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi } from '@/lib/api';
import { formatMoney } from '@/lib/money';

const statusVariant = {
    draft: 'secondary',
    published: 'default',
    cancelled: 'destructive',
};

const StatTile = ({ icon: Icon, label, value }) => (
    <Card>
        <CardContent className="flex items-center gap-4 p-5">
            <div className="rounded-xl bg-primary/10 p-3 text-primary">
                <Icon className="h-5 w-5" />
            </div>
            <div className="min-w-0">
                <p className="text-sm text-muted-foreground">{label}</p>
                {/* break-words (not truncate): revenue can be a per-currency
                    breakdown ("R 3,894.00 · ¥13,500 · KWD 98.250 · ...") when
                    an org's events span multiple currencies — wrapping beats
                    silently cutting a real figure off with an ellipsis. */}
                <p className="break-words text-2xl font-bold leading-tight tabular-nums">{value}</p>
            </div>
        </CardContent>
    </Card>
);

const HomePage = () => {
    const navigate = useNavigate();
    const { activeOrg, orgs } = useAuth();
    const [state, setState] = useState({ events: [], loading: true, error: null });
    const [statsById, setStatsById] = useState({});

    useEffect(() => {
        if (!activeOrg?.id) return;
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));

        eventsApi
            .listForOrg(activeOrg.id)
            .then(async (data) => {
                if (cancelled) return;
                const list = Array.isArray(data) ? data : (data?.events ?? []);
                setState({ events: list, loading: false, error: null });

                // Best-effort per-event stats for the dashboard totals. A single
                // event's stats failing to load shouldn't blank the whole
                // dashboard — it just doesn't contribute to the totals.
                const results = await Promise.allSettled(list.map((ev) => eventsApi.stats(ev.id)));
                if (cancelled) return;
                const next = {};
                results.forEach((r, i) => {
                    if (r.status === 'fulfilled') {
                        next[list[i].id] = r.value?.stats ?? r.value;
                    }
                });
                setStatsById(next);
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ events: [], loading: false, error: err.message || 'Could not load your events.' });
            });

        return () => {
            cancelled = true;
        };
    }, [activeOrg?.id]);

    const eventsById = useMemo(() => Object.fromEntries(state.events.map((e) => [e.id, e])), [state.events]);

    const totals = useMemo(() => {
        const entries = Object.entries(statsById);
        const base = { sold: 0, admitted: 0, revenueByCurrency: {} };
        return entries.reduce((acc, [eventId, s]) => {
            const currency = eventsById[eventId]?.currency || '';
            return {
                sold: acc.sold + (s?.sold ?? 0),
                admitted: acc.admitted + (s?.admitted ?? 0),
                revenueByCurrency: {
                    ...acc.revenueByCurrency,
                    [currency]: (acc.revenueByCurrency[currency] ?? 0) + (s?.revenue_minor ?? 0),
                },
            };
        }, base);
    }, [statsById, eventsById]);

    // An organiser's events can span multiple currencies (Cackle has no
    // privileged currency) — there is no single meaningful "total revenue"
    // number to blend them into, so render one figure per currency
    // instead of silently adding a JPY total to a ZAR total.
    const revenueDisplay = useMemo(() => {
        const entries = Object.entries(totals.revenueByCurrency);
        if (entries.length === 0) return '—';
        return entries.map(([currency, minor]) => formatMoney(minor, currency)).join(' · ');
    }, [totals]);

    const nextEvent = useMemo(() => {
        const now = Date.now();
        const upcoming = state.events
            .filter((e) => e.status === 'published' && e.starts_at && new Date(e.starts_at).getTime() >= now)
            .sort((a, b) => new Date(a.starts_at) - new Date(b.starts_at));
        return upcoming[0] ?? null;
    }, [state.events]);

    const drafts = state.events.filter((e) => e.status === 'draft').length;
    const published = state.events.filter((e) => e.status === 'published').length;

    if (!orgs || orgs.length === 0) {
        return (
            <div className="mx-auto flex max-w-lg flex-col items-center gap-4 py-24 text-center">
                <Building2 className="h-12 w-12 text-muted-foreground" />
                <h1 className="font-display text-2xl font-bold">No organization yet</h1>
                <p className="text-muted-foreground">
                    Your account isn&apos;t attached to an organizer profile yet. This usually happens automatically at signup — try
                    signing out and back in, or contact support if it persists.
                </p>
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-6xl">
            <div className="mb-8">
                <div className="mb-2 flex items-center gap-3">
                    <Ticket className="h-8 w-8 text-primary" />
                    <h1 className="font-display text-3xl font-bold sm:text-4xl">{activeOrg?.name ?? 'Your events'}</h1>
                </div>
                <p className="text-muted-foreground">Manage events, sell tickets, and run the gate — all from here.</p>
            </div>

            {state.error && (
                <ErrorState className="mb-8" description={state.error} onRetry={() => window.location.reload()} />
            )}

            {!state.error && (
                <>
                    {/* Stats at a glance */}
                    <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
                        <StatTile icon={Calendar} label="Published events" value={state.loading ? '—' : published} />
                        <StatTile icon={Ticket} label="Tickets sold" value={state.loading ? '—' : totals.sold} />
                        <StatTile icon={Coins} label="Revenue" value={state.loading ? '—' : revenueDisplay} />
                        <StatTile icon={ShieldCheck} label="Admitted" value={state.loading ? '—' : totals.admitted} />
                    </div>

                    <div className="mb-6 grid grid-cols-1 gap-6 lg:grid-cols-3">
                        {/* Next event */}
                        <Card className="lg:col-span-2">
                            <CardHeader>
                                <CardTitle>Next up</CardTitle>
                                <CardDescription>Your soonest published event.</CardDescription>
                            </CardHeader>
                            <CardContent>
                                {state.loading ? (
                                    <div className="h-24 animate-pulse rounded-xl bg-muted" />
                                ) : nextEvent ? (
                                    <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
                                        <div className="min-w-0">
                                            <div className="flex items-center gap-2">
                                                <p className="truncate text-lg font-semibold">{nextEvent.title}</p>
                                                <Badge variant={statusVariant[nextEvent.status] ?? 'secondary'}>{nextEvent.status}</Badge>
                                            </div>
                                            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
                                                <span className="flex items-center gap-1.5">
                                                    <Calendar className="h-3.5 w-3.5" />
                                                    {format(new Date(nextEvent.starts_at), 'PPP')} · in{' '}
                                                    {formatDistanceToNowStrict(new Date(nextEvent.starts_at))}
                                                </span>
                                                {nextEvent.venue_name && (
                                                    <span className="flex items-center gap-1.5">
                                                        <MapPin className="h-3.5 w-3.5" />
                                                        {nextEvent.venue_name}
                                                    </span>
                                                )}
                                            </div>
                                            {statsById[nextEvent.id] && (
                                                <p className="mt-2 text-sm text-muted-foreground">
                                                    {statsById[nextEvent.id].sold ?? 0} sold ·{' '}
                                                    {formatMoney(statsById[nextEvent.id].revenue_minor, nextEvent.currency)} revenue
                                                </p>
                                            )}
                                        </div>
                                        <div className="flex shrink-0 gap-2">
                                            <Button variant="outline" size="sm" onClick={() => navigate(`/admin/events/${nextEvent.id}/stats`)}>
                                                Stats
                                            </Button>
                                            <Button size="sm" onClick={() => navigate(`/admin/events/${nextEvent.id}`)}>
                                                Manage
                                                <ArrowRight className="ml-2 h-4 w-4" />
                                            </Button>
                                        </div>
                                    </div>
                                ) : (
                                    <EmptyState
                                        icon={Calendar}
                                        title="Nothing on the calendar"
                                        description="Publish an event to see it here."
                                        action={
                                            <Button size="sm" onClick={() => navigate('/admin/events')}>
                                                <Plus className="mr-2 h-4 w-4" />
                                                Go to Events
                                            </Button>
                                        }
                                    />
                                )}
                            </CardContent>
                        </Card>

                        {/* Quick actions */}
                        <Card className="flex flex-col">
                            <CardHeader>
                                <CardTitle className="flex items-center gap-2 text-base">
                                    <QrCode className="h-4 w-4" />
                                    Scan the gate
                                </CardTitle>
                                <CardDescription>Works fully offline.</CardDescription>
                            </CardHeader>
                            <CardContent className="flex-1">
                                <p className="text-sm text-muted-foreground">
                                    Download the scan bundle once while online, then admit guests with no signal.
                                </p>
                            </CardContent>
                            <CardFooter className="gap-2">
                                <Button variant="outline" className="w-full" onClick={() => navigate('/admin/events')}>
                                    <Plus className="mr-2 h-4 w-4" />
                                    New event
                                </Button>
                                <Button className="w-full" onClick={() => navigate('/admin/scanner')}>
                                    <QrCode className="mr-2 h-4 w-4" />
                                    Scanner
                                </Button>
                            </CardFooter>
                        </Card>
                    </div>

                    {/* All events */}
                    <div>
                        <div className="mb-4 flex items-center justify-between">
                            <h2 className="text-xl font-semibold">Your events</h2>
                            {!state.loading && state.events.length > 0 && (
                                <p className="text-sm text-muted-foreground">
                                    {published} published{drafts > 0 ? ` · ${drafts} draft${drafts === 1 ? '' : 's'}` : ''}
                                </p>
                            )}
                        </div>
                        {state.loading ? (
                            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                                {[0, 1, 2].map((i) => (
                                    <div key={i} className="h-28 animate-pulse rounded-xl bg-muted" />
                                ))}
                            </div>
                        ) : state.events.length === 0 ? (
                            <EmptyState
                                icon={Calendar}
                                title="No events yet"
                                description="Create your first event to start selling tickets."
                                action={
                                    <Button size="sm" onClick={() => navigate('/admin/events')}>
                                        <Plus className="mr-2 h-4 w-4" />
                                        Create Event
                                    </Button>
                                }
                            />
                        ) : (
                            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                                {state.events.map((event) => {
                                    const s = statsById[event.id];
                                    return (
                                        <Card key={event.id} className="cursor-pointer hover:shadow-md" onClick={() => navigate(`/admin/events/${event.id}`)}>
                                            <CardContent className="p-5">
                                                <div className="flex items-center gap-2">
                                                    <p className="truncate font-medium">{event.title}</p>
                                                    <Badge variant={statusVariant[event.status] ?? 'secondary'} className="shrink-0">
                                                        {event.status}
                                                    </Badge>
                                                </div>
                                                {event.venue_name && <p className="mt-1 text-sm text-muted-foreground">{event.venue_name}</p>}
                                                <p className="mt-2 text-sm text-muted-foreground">
                                                    {s ? `${s.sold ?? 0} sold · ${formatMoney(s.revenue_minor, event.currency)}` : '—'}
                                                </p>
                                            </CardContent>
                                        </Card>
                                    );
                                })}
                            </div>
                        )}
                    </div>
                </>
            )}
        </div>
    );
};

export default HomePage;
