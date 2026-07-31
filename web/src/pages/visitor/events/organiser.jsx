import React, { useEffect, useState } from 'react';
import { ExternalLink } from 'lucide-react';
import { events as eventsApi } from '@/lib/api';
import { orgForEvent, showsOrgLabels } from '@/lib/host';
import { orgPageHref } from '@/lib/org-page';

/**
 * "Presented by X" on an event page, linking to that organiser's own page.
 *
 * # Why this needs a second request
 *
 * `GET /api/events/{ref}` returns the event, which carries `org_id` and
 * nothing else about the organisation — no name, no slug. The one place that
 * resolves an org_id into a name is the `host` envelope on `GET /api/events`,
 * and `orgForEvent()` in @/lib/host is the one function that does it. So this
 * component fetches the listing envelope rather than guessing, and rather than
 * growing a second source of truth for what an organisation is called.
 *
 * It asks for `limit: 1`: the envelope is what it wants and the events beside
 * it are waste. The request is fire-and-forget — every failure path, including
 * an older server that sends no envelope at all, renders NOTHING. An event page
 * that could not resolve its organiser says nothing about one, because a
 * half-known attribution ("Presented by an organiser") is worse than none.
 *
 * # Why only on a multi-organisation box
 *
 * `showsOrgLabels()` is the same switch the event cards use, and it comes from
 * the host itself rather than from counting what is on screen. On a box that
 * hosts one organisation, the whole site already IS that organisation: an
 * attribution line would be telling a visitor at The Bijou that this event is
 * presented by The Bijou, and a link to "their page" would lead to a second,
 * near-identical copy of the listing they are already on. A single venue is not
 * dressed up as a directory, here or anywhere else.
 */
export default function OrganiserAttribution({ event, className = '' }) {
    const [host, setHost] = useState(null);

    useEffect(() => {
        let cancelled = false;
        eventsApi
            .list({ limit: 1 })
            .then((data) => {
                if (cancelled || Array.isArray(data)) return;
                setHost(data?.host ?? null);
            })
            .catch(() => {
                // Deliberately silent. The attribution is additive; failing to
                // load it must not put an error on an event page that is
                // otherwise fine.
            });
        return () => {
            cancelled = true;
        };
    }, [event?.id, event?.slug]);

    if (!showsOrgLabels(host)) return null;
    const org = orgForEvent(host, event);
    const href = orgPageHref(org);
    if (!org || !href) return null;

    return (
        <p className={`text-sm text-muted-foreground ${className}`}>
            Presented by{' '}
            <a
                href={href}
                className="inline-flex min-h-[44px] items-center gap-1.5 font-medium text-foreground underline underline-offset-4 hover:text-primary sm:min-h-0"
            >
                {org.name}
                <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
            </a>
        </p>
    );
}
