package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// claimTimeText formats a claim's scan time the way `sync_op.claim_scanned_at`
// stores it.
//
// RFC3339Nano rather than timeToText's RFC3339, for one reason that matters: a
// whole-second time formats IDENTICALLY under both (RFC3339Nano drops trailing
// zeros), and every `admissions.scanned_at` in this schema is written to
// whole-second precision — so the anti-join in UnmintedAdmissions still matches
// text to text. But a claim arriving from a peer carries whatever precision its
// author minted, and truncating that here would map two distinct claims onto one
// idempotency key and silently drop one of them.
func claimTimeText(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// SyncOp is one signed admission claim in this node's op log: the replicable
// unit of the admission ledger.
//
// The envelope in Cose is a COSE_Sign1 over a §4.1 SyncOp, minted and verified
// by internal/scan/substrate. This package moves and stores it without opening
// it — the claim fields alongside are a denormalised copy for indexing, never a
// second source of truth. A reader that needs to trust the contents verifies the
// envelope.
type SyncOp struct {
	// Seq is this node's local, monotone, commit-ordered position. It is the
	// replication cursor a peer pages through. It is meaningful only on the node
	// that assigned it.
	Seq int64
	// OpID is the §4.1 content address, lowercase hex.
	OpID string
	// EventID is the §7 namespace the op lives in.
	EventID string
	// Author is the node key that signed it, lowercase hex.
	Author string
	// DeliveredBy is the enrolled peer key that handed this op over, or "" when
	// this node minted it. It is kept separate from Author because a relayed op
	// has both, and because a signature attests that the AUTHORING NODE recorded
	// the claim — never that a physical scanner produced it. Cackle gates have no
	// keypairs; see internal/scan/substrate's package doc.
	DeliveredBy string
	// ClaimTicket, ClaimDevice and ClaimScannedAt are the sync idempotency key —
	// the same (ticket_id, device_id, scanned_at) POST /api/scan/sync has always
	// used.
	ClaimTicket    string
	ClaimDevice    string
	ClaimScannedAt time.Time
	// Applied reports whether this claim also reached the `admissions` table. A
	// verified op for a ticket this node does not hold stays false: known, but
	// not counted anywhere a report reads. It is surfaced rather than hidden.
	Applied   bool
	Cose      []byte
	CreatedAt time.Time
}

// AppendSyncOp stores an op, and reports whether it was new to this node.
//
// It is idempotent twice over, and both conflicts are intentional:
//
//   - op_id: the identical envelope arriving again (a re-push after a dropped
//     connection) is one row.
//   - (event_id, claim_ticket, claim_device, claim_scanned_at): the same CLAIM
//     arriving under a different author — because the other node minted its own
//     op for a scan both nodes hold — is also one row. The §4.3 element carries
//     the claim and not its author, so both envelopes describe the same set
//     member and keeping one converges to the same observable state as keeping
//     both. Keeping one is what stops the log from growing a copy per node.
//
// A false return is therefore "already held", not an error, and a caller
// advancing a cursor past it is correct.
func (s *Store) AppendSyncOp(ctx context.Context, op SyncOp) (bool, error) {
	if op.OpID == "" || op.EventID == "" || op.Author == "" || len(op.Cose) == 0 {
		return false, fmt.Errorf("store: append sync op: incomplete op (id=%q ns=%q author=%q %d envelope bytes)",
			op.OpID, op.EventID, op.Author, len(op.Cose))
	}
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now()
	}
	applied := 0
	if op.Applied {
		applied = 1
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_op (op_id, event_id, author, delivered_by,
		                     claim_ticket, claim_device, claim_scanned_at,
		                     applied, cose, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		op.OpID, op.EventID, op.Author, op.DeliveredBy,
		op.ClaimTicket, op.ClaimDevice, claimTimeText(op.ClaimScannedAt),
		applied, op.Cose, timeToText(op.CreatedAt),
	)
	if err != nil {
		return false, fmt.Errorf("store: append sync op: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: append sync op: %w", err)
	}
	return n > 0, nil
}

// MarkSyncOpApplied records that an op's claim also reached the `admissions`
// table.
func (s *Store) MarkSyncOpApplied(ctx context.Context, opID string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sync_op SET applied = 1 WHERE op_id = ?`, opID); err != nil {
		return fmt.Errorf("store: mark sync op applied: %w", err)
	}
	return nil
}

// syncOpColumns is fully qualified because every read of this table joins
// `events` to resolve the organisation, and both tables have a `created_at` —
// SQLite calls that ambiguous and refuses the query rather than picking one.
const syncOpColumns = `sync_op.seq, sync_op.op_id, sync_op.event_id, sync_op.author,
	sync_op.delivered_by, sync_op.claim_ticket, sync_op.claim_device,
	sync_op.claim_scanned_at, sync_op.applied, sync_op.cose, sync_op.created_at`

func scanSyncOp(sc interface{ Scan(...any) error }) (SyncOp, error) {
	var (
		op        SyncOp
		applied   int
		scannedAt string
		createdAt string
	)
	if err := sc.Scan(&op.Seq, &op.OpID, &op.EventID, &op.Author, &op.DeliveredBy,
		&op.ClaimTicket, &op.ClaimDevice, &scannedAt, &applied, &op.Cose, &createdAt); err != nil {
		return SyncOp{}, err
	}
	op.Applied = applied == 1
	t, err := time.Parse(time.RFC3339Nano, scannedAt)
	if err != nil {
		return SyncOp{}, fmt.Errorf("store: sync op %s claim_scanned_at %q: %w", op.OpID, scannedAt, err)
	}
	op.ClaimScannedAt = t
	c, err := textToTime(createdAt)
	if err != nil {
		return SyncOp{}, fmt.Errorf("store: sync op %s created_at %q: %w", op.OpID, createdAt, err)
	}
	op.CreatedAt = c
	return op, nil
}

// SyncOpsForOrgAfter returns up to limit ops for orgID's events with seq greater
// than after, in seq order — one page of what a peer enrolled for that org may
// pull.
//
// The org filter is a join through `events`, and it is the authorisation
// boundary: a peer enrolled for one organisation is served that organisation's
// admission ledger and no other's. A consequence worth stating rather than
// discovering: an op about an event this node does not hold cannot be
// org-resolved and is therefore NOT forwarded onward. This node stores and
// reports such a claim, but only a node that knows the event relays it.
func (s *Store) SyncOpsForOrgAfter(ctx context.Context, orgID string, after int64, limit int) ([]SyncOp, error) {
	if limit <= 0 {
		return []SyncOp{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+syncOpColumns+`
		  FROM sync_op
		  JOIN events ON events.id = sync_op.event_id
		 WHERE events.org_id = ? AND sync_op.seq > ?
		 ORDER BY sync_op.seq
		 LIMIT ?`, orgID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list sync ops: %w", err)
	}
	defer rows.Close()

	out := make([]SyncOp, 0, limit)
	for rows.Next() {
		op, err := scanSyncOp(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan sync op: %w", err)
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

// UnmintedAdmissions returns admission rows for orgID that have no op in the log
// yet — the local claims a mint pass still has to sign.
//
// It is an anti-join on the sync idempotency key rather than on `admissions.id`,
// and that choice does real work: a claim this node learned from a PEER is
// written into `admissions` with a fresh local row id, and joining on the id
// would make this node mint a second op for a claim it already holds — a copy
// per node, per hop, forever. On the claim key it is already covered and is
// correctly skipped.
//
// Oldest first by `admissions.id` (a ULID, so roughly insertion order). Ordering
// only affects which page comes first; nothing is skipped, because a row stays
// unminted until it is minted.
func (s *Store) UnmintedAdmissions(ctx context.Context, orgID string, limit int) ([]AdmissionClaim, error) {
	if limit <= 0 {
		return []AdmissionClaim{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.ticket_id, a.event_id, a.gate_id, a.device_id, a.scanned_at,
		       a.result, a.reported_result, a.note
		  FROM admissions a
		  JOIN events e ON e.id = a.event_id
		  LEFT JOIN sync_op o
		         ON o.event_id = a.event_id
		        AND o.claim_ticket = a.ticket_id
		        AND o.claim_device = a.device_id
		        AND o.claim_scanned_at = a.scanned_at
		 WHERE e.org_id = ? AND o.seq IS NULL
		 ORDER BY a.id
		 LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list unminted admissions: %w", err)
	}
	defer rows.Close()

	out := make([]AdmissionClaim, 0, limit)
	for rows.Next() {
		var (
			c         AdmissionClaim
			scannedAt string
		)
		if err := rows.Scan(&c.TicketID, &c.EventID, &c.GateID, &c.DeviceID,
			&scannedAt, &c.Result, &c.ReportedResult, &c.Note); err != nil {
			return nil, fmt.Errorf("store: scan unminted admission: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, scannedAt)
		if err != nil {
			return nil, fmt.Errorf("store: admission %s/%s has an unparseable scanned_at %q: %w",
				c.TicketID, c.DeviceID, scannedAt, err)
		}
		c.ScannedAt = t
		out = append(out, c)
	}
	return out, rows.Err()
}

// SyncOpStats is what an operator is shown about the op log for one
// organisation.
type SyncOpStats struct {
	// Ops is how many signed claims this node holds for the org's events.
	Ops int `json:"ops"`
	// Unapplied is how many of them are NOT in the `admissions` table — a
	// verified claim about a ticket this node does not hold. It is reported
	// because a zero here is the only thing that makes the rest of the numbers
	// a complete picture.
	Unapplied int `json:"unapplied"`
	// HighestSeq is this node's newest op position for the org, the value a peer
	// pages towards.
	HighestSeq int64 `json:"highest_seq"`
	// Pending is how many local admission rows have not been minted into ops
	// yet — work the next round will do.
	Pending int `json:"pending"`
}

// SyncOpStatsForOrg summarises the op log for one organisation.
func (s *Store) SyncOpStatsForOrg(ctx context.Context, orgID string) (SyncOpStats, error) {
	var out SyncOpStats
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN o.applied = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(MAX(o.seq), 0)
		  FROM sync_op o
		  JOIN events e ON e.id = o.event_id
		 WHERE e.org_id = ?`, orgID).Scan(&out.Ops, &out.Unapplied, &out.HighestSeq)
	if err != nil {
		return SyncOpStats{}, fmt.Errorf("store: sync op stats: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		  FROM admissions a
		  JOIN events e ON e.id = a.event_id
		  LEFT JOIN sync_op o
		         ON o.event_id = a.event_id
		        AND o.claim_ticket = a.ticket_id
		        AND o.claim_device = a.device_id
		        AND o.claim_scanned_at = a.scanned_at
		 WHERE e.org_id = ? AND o.seq IS NULL`, orgID).Scan(&out.Pending)
	if err != nil {
		return SyncOpStats{}, fmt.Errorf("store: sync op pending count: %w", err)
	}
	return out, nil
}

// EventOrgIDs resolves the organisation for each of eventIDs that this node
// actually holds.
//
// An event this node does not have is simply absent from the result — a peer's
// claim about an unknown event is a fact to store, not an error to raise. A
// STORAGE failure is a different thing entirely and is returned, because
// treating "the database is broken" as "the event does not exist" would let an
// authorisation check fail open.
func (s *Store) EventOrgIDs(ctx context.Context, eventIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(eventIDs))
	for _, id := range eventIDs {
		if id == "" {
			continue
		}
		if _, done := out[id]; done {
			continue
		}
		var orgID string
		err := s.db.QueryRowContext(ctx, `SELECT org_id FROM events WHERE id = ?`, id).Scan(&orgID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("store: resolve event org: %w", err)
		}
		out[id] = orgID
	}
	return out, nil
}
