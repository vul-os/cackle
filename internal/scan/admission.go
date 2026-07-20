// Package scan implements Cackle's gate-side admission logic: the decision
// a scanner makes when a QR code is presented, entirely from data already
// resident on the device (a cached tickets.KeyRing and a local SeenSet) —
// no network call required. See internal/tickets for the underlying
// Ed25519 capability format this package verifies against.
package scan

import (
	"context"
	"time"

	"github.com/vul-os/cackle/internal/tickets"
)

// Status is the outcome of an admission decision. It is a closed set —
// exactly these four values, matching the `admissions.result` CHECK
// constraint in the store schema (internal/store owns that table; this
// package only produces the Status value, it never writes the row).
type Status string

const (
	// Admitted means this is the first time this ticket has been seen by
	// this SeenSet, the capability verified successfully, and it belongs
	// to the event being scanned for.
	Admitted Status = "admitted"
	// Duplicate means the capability verified successfully and belongs to
	// the right event, but this ticket_id has already been admitted
	// (by this SeenSet) previously. First scan wins; this one does not
	// override it.
	Duplicate Status = "duplicate"
	// Invalid means the capability failed verification: tampered, wrong
	// key, unknown kid, expired, not-yet-valid, or malformed.
	Invalid Status = "invalid"
	// WrongEvent means the capability verified successfully (it is a
	// genuine, currently-valid ticket for SOME event) but not for the
	// event this gate is scanning for.
	WrongEvent Status = "wrong_event"
)

// Result is what Decide returns: the outcome plus enough context for the
// caller to log/display it. Payload is non-nil for Admitted, Duplicate and
// WrongEvent (the capability verified, so we know who/what it is) and nil
// for Invalid (verification never got far enough to know).
type Result struct {
	Status  Status
	Payload *tickets.Payload
	// Reason is a short, human-readable explanation — the underlying
	// verification error for Invalid, or a fixed description otherwise.
	// It is safe to display to a gate operator; it is deliberately never
	// the raw attacker-supplied token.
	Reason string
}

// SeenSet is the local dedupe oracle Decide consults: has this ticket_id
// already been admitted? Implementations must make MarkSeen atomic — under
// concurrent calls for the same ticketID, exactly one call may observe
// firstSeen == true. See MemorySeenSet and SQLiteSeenSet in this package.
type SeenSet interface {
	// MarkSeen atomically checks whether ticketID has been seen before and
	// records that it has now been seen (idempotently — calling it again
	// for the same ticketID is safe and just reports firstSeen=false).
	// firstSeen is true only for the call that wins the race to record it
	// first.
	MarkSeen(ctx context.Context, ticketID string, at time.Time) (firstSeen bool, err error)
	// Seen reports whether ticketID has already been marked, without
	// itself recording anything.
	Seen(ctx context.Context, ticketID string) (bool, error)
}

// Decide is the entire offline admission decision. It is deliberately thin:
// verify the capability purely (tickets.VerifyWithRing — no I/O of its
// own), check the event id matches, then consult seen for the local
// first-scan-wins dedupe. The only I/O in this whole call is whatever
// seen.MarkSeen does (typically a local SQLite write) — there is still no
// network access anywhere in this path, which is the whole point.
//
// Decide has no notion of ticket revocation — it only ever proves a
// capability was validly issued, never that it wasn't later voided or
// refunded. Callers with a Bundle (i.e. anything that went through
// scan-bundle) should prefer DecideWithBundle, which layers exactly that
// check on top without altering this signature — kept stable because a JS
// port (web/src/lib/capability.js, consumed by
// web/src/pages/organizers/scanner/use-scan-engine.js) and internal/httpapi
// both depend on this exact shape.
func Decide(ctx context.Context, cap string, ring tickets.KeyRing, eventID string, seen SeenSet, now time.Time) Result {
	payload, terminal, ok := verifyForEvent(cap, ring, eventID, now)
	if !ok {
		return terminal
	}
	return admitOrDuplicate(ctx, payload, seen, now)
}

