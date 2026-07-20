import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { toast } from '@/components/ui/use-toast';
import { EventPageHeader } from './header';
import { EventDetailsCard } from './details';
import { useEventForm } from './event-form-hook';
import { Spinner } from '@/components/ui/spinner';
import { events as eventsApi } from '@/lib/api';

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
        currency: form.currency || undefined,
    };
}

const EventPage = () => {
    const { id } = useParams();
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [notFound, setNotFound] = useState(false);
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [isPublishing, setIsPublishing] = useState(false);

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
        setIsSubmitting(true);
        try {
            // No DELETE /api/events/:id in the documented API — publishing status
            // (draft/cancelled) is the supported lifecycle. We surface this
            // honestly instead of pretending a delete happened.
            toast({
                title: 'Not available yet',
                description: 'Deleting events isn’t wired up in this build. Set status to cancelled from ticket sales instead.',
            });
        } finally {
            setIsSubmitting(false);
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

    if (loading) return <Spinner />;
    if (notFound) {
        return (
            <div className="mx-auto max-w-2xl py-16 text-center">
                <p className="text-lg font-medium">Event not found</p>
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
            />

            <EventDetailsCard
                editForm={editForm}
                handleInputChange={handleInputChange}
                isSubmitting={isSubmitting}
                hasChanges={hasChanges}
                onSave={handleSave}
                onDelete={handleDelete}
            />
        </div>
    );
};

export default EventPage;
