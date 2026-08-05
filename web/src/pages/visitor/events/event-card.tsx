import React from 'react';
import { Link } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Calendar, MapPin } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { getCoverImageUrl, type EventMediaSource } from './media';
import { Money } from '@/components/ui/money';
import type { CackleEvent } from '@/lib/api-types';
import type { HostOrgRef } from '@/lib/host';

function formatDate(iso: string | null | undefined): string {
    if (!iso) return 'Date TBA';
    try {
        return new Date(iso).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
        return 'Date TBA';
    }
}

/** The fields this card reads off an event: the public DTO plus the legacy/joined media shape (see media.ts). */
export type EventCardEvent = CackleEvent & Pick<EventMediaSource, 'gallery'>;

/** Best-effort per-card pricing — see use-event-pricing.ts. */
export interface EventCardPricing {
    minPriceMinor: number | null;
    soldOut: boolean;
}

export interface EventCardProps {
    event: EventCardEvent;
    org?: HostOrgRef | null | undefined;
    pricing?: EventCardPricing | null | undefined;
    index?: number | undefined;
    featured?: boolean | undefined;
}

/**
 * The one event-preview card used across landing (featured/upcoming) and
 * browse — same visual language everywhere a visitor scans a list of
 * events. `pricing` is optional best-effort per-card data ({ minPriceMinor,
 * soldOut }); omit it where the extra per-card lookup isn't worth it and the
 * card just won't show a price badge.
 *
 * `org` is optional and comes from `host.organisations` on GET /api/events,
 * resolved by `orgForEvent()` in @/lib/host. Callers pass it ONLY on a box
 * that hosts more than one organisation; on a single venue it is null and no
 * organisation chrome renders, because a single venue must not be dressed up
 * as a directory. The card never fetches it and never guesses it — a null org
 * means "say nothing".
 *
 * The organisation is a NAME here, not a link: the whole card is already an
 * <a> (the <Link> below), and nesting an anchor inside an anchor is invalid
 * HTML. The link-through to an organisation's own listing lives beside the
 * page heading instead.
 */
const EventCard = ({ event, org, pricing, index = 0, featured = false }: EventCardProps) => {
    const coverUrl = getCoverImageUrl(event);
    const price = pricing?.minPriceMinor;
    const showPrice = !pricing?.soldOut && price !== undefined && price !== null;
    const showCornerChrome = Boolean(event.category) || showPrice;
    // `org.name` arrives over the wire as `unknown` (see HostOrgRef in
    // @/lib/host) — narrowed here the same way the rest of that module
    // narrows the host envelope, rather than assumed.
    const orgName = typeof org?.name === 'string' ? org.name : null;

    return (
        <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.3, delay: Math.min(index * 0.04, 0.4) }}
            className="h-full"
        >
            <Link to={`/events/${event.slug}`} className="group block h-full" data-testid="event-card">
                <Card className="h-full overflow-hidden transition-all duration-300 hover:-translate-y-1 hover:shadow-xl">
                    <div className={`relative overflow-hidden bg-muted ${featured ? 'aspect-[4/3] sm:aspect-[16/10]' : 'aspect-[16/9]'}`}>
                        {coverUrl ? (
                            <img
                                src={coverUrl}
                                alt={event.title}
                                className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
                                loading="lazy"
                            />
                        ) : (
                            <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-primary/20 to-primary/5">
                                <Calendar className="h-10 w-10 text-primary-emphasis/50" aria-hidden="true" />
                            </div>
                        )}
                        {showCornerChrome && (
                            // Demo covers range from a flat product shot to a
                            // saturated neon sign or a magenta-lit concert —
                            // an arbitrary photo, not a theme surface. A pill
                            // that only leans on a badge/brand-fill colour
                            // (which flips with theme, or can be the same hue
                            // as the sign behind it) has no guaranteed
                            // contrast. This gradient plus the media-ink/
                            // media-ground glass pills below are the same
                            // fixed-in-both-themes on-photo chrome as
                            // header.tsx and gallery.tsx, so the corner
                            // labels stay legible regardless of what's under
                            // them.
                            <div
                                className="pointer-events-none absolute inset-x-0 top-0 h-16 bg-gradient-to-b from-media-ground/70 to-transparent"
                                aria-hidden="true"
                            />
                        )}
                        {event.category && (
                            <span className="absolute left-3 top-3 rounded-full bg-media-ground/40 px-2.5 py-0.5 text-xs font-semibold capitalize text-media-ink backdrop-blur-sm">
                                {event.category}
                            </span>
                        )}
                        {pricing?.soldOut && (
                            // `media-ground` (index.css `--on-media-ground`),
                            // not `bg-black`: this darkens an arbitrary cover
                            // PHOTO for legibility, not an app surface, so it
                            // is fixed in both themes the same way the
                            // gallery/header on-photo chrome is. The "Sold
                            // out" pill itself is fully themed (`bg-background`,
                            // inherited text colour).
                            <div className="absolute inset-0 flex items-center justify-center bg-media-ground/50">
                                <span className="rounded-full bg-background px-4 py-1.5 text-sm font-semibold">Sold out</span>
                            </div>
                        )}
                        {showPrice && (
                            <div className="absolute right-3 top-3 rounded-full bg-media-ground/40 px-3 py-1 text-xs font-semibold text-media-ink backdrop-blur-sm">
                                {price === 0 ? (
                                    'Free'
                                ) : (
                                    <>
                                        From <Money minor={price as number} currency={event.currency} />
                                    </>
                                )}
                            </div>
                        )}
                    </div>
                    <CardContent className={`space-y-2 ${featured ? 'p-6' : 'p-5'}`}>
                        <h3 className={`font-display font-bold leading-snug tracking-tight group-hover:text-primary-emphasis ${featured ? 'text-xl' : 'text-lg'}`}>
                            {event.title}
                        </h3>
                        {orgName && (
                            <p className="text-sm text-muted-foreground">
                                <span className="sr-only">Organised by </span>
                                {orgName}
                            </p>
                        )}
                        <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Calendar className="h-4 w-4 shrink-0" aria-hidden="true" />
                            <span>{formatDate(event.starts_at)}</span>
                        </div>
                        {event.venue_name && (
                            <div className="flex items-center gap-2 text-sm text-muted-foreground">
                                <MapPin className="h-4 w-4 shrink-0" aria-hidden="true" />
                                <span className="truncate">{event.venue_name}</span>
                            </div>
                        )}
                    </CardContent>
                </Card>
            </Link>
        </motion.div>
    );
};

export default EventCard;
