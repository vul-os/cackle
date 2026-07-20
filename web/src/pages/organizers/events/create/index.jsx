import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Spinner } from '@/components/ui/spinner';
import { ErrorState } from '@/components/ui/error-state';
import { toast } from '@/components/ui/use-toast';
import { X } from 'lucide-react';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import { useAuth } from '@/context/use-auth';
import { slugify } from '../slug';
import WizardStepper from './stepper';
import BasicsStep from './steps/basics';
import ScheduleVenueStep from './steps/schedule-venue';
import TicketsStep from './steps/tickets-step';
import ImagesStep from './steps/images-step';
import ReviewStep from './steps/review';

const STEPS = [
    { key: 'basics', label: 'Basics' },
    { key: 'schedule', label: 'Date & Venue' },
    { key: 'tickets', label: 'Tickets' },
    { key: 'images', label: 'Images' },
    { key: 'review', label: 'Review' },
];

const EMPTY_EVENT = {
    title: '',
    category: '',
    summary: '',
    description: '',
    venue_name: '',
    address: '',
    lat: '',
    lng: '',
    // No hardcoded default currency — Cackle has no privileged currency;
    // the organiser picks explicitly in the schedule/venue step.
    currency: '',
    starts_at: '',
    ends_at: '',
};

// internal/events.Service.Create requires slug + title + starts_at + ends_at
// up front (see internal/events/events.go) — there's no server-side slug
// generation, and slug is globally unique. So the wizard can't actually
// persist a draft after step 1 (Basics) alone; the real POST /api/events
// call is deferred until step 2 (Date & Venue) supplies the missing
// required fields. Basics-only progress lives in local component state
// until then — see handleScheduleSubmit.

function computeInitialStep(event, ticketTypes) {
    if (!event.title) return 0;
    if (!event.starts_at || !event.venue_name) return 1;
    if (ticketTypes.length === 0) return 2;
    return 4; // images are optional — resuming a mostly-done draft lands on review
}

/**
 * Guided multi-step event creation flow: basics -> date/venue -> tickets ->
 * images -> review & publish. Also handles resuming an existing draft
 * (`/admin/events/:id/wizard`) so "save a draft and come back" is a real
 * capability rather than a dead end — the flat event editor
 * (`events/event/index.jsx`) remains the quick-edit surface for events
 * that are already fully set up.
 */
