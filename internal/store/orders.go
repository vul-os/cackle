package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Order is a buyer's purchase of one or more ticket types for an event.
type Order struct {
	ID            string
	EventID       string
	UserID        *string
	BuyerEmail    string
	BuyerName     string
	Status        string
	SubtotalMinor int64
	FeeMinor      int64
	TotalMinor    int64
	Currency      string
	Provider      string
	ProviderRef   *string
	CreatedAt     time.Time
	PaidAt        *time.Time
}

// OrderLine is one requested ticket_type/quantity pair for
// CreateOrderWithItems. UnitPriceMinor MUST already have been read from the
// ticket_types table by the caller — never from anything client-supplied —
// this is what makes price forgery structurally impossible one layer up in
// internal/orders.
type OrderLine struct {
	TicketTypeID   string
	Quantity       int
	UnitPriceMinor int64
}

// ErrSoldOut is returned by CreateOrderWithItems when reserving inventory
// for a line would exceed a ticket type's quantity_total. This is the
// cardinal no-oversell guarantee: the check and the decrement happen in the
// same UPDATE statement, inside the same transaction as the order/order_item
// inserts, so concurrent purchasers racing for the last unit can never both
// win.
var ErrSoldOut = errors.New("store: ticket type sold out")

// CreateOrderWithItems atomically: for each line, increments
// ticket_types.quantity_sold by the requested quantity IFF doing so would
// not exceed quantity_total (rejecting with ErrSoldOut otherwise); then
// inserts the order row and one order_items row per line. All of this
// happens in a single database transaction, so a concurrent purchase
// racing for the same inventory either fully commits or fully rolls back —
// never a partial decrement.
func (s *Store) CreateOrderWithItems(ctx context.Context, o *Order, lines []OrderLine) ([]OrderItem, error) {
	if len(lines) == 0 {
		return nil, errors.New("store: create order: no line items")
	}
	if o.ID == "" {
		o.ID = NewID()
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now()
	}
	if o.Status == "" {
		o.Status = "pending"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: create order: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	for _, ln := range lines {
		if ln.Quantity <= 0 {
			return nil, fmt.Errorf("store: create order: line for ticket type %s has non-positive quantity %d", ln.TicketTypeID, ln.Quantity)
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE ticket_types
			SET quantity_sold = quantity_sold + ?
			WHERE id = ? AND quantity_sold + ? <= quantity_total`,
			ln.Quantity, ln.TicketTypeID, ln.Quantity)
		if err != nil {
			return nil, fmt.Errorf("store: create order: reserve inventory: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("store: create order: rows affected: %w", err)
		}
		if n == 0 {
			// Either the ticket type doesn't exist, or reserving would
			// oversell it. Distinguish the two for a clearer error.
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ticket_types WHERE id = ?`, ln.TicketTypeID).Scan(&exists); err != nil {
				return nil, fmt.Errorf("store: create order: check ticket type existence: %w", err)
			}
			if exists == 0 {
				return nil, fmt.Errorf("%w: ticket type %s", ErrNotFound, ln.TicketTypeID)
			}
			return nil, fmt.Errorf("%w: ticket type %s", ErrSoldOut, ln.TicketTypeID)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO orders (id, event_id, user_id, buyer_email, buyer_name, status,
			subtotal_minor, fee_minor, total_minor, currency, provider, provider_ref, created_at, paid_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID, o.EventID, nullString(o.UserID), o.BuyerEmail, o.BuyerName, o.Status,
		o.SubtotalMinor, o.FeeMinor, o.TotalMinor, o.Currency, o.Provider, nullString(o.ProviderRef),
		timeToText(o.CreatedAt), nullTimeToText(o.PaidAt),
	); err != nil {
		return nil, fmt.Errorf("store: create order: %w", err)
	}

	items := make([]OrderItem, 0, len(lines))
	for _, ln := range lines {
		item := OrderItem{
			ID:             NewID(),
			OrderID:        o.ID,
			TicketTypeID:   ln.TicketTypeID,
			Quantity:       ln.Quantity,
			UnitPriceMinor: ln.UnitPriceMinor,
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO order_items (id, order_id, ticket_type_id, quantity, unit_price_minor)
			VALUES (?, ?, ?, ?, ?)`,
			item.ID, item.OrderID, item.TicketTypeID, item.Quantity, item.UnitPriceMinor,
		); err != nil {
			return nil, fmt.Errorf("store: create order item: %w", err)
		}
		items = append(items, item)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: create order: commit: %w", err)
	}
	return items, nil
}

// CancelOrderReleaseInventory marks a still-pending order as failed and
// releases the inventory it had reserved back to each ticket type
// (decrementing quantity_sold by the same amounts CreateOrderWithItems
// incremented it by). It is a no-op (returns false, nil) if the order is
// not currently pending — e.g. it was already settled or already
// cancelled — so it is safe to call defensively after a failed payment
// initiation without double-releasing inventory.
func (s *Store) CancelOrderReleaseInventory(ctx context.Context, orderID string) (released bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: cancel order: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	res, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'failed' WHERE id = ? AND status = 'pending'`, orderID)
	if err != nil {
		return false, fmt.Errorf("store: cancel order: update status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: cancel order: rows affected: %w", err)
	}
	if n == 0 {
		return false, nil
	}

	rows, err := tx.QueryContext(ctx, `SELECT ticket_type_id, quantity FROM order_items WHERE order_id = ?`, orderID)
	if err != nil {
		return false, fmt.Errorf("store: cancel order: list items: %w", err)
	}
	type line struct {
		ticketTypeID string
		quantity     int
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.ticketTypeID, &l.quantity); err != nil {
			rows.Close()
			return false, fmt.Errorf("store: cancel order: scan item: %w", err)
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, fmt.Errorf("store: cancel order: iterate items: %w", err)
	}
	rows.Close()

	for _, l := range lines {
		if _, err := tx.ExecContext(ctx, `
			UPDATE ticket_types SET quantity_sold = quantity_sold - ? WHERE id = ?`,
			l.quantity, l.ticketTypeID); err != nil {
			return false, fmt.Errorf("store: cancel order: release inventory: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: cancel order: commit: %w", err)
	}
	return true, nil
}

// SettleOrder atomically marks a pending order paid and inserts the given
// tickets, IFF the order is still in status 'pending' at the moment the
// update runs. If the order is no longer pending (e.g. a concurrent
// delivery of the same webhook already settled it first), settled is
// false and NOTHING is written — no double status flip, no duplicate
// ticket rows. This single conditional UPDATE is what makes Settle
// idempotent under concurrent/duplicate webhook delivery.
func (s *Store) SettleOrder(ctx context.Context, orderID string, paidAt time.Time, tks []Ticket) (settled bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: settle order: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	res, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = 'paid', paid_at = ? WHERE id = ? AND status = 'pending'`,
		timeToText(paidAt), orderID)
	if err != nil {
		return false, fmt.Errorf("store: settle order: update status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: settle order: rows affected: %w", err)
	}
	if n == 0 {
		return false, nil
	}

	for _, t := range tks {
		if t.ID == "" {
			t.ID = NewID()
		}
		if t.Status == "" {
			t.Status = "valid"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tickets (id, order_id, event_id, ticket_type_id, holder_user_id, holder_name,
				serial, capability, status, issued_at, voided_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.OrderID, t.EventID, t.TicketTypeID, nullString(t.HolderUserID), t.HolderName,
			t.Serial, t.Capability, t.Status, timeToText(t.IssuedAt), nullTimeToText(t.VoidedAt),
		); err != nil {
			return false, fmt.Errorf("store: settle order: insert ticket: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: settle order: commit: %w", err)
	}
	return true, nil
}

const orderSelectColumns = `SELECT id, event_id, user_id, buyer_email, buyer_name, status,
	subtotal_minor, fee_minor, total_minor, currency, provider, provider_ref, created_at, paid_at`

// GetOrderByID looks up an order by primary key. Returns ErrNotFound if
// absent.
func (s *Store) GetOrderByID(ctx context.Context, id string) (*Order, error) {
	return s.scanOrder(s.db.QueryRowContext(ctx, orderSelectColumns+` FROM orders WHERE id = ?`, id))
}

// ListOrdersForUser returns every order placed by a user, most recent
// first.
func (s *Store) ListOrdersForUser(ctx context.Context, userID string) ([]Order, error) {
	rows, err := s.db.QueryContext(ctx, orderSelectColumns+`
		FROM orders WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list orders for user: %w", err)
	}
	defer rows.Close()

	var out []Order
	for rows.Next() {
		o, err := scanOrderRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListOrdersForEvent returns every order placed against an event, most
// recent first — the organiser-facing counterpart to ListOrdersForUser
// (which is scoped to a buyer's own purchases). This backs the organiser
// orders screen, which is what makes the `manual` payment provider's
// mark-paid/mark-failed actions reachable at all — see
// internal/httpapi/orders_handlers.go.
func (s *Store) ListOrdersForEvent(ctx context.Context, eventID string) ([]Order, error) {
	rows, err := s.db.QueryContext(ctx, orderSelectColumns+`
		FROM orders WHERE event_id = ? ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list orders for event: %w", err)
	}
	defer rows.Close()

	var out []Order
	for rows.Next() {
		o, err := scanOrderRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) scanOrder(row *sql.Row) (*Order, error) {
	var o Order
	var userID, providerRef sql.NullString
	var createdAt string
	var paidAt sql.NullString

	err := row.Scan(&o.ID, &o.EventID, &userID, &o.BuyerEmail, &o.BuyerName, &o.Status,
		&o.SubtotalMinor, &o.FeeMinor, &o.TotalMinor, &o.Currency, &o.Provider, &providerRef, &createdAt, &paidAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan order: %w", err)
	}
	if err := hydrateOrder(&o, userID, providerRef, createdAt, paidAt); err != nil {
		return nil, err
	}
	return &o, nil
}

func scanOrderRow(row rowScanner) (Order, error) {
	var o Order
	var userID, providerRef sql.NullString
	var createdAt string
	var paidAt sql.NullString

	if err := row.Scan(&o.ID, &o.EventID, &userID, &o.BuyerEmail, &o.BuyerName, &o.Status,
		&o.SubtotalMinor, &o.FeeMinor, &o.TotalMinor, &o.Currency, &o.Provider, &providerRef, &createdAt, &paidAt); err != nil {
		return Order{}, fmt.Errorf("store: scan order row: %w", err)
	}
	if err := hydrateOrder(&o, userID, providerRef, createdAt, paidAt); err != nil {
		return Order{}, err
	}
	return o, nil
}

func hydrateOrder(o *Order, userID, providerRef sql.NullString, createdAt string, paidAt sql.NullString) error {
	var err error
	if userID.Valid {
		v := userID.String
		o.UserID = &v
	}
	if providerRef.Valid {
		v := providerRef.String
		o.ProviderRef = &v
	}
	if o.CreatedAt, err = textToTime(createdAt); err != nil {
		return fmt.Errorf("store: parse order created_at: %w", err)
	}
	if o.PaidAt, err = textToNullTime(paidAt); err != nil {
		return fmt.Errorf("store: parse order paid_at: %w", err)
	}
	return nil
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
