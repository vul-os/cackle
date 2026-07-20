package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// orderLookupAdapter satisfies payments.OrderLookup. Per internal/payments'
// Order doc comment, an order's ID IS its provider reference — Cackle
// never invents a separate reference — so Lookup is just orders.Get by
// that same id. This is the caller's own database read Reconcile requires;
// it never trusts anything the client or provider claims about the order
// beyond the reference used to look it up.
type orderLookupAdapter struct{ orders *orders.Service }

func (a orderLookupAdapter) Lookup(ctx context.Context, reference string) (payments.OrderRef, error) {
	o, err := a.orders.Get(ctx, reference)
	if err != nil {
		return payments.OrderRef{}, err
	}
	return payments.OrderRef{ID: o.ID, TotalCents: o.TotalCents, Currency: o.Currency}, nil
}

// memorySeenStore is a process-lifetime payments.SeenStore for webhook
// replay protection. It does NOT survive a restart — that's an accepted
// trade-off, because the authoritative idempotency guarantee against
// double ticket issuance is orders.Service.Settle itself (documented as
// idempotent per reference), not this. This store only exists to reject
// obvious rapid-fire replays cheaply before ever reaching Settle.
type memorySeenStore struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func newMemorySeenStore() *memorySeenStore {
	return &memorySeenStore{seen: make(map[string]struct{})}
}

func (m *memorySeenStore) MarkSeen(_ context.Context, provider, eventID string) (bool, error) {
	key := provider + ":" + eventID
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.seen[key]; ok {
		return false, nil
	}
	m.seen[key] = struct{}{}
	return true, nil
}

type verifyPaymentRequest struct {
	Reference string `json:"reference"`
}

// handleVerifyPayment serves POST /api/payments/verify. Public (no
// requireUser) — a guest checkout buyer polling with the reference they
// were handed at checkout time never had to be logged in. Fails closed:
// any verification ambiguity is reported as "not confirmed", never as
// paid.
func (s *server) handleVerifyPayment(w http.ResponseWriter, r *http.Request) {
	var req verifyPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Reference == "" {
		badRequest(w, "reference is required")
		return
	}

	order, err := s.deps.Orders.Get(r.Context(), req.Reference)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "order not found")
			return
		}
		internalError(w, s.log(), "verify payment: get order", err)
		return
	}

	provider, ok := s.deps.Payments.Get(order.Provider)
	if !ok {
		internalError(w, s.log(), "verify payment: provider not registered", errors.New("provider not registered"))
		return
	}

	result, err := payments.HandleVerify(r.Context(), provider, req.Reference, orderLookupAdapter{s.deps.Orders})
	if err != nil {
		// Fail closed: transport errors, ambiguous status, and amount/
		// currency mismatches all land here. Never report paid.
		writeError(w, http.StatusPaymentRequired, "payment_not_confirmed", "payment could not be verified as paid")
		return
	}

	_, tickets, err := s.deps.Orders.Settle(r.Context(), result)
	if err != nil {
		internalError(w, s.log(), "verify payment: settle", err)
		return
	}

	updated, err := s.deps.Orders.Get(r.Context(), req.Reference)
	if err != nil {
		internalError(w, s.log(), "verify payment: reload order", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": updated, "tickets": tickets})
}

// handleWebhook serves POST /api/payments/webhook/{provider}. The raw
// request body is read (for HMAC verification) entirely inside
// provider.Webhook / payments.HandleWebhook — nothing in this handler or
// the middleware chain ahead of it decodes JSON or otherwise consumes
// r.Body first, which is what makes that signature check meaningful.
// Fails closed: a missing/invalid signature, malformed payload, or
// amount/currency mismatch is rejected outright, never acknowledged as a
// processed payment.
func (s *server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	provider, ok := s.deps.Payments.Get(providerName)
	if !ok {
		notFound(w, "unknown payment provider")
		return
	}

	result, err := payments.HandleWebhook(r.Context(), provider, r, s.webhookSeen, orderLookupAdapter{s.deps.Orders})
	if err != nil {
		if errors.Is(err, payments.ErrUnhandledEvent) || errors.Is(err, payments.ErrReplayed) {
			// A validly-signed event this build doesn't treat as a
			// settlement, or one already processed: ack with 200 so the
			// provider doesn't retry forever, but take no further action.
			w.WriteHeader(http.StatusOK)
			return
		}
		// Never log the raw payload/signature/secret — only that a
		// webhook was rejected and for which provider.
		s.log().Warn("webhook rejected", "provider", providerName)
		writeError(w, http.StatusBadRequest, codeInvalidRequest, "webhook rejected")
		return
	}

	if _, _, err := s.deps.Orders.Settle(r.Context(), result); err != nil {
		internalError(w, s.log(), "webhook settle", err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
