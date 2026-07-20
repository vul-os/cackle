// Public event browse — the storefront's main listing surface. Search and
// filter published events, anonymously, with real availability/price info
// pulled in per-card from the public event-detail endpoint (the list
// endpoint itself carries no pricing — see BUILD-SPEC.md).
import React, { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Calendar, MapPin, Search, AlertCircle, CalendarX2, X } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import { events as eventsApi } from '@/lib/api';

const PAGE_SIZE = 24;

const DATE_FILTERS = {
    any: { label: 'Any time' },
    today: { label: 'Today' },
    week: { label: 'This week' },
    month: { label: 'This month' },
};

function dateRangeFor(key) {
    const now = new Date();
    const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    if (key === 'today') {
        const end = new Date(startOfToday);
        end.setDate(end.getDate() + 1);
        return { from: startOfToday, to: end };
    }
    if (key === 'week') {
        const end = new Date(startOfToday);
        end.setDate(end.getDate() + 7);
        return { from: startOfToday, to: end };
    }
    if (key === 'month') {
        const end = new Date(startOfToday);
        end.setMonth(end.getMonth() + 1);
        return { from: startOfToday, to: end };
    }
    return { from: null, to: null };
}

const PRICE_FILTERS = {
    any: { label: 'Any price', test: () => true },
    free: { label: 'Free', test: (c) => c === 0 },
    under250: { label: 'Under R250', test: (c) => c > 0 && c < 25000 },
    under750: { label: 'R250 – R750', test: (c) => c >= 25000 && c < 75000 },
    over750: { label: 'R750+', test: (c) => c >= 75000 },
};

function formatMoney(cents, currency = 'ZAR') {
    if (cents === undefined || cents === null) return '';
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(cents / 100);
    } catch {
        return `R ${(cents / 100).toFixed(2)}`;
    }
}

function formatDate(iso) {
    if (!iso) return 'Date TBA';
    try {
        return new Date(iso).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
        return 'Date TBA';
    }
}

// Best-effort per-event pricing/availability, resolved against the public
// GET /api/events/{slug} endpoint (the only public route that carries
// ticket_types). A failure here degrades to "no price shown" for that one
// card rather than failing the whole page.
function usePricing(events) {
    const [byId, setById] = useState({});

    useEffect(() => {
        const ids = events.map((e) => e.slug || e.id).filter((ref) => ref && !(ref in byId));
        if (ids.length === 0) return;
        let cancelled = false;
        Promise.allSettled(ids.map((ref) => eventsApi.get(ref))).then((results) => {
            if (cancelled) return;
            setById((prev) => {
                const next = { ...prev };
                results.forEach((res, i) => {
                    if (res.status !== 'fulfilled') {
                        next[ids[i]] = null;
                        return;
                    }
                    const types = res.value?.ticket_types ?? [];
                    if (types.length === 0) {
                        next[ids[i]] = { minPriceCents: null, soldOut: false };
                        return;
                    }
                    const remaining = (t) => Math.max(0, (t.quantity_total ?? 0) - (t.quantity_sold ?? 0));
                    const available = types.filter((t) => t.status !== 'hidden');
                    const soldOut = available.length > 0 && available.every((t) => remaining(t) <= 0);
                    const minPriceCents = available.reduce(
                        (min, t) => (min === null || t.price_cents < min ? t.price_cents : min),
                        null,
                    );
                    next[ids[i]] = { minPriceCents, soldOut };
                });
                return next;
            });
        });
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [events]);

    return byId;
}

const EventCard = ({ event, pricing, index }) => {
    const price = pricing?.minPriceCents;
    return (
        <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.3, delay: Math.min(index * 0.04, 0.4) }}
        >
            <Link to={`/events/${event.slug}`} className="group block h-full" data-testid="event-card">
                <Card className="h-full overflow-hidden transition-all duration-300 hover:-translate-y-1 hover:shadow-xl">
                    <div className="relative aspect-[16/9] overflow-hidden bg-muted">
                        {event.cover_image ? (
                            <img
                                src={event.cover_image}
                                alt={event.title}
                                className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
                                loading="lazy"
                            />
                        ) : (
                            <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-primary/20 to-primary/5">
                                <Calendar className="h-10 w-10 text-primary/50" />
                            </div>
                        )}
                        {pricing?.soldOut && (
                            <div className="absolute inset-0 flex items-center justify-center bg-black/50">
                                <span className="rounded-full bg-background px-4 py-1.5 text-sm font-semibold">Sold out</span>
                            </div>
                        )}
                        {!pricing?.soldOut && price !== undefined && price !== null && (
                            <div className="absolute right-3 top-3 rounded-full bg-primary px-3 py-1 text-xs font-semibold text-primary-foreground shadow">
                                {price === 0 ? 'Free' : `From ${formatMoney(price, event.currency)}`}
                            </div>
                        )}
                    </div>
                    <CardContent className="space-y-2 p-5">
                        <h3 className="font-display text-lg font-bold leading-snug tracking-tight group-hover:text-primary">{event.title}</h3>
                        <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Calendar className="h-4 w-4 shrink-0" />
                            <span>{formatDate(event.starts_at)}</span>
                        </div>
                        {event.venue_name && (
                            <div className="flex items-center gap-2 text-sm text-muted-foreground">
                                <MapPin className="h-4 w-4 shrink-0" />
                                <span className="truncate">{event.venue_name}</span>
                            </div>
                        )}
                    </CardContent>
                </Card>
            </Link>
        </motion.div>
    );
};

