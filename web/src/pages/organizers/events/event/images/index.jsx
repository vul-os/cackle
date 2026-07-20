import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { ArrowLeft, ImageIcon } from 'lucide-react';
import { Spinner } from '@/components/ui/spinner';
import { ErrorState } from '@/components/ui/error-state';
import { toast } from '@/components/ui/use-toast';
import { events as eventsApi } from '@/lib/api';
import ImageUploader from '../image-uploader';

const EventImagesPage = () => {
    const { id: eventId } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState({ event: null, loading: true, error: null });
    const [images, setImages] = useState([]);

    const load = useCallback(async () => {
        setState((s) => ({ ...s, loading: true, error: null }));
        try {
            const data = await eventsApi.get(eventId);
            const event = data?.event ?? data;
            setState({ event, loading: false, error: null });
            // `gallery` is a sibling of `event` in the GET /api/events/{id}
            // response shape, not a field on the event object itself.
            setImages(data?.gallery ?? []);
        } catch (err) {
            setState({ event: null, loading: false, error: err.message || 'Could not load this event.' });
        }
    }, [eventId]);

    useEffect(() => {
        load();
    }, [load]);

    const handleCoverChange = async (imageId) => {
        setState((s) => (s.event ? { ...s, event: { ...s.event, cover_image_id: imageId } } : s));
        try {
            await eventsApi.update(eventId, { cover_image_id: imageId });
            toast({ title: 'Cover image updated' });
        } catch (err) {
            toast({ title: 'Could not set cover image', description: err.message, variant: 'destructive' });
        }
    };

    if (state.loading) return <Spinner />;

    if (state.error) {
        return (
            <div className="mx-auto max-w-3xl py-8">
                <ErrorState description={state.error} onRetry={load} />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-3xl">
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${eventId}`)} className="mb-6">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <Card>
                <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                        <ImageIcon className="h-5 w-5 text-primary" />
                        {state.event?.title ?? 'Images'}
                    </CardTitle>
                    <CardDescription>
                        Upload a cover image and gallery shots. The cover image is what buyers see first on your listing and in
                        search results.
                    </CardDescription>
                </CardHeader>
                <CardContent>
                    <ImageUploader
                        eventId={eventId}
                        images={images}
                        coverImageId={state.event?.cover_image_id}
                        onImagesChange={(updater) => setImages((current) => (typeof updater === 'function' ? updater(current) : updater))}
                        onCoverChange={handleCoverChange}
                    />
                </CardContent>
            </Card>
        </div>
    );
};

export default EventImagesPage;
