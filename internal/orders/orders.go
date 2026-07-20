// Package orders owns Cackle's cart/checkout/ticket-issuance domain logic:
// creating orders against real, database-sourced prices and inventory, and
// settling them into issued ticket capabilities once a payment provider
// confirms funds landed.
//
// Two invariants matter more than anything else in this package:
//
//  1. No oversell. Reserving inventory (ticket_types.quantity_sold) happens
//     in the exact same database transaction as creating the order and its
//     order_items — see internal/store.CreateOrderWithItems. A purchase
//     that would exceed a ticket type's quantity_total is rejected with
//     ErrSoldOut, and under concurrent purchases racing for the same last
//     unit, exactly one wins.
//  2. The client never sets a price. CreateOrderInput carries only
//     ticket_type_id + quantity per line — there is no price field to
//     forge. Every cent charged is read from ticket_types.price_cents at
//     order-creation time.
//
// Settle is idempotent: calling it twice for the same payment reference
// must issue tickets exactly once. See Settle's doc comment for how that is
// enforced.
package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
	"github.com/vul-os/cackle/internal/tickets"
)

// Sentinel errors. store.ErrNotFound is returned as-is (not wrapped) where
// the missing thing is unambiguous from context (e.g. Get, Ticket).
var (
	// ErrEmptyOrder is returned by Create when Items is empty.
	ErrEmptyOrder = errors.New("orders: order must contain at least one item")
	// ErrInvalidQuantity is returned by Create for a non-positive quantity.
	ErrInvalidQuantity = errors.New("orders: quantity must be positive")
	// ErrTicketTypeNotFound is returned by Create when a requested
	// ticket_type_id does not exist, or does not belong to the requested
	// event.
	ErrTicketTypeNotFound = errors.New("orders: ticket type not found for this event")
	// ErrTicketTypeNotAvailable is returned by Create when a ticket type
	// is not currently on sale (wrong status, or outside its sales
	// window).
	ErrTicketTypeNotAvailable = errors.New("orders: ticket type is not currently on sale")
	// ErrMaxPerOrderExceeded is returned by Create when a line's quantity
	// exceeds the ticket type's max_per_order.
	ErrMaxPerOrderExceeded = errors.New("orders: quantity exceeds this ticket type's max per order")
	// ErrEventNotPublished is returned by Create when the event is not
	// (yet, or no longer) published — draft and cancelled events cannot
	// be ordered against.
	ErrEventNotPublished = errors.New("orders: event is not published")
	// ErrSoldOut is returned by Create when a line would exceed a ticket
	// type's remaining inventory. This is the cardinal no-oversell
	// guarantee — see internal/store.ErrSoldOut, which this wraps.
	ErrSoldOut = errors.New("orders: sold out")
	// ErrProviderRequired is returned by Create when CreateOrderInput.Provider
	// is empty and the payments Registry has anything other than exactly
	// one registered provider (so there is no unambiguous default).
	ErrProviderRequired = errors.New("orders: a payment provider must be specified")
	// ErrUnknownProvider is returned by Create when the requested provider
	// name is not registered.
	ErrUnknownProvider = errors.New("orders: unknown payment provider")
	// ErrOrderNotSettleable is returned by Settle when the order is in a
	// terminal status other than "paid" (e.g. "failed", "refunded",
	// "cancelled") and therefore cannot be settled.
	ErrOrderNotSettleable = errors.New("orders: order cannot be settled from its current status")
)

