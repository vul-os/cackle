import React, { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Card, CardContent } from '@/components/ui/card';
import { AlertCircle, Calendar } from 'lucide-react';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer.jsx';
import { events as eventsApi } from '@/lib/api';

import EventHeader from './header';
import EventQuickInfo from './quick-info';
import ProcessedText from './processed-text';
import LocationSection from './location';

const LoadingView = () => (
    <div className="min-h-screen bg-background">
        <Header />
        <div className="mx-auto max-w-6xl animate-pulse px-4 pt-24">
            <div className="aspect-[21/9] rounded-xl bg-muted" />
            <div className="mt-8 h-8 w-1/2 rounded bg-muted" />
            <div className="mt-4 h-4 w-1/3 rounded bg-muted" />
        </div>
    </div>
);

const ErrorView = ({ message }) => (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background px-4 text-center">
        <AlertCircle className="h-10 w-10 text-destructive" />
        <h1 className="text-xl font-semibold">Couldn&apos;t load this event</h1>
        <p className="max-w-sm text-muted-foreground">{message}</p>
    </div>
);

const EventPage = () => {
    const { slug } = useParams();
    const [state, setState] = useState({ event: null, ticketTypes: [], loading: true, error: null });

    useEffect(() => {
        let cancelled = false;
        setState((s) => ({ ...s, loading: true, error: null }));
        eventsApi
            .get(slug)
            .then((data) => {
                if (cancelled) return;
                const event = data?.event ?? data;
                const ticketTypes = data?.ticket_types ?? event?.ticket_types ?? [];
                setState({ event, ticketTypes, loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ event: null, ticketTypes: [], loading: false, error: err.message || 'Event not found.' });
            });
        return () => {
            cancelled = true;
        };
    }, [slug]);

    const { event, ticketTypes, loading, error } = state;

    if (loading) return <LoadingView />;
    if (error) return <ErrorView message={error} />;
    if (!event) return <ErrorView message="This event doesn't exist, or isn't published yet." />;

    return (
        <div className="min-h-screen bg-background">
            <Header />

            <div className="flex flex-col pt-16">
                <div className="relative h-[50vh] min-h-[320px] sm:h-[60vh]">
                    {event.cover_image ? (
                        <img src={event.cover_image} alt={event.title} className="h-full w-full object-cover" />
                    ) : (
                        <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-primary/30 to-primary/5">
                            <Calendar className="h-20 w-20 text-primary/40" />
                        </div>
                    )}
                    <EventHeader title={event.title} venueName={event.venue_name} />
                </div>

                <div className="border-t border-border bg-card shadow-lg">
                    <div className="mx-auto max-w-6xl p-6 sm:p-8">
                        <EventQuickInfo event={event} ticketTypes={ticketTypes} />
                    </div>
                </div>

                <div className="mx-auto my-8 grid max-w-6xl grid-cols-1 gap-8 p-4 sm:p-8 md:grid-cols-3">
                    <div className="space-y-8 md:col-span-2">
                        {(event.summary || event.description) && (
                            <Card>
                                <CardContent className="p-8">
                                    <h2 className="mb-4 font-display text-2xl font-bold">About this event</h2>
                                    {event.summary && <p className="mb-4 text-lg text-muted-foreground">{event.summary}</p>}
                                    <ProcessedText content={event.description} />
                                </CardContent>
                            </Card>
                        )}
                        <LocationSection venueName={event.venue_name} address={event.address} lat={event.lat} lng={event.lng} />
                    </div>

                    <div className="md:col-span-1">
                        <Card>
                            <CardContent className="p-6">
                                <h3 className="mb-4 font-display text-lg font-bold">Ticket types</h3>
                                {ticketTypes.length === 0 ? (
                                    <p className="text-sm text-muted-foreground">No ticket types published yet.</p>
                                ) : (
                                    <ul className="space-y-3">
                                        {ticketTypes.map((t) => (
                                            <li key={t.id} className="flex items-center justify-between text-sm">
                                                <span className="font-medium">{t.name}</span>
                                                <span className="text-muted-foreground">
                                                    {new Intl.NumberFormat(undefined, { style: 'currency', currency: event.currency || 'ZAR' }).format(
                                                        (t.price_cents || 0) / 100,
                                                    )}
                                                </span>
                                            </li>
                                        ))}
                                    </ul>
                                )}
                            </CardContent>
                        </Card>
                    </div>
                </div>
            </div>

            <Footer />
        </div>
    );
};

export default EventPage;
