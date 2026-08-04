// The decisions behind a borrowed listing, kept out of the component so they
// can be tested directly rather than inferred from a rendered tree.
//
// See ./peer-events.tsx for what a borrowed listing IS and the rule it holds.
// This file answers three questions: does this host display borrowed listings
// at all, whose does it display, and which rows off the wire are safe to put
// on a screen. All three must say yes before anything reaches a screen.

// Relative, with its extension, rather than the `@/` alias every component
// uses: this module is loaded directly by `node --test` as well as by Vite,
// and only a real specifier resolves under both. `lib/host.js` is the app's
// one reader of the host envelope and stays that way — the envelope is not
// parsed here or in the component.
import { showsPeerEvents, type MaybeHost } from '../../../lib/host.ts';

/**
 * A row off `GET /api/peer-events`, kept as untrusted as the host envelope
 * itself (see lib/host.ts's own banner) — every field is `unknown` and
 * narrowed by hand at the point of use, never assumed from a declared type.
 */
export interface PeerEventRow {
    external?: unknown;
    url?: unknown;
    [key: string]: unknown;
}

export interface PeerEventsPayload {
    events?: unknown;
}

/**
 * The organisation whose borrowed listings this page shows, or null for
 * "show none".
 *
 * The first question is not "whose" but "any at all": borrowed listings are
 * shown publicly only when the operator set `CACKLE_HOST_SCOPE=peers`, which
 * arrives as `peers_included` on the host envelope and is read in exactly one
 * place (`@/lib/host`). Without it this answers null and nothing is even
 * fetched — a host that does not publish another organiser's programme does
 * not ask for it either.
 *
 * Then, because GET /api/peer-events takes ONE org id, a page has to answer
 * "whose". The three cases:
 *
 *  - `?host=` narrowed the listing to one organisation — that one.
 *  - exactly one organisation on this box — trivially that one.
 *  - several organisations and no narrowing — NULL, deliberately. Merging
 *    several organisations' borrowed listings under a single heading would
 *    attribute one operator's enrolment decisions to another operator, and
 *    the enrolment decision is the entire basis on which a borrowed listing
 *    is allowed to appear at all. The route to them there is narrowing to the
 *    organisation, which is what the organisation links on the listing do.
 *
 * Tolerates a missing or malformed host envelope by answering null: no
 * envelope, no borrowed listings, rather than a guess.
 */
export function peerOrgId(host: MaybeHost): string | null {
    if (!showsPeerEvents(host)) return null;
    if (typeof host?.org?.id === 'string' && host.org.id) return host.org.id;
    const orgs = Array.isArray(host?.organisations) ? host.organisations : [];
    if (orgs.length !== 1) return null;
    const id = (orgs[0] as { id?: unknown } | undefined)?.id;
    return typeof id === 'string' && id ? id : null;
}

/**
 * The rows off `GET /api/peer-events` that may be rendered.
 *
 * `external` is always true on the wire and is sent first in the object on
 * purpose: a renderer that forgets to check it is a renderer that will make
 * somebody else's show look like this host's stock. A row that does not say
 * it is external is a bug in the producer, and the safe response to a bug is
 * to drop the row — rendering it would present another organiser's event as
 * this host's own, which is the one thing this whole feature must never do.
 *
 * A row with no `url` is dropped for the same reason from the other side: the
 * link out is the ONLY action a borrowed listing offers, and a listing with no
 * way to reach its publisher is a dead end that looks like something this host
 * might sell.
 */
export function usablePeerRows(data: PeerEventsPayload | null | undefined): PeerEventRow[] {
    const rows = (Array.isArray(data?.events) ? data.events : []) as PeerEventRow[];
    return rows.filter((e) => e?.external === true && typeof e?.url === 'string' && e.url !== '');
}

/**
 * The rows this page may actually RENDER: the usable ones, but only if this
 * host displays borrowed listings at all.
 *
 * This is the gate, and it is a separate function from `usablePeerRows` on
 * purpose. `peerOrgId` already refuses to fetch when the scope is off, but a
 * gate that only guards the fetch is a gate that a cached response, a
 * refactor, or a second caller walks straight around. This one sits at the
 * point where rows become pixels, and it takes the rows as an argument so the
 * case that matters can be tested head-on: rows present AND the scope off,
 * which is the only case that isolates the guard from "there was nothing to
 * show anyway".
 *
 * The component calls this and nothing else, so there is no path from a
 * fetched row to a screen that skips the operator's decision.
 */
export function renderablePeerRows(
    host: MaybeHost,
    data: PeerEventsPayload | null | undefined,
): PeerEventRow[] {
    if (!showsPeerEvents(host)) return [];
    return usablePeerRows(data);
}
