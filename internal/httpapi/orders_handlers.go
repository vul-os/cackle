package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/store"
)

// NOTE on internal/orders field names: as with internal/events (see
// event_handlers.go), only Service method SIGNATURES were fixed ahead of
// time by WAVE2-CONTRACT.md; orders.CreateOrderInput's own field names are
// an informed guess reconciled against the real package once it landed —
// see the final report.

type orderItemRequest struct {
	TicketTypeID string `json:"ticket_type_id"`
	Quantity     int    `json:"quantity"`
}

type buyerRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type createOrderRequest struct {
	EventID string             `json:"event_id"`
	Items   []orderItemRequest `json:"items"`
	Buyer   buyerRequest       `json:"buyer"`
	// Provider optionally selects which registered payments.Provider to
	// use (e.g. "stub" in --demo, "paystack" in production). Empty means
	// let orders.Service apply its own default.
	Provider string `json:"provider,omitempty"`
}

// handleCreateOrder serves POST /api/orders. Buyer auth is optional — the
// `orders.user_id` column is nullable specifically for guest checkout
// (schema comment: ON DELETE SET NULL); an authenticated caller's user id
// is still attached server-side when present so "GET /api/orders mine"
// has something to scope against. Price is NEVER taken from the client:
// only ticket_type_id + quantity are read from items — orders.Service is
// responsible for pricing from its own stored ticket_types row.
func (s *server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.EventID == "" || len(req.Items) == 0 || req.Buyer.Email == "" {
		badRequest(w, "event_id, at least one item, and buyer.email are required")
		return
	}

	items := make([]orders.OrderItemInput, 0, len(req.Items))
	for _, it := range req.Items {
		if it.TicketTypeID == "" || it.Quantity <= 0 {
			badRequest(w, "each item requires ticket_type_id and a positive quantity")
			return
		}
		items = append(items, orders.OrderItemInput{TicketTypeID: it.TicketTypeID, Quantity: it.Quantity})
	}

	in := orders.CreateOrderInput{
		EventID:    req.EventID,
		Items:      items,
		BuyerEmail: req.Buyer.Email,
		BuyerName:  req.Buyer.Name,
		Provider:   req.Provider,
	}
	if user, ok := userFromContext(r.Context()); ok {
		in.UserID = user.ID
	}

	order, charge, err := s.deps.Orders.Create(r.Context(), in)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event or ticket type not found")
			return
		}
		// orders.Service is documented to reject oversell — surface that
		// as a conflict, everything else as a generic 400 rather than
		// guessing at more sentinel errors that may not exist yet.
		badRequest(w, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"order": order,
		"payment": map[string]any{
			"provider":     charge.Provider,
			"redirect_url": charge.RedirectURL,
			"reference":    charge.Reference,
			"instructions": charge.Instructions,
		},
	})
}

// handleListMyOrders serves GET /api/orders (mine).
func (s *server) handleListMyOrders(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	list, err := s.deps.Orders.ListForUser(r.Context(), user.ID)
	if err != nil {
		internalError(w, s.log(), "list orders for user", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": list})
}

// handleGetOrder serves GET /api/orders/{id}. Ownership is enforced: an
// order belonging to a different user is reported as 404, not 403 — this
// avoids confirming the order id exists at all to a caller who doesn't own
// it.
func (s *server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	id := chi.URLParam(r, "id")

	order, err := s.deps.Orders.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "order not found")
			return
		}
		internalError(w, s.log(), "get order", err)
		return
	}
	if order.UserID == "" || order.UserID != user.ID {
		notFound(w, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": order})
}
