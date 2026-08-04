import React from 'react';
import { Share2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/use-toast';

// This sits on top of an arbitrary cover PHOTO, not the app's own
// background, so it uses the `media-ink` / `media-ground` tokens
// (index.css `--on-media` / `--on-media-ground`) rather than
// `text-white` / `bg-black`: fixed in both themes on purpose, because the
// photo underneath does not change with the OS theme either — flipping to
// `text-foreground`/`bg-background` could land ink-on-photo in light mode.
// Chrome that sits on the actual page (the venue badge, `bg-primary`) uses
// the ordinary theme tokens; only the on-photo layer reaches for `media-*`.
const EventHeader = ({ title, venueName, category }) => {
    const handleShare = async () => {
        const shareData = { title, url: window.location.href };
        try {
            if (navigator.share) {
                await navigator.share(shareData);
            } else {
                await navigator.clipboard.writeText(window.location.href);
                toast({ title: 'Link copied', description: 'Event link copied to your clipboard.' });
            }
        } catch {
            // user cancelled the share sheet — nothing to do
        }
    };

    return (
        <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-media-ground/90 via-media-ground/50 to-transparent p-8 sm:p-12">
            <div className="mx-auto max-w-5xl">
                <div className="flex flex-wrap items-center gap-2">
                    {category && (
                        <span className="rounded-full bg-media-ink/15 px-4 py-1.5 text-sm font-semibold capitalize text-media-ink backdrop-blur-md">
                            {category}
                        </span>
                    )}
                    {venueName && (
                        <span className="rounded-full bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground">{venueName}</span>
                    )}
                    <div className="ml-auto">
                        <Button
                            variant="outline"
                            className="border-media-ink/20 bg-media-ink/10 text-media-ink backdrop-blur-md hover:bg-media-ink/20 hover:text-media-ink"
                            onClick={handleShare}
                        >
                            <Share2 className="mr-2 h-4 w-4" />
                            Share
                        </Button>
                    </div>
                </div>
                <h1 className="mt-6 font-display text-4xl font-black tracking-tight text-media-ink drop-shadow-lg sm:text-6xl">{title}</h1>
            </div>
        </div>
    );
};

export default EventHeader;