// Order is a buyer's purchase of one or more ticket types for an event.
type Order struct {
	ID            string      `json:"id"`
	EventID       string      `json:"event_id"`
	UserID        string      `json:"user_id,omitempty"` // empty if the order was placed without an account
	BuyerEmail    string      `json:"buyer_email"`
	BuyerName     string      `json:"buyer_name"`
	Status        string      `json:"status"` // "pending", "paid", "failed", "refunded", "cancelled"
	SubtotalCents int64       `json:"subtotal_cents"`
	FeeCents      int64       `json:"fee_cents"`
	TotalCents    int64       `json:"total_cents"`
	Currency      string      `json:"currency"`
	Provider      string      `json:"provider"`
	ProviderRef   string      `json:"provider_ref,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	PaidAt        *time.Time  `json:"paid_at,omitempty"`
	Items         []OrderItem `json:"items,omitempty"`
}

// OrderItem is one ticket_type/quantity line within an order.
// UnitPriceCents is the price actually charged, frozen at order-creation
// time from the ticket type's own price_cents — never client-supplied.
type OrderItem struct {
	ID             string `json:"id"`
	TicketTypeID   string `json:"ticket_type_id"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

// Ticket is one issued, signed capability belonging to a settled order.
type Ticket struct {
	ID           string     `json:"id"`
	OrderID      string     `json:"order_id"`
	EventID      string     `json:"event_id"`
	TicketTypeID string     `json:"ticket_type_id"`
	HolderUserID string     `json:"holder_user_id,omitempty"`
	HolderName   string     `json:"holder_name"`
	Serial       string     `json:"serial"`
	Capability   string     `json:"capability"`
	Status       string     `json:"status"` // "valid", "void", "refunded"
	IssuedAt     time.Time  `json:"issued_at"`
	VoidedAt     *time.Time `json:"voided_at,omitempty"`
}

// OrderItemInput is a requested line item: which ticket type, how many.
// There is deliberately no price field — the client cannot forge a price
// because there is nowhere in this struct to put one.
type OrderItemInput struct {
	TicketTypeID string `json:"ticket_type_id"`
	Quantity     int    `json:"quantity"`
}

// CreateOrderInput is the input to Create.
type CreateOrderInput struct {
	EventID     string           `json:"event_id"`
	UserID      string           `json:"-"` // set server-side from the session, never from the request body
	BuyerEmail  string           `json:"buyer_email"`
	BuyerName   string           `json:"buyer_name"`
	Items       []OrderItemInput `json:"items"`
	Provider    string           `json:"provider,omitempty"` // payment provider name; required unless exactly one is registered
	CallbackURL string           `json:"callback_url,omitempty"`
}

// Service is the entrypoint for all order/checkout/ticket-issuance
// operations.
type Service struct {
	store    *store.Store
	events   *events.Service
	payments *payments.Registry
}

// New constructs an orders Service backed by st, using ev to validate
// events and sign issued tickets, and reg to look up payment providers by
// name.
func New(st *store.Store, ev *events.Service, reg *payments.Registry) *Service {
	return &Service{store: st, events: ev, payments: reg}
}

// Create validates a cart against the database (event must be published;
// every ticket type must exist, belong to the event, be on sale, and have
// enough remaining inventory), atomically reserves that inventory and
// creates the order, then begins a charge with the requested (or sole
// registered) payment provider.
//
// Every price in the resulting order comes from ticket_types.price_cents
// as read inside this call — in.Items carries only ticket_type_id and
// quantity, so a forged price in the request has nothing to attach to.
func (s *Service) Create(ctx context.Context, in CreateOrderInput) (*Order, *payments.Charge, error) {
	if len(in.Items) == 0 {
		return nil, nil, ErrEmptyOrder
	}

	ev, err := s.events.Get(ctx, in.EventID)
	if err != nil {
		return nil, nil, err
	}
	if ev.Status != "published" {
		return nil, nil, ErrEventNotPublished
	}

	now := time.Now()
	lines := make([]store.OrderLine, 0, len(in.Items))
	var subtotal int64

	for _, item := range in.Items {
		if item.Quantity <= 0 {
			return nil, nil, ErrInvalidQuantity
		}
		tt, err := s.store.GetTicketTypeByID(ctx, item.TicketTypeID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrTicketTypeNotFound
		}
		if err != nil {
			return nil, nil, fmt.Errorf("orders: create: look up ticket type: %w", err)
		}
		if tt.EventID != in.EventID {
			return nil, nil, ErrTicketTypeNotFound
		}
		if tt.Status != "active" {
			return nil, nil, ErrTicketTypeNotAvailable
		}
		if tt.SalesStart != nil && now.Before(*tt.SalesStart) {
			return nil, nil, ErrTicketTypeNotAvailable
		}
		if tt.SalesEnd != nil && now.After(*tt.SalesEnd) {
			return nil, nil, ErrTicketTypeNotAvailable
		}
		if tt.MaxPerOrder > 0 && item.Quantity > tt.MaxPerOrder {
			return nil, nil, ErrMaxPerOrderExceeded
		}

		// The price charged is ALWAYS tt.PriceCents, read from the
		// database right here — in.Items has no field a client could use
		// to smuggle a different amount.
		unit := tt.PriceCents
		subtotal += unit * int64(item.Quantity)
		lines = append(lines, store.OrderLine{
			TicketTypeID:   tt.ID,
			Quantity:       item.Quantity,
			UnitPriceCents: unit,
		})
	}

	provider, err := s.resolveProvider(in.Provider)
	if err != nil {
		return nil, nil, err
	}

	currency := ev.Currency
	if currency == "" {
		currency = "ZAR"
	}

	orderID := store.NewID()
	ord := &store.Order{
		ID:            orderID,
		EventID:       in.EventID,
		BuyerEmail:    in.BuyerEmail,
		BuyerName:     in.BuyerName,
		Status:        "pending",
		SubtotalCents: subtotal,
		FeeCents:      0,
		TotalCents:    subtotal,
		Currency:      currency,
		Provider:      provider.Name(),
		ProviderRef:   &orderID, // the order ID is always the provider reference
		CreatedAt:     now,
	}
	if in.UserID != "" {
		uid := in.UserID
		ord.UserID = &uid
	}

	items, err := s.store.CreateOrderWithItems(ctx, ord, lines)
	if err != nil {
		if errors.Is(err, store.ErrSoldOut) {
			return nil, nil, fmt.Errorf("%w: %v", ErrSoldOut, err)
		}
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrTicketTypeNotFound
		}
		return nil, nil, fmt.Errorf("orders: create: %w", err)
	}

	charge, err := provider.Begin(ctx, payments.Order{
		ID:          ord.ID,
		EventID:     ord.EventID,
		BuyerEmail:  ord.BuyerEmail,
		BuyerName:   ord.BuyerName,
		TotalCents:  ord.TotalCents,
		Currency:    ord.Currency,
		CallbackURL: in.CallbackURL,
	})
	if err != nil {
		// Begin failed after inventory was already reserved and the order
		// created: release the reservation and mark the order failed
		// rather than leaving capacity stuck on a payment that never
		// started. Best-effort — the original Begin error is what we
		// report either way.
		_, _ = s.store.CancelOrderReleaseInventory(ctx, ord.ID)
		return nil, nil, fmt.Errorf("orders: create: begin payment: %w", err)
	}

	out := toOrder(ord, items)
	return &out, &charge, nil
}

