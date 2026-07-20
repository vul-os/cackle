package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Ticket is one issued, signed capability. Rows are inserted only by
// SettleOrder (in orders.go), inside the same transaction as the order
// being marked paid — a ticket must never exist for an unpaid order.
type Ticket struct {
	ID           string
	OrderID      string
	EventID      string
	TicketTypeID string
	HolderUserID *string
	HolderName   string
	Serial       string
	Capability   string
	Status       string
	IssuedAt     time.Time
	VoidedAt     *time.Time
}

const ticketSelectColumns = `SELECT id, order_id, event_id, ticket_type_id, holder_user_id, holder_name,
	serial, capability, status, issued_at, voided_at`

// GetTicketByID looks up a ticket by primary key. Returns ErrNotFound if
// absent.
func (s *Store) GetTicketByID(ctx context.Context, id string) (*Ticket, error) {
	return s.scanTicket(s.db.QueryRowContext(ctx, ticketSelectColumns+` FROM tickets WHERE id = ?`, id))
}

// ListTicketsForOrder returns every ticket issued for an order.
func (s *Store) ListTicketsForOrder(ctx context.Context, orderID string) ([]Ticket, error) {
	return s.queryTickets(ctx, ticketSelectColumns+` FROM tickets WHERE order_id = ? ORDER BY issued_at ASC`, orderID)
}

// ListTicketsForUser returns every ticket held by a user, most recently
// issued first.
func (s *Store) ListTicketsForUser(ctx context.Context, userID string) ([]Ticket, error) {
	return s.queryTickets(ctx, ticketSelectColumns+` FROM tickets WHERE holder_user_id = ? ORDER BY issued_at DESC`, userID)
}

// AttendeeRow is one ticket holder row for an event's organiser-facing
// roster: a ticket joined with its ticket type's name and — when the
// ticket has been scanned in — the single 'admitted' admission's
// timestamp (the admissions schema guarantees at most one such row per
// ticket, see migration 0001's idx_admissions_admitted_once).
//
// Deliberately absent: the buyer's email. See ListEventAttendees' doc for
// why the roster stops at holder_name.
type AttendeeRow struct {
	TicketID       string
	OrderID        string
	Serial         string
	HolderName     string
	Status         string // valid, void, refunded (tickets.status)
	TicketTypeID   string
	TicketTypeName string
	IssuedAt       time.Time
	VoidedAt       *time.Time
	AdmittedAt     *time.Time // nil until/unless the gate has admitted this ticket
}

// AttendeeFilter narrows ListEventAttendees. Query is matched as a
// case-insensitive substring of holder_name. Status, if non-empty, must be
// one of "valid", "void", "refunded" (ticket status) or "admitted" /
// "not_admitted" (gate status) — anything else matches nothing, by design:
// an unrecognised filter should return an empty page, not silently ignore
// itself and return everyone.
type AttendeeFilter struct {
	Query  string
	Status string
	Limit  int
	Offset int
}

// MaxAttendeeLimit is the hard ceiling ListEventAttendees clamps Limit to,
// regardless of what the caller asks for — an event can have tens of
// thousands of tickets, and a roster page must never be usable as an
// accidental unbounded dump of the whole tickets table.
const MaxAttendeeLimit = 200

// DefaultAttendeeLimit is what ListEventAttendees uses when the caller
// specifies no limit (Limit <= 0).
const DefaultAttendeeLimit = 50

var validAttendeeStatuses = map[string]bool{
	"valid": true, "void": true, "refunded": true,
	"admitted": true, "not_admitted": true,
}

