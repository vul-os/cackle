package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/store"
)

// TestRBAC_OrgEventRoutesRejectUnauthenticatedAndNonMembers is the mandatory
// regression test called out in BUILD-SPEC's security bar: the old app
// shipped /admin/events/:id/payouts with NO protection at all. Every
// mutating (and org-scoped-read) org/event route in this table must:
//
//  1. reject a request with no Authorization at all (401), and
//  2. reject a request from a real, authenticated user who is simply not
//     a member of the event's org (403).
//
// New routes should be added to this table as they're added to the
// router, so a missing RBAC check fails CI instead of shipping quietly —
// exactly the class of bug this test exists to catch.
func TestRBAC_OrgEventRoutesRejectUnauthenticatedAndNonMembers(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "rbac")

	outsiderToken, _ := h.signupUser("outsider-rbac@example.com", "outsider-password-123", "Outsider")

	type route struct {
		name   string
		method string
		path   string
		body   any
	}

	routes := []route{
		{"create event", http.MethodPost, "/api/events", struct {
			OrgID string `json:"org_id"`
			Title string `json:"title"`
		}{OrgID: fx.orgID, Title: "Should not be created"}},
		{"update event", http.MethodPatch, "/api/events/" + fx.eventID, map[string]any{"title": "hijacked"}},
		{"publish event", http.MethodPost, "/api/events/" + fx.eventID + "/publish", nil},
		{"event stats", http.MethodGet, "/api/events/" + fx.eventID + "/stats", nil},
		{"list ticket types (management)", http.MethodGet, "/api/events/" + fx.eventID + "/ticket-types", nil},
		{"create ticket type", http.MethodPost, "/api/events/" + fx.eventID + "/ticket-types", map[string]any{"name": "VIP", "price_cents": 1000, "quantity_total": 10}},
		{"update ticket type", http.MethodPatch, "/api/ticket-types/" + fx.ticketTypeID, map[string]any{"name": "hijacked"}},
		{"delete ticket type", http.MethodDelete, "/api/ticket-types/" + fx.ticketTypeID, nil},
		{"scan bundle", http.MethodGet, "/api/events/" + fx.eventID + "/scan-bundle", nil},
		{"online scan", http.MethodPost, "/api/scan", scanRequest{
			EventID: fx.eventID, Capability: "cackle.bogus.bogus", DeviceID: "dev-1", GateID: "gate-1", ScannedAt: time.Now(),
		}},
		{"scan sync", http.MethodPost, "/api/scan/sync", scanSyncRequest{Admissions: []scanSyncItem{
			{TicketID: "t_bogus", EventID: fx.eventID, DeviceID: "dev-1", GateID: "gate-1", ScannedAt: time.Now(), Result: "admitted"},
		}}},
		// This is exactly the route the old app shipped unprotected
		// (/admin/events/:id/payouts) — payouts have no HTTP route in
		// this build at all (see BUILD-SPEC's API list), so there is
		// nothing to test here yet, but this table is where a payouts
		// route MUST be added the day one is implemented.
	}

	for _, rt := range routes {
		t.Run(rt.name+"/unauthenticated", func(t *testing.T) {
			rec := h.do(rt.method, rt.path, "", rt.body)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s with no auth: expected 401, got %d body %s", rt.method, rt.path, rec.Code, rec.Body.String())
			}
			assertErrorShape(t, rec)
		})
		t.Run(rt.name+"/non-member", func(t *testing.T) {
			rec := h.do(rt.method, rt.path, outsiderToken, rt.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s as non-member: expected 403, got %d body %s", rt.method, rt.path, rec.Code, rec.Body.String())
			}
			assertErrorShape(t, rec)
		})
	}
}

// TestRBAC_ScannerRoleSufficesButDoesNotElevate checks the role hierarchy
// at the boundary: a scanner-role member can read stats and pull a scan
// bundle, but cannot perform admin-level mutations like updating the
// event or creating ticket types.
func TestRBAC_ScannerRoleSufficesButDoesNotElevate(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "scanner-role")

	scannerToken, scannerID := h.signupUser("scanner-role@example.com", "scanner-password-123", "Scanner")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: scannerID, Role: "scanner"}); err != nil {
		t.Fatalf("add scanner member: %v", err)
	}

	rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/scan-bundle", scannerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner scan-bundle: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/stats", scannerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner stats: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodPatch, "/api/events/"+fx.eventID, scannerToken, map[string]any{"title": "should be forbidden"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("scanner update event: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodPost, "/api/events/"+fx.eventID+"/ticket-types", scannerToken, map[string]any{"name": "VIP", "price_cents": 1000, "quantity_total": 10})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("scanner create ticket type: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
}
