import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { toast } from '@/components/ui/use-toast';
import { EventPageHeader } from './header';
import { EventDetailsCard } from './details';
import DeleteEventDialog from './delete-dialog';
import { useEventForm } from './event-form-hook';
import { Skeleton } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { CalendarX } from 'lucide-react';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import { useAuth } from '@/context/use-auth';
import { slugify } from '../slug';

/**
 * Shape of the editor while the event is loading — mirrors the real layout
 * (back link, title row, nav-button row, the details card's header and its
 * first section) so the page doesn't jump around once data arrives, and so
 * "loading" reads as this specific page rather than a generic blank spinner.
 */
const EventEditorSkeleton = () => (
    <div className="mx-auto max-w-4xl" role="status" aria-label="Loading event">
        <div className="mb-8 space-y-4">
            <Skeleton className="h-9 w-36" />
            <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                <Skeleton className="h-10 w-72" />
                <div className="flex flex-wrap gap-2">
                    {Array.from({ length: 6 }).map((_, i) => (
                        <Skeleton key={i} className="h-9 w-28" />
                    ))}
                </div>
            </div>
        </div>
        <div className="rounded-xl border border-border">
            <div className="flex items-center justify-between border-b border-border p-4 sm:p-6">
                <Skeleton className="h-6 w-36" />
                <div className="flex gap-2">
                    <Skeleton className="h-9 w-20" />
                    <Skeleton className="h-9 w-9" />
                </div>
            </div>
            <div className="space-y-4 p-4 sm:p-6">
                <Skeleton className="aspect-[21/9] w-full" />
                <Skeleton className="h-9 w-40" />
            </div>
        </div>
    </div>
);

function toApiPayload(form) {
    return {
        title: form.title,
        summary: form.summary || undefined,
        description: form.description || undefined,
        venue_name: form.venue_name || undefined,
        address: form.address || undefined,
        lat: form.lat === '' ? undefined : Number(form.lat),
        lng: form.lng === '' ? undefined : Number(form.lng),
        starts_at: form.starts_at || undefined,
        ends_at: form.ends_at || undefined,
        timezone: form.timezone || undefined,
        cover_image: form.cover_image || undefined,
        category: form.category || undefined,
        currency: form.currency || undefined,
    };
}

