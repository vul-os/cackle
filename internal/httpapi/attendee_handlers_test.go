package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// attendeeListResponse mirrors GET /api/events/{id}/attendees's wire shape.
type attendeeListResponse struct {
	Attendees []struct {
		TicketID       string `json:"ticket_id"`
		OrderID        string `json:"order_id"`
		Serial         string `json:"serial"`
		HolderName     string `json:"holder_name"`
		Status         string `json:"status"`
		TicketTypeID   string `json:"ticket_type_id"`
		TicketTypeName string `json:"ticket_type_name"`
		IssuedAt       string `json:"issued_at"`
		AdmittedAt     string `json:"admitted_at,omitempty"`
		Admitted       bool   `json:"admitted"`
	} `json:"attendees"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// placeAndSettleOrder is a small helper: places an order over HTTP for
// buyerToken, verifies payment (settling it), and returns the issued
// tickets' (id, capability) pairs.
func (h *testHarness) placeAndSettleOrder(t *testing.T, buyerToken, eventID, ticketTypeID, buyerEmail, buyerName string, qty int) []struct{ ID, Capability string } {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/orders", buyerToken, createOrderRequest{
		EventID:  eventID,
		Items:    []orderItemRequest{{TicketTypeID: ticketTypeID, Quantity: qty}},
		Buyer:    buyerRequest{Email: buyerEmail, Name: buyerName},
		Provider: "stub",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order: status %d body %s", rec.Code, rec.Body.String())
	}
	created := decodeBody[struct {
		Payment struct {
			Reference string `json:"reference"`
		} `json:"payment"`
	}](t, rec)

	rec = h.do(http.MethodPost, "/api/payments/verify", "", verifyPaymentRequest{Reference: created.Payment.Reference})
	if rec.Code != http.StatusOK {
		t.Fatalf("verify payment: status %d body %s", rec.Code, rec.Body.String())
	}
	settled := decodeBody[struct {
		Tickets []struct {
			ID         string `json:"id"`
			Capability string `json:"capability"`
		} `json:"tickets"`
	}](t, rec)
	if len(settled.Tickets) != qty {
		t.Fatalf("expected %d tickets, got %d", qty, len(settled.Tickets))
	}

	out := make([]struct{ ID, Capability string }, len(settled.Tickets))
	for i, tk := range settled.Tickets {
		out[i] = struct{ ID, Capability string }{tk.ID, tk.Capability}
	}
	return out
}

// TestListEventAttendees covers the roster endpoint end to end: correct
// counts, search, status filtering (both ticket status and admission
// status), pagination bounds, and — the security-relevant assertion — that
// the buyer's email never appears anywhere in the response.
func TestListEventAttendees(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "attendees")

	thandiToken, _ := h.signupUser("thandi-attendees@example.com", "thandi-password-123", "Thandi Nkosi")
	pieterToken, _ := h.signupUser("pieter-attendees@example.com", "pieter-password-123", "Pieter van der Merwe")

	thandiTickets := h.placeAndSettleOrder(t, thandiToken, fx.eventID, fx.ticketTypeID, "thandi-attendees@example.com", "Thandi Nkosi", 1)
	h.placeAndSettleOrder(t, pieterToken, fx.eventID, fx.ticketTypeID, "pieter-attendees@example.com", "Pieter van der Merwe", 2)

	// Admit Thandi's one ticket at the gate.
	rec := h.do(http.MethodPost, "/api/scan", fx.ownerToken, scanRequest{
		EventID: fx.eventID, Capability: thandiTickets[0].Capability,
		DeviceID: "gate-device-1", GateID: "main-gate", ScannedAt: time.Now(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("scan: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := decodeBody[scanResponse](t, rec).Result; got != "admitted" {
		t.Fatalf("expected admitted, got %q", got)
	}

	// --- Unfiltered list: 3 tickets total across the two orders. ---
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list attendees: status %d body %s", rec.Code, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "buyer_email") || strings.Contains(bodyStr, "thandi-attendees@example.com") || strings.Contains(bodyStr, "pieter-attendees@example.com") {
		t.Fatalf("attendee roster leaked buyer email into the response: %s", bodyStr)
	}

	all := decodeBody[attendeeListResponse](t, rec)
	if all.Total != 3 {
		t.Fatalf("expected total 3, got %d", all.Total)
	}
	if len(all.Attendees) != 3 {
		t.Fatalf("expected 3 attendee rows, got %d", len(all.Attendees))
	}
	var admittedCount int
	for _, a := range all.Attendees {
		if a.HolderName == "" {
			t.Fatalf("attendee row missing holder_name: %+v", a)
		}
		if a.Admitted {
			admittedCount++
			if a.AdmittedAt == "" {
				t.Fatalf("admitted row missing admitted_at: %+v", a)
			}
		}
	}
	if admittedCount != 1 {
		t.Fatalf("expected exactly 1 admitted attendee, got %d", admittedCount)
	}

	// --- Search by holder name. ---
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?q=Thandi", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("search attendees: status %d body %s", rec.Code, rec.Body.String())
	}
	searched := decodeBody[attendeeListResponse](t, rec)
	if searched.Total != 1 || len(searched.Attendees) != 1 {
		t.Fatalf("expected 1 match for %q, got total=%d rows=%d", "Thandi", searched.Total, len(searched.Attendees))
	}
	if searched.Attendees[0].HolderName != "Thandi Nkosi" {
		t.Fatalf("expected Thandi Nkosi, got %q", searched.Attendees[0].HolderName)
	}

	// --- Filter by admission status. ---
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?status=admitted", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=admitted: status %d body %s", rec.Code, rec.Body.String())
	}
	admitted := decodeBody[attendeeListResponse](t, rec)
	if admitted.Total != 1 {
		t.Fatalf("expected 1 admitted attendee, got %d", admitted.Total)
	}

	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?status=not_admitted", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=not_admitted: status %d body %s", rec.Code, rec.Body.String())
	}
	notAdmitted := decodeBody[attendeeListResponse](t, rec)
	if notAdmitted.Total != 2 {
		t.Fatalf("expected 2 not-admitted attendees, got %d", notAdmitted.Total)
	}

	// --- Filter by ticket status. ---
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?status=valid", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=valid: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := decodeBody[attendeeListResponse](t, rec).Total; got != 3 {
		t.Fatalf("expected 3 valid attendees, got %d", got)
	}

	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?status=void", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=void: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := decodeBody[attendeeListResponse](t, rec).Total; got != 0 {
		t.Fatalf("expected 0 void attendees, got %d", got)
	}

	// An unrecognised status value fails closed to an empty page, not the
	// full unfiltered roster.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?status=bogus", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=bogus: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := decodeBody[attendeeListResponse](t, rec).Total; got != 0 {
		t.Fatalf("expected unrecognised status to yield 0 rows, got %d", got)
	}

	// --- Pagination. ---
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?limit=1&offset=0", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("page 1: status %d body %s", rec.Code, rec.Body.String())
	}
	page1 := decodeBody[attendeeListResponse](t, rec)
	if len(page1.Attendees) != 1 || page1.Total != 3 || page1.Limit != 1 || page1.Offset != 0 {
		t.Fatalf("page1 unexpected shape: %+v", page1)
	}

	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?limit=1&offset=1", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("page 2: status %d body %s", rec.Code, rec.Body.String())
	}
	page2 := decodeBody[attendeeListResponse](t, rec)
	if len(page2.Attendees) != 1 || page2.Offset != 1 {
		t.Fatalf("page2 unexpected shape: %+v", page2)
	}
	if page1.Attendees[0].TicketID == page2.Attendees[0].TicketID {
		t.Fatalf("page1 and page2 returned the same ticket %q — pagination is broken", page1.Attendees[0].TicketID)
	}

	// A requested limit above the hard maximum is clamped, not honoured.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?limit=100000", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("huge limit: status %d body %s", rec.Code, rec.Body.String())
	}
	if got := decodeBody[attendeeListResponse](t, rec).Limit; got != 200 {
		t.Fatalf("expected limit clamped to 200, got %d", got)
	}

	// A negative limit/offset is a 400, not silently coerced.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?limit=-1", fx.ownerToken, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative limit: expected 400, got %d", rec.Code)
	}
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees?offset=-1", fx.ownerToken, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative offset: expected 400, got %d", rec.Code)
	}
}
