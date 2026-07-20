package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/scan"
	"github.com/vul-os/cackle/internal/store"
)

// This file wires internal/scan's pure decision logic (Decide, SeenSet,
// SyncSink) onto the real `admissions` table. internal/scan deliberately
// never imports internal/store (see its package doc — it must stay
// self-contained so a fully offline gate binary can embed just that
// package), which means the server-side persistence for the online
// /api/scan and /api/scan/sync routes has nowhere else to live but here.
//
// NOTE on /api/scan's event scoping: BUILD-SPEC/docs/API.md describe the
// request body as {capability, device_id, gate_id, scanned_at} with no
// event id anywhere in the route or body. But internal/scan.Decide requires
// an eventID to check the capability against (that's what produces
// wrong_event rather than trusting whatever event id the token itself
// claims), and RBAC requires knowing which event/org to authorize against.
// Rather than silently trusting the unverified token's own "eid" claim for
// both authorization AND the wrong-event check (which would make
// wrong_event unreachable — the check would always trivially pass), this
// handler requires an explicit event_id field in the request body. This is
// an additive, backward-compatible field, not a breaking change to the
// documented shape; flagged here for the docs owner to reconcile.

// --- dbSeenSet: online-scan SeenSet backed by the admissions table ---

// dbSeenSet implements scan.SeenSet against the real `admissions` table,
// for the ONLINE /api/scan path (a gate with connectivity that wants
// server-side admission recorded immediately rather than batched — see
// docs/API.md's "Offline gate" section). It is deliberately request-scoped:
// one instance per HTTP call, carrying just enough context (event/gate/
// device/caller) to write a complete admissions row the moment Decide
// calls MarkSeen.
//
// The admitted row IS the row MarkSeen inserts — there is no separate
// insert afterwards for the Admitted case. The database's own partial
// unique index (admissions(ticket_id) WHERE result='admitted') is the
// actual race-free "first scan wins" guarantee; a concurrent duplicate
// simply fails that INSERT and MarkSeen reports firstSeen=false, exactly
// mirroring SQLiteSeenSet's INSERT OR IGNORE technique for the offline
// case.
type dbSeenSet struct {
	db        *sql.DB
	eventID   string
	gateID    string
	deviceID  string
	scannedBy string // caller's user id; empty means unknown/system
}

var _ scan.SeenSet = (*dbSeenSet)(nil)

