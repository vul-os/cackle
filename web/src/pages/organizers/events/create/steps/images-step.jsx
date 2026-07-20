import React from 'react';
import { Button } from '@/components/ui/button';
import { ArrowLeft, ArrowRight } from 'lucide-react';
import ImageUploader from '@/pages/organizers/events/event/image-uploader';

const ImagesStep = ({ eventId, images, coverImageId, onImagesChange, onCoverChange, onBack, onSubmit, submitting }) => {
    return (
        <div className="space-y-6">
            <p className="text-sm text-muted-foreground">
                Optional, but events with a cover image sell noticeably better. You can always add more later.
            </p>

            <ImageUploader
                eventId={eventId}
                images={images}
                coverImageId={coverImageId}
                onImagesChange={onImagesChange}
                onCoverChange={onCoverChange}
            />

            <div className="flex justify-between pt-2">
                <Button type="button" variant="outline" onClick={onBack} disabled={submitting}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    Back
                </Button>
                <Button type="button" onClick={onSubmit} disabled={submitting}>
                    Continue
                    <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
            </div>
        </div>
    );
};

export default ImagesStep;
