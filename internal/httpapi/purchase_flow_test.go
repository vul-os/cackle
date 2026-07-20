package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/events"
)

// eventFixture creates an owner user + org + a published event with one
// ticket type, entirely over HTTP (except org creation, which has no
// route in BUILD-SPEC — see newOrgWithOwner).
type eventFixture struct {
	ownerToken   string
	ownerID      string
	orgID        string
	eventID      string
	ticketTypeID string
}

func (h *testHarness) newPublishedEvent(t *testing.T, emailSuffix string) eventFixture {
	t.Helper()
	ownerToken, ownerID := h.signupUser("owner-"+emailSuffix+"@example.com", "owner-password-123", "Owner")
	orgID := h.newOrgWithOwner("Test Org "+emailSuffix, "test-org-"+emailSuffix, ownerID)

	starts := time.Now().Add(30 * 24 * time.Hour)
	ends := starts.Add(4 * time.Hour)
	createBody := struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: orgID,
		CreateEventInput: events.CreateEventInput{
			Slug: "test-event-" + emailSuffix, Title: "Test Event " + emailSuffix,
			VenueName: "Test Venue", Address: "1 Test Street",
			StartsAt: starts, EndsAt: ends, Timezone: "Africa/Johannesburg", Currency: "ZAR",
		},
	}
	rec := h.do(http.MethodPost, "/api/events", ownerToken, createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create event: status %d body %s", rec.Code, rec.Body.String())
	}
	created := decodeBody[struct {
		Event events.Event `json:"event"`
	}](t, rec)

	ttBody := events.TicketTypeInput{Name: "General", PriceMinor: 15000, QuantityTotal: 5, MaxPerOrder: 5, Status: "active"}
	rec = h.do(http.MethodPost, "/api/events/"+created.Event.ID+"/ticket-types", ownerToken, ttBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create ticket type: status %d body %s", rec.Code, rec.Body.String())
	}
	tt := decodeBody[struct {
		TicketType events.TicketType `json:"ticket_type"`
	}](t, rec)

	rec = h.do(http.MethodPost, "/api/events/"+created.Event.ID+"/publish", ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish event: status %d body %s", rec.Code, rec.Body.String())
	}

	return eventFixture{
		ownerToken: ownerToken, ownerID: ownerID, orgID: orgID,
		eventID: created.Event.ID, ticketTypeID: tt.TicketType.ID,
	}
}