func (s *dbSeenSet) MarkSeen(ctx context.Context, ticketID string, at time.Time) (bool, error) {
	var scannedBy sql.NullString
	if s.scannedBy != "" {
		scannedBy = sql.NullString{String: s.scannedBy, Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admissions (id, ticket_id, event_id, gate_id, scanned_by, device_id, scanned_at, result, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'admitted', '')`,
		store.NewID(), ticketID, s.eventID, s.gateID, scannedBy, s.deviceID, nowText(at),
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *dbSeenSet) Seen(ctx context.Context, ticketID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admissions WHERE ticket_id = ? AND result = 'admitted'`, ticketID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// --- dbSyncSink: POST /api/scan/sync's server-side SyncSink ---

// dbSyncSink implements scan.SyncSink against the admissions table for
// batch sync of offline-recorded scans. Idempotency key is exactly what
// BUILD-SPEC specifies: (ticket_id, device_id, scanned_at). A second
// device that independently believed it was first to admit a ticket is
// downgraded to 'duplicate' server-side — the admissions table's partial
// unique index is the final authority on which single row gets to be
// 'admitted' for a given ticket, no matter how many devices disagreed
// while offline.
type dbSyncSink struct {
	db *sql.DB
}

var _ scan.SyncSink = (*dbSyncSink)(nil)

func (s *dbSyncSink) Apply(ctx context.Context, batch []scan.QueuedAdmission) ([]bool, error) {
	applied := make([]bool, len(batch))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	for i, a := range batch {
		if a.TicketID == "" || a.Result == scan.Invalid {
			// Nothing to persist (invalid scans never resolved to a real
			// ticket id) — treat as already-applied so the client can
			// drop it from its local queue without retrying forever.
			applied[i] = true
			continue
		}

		var already int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM admissions WHERE ticket_id = ? AND device_id = ? AND scanned_at = ?`,
			a.TicketID, a.DeviceID, nowText(a.ScannedAt),
		).Scan(&already); err != nil {
			return nil, err
		}
		if already > 0 {
			applied[i] = false // exact same scan event already synced
			continue
		}

		result := a.Result
		if result == scan.Admitted {
			var admittedAlready int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM admissions WHERE ticket_id = ? AND result = 'admitted'`, a.TicketID,
			).Scan(&admittedAlready); err != nil {
				return nil, err
			}
			if admittedAlready > 0 {
				result = scan.Duplicate
			}
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO admissions (id, ticket_id, event_id, gate_id, scanned_by, device_id, scanned_at, result, note)
			VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?)`,
			store.NewID(), a.TicketID, a.EventID, a.GateID, a.DeviceID, nowText(a.ScannedAt), string(result), a.Note,
		)
		if err != nil {
			if isForeignKeyErr(err) {
				applied[i] = false // unknown ticket_id — reject just this item, not the batch
				continue
			}
			return nil, err
		}
		applied[i] = true
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return applied, nil
}

// --- HTTP handlers ---

type scanRequest struct {
	EventID    string    `json:"event_id"`
	Capability string    `json:"capability"`
	DeviceID   string    `json:"device_id"`
	GateID     string    `json:"gate_id"`
	ScannedAt  time.Time `json:"scanned_at"`
}

type scanTicketView struct {
	ID           string `json:"id"`
	EventID      string `json:"event_id"`
	TicketTypeID string `json:"ticket_type_id"`
	Seat         string `json:"seat,omitempty"`
}

type scanHolderView struct {
	UserID string `json:"user_id,omitempty"`
	Name   string `json:"name"`
}

type scanResponse struct {
	Result string          `json:"result"`
	Reason string          `json:"reason,omitempty"`
	Ticket *scanTicketView `json:"ticket,omitempty"`
	Holder *scanHolderView `json:"holder,omitempty"`
}

// handleScanBundle serves GET /api/events/{id}/scan-bundle: everything a
// gate needs to run for the whole event with the network unplugged.
func (s *server) handleScanBundle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := userFromContext(ctx)
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(ctx, user.ID, eventID, auth.RoleScanner)
	if err != nil {
		internalError(w, s.log(), "scan-bundle rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not a scanner/admin/owner on this event's org")
		return
	}

	ev, err := s.deps.Events.Get(ctx, eventID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return
		}
		internalError(w, s.log(), "scan-bundle get event", err)
		return
	}

	ring, err := s.deps.Events.IssuerPublicKeys(ctx, eventID)
	if err != nil {
		internalError(w, s.log(), "scan-bundle issuer keys", err)
		return
	}

	ticketIDs, err := s.deps.Store.ListValidTicketIDsForEvent(ctx, eventID)
	if err != nil {
		internalError(w, s.log(), "scan-bundle ticket index", err)
		return
	}

	bundle := scan.Bundle{
		Event:              eventToMeta(ev),
		IssuerKeys:         ring,
		TicketIndex:        ticketIDs,
		TicketIndexPresent: true, // ticketIDs is the authoritative valid set from the DB, even if empty
		Allocation:         nil,  // capacity delegation to sub-issuers is roadmap work (ROADMAP.md)
		IssuedAt:           time.Now(),
	}
	if err := bundle.Validate(); err != nil {
		internalError(w, s.log(), "scan-bundle validate", err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

// handleScan serves POST /api/scan: the online admission decision path.
func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := userFromContext(ctx)

	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.EventID == "" || req.Capability == "" || req.DeviceID == "" {
		badRequest(w, "event_id, capability, and device_id are required")
		return
	}

	ok, err := s.deps.Auth.CanManageEvent(ctx, user.ID, req.EventID, auth.RoleScanner)
	if err != nil {
		internalError(w, s.log(), "scan rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not a scanner/admin/owner on this event's org")
		return
	}

	ring, err := s.deps.Events.IssuerPublicKeys(ctx, req.EventID)
	if err != nil {
		internalError(w, s.log(), "scan issuer keys", err)
		return
	}

	// The online path honours the same ticket_index check an offline gate
	// gets from its cached bundle (see scan.DecideWithBundle) — there is no
	// reason an online scanner should be laxer about a refunded ticket than
	// an offline one. We build a Bundle for exactly this call rather than
	// serving a stale cached one; Event only needs EventID populated,
	// DecideWithBundle never looks at the rest.
	ticketIDs, err := s.deps.Store.ListValidTicketIDsForEvent(ctx, req.EventID)
	if err != nil {
		internalError(w, s.log(), "scan ticket index", err)
		return
	}

	scannedAt := req.ScannedAt
	if scannedAt.IsZero() {
		scannedAt = time.Now()
	}

	seen := &dbSeenSet{db: s.deps.Store.DB(), eventID: req.EventID, gateID: req.GateID, deviceID: req.DeviceID, scannedBy: user.ID}
	bundle := scan.Bundle{
		Event:              scan.EventMeta{EventID: req.EventID},
		IssuerKeys:         ring,
		TicketIndex:        ticketIDs,
		TicketIndexPresent: true, // ticketIDs is the authoritative valid set from the DB, even if empty
	}
	result := scan.DecideWithBundle(ctx, req.Capability, bundle, seen, scannedAt)

	// Admitted was already written by seen.MarkSeen above (the atomic
	// claim IS the row). Every other outcome that got far enough to know
	// a ticket id (Duplicate, WrongEvent) still needs its own append-only
	// row; Invalid never has a known ticket id, so there is nothing valid
	// to reference and no row is written for it (still logged via the
	// request logger's status/duration line).
	if result.Payload != nil && result.Status != scan.Admitted {
		_, err := s.deps.Store.DB().ExecContext(ctx, `
			INSERT INTO admissions (id, ticket_id, event_id, gate_id, scanned_by, device_id, scanned_at, result, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			store.NewID(), result.Payload.TID, req.EventID, req.GateID, user.ID, req.DeviceID, nowText(scannedAt), string(result.Status), result.Reason,
		)
		if err != nil && !isForeignKeyErr(err) {
			internalError(w, s.log(), "scan record admission", err)
			return
		}
	}

	resp := scanResponse{Result: string(result.Status), Reason: result.Reason}
	if result.Payload != nil {
		resp.Ticket = &scanTicketView{
			ID:           result.Payload.TID,
			EventID:      result.Payload.EID,
			TicketTypeID: result.Payload.TT,
			Seat:         result.Payload.Seat,
		}
		resp.Holder = &scanHolderView{UserID: result.Payload.Sub, Name: result.Payload.Name}
	}
	writeJSON(w, http.StatusOK, resp)
}

type scanSyncItem struct {
	TicketID  string    `json:"ticket_id"`
	EventID   string    `json:"event_id"`
	GateID    string    `json:"gate_id"`
	DeviceID  string    `json:"device_id"`
	ScannedAt time.Time `json:"scanned_at"`
	Result    string    `json:"result"`
	Note      string    `json:"note,omitempty"`
}

type scanSyncRequest struct {
	Admissions []scanSyncItem `json:"admissions"`
}

type scanSyncResponse struct {
	Applied []bool `json:"applied"`
}

// handleScanSync serves POST /api/scan/sync: idempotent batch upload of
// scans recorded while a gate was offline.
func (s *server) handleScanSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := userFromContext(ctx)

	var req scanSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if len(req.Admissions) == 0 {
		writeJSON(w, http.StatusOK, scanSyncResponse{Applied: []bool{}})
		return
	}

	checkedEvents := make(map[string]bool, 4)
	batch := make([]scan.QueuedAdmission, 0, len(req.Admissions))
	for _, item := range req.Admissions {
		if item.EventID == "" || item.DeviceID == "" || item.ScannedAt.IsZero() {
			badRequest(w, "each admission requires event_id, device_id, and scanned_at")
			return
		}
		if !checkedEvents[item.EventID] {
			ok, err := s.deps.Auth.CanManageEvent(ctx, user.ID, item.EventID, auth.RoleScanner)
			if err != nil {
				internalError(w, s.log(), "scan sync rbac", err)
				return
			}
			if !ok {
				forbidden(w, "you are not a scanner/admin/owner on event "+item.EventID)
				return
			}
			checkedEvents[item.EventID] = true
		}

		status := scan.Status(item.Result)
		switch status {
		case scan.Admitted, scan.Duplicate, scan.WrongEvent, scan.Invalid:
		default:
			badRequest(w, "invalid result value: "+item.Result)
			return
		}

		batch = append(batch, scan.QueuedAdmission{
			TicketID:  item.TicketID,
			EventID:   item.EventID,
			GateID:    item.GateID,
			DeviceID:  item.DeviceID,
			ScannedAt: item.ScannedAt,
			Result:    status,
			Note:      item.Note,
		})
	}

	sink := &dbSyncSink{db: s.deps.Store.DB()}
	applied, err := sink.Apply(ctx, batch)
	if err != nil {
		internalError(w, s.log(), "scan sync apply", err)
		return
	}
	writeJSON(w, http.StatusOK, scanSyncResponse{Applied: applied})
}
