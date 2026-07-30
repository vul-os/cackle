package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/events"
)

// The invite flow exists for exactly one reason: an owner has to be able
// to put somebody ELSE on the door. Until this file existed, every invite
// test stopped at "the membership row says scanner" — which is a database
// assertion, not a product one. Nobody had ever asserted that the person
// who accepted a scanner invite can actually work a gate, or that they
// cannot do anything else.
//
// These tests drive the whole path over HTTP, with no store fixtures:
// signup -> POST /api/orgs -> create/publish an event -> invite at
// scanner -> the invitee signs up and accepts -> the invitee scans a real
// Ed25519 capability at the door. Then the negative half: every
// admin-and-above route in the product, asserted 403 for that same
// scanner.
//
// Note on what an invite is NOT allowed to be: the plaintext token is a
// bearer credential. Only its sha256 hash is persisted (internal/orgs.
// CreateInvite), so it is returned exactly once, by POST .../invites, and
// there is deliberately no route that can ever show it again.
// TestInvite_TokenIsReturnedOnceAndNeverLogged pins both halves of that.

// staffedGate is one org with one published event, an owner, and a
// scanner who got there by accepting an invite.
type staffedGate struct {
	orgID        string
	eventID      string
	ticketTypeID string
	ownerToken   string
	ownerID      string
	scannerToken string
	scannerID    string
	inviteToken  string
}

