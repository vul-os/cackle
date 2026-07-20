import React, { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Calendar, MapPin, ShieldCheck, QrCode, Wifi, WifiOff, AlertCircle } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import Header from '@/pages/visitor/header';
import Footer from './footer';
import Hero from './hero';
import { events as eventsApi } from '@/lib/api';

function formatDate(iso) {
    if (!iso) return 'Date TBA';
    try {
        return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
        return 'Date TBA';
    }
}

function formatMoney(cents, currency = 'ZAR') {
    if (cents === undefined || cents === null) return '';
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(cents / 100);
    } catch {
        return `${(cents / 100).toFixed(2)} ${currency}`;
    }
}

const EventCard = ({ event, index }) => (
    <motion.div
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, delay: Math.min(index * 0.05, 0.4) }}
    >
        <Link to={`/events/${event.slug}`} className="group block h-full">
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
                    {event.min_price_cents !== undefined && (
                        <div className="absolute right-3 top-3 rounded-full bg-primary px-3 py-1 text-xs font-semibold text-primary-foreground shadow">
                            {event.min_price_cents === 0 ? 'Free' : `From ${formatMoney(event.min_price_cents, event.currency)}`}
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
    const [searchParams, setSearchParams] = useSearchParams();
    const query = searchParams.get('q') || '';

    const [state, setState] = useState({ events: [], loading: true, error: null });

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        eventsApi
            .list({ q: query || undefined, limit: 24 })
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
    }, [query]);

    const handleSearch = (value) => {
        const params = {};
        if (value) params.q = value;
        setSearchParams(params);
    };

    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main>
                <Hero query={query} onSearch={handleSearch} />

                <section className="container mx-auto px-4 py-16">
                    <div className="mb-8 flex items-end justify-between">
                        <div>
                            <h2 className="font-display text-2xl font-bold tracking-tight sm:text-3xl">
                                {query ? `Results for "${query}"` : 'Upcoming events'}
                            </h2>
                            <p className="mt-1 text-muted-foreground">Find something happening near you.</p>
                        </div>
                    </div>

                    {state.error && (
                        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
                            <AlertCircle className="h-8 w-8 text-destructive" />
                            <p className="font-medium">Couldn&apos;t load events</p>
                            <p className="max-w-sm text-sm text-muted-foreground">{state.error}</p>
                        </div>
                    )}

                    {!state.error && state.loading && (
                        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
                            {Array.from({ length: 6 }).map((_, i) => (
                                <CardSkeleton key={i} />
                            ))}
                        </div>
                    )}

                    {!state.error && !state.loading && state.events.length === 0 && (
                        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
                            <Wifi className="h-8 w-8 text-muted-foreground" />
                            <p className="font-medium">No events found</p>
                            <p className="max-w-sm text-sm text-muted-foreground">
                                {query ? 'Try a different search.' : 'Check back soon — new events are added all the time.'}
                            </p>
                        </div>
                    )}

                    {!state.error && !state.loading && state.events.length > 0 && (
                        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
                            {state.events.map((event, i) => (
                                <EventCard key={event.id} event={event} index={i} />
                            ))}
                        </div>
                    )}
                </section>

                <section className="border-t border-border bg-muted/30 py-20">
                    <div className="container mx-auto px-4">
                        <div className="mx-auto mb-12 max-w-2xl text-center">
                            <h2 className="font-display text-2xl font-bold tracking-tight sm:text-3xl">How it works</h2>
                            <p className="mt-2 text-muted-foreground">The gate is the whole product. Here&apos;s the flow.</p>
                        </div>
                        <div className="grid grid-cols-1 gap-8 md:grid-cols-3">
                            {HOW_IT_WORKS.map(({ icon: Icon, title, description }, i) => (
                                <motion.div
                                    key={title}
                                    initial={{ opacity: 0, y: 16 }}
                                    whileInView={{ opacity: 1, y: 0 }}
                                    viewport={{ once: true, margin: '-80px' }}
                                    transition={{ duration: 0.4, delay: i * 0.1 }}
                                    className="rounded-2xl border border-border bg-card p-6 text-center shadow-sm"
                                >
                                    <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
                                        <Icon className="h-6 w-6" />
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
