import React from 'react';
import { Share2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/use-toast';

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
        <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/90 via-black/50 to-transparent p-8 sm:p-12">
            <div className="mx-auto max-w-5xl">
                <div className="flex flex-wrap items-center gap-2">
                    {category && (
                        <span className="rounded-full bg-white/15 px-4 py-1.5 text-sm font-semibold capitalize text-white backdrop-blur-md">
                            {category}
                        </span>
                    )}
                    {venueName && (
                        <span className="rounded-full bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground">{venueName}</span>
                    )}
                    <div className="ml-auto">
                        <Button
                            variant="outline"
                            className="border-white/20 bg-white/10 text-white backdrop-blur-md hover:bg-white/20 hover:text-white"
                            onClick={handleShare}
                        >
                            <Share2 className="mr-2 h-4 w-4" />
                            Share
                        </Button>
                    </div>
                </div>
                <h1 className="mt-6 font-display text-4xl font-black tracking-tight text-white drop-shadow-lg sm:text-6xl">{title}</h1>
            </div>
        </div>
    );
};

export default EventHeader;
