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

const FEATURED_COUNT = 3;

const HOW_IT_WORKS = [
    {
        icon: QrCode,
        title: 'Buy a ticket',
        description: 'Every ticket is a compact Ed25519-signed capability — it fits in a QR code and travels with you.',
    },
    {
        icon: WifiOff,
        title: 'Gate goes offline',
        description: 'Staff download a scan bundle once while online. From then on, the network is optional.',
    },
    {
        icon: ShieldCheck,
        title: 'Instant, offline admission',
        description: 'The scanner verifies the signature locally and blocks duplicates — no venue server required.',
    },
];

const LandingPage = () => {
    const navigate = useNavigate();
    const [state, setState] = useState({ events: [], loading: true, error: null });
    const [reloadToken, setReloadToken] = useState(0);
    const { categories, loading: categoriesLoading, error: categoriesError } = useCategories();

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        eventsApi
            .list({ limit: 8 })
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

    const featured = state.events.slice(0, FEATURED_COUNT);
    const upcoming = state.events.slice(FEATURED_COUNT);

    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main>
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
                        <ErrorState description={state.error} onRetry={() => setReloadToken((t) => t + 1)} className="py-20" />
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
                            title="No events yet"
                            description="Check back soon — new events are added all the time."
                            className="py-20"
                        />
                    )}

                    {!state.error && !state.loading && featured.length > 0 && (
                        <div className="mb-16">
                            <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
                                <div>
                                    <h2 className="font-display text-2xl font-bold tracking-tight sm:text-3xl">Featured</h2>
                                    <p className="mt-1 text-muted-foreground">Hand-picked events happening soon.</p>
                                </div>
                            </div>
                            <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
                                {featured.map((event, i) => (
                                    <EventCard key={event.id} event={event} pricing={pricing[event.slug || event.id]} index={i} featured />
                                ))}
                            </div>
                        </div>
                    )}

                    {!state.error && !state.loading && upcoming.length > 0 && (
                        <div>
                            <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
                                <div>
                                    <h2 className="font-display text-2xl font-bold tracking-tight sm:text-3xl">Upcoming events</h2>
                                    <p className="mt-1 text-muted-foreground">Find something happening near you.</p>
                                </div>
                                <Button variant="outline" asChild>
                                    <Link to="/events">
                                        Browse all events
                                        <ArrowRight className="ml-2 h-4 w-4" />
                                    </Link>
                                </Button>
                            </div>
                            <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
                                {upcoming.map((event, i) => (
                                    <EventCard key={event.id} event={event} pricing={pricing[event.slug || event.id]} index={i} />
                                ))}
                            </div>
                        </div>
                    )}

                    {!state.error && !state.loading && state.events.length > 0 && upcoming.length === 0 && (
                        <div className="mt-4 flex justify-center">
                            <Button variant="outline" asChild>
                                <Link to="/events">
                                    Browse all events
                                    <ArrowRight className="ml-2 h-4 w-4" />
                                </Link>
                            </Button>
                        </div>
                    )}
                </section>

                <section className="border-t border-border bg-muted/30 py-20">
                    <div className="container mx-auto px-4">
                        <div className="mx-auto mb-14 max-w-2xl text-center">
                            <span className="text-xs font-semibold uppercase tracking-[0.2em] text-primary">The whole flow</span>
                            <h2 className="mt-3 font-display text-2xl font-bold tracking-tight sm:text-3xl">How it works</h2>
                            <p className="mt-2 text-muted-foreground">The gate is the whole product. Here&apos;s the flow, start to finish.</p>
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
                                    <div className="relative mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full border-4 border-muted/30 bg-primary/10 text-primary">
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
