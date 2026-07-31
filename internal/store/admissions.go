package store

import (
	"context"
	"fmt"
	"time"
)

// AdmissionClaim is one gate device's own account of one scan, read back out
// of the `admissions` table.
//
// It is deliberately the DEVICE's account rather than the server's verdict.
// Result is what the server concluded and stored (the value constrained by
// `admissions.result`, and by `idx_admissions_admitted_once` to a single
// 'admitted' per ticket); ReportedResult is what the scanning device said
// when it synced, stored verbatim by migration 0002 and never rewritten.
//
// When the two disagree, that disagreement IS the cross-gate conflict: the
// device believed it admitted somebody and the server, seeing another gate's
// claim already recorded, downgraded it. Both facts are needed to describe
// what happened at the door, and only ReportedResult can tell "this gate let
// a second person in while it was partitioned" apart from "this gate refused
// a ticket its own log already had".
//
// ReportedResult is "" for a row written before migration 0002, or by the
// online /api/scan path where the server IS the device's oracle and there is
// no separate device claim to record. "" means unknown, never "same as
// Result" — see the migration's own note on why it is not backfilled.
type AdmissionClaim struct {
	TicketID       string
	EventID        string
	GateID         string
	DeviceID       string
	ScannedAt      time.Time
	Result         string
	ReportedResult string
	Note           string
}

// ListAdmissionClaimsForEvent returns every admission row for eventID whose
// ticket was claimed as admitted by MORE THAN ONE device — the exact
// signature of a double-scan that slipped through while two gates were
// partitioned from each other.
//
// "Claimed as admitted by more than one device" is read off ReportedResult
// where a device reported one, and off Result otherwise, so a row from the
// online /api/scan path (which has no separate device claim) still counts as
// the admission it was. That COALESCE is the whole reason migration 0002
// exists: before it, the second gate's claim had already been rewritten to
// 'duplicate' on the way in and this query could not have been written at
// all.
//
// What this does and does not tell you, stated plainly:
//
//   - It reports a conflict that ALREADY HAPPENED. Two partitioned gates
//     cannot be prevented from double-admitting a ticket — that needs
//     coordination they do not have, and no merge rule creates it. This query
//     is the after-the-fact record, not a guard.
//
//   - It is only as complete as the sync is. A device that never syncs (lost,
//     broken, battery flat) contributes nothing here, and its admissions are
//     invisible — the conflict it was part of will look like a clean single
//     admission. docs/OFFLINE-GATES.md says the same thing about a dead
//     device's log.
//
//   - Rows come back grouped by ticket and ordered by scanned_at within a
//     ticket, so a caller can present them in the order the door actually
//     happened without sorting.
func (s *Store) ListAdmissionClaimsForEvent(ctx context.Context, eventID string) ([]AdmissionClaim, error) {
	const q = `
		WITH claims AS (
			SELECT id, ticket_id, event_id, gate_id, device_id, scanned_at, result,
			       reported_result, note,
			       COALESCE(NULLIF(reported_result, ''), result) AS claimed
			FROM admissions
			WHERE event_id = ?
		),
		contested AS (
			SELECT ticket_id
			FROM claims
			WHERE claimed = 'admitted'
			GROUP BY ticket_id
			HAVING COUNT(DISTINCT device_id) > 1
		)
		SELECT ticket_id, event_id, gate_id, device_id, scanned_at, result,
		       reported_result, note
		FROM claims
		WHERE ticket_id IN (SELECT ticket_id FROM contested)
		ORDER BY ticket_id, scanned_at, device_id, id`

	rows, err := s.db.QueryContext(ctx, q, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list admission claims: %w", err)
	}
	defer rows.Close()

	out := make([]AdmissionClaim, 0)
	for rows.Next() {
		var c AdmissionClaim
		var scannedAt string
		if err := rows.Scan(&c.TicketID, &c.EventID, &c.GateID, &c.DeviceID,
			&scannedAt, &c.Result, &c.ReportedResult, &c.Note); err != nil {
			return nil, fmt.Errorf("store: scan admission claim: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, scannedAt)
		if err != nil {
			// Stored timestamps are written by nowText (RFC3339Nano, UTC).
			// A value that will not parse is corrupt data, not a row to
			// silently drop from a report about who got through the door.
			return nil, fmt.Errorf("store: admission claim %s/%s has an unparseable scanned_at %q: %w",
				c.TicketID, c.DeviceID, scannedAt, err)
		}
		c.ScannedAt = t
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListAdmittedTicketIDsForEvent returns the ticket ids that already have a
// recorded admission for eventID — the server's converged view of "these
// people are already inside".
//
// This is what a gate downloads in its scan bundle (scan.Bundle.AdmittedIndex)
// so that re-pulling a bundle actually carries the other gates' admissions
// down to this one. It is read off `result` rather than the device's claim on
// purpose: `result='admitted'` is the reconciled, single-winner view that
// idx_admissions_admitted_once enforces, and it is that view — not every
// device's belief — that a gate should treat as "already used".
//
// Like ListValidTicketIDsForEvent this is a point-in-time read. A ticket
// admitted at another gate one second after this call is not in the answer,
// and the gate holding the resulting bundle cannot know about it until it
// re-pulls. That is what offline means; see docs/OFFLINE-GATES.md.
func (s *Store) ListAdmittedTicketIDsForEvent(ctx context.Context, eventID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ticket_id FROM admissions WHERE event_id = ? AND result = 'admitted'`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list admitted ticket ids: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan admitted ticket id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
