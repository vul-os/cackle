// The two decisions behind a borrowed listing, kept out of the component so
// they can be tested directly rather than inferred from a rendered tree.
//
// See ./peer-events.jsx for what a borrowed listing IS and the rule it holds.
// This file answers only: whose borrowed listings does a page show, and which
// rows off the wire are safe to put on a screen.

/**
 * The organisation whose borrowed listings this page shows, or null for
 * "show none".
 *
 * GET /api/peer-events takes ONE org id, so a page has to answer "whose". The
 * three cases:
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
export function peerOrgId(host) {
    if (typeof host?.org?.id === 'string' && host.org.id) return host.org.id;
    const orgs = Array.isArray(host?.organisations) ? host.organisations : [];
    if (orgs.length !== 1) return null;
    const id = orgs[0]?.id;
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
export function usablePeerRows(data) {
    const rows = Array.isArray(data?.events) ? data.events : [];
    return rows.filter((e) => e?.external === true && typeof e?.url === 'string' && e.url !== '');
}