const CardSkeleton = () => (
    <div className="overflow-hidden rounded-xl border border-border">
        <div className="aspect-[16/9] animate-pulse bg-muted" />
        <div className="space-y-2 p-5">
            <div className="h-5 w-3/4 animate-pulse rounded bg-muted" />
            <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
            <div className="h-4 w-2/3 animate-pulse rounded bg-muted" />
        </div>
    </div>
);

export default function BrowsePage() {
    const [searchParams, setSearchParams] = useSearchParams();
    const query = searchParams.get('q') || '';
    const [searchValue, setSearchValue] = useState(query);
    const [dateFilter, setDateFilter] = useState('any');
    const [priceFilter, setPriceFilter] = useState('any');
    const [limit, setLimit] = useState(PAGE_SIZE);
    const [reloadToken, setReloadToken] = useState(0);

    const [state, setState] = useState({ events: [], loading: true, error: null });

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        const { from, to } = dateRangeFor(dateFilter);
        eventsApi
            .list({
                q: query || undefined,
                from: from ? from.toISOString() : undefined,
                to: to ? to.toISOString() : undefined,
                limit,
            })
            .then((data) => {
                if (cancelled) return;
                setState({ events: Array.isArray(data) ? data : (data?.events ?? []), loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ events: [], loading: false, error: err.message || 'Could not load events.' });
            });
        return () => {
            cancelled = true;
        };
    }, [query, dateFilter, limit, reloadToken]);

    const pricing = usePricing(state.events);

    const filteredEvents = useMemo(() => {
        if (priceFilter === 'any') return state.events;
        const test = PRICE_FILTERS[priceFilter].test;
        return state.events.filter((e) => {
            const p = pricing[e.slug || e.id];
            // While pricing is still loading for a card, keep it visible
            // rather than flashing it out then back in.
            if (!p || p.minPriceCents === null || p.minPriceCents === undefined) return true;
            return test(p.minPriceCents);
        });
    }, [state.events, pricing, priceFilter]);

    const handleSearchSubmit = (e) => {
        e.preventDefault();
        const params = {};
        if (searchValue) params.q = searchValue;
        setSearchParams(params);
        setLimit(PAGE_SIZE);
    };

    const hasActiveFilters = Boolean(query) || dateFilter !== 'any' || priceFilter !== 'any';
    const clearFilters = () => {
        setSearchValue('');
        setSearchParams({});
        setDateFilter('any');
        setPriceFilter('any');
        setLimit(PAGE_SIZE);
    };

    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main className="pt-16">
                <div className="border-b border-border bg-muted/30">
                    <div className="container mx-auto px-4 py-10">
                        <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">Browse events</h1>
                        <p className="mt-2 text-muted-foreground">Search live events across South Africa and grab your tickets.</p>

                        <form onSubmit={handleSearchSubmit} className="mt-6 flex flex-col gap-3 sm:flex-row" role="search">
                            <div className="relative flex-1">
                                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                                <label htmlFor="browse-search" className="sr-only">
                                    Search events, venues, or organisers
                                </label>
                                <Input
                                    id="browse-search"
                                    value={searchValue}
                                    onChange={(e) => setSearchValue(e.target.value)}
                                    placeholder="Search events, venues, or organisers"
                                    className="pl-10"
                                />
                            </div>

                            <div className="flex gap-3">
                                <Select
                                    value={dateFilter}
                                    onValueChange={(v) => {
                                        setDateFilter(v);
                                        setLimit(PAGE_SIZE);
                                    }}
                                >
                                    <SelectTrigger className="w-[150px]" aria-label="Filter by date">
                                        <SelectValue placeholder="Any time" />
                                    </SelectTrigger>
                                    <SelectContent>
                                        {Object.entries(DATE_FILTERS).map(([key, { label }]) => (
                                            <SelectItem key={key} value={key}>
                                                {label}
                                            </SelectItem>
                                        ))}
                                    </SelectContent>
                                </Select>

                                <Select value={priceFilter} onValueChange={setPriceFilter}>
                                    <SelectTrigger className="w-[150px]" aria-label="Filter by price">
                                        <SelectValue placeholder="Any price" />
                                    </SelectTrigger>
                                    <SelectContent>
                                        {Object.entries(PRICE_FILTERS).map(([key, { label }]) => (
                                            <SelectItem key={key} value={key}>
                                                {label}
                                            </SelectItem>
                                        ))}
                                    </SelectContent>
                                </Select>

                                <Button type="submit">Search</Button>
                            </div>
                        </form>

                        {hasActiveFilters && (
                            <button
                                type="button"
                                onClick={clearFilters}
                                className="mt-3 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
                            >
                                <X className="h-3.5 w-3.5" />
                                Clear filters
                            </button>
                        )}
                    </div>
                </div>

                <section className="container mx-auto px-4 py-10" data-surface="event-browse">
                    {state.error && (
                        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-20 text-center">
                            <AlertCircle className="h-10 w-10 text-destructive" />
                            <p className="font-medium">Couldn&apos;t load events</p>
                            <p className="max-w-sm text-sm text-muted-foreground">{state.error}</p>
                            <Button variant="outline" onClick={() => setReloadToken((t) => t + 1)}>
                                Try again
                            </Button>
                        </div>
                    )}

                    {!state.error && state.loading && (
                        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                            {Array.from({ length: 8 }).map((_, i) => (
                                <CardSkeleton key={i} />
                            ))}
                        </div>
                    )}

                    {!state.error && !state.loading && state.events.length === 0 && (
                        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-20 text-center">
                            <CalendarX2 className="h-10 w-10 text-muted-foreground" />
                            <p className="font-medium">No events found</p>
                            <p className="max-w-sm text-sm text-muted-foreground">
                                {hasActiveFilters
                                    ? 'Try a different search or loosen your filters.'
                                    : 'Check back soon — new events are added all the time.'}
                            </p>
                            {hasActiveFilters && (
                                <Button variant="outline" onClick={clearFilters}>
                                    Clear filters
                                </Button>
                            )}
                        </div>
                    )}

                    {!state.error && !state.loading && state.events.length > 0 && filteredEvents.length === 0 && (
                        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-20 text-center">
                            <CalendarX2 className="h-10 w-10 text-muted-foreground" />
                            <p className="font-medium">No events match that price range</p>
                            <p className="max-w-sm text-sm text-muted-foreground">Try a wider price filter.</p>
                            <Button variant="outline" onClick={() => setPriceFilter('any')}>
                                Reset price filter
                            </Button>
                        </div>
                    )}

                    {!state.error && !state.loading && filteredEvents.length > 0 && (
                        <>
                            <p className="mb-6 text-sm text-muted-foreground">
                                {filteredEvents.length} event{filteredEvents.length === 1 ? '' : 's'}
                            </p>
                            <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                                {filteredEvents.map((event, i) => (
                                    <EventCard key={event.id} event={event} pricing={pricing[event.slug || event.id]} index={i} />
                                ))}
                            </div>

                            {state.events.length >= limit && (
                                <div className="mt-10 flex justify-center">
                                    <Button variant="outline" onClick={() => setLimit((l) => l + PAGE_SIZE)}>
                                        Load more events
                                    </Button>
                                </div>
                            )}
                        </>
                    )}
                </section>
            </main>
            <Footer />
        </div>
    );
}