const EventPage = () => {
    const { id } = useParams();
    const navigate = useNavigate();
    const { activeOrg } = useAuth();
    const [loading, setLoading] = useState(true);
    const [notFound, setNotFound] = useState(false);
    const [loadError, setLoadError] = useState(null);
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [isPublishing, setIsPublishing] = useState(false);
    const [isDuplicating, setIsDuplicating] = useState(false);
    const [showDeleteDialog, setShowDeleteDialog] = useState(false);
    const [isDeleting, setIsDeleting] = useState(false);

    const { editForm, hasChanges, setHasChanges, handleInputChange, initializeForm } = useEventForm();

    // Split out so the error state's "Try again" button can call the exact
    // same fetch rather than a copy of it drifting out of sync.
    const load = useCallback(() => {
        if (!id) return undefined;
        let cancelled = false;
        setLoading(true);
        setNotFound(false);
        setLoadError(null);
        eventsApi
            .get(id)
            .then((data) => {
                if (cancelled) return;
                const event = data?.event ?? data;
                initializeForm(event);
                setLoading(false);
            })
            .catch((err) => {
                if (cancelled) return;
                // A 404 means the event genuinely doesn't exist (deleted, bad
                // link) — that's an empty state, not an error. Everything else
                // (network blip, 5xx) is transient and deserves a retry rather
                // than being told the event is gone.
                if (err?.status === 404) {
                    setNotFound(true);
                } else {
                    setLoadError(err?.message || 'Could not load this event.');
                }
                setLoading(false);
            });
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [id]);

    useEffect(() => {
        const cleanup = load();
        return cleanup;
    }, [load]);

    const handleSave = async () => {
        setIsSubmitting(true);
        try {
            await eventsApi.update(id, toApiPayload(editForm));
            setHasChanges(false);
            toast({ title: 'Saved', description: 'Event updated successfully.' });
        } catch (err) {
            toast({ title: 'Could not save', description: err.message, variant: 'destructive' });
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleDelete = async () => {
        setIsDeleting(true);
        try {
            await eventsApi.remove(id);
            toast({ title: 'Deleted', description: 'The event has been removed.' });
            navigate('/admin/events');
        } catch (err) {
            // 409 conflict: the event has issued tickets — the server steers
            // toward cancelling instead (see docs/API.md). Its message
            // already says so; just surface it rather than a generic one.
            toast({ title: 'Could not delete', description: err.message, variant: 'destructive' });
        } finally {
            setIsDeleting(false);
            setShowDeleteDialog(false);
        }
    };

    const handleDuplicate = async () => {
        setIsDuplicating(true);
        try {
            const [ttData] = await Promise.all([ticketTypesApi.list(id)]);
            const sourceTicketTypes = Array.isArray(ttData) ? ttData : (ttData?.ticket_types ?? []);

            // Create requires starts_at/ends_at and a unique slug (see
            // internal/events.Service.Create) — a duplicate starts on the same
            // date/venue as the source event, which the organiser can then
            // change from the normal editor. Images aren't copied: stored image
            // files belong to the source event.
            const created = await eventsApi.create({
                org_id: activeOrg.id,
                slug: slugify(editForm.title),
                title: `${editForm.title || 'Untitled event'} (Copy)`,
                summary: editForm.summary || undefined,
                description: editForm.description || undefined,
                venue_name: editForm.venue_name || undefined,
                address: editForm.address || undefined,
                lat: editForm.lat === '' ? undefined : Number(editForm.lat),
                lng: editForm.lng === '' ? undefined : Number(editForm.lng),
                starts_at: editForm.starts_at || undefined,
                ends_at: editForm.ends_at || undefined,
                timezone: editForm.timezone || undefined,
                category: editForm.category || undefined,
                currency: editForm.currency || undefined,
            });
            const newEvent = created?.event ?? created;

            await Promise.all(
                sourceTicketTypes.map((tt) =>
                    ticketTypesApi.create(newEvent.id, {
                        name: tt.name,
                        description: tt.description || undefined,
                        price_minor: tt.price_minor,
                        quantity_total: tt.quantity_total,
                        max_per_order: tt.max_per_order,
                        sales_start: tt.sales_start,
                        sales_end: tt.sales_end,
                        sort_order: tt.sort_order,
                    }),
                ),
            );

            toast({ title: 'Duplicated', description: 'A new draft was created with the same details and ticket types.' });
            navigate(`/admin/events/${newEvent.id}`);
        } catch (err) {
            toast({ title: 'Could not duplicate', description: err.message, variant: 'destructive' });
        } finally {
            setIsDuplicating(false);
        }
    };

    const handlePublish = async () => {
        setIsPublishing(true);
        try {
            await eventsApi.publish(id);
            handleInputChange('status', 'published');
            toast({ title: 'Published', description: 'Your event is now live.' });
        } catch (err) {
            toast({ title: 'Could not publish', description: err.message, variant: 'destructive' });
        } finally {
            setIsPublishing(false);
        }
    };

    if (loading) return <EventEditorSkeleton />;
    if (notFound) {
        return (
            <div className="mx-auto max-w-2xl py-8">
                <EmptyState
                    icon={CalendarX}
                    title="Event not found"
                    description="It may have been deleted, or the link is wrong."
                    action={
                        <button type="button" className="text-sm font-medium text-primary-emphasis underline-offset-4 hover:underline" onClick={() => navigate('/admin/events')}>
                            Back to events
                        </button>
                    }
                />
            </div>
        );
    }
    if (loadError) {
        return (
            <div className="mx-auto max-w-2xl py-8">
                <ErrorState description={loadError} onRetry={load} />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-4xl">
            <EventPageHeader
                editForm={editForm}
                handleInputChange={handleInputChange}
                navigate={navigate}
                isSubmitting={isSubmitting}
                onPublish={handlePublish}
                isPublishing={isPublishing}
                onDuplicate={handleDuplicate}
                isDuplicating={isDuplicating}
            />

            <EventDetailsCard
                editForm={editForm}
                handleInputChange={handleInputChange}
                isSubmitting={isSubmitting}
                hasChanges={hasChanges}
                onSave={handleSave}
                onDeleteRequest={() => setShowDeleteDialog(true)}
            />

            <DeleteEventDialog
                open={showDeleteDialog}
                onOpenChange={setShowDeleteDialog}
                eventTitle={editForm.title}
                onConfirm={handleDelete}
                isDeleting={isDeleting}
            />
        </div>
    );
};

export default EventPage;