const CreateEventWizard = () => {
    const { id: routeId } = useParams();
    const navigate = useNavigate();
    const { activeOrg } = useAuth();

    const [loading, setLoading] = useState(!!routeId);
    const [loadError, setLoadError] = useState(null);
    const [eventId, setEventId] = useState(routeId || null);
    const [event, setEvent] = useState(EMPTY_EVENT);
    const [ticketTypes, setTicketTypes] = useState([]);
    const [images, setImages] = useState([]);
    const [coverImageId, setCoverImageId] = useState(null);

    const [step, setStep] = useState(0);
    const [maxStepReached, setMaxStepReached] = useState(0);
    const [submitting, setSubmitting] = useState(false);
    const [isPublishing, setIsPublishing] = useState(false);

    useEffect(() => {
        if (!routeId) return;
        let cancelled = false;
        (async () => {
            try {
                const [eventData, ttData] = await Promise.all([eventsApi.get(routeId), ticketTypesApi.list(routeId)]);
                if (cancelled) return;
                const ev = eventData?.event ?? eventData;
                const tts = Array.isArray(ttData) ? ttData : (ttData?.ticket_types ?? []);
                setEvent({
                    title: ev.title ?? '',
                    category: ev.category ?? '',
                    summary: ev.summary ?? '',
                    description: ev.description ?? '',
                    venue_name: ev.venue_name ?? '',
                    address: ev.address ?? '',
                    lat: ev.lat ?? '',
                    lng: ev.lng ?? '',
                    currency: ev.currency ?? '',
                    starts_at: ev.starts_at ?? '',
                    ends_at: ev.ends_at ?? '',
                });
                setTicketTypes(tts);
                // `gallery` is a sibling of `event` in the GET /api/events/{id}
                // response shape, not a field on the event object itself.
                setImages(eventData?.gallery ?? []);
                setCoverImageId(ev.cover_image_id ?? null);
                const initial = computeInitialStep(ev, tts);
                setStep(initial);
                setMaxStepReached(initial);
                setLoading(false);
            } catch (err) {
                if (cancelled) return;
                setLoadError(err.message || 'Could not load this draft.');
                setLoading(false);
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [routeId]);

    const goToStep = (index) => {
        setStep(index);
        setMaxStepReached((m) => Math.max(m, index));
    };

    const handleBasicsSubmit = async (data) => {
        setSubmitting(true);
        try {
            if (!eventId) {
                // Nothing to persist yet — Create requires starts_at/ends_at too
                // (see the note above EMPTY_EVENT). Just carry the fields forward
                // in local state until the schedule step can supply the rest.
                setEvent((e) => ({ ...e, ...data }));
            } else {
                await eventsApi.update(eventId, {
                    title: data.title,
                    category: data.category || undefined,
                    summary: data.summary || undefined,
                    description: data.description || undefined,
                });
                setEvent((e) => ({ ...e, ...data }));
            }
            goToStep(1);
        } catch (err) {
            toast({ title: 'Could not save', description: err.message, variant: 'destructive' });
        } finally {
            setSubmitting(false);
        }
    };

    const handleScheduleSubmit = async (data) => {
        setSubmitting(true);
        try {
            if (!eventId) {
                const created = await eventsApi.create({
                    org_id: activeOrg?.id,
                    slug: slugify(event.title),
                    title: event.title,
                    category: event.category || undefined,
                    summary: event.summary || undefined,
                    description: event.description || undefined,
                    venue_name: data.venue_name,
                    address: data.address || undefined,
                    lat: data.lat === '' ? undefined : Number(data.lat),
                    lng: data.lng === '' ? undefined : Number(data.lng),
                    currency: data.currency,
                    starts_at: data.starts_at,
                    ends_at: data.ends_at,
                    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
                });
                const createdEvent = created?.event ?? created;
                setEventId(createdEvent.id);
            } else {
                await eventsApi.update(eventId, {
                    venue_name: data.venue_name,
                    address: data.address || undefined,
                    lat: data.lat === '' ? undefined : Number(data.lat),
                    lng: data.lng === '' ? undefined : Number(data.lng),
                    currency: data.currency,
                    starts_at: data.starts_at,
                    ends_at: data.ends_at,
                });
            }
            setEvent((e) => ({ ...e, ...data }));
            goToStep(2);
        } catch (err) {
            toast({ title: 'Could not save', description: err.message, variant: 'destructive' });
        } finally {
            setSubmitting(false);
        }
    };

    const handlePublish = async () => {
        setIsPublishing(true);
        try {
            await eventsApi.publish(eventId);
            toast({ title: 'Published', description: 'Your event is now live.' });
            navigate(`/admin/events/${eventId}`);
        } catch (err) {
            toast({ title: 'Could not publish', description: err.message, variant: 'destructive' });
        } finally {
            setIsPublishing(false);
        }
    };

    const handleExit = () => navigate('/admin/events');

    const handleCoverChange = async (imageId) => {
        setCoverImageId(imageId);
        try {
            await eventsApi.update(eventId, { cover_image_id: imageId });
        } catch (err) {
            toast({ title: 'Could not set cover image', description: err.message, variant: 'destructive' });
        }
    };

    if (loading) return <Spinner />;

    if (loadError) {
        return (
            <div className="mx-auto max-w-2xl py-8">
                <ErrorState description={loadError} onRetry={() => window.location.reload()} />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-3xl">
            <div className="mb-6 flex items-center justify-between">
                <div>
                    <h1 className="font-display text-2xl font-bold sm:text-3xl">
                        {routeId ? `Finish setting up “${event.title || 'your event'}”` : 'Create a new event'}
                    </h1>
                    <p className="text-sm text-muted-foreground">
                        {eventId
                            ? 'Your progress is saved as you go — you can leave and come back anytime.'
                            : 'Pick a date on the next step to save your draft — after that you can leave and come back anytime.'}
                    </p>
                </div>
                <Button variant="ghost" size="icon" onClick={handleExit} aria-label="Exit and return to events">
                    <X className="h-5 w-5" />
                </Button>
            </div>

            <WizardStepper steps={STEPS} currentStep={step} maxStepReached={maxStepReached} onStepClick={goToStep} />

            <Card>
                <CardContent className="pt-6">
                    {step === 0 && <BasicsStep defaultValues={event} onSubmit={handleBasicsSubmit} submitting={submitting} />}

                    {step === 1 && (
                        <ScheduleVenueStep
                            defaultValues={event}
                            onSubmit={handleScheduleSubmit}
                            onBack={() => goToStep(0)}
                            submitting={submitting}
                        />
                    )}

                    {step === 2 && (
                        <TicketsStep
                            eventId={eventId}
                            currency={event.currency}
                            ticketTypes={ticketTypes}
                            onTicketTypesChange={(updater) => setTicketTypes((cur) => (typeof updater === 'function' ? updater(cur) : updater))}
                            onBack={() => goToStep(1)}
                            onSubmit={() => goToStep(3)}
                            submitting={submitting}
                        />
                    )}

                    {step === 3 && (
                        <ImagesStep
                            eventId={eventId}
                            images={images}
                            coverImageId={coverImageId}
                            onImagesChange={(updater) => setImages((cur) => (typeof updater === 'function' ? updater(cur) : updater))}
                            onCoverChange={handleCoverChange}
                            onBack={() => goToStep(2)}
                            onSubmit={() => goToStep(4)}
                            submitting={submitting}
                        />
                    )}

                    {step === 4 && (
                        <ReviewStep
                            event={event}
                            ticketTypes={ticketTypes}
                            coverImageId={coverImageId}
                            onBack={() => goToStep(3)}
                            onPublish={handlePublish}
                            onSaveDraft={handleExit}
                            isPublishing={isPublishing}
                        />
                    )}
                </CardContent>
            </Card>
        </div>
    );
};

export default CreateEventWizard;