// ListEventAttendees returns a page of ticket holders for eventID, most
// recently issued first, plus the total row count matching filter (before
// pagination) so a caller can render "showing N of Total". Limit is
// clamped to [1, MaxAttendeeLimit]; Offset below 0 is treated as 0.
func (s *Store) ListEventAttendees(ctx context.Context, eventID string, filter AttendeeFilter) ([]AttendeeRow, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultAttendeeLimit
	}
	if limit > MaxAttendeeLimit {
		limit = MaxAttendeeLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	where := `t.event_id = ?`
	args := []any{eventID}

	q := strings.TrimSpace(filter.Query)
	if q != "" {
		where += ` AND t.holder_name LIKE ? ESCAPE '\'`
		args = append(args, "%"+likeEscape(q)+"%")
	}

	status := strings.TrimSpace(filter.Status)
	if status != "" {
		if !validAttendeeStatuses[status] {
			// Unrecognised filter value: fail closed to an empty result
			// rather than silently returning the unfiltered roster.
			return nil, 0, nil
		}
		switch status {
		case "admitted":
			where += ` AND a.scanned_at IS NOT NULL`
		case "not_admitted":
			where += ` AND a.scanned_at IS NULL`
		default: // valid, void, refunded
			where += ` AND t.status = ?`
			args = append(args, status)
		}
	}

	const fromJoin = `
		FROM tickets t
		JOIN ticket_types tt ON tt.id = t.ticket_type_id
		LEFT JOIN admissions a ON a.ticket_id = t.id AND a.result = 'admitted'
	`

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+fromJoin+` WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count event attendees: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.order_id, t.serial, t.holder_name, t.status,
			t.ticket_type_id, tt.name, t.issued_at, t.voided_at, a.scanned_at
		`+fromJoin+`
		WHERE `+where+`
		ORDER BY t.issued_at DESC, t.id DESC
		LIMIT ? OFFSET ?`,
		append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list event attendees: %w", err)
	}
	defer rows.Close()

	out := make([]AttendeeRow, 0, limit)
	for rows.Next() {
		var a AttendeeRow
		var issuedAt string
		var voidedAt, admittedAt sql.NullString
		if err := rows.Scan(&a.TicketID, &a.OrderID, &a.Serial, &a.HolderName, &a.Status,
			&a.TicketTypeID, &a.TicketTypeName, &issuedAt, &voidedAt, &admittedAt); err != nil {
			return nil, 0, fmt.Errorf("store: scan event attendee row: %w", err)
		}
		if a.IssuedAt, err = textToTime(issuedAt); err != nil {
			return nil, 0, fmt.Errorf("store: parse attendee issued_at: %w", err)
		}
		if a.VoidedAt, err = textToNullTime(voidedAt); err != nil {
			return nil, 0, fmt.Errorf("store: parse attendee voided_at: %w", err)
		}
		if a.AdmittedAt, err = textToNullTime(admittedAt); err != nil {
			return nil, 0, fmt.Errorf("store: parse attendee admitted_at: %w", err)
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// VoidTicket marks a ticket void (e.g. refunded/cancelled after issuance).
// Voiding has no effect on tickets.Verify (which knows nothing of status) —
// it is enforced by internal/scan consulting this column, or by a future
// online allow/deny check, not by the offline capability check itself.
func (s *Store) VoidTicket(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tickets SET status = 'void', voided_at = ? WHERE id = ? AND status = 'valid'`,
		timeToText(at), id)
	if err != nil {
		return fmt.Errorf("store: void ticket: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// ListValidTicketIDsForEvent returns the IDs of every ticket issued for
// eventID whose status is currently 'valid' — i.e. not void, not refunded.
// This is the set internal/scan.Bundle's TicketIndex is built from (see
// internal/httpapi's handleScanBundle and handleScan): a signature alone
// proves a capability was validly issued, never that it wasn't later
// voided or refunded by VoidTicket, so a gate needs this list as a second,
// independent signal to catch that. Like the rest of this file's queries,
// this is a point-in-time read — a ticket voided a moment after this call
// returns is simply not reflected until the next call.
func (s *Store) ListValidTicketIDsForEvent(ctx context.Context, eventID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM tickets WHERE event_id = ? AND status = 'valid'`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list valid ticket ids: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan valid ticket id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) scanTicket(row *sql.Row) (*Ticket, error) {
	var t Ticket
	var holderUserID sql.NullString
	var issuedAt string
	var voidedAt sql.NullString

	err := row.Scan(&t.ID, &t.OrderID, &t.EventID, &t.TicketTypeID, &holderUserID, &t.HolderName,
		&t.Serial, &t.Capability, &t.Status, &issuedAt, &voidedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan ticket: %w", err)
	}
	if err := hydrateTicket(&t, holderUserID, issuedAt, voidedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) queryTickets(ctx context.Context, query string, args ...any) ([]Ticket, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list tickets: %w", err)
	}
	defer rows.Close()

	var out []Ticket
	for rows.Next() {
		var t Ticket
		var holderUserID sql.NullString
		var issuedAt string
		var voidedAt sql.NullString
		if err := rows.Scan(&t.ID, &t.OrderID, &t.EventID, &t.TicketTypeID, &holderUserID, &t.HolderName,
			&t.Serial, &t.Capability, &t.Status, &issuedAt, &voidedAt); err != nil {
			return nil, fmt.Errorf("store: scan ticket row: %w", err)
		}
		if err := hydrateTicket(&t, holderUserID, issuedAt, voidedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func hydrateTicket(t *Ticket, holderUserID sql.NullString, issuedAt string, voidedAt sql.NullString) error {
	var err error
	if holderUserID.Valid {
		v := holderUserID.String
		t.HolderUserID = &v
	}
	if t.IssuedAt, err = textToTime(issuedAt); err != nil {
		return fmt.Errorf("store: parse ticket issued_at: %w", err)
	}
	if t.VoidedAt, err = textToNullTime(voidedAt); err != nil {
		return fmt.Errorf("store: parse ticket voided_at: %w", err)
	}
	return nil
}