func TestFullPurchaseSettleTicketScanFlow(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "purchase")

	// Public listing shows the published event.
	rec := h.do(http.MethodGet, "/api/events", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list public events: status %d body %s", rec.Code, rec.Body.String())
	}

	// Public event detail by slug includes ticket types + issuer keys.
	rec = h.do(http.MethodGet, "/api/events/test-event-purchase", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get public event: status %d body %s", rec.Code, rec.Body.String())
	}

	// Buyer signs up and places an order.
	buyerToken, _ := h.signupUser("buyer-purchase@example.com", "buyer-password-123", "Buyer")

	orderReq := createOrderRequest{
		EventID:  fx.eventID,
		Items:    []orderItemRequest{{TicketTypeID: fx.ticketTypeID, Quantity: 2}},
		Buyer:    buyerRequest{Email: "buyer-purchase@example.com", Name: "Buyer"},
		Provider: "stub",
	}
	rec = h.do(http.MethodPost, "/api/orders", buyerToken, orderReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order: status %d body %s", rec.Code, rec.Body.String())
	}
	created := decodeBody[struct {
		Order struct {
			ID         string `json:"id"`
			TotalMinor int64  `json:"total_minor"`
		} `json:"order"`
		Payment struct {
			Reference string `json:"reference"`
		} `json:"payment"`
	}](t, rec)
	if created.Order.TotalMinor != 30000 {
		t.Fatalf("expected total_minor 30000 (2x15000), got %d", created.Order.TotalMinor)
	}

	// Verify settles the order and issues tickets.
	rec = h.do(http.MethodPost, "/api/payments/verify", "", verifyPaymentRequest{Reference: created.Payment.Reference})
	if rec.Code != http.StatusOK {
		t.Fatalf("verify payment: status %d body %s", rec.Code, rec.Body.String())
	}
	settled := decodeBody[struct {
		Order struct {
			Status string `json:"status"`
		} `json:"order"`
		Tickets []struct {
			ID         string `json:"id"`
			Capability string `json:"capability"`
		} `json:"tickets"`
	}](t, rec)
	if settled.Order.Status != "paid" {
		t.Fatalf("expected order status paid, got %q", settled.Order.Status)
	}
	if len(settled.Tickets) != 2 {
		t.Fatalf("expected 2 tickets issued, got %d", len(settled.Tickets))
	}

	// Verify is idempotent: calling it again must not double-issue.
	rec = h.do(http.MethodPost, "/api/payments/verify", "", verifyPaymentRequest{Reference: created.Payment.Reference})
	if rec.Code != http.StatusOK {
		t.Fatalf("second verify: status %d body %s", rec.Code, rec.Body.String())
	}
	settledAgain := decodeBody[struct {
		Tickets []struct{ ID string } `json:"tickets"`
	}](t, rec)
	if len(settledAgain.Tickets) != 2 {
		t.Fatalf("second verify: expected the same 2 tickets, got %d", len(settledAgain.Tickets))
	}

	// Buyer can list and fetch their tickets.
	rec = h.do(http.MethodGet, "/api/tickets", buyerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list my tickets: status %d body %s", rec.Code, rec.Body.String())
	}
	mine := decodeBody[struct {
		Tickets []struct{ ID string } `json:"tickets"`
	}](t, rec)
	if len(mine.Tickets) != 2 {
		t.Fatalf("expected 2 tickets for buyer, got %d", len(mine.Tickets))
	}

	rec = h.do(http.MethodGet, "/api/tickets/"+settled.Tickets[0].ID, buyerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get ticket: status %d body %s", rec.Code, rec.Body.String())
	}

	// A stranger cannot fetch someone else's ticket (404, not 403).
	strangerToken, _ := h.signupUser("stranger-purchase@example.com", "stranger-password-123", "Stranger")
	rec = h.do(http.MethodGet, "/api/tickets/"+settled.Tickets[0].ID, strangerToken, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("stranger get ticket: expected 404, got %d", rec.Code)
	}

	// PDF route returns a real PDF.
	rec = h.do(http.MethodGet, "/api/tickets/"+settled.Tickets[0].ID+"/pdf", buyerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket pdf: status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("ticket pdf: expected Content-Type application/pdf, got %q", ct)
	}
	if len(rec.Body.Bytes()) < 8 || string(rec.Body.Bytes()[:5]) != "%PDF-" {
		t.Fatalf("ticket pdf: body does not look like a PDF")
	}

	// --- Offline gate flow ---

	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/scan-bundle", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scan-bundle: status %d body %s", rec.Code, rec.Body.String())
	}
	bundle := decodeBody[struct {
		Event struct {
			EventID string `json:"event_id"`
		} `json:"event"`
		IssuerKeys struct {
			Keys map[string]string `json:"keys"`
		} `json:"issuer_keys"`
	}](t, rec)
	if bundle.Event.EventID != fx.eventID {
		t.Fatalf("scan-bundle: expected event_id %q, got %q", fx.eventID, bundle.Event.EventID)
	}
	if len(bundle.IssuerKeys.Keys) == 0 {
		t.Fatal("scan-bundle: expected at least one issuer key")
	}

	scanReq := scanRequest{
		EventID: fx.eventID, Capability: settled.Tickets[0].Capability,
		DeviceID: "gate-device-1", GateID: "main-gate", ScannedAt: time.Now(),
	}
	rec = h.do(http.MethodPost, "/api/scan", fx.ownerToken, scanReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("scan: status %d body %s", rec.Code, rec.Body.String())
	}
	scanResult := decodeBody[scanResponse](t, rec)
	if scanResult.Result != "admitted" {
		t.Fatalf("expected first scan admitted, got %q (reason %q)", scanResult.Result, scanResult.Reason)
	}

	// Second scan of the same ticket is a duplicate, never overwriting the
	// first admission.
	rec = h.do(http.MethodPost, "/api/scan", fx.ownerToken, scanReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("second scan: status %d body %s", rec.Code, rec.Body.String())
	}
	dup := decodeBody[scanResponse](t, rec)
	if dup.Result != "duplicate" {
		t.Fatalf("expected second scan duplicate, got %q", dup.Result)
	}

	// A tampered capability is rejected as invalid, not admitted.
	tampered := settled.Tickets[1].Capability + "x"
	rec = h.do(http.MethodPost, "/api/scan", fx.ownerToken, scanRequest{
		EventID: fx.eventID, Capability: tampered, DeviceID: "gate-device-1", GateID: "main-gate", ScannedAt: time.Now(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("tampered scan: status %d body %s", rec.Code, rec.Body.String())
	}
	invalidResult := decodeBody[scanResponse](t, rec)
	if invalidResult.Result != "invalid" {
		t.Fatalf("expected tampered capability invalid, got %q", invalidResult.Result)
	}

	// Offline batch sync for the second ticket: first upload wins as
	// admitted, a retried identical upload is an idempotent no-op, and a
	// second device's conflicting claim is downgraded to duplicate.
	scannedAt := time.Now().UTC().Truncate(time.Second)
	syncReq := scanSyncRequest{Admissions: []scanSyncItem{
		{TicketID: settled.Tickets[1].ID, EventID: fx.eventID, GateID: "gate-2", DeviceID: "device-A", ScannedAt: scannedAt, Result: "admitted"},
	}}
	rec = h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, syncReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("scan sync: status %d body %s", rec.Code, rec.Body.String())
	}
	syncResp := decodeBody[scanSyncResponse](t, rec)
	if len(syncResp.Applied) != 1 || !syncResp.Applied[0] {
		t.Fatalf("scan sync: expected [true], got %+v", syncResp.Applied)
	}

	// Retrying the exact same sync item is a no-op (idempotent by
	// ticket_id, device_id, scanned_at).
	rec = h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, syncReq)
	syncResp2 := decodeBody[scanSyncResponse](t, rec)
	if len(syncResp2.Applied) != 1 || syncResp2.Applied[0] {
		t.Fatalf("retried scan sync: expected [false] (already applied), got %+v", syncResp2.Applied)
	}

	// A second, different device's admitted claim for the same ticket is
	// downgraded to duplicate server-side, never overwriting the winner.
	syncReq2 := scanSyncRequest{Admissions: []scanSyncItem{
		{TicketID: settled.Tickets[1].ID, EventID: fx.eventID, GateID: "gate-3", DeviceID: "device-B", ScannedAt: scannedAt.Add(time.Second), Result: "admitted"},
	}}
	rec = h.do(http.MethodPost, "/api/scan/sync", fx.ownerToken, syncReq2)
	syncResp3 := decodeBody[scanSyncResponse](t, rec)
	if len(syncResp3.Applied) != 1 || !syncResp3.Applied[0] {
		t.Fatalf("conflicting device scan sync: expected [true] (new row, downgraded), got %+v", syncResp3.Applied)
	}

	// Stats now reflect the paid order and the online-admitted ticket.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/stats", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("event stats: status %d body %s", rec.Code, rec.Body.String())
	}
	stats := decodeBody[struct {
		Stats events.Stats `json:"stats"`
	}](t, rec)
	if stats.Stats.Sold != 2 {
		t.Fatalf("expected 2 sold, got %d", stats.Stats.Sold)
	}
	if stats.Stats.Admitted < 1 {
		t.Fatalf("expected at least 1 admitted, got %d", stats.Stats.Admitted)
	}
}
