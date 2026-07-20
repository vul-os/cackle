package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TicketType is one class of ticket sold for an event (e.g. "General",
// "VIP"). QuantitySold is incremented atomically alongside order creation
// — see CreateOrderWithItems in orders.go — and must never be written
// directly by anything else.
type TicketType struct {
	ID            string
	EventID       string
	Name          string
	Description   string
	PriceCents    int64
	QuantityTotal int
	QuantitySold  int
	SalesStart    *time.Time
	SalesEnd      *time.Time
	MaxPerOrder   int
	Status        string
	SortOrder     int
}

const ticketTypeSelectColumns = `SELECT id, event_id, name, description, price_cents, quantity_total,
	quantity_sold, sales_start, sales_end, max_per_order, status, sort_order`

// CreateTicketType inserts a new ticket type. QuantitySold is always
// inserted as 0 regardless of the passed-in value — a ticket type starts
// with nothing sold.
func (s *Store) CreateTicketType(ctx context.Context, tt *TicketType) error {
	if tt.ID == "" {
		tt.ID = NewID()
	}
	tt.QuantitySold = 0
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ticket_types (id, event_id, name, description, price_cents, quantity_total,
			quantity_sold, sales_start, sales_end, max_per_order, status, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?)`,
		tt.ID, tt.EventID, tt.Name, tt.Description, tt.PriceCents, tt.QuantityTotal,
		nullTimeToText(tt.SalesStart), nullTimeToText(tt.SalesEnd), tt.MaxPerOrder, tt.Status, tt.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("store: create ticket type: %w", err)
	}
	return nil
}

// UpdateTicketType replaces every editable column (name, description,
// price, total quantity, sales window, max per order, status, sort order)
// of an existing ticket type. It deliberately does NOT take or modify
// QuantitySold — that field is only ever touched by CreateOrderWithItems
// and CancelOrderReleaseInventory.
func (s *Store) UpdateTicketType(ctx context.Context, tt *TicketType) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE ticket_types SET
			name = ?, description = ?, price_cents = ?, quantity_total = ?,
			sales_start = ?, sales_end = ?, max_per_order = ?, status = ?, sort_order = ?
		WHERE id = ?`,
		tt.Name, tt.Description, tt.PriceCents, tt.QuantityTotal,
		nullTimeToText(tt.SalesStart), nullTimeToText(tt.SalesEnd), tt.MaxPerOrder, tt.Status, tt.SortOrder, tt.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update ticket type: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// DeleteTicketType removes a ticket type. If any order_items reference it,
// the foreign key constraint (ticket_types is RESTRICT-referenced, no
// cascade) rejects the delete — this is the backstop; callers should give
// a cleaner error by checking QuantitySold first.
func (s *Store) DeleteTicketType(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ticket_types WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete ticket type: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// GetTicketTypeByID looks up a ticket type by primary key. Returns
// ErrNotFound if absent.
func (s *Store) GetTicketTypeByID(ctx context.Context, id string) (*TicketType, error) {
	return s.scanTicketType(s.db.QueryRowContext(ctx, ticketTypeSelectColumns+` FROM ticket_types WHERE id = ?`, id))
}

// ListTicketTypesForEvent returns every ticket type for an event, in
// display order.
func (s *Store) ListTicketTypesForEvent(ctx context.Context, eventID string) ([]TicketType, error) {
	rows, err := s.db.QueryContext(ctx, ticketTypeSelectColumns+`
		FROM ticket_types WHERE event_id = ? ORDER BY sort_order ASC, name ASC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list ticket types: %w", err)
	}
	defer rows.Close()

	var out []TicketType
	for rows.Next() {
		tt, err := scanTicketTypeRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tt)
	}
	return out, rows.Err()
}

func (s *Store) scanTicketType(row *sql.Row) (*TicketType, error) {
	var tt TicketType
	var salesStart, salesEnd sql.NullString
	err := row.Scan(&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.PriceCents, &tt.QuantityTotal,
		&tt.QuantitySold, &salesStart, &salesEnd, &tt.MaxPerOrder, &tt.Status, &tt.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan ticket type: %w", err)
	}
	if tt.SalesStart, err = textToNullTime(salesStart); err != nil {
		return nil, fmt.Errorf("store: parse ticket type sales_start: %w", err)
	}
	if tt.SalesEnd, err = textToNullTime(salesEnd); err != nil {
		return nil, fmt.Errorf("store: parse ticket type sales_end: %w", err)
	}
	return &tt, nil
}

// rowScanner is the subset of *sql.Rows (or *sql.Row) that Scan needs.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTicketTypeRow(row rowScanner) (TicketType, error) {
	var tt TicketType
	var salesStart, salesEnd sql.NullString
	err := row.Scan(&tt.ID, &tt.EventID, &tt.Name, &tt.Description, &tt.PriceCents, &tt.QuantityTotal,
		&tt.QuantitySold, &salesStart, &salesEnd, &tt.MaxPerOrder, &tt.Status, &tt.SortOrder)
	if err != nil {
		return TicketType{}, fmt.Errorf("store: scan ticket type row: %w", err)
	}
	if tt.SalesStart, err = textToNullTime(salesStart); err != nil {
		return TicketType{}, fmt.Errorf("store: parse ticket type sales_start: %w", err)
	}
	if tt.SalesEnd, err = textToNullTime(salesEnd); err != nil {
		return TicketType{}, fmt.Errorf("store: parse ticket type sales_end: %w", err)
	}
	return tt, nil
}
