import React from 'react';
import { Link } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Calendar, MapPin } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { getCoverImageUrl } from './media';
import { formatMoney } from './ticket-utils';

function formatDate(iso) {
    if (!iso) return 'Date TBA';
    try {
        return new Date(iso).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
        return 'Date TBA';
    }
}

/**
 * The one event-preview card used across landing (featured/upcoming) and
 * browse — same visual language everywhere a visitor scans a list of
 * events. `pricing` is optional best-effort per-card data ({ minPriceMinor,
 * soldOut }); omit it where the extra per-card lookup isn't worth it and the
 * card just won't show a price badge.
 */
const EventCard = ({ event, pricing, index = 0, featured = false }) => {
    const coverUrl = getCoverImageUrl(event);
    const price = pricing?.minPriceMinor;

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
                                <Calendar className="h-10 w-10 text-primary/50" aria-hidden="true" />
                            </div>
                        )}
                        {event.category && (
                            <Badge variant="secondary" className="absolute left-3 top-3 capitalize shadow">
                                {event.category}
                            </Badge>
                        )}
                        {pricing?.soldOut && (
                            <div className="absolute inset-0 flex items-center justify-center bg-black/50">
                                <span className="rounded-full bg-background px-4 py-1.5 text-sm font-semibold">Sold out</span>
                            </div>
                        )}
                        {!pricing?.soldOut && price !== undefined && price !== null && (
                            <div className="absolute right-3 top-3 rounded-full bg-primary px-3 py-1 text-xs font-semibold text-primary-foreground shadow">
                                {price === 0 ? 'Free' : `From ${formatMoney(price, event.currency)}`}
                            </div>
                        )}
                    </div>
                    <CardContent className={`space-y-2 ${featured ? 'p-6' : 'p-5'}`}>
                        <h3 className={`font-display font-bold leading-snug tracking-tight group-hover:text-primary ${featured ? 'text-xl' : 'text-lg'}`}>
                            {event.title}
                        </h3>
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
