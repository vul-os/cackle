import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { toast } from '@/components/ui/use-toast';
import { EventPageHeader } from './header';
import { EventDetailsCard } from './details';
import DeleteEventDialog from './delete-dialog';
import { useEventForm } from './event-form-hook';
import { Spinner } from '@/components/ui/spinner';
import { EmptyState } from '@/components/ui/empty-state';
import { CalendarX } from 'lucide-react';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import { useAuth } from '@/context/use-auth';
import { slugify } from '../slug';
import { setPendingDraft, clearPendingDraft } from '../pending-draft';

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
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [isPublishing, setIsPublishing] = useState(false);
    const [isDuplicating, setIsDuplicating] = useState(false);
    const [showDeleteDialog, setShowDeleteDialog] = useState(false);
    const [isDeleting, setIsDeleting] = useState(false);

    const { editForm, hasChanges, setHasChanges, handleInputChange, initializeForm } = useEventForm();

    useEffect(() => {
        if (!id) return;
        let cancelled = false;
        setLoading(true);
        eventsApi
            .get(id)
            .then((data) => {
                if (cancelled) return;
                const event = data?.event ?? data;
                initializeForm(event);
                setLoading(false);
            })
            .catch(() => {
                if (cancelled) return;
                setNotFound(true);
                setLoading(false);
            });
        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [id]);

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
            // DELETE /api/events/{id} isn't confirmed in the documented API as of
            // this wave — a 404/405 here means the route genuinely doesn't exist
            // yet, not that anything went wrong on the user's end. Say so plainly
            // rather than pretending the delete happened.
            if (err.status === 404 || err.status === 405) {
                toast({
                    title: 'Not available yet',
                    description: 'Deleting events isn’t wired up on this server build. Set status to cancelled instead.',
                });
            } else {
                toast({ title: 'Could not delete', description: err.message, variant: 'destructive' });
            }
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

            // The duplicate is a draft and — like any draft — invisible in the
            // Events list until published (see pending-draft.js); remember it
            // so there's a way back if the organiser navigates away first.
            setPendingDraft(activeOrg.id, newEvent.id);
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
            clearPendingDraft(activeOrg?.id);
            toast({ title: 'Published', description: 'Your event is now live.' });
        } catch (err) {
            toast({ title: 'Could not publish', description: err.message, variant: 'destructive' });
        } finally {
            setIsPublishing(false);
        }
    };

    if (loading) return <Spinner />;
    if (notFound) {
        return (
            <div className="mx-auto max-w-2xl py-8">
                <EmptyState
                    icon={CalendarX}
                    title="Event not found"
                    description="It may have been deleted, or the link is wrong."
                    action={
                        <button type="button" className="text-sm font-medium text-primary underline-offset-4 hover:underline" onClick={() => navigate('/admin/events')}>
                            Back to events
                        </button>
                    }
                />
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
