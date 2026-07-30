// Borrowed listings — events belonging to another organiser, shown here
// because THIS host's operator enrolled that organiser by hand, by key.
//
// # The one rule this file exists to hold
//
// A borrowed listing must never look like it is sold by this host. It is not.
// A ticket for somebody else's event is issued by somebody else's key on
// somebody else's box; this node cannot issue one, cannot hold the money for
// one and cannot refund one. So there is no price, no ticket picker, no cart
// and no checkout anywhere below — and there never will be. The API shape
// carries no price field either (internal/httpapi/peer_feed.go), and a Go test
// asserts that, so this is a floor rather than a convention.
//
// The only action a borrowed listing offers is a link OUT, to the publisher's
// own page on the publisher's own box, which is where its tickets are sold.
//
// Everything about the presentation is chosen so a person scanning the page
// can tell "this one is somebody else's" WITHOUT reading: the section sits
// below this host's own events behind a tear line and never interleaves with
// them, and each listing is a dashed, image-less, off-ground card rather than
// the solid photo-topped card a local event gets. Interleaving is precisely
// what would make a borrowed listing read as stock.
//
// `external`, `publisher` and `notice` travel IN the payload rather than
// living here, deliberately, so a redesign cannot drop the attribution. The
// notice is rendered verbatim, never paraphrased, so there is one wording for
// this across the whole product.
//
// # What this must not imply
//
// Cackle finds no peers on its own. There is no directory, no index, no
// rendezvous, no geo lookup and no discovery of any kind — in the code or in
// the spec. Every organiser here was typed in by this box's operator. So the
// section renders ONLY when there is at least one borrowed listing to show:
// no empty state, no "no other organisers yet", nothing at all. A visitor to a
// box with no peers must not learn that peers are a thing that could appear.
import React, { useEffect, useState } from 'react';
import { Calendar, ExternalLink, MapPin } from 'lucide-react';
import { request } from '@/lib/api';
import { Separator } from '@/components/ui/separator';
import { peerOrgId, usablePeerRows } from './peer-scope';

function formatDate(iso) {
    if (!iso) return 'Date TBA';
    try {
        return new Date(iso).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
        return 'Date TBA';
    }
}

function formatChecked(iso) {
    if (!iso) return null;
    try {
        return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
        return null;
    }
}

/**
 * The cache for one organisation. Never fires a network call to a peer — this
 * reads THIS node's local cache, which is only ever filled when the operator
 * pressed "fetch now" against an organiser they enrolled. Nothing polls, so
 * what a visitor sees is whatever the last operator-triggered pull brought
 * back; that is what `fetched_at` is for, and why it is on screen.
 *
 * A failure here renders nothing at all. A borrowed listing is a courtesy, not
 * the page — an error fetching one must never take out this host's own events.
 */
function usePeerEvents(orgId) {
    const [state, setState] = useState({ events: [], caveat: '' });

    useEffect(() => {
        if (!orgId) {
            setState({ events: [], caveat: '' });
            return undefined;
        }
        let cancelled = false;
        request('/peer-events', { method: 'GET', query: { org: orgId } })
            .then((data) => {
                if (cancelled) return;
                setState({ events: usablePeerRows(data), caveat: data?.caveat || '' });
            })
            .catch(() => {
                if (!cancelled) setState({ events: [], caveat: '' });
            });
        return () => {
            cancelled = true;
        };
    }, [orgId]);

    return state;
}

/** One borrowed listing. No image, no price, no cart — a dashed note with a way out. */
function PeerEventCard({ event }) {
    const checked = formatChecked(event.fetched_at);
    return (
        <article
            data-testid="peer-event-card"
            className="flex h-full flex-col rounded-2xl border-2 border-dashed border-border bg-muted/40 p-5"
        >
            <p className="inline-flex w-fit items-center gap-1.5 rounded-full border border-border bg-background px-2.5 py-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                Another organiser
            </p>

            <h3 className="mt-3 font-display text-lg font-bold leading-snug tracking-tight">{event.title}</h3>

            <p className="mt-1 text-sm text-muted-foreground">
                <span className="sr-only">Published by </span>
                {event.publisher || 'An organiser this host added'}
            </p>

            <div className="mt-3 space-y-1.5 text-sm text-muted-foreground">
                <div className="flex items-center gap-2">
                    <Calendar className="h-4 w-4 shrink-0" aria-hidden="true" />
                    <span>{formatDate(event.starts_at)}</span>
                </div>
                {event.venue_name && (
                    <div className="flex items-center gap-2">
                        <MapPin className="h-4 w-4 shrink-0" aria-hidden="true" />
                        <span className="truncate">{event.venue_name}</span>
                    </div>
                )}
            </div>

            {/* The payload's own sentence, verbatim. It is in the payload so a
                redesign cannot drop it; paraphrasing it here would defeat the
                point of it being there. */}
            <p className="mt-4 text-sm leading-relaxed text-foreground/80">{event.notice}</p>

            {/* The only action. It leaves this box. */}
            <a
                href={event.url}
                target="_blank"
                rel="noreferrer noopener"
                className="mt-4 inline-flex min-h-[44px] w-fit items-center gap-2 rounded-md text-sm font-semibold text-primary-emphasis hover:underline"
            >
                Open on their site
                <ExternalLink className="h-4 w-4" aria-hidden="true" />
                <span className="sr-only">(opens {event.publisher || 'the publisher'}&apos;s own site in a new tab)</span>
            </a>

            <div className="mt-auto pt-4 text-xs text-muted-foreground">
                {checked && <p>This host last checked on {checked}.</p>}
                {event.publisher_key && (
                    <p className="mt-1 break-all font-mono" title={event.publisher_key}>
                        Key {event.publisher_key.slice(0, 16)}
                    </p>
                )}
            </div>
        </article>
    );
}

/**
 * The whole section. Renders nothing — not a heading, not an empty state —
 * unless this host is actually carrying at least one borrowed listing.
 */
export default function PeerEvents({ host }) {
    const orgId = peerOrgId(host);
    const { events, caveat } = usePeerEvents(orgId);

    if (events.length === 0) return null;

    return (
        <section className="mt-16" data-surface="peer-events">
            {/* The tear line is the seam: everything above it is this host's,
                everything below it is somebody else's. */}
            <Separator variant="perforated" style={{ '--notch': 'var(--background)' }} />

            <div className="pt-10">
                <h2 className="font-display text-2xl font-bold tracking-tight sm:text-3xl">
                    Events from organisers this host has added
                </h2>
                {caveat && <p className="mt-2 max-w-3xl text-sm leading-relaxed text-muted-foreground">{caveat}</p>}

                <div className="mt-8 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
                    {events.map((event) => (
                        <PeerEventCard key={`${event.publisher_key}:${event.url}`} event={event} />
                    ))}
                </div>
            </div>
        </section>
    );
}
