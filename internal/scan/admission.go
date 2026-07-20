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
func Decide(ctx context.Context, cap string, ring tickets.KeyRing, eventID string, seen SeenSet, now time.Time) Result {
	payload, err := tickets.VerifyWithRing(cap, ring, now)
	if err != nil {
		return Result{Status: Invalid, Reason: err.Error()}
	}

	if payload.EID != eventID {
		return Result{Status: WrongEvent, Payload: payload, Reason: "capability is for a different event"}
	}

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