func (s *Service) resolveProvider(name string) (payments.Provider, error) {
	if name == "" {
		names := s.payments.Names()
		if len(names) != 1 {
			return nil, ErrProviderRequired
		}
		name = names[0]
	}
	p, ok := s.payments.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, name)
	}
	return p, nil
}

// Get looks up an order (with its line items) by ID. Returns
// store.ErrNotFound if absent.
func (s *Service) Get(ctx context.Context, orderID string) (*Order, error) {
	ord, err := s.store.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListOrderItemsForOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("orders: get: %w", err)
	}
	out := toOrder(ord, items)
	return &out, nil
}

// ListForUser returns every order placed by a user, most recent first.
// Line items are not populated — call Get for a single order's items.
func (s *Service) ListForUser(ctx context.Context, userID string) ([]Order, error) {
	rows, err := s.store.ListOrdersForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Order, len(rows))
	for i := range rows {
		out[i] = toOrder(&rows[i], nil)
	}
	return out, nil
}

// Settle reconciles a payment provider's settlement result against the
// order it claims to be about, and — the first time this succeeds for a
// given order — issues one signed ticket capability per unit purchased.
//
// Idempotency: a webhook (or verify poll) can legitimately be delivered
// more than once for the same reference. Settle handles this in two
// layers:
//
//  1. If the order is already "paid" by the time Settle looks it up, it
//     skips straight to returning the tickets that were issued the first
//     time — no new tickets, no re-signing, no error.
//  2. If two calls race past that check concurrently, the actual state
//     transition (order status pending -> paid, plus inserting the ticket
//     rows) happens in a single conditional UPDATE ... WHERE status =
//     'pending' inside internal/store.SettleOrder. Only one caller's
//     transaction can ever see status still "pending" and win; the loser
//     discards the tickets it speculatively signed and instead returns
//     whatever the winner actually persisted.
//
// Either way, tickets are issued exactly once per order.
func (s *Service) Settle(ctx context.Context, res payments.Result) (*Order, []Ticket, error) {
	ord, err := s.store.GetOrderByID(ctx, res.Reference)
	if err != nil {
		return nil, nil, err
	}

	if err := payments.Reconcile(res, payments.OrderRef{
		ID:         ord.ID,
		TotalCents: ord.TotalCents,
		Currency:   ord.Currency,
	}); err != nil {
		return nil, nil, err
	}

	if ord.Status == "paid" {
		return s.alreadySettled(ctx, ord)
	}
	if ord.Status != "pending" {
		return nil, nil, fmt.Errorf("%w: status=%q", ErrOrderNotSettleable, ord.Status)
	}

	items, err := s.store.ListOrderItemsForOrder(ctx, ord.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("orders: settle: %w", err)
	}

	paidAt := res.PaidAt
	if paidAt.IsZero() {
		paidAt = time.Now()
	}

	toInsert := make([]store.Ticket, 0)
	for _, item := range items {
		for i := 0; i < item.Quantity; i++ {
			tid := store.NewID()
			payload := tickets.Payload{
				TID:  tid,
				EID:  ord.EventID,
				TT:   item.TicketTypeID,
				Sub:  valueOrEmpty(ord.UserID),
				Name: ord.BuyerName,
				IAT:  paidAt.Unix(),
			}
			cap, _, err := s.events.IssueTicket(ctx, ord.EventID, payload)
			if err != nil {
				return nil, nil, fmt.Errorf("orders: settle: issue ticket: %w", err)
			}
			toInsert = append(toInsert, store.Ticket{
				ID:           tid,
				OrderID:      ord.ID,
				EventID:      ord.EventID,
				TicketTypeID: item.TicketTypeID,
				HolderUserID: ord.UserID,
				HolderName:   ord.BuyerName,
				Serial:       tid,
				Capability:   cap,
				Status:       "valid",
				IssuedAt:     paidAt,
			})
		}
	}

	settled, err := s.store.SettleOrder(ctx, ord.ID, paidAt, toInsert)
	if err != nil {
		return nil, nil, fmt.Errorf("orders: settle: %w", err)
	}
	if !settled {
		// Lost the race: another delivery of this same event already
		// settled the order first. Discard what we just signed and
		// return whatever actually got persisted, so this call is still
		// idempotent from the caller's point of view.
		fresh, err := s.store.GetOrderByID(ctx, ord.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("orders: settle: %w", err)
		}
		return s.alreadySettled(ctx, fresh)
	}

	fresh, err := s.store.GetOrderByID(ctx, ord.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("orders: settle: %w", err)
	}
	out := toOrder(fresh, nil)
	return &out, toTicketsFromStore(toInsert), nil
}

