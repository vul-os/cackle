import React from 'react';
import { format } from 'date-fns';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ArrowLeft, Globe, Loader2, MapPin, Calendar, Tag, Ticket } from 'lucide-react';
import { images as imagesApi } from '@/lib/api';
import { categoryLabel } from '@/pages/organizers/events/categories';
import { formatMoney } from '@/lib/money';

const Row = ({ icon: Icon, children }) => (
    <div className="flex items-start gap-2 text-sm">
        <Icon className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <span>{children}</span>
    </div>
);

const ReviewStep = ({ event, ticketTypes, coverImageId, onBack, onPublish, onSaveDraft, isPublishing }) => {
    const coverUrl = coverImageId ? imagesApi.url(coverImageId) : null;
    const canPublish = ticketTypes.length > 0;

    return (
        <div className="space-y-6">
            {coverUrl ? (
                <div className="aspect-[21/9] w-full overflow-hidden rounded-lg bg-muted">
                    <img src={coverUrl} alt="Cover preview" className="h-full w-full object-cover" />
                </div>
            ) : (
                <div className="flex aspect-[21/9] w-full items-center justify-center rounded-lg border border-dashed border-border bg-muted/40 text-sm text-muted-foreground">
                    No cover image
                </div>
            )}

            <div>
                <h2 className="font-display text-2xl font-bold">{event.title || 'Untitled event'}</h2>
                {event.summary && <p className="mt-1 text-muted-foreground">{event.summary}</p>}
            </div>

            <div className="grid grid-cols-1 gap-3 rounded-lg border border-border p-4 sm:grid-cols-2">
                {event.starts_at && (
                    <Row icon={Calendar}>
                        {format(new Date(event.starts_at), 'PPP p')}
                        {event.ends_at ? ` – ${format(new Date(event.ends_at), 'PPP p')}` : ''}
                    </Row>
                )}
                {event.venue_name && (
                    <Row icon={MapPin}>
                        {event.venue_name}
                        {event.address ? `, ${event.address}` : ''}
                    </Row>
                )}
                {event.category && <Row icon={Tag}>{categoryLabel(event.category)}</Row>}
                <Row icon={Ticket}>
                    {ticketTypes.length === 0
                        ? 'No ticket types yet'
                        : `${ticketTypes.length} ticket type${ticketTypes.length === 1 ? '' : 's'}`}
                </Row>
            </div>

            {ticketTypes.length > 0 && (
                <div className="space-y-2">
                    <h3 className="text-sm font-medium">Ticket types</h3>
                    <div className="divide-y divide-border rounded-lg border border-border">
                        {ticketTypes.map((tt) => (
                            <div key={tt.id} className="flex items-center justify-between p-3 text-sm">
                                <span>{tt.name}</span>
                                <span className="tabular-nums text-muted-foreground">
                                    {formatMoney(tt.price_minor, event.currency)} · {tt.quantity_total ?? 0} available
                                </span>
                            </div>
                        ))}
                    </div>
                </div>
            )}

            {!canPublish && (
                <div className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-400">
                    <Badge variant="outline" className="border-amber-500/40 text-amber-700 dark:text-amber-400">
                        Missing
                    </Badge>
                    Add a ticket type before you publish.
                </div>
            )}

            <div className="flex flex-col-reverse justify-between gap-3 pt-2 sm:flex-row">
                <Button type="button" variant="outline" onClick={onBack} disabled={isPublishing}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    Back
                </Button>
                <div className="flex flex-col-reverse gap-2 sm:flex-row">
                    <Button type="button" variant="outline" onClick={onSaveDraft} disabled={isPublishing}>
                        Save as draft, finish later
                    </Button>
                    <Button type="button" onClick={onPublish} disabled={isPublishing || !canPublish}>
                        {isPublishing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Globe className="mr-2 h-4 w-4" />}
                        Publish event
                    </Button>
                </div>
            </div>
        </div>
    );
};

export default ReviewStep;
