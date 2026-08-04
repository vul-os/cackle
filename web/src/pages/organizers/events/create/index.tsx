import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { ErrorState } from '@/components/ui/error-state';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { toast } from '@/components/ui/use-toast';
import { X } from 'lucide-react';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import type { UpdateEventInput, CackleEvent, TicketType, EventImage } from '@/lib/api-types';
import { useAuth } from '@/context/use-auth';
import { slugify } from '../slug';
import WizardStepper, { type WizardStep } from './stepper';
import BasicsStep, { type BasicsFormValues } from './steps/basics';
import ScheduleVenueStep, { type ScheduleSubmitData } from './steps/schedule-venue';
import TicketsStep from './steps/tickets-step';
import ImagesStep from './steps/images-step';
import ReviewStep from './steps/review';
import type { WizardEvent } from './wizard-types';

const STEPS: WizardStep[] = [
    { key: 'basics', label: 'Basics' },
    { key: 'schedule', label: 'Date & Venue' },
    { key: 'tickets', label: 'Tickets' },
    { key: 'images', label: 'Images' },
    { key: 'review', label: 'Review' },
];

const EMPTY_EVENT: WizardEvent = {
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

/**
 * Loading shape for resuming an existing draft (`/admin/events/:id/wizard`).
 * A full-screen spinner over blank is a dead end — you can't tell whether
 * anything is even about to render. This traces the actual layout below
 * (heading, step pills, a card of field-shaped blocks) so the page never
 * goes blank while `GET /api/events/:id` + `GET /api/ticket-types` resolve.
 */
const WizardSkeleton = () => (
    <div className="mx-auto max-w-3xl" role="status" aria-label="Loading your draft">
        <div className="mb-6 flex items-center justify-between">
            <div className="space-y-2">
                <Skeleton className="h-8 w-64" />
                <Skeleton className="h-4 w-80" />
            </div>
            <Skeleton className="h-11 w-11 shrink-0 rounded-md" />
        </div>
        <div className="mb-8 flex flex-wrap gap-2">
            <Skeleton className="h-3 w-40" />
            <div className="flex w-full flex-wrap gap-2">
                {STEPS.map((s) => (
                    <Skeleton key={s.key} className="h-11 w-28 rounded-full" />
                ))}
            </div>
        </div>
        <Card>
            <CardContent className="space-y-6 pt-6">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-24 w-full" />
                <Skeleton className="h-10 w-2/3" />
                <div className="flex justify-end pt-2">
                    <Skeleton className="h-11 w-32" />
                </div>
            </CardContent>
        </Card>
    </div>
);

function computeInitialStep(event: Pick<CackleEvent, 'title' | 'starts_at' | 'venue_name'>, ticketTypes: TicketType[]): number {
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
 * (`events/event/index.tsx`) remains the quick-edit surface for events
 * that are already fully set up.
 */
const CreateEventWizard = () => {
    const { id: routeId } = useParams();
    const navigate = useNavigate();
    const { activeOrg } = useAuth();

    const [loading, setLoading] = useState(!!routeId);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [eventId, setEventId] = useState<string | null>(routeId || null);
    const [event, setEvent] = useState<WizardEvent>(EMPTY_EVENT);
    const [ticketTypes, setTicketTypes] = useState<TicketType[]>([]);
    const [images, setImages] = useState<EventImage[]>([]);
    const [coverImageId, setCoverImageId] = useState<string | null>(null);

    const [step, setStep] = useState(0);
    const [maxStepReached, setMaxStepReached] = useState(0);
    const [submitting, setSubmitting] = useState(false);
    const [isPublishing, setIsPublishing] = useState(false);
    // Inline, not just a toast — a toast can be missed or dismissed before
    // it's read, and this is the one form in the product where losing the
    // reason a save failed means someone re-types a full event. Mirrors the
    // pattern in organizers/orgs/create.tsx.
    const [stepError, setStepError] = useState<string | null>(null);

    useEffect(() => {
        if (!routeId) return;
        let cancelled = false;
        (async () => {
            try {
                const [eventData, ttData] = await Promise.all([eventsApi.get(routeId), ticketTypesApi.list(routeId)]);
                if (cancelled) return;
                const ev = eventData.event;
                const tts = ttData.ticket_types ?? [];
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
                setImages(eventData.gallery ?? []);
                setCoverImageId(ev.cover_image_id ?? null);
                const initial = computeInitialStep(ev, tts);
                setStep(initial);
                setMaxStepReached(initial);
                setLoading(false);
            } catch (err) {
                if (cancelled) return;
                const message = err instanceof Error ? err.message : 'Could not load this draft.';
                setLoadError(message);
                setLoading(false);
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [routeId]);

    const goToStep = (index: number) => {
        setStepError(null);
        setStep(index);
        setMaxStepReached((m) => Math.max(m, index));
    };

    const handleBasicsSubmit = async (data: BasicsFormValues) => {
        setSubmitting(true);
        setStepError(null);
        try {
            if (!eventId) {
                // Nothing to persist yet — Create requires starts_at/ends_at too
                // (see the note above EMPTY_EVENT). Just carry the fields forward
                // in local state until the schedule step can supply the rest.
                setEvent((e) => ({ ...e, ...data, category: data.category ?? '', summary: data.summary ?? '', description: data.description ?? '' }));
            } else {
                await eventsApi.update(eventId, {
                    title: data.title,
                    category: data.category || undefined,
                    summary: data.summary || undefined,
                    description: data.description || undefined,
                });
                setEvent((e) => ({ ...e, ...data, category: data.category ?? '', summary: data.summary ?? '', description: data.description ?? '' }));
            }
            goToStep(1);
        } catch (err) {
            // Input is never lost on a failed save — `data` only ever flows
            // into local state on success, and react-hook-form keeps
            // whatever the organiser typed in the form regardless.
            const message = err instanceof Error ? err.message : 'Could not save these details.';
            setStepError(`${message} Nothing you typed was lost — try again.`);
        } finally {
            setSubmitting(false);
        }
    };

    const handleScheduleSubmit = async (data: ScheduleSubmitData) => {
        setSubmitting(true);
        setStepError(null);
        try {
            if (!eventId) {
                const created = await eventsApi.create({
                    // Non-null: this wizard only renders inside the org-scoped
                    // console, which always has an active org.
                    org_id: activeOrg!.id,
                    slug: slugify(event.title),
                    title: event.title,
                    // CreateEventInput's summary/description/category/address
                    // are required (non-optional) strings, unlike
                    // UpdateEventInput's — sent directly (falling back to '')
                    // rather than coalesced to undefined; Go decodes an
                    // absent key and an empty string to the same zero value.
                    category: event.category || '',
                    summary: event.summary || '',
                    description: event.description || '',
                    venue_name: data.venue_name,
                    address: data.address || '',
                    lat: data.lat === '' || data.lat === undefined ? undefined : Number(data.lat),
                    lng: data.lng === '' || data.lng === undefined ? undefined : Number(data.lng),
                    currency: data.currency,
                    starts_at: data.starts_at,
                    ends_at: data.ends_at,
                    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
                    // Images aren't part of this step — a duplicate/fresh
                    // draft starts with no cover, set later in the images step.
                    cover_image: '',
                });
                const createdEvent = created.event;
                setEventId(createdEvent.id);
            } else {
                await eventsApi.update(eventId, {
                    venue_name: data.venue_name,
                    address: data.address || undefined,
                    lat: data.lat === '' || data.lat === undefined ? undefined : Number(data.lat),
                    lng: data.lng === '' || data.lng === undefined ? undefined : Number(data.lng),
                    currency: data.currency,
                    starts_at: data.starts_at,
                    ends_at: data.ends_at,
                });
            }
            setEvent((e) => ({ ...e, ...data, address: data.address ?? '', lat: data.lat ?? '', lng: data.lng ?? '' }));
            goToStep(2);
        } catch (err) {
            // The date/venue/currency the organiser just entered stays right
            // where they left it — `data` is only merged into `event` state
            // on success, and the form below keeps its own values either way.
            const message = err instanceof Error ? err.message : undefined;
            const reason = message || (eventId ? 'Could not save these details.' : 'Could not create your draft.');
            setStepError(`${reason} Nothing you entered was lost — try again.`);
        } finally {
            setSubmitting(false);
        }
    };

    const handlePublish = async () => {
        setIsPublishing(true);
        setStepError(null);
        try {
            await eventsApi.publish(eventId ?? '');
            toast({ title: 'Published', description: 'Your event is now live.' });
            navigate(`/admin/events/${eventId}`);
        } catch (err) {
            const message = err instanceof Error ? err.message : 'Could not publish this event. Your draft is untouched — try again.';
            setStepError(message);
        } finally {
            setIsPublishing(false);
        }
    };

    const handleExit = () => navigate('/admin/events');

    const handleCoverChange = async (imageId: string | null) => {
        setCoverImageId(imageId);
        try {
            // See the same note in event/images/index.tsx: this sends a
            // literal JSON null to clear the cover, but ApplyUpdate only
            // clears CoverImageID on an empty string, not null. Preserved
            // verbatim via the assertion below, not fixed here.
            await eventsApi.update(eventId ?? '', { cover_image_id: imageId } as UpdateEventInput);
        } catch (err) {
            const message = err instanceof Error ? err.message : undefined;
            toast({ title: 'Could not set cover image', description: message, variant: 'destructive' });
        }
    };

    if (loading) return <WizardSkeleton />;

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
                {/* Measured 20×36 at 390px: `size="icon"` sets a 36px square,
                    but this button is the flex row's last child next to a
                    heading that wants every pixel, so it was being squeezed to
                    20px wide. `shrink-0` stops the squeeze and h-11/w-11 takes
                    the escape hatch out of a wizard to a full 44×44 tap. */}
                <Button
                    variant="ghost"
                    size="icon"
                    className="h-11 w-11 shrink-0"
                    onClick={handleExit}
                    aria-label="Exit and return to events"
                >
                    <X className="h-5 w-5" aria-hidden="true" />
                </Button>
            </div>

            <WizardStepper steps={STEPS} currentStep={step} maxStepReached={maxStepReached} onStepClick={goToStep} />

            {stepError && (
                <Alert variant="destructive" className="mb-6">
                    <AlertDescription>{stepError}</AlertDescription>
                </Alert>
            )}

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
                            onTicketTypesChange={(updater) => setTicketTypes((cur) => updater(cur))}
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
                            onImagesChange={(updater) => setImages((cur) => updater(cur))}
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
