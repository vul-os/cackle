package httpapi

import (
	"net/http"
	"testing"
)

// seedManualOrder places a guest-checkout order against fx's event on the
// manual payment provider and returns its id. Manual's Begin (called by
// orders.Service.Create) always succeeds without any network call — see
// internal/payments/manual.go — so this is real order-creation-through-
// checkout, not a store-level shortcut.
func (h *testHarness) seedManualOrder(t *testing.T, fx eventFixture, buyerEmail string, qty int) string {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/orders", "", createOrderRequest{
		EventID:  fx.eventID,
		Items:    []orderItemRequest{{TicketTypeID: fx.ticketTypeID, Quantity: qty}},
		Buyer:    buyerRequest{Email: buyerEmail, Name: "Buyer"},
		Provider: "manual",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed manual order: status %d body %s", rec.Code, rec.Body.String())
	}
	created := decodeBody[struct {
		Order struct {
			ID string `json:"id"`
		} `json:"order"`
	}](t, rec)
	return created.Order.ID
}

// TestMarkOrderPaid_SettlesAndIssuesTickets is DEFECT 1's core regression:
// before this fix, a manual order could never be confirmed over HTTP at
// all (MarkPaid/MarkFailed had zero callers) — a bank-transfer/cash/
// invoice order sat pending forever and no ticket was ever issued on
// Cackle's zero-config default payment path.
func TestMarkOrderPaid_SettlesAndIssuesTickets(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "mark-paid")
	orderID := h.seedManualOrder(t, fx, "buyer-mark-paid@example.com", 2)

	// Not yet paid: no tickets should exist yet. GET /api/orders/{id} is
	// scoped to the buyer's own account (this was a guest checkout, so it
	// 404s for everyone) — the organiser's view is the event orders list.
	listRec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/orders", fx.ownerToken, nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list event orders before mark-paid: status %d body %s", listRec.Code, listRec.Body.String())
	}
	before := decodeBody[struct {
		Orders []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"orders"`
	}](t, listRec)
	if len(before.Orders) != 1 || before.Orders[0].ID != orderID {
		t.Fatalf("expected exactly the seeded order in the list, got %+v", before.Orders)
	}
	if before.Orders[0].Status != "pending" {
		t.Fatalf("expected freshly-created manual order to be pending, got %q", before.Orders[0].Status)
	}

	rec := h.do(http.MethodPost, "/api/orders/"+orderID+"/mark-paid", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("mark order paid: status %d body %s", rec.Code, rec.Body.String())
	}
	settled := decodeBody[struct {
		Order struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"order"`
		Tickets []struct {
			ID         string `json:"id"`
			Capability string `json:"capability"`
		} `json:"tickets"`
	}](t, rec)
	if settled.Order.Status != "paid" {
		t.Fatalf("expected order status paid after mark-paid, got %q", settled.Order.Status)
	}
	if len(settled.Tickets) != 2 {
		t.Fatalf("expected 2 tickets issued (quantity ordered), got %d", len(settled.Tickets))
	}
	for i, tk := range settled.Tickets {
		if tk.Capability == "" {
			t.Fatalf("ticket %d has no capability issued", i)
		}
	}

	// Idempotent: marking an already-paid order paid again must not
	// double-issue tickets.
	rec2 := h.do(http.MethodPost, "/api/orders/"+orderID+"/mark-paid", fx.ownerToken, nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second mark-paid: status %d body %s", rec2.Code, rec2.Body.String())
	}
	again := decodeBody[struct {
		Order struct {
			Status string `json:"status"`
		} `json:"order"`
		Tickets []struct{ ID string } `json:"tickets"`
	}](t, rec2)
	if again.Order.Status != "paid" {
		t.Fatalf("expected order status still paid, got %q", again.Order.Status)
	}
	if len(again.Tickets) != 2 {
		t.Fatalf("expected the same 2 tickets on repeat mark-paid (no double issuance), got %d", len(again.Tickets))
	}
	if again.Tickets[0].ID != settled.Tickets[0].ID || again.Tickets[1].ID != settled.Tickets[1].ID {
		t.Fatalf("expected identical ticket ids on repeat mark-paid, got %+v vs %+v", settled.Tickets, again.Tickets)
	}
}

