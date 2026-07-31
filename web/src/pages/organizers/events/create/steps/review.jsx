import React from 'react';
import { format } from 'date-fns';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Separator } from '@/components/ui/separator';
import { ArrowLeft, Globe, Loader2, MapPin, Calendar, Tag, Ticket, Coins, AlertTriangle } from 'lucide-react';
import { images as imagesApi } from '@/lib/api';
import { categoryLabel } from '@/pages/organizers/events/categories';
import { Money } from '@/components/ui/money';

const Row = ({ icon: Icon, children }) => (
    <div className="flex items-start gap-2 text-sm">
        <Icon className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <span>{children}</span>
    </div>
);

/** "Sales open in 3 days" / "Sales close 12 Aug" / "Sales closed" — the same
 * plain-language framing as the organiser's ticket-type list, so a ticket's
 * window reads the same way here as it will everywhere else it's shown. */
function saleWindowLabel(tt) {
    if (!tt.sales_start || !tt.sales_end) return null;
    const now = Date.now();
    const start = new Date(tt.sales_start).getTime();
    const end = new Date(tt.sales_end).getTime();
    if (now < start) return `Sales open ${format(new Date(tt.sales_start), 'd MMM')}`;
    if (now > end) return 'Sales closed';
    return `Sales close ${format(new Date(tt.sales_end), 'd MMM')}`;
}

const ReviewStep = ({ event, ticketTypes, coverImageId, onBack, onPublish, onSaveDraft, isPublishing }) => {
    const coverUrl = coverImageId ? imagesApi.url(coverImageId) : null;
    const canPublish = ticketTypes.length > 0;

    return (
        <div className="space-y-6">
            <div>
                <h2 className="font-display text-xl font-bold tracking-tight">Review before you publish</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                    This is what goes live — check it the way a buyer would read it, top to bottom.
                </p>
            </div>

            {coverUrl ? (
                <div className="aspect-[21/9] w-full overflow-hidden rounded-lg bg-muted">
                    <img src={coverUrl} alt="Cover preview" className="h-full w-full object-cover" />
                </div>
            ) : (
                <div className="flex aspect-[21/9] w-full items-center justify-center rounded-lg border border-dashed border-border bg-muted/40 text-sm text-muted-foreground">
                    No cover image — optional, but events with one sell noticeably better
                </div>
            )}

            <div>
                <h3 className="font-display text-2xl font-bold">{event.title || 'Untitled event'}</h3>
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
                {event.currency && <Row icon={Coins}>Priced in {event.currency}</Row>}
                <Row icon={Ticket}>
                    {ticketTypes.length === 0
                        ? 'No ticket types yet'
                        : `${ticketTypes.length} ticket type${ticketTypes.length === 1 ? '' : 's'}`}
                </Row>
            </div>

            {ticketTypes.length > 0 && (
                <div className="space-y-3">
                    {/* The perforation motif earns its place here: this is the
                        literal seam between "what the event is" above and
                        "what someone actually buys" below — the same divide
                        a paper ticket stub marks with a tear line. `--notch`
                        is set to the surrounding card surface, not the page,
                        per the component's own doc comment (it defaults to
                        `--background`, which is wrong nested inside a card). */}
                    <div style={{ '--notch': 'var(--card)' }}>
                        <Separator variant="perforated" />
                    </div>

                    <h4 className="text-sm font-semibold text-foreground">What buyers can purchase</h4>
                    <div className="divide-y divide-border rounded-lg border border-border">
                        {ticketTypes.map((tt) => {
                            const windowLabel = saleWindowLabel(tt);
                            return (
                                <div key={tt.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 p-3 text-sm">
                                    <div className="min-w-0">
                                        <p className="font-medium text-foreground">{tt.name}</p>
                                        <p className="text-xs text-muted-foreground">
                                            Max {tt.max_per_order ?? 10} per order
                                            {windowLabel ? ` · ${windowLabel}` : ''}
                                        </p>
                                    </div>
                                    <div className="text-right">
                                        <Money minor={tt.price_minor} currency={event.currency} className="font-medium tabular-nums" />
                                        <p className="text-xs text-muted-foreground">{tt.quantity_total ?? 0} available</p>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>
            )}

            {!canPublish && (
                <Alert variant="warning">
                    <AlertTriangle className="h-4 w-4" />
                    <AlertDescription>
                        <Badge variant="warning" className="mr-2 align-middle">
                            Missing
                        </Badge>
                        Add a ticket type before you publish — there's nothing for a buyer to purchase yet.
                    </AlertDescription>
                </Alert>
            )}

            <div className="rounded-lg border border-border bg-muted/30 p-4 text-sm leading-relaxed text-muted-foreground">
                Publishing puts <span className="font-medium text-foreground">{event.title || 'this event'}</span> on your
                storefront and opens ticket sales immediately. Everything above — details, ticket types, images — stays
                editable from the event page afterwards, so this isn&apos;t a one-shot form; it&apos;s the switch that makes
                it public.
            </div>

            <div className="flex flex-col-reverse justify-between gap-3 pt-2 sm:flex-row">
                <Button type="button" variant="outline" size="lg" onClick={onBack} disabled={isPublishing}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    Back
                </Button>
                <div className="flex flex-col-reverse gap-2 sm:flex-row">
                    <Button type="button" variant="outline" size="lg" onClick={onSaveDraft} disabled={isPublishing}>
                        Save as draft, finish later
                    </Button>
                    <Button type="button" size="lg" onClick={onPublish} disabled={isPublishing || !canPublish}>
                        {isPublishing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Globe className="mr-2 h-4 w-4" />}
                        {isPublishing ? 'Publishing…' : 'Publish event'}
                    </Button>
                </div>
            </div>
        </div>
    );
};

export default ReviewStep;
