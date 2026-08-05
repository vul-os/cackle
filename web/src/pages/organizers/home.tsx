import { Fragment, useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { format, formatDistanceToNowStrict } from 'date-fns';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { Skeleton, SkeletonCardGrid } from '@/components/ui/skeleton';
import { Money } from '@/components/ui/money';
import { Calendar, Plus, QrCode, Ticket, Coins, ShieldCheck, MapPin, ArrowRight, type LucideIcon } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi } from '@/lib/api';
import type { CackleEvent, EventStats } from '@/lib/api-types';

const statusVariant: Record<string, 'secondary' | 'default' | 'destructive'> = {
    draft: 'secondary',
    published: 'default',
    cancelled: 'destructive',
};

interface HomeState {
    events: CackleEvent[];
    loading: boolean;
    error: string | null;
}

// The icon and the label ride one row; the VALUE gets the card's full inner
// width on the row below.
//
// It used to sit in a column beside the icon, which cost the number ~60px of a
// ~257px tile. A multi-currency revenue rollup could not fit a single figure in
// what was left, so `break-words` broke one mid-digits — "KWD 1,437.0" on one
// line and "00" on the next, which reads as a different number, on the one
// figure in the product where a misreading matters most. Two consequences of
// that stacking are fixed here as well: the "·" separator no longer lands alone
// on a line (see RevenueValue), and the icon no longer drifts out of line with
// the other three tiles, because it now sits on a row of its own height rather
// than centred against a value 1–3 lines tall.
//
// The value's type size steps DOWN as the grid gains columns, because a tile in
// a 4-up row is far narrower than a full-width one on a phone. Measured against
// the widest real demo figure ("KWD 1,437.000", 180px at 24px / 150px at 20px)
// every band keeps at least 27px of slack, so a figure never has to break.
interface StatTileProps {
    icon: LucideIcon;
    label: string;
    value: ReactNode;
    loading: boolean;
}

const StatTile = ({ icon: Icon, label, value, loading }: StatTileProps) => (
    <Card>
        <CardContent className="flex flex-col gap-3 p-5">
            <div className="flex items-center gap-3">
                <div className="shrink-0 rounded-xl bg-primary/10 p-2.5 text-primary-emphasis">
                    <Icon className="h-5 w-5" />
                </div>
                <p className="min-w-0 text-sm text-muted-foreground">{label}</p>
            </div>
            {loading ? (
                <Skeleton className="h-7 w-24" />
            ) : (
                <p className="text-2xl font-bold leading-tight tabular-nums sm:text-xl 2xl:text-2xl">{value}</p>
            )}
        </CardContent>
    </Card>
);

// Renders a per-currency revenue rollup as real <Money> primitives rather
// than a hand-formatted string, so every figure gets the shared tabular
// treatment. Cackle has no privileged currency, so an org's events can span
// several — there is no single meaningful "total" to blend them into.
const RevenueValue = ({ revenueByCurrency }: { revenueByCurrency: Record<string, number> }) => {
    const entries = Object.entries(revenueByCurrency);
    const [firstEntry] = entries;
    if (!firstEntry) return <>—</>;
    // Only surface currencies that actually earned something. An org whose
    // events span currencies it has made no sales in would otherwise pad the
    // figure with "€0.00 · $0.00" noise. If every currency is genuinely at
    // zero, show a single zero in one real currency rather than a dash.
    const earning = entries.filter(([, minor]) => minor > 0);
    const shown = earning.length > 0 ? earning : [firstEntry];
    return (
        <>
            {shown.map(([currency, minor], i) => (
                <Fragment key={currency || i}>
                    {i > 0 ? ' ' : null}
                    {/* Each figure is one unbreakable unit, and the separator
                        is bound to the END of the figure it follows — like a
                        trailing comma. The only break opportunity left is the
                        space BETWEEN figures, so a line can end in "· " but a
                        number can never be cut in half and a lone "·" can never
                        start (or own) a line. */}
                    <span className="whitespace-nowrap">
                        <Money minor={minor} currency={currency} />
                        {i < shown.length - 1 ? ' ·' : null}
                    </span>
                </Fragment>
            ))}
        </>
    );
};

