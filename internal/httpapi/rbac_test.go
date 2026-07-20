package httpapi

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/media"
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

	// Fixtures for the invite/image delete routes below: something real to
	// (attempt to) delete, created by the owner so the outsider's 403 is a
	// genuine RBAC denial rather than an incidental 404.
	inviteRec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, map[string]any{
		"email": "invitee-rbac@example.com", "role": "scanner",
	})
	if inviteRec.Code != http.StatusCreated {
		t.Fatalf("seed invite: status %d body %s", inviteRec.Code, inviteRec.Body.String())
	}
	invite := decodeBody[struct {
		InviteID string `json:"invite_id"`
	}](t, inviteRec)

	imageID := h.seedImage(t, fx.eventID)

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
		{"create ticket type", http.MethodPost, "/api/events/" + fx.eventID + "/ticket-types", map[string]any{"name": "VIP", "price_minor": 1000, "quantity_total": 10}},
		{"update ticket type", http.MethodPatch, "/api/ticket-types/" + fx.ticketTypeID, map[string]any{"name": "hijacked"}},
		{"delete ticket type", http.MethodDelete, "/api/ticket-types/" + fx.ticketTypeID, nil},
		{"scan bundle", http.MethodGet, "/api/events/" + fx.eventID + "/scan-bundle", nil},
		{"list attendees", http.MethodGet, "/api/events/" + fx.eventID + "/attendees", nil},
		{"online scan", http.MethodPost, "/api/scan", scanRequest{
			EventID: fx.eventID, Capability: "cackle.bogus.bogus", DeviceID: "dev-1", GateID: "gate-1", ScannedAt: time.Now(),
		}},
		{"scan sync", http.MethodPost, "/api/scan/sync", scanSyncRequest{Admissions: []scanSyncItem{
			{TicketID: "t_bogus", EventID: fx.eventID, DeviceID: "dev-1", GateID: "gate-1", ScannedAt: time.Now(), Result: "admitted"},
		}}},
		// Wave 3 additions. "upload image" is technically a multipart
		// route, but RBAC runs before multipart parsing in
		// handleUploadImage, so a plain JSON body still exercises the
		// 401/403 paths correctly (it never reaches the body at all).
		{"list org members", http.MethodGet, "/api/orgs/" + fx.orgID + "/members", nil},
		{"create org invite", http.MethodPost, "/api/orgs/" + fx.orgID + "/invites", map[string]any{"email": "someone@example.com", "role": "scanner"}},
		{"list org invites", http.MethodGet, "/api/orgs/" + fx.orgID + "/invites", nil},
		{"delete org invite", http.MethodDelete, "/api/invites/" + invite.InviteID, nil},
		{"get bank account", http.MethodGet, "/api/orgs/" + fx.orgID + "/bank-account", nil},
		{"set bank account", http.MethodPut, "/api/orgs/" + fx.orgID + "/bank-account", map[string]any{
			"bank_code": "051001", "account_number": "1234567890", "account_name": "Test Org",
		}},
		{"upload image", http.MethodPost, "/api/events/" + fx.eventID + "/images", nil},
		{"delete image", http.MethodDelete, "/api/images/" + imageID, nil},
		// This is exactly the route the old app shipped unprotected
		// (/admin/events/:id/payouts) — now implemented, admin+ gated,
		// and covered here so a regression fails CI instead of shipping
		// quietly.
		{"event payouts", http.MethodGet, "/api/events/" + fx.eventID + "/payouts", nil},
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

	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/attendees", scannerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner attendees: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodPatch, "/api/events/"+fx.eventID, scannerToken, map[string]any{"title": "should be forbidden"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("scanner update event: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodPost, "/api/events/"+fx.eventID+"/ticket-types", scannerToken, map[string]any{"name": "VIP", "price_minor": 1000, "quantity_total": 10})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("scanner create ticket type: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestRBAC_AuthOnlyRoutesRequireLogin covers the two wave-3 routes that
// aren't org-scoped by URL at all — GET /api/banks (general reference
// data) and POST /api/invites/accept (the token itself, not org
// membership, carries the authority) — so they don't fit the
// unauthenticated+non-member table above. Both must still reject an
// anonymous caller outright.
func TestRBAC_AuthOnlyRoutesRequireLogin(t *testing.T) {
	h := newTestHarness(t)

	rec := h.do(http.MethodGet, "/api/banks", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/banks unauthenticated: expected 401, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)

	rec = h.do(http.MethodPost, "/api/invites/accept", "", map[string]any{"token": "bogus"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/invites/accept unauthenticated: expected 401, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)
}

// seedImage inserts a real image row (and a matching file on disk, so
// GET /media/{id} and the delete handler's own file-removal step both
// stay consistent) directly against the store, bypassing the upload HTTP
// route entirely. Used by fixtures that just need SOME real image id to
// exercise RBAC/delete against.
func (h *testHarness) seedImage(t *testing.T, eventID string) string {
	t.Helper()
	img := &store.Image{EventID: eventID, Format: string(media.FormatPNG), Width: 1, Height: 1, SizeBytes: 1}
	if err := h.store.CreateImage(h.t.Context(), img); err != nil {
		t.Fatalf("seed image: %v", err)
	}
	if err := os.MkdirAll(h.mediaDir, 0o700); err != nil {
		t.Fatalf("seed image mkdir: %v", err)
	}
	path := imagePath(h.mediaDir, img.ID, media.FormatPNG)
	if err := os.WriteFile(path, []byte{0}, 0o600); err != nil {
		t.Fatalf("seed image file: %v", err)
	}
	return img.ID
}