// TestMarkOrderFailed_ReleasesInventory covers the companion action: an
// organiser recording that a manual order will never be paid, and the
// inventory it reserved going back on sale.
func TestMarkOrderFailed_ReleasesInventory(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "mark-failed")
	orderID := h.seedManualOrder(t, fx, "buyer-mark-failed@example.com", 1)

	rec := h.do(http.MethodPost, "/api/orders/"+orderID+"/mark-failed", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("mark order failed: status %d body %s", rec.Code, rec.Body.String())
	}
	failed := decodeBody[struct {
		Order struct {
			Status string `json:"status"`
		} `json:"order"`
	}](t, rec)
	if failed.Order.Status != "failed" {
		t.Fatalf("expected order status failed, got %q", failed.Order.Status)
	}

	// Idempotent no-op: marking an already-failed order failed again must
	// not error.
	rec2 := h.do(http.MethodPost, "/api/orders/"+orderID+"/mark-failed", fx.ownerToken, nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second mark-failed: expected 200 (idempotent no-op), got %d body %s", rec2.Code, rec2.Body.String())
	}

	// A paid order can never be marked failed after the fact.
	paidOrderID := h.seedManualOrder(t, fx, "buyer-mark-failed-2@example.com", 1)
	payRec := h.do(http.MethodPost, "/api/orders/"+paidOrderID+"/mark-paid", fx.ownerToken, nil)
	if payRec.Code != http.StatusOK {
		t.Fatalf("mark paid (setup): status %d body %s", payRec.Code, payRec.Body.String())
	}
	failAfterPaidRec := h.do(http.MethodPost, "/api/orders/"+paidOrderID+"/mark-failed", fx.ownerToken, nil)
	if failAfterPaidRec.Code != http.StatusConflict {
		t.Fatalf("mark-failed on a paid order: expected 409, got %d body %s", failAfterPaidRec.Code, failAfterPaidRec.Body.String())
	}
}

// TestMarkOrder_RejectsNonManualProvider ensures the mark-paid/mark-failed
// shortcut can't be used to bypass a real provider's own Verify/Webhook
// reconciliation — it must only ever apply to orders actually created on
// the manual provider.
func TestMarkOrder_RejectsNonManualProvider(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "mark-non-manual")

	rec := h.do(http.MethodPost, "/api/orders", "", createOrderRequest{
		EventID:  fx.eventID,
		Items:    []orderItemRequest{{TicketTypeID: fx.ticketTypeID, Quantity: 1}},
		Buyer:    buyerRequest{Email: "buyer-stub@example.com", Name: "Buyer"},
		Provider: "stub",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create stub order: status %d body %s", rec.Code, rec.Body.String())
	}
	created := decodeBody[struct {
		Order struct {
			ID string `json:"id"`
		} `json:"order"`
	}](t, rec)

	markRec := h.do(http.MethodPost, "/api/orders/"+created.Order.ID+"/mark-paid", fx.ownerToken, nil)
	if markRec.Code != http.StatusBadRequest {
		t.Fatalf("mark-paid on a non-manual order: expected 400, got %d body %s", markRec.Code, markRec.Body.String())
	}
}

// TestMarkOrder_UnknownOrder404s covers the plain not-found path.
func TestMarkOrder_UnknownOrder404s(t *testing.T) {
	h := newTestHarness(t)
	ownerToken, _ := h.signupUser("owner-mark-unknown@example.com", "owner-password-123", "Owner")
	rec := h.do(http.MethodPost, "/api/orders/nonexistent-order-id/mark-paid", ownerToken, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mark-paid unknown order: expected 404, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestListEventOrders_ReportsAuditTrail exercises the organiser orders
// screen's data source: it lists every order for the event, and once a
// manual order has been marked paid, surfaces who did it and when — the
// audit trail the manual provider's payment_records table exists for.
func TestListEventOrders_ReportsAuditTrail(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "list-orders")
	orderID := h.seedManualOrder(t, fx, "buyer-list-orders@example.com", 1)

	listBefore := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/orders", fx.ownerToken, nil)
	if listBefore.Code != http.StatusOK {
		t.Fatalf("list event orders: status %d body %s", listBefore.Code, listBefore.Body.String())
	}
	before := decodeBody[struct {
		Orders []struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			MarkedBy string `json:"marked_by,omitempty"`
		} `json:"orders"`
	}](t, listBefore)
	if len(before.Orders) != 1 || before.Orders[0].ID != orderID {
		t.Fatalf("expected exactly the seeded order in the list, got %+v", before.Orders)
	}
	if before.Orders[0].MarkedBy != "" {
		t.Fatalf("expected no marked_by before mark-paid, got %q", before.Orders[0].MarkedBy)
	}

	markRec := h.do(http.MethodPost, "/api/orders/"+orderID+"/mark-paid", fx.ownerToken, nil)
	if markRec.Code != http.StatusOK {
		t.Fatalf("mark order paid: status %d body %s", markRec.Code, markRec.Body.String())
	}

	listAfter := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/orders", fx.ownerToken, nil)
	if listAfter.Code != http.StatusOK {
		t.Fatalf("list event orders (after): status %d body %s", listAfter.Code, listAfter.Body.String())
	}
	after := decodeBody[struct {
		Orders []struct {
			ID       string  `json:"id"`
			Status   string  `json:"status"`
			MarkedBy string  `json:"marked_by,omitempty"`
			MarkedAt *string `json:"marked_at,omitempty"`
		} `json:"orders"`
	}](t, listAfter)
	if len(after.Orders) != 1 {
		t.Fatalf("expected exactly 1 order, got %d", len(after.Orders))
	}
	if after.Orders[0].Status != "paid" {
		t.Fatalf("expected order status paid, got %q", after.Orders[0].Status)
	}
	if after.Orders[0].MarkedBy == "" {
		t.Fatalf("expected marked_by to be populated after mark-paid, got %q", after.Orders[0].MarkedBy)
	}
	if after.Orders[0].MarkedAt == nil {
		t.Fatalf("expected marked_at to be populated after mark-paid")
	}
}