const HomePage = () => {
    const navigate = useNavigate();
    const { activeOrg, orgs } = useAuth();
    const [state, setState] = useState<HomeState>({ events: [], loading: true, error: null });
    const [statsById, setStatsById] = useState<Record<string, EventStats>>({});

    // Owner/admin can create, edit, duplicate and delete events; a scanner
    // can see everything on this page (it's the same read bar as stats/
    // attendees) but the server 403s them on every write. Gating the
    // affordance here means a scanner never sees a button that only exists
    // to fail for them.
    const canManage = activeOrg?.role === 'owner' || activeOrg?.role === 'admin';

    const loadDashboard = useCallback(() => {
        if (!activeOrg?.id) return;
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));

        eventsApi
            .listForOrg(activeOrg.id)
            .then(async (data) => {
                if (cancelled) return;
                const list = data?.events ?? [];
                setState({ events: list, loading: false, error: null });

                // Best-effort per-event stats for the dashboard totals. A single
                // event's stats failing to load shouldn't blank the whole
                // dashboard — it just doesn't contribute to the totals.
                const results = await Promise.allSettled(list.map((ev) => eventsApi.stats(ev.id)));
                if (cancelled) return;
                const next: Record<string, EventStats> = {};
                results.forEach((r, i) => {
                    const ev = list[i];
                    if (ev && r.status === 'fulfilled' && r.value?.stats) {
                        next[ev.id] = r.value.stats;
                    }
                });
                setStatsById(next);
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ events: [], loading: false, error: err instanceof Error ? err.message : 'Could not load your events.' });
            });

        return () => {
            cancelled = true;
        };
    }, [activeOrg?.id]);

    useEffect(() => loadDashboard(), [loadDashboard]);

    const eventsById = useMemo<Record<string, CackleEvent>>(
        () => Object.fromEntries(state.events.map((e) => [e.id, e])),
        [state.events],
    );

    const totals = useMemo(() => {
        const entries = Object.entries(statsById);
        const base: { sold: number; admitted: number; revenueByCurrency: Record<string, number> } = {
            sold: 0,
            admitted: 0,
            revenueByCurrency: {},
        };
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

    const nextEvent = useMemo(() => {
        const now = Date.now();
        const upcoming = state.events
            .filter((e) => e.status === 'published' && e.starts_at && new Date(e.starts_at).getTime() >= now)
            .sort((a, b) => new Date(a.starts_at).getTime() - new Date(b.starts_at).getTime());
        return upcoming[0] ?? null;
    }, [state.events]);

    const drafts = state.events.filter((e) => e.status === 'draft').length;
    const published = state.events.filter((e) => e.status === 'published').length;

    // A brand new account has no org: signup creates a user and nothing
    // else (internal/auth.Signup), deliberately, because most people
    // signing up are buying a ticket rather than selling one.
    //
    // This branch used to claim an org "usually happens automatically at
    // signup" and tell the reader to sign out and back in. That was simply
    // false — nothing anywhere created an org — so it left a new organiser
    // re-reading a dead end instead of finishing setup. Now the console's
    // entry point sends them straight to the one thing they actually need
    // to do, which is also the only route in /admin that works without an
    // org.
    if (!orgs || orgs.length === 0) {
        return <Navigate to="/admin/orgs/new" replace />;
    }

    return (
        <div className="mx-auto max-w-6xl">
            <div className="mb-8">
                <div className="mb-2 flex items-center gap-3">
                    <Ticket className="h-8 w-8 text-primary-emphasis" />
                    <h1 className="font-display text-3xl font-bold sm:text-4xl">{activeOrg?.name ?? 'Your events'}</h1>
                </div>
                <p className="text-muted-foreground">Manage events, sell tickets, and run the gate — all from here.</p>
            </div>

            {state.error && (
                <ErrorState className="mb-8" description={state.error} onRetry={loadDashboard} />
            )}

            {!state.error && (
                <>
                    {/* Stats at a glance */}
                    {/* 4-up starts at xl, not lg: with the console's fixed
                        sidebar, a 1024px window leaves the content column
                        ~660px, and a quarter of that gave the Revenue tile a
                        58px value column — narrower than any money figure at
                        any legible size. */}
                    <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
                        <StatTile icon={Calendar} label="Published events" value={published} loading={state.loading} />
                        <StatTile icon={Ticket} label="Tickets sold" value={totals.sold} loading={state.loading} />
                        <StatTile
                            icon={Coins}
                            label="Revenue"
                            value={<RevenueValue revenueByCurrency={totals.revenueByCurrency} />}
                            loading={state.loading}
                        />
                        <StatTile icon={ShieldCheck} label="Admitted" value={totals.admitted} loading={state.loading} />
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
                                    <div role="status" aria-label="Loading" className="flex items-center justify-between gap-4">
                                        <div className="min-w-0 flex-1 space-y-2">
                                            <Skeleton className="h-5 w-1/2" />
                                            <Skeleton className="h-4 w-2/3" />
                                        </div>
                                        <Skeleton className="h-11 w-24 shrink-0" />
                                    </div>
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
                                            {(() => {
                                                const stats = statsById[nextEvent.id];
                                                return (
                                                    stats && (
                                                        <p className="mt-2 text-sm text-muted-foreground">
                                                            {stats.sold ?? 0} sold ·{' '}
                                                            <Money minor={stats.revenue_minor} currency={nextEvent.currency} /> revenue
                                                        </p>
                                                    )
                                                );
                                            })()}
                                        </div>
                                        <div className="flex shrink-0 gap-2">
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                className="min-h-11"
                                                onClick={() => navigate(`/admin/events/${nextEvent.id}/stats`)}
                                            >
                                                Stats
                                            </Button>
                                            <Button size="sm" className="min-h-11" onClick={() => navigate(`/admin/events/${nextEvent.id}`)}>
                                                {canManage ? 'Manage' : 'View'}
                                                <ArrowRight className="ml-2 h-4 w-4" />
                                            </Button>
                                        </div>
                                    </div>
                                ) : (
                                    <EmptyState
                                        icon={Calendar}
                                        title="Nothing on the calendar"
                                        description={
                                            canManage
                                                ? 'Publish an event to see it here.'
                                                : 'Nothing published yet. Ask an owner or admin to publish one.'
                                        }
                                        action={
                                            canManage && (
                                                <Button size="sm" className="min-h-11" onClick={() => navigate('/admin/events/new')}>
                                                    <Plus className="mr-2 h-4 w-4" />
                                                    Create event
                                                </Button>
                                            )
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
                                {canManage && (
                                    <Button variant="outline" className="min-h-11 w-full" onClick={() => navigate('/admin/events/new')}>
                                        <Plus className="mr-2 h-4 w-4" />
                                        New event
                                    </Button>
                                )}
                                <Button className="min-h-11 w-full" onClick={() => navigate('/admin/scanner')}>
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
                            <SkeletonCardGrid count={3} />
                        ) : state.events.length === 0 ? (
                            <EmptyState
                                icon={Calendar}
                                title="No events yet"
                                description={
                                    canManage
                                        ? 'Create your first event to start selling tickets.'
                                        : 'Ask an owner or admin to create one to get started.'
                                }
                                action={
                                    canManage && (
                                        <Button size="sm" className="min-h-11" onClick={() => navigate('/admin/events/new')}>
                                            <Plus className="mr-2 h-4 w-4" />
                                            Create Event
                                        </Button>
                                    )
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
                                                    {s ? (
                                                        <>
                                                            {s.sold ?? 0} sold · <Money minor={s.revenue_minor} currency={event.currency} />
                                                        </>
                                                    ) : (
                                                        '—'
                                                    )}
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
