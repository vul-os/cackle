package httpapi

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/vul-os/cackle/internal/store"
)

// This file holds small, read-only or narrowly-scoped SQL helpers used by
// httpapi handlers for the handful of things not exposed as a typed method
// on internal/store, internal/events, or internal/orders: resolving which
// event a ticket_type belongs to (for RBAC on the flat /api/ticket-types/{id}
// routes) and persisting gate admissions (internal/scan deliberately never
// imports internal/store — see its package doc — so the server-side wiring
// of its SeenSet/SyncSink interfaces onto the real `admissions` table lives
// here). Store.DB() is exported exactly so callers outside internal/store
// can do this without internal/store growing routes-shaped methods it
// doesn't otherwise need.
//
// Every query here is parameterised — no string-built SQL, ever.

// ticketTypeEventID resolves the event_id a ticket_type belongs to, so a
// handler can run RBAC (auth.CanManageEvent) before allowing a mutation on
// a ticket type that the URL only identifies by its own id.
func ticketTypeEventID(ctx context.Context, db *sql.DB, ticketTypeID string) (string, error) {
	var eventID string
	err := db.QueryRowContext(ctx, `SELECT event_id FROM ticket_types WHERE id = ?`, ticketTypeID).Scan(&eventID)
	if err == sql.ErrNoRows {
		return "", store.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return eventID, nil
}

// isUniqueConstraintErr reports whether err came from a SQLite UNIQUE
// constraint violation (modernc.org/sqlite surfaces this in the error
// text). Used to detect "this ticket was already admitted" races against
// the partial unique index on admissions(ticket_id) WHERE result='admitted'.
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isForeignKeyErr reports whether err came from a SQLite FOREIGN KEY
// constraint violation — e.g. a scan/sync batch item claiming a ticket_id
// that doesn't actually exist. Callers should reject just that item rather
// than fail an entire batch or leak the underlying SQL error to a client.
func isForeignKeyErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

func nowText(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
