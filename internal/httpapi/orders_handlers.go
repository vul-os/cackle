package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/payments"
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

// manualMarker is implemented by payment providers that support an
// organiser explicitly recording a settlement decision out of band — only
// payments.ManualProvider today (see internal/payments/manual.go). Handlers
// type-assert to this narrow interface rather than adding
// MarkPaid/MarkFailed to payments.Provider itself, since no other provider
// has (or should have) a manual override path.
type manualMarker interface {
	MarkPaid(ctx context.Context, reference, markedBy string) (payments.Result, error)
	MarkFailed(ctx context.Context, reference, markedBy string) (payments.Result, error)
}

// manualRecordLookup is implemented by payment providers that can report
// their own durable audit trail for a reference — only
// payments.ManualProvider today. Used by handleListEventOrders to surface
// marked_by/marked_at to the organiser orders screen.
type manualRecordLookup interface {
	Record(reference string) (payments.ManualRecord, bool)
}

// orderView is the organiser-facing shape of an order in the orders list:
// the order itself, plus the manual provider's audit fields (who marked it
// paid/failed, and when) when applicable. Those fields live in the
// payment_records table via the manual provider (see
// internal/payments/manual.go's ManualRecord) — orders.Order itself does
// not carry them, since every other provider settles via its own
// Verify/Webhook and has no human-in-the-loop marker to record.
type orderView struct {
	orders.Order
	MarkedBy string     `json:"marked_by,omitempty"`
	MarkedAt *time.Time `json:"marked_at,omitempty"`
}

// handleListEventOrders serves GET /api/events/{id}/orders — admin+ on the
// event's org. This is the organiser orders screen's data source: every
// order placed against the event, most recent first, with enough to
// reconcile a bank transfer/cash/invoice payment against the manual
// provider's buyer-facing instructions and then act on it via
// handleMarkOrderPaid/handleMarkOrderFailed. Admin+ (not scanner) because
// this exposes buyer email and payment amounts — the same bar as
// event payouts and the org bank account, not the scanner-readable
// attendee roster (which deliberately omits buyer_email — see
// attendee_handlers.go).
func (s *server) handleListEventOrders(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "list event orders rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	list, err := s.deps.Orders.ListForEvent(r.Context(), eventID)
	if err != nil {
		internalError(w, s.log(), "list event orders", err)
		return
	}

	views := make([]orderView, len(list))
	for i, ord := range list {
		views[i] = orderView{Order: ord}
		if ord.Provider != payments.ProviderNameManual {
			continue
		}
		provider, ok := s.deps.Payments.Get(ord.Provider)
		if !ok {
			continue
		}
		lookup, ok := provider.(manualRecordLookup)
		if !ok {
			continue
		}
		rec, ok := lookup.Record(ord.ID)
		if !ok || rec.MarkedBy == "" {
			continue
		}
		views[i].MarkedBy = rec.MarkedBy
		markedAt := rec.MarkedAt
		views[i].MarkedAt = &markedAt
	}

	writeJSON(w, http.StatusOK, map[string]any{"orders": views})
}

// handleMarkOrderPaid serves POST /api/orders/{id}/mark-paid — admin+ on
// the order's event's org. This is the organiser-facing action that makes
// Cackle's default `manual` payment provider (see
// internal/payments/manual.go) actually usable end to end: an organiser who
// received a bank transfer, cash at the door, or a paid invoice records
// that here, and — exactly like any other provider's webhook or verify
// poll — the order is settled and tickets are issued via
// orders.Service.Settle. Without this route, a manual order could never be
// confirmed and no ticket would ever be issued on Cackle's zero-config
// default path.
//
// Only orders created with the manual provider can be marked this way;
// every other provider settles exclusively through its own Verify/Webhook.
//
// Idempotent: calling this twice for the same order does not double-issue
// tickets. The manual provider's own MarkPaid is idempotent once already
// paid (it does not overwrite the original marked_by/marked_at), and
// orders.Service.Settle is idempotent per its own doc comment — a second
// call just returns the tickets issued the first time.
func (s *server) handleMarkOrderPaid(w http.ResponseWriter, r *http.Request) {
	s.handleMarkOrder(w, r, true)
}

// handleMarkOrderFailed serves POST /api/orders/{id}/mark-failed — admin+
// on the order's event's org. Records that a manual order will never be
// paid (buyer backed out, duplicate order, ...) and releases the inventory
// it had reserved back to sale — see orders.Service.MarkFailed.
func (s *server) handleMarkOrderFailed(w http.ResponseWriter, r *http.Request) {
	s.handleMarkOrder(w, r, false)
}

// handleMarkOrder is the shared implementation behind
// handleMarkOrderPaid/handleMarkOrderFailed: look up the order, enforce
// admin+ RBAC on its event's org, refuse anything not on the manual
// provider, then either settle it paid (issuing tickets) or mark it failed
// (releasing reserved inventory).
func (s *server) handleMarkOrder(w http.ResponseWriter, r *http.Request, paid bool) {
	user, _ := userFromContext(r.Context())
	orderID := chi.URLParam(r, "id")

	ord, err := s.deps.Orders.Get(r.Context(), orderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "order not found")
			return
		}
		internalError(w, s.log(), "mark order: get order", err)
		return
	}

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, ord.EventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "mark order rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	if ord.Provider != payments.ProviderNameManual {
		badRequest(w, "order was not created with the manual payment provider")
		return
	}

	provider, ok := s.deps.Payments.Get(ord.Provider)
	if !ok {
		internalError(w, s.log(), "mark order: provider lookup", fmt.Errorf("registered order provider %q not found in registry", ord.Provider))
		return
	}
	marker, ok := provider.(manualMarker)
	if !ok {
		internalError(w, s.log(), "mark order: provider capability", fmt.Errorf("provider %q does not support marking", ord.Provider))
		return
	}

	// markedBy is the auditable "who" the manual provider's audit trail
	// requires (see manual.go's ManualRecord.MarkedBy) — the caller's own
	// email, since RBAC already proved they're an admin/owner of this
	// event's org.
	markedBy := user.Email

	if paid {
		if ord.Status != "pending" && ord.Status != "paid" {
			conflict(w, fmt.Sprintf("order cannot be marked paid from its current status (%s)", ord.Status))
			return
		}
		result, err := marker.MarkPaid(r.Context(), orderID, markedBy)
		if err != nil {
			internalError(w, s.log(), "mark order paid: provider", err)
			return
		}
		updated, tks, err := s.deps.Orders.Settle(r.Context(), result)
		if err != nil {
			if errors.Is(err, orders.ErrOrderNotSettleable) {
				conflict(w, "order could not be settled from its current status")
				return
			}
			internalError(w, s.log(), "mark order paid: settle", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"order": updated, "tickets": tks})
		return
	}

	if ord.Status == "failed" {
		// Idempotent no-op: already resolved as failed, nothing left to
		// release a second time.
		writeJSON(w, http.StatusOK, map[string]any{"order": ord})
		return
	}
	if ord.Status != "pending" {
		conflict(w, fmt.Sprintf("order cannot be marked failed from its current status (%s)", ord.Status))
		return
	}
	if _, err := marker.MarkFailed(r.Context(), orderID, markedBy); err != nil {
		internalError(w, s.log(), "mark order failed: provider", err)
		return
	}
	updated, err := s.deps.Orders.MarkFailed(r.Context(), orderID)
	if err != nil {
		if errors.Is(err, orders.ErrOrderNotPending) {
			conflict(w, "order could not be marked failed from its current status")
			return
		}
		internalError(w, s.log(), "mark order failed: release inventory", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": updated})
}