// DecideWithBundle is Decide plus one additional, purely-local check: when
// the bundle carries an authoritative ticket index, a ticket whose id is
// absent from it is rejected as Invalid even though its signature verifies
// cleanly. That is the whole point of TicketIndex (see bundle.go) — a
// signature only proves a capability was validly issued, never that it
// wasn't later voided or refunded, and this is what closes that gap.
//
// The index is authoritative when b.TicketIndexPresent is true, which the
// server ALWAYS sets — it queried the current valid set to build the
// bundle. Crucially, an authoritative index that happens to be EMPTY means
// "admit nothing": every ticket for this event has been voided/refunded (or
// none was ever issued). Earlier this was inferred from length alone, which
// could not tell "all tickets revoked" apart from "no index data", and so
// would have silently re-admitted every physically-held ticket for a
// cancelled event. It no longer can.
//
// Compatibility fallback, stated plainly: only when b.TicketIndexPresent is
// false — a legacy or hand-built bundle carrying no index data at all — does
// DecideWithBundle fall back to signature-only behaviour identical to
// Decide. Refusing to admit anyone just because a legacy bundle lacks the
// field would trade one failure mode for a worse one. This fallback is
// keyed on the explicit "present" flag, never on the index being empty.
//
// Point-in-time limitation, also stated plainly: TicketIndex is a snapshot
// as of the bundle's IssuedAt. A ticket refunded after a gate downloaded
// its bundle is still admittable at that gate until it re-pulls a fresh
// one — DecideWithBundle cannot see a revocation it was never told about.
// The mitigation is operational (re-sync bundles periodically, e.g. between
// shifts or at a venue's re-entry point), not a code fix; see
// docs/OFFLINE-GATES.md.
//
// Like Decide, DecideWithBundle is pure aside from seen.MarkSeen: the index
// is just more pinned data handed in by the caller, exactly like the key
// ring — no network, no DB lookup inside this function.
func DecideWithBundle(ctx context.Context, cap string, b Bundle, seen SeenSet, now time.Time) Result {
	payload, terminal, ok := verifyForEvent(cap, b.IssuerKeys, b.Event.EventID, now)
	if !ok {
		return terminal
	}

	// An authoritative index (present=true) is checked even when empty —
	// empty means "admit nothing". Only an absent index (present=false)
	// falls through to signature-only admission.
	if b.TicketIndexPresent && !ticketIndexContains(b.TicketIndex, payload.TID) {
		return Result{Status: Invalid, Reason: "ticket revoked or not issued for this event"}
	}

	return admitOrDuplicate(ctx, payload, seen, now)
}

// verifyForEvent runs the two checks Decide and DecideWithBundle share
// before either of them consult SeenSet: signature verification against the
// pinned ring, then the event-id match. On success it returns the decoded
// payload and ok=true, meaning the caller should proceed. On failure it
// returns a terminal Result (Invalid or WrongEvent) and ok=false, meaning
// the caller should return that Result immediately without touching seen.
func verifyForEvent(cap string, ring tickets.KeyRing, eventID string, now time.Time) (payload *tickets.Payload, terminal Result, ok bool) {
	payload, err := tickets.VerifyWithRing(cap, ring, now)
	if err != nil {
		return nil, Result{Status: Invalid, Reason: err.Error()}, false
	}
	if payload.EID != eventID {
		return nil, Result{Status: WrongEvent, Payload: payload, Reason: "capability is for a different event"}, false
	}
	return payload, Result{}, true
}

// admitOrDuplicate is the local first-scan-wins dedupe shared by Decide and
// DecideWithBundle, run only once a capability has already verified and
// passed every other check (event id, and — for DecideWithBundle — the
// ticket index).
func admitOrDuplicate(ctx context.Context, payload *tickets.Payload, seen SeenSet, now time.Time) Result {
	firstSeen, err := seen.MarkSeen(ctx, payload.TID, now)
	if err != nil {
		// The dedupe store itself failed (e.g. disk I/O error). We fail
		// CLOSED — refuse admission rather than risk double-admitting a
		// ticket because we couldn't check.
		return Result{Status: Invalid, Payload: payload, Reason: "local dedupe check failed: " + err.Error()}
	}
	if !firstSeen {
		return Result{Status: Duplicate, Payload: payload, Reason: "ticket already admitted"}
	}

	return Result{Status: Admitted, Payload: payload, Reason: "ok"}
}

// ticketIndexContains reports whether ticketID appears in index. Linear
// scan is fine here: index sizes track an event's ticket count, this runs
// once per scan on a gate device, and it keeps Bundle's wire shape a plain
// JSON array rather than forcing every caller (including the JS port) to
// maintain a parallel set.
func ticketIndexContains(index []string, ticketID string) bool {
	for _, id := range index {
		if id == ticketID {
			return true
		}
	}
	return false
}
