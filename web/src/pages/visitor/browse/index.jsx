// Public event browse — the storefront's main listing surface. Search and
// filter published events, anonymously, with real availability/price info
// pulled in per-card from the public event-detail endpoint (the list
// endpoint itself carries no pricing — see docs/API.md), plus a category
// filter wired to ?category= and GET /api/categories.
import React, { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Search, CalendarX2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonCardGrid } from '@/components/ui/skeleton';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import { events as eventsApi } from '@/lib/api';
import EventCard from '@/pages/visitor/events/event-card';
import CategoryTabs from '@/pages/visitor/events/category-tabs';
import { useCategories } from '@/pages/visitor/events/use-categories';
import { useEventPricing } from '@/pages/visitor/events/use-event-pricing';
import { minorToMajorNumber } from '@/lib/money';

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

// Price bands compare MAJOR-unit values, not raw minor units — 250 means
// "250 of whatever the event's own currency is" (250 ZAR, 250 JPY, 250
// KWD, ...). Cackle never converts currencies, so these bands are
// deliberately currency-neutral (no "R" prefix) rather than pretending a
// single absolute threshold means the same thing across every currency
// an event listing might mix together.
const PRICE_FILTERS = {
    any: { label: 'Any price', test: () => true },
    free: { label: 'Free', test: (major) => major === 0 },
    under250: { label: 'Budget (under 250)', test: (major) => major > 0 && major < 250 },
    under750: { label: 'Mid-range (250 – 750)', test: (major) => major >= 250 && major < 750 },
    over750: { label: 'Premium (750+)', test: (major) => major >= 750 },
};

export default function BrowsePage() {
    const [searchParams, setSearchParams] = useSearchParams();
    const query = searchParams.get('q') || '';
    const category = searchParams.get('category') || '';
    const [searchValue, setSearchValue] = useState(query);
    const [dateFilter, setDateFilter] = useState('any');
    const [priceFilter, setPriceFilter] = useState('any');
    const [limit, setLimit] = useState(PAGE_SIZE);
    const [reloadToken, setReloadToken] = useState(0);

    const [state, setState] = useState({ events: [], loading: true, error: null });
    const { categories, loading: categoriesLoading, error: categoriesError } = useCategories();

    // Keep the search box in sync if the query changes from outside this
    // component (e.g. the landing page's search bar navigating in with ?q=).
    useEffect(() => {
        setSearchValue(query);
    }, [query]);

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        const { from, to } = dateRangeFor(dateFilter);
        eventsApi
            .list({
                q: query || undefined,
                category: category || undefined,
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
    }, [query, category, dateFilter, limit, reloadToken]);

    const pricing = useEventPricing(state.events);

    const filteredEvents = useMemo(() => {
        if (priceFilter === 'any') return state.events;
        const test = PRICE_FILTERS[priceFilter].test;
        return state.events.filter((e) => {
            const p = pricing[e.slug || e.id];
            // While pricing is still loading for a card, keep it visible
            // rather than flashing it out then back in.
            if (!p || p.minPriceMinor === null || p.minPriceMinor === undefined) return true;
            return test(minorToMajorNumber(p.minPriceMinor, e.currency));
        });
    }, [state.events, pricing, priceFilter]);

    const handleSearchSubmit = (e) => {
        e.preventDefault();
        const params = {};
        if (searchValue) params.q = searchValue;
        if (category) params.category = category;
        setSearchParams(params);
        setLimit(PAGE_SIZE);
    };

    const handleCategorySelect = (slug) => {
        const params = {};
        if (query) params.q = query;
        if (slug) params.category = slug;
        setSearchParams(params);
        setLimit(PAGE_SIZE);
    };

    const hasActiveFilters = Boolean(query) || Boolean(category) || dateFilter !== 'any' || priceFilter !== 'any';
    const clearFilters = () => {
        setSearchValue('');
        setSearchParams({});
        setDateFilter('any');
        setPriceFilter('any');
        setLimit(PAGE_SIZE);
    };

    const activeCategoryLabel = category ? categories.find((c) => c.slug === category)?.label || category : '';

    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main className="pt-16">
                <div className="border-b border-border bg-muted/30">
                    <div className="container mx-auto px-4 py-10">
                        <h1 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">Browse events</h1>
                        <p className="mt-2 text-muted-foreground">Search live events and grab your tickets.</p>

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

                        <CategoryTabs
                            categories={categories}
                            value={category}
                            loading={categoriesLoading}
                            error={categoriesError}
                            onSelect={handleCategorySelect}
                            className="mt-5"
                        />

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
                        <ErrorState description={state.error} onRetry={() => setReloadToken((t) => t + 1)} className="py-20" />
                    )}

                    {!state.error && state.loading && <SkeletonCardGrid count={8} className="sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4" />}

                    {!state.error && !state.loading && state.events.length === 0 && (
                        <EmptyState
                            icon={CalendarX2}
                            title="No events found"
                            description={
                                hasActiveFilters
                                    ? 'Try a different search or loosen your filters.'
                                    : 'Check back soon — new events are added all the time.'
                            }
                            action={
                                hasActiveFilters ? (
                                    <Button variant="outline" onClick={clearFilters}>
                                        Clear filters
                                    </Button>
                                ) : undefined
                            }
                            className="py-20"
                        />
                    )}

                    {!state.error && !state.loading && state.events.length > 0 && filteredEvents.length === 0 && (
                        <EmptyState
                            icon={CalendarX2}
                            title="No events match that price range"
                            description="Try a wider price filter."
                            action={
                                <Button variant="outline" onClick={() => setPriceFilter('any')}>
                                    Reset price filter
                                </Button>
                            }
                            className="py-20"
                        />
                    )}

                    {!state.error && !state.loading && filteredEvents.length > 0 && (
                        <>
                            <p className="mb-6 text-sm text-muted-foreground">
                                {filteredEvents.length} event{filteredEvents.length === 1 ? '' : 's'}
                                {activeCategoryLabel ? <span className="capitalize"> in {activeCategoryLabel}</span> : null}
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
