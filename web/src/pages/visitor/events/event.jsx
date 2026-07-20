import React, { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { ErrorState } from '@/components/ui/error-state';
import { CalendarX2 } from 'lucide-react';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer.jsx';
import { events as eventsApi } from '@/lib/api';

import EventGallery from './gallery';
import EventHeader from './header';
import EventQuickInfo from './quick-info';
import ProcessedText from './processed-text';
import LocationSection from './location';
import TicketSelection, { MobileStickyCta } from './ticket-selection';
import { getEventImages } from './media';

const LoadingView = () => (
    <div className="min-h-screen bg-background">
        <Header />
        <div className="pt-16">
            <Skeleton className="h-[50vh] min-h-[320px] w-full rounded-none sm:h-[60vh]" />
            <div className="mx-auto max-w-6xl space-y-6 p-6 sm:p-8">
                <div className="grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2 lg:grid-cols-5">
                    {Array.from({ length: 5 }).map((_, i) => (
                        <div key={i} className="flex items-center gap-3">
                            <Skeleton className="h-11 w-11 shrink-0 rounded-full" />
                            <div className="min-w-0 flex-1 space-y-2">
                                <Skeleton className="h-3 w-16" />
                                <Skeleton className="h-4 w-24" />
                            </div>
                        </div>
                    ))}
                </div>
                <div className="grid grid-cols-1 gap-8 md:grid-cols-3">
                    <div className="space-y-4 md:col-span-2">
                        <Skeleton className="h-8 w-1/2" />
                        <Skeleton className="h-4 w-full" />
                        <Skeleton className="h-4 w-5/6" />
                        <Skeleton className="h-4 w-2/3" />
                    </div>
                    <Skeleton className="h-64 w-full md:col-span-1" />
                </div>
            </div>
        </div>
    </div>
);

const NotFoundView = ({ message }) => (
    <div className="min-h-screen bg-background">
        <Header />
        <div className="flex min-h-screen flex-col items-center justify-center gap-3 px-4 pt-16 text-center">
            <ErrorState icon={CalendarX2} title="Couldn't load this event" description={message} className="border-none bg-transparent" />
        </div>
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
                const rawEvent = data?.event ?? data;
                const ticketTypes = data?.ticket_types ?? rawEvent?.ticket_types ?? [];
                // The gallery rides alongside `event` in this response, not
                // nested inside it (see docs/API.md) — fold it in here so
                // `getEventImages` has one consistent shape to read from.
                const gallery = data?.gallery ?? rawEvent?.gallery ?? [];
                const event = rawEvent ? { ...rawEvent, gallery } : rawEvent;
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
    if (error) return <NotFoundView message={error} />;
    if (!event) return <NotFoundView message="This event doesn't exist, or isn't published yet." />;

    const images = getEventImages(event);

    return (
        <div className="min-h-screen bg-background">
            <Header />

            <div className="flex flex-col pt-16">
                <div className="relative h-[50vh] min-h-[320px] sm:h-[60vh]">
                    <EventGallery images={images} title={event.title} className="absolute inset-0" />
                    <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/90 via-black/40 to-transparent" />
                    <EventHeader title={event.title} venueName={event.venue_name} category={event.category} />
                </div>

                <div className="border-t border-border bg-card shadow-lg">
                    <div className="mx-auto max-w-6xl p-6 sm:p-8">
                        <EventQuickInfo event={event} ticketTypes={ticketTypes} />
                    </div>
                </div>

                <div className="mx-auto my-8 grid w-full max-w-6xl grid-cols-1 gap-8 p-4 pb-28 sm:p-8 lg:grid-cols-3 lg:pb-8">
                    <div className="space-y-8 lg:col-span-2">
                        {(event.summary || event.description) && (
                            <Card>
                                <CardContent className="p-6 sm:p-8">
                                    <h2 className="mb-4 font-display text-2xl font-bold">About this event</h2>
                                    {event.summary && <p className="mb-4 text-lg text-muted-foreground">{event.summary}</p>}
                                    <ProcessedText content={event.description} />
                                </CardContent>
                            </Card>
                        )}
                        <LocationSection venueName={event.venue_name} address={event.address} lat={event.lat} lng={event.lng} />
                    </div>

                    <div className="lg:col-span-1">
                        <TicketSelection event={event} ticketTypes={ticketTypes} className="lg:sticky lg:top-24" />
                    </div>
                </div>
            </div>

            <Footer />
            {/* Reserves room below the footer so the fixed mobile CTA bar never
                permanently covers the last bit of content — only needed on the
                small screens where that bar renders. */}
            <div className="h-24 lg:hidden" aria-hidden="true" />

            <MobileStickyCta event={event} ticketTypes={ticketTypes} />
        </div>
    );
};

export default EventPage;