// newStaffedGate walks the entire "I need someone on the door tonight"
// path over the real routes. Every step fails the test loudly, because a
// silent skip here would leave the negative assertions below testing an
// empty set.
func (h *testHarness) newStaffedGate(t *testing.T, suffix string) staffedGate {
	t.Helper()

	ownerToken, ownerID := h.signupUser("gate-owner-"+suffix+"@example.com", "owner-password-123", "Gate Owner")
	org := h.createOrg(t, ownerToken, map[string]any{"name": "Door Venue " + suffix})

	starts := time.Now().Add(24 * time.Hour)
	rec := h.do(http.MethodPost, "/api/events", ownerToken, struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: org.Org.ID,
		CreateEventInput: events.CreateEventInput{
			Slug: "door-night-" + suffix, Title: "Door Night " + suffix,
			VenueName: "The Door", Address: "1 Door Street",
			StartsAt: starts, EndsAt: starts.Add(4 * time.Hour),
			Timezone: "Africa/Johannesburg", Currency: "ZAR",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create event: status %d body %s", rec.Code, rec.Body.String())
	}
	ev := decodeBody[struct {
		Event events.Event `json:"event"`
	}](t, rec)

	rec = h.do(http.MethodPost, "/api/events/"+ev.Event.ID+"/ticket-types", ownerToken,
		events.TicketTypeInput{Name: "Door", PriceMinor: 10000, QuantityTotal: 4, MaxPerOrder: 4, Status: "active"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create ticket type: status %d body %s", rec.Code, rec.Body.String())
	}
	tt := decodeBody[struct {
		TicketType events.TicketType `json:"ticket_type"`
	}](t, rec)

	if rec = h.do(http.MethodPost, "/api/events/"+ev.Event.ID+"/publish", ownerToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("publish event: status %d body %s", rec.Code, rec.Body.String())
	}

	// The invite itself. This response is the ONLY time the plaintext
	// token exists outside the inviter's browser.
	scannerEmail := "door-scanner-" + suffix + "@example.com"
	rec = h.do(http.MethodPost, "/api/orgs/"+org.Org.ID+"/invites", ownerToken, map[string]any{
		"email": scannerEmail, "role": "scanner",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scanner invite: status %d body %s", rec.Code, rec.Body.String())
	}
	inv := decodeBody[struct {
		InviteID  string `json:"invite_id"`
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}](t, rec)
	if inv.Token == "" {
		t.Fatal("invite response carried no token — the owner has nothing to send anyone")
	}
	if inv.ExpiresAt == "" {
		t.Fatal("invite response carried no expires_at — the UI cannot state the expiry it promises to state")
	}

	scannerToken, scannerID := h.signupUser(scannerEmail, "scanner-password-123", "Door Scanner")
	rec = h.do(http.MethodPost, "/api/invites/accept", scannerToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept scanner invite: status %d body %s", rec.Code, rec.Body.String())
	}
	accepted := decodeBody[struct {
		OrgID string `json:"org_id"`
		Role  string `json:"role"`
	}](t, rec)
	if accepted.OrgID != org.Org.ID || accepted.Role != "scanner" {
		t.Fatalf("accept granted %+v, want org %s at role scanner", accepted, org.Org.ID)
	}

	return staffedGate{
		orgID: org.Org.ID, eventID: ev.Event.ID, ticketTypeID: tt.TicketType.ID,
		ownerToken: ownerToken, ownerID: ownerID,
		scannerToken: scannerToken, scannerID: scannerID,
		inviteToken: inv.Token,
	}
}

// issueTicket buys and settles one ticket through the stub provider and
// returns its capability — a real signed ticket, not a fixture.
func (h *testHarness) issueTicket(t *testing.T, g staffedGate, buyerEmail string) string {
	t.Helper()
	buyerToken, _ := h.signupUser(buyerEmail, "buyer-password-123", "Buyer")
	rec := h.do(http.MethodPost, "/api/orders", buyerToken, createOrderRequest{
		EventID:  g.eventID,
		Items:    []orderItemRequest{{TicketTypeID: g.ticketTypeID, Quantity: 1}},
		Buyer:    buyerRequest{Email: buyerEmail, Name: "Buyer"},
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
			Capability string `json:"capability"`
		} `json:"tickets"`
	}](t, rec)
	if len(settled.Tickets) != 1 || settled.Tickets[0].Capability == "" {
		t.Fatalf("expected 1 issued ticket with a capability, got %+v", settled.Tickets)
	}
	return settled.Tickets[0].Capability
}

// TestInvite_ScannerCanWorkTheDoor is the positive half: the whole point
// of the feature. Somebody who was not in the org an hour ago is now
// admitting people at the gate, purely because an owner sent them a link.
func TestInvite_ScannerCanWorkTheDoor(t *testing.T) {
	h := newTestHarness(t)
	g := h.newStaffedGate(t, "works")

	// The scanner's own account now reports exactly one org, at scanner.
	// This is what the console reads to decide what it may render.
	rec := h.do(http.MethodGet, "/api/auth/me", g.scannerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner GET /api/auth/me: status %d body %s", rec.Code, rec.Body.String())
	}
	me := decodeBody[struct {
		Orgs []orgMembershipView `json:"orgs"`
	}](t, rec)
	if len(me.Orgs) != 1 {
		t.Fatalf("scanner sees %d orgs, want exactly 1: %+v", len(me.Orgs), me.Orgs)
	}
	if me.Orgs[0].ID != g.orgID || me.Orgs[0].Role != "scanner" {
		t.Fatalf("scanner membership = %+v, want org %s at scanner", me.Orgs[0], g.orgID)
	}

	// The gate surface, in the order the scanner page itself uses it:
	// list the org's events, then download that event's scan bundle.
	rec = h.do(http.MethodGet, "/api/orgs/"+g.orgID+"/events", g.scannerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner list org events: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	listed := decodeBody[struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"events"`
	}](t, rec)
	var found bool
	for _, e := range listed.Events {
		if e.ID == g.eventID {
			found = true
		}
	}
	if !found {
		t.Fatalf("scanner cannot see the event they are meant to staff: %+v", listed.Events)
	}

	rec = h.do(http.MethodGet, "/api/events/"+g.eventID+"/scan-bundle", g.scannerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner scan-bundle: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	bundle := decodeBody[struct {
		IssuerKeys struct {
			Keys map[string]string `json:"keys"`
		} `json:"issuer_keys"`
	}](t, rec)
	if len(bundle.IssuerKeys.Keys) == 0 {
		t.Fatal("scan bundle carried no issuer keys — the scanner cannot verify anything offline")
	}

	// And the thing that matters: admitting a real ticket.
	cap := h.issueTicket(t, g, "door-buyer-works@example.com")
	rec = h.do(http.MethodPost, "/api/scan", g.scannerToken, scanRequest{
		EventID: g.eventID, Capability: cap,
		DeviceID: "scanner-phone", GateID: "front-door", ScannedAt: time.Now(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner POST /api/scan: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	if res := decodeBody[scanResponse](t, rec); res.Result != "admitted" {
		t.Fatalf("scanner scan result = %q (reason %q), want admitted", res.Result, res.Reason)
	}

	// Offline catch-up too — a gate that scanned with no signal has to be
	// able to push its ledger back up under the same credential.
	rec = h.do(http.MethodPost, "/api/scan/sync", g.scannerToken, scanSyncRequest{Admissions: nil})
	if rec.Code != http.StatusOK {
		t.Fatalf("scanner POST /api/scan/sync: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestInvite_ScannerCannotReachAnythingAdminCan is the negative half, and
// it is the one that matters: accepting an invite must grant EXACTLY the
// invited role. Every route below is one an owner or admin uses daily.
// The owner's own token is asserted against the same routes first, so a
// route that 403s for everyone (a typo'd path, a broken fixture) cannot
// pass as "correctly refused".
func TestInvite_ScannerCannotReachAnythingAdminCan(t *testing.T) {
	h := newTestHarness(t)
	g := h.newStaffedGate(t, "denied")

	type probe struct {
		name       string
		method     string
		path       string
		body       any
		ownerWants int
	}
	probes := []probe{
		{"list team members", http.MethodGet, "/api/orgs/" + g.orgID + "/members", nil, http.StatusOK},
		{"list pending invites", http.MethodGet, "/api/orgs/" + g.orgID + "/invites", nil, http.StatusOK},
		{"invite someone else", http.MethodPost, "/api/orgs/" + g.orgID + "/invites",
			map[string]any{"email": "escalated@example.com", "role": "scanner"}, http.StatusCreated},
		{"change a member's role", http.MethodPatch, "/api/orgs/" + g.orgID + "/members/" + g.scannerID,
			map[string]any{"role": "admin"}, http.StatusOK},
		{"read the bank account", http.MethodGet, "/api/orgs/" + g.orgID + "/bank-account", nil, http.StatusNotFound},
		{"edit the event", http.MethodPatch, "/api/events/" + g.eventID,
			map[string]any{"title": "Renamed by a scanner"}, http.StatusOK},
		{"add a ticket type", http.MethodPost, "/api/events/" + g.eventID + "/ticket-types",
			map[string]any{"name": "VIP", "price_minor": 50000, "quantity_total": 2, "max_per_order": 2, "status": "active"}, http.StatusCreated},
		{"read the orders ledger", http.MethodGet, "/api/events/" + g.eventID + "/orders", nil, http.StatusOK},
		{"read payouts", http.MethodGet, "/api/events/" + g.eventID + "/payouts", nil, http.StatusOK},
		{"delete the event", http.MethodDelete, "/api/events/" + g.eventID, nil, http.StatusNoContent},
	}

	// Owner-first pass: prove each probe is a real, reachable route.
	// Delete is last in the table for a reason — it destroys the event —
	// so run the owner pass on a SEPARATE gate.
	ownerGate := h.newStaffedGate(t, "ownerpass")
	for _, p := range probes {
		path := strings.ReplaceAll(p.path, g.orgID, ownerGate.orgID)
		path = strings.ReplaceAll(path, g.eventID, ownerGate.eventID)
		path = strings.ReplaceAll(path, g.scannerID, ownerGate.scannerID)
		rec := h.do(p.method, path, ownerGate.ownerToken, p.body)
		if rec.Code != p.ownerWants {
			t.Fatalf("owner %s: got %d (want %d) body %s — this probe is not exercising a live route, so its 403 below would prove nothing",
				p.name, rec.Code, p.ownerWants, rec.Body.String())
		}
	}

	// Scanner pass: every one of them must be refused, with the API's own
	// error envelope rather than a stray 404/500.
	for _, p := range probes {
		rec := h.do(p.method, p.path, g.scannerToken, p.body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("scanner %s: expected 403, got %d body %s", p.name, rec.Code, rec.Body.String())
		}
		assertErrorShape(t, rec)
	}

	// And the membership itself is unchanged: nothing above nudged the
	// role up as a side effect.
	m, err := h.store.GetOrgMember(t.Context(), g.orgID, g.scannerID)
	if err != nil {
		t.Fatalf("get org member: %v", err)
	}
	if m.Role != "scanner" {
		t.Fatalf("scanner's role drifted to %q after the refusal pass", m.Role)
	}
}

// TestInvite_ScannerCannotCreateAnEventInSomeoneElsesOrg covers the one
// admin-gated route that is not org-scoped in its URL: POST /api/events
// takes org_id in the BODY, so it cannot be caught by a path-shaped RBAC
// table and is easy to forget.
func TestInvite_ScannerCannotCreateAnEventInSomeoneElsesOrg(t *testing.T) {
	h := newTestHarness(t)
	g := h.newStaffedGate(t, "createevent")

	starts := time.Now().Add(48 * time.Hour)
	rec := h.do(http.MethodPost, "/api/events", g.scannerToken, struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: g.orgID,
		CreateEventInput: events.CreateEventInput{
			Slug: "scanner-should-not-own-this", Title: "Scanner Should Not Own This",
			VenueName: "Nowhere", Address: "0 Nowhere",
			StartsAt: starts, EndsAt: starts.Add(time.Hour),
			Timezone: "Africa/Johannesburg", Currency: "ZAR",
		},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("scanner POST /api/events into their org: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)
}

// TestInvite_TokenIsReturnedOnceAndNeverLogged pins the two properties
// the UI's "copy this link now, it is not shown again" promise rests on.
//
// If either half of this ever stops being true, the honest thing is to
// change the UI copy, not this test.
func TestInvite_TokenIsReturnedOnceAndNeverLogged(t *testing.T) {
	h := newTestHarness(t)
	g := h.newStaffedGate(t, "once")

	// 1. No route echoes it back. The invite listing is the only place an
	//    invite is ever readable again, and it must carry no token.
	rec := h.do(http.MethodGet, "/api/orgs/"+g.orgID+"/invites", g.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list invites: status %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), g.inviteToken) {
		t.Fatal("GET .../invites echoed the plaintext invite token — it must only ever be returned by the POST that minted it")
	}

	// 2. It is not in the request log. The link the owner sends carries
	//    the token in a query string (/accept-invite?token=...), and this
	//    server's request logger records r.URL.Path deliberately, never
	//    RawQuery — a credential in a log file is a credential leaked to
	//    everyone with log access. Fetch that exact URL and read the log.
	//
	//    (The status is not asserted: this harness has no embedded
	//    frontend, so the SPA route answers 503. What matters is that the
	//    request went through the logging middleware at all, which the
	//    "did we log the path" check below establishes.)
	h.do(http.MethodGet, "/accept-invite?token="+g.inviteToken, "", nil)
	logged := h.logs.String()
	if !strings.Contains(logged, "/accept-invite") {
		t.Fatalf("the invite-link request was never logged, so this test proves nothing about logging:\n%s", logged)
	}
	if strings.Contains(logged, g.inviteToken) {
		t.Fatalf("the invite token appeared in the server log:\n%s", logged)
	}
	// The POST that redeems it carries the token in a JSON body, which is
	// likewise never logged.
	if strings.Contains(logged, "token=") {
		t.Fatalf("a query string was logged verbatim — invite links carry a credential there:\n%s", logged)
	}

	// 3. Accepting really does consume it, so a link that leaks later is
	//    worth nothing.
	other, _ := h.signupUser("door-scanner-once@example.com.other@example.com", "other-password-123", "Other")
	rec = h.do(http.MethodPost, "/api/invites/accept", other, map[string]any{"token": g.inviteToken})
	if rec.Code == http.StatusOK {
		t.Fatal("a spent invite token was accepted a second time")
	}
}
