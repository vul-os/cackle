package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/store"
)

// futureSchedule returns a valid (starts_at, ends_at) pair for event
// creation in these tests — the actual dates don't matter, only that
// starts_at is in the future and ends_at is after it.
func futureSchedule() (starts, ends time.Time) {
	starts = time.Now().Add(30 * 24 * time.Hour)
	return starts, starts.Add(4 * time.Hour)
}

// TestListOrgEvents_IncludesDraftsMembersOnly is the regression test for
// the "organiser drafts are invisible" gap: GET /api/orgs/{id}/events must
// return every event belonging to the org regardless of status (so a
// draft never vanishes from the organiser's own Events list/dashboard),
// while the public GET /api/events listing must keep returning
// published-only, never leaking a draft to an anonymous caller.
func TestListOrgEvents_IncludesDraftsMembersOnly(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "org-listing")

	// A second, never-published draft in the same org.
	starts, ends := futureSchedule()
	rec := h.do(http.MethodPost, "/api/events", fx.ownerToken, struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: fx.orgID,
		CreateEventInput: events.CreateEventInput{
			Slug: "org-listing-draft", Title: "Still A Draft",
			VenueName: "TBD", Address: "TBD",
			StartsAt: starts, EndsAt: ends,
			Timezone: "UTC", Currency: "ZAR",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create draft event: status %d body %s", rec.Code, rec.Body.String())
	}
	draft := decodeBody[struct {
		Event events.Event `json:"event"`
	}](t, rec)
	if draft.Event.Status != "draft" {
		t.Fatalf("expected freshly created event to be a draft, got %q", draft.Event.Status)
	}

	// Owner (admin+) sees BOTH events via the org-scoped listing.
	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/events", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list org events: status %d body %s", rec.Code, rec.Body.String())
	}
	orgList := decodeBody[struct {
		Events []events.Event `json:"events"`
	}](t, rec)
	if len(orgList.Events) != 2 {
		t.Fatalf("expected 2 events (draft + published) in org listing, got %d: %+v", len(orgList.Events), orgList.Events)
	}
	var sawDraft, sawPublished bool
	for _, e := range orgList.Events {
		if e.ID == draft.Event.ID {
			sawDraft = true
		}
		if e.ID == fx.eventID {
			sawPublished = true
		}
	}
	if !sawDraft {
		t.Fatal("org listing did not include the draft event")
	}
	if !sawPublished {
		t.Fatal("org listing did not include the published event")
	}

	// A plain scanner-role member also sees the draft — this is a
	// read-only listing at the same bar as stats/attendees, not an
	// admin-only surface.
	scannerToken, scannerID := h.signupUser("scanner-org-listing@example.com", "scanner-password-123", "Scanner")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: scannerID, Role: "scanner"}); err != nil {
		t.Fatalf("add scanner member: %v", err)
	}
	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/events", scannerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner list org events: status %d body %s", rec.Code, rec.Body.String())
	}
	scannerList := decodeBody[struct {
		Events []events.Event `json:"events"`
	}](t, rec)
	if len(scannerList.Events) != 2 {
		t.Fatalf("scanner: expected 2 events, got %d", len(scannerList.Events))
	}

	// The PUBLIC listing must still exclude the draft entirely — no auth,
	// no org scoping, published-only.
	rec = h.do(http.MethodGet, "/api/events", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("public list events: status %d body %s", rec.Code, rec.Body.String())
	}
	pubList := decodeBody[struct {
		Events []events.Event `json:"events"`
	}](t, rec)
	for _, e := range pubList.Events {
		if e.ID == draft.Event.ID {
			t.Fatal("SECURITY: public GET /api/events leaked a draft event")
		}
		if e.Status != "published" {
			t.Fatalf("public listing returned a non-published event: %+v", e)
		}
	}
}

// TestDeleteEvent_RefusedWithIssuedTicketsAllowedOtherwise is the
// regression test for the "DELETE /api/events/{id} does not exist" gap.
// An event with at least one issued ticket must be refused (409 conflict,
// steering the caller to cancel instead) — never silently orphaning the
// ticket/order. An event with no issued tickets (nobody has bought
// anything, paid or not) can be deleted outright.
func TestDeleteEvent_RefusedWithIssuedTicketsAllowedOtherwise(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "delete-http")

	buyerToken, _ := h.signupUser("buyer-delete-http@example.com", "buyer-password-123", "Buyer")
	h.placeAndSettleOrder(t, buyerToken, fx.eventID, fx.ticketTypeID, "buyer-delete-http@example.com", "Buyer", 1)

	// Refused: this event now has an issued ticket.
	rec := h.do(http.MethodDelete, "/api/events/"+fx.eventID, fx.ownerToken, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete event with issued ticket: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)

	// The event must still exist afterward — never silently removed.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("event should still exist after refused delete: status %d", rec.Code)
	}

	// A fresh draft with zero tickets CAN be deleted.
	starts, ends := futureSchedule()
	rec = h.do(http.MethodPost, "/api/events", fx.ownerToken, struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: fx.orgID,
		CreateEventInput: events.CreateEventInput{
			Slug: "delete-http-fresh", Title: "Freshly Created",
			VenueName: "TBD", Address: "TBD",
			StartsAt: starts, EndsAt: ends,
			Timezone: "UTC", Currency: "ZAR",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create fresh event: status %d body %s", rec.Code, rec.Body.String())
	}
	fresh := decodeBody[struct {
		Event events.Event `json:"event"`
	}](t, rec)

	rec = h.do(http.MethodDelete, "/api/events/"+fresh.Event.ID, fx.ownerToken, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete fresh event: expected 204, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodGet, "/api/events/"+fresh.Event.ID, "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted event should 404, got %d", rec.Code)
	}

	// Deleting a nonexistent event: RBAC can't resolve an org for an id
	// that doesn't exist, so this is a 403 (not a 404) — exactly the same
	// shape every other event route in this package returns for a bogus
	// id (see auth.CanManageEvent's doc: a nonexistent event is RBAC
	// denial, not a lookup error), never a 500.
	rec = h.do(http.MethodDelete, "/api/events/does-not-exist", fx.ownerToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete nonexistent event: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
}
