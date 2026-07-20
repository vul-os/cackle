package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
