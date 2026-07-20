package store

import (
	"context"
	"fmt"
)

// OrderItem is one ticket_type/quantity line within an order.
// UnitPriceMinor is frozen at the price read from ticket_types at the
// moment the order was created (see CreateOrderWithItems in orders.go) —
// it does not follow later price changes to the ticket type.
type OrderItem struct {
	ID             string
	OrderID        string
	TicketTypeID   string
	Quantity       int
	UnitPriceMinor int64
}

// ListOrderItemsForOrder returns every line item belonging to an order.
// Rows are inserted by CreateOrderWithItems; there is no standalone
// CreateOrderItem because an order item never exists without its parent
// order in the same transaction.
func (s *Store) ListOrderItemsForOrder(ctx context.Context, orderID string) ([]OrderItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, order_id, ticket_type_id, quantity, unit_price_minor
		FROM order_items WHERE order_id = ?`, orderID)
	if err != nil {
		return nil, fmt.Errorf("store: list order items: %w", err)
	}
	defer rows.Close()

	var out []OrderItem
	for rows.Next() {
		var it OrderItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.TicketTypeID, &it.Quantity, &it.UnitPriceMinor); err != nil {
			return nil, fmt.Errorf("store: scan order item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