func (s *Service) alreadySettled(ctx context.Context, ord *store.Order) (*Order, []Ticket, error) {
	existing, err := s.store.ListTicketsForOrder(ctx, ord.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("orders: settle: %w", err)
	}
	out := toOrder(ord, nil)
	return &out, toTicketsFromStore(existing), nil
}

// TicketsForUser returns every ticket held by a user, most recently issued
// first.
func (s *Service) TicketsForUser(ctx context.Context, userID string) ([]Ticket, error) {
	rows, err := s.store.ListTicketsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toTicketsFromStore(rows), nil
}

// Ticket looks up a single ticket by ID. Returns store.ErrNotFound if
// absent.
func (s *Service) Ticket(ctx context.Context, ticketID string) (*Ticket, error) {
	t, err := s.store.GetTicketByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	out := toTicketFromStore(t)
	return &out, nil
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toOrder(o *store.Order, items []store.OrderItem) Order {
	out := Order{
		ID:            o.ID,
		EventID:       o.EventID,
		UserID:        valueOrEmpty(o.UserID),
		BuyerEmail:    o.BuyerEmail,
		BuyerName:     o.BuyerName,
		Status:        o.Status,
		SubtotalCents: o.SubtotalCents,
		FeeCents:      o.FeeCents,
		TotalCents:    o.TotalCents,
		Currency:      o.Currency,
		Provider:      o.Provider,
		ProviderRef:   valueOrEmpty(o.ProviderRef),
		CreatedAt:     o.CreatedAt,
		PaidAt:        o.PaidAt,
	}
	if items != nil {
		out.Items = make([]OrderItem, len(items))
		for i, it := range items {
			out.Items[i] = OrderItem{
				ID:             it.ID,
				TicketTypeID:   it.TicketTypeID,
				Quantity:       it.Quantity,
				UnitPriceCents: it.UnitPriceCents,
			}
		}
	}
	return out
}

func toTicketsFromStore(rows []store.Ticket) []Ticket {
	out := make([]Ticket, len(rows))
	for i := range rows {
		out[i] = toTicketFromStore(&rows[i])
	}
	return out
}

func toTicketFromStore(t *store.Ticket) Ticket {
	return Ticket{
		ID:           t.ID,
		OrderID:      t.OrderID,
		EventID:      t.EventID,
		TicketTypeID: t.TicketTypeID,
		HolderUserID: valueOrEmpty(t.HolderUserID),
		HolderName:   t.HolderName,
		Serial:       t.Serial,
		Capability:   t.Capability,
		Status:       t.Status,
		IssuedAt:     t.IssuedAt,
		VoidedAt:     t.VoidedAt,
	}
}
