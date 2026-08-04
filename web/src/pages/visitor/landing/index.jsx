import React, { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { motion } from 'framer-motion';
import { QrCode, ShieldCheck, WifiOff, ArrowRight, CalendarX2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonCardGrid } from '@/components/ui/skeleton';
import Header from '@/pages/visitor/header';
import Footer from './footer';
import Hero from './hero';
import { events as eventsApi } from '@/lib/api';
import EventCard from '@/pages/visitor/events/event-card';
import CategoryTabs from '@/pages/visitor/events/category-tabs';
import { useCategories } from '@/pages/visitor/events/use-categories';
import { useEventPricing } from '@/pages/visitor/events/use-event-pricing';
import { humanError } from '@/pages/visitor/errors';
import { EMPTY_HEADING, emptyDescription, hostSubheading, orgForEvent, showsOrgLabels } from '@/lib/host';

// The homepage is a preview, but the preview has to actually be wide enough
// to show what's on — a handful of events isn't a preview, it's the whole
// catalogue looking sparse. 12 comfortably covers a new/small deployment
// (--demo seeds 10) while still being a real cap for a large one; "See what
// else is on" below always covers the rest either way.
const HOMEPAGE_EVENT_LIMIT = 12;

// Plain language, because the person deciding whether to use this runs a
// venue, not a distributed system. The signing scheme is true and it is
// still on the site — one page down, in /docs, under "how it actually
// works", where an engineer evaluating it will look for it.
const HOW_IT_WORKS = [
    {
        icon: QrCode,
        title: 'They buy a ticket',
        description: 'The ticket is a QR code that carries its own proof. It goes wherever the buyer goes — no app to install.',
    },
    {
        icon: WifiOff,
        title: 'You open the scanner once',
        description: 'One tap while you still have signal downloads everything the door needs for the whole night. Then unplug.',
    },
    {
        icon: ShieldCheck,
        title: 'People get in',
        description: 'The phone checks each ticket on the spot and catches repeats at that door. Nothing to ask, nobody to wait for.',
    },
];

const LandingPage = () => {
    const navigate = useNavigate();
    const [state, setState] = useState({ events: [], host: null, loading: true, error: null });
    const [reloadToken, setReloadToken] = useState(0);
    const { categories, loading: categoriesLoading, error: categoriesError } = useCategories();

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        eventsApi
            .list({ limit: HOMEPAGE_EVENT_LIMIT })
            .then((data) => {
                if (cancelled) return;
                // `host` says WHOSE events these are. A missing envelope stays
                // null and every @/lib/host helper degrades to the name-free
                // single-tenant page rather than inventing a name.
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
    }, [reloadToken]);

    const pricing = useEventPricing(state.events);

    // The homepage shows a preview only — searching, and picking a category,
    // both send the visitor to the full browse/search/filter surface at
    // /events rather than duplicating that filtering logic here.
    const handleSearch = (value) => {
        const params = new URLSearchParams();
        if (value) params.set('q', value);
        navigate(`/events${params.toString() ? `?${params}` : ''}`);
    };

    return (
        <div className="flex min-h-screen flex-col bg-background">
            <Header />
            <main id="main" className="flex-1">
                <Hero query="" onSearch={handleSearch} />

                <div className="border-b border-border bg-background/60 py-5">
                    <div className="container mx-auto px-4">
                        <CategoryTabs
                            categories={categories}
                            loading={categoriesLoading}
                            error={categoriesError}
                            getHref={(slug) => `/events${slug ? `?category=${encodeURIComponent(slug)}` : ''}`}
                        />
                    </div>
                </div>

                <section className="container mx-auto px-4 py-16">
                    {state.error && (
                        <ErrorState
                            title="We couldn't load the events"
                            description={humanError(state.error, 'Something went wrong on the way to the server. Check your connection and try again.')}
                            onRetry={() => setReloadToken((t) => t + 1)}
                            className="py-20"
                        />
                    )}

                    {!state.error && state.loading && (
                        <>
                            <div className="mb-8 h-8 w-48 animate-pulse rounded bg-muted" />
                            <SkeletonCardGrid count={3} className="sm:grid-cols-2 lg:grid-cols-3" />
                        </>
                    )}

                    {!state.error && !state.loading && state.events.length === 0 && (
                        <EmptyState
                            icon={CalendarX2}
                            title={EMPTY_HEADING}
                            description={emptyDescription(state.host)}
                            className="py-20"
                        />
                    )}

                    {/* One listing, not two stacked grids. It used to split
                        into "Next up" (the soonest three) and "Upcoming
                        events" (the rest) with matching card treatment for
                        both — which read as one undifferentiated wall of
                        cards, not two meaningful groups. The soonest event
                        still gets a genuinely different, wider treatment
                        below (`sm:col-span-2`, `featured`) so a visitor's eye
                        lands somewhere before it hits the grid; it does not
                        get its own heading, because "the soonest one" isn't
                        a claim worth a second section, just a claim worth a
                        bigger card. */}
                    {!state.error && !state.loading && state.events.length > 0 && (
                        <div>
                            <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
                                <div>
                                    <h2 className="font-display text-2xl font-bold tracking-tight sm:text-3xl">What&apos;s on</h2>
                                    {/* Not "happening near you" — there is no
                                        geolocation and no discovery anywhere in
                                        this product. The honest line is who is
                                        selling, which is what hostSubheading
                                        answers. */}
                                    <p className="mt-1 text-muted-foreground">{hostSubheading(state.host)}</p>
                                </div>
                                <Button variant="outline" asChild>
                                    <Link to="/events">
                                        See what else is on
                                        <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                                    </Link>
                                </Button>
                            </div>
                            <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
                                {state.events.map((event, i) => (
                                    <div key={event.id} className={i === 0 ? 'sm:col-span-2' : undefined}>
                                        <EventCard
                                            event={event}
                                            org={showsOrgLabels(state.host) ? orgForEvent(state.host, event) : null}
                                            pricing={pricing[event.slug || event.id]}
                                            index={i}
                                            featured={i === 0}
                                        />
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}
                </section>

                <section className="border-t border-border bg-muted/30 py-20">
                    <div className="container mx-auto px-4">
                        <div className="mx-auto mb-14 max-w-2xl text-center">
                            <span className="text-xs font-semibold uppercase tracking-[0.2em] text-primary-emphasis">The whole flow</span>
                            <h2 className="mt-3 font-display text-2xl font-bold tracking-tight sm:text-3xl">How it works</h2>
                            <p className="mt-2 text-muted-foreground">The door is the whole product. Here&apos;s the night, start to finish.</p>
                        </div>

                        {/* A dashed thread connects the three steps on wide screens — the
                            same tear-line motif as the ticket mark, running through the
                            whole sequence rather than three disconnected cards. */}
                        <div className="relative grid grid-cols-1 gap-8 md:grid-cols-3">
                            <div
                                className="pointer-events-none absolute left-0 right-0 top-8 hidden border-t-2 border-dashed border-border md:block"
                                aria-hidden="true"
                            />
                            {HOW_IT_WORKS.map(({ icon: Icon, title, description }, i) => (
                                <motion.div
                                    key={title}
                                    initial={{ opacity: 0, y: 16 }}
                                    whileInView={{ opacity: 1, y: 0 }}
                                    viewport={{ once: true, margin: '-80px' }}
                                    transition={{ duration: 0.4, delay: i * 0.1 }}
                                    className="relative rounded-2xl border border-border bg-card p-6 text-center shadow-sm"
                                >
                                    <div className="relative mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full border-4 border-muted/30 bg-primary/10 text-primary-emphasis">
                                        <Icon className="h-7 w-7" aria-hidden="true" />
                                        <span className="absolute -right-1.5 -top-1.5 flex h-6 w-6 items-center justify-center rounded-full bg-primary text-xs font-black text-primary-foreground shadow-soft">
                                            {i + 1}
                                        </span>
                                    </div>
                                    <h3 className="mb-2 font-semibold">{title}</h3>
                                    <p className="text-sm text-muted-foreground">{description}</p>
                                </motion.div>
                            ))}
                        </div>
                        <div className="mt-12 flex justify-center">
                            <Button size="lg" asChild>
                                <Link to="/pricing">Start selling tickets</Link>
                            </Button>
                        </div>
                    </div>
                </section>
            </main>
            <Footer />
        </div>
    );
};

export default LandingPage;
