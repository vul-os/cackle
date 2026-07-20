// Tracks the most recently in-progress draft event created by the create
// wizard, so "save a draft and come back" is a real capability rather than
// a dead end.
//
// Why this exists at all: GET /api/events (events.list()) only ever
// returns PUBLISHED events (internal/events.Service.ListPublic — see
// internal/httpapi/event_handlers.go's handleListPublicEvents), regardless
// of the caller's org membership. There's no wired-up route for "every
// event my org owns, any status" (events.Service.ListByOrg exists but
// nothing in the router calls it as of this wave). That means a draft
// created here is otherwise invisible in the Events list and the Home
// dashboard the moment the user navigates away — no way back to it except
// guessing the URL. This is a stopgap until that route lands: we remember
// the id locally and offer a direct link back, verified live against
// GET /api/events/{id} (which — unlike the list route — does resolve by id
// regardless of status) rather than trusting the stale pointer blindly.
const KEY_PREFIX = 'cackle_pending_draft_event_id';

function keyFor(orgId) {
    return orgId ? `${KEY_PREFIX}:${orgId}` : KEY_PREFIX;
}

export function setPendingDraft(orgId, eventId) {
    try {
        localStorage.setItem(keyFor(orgId), eventId);
    } catch {
        // localStorage unavailable — the banner just won't offer a resume link
    }
}

export function getPendingDraft(orgId) {
    try {
        return localStorage.getItem(keyFor(orgId));
    } catch {
        return null;
    }
}

export function clearPendingDraft(orgId) {
    try {
        localStorage.removeItem(keyFor(orgId));
    } catch {
        // nothing to clear if storage isn't available in the first place
    }
}
