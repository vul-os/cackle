// This host's event listing. Search and filter the published events on THIS
// box, anonymously, with real availability/price info pulled in per-card from
// the public event-detail endpoint (the list endpoint itself carries no
// pricing — see docs/API.md), plus a category filter wired to ?category= and
// GET /api/categories.
//
// It is not a marketplace and the copy must not read like one. Cackle is
// self-hosted: somebody installed it to sell tickets to their own events, so
// this page is "what's on at their place", not "every event there is". Who
// that somebody is comes from the `host` envelope on GET /api/events and is
// turned into words in ONE place — @/lib/host — so no page can invent a name
// or dress a single venue up as a directory. `?host=<org-slug>` narrows the
// listing to one organisation on a box that hosts several.
//
// # The seven "overflowing" elements at 390px — measured, and both false
//
// A sweep of this page reports seven element boxes reaching outside a 390px
// viewport. Measured against check-site.mjs's rule — only a genuine
// `overflow-x: auto|scroll` ancestor forgives, and the walk stops before
// <body> so index.css's `body{overflow-x:hidden}` cannot launder anything —
// both classes are correct by construction, not defects:
//
//  1. The header's skip link (`pages/visitor/header.tsx`) is Tailwind's
//     `sr-only`: 1x1, `position:absolute`, `margin:-1px`, `clip:rect(0,0,0,0)`.
//     The -1px margin is what puts it at x=-1…0. That is the off-screen
//     technique itself, it is clipped to nothing, and leftward overhang cannot
//     extend the scroll area in an LTR document — with body's overflow-x
//     forced back to `visible` the document still measures
//     scrollWidth 390 == clientWidth 390. Moving it would break the affordance
//     to satisfy a measurement.
//  2. The six category chips (`pages/visitor/events/category-tabs.tsx`) sit in
//     a real scroll container: computed `overflow-x: auto`, clientWidth 358,
//     scrollWidth 590, and scrollLeft reaches 232. They scroll; nothing is
//     silently clipped. `hidden`/`clip` would NOT be forgiven here — that is
//     the bug class the rule exists to find — but this is not that.
//
// Neither is masked by `body { overflow-x: hidden }` in index.css: that rule
// is the seatbelt for a stray element, and removing it changes nothing here.
// The multi-organisation branch below was measured too, with a 76-character
// organisation name, at 390/768/1440 in both themes: the chip row wraps and
// nothing overflows.
import React, { useEffect, useMemo, useState } from 'react';
import { useSearchParams, Link } from 'react-router-dom';
import { Search, CalendarX2, X, ExternalLink } from 'lucide-react';
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
import { humanError } from '@/pages/visitor/errors';
import { EMPTY_HEADING, emptyDescription, hostHeading, hostOrgs, hostSubheading, orgForEvent, orgHref, showsOrgLabels } from '@/lib/host';
import { orgPageHref } from '@/lib/org-page';
import PeerEvents from '@/pages/visitor/events/peer-events';

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
    // ?host= narrows the listing to one organisation. It is a SCOPE, not a
    // filter — "Clear filters" keeps it, because the visitor followed an
    // organisation's own link to get here and clearing their search should
    // not silently widen the page back out to everybody on the box.
    const hostParam = searchParams.get('host') || '';
    const [searchValue, setSearchValue] = useState(query);
    const [dateFilter, setDateFilter] = useState('any');
    const [priceFilter, setPriceFilter] = useState('any');
    const [limit, setLimit] = useState(PAGE_SIZE);
    const [reloadToken, setReloadToken] = useState(0);

    const [state, setState] = useState({ events: [], host: null, loading: true, error: null });
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
                host: hostParam || undefined,
                from: from ? from.toISOString() : undefined,
                to: to ? to.toISOString() : undefined,
                limit,
            })
            .then((data) => {
                if (cancelled) return;
                // An older server, or a request that fell back to a bare
                // array, leaves `host` null — every helper in @/lib/host
                // degrades that to the name-free single-tenant page rather
                // than guessing, so a missing envelope can never produce
                // marketplace copy.
                setState({
                    events: Array.isArray(data) ? data : (data?.events ?? []),
                    host: Array.isArray(data) ? null : (data?.host ?? null),
                    loading: false,
                    error: null,
                });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ events: [], host: null, loading: false, error: err.message || 'Could not load events.' });
            });
        return () => {
            cancelled = true;
        };
    }, [query, category, hostParam, dateFilter, limit, reloadToken]);

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
        if (hostParam) params.host = hostParam;
        setSearchParams(params);
        setLimit(PAGE_SIZE);
    };

    const handleCategorySelect = (slug) => {
        const params = {};
        if (query) params.q = query;
        if (slug) params.category = slug;
        if (hostParam) params.host = hostParam;
        setSearchParams(params);
        setLimit(PAGE_SIZE);
    };

    const hasActiveFilters = Boolean(query) || Boolean(category) || dateFilter !== 'any' || priceFilter !== 'any';
    const clearFilters = () => {
        setSearchValue('');
        setSearchParams(hostParam ? { host: hostParam } : {});
        setDateFilter('any');
        setPriceFilter('any');
        setLimit(PAGE_SIZE);
    };

    const activeCategoryLabel = category ? categories.find((c) => c.slug === category)?.label || category : '';

    return (
        <div className="flex min-h-screen flex-col bg-background">
            <Header />
            <main id="main" className="flex-1 pt-16">
                <div className="border-b border-border bg-muted/30">
                    <div className="container mx-auto px-4 py-10">
                        <h1 className="font-display text-display-sm font-extrabold tracking-tight sm:text-display-lg">
                            {hostHeading(state.host)}
                        </h1>
                        <p className="mt-2 text-muted-foreground">{hostSubheading(state.host)}</p>

                        <form onSubmit={handleSearchSubmit} className="mt-6 flex flex-col gap-3 sm:flex-row" role="search">
                            <div className="relative flex-1">
                                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                                {/* "or organisers" promised a search across
                                    organisers. The query only ever matches the
                                    title, summary and venue of the events
                                    already on this page (internal/store/events.go
                                    ListPublished) — so say that. */}
                                <label htmlFor="browse-search" className="sr-only">
                                    Search these events
                                </label>
                                <Input
                                    id="browse-search"
                                    value={searchValue}
                                    onChange={(e) => setSearchValue(e.target.value)}
                                    placeholder="Search these events"
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

                        {/* The organisations on this box, and the only way to
                            narrow to one. Rendered strictly on `multi_org` —
                            a single venue gets no row at all, because a row of
                            one is a directory of one, and this box is not a
                            directory.

                            These chips stay pointed at this same listing,
                            narrowed. That is deliberate now that an
                            organisation DOES have a page of its own (/o/{ref}):
                            a filter chip should filter, keeping the visitor's
                            search and category intact, rather than navigate
                            them off the page they are shopping on. The link to
                            the organisation's own page sits below, where the
                            listing has already been narrowed to it. */}
                        {showsOrgLabels(state.host) && (
                            <nav className="mt-5 flex flex-wrap gap-2" aria-label="Organisations on this site">
                                {hostOrgs(state.host).map((org) => {
                                    const active = state.host?.org?.id === org.id;
                                    return (
                                        <Link
                                            key={org.id}
                                            to={orgHref(org)}
                                            aria-current={active ? 'true' : undefined}
                                            className={`inline-flex min-h-[44px] items-center rounded-full border px-4 text-sm font-medium transition-colors sm:min-h-[36px] ${
                                                active
                                                    ? 'border-transparent bg-primary text-primary-foreground'
                                                    : 'border-border bg-background text-foreground/80 hover:border-foreground/30'
                                            }`}
                                        >
                                            {org.name}
                                        </Link>
                                    );
                                })}
                            </nav>
                        )}

                        <div className="flex flex-wrap items-center gap-x-5">
                            {hasActiveFilters && (
                                <button
                                    type="button"
                                    onClick={clearFilters}
                                    className="mt-3 inline-flex min-h-[44px] items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground sm:min-h-0"
                                >
                                    <X className="h-3.5 w-3.5" aria-hidden="true" />
                                    Clear filters
                                </button>
                            )}

                            {/* The way back out of a ?host= narrowing. Only on a
                                box that actually hosts more than one
                                organisation — on a single venue there is no
                                "everything else" to return to, and offering the
                                link would imply there is. */}
                            {state.host?.org && showsOrgLabels(state.host) && (
                                <Link
                                    to="/events"
                                    className="mt-3 inline-flex min-h-[44px] items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground sm:min-h-0"
                                >
                                    <X className="h-3.5 w-3.5" aria-hidden="true" />
                                    Show everything on this site
                                </Link>
                            )}

                            {/* The organiser's own page. Shown once the listing
                                has been narrowed to one organisation, because
                                until then there is no single organiser whose
                                page this would be.

                                It is a plain link out of the app to a
                                server-rendered page (/o/{slug}) — the address
                                you would put on a poster, and the one another
                                operator's box links back to when it shows a
                                borrowed listing. `orgPageHref` returns null
                                when there is nothing to link to, and the guard
                                below renders nothing rather than a dead link.

                                The visible text does not repeat the
                                organisation's name — the heading two lines
                                above already says "Events from X", and
                                "X&rsquo;s own page" reads badly for any name
                                ending in s. The accessible name carries it, so
                                the link is still unambiguous read on its own. */}
                            {state.host?.org && orgPageHref(state.host.org) && (
                                <a
                                    href={orgPageHref(state.host.org)}
                                    aria-label={`${state.host.org.name} — the organiser's own page`}
                                    className="mt-3 inline-flex min-h-[44px] items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground sm:min-h-0"
                                >
                                    <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                                    Organiser&rsquo;s own page
                                </a>
                            )}
                        </div>
                    </div>
                </div>

                <section className="container mx-auto px-4 py-10" data-surface="event-browse">
                    {state.error && (
                        <ErrorState
                            title="We couldn't load the events"
                            description={humanError(state.error, 'Something went wrong on the way to the server. Check your connection and try again.')}
                            onRetry={() => setReloadToken((t) => t + 1)}
                            className="py-20"
                        />
                    )}

                    {!state.error && state.loading && <SkeletonCardGrid count={8} className="sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4" />}

                    {!state.error && !state.loading && state.events.length === 0 && (
                        <EmptyState
                            icon={CalendarX2}
                            title={hasActiveFilters ? 'No events found' : EMPTY_HEADING}
                            // NOT "check back soon, new events are added all
                            // the time". Nobody adds events to somebody else's
                            // box, and on a venue between programmes that
                            // sentence is simply untrue.
                            description={
                                hasActiveFilters ? 'Try a different search or loosen your filters.' : emptyDescription(state.host)
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
                                    <EventCard
                                        key={event.id}
                                        event={event}
                                        // null on a single-organisation box, so
                                        // no organisation chrome appears at all.
                                        org={showsOrgLabels(state.host) ? orgForEvent(state.host, event) : null}
                                        pricing={pricing[event.slug || event.id]}
                                        index={i}
                                    />
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

                    {/* Somebody else's events, below this host's own and never
                        mixed into them. Renders nothing at all unless this box
                        is actually carrying a borrowed listing — see the file
                        header for why there is no empty state here. */}
                    {!state.error && !state.loading && <PeerEvents host={state.host} />}
                </section>
            </main>
            <Footer />
        </div>
    );
}
