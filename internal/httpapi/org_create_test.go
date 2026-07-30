package httpapi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/store"
)

// createdOrg is POST /api/orgs' response body.
type createdOrgResponse struct {
	Org struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Slug            string `json:"slug"`
		DefaultCurrency string `json:"default_currency"`
		Role            string `json:"role"`
	} `json:"org"`
}

// createOrg posts to the real route and fails the test unless it 201s.
func (h *testHarness) createOrg(t *testing.T, token string, body map[string]any) createdOrgResponse {
	t.Helper()
	rec := h.do(http.MethodPost, "/api/orgs", token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/orgs: expected 201, got %d body %s", rec.Code, rec.Body.String())
	}
	return decodeBody[createdOrgResponse](t, rec)
}

// TestCreateOrg_MakesCallerOwnerAndUnblocksTheConsole is the positive
// case, asserted all the way through to the thing that was actually
// broken: before POST /api/orgs existed, signup produced a user row and
// nothing else (internal/auth.Signup), so a brand new account had no org
// and every surface behind one — events, ticket types, stats, the gate —
// answered 403 forever with no path forward.
//
// So this does not stop at "201 Created". It signs up a genuinely fresh
// user, creates an org over HTTP, and then proves the owner role is real
// by exercising owner-and-admin-gated routes with it.
func TestCreateOrg_MakesCallerOwnerAndUnblocksTheConsole(t *testing.T) {
	h := newTestHarness(t)
	token, userID := h.signupUser("fresh-signup@example.com", "fresh-password-123", "Fresh Signup")

	// Precondition: a fresh signup really does start with zero orgs. If
	// this ever stops being true the rest of this test is testing nothing.
	meRec := h.do(http.MethodGet, "/api/auth/me", token, nil)
	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/me: status %d body %s", meRec.Code, meRec.Body.String())
	}
	me := decodeBody[struct {
		Orgs []orgMembershipView `json:"orgs"`
	}](t, meRec)
	if len(me.Orgs) != 0 {
		t.Fatalf("a fresh signup already had %d org(s): %+v — this test's premise is gone", len(me.Orgs), me.Orgs)
	}

	created := h.createOrg(t, token, map[string]any{"name": "Neon Nights", "default_currency": "ZAR"})
	if created.Org.ID == "" {
		t.Fatalf("created org has no id: %+v", created.Org)
	}
	if created.Org.Role != "owner" {
		t.Fatalf("created org role = %q, want %q", created.Org.Role, "owner")
	}
	if created.Org.Slug != "neon-nights" {
		t.Fatalf("slug = %q, want %q (derived from the name)", created.Org.Slug, "neon-nights")
	}
	if created.Org.DefaultCurrency != "ZAR" {
		t.Fatalf("default_currency = %q, want ZAR", created.Org.DefaultCurrency)
	}

	// The membership is durable and visible where the app reads it from.
	meRec = h.do(http.MethodGet, "/api/auth/me", token, nil)
	me = decodeBody[struct {
		Orgs []orgMembershipView `json:"orgs"`
	}](t, meRec)
	if len(me.Orgs) != 1 || me.Orgs[0].ID != created.Org.ID || me.Orgs[0].Role != "owner" {
		t.Fatalf("GET /api/auth/me orgs = %+v, want exactly one owner membership of %s", me.Orgs, created.Org.ID)
	}

	// Owner-only route (bank account) — the highest bar in the org.
	rec := h.do(http.MethodGet, "/api/orgs/"+created.Org.ID+"/bank-account", token, nil)
	if rec.Code != http.StatusNotFound {
		// 404 = "no account on file yet", i.e. authorised and answered.
		// 403 would mean the owner role never landed.
		t.Fatalf("owner GET bank-account: expected 404 (authorised, nothing on file), got %d body %s", rec.Code, rec.Body.String())
	}

	// Members list shows exactly one member — the creator, as owner. An
	// org with no owner would be unmanageable forever, so this is the
	// invariant CreateOrgWithOwner's transaction exists to hold.
	rec = h.do(http.MethodGet, "/api/orgs/"+created.Org.ID+"/members", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner GET members: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	members := decodeBody[struct {
		Members []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"members"`
	}](t, rec)
	if len(members.Members) != 1 || members.Members[0].UserID != userID || members.Members[0].Role != "owner" {
		t.Fatalf("members = %+v, want exactly [{%s owner}]", members.Members, userID)
	}

	// And the thing the whole feature is for: the console is now usable.
	starts := time.Now().Add(21 * 24 * time.Hour)
	rec = h.do(http.MethodPost, "/api/events", token, struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: created.Org.ID,
		CreateEventInput: events.CreateEventInput{
			Slug: "neon-nights-launch", Title: "Neon Nights Launch",
			VenueName: "The Old Biscuit Mill", StartsAt: starts, EndsAt: starts.Add(5 * time.Hour),
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create event in the new org: expected 201, got %d body %s", rec.Code, rec.Body.String())
	}
	ev := decodeBody[struct {
		Event events.Event `json:"event"`
	}](t, rec)
	// Currency was never stated on the event — it must inherit the org
	// default this route just set, proving default_currency was persisted
	// and not merely echoed back in the create response.
	if ev.Event.Currency != "ZAR" {
		t.Fatalf("event currency = %q, want ZAR inherited from the new org's default", ev.Event.Currency)
	}
}

// TestCreateOrg_OwnerOfNewOrgIsStillNonMemberElsewhere is THE negative
// test for this route, and the reason POST /api/orgs is safe to leave
// ungated by CanManageOrg.
//
// Any authenticated user may create an org. That must not become a way to
// acquire authority over an org that already exists. So: two users each
// create their own org, and then each is checked against the OTHER's org
// across the full role range — owner-only (bank account), admin+ (invite,
// create event), and scanner+ (list org events). Every one must be 403.
//
// The failure this guards against is not hypothetical hand-waving: it is
// what happens if a future change resolves the caller's role from "any
// membership they hold" rather than "their membership in the org named in
// THIS request". Disable CanManageOrg's per-org lookup and this test goes
// red; see the mutation note in the commit that added it.
func TestCreateOrg_OwnerOfNewOrgIsStillNonMemberElsewhere(t *testing.T) {
	h := newTestHarness(t)

	aliceToken, _ := h.signupUser("alice-orgcreate@example.com", "alice-password-123", "Alice")
	bobToken, _ := h.signupUser("bob-orgcreate@example.com", "bob-password-123", "Bob")

	alice := h.createOrg(t, aliceToken, map[string]any{"name": "Alice Presents"})
	bob := h.createOrg(t, bobToken, map[string]any{"name": "Bob Productions"})

	if alice.Org.ID == bob.Org.ID {
		t.Fatal("both users got the same org id")
	}

	// Bob owns an org. That gives him nothing in Alice's.
	crossChecks := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"owner-only: read bank account", http.MethodGet, "/api/orgs/" + alice.Org.ID + "/bank-account", nil},
		{"owner-only: set bank account", http.MethodPut, "/api/orgs/" + alice.Org.ID + "/bank-account", map[string]any{
			"bank_code": "051001", "account_number": "62812345678", "account_name": "Not Bob's",
		}},
		{"admin+: list members", http.MethodGet, "/api/orgs/" + alice.Org.ID + "/members", nil},
		{"admin+: invite a member", http.MethodPost, "/api/orgs/" + alice.Org.ID + "/invites", map[string]any{
			"email": "mallory@example.com", "role": "owner",
		}},
		{"admin+: list invites", http.MethodGet, "/api/orgs/" + alice.Org.ID + "/invites", nil},
		{"admin+: create an event", http.MethodPost, "/api/events", struct {
			OrgID string `json:"org_id"`
			Title string `json:"title"`
			Slug  string `json:"slug"`
		}{OrgID: alice.Org.ID, Title: "Squatting on Alice's org", Slug: "squat"}},
		{"scanner+: list org events", http.MethodGet, "/api/orgs/" + alice.Org.ID + "/events", nil},
	}

	for _, c := range crossChecks {
		rec := h.do(c.method, c.path, bobToken, c.body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("bob -> alice's org, %s (%s %s): expected 403, got %d body %s",
				c.name, c.method, c.path, rec.Code, rec.Body.String())
		}
		assertErrorShape(t, rec)
	}

	// Symmetric: Alice has no reach into Bob's org either.
	rec := h.do(http.MethodGet, "/api/orgs/"+bob.Org.ID+"/members", aliceToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("alice -> bob's org members: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}

	// And nothing leaked in the other direction: Alice's own org still
	// lists exactly one member (her), so Bob's attempts wrote nothing.
	rec = h.do(http.MethodGet, "/api/orgs/"+alice.Org.ID+"/members", aliceToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("alice's own members: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	members := decodeBody[struct {
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
	}](t, rec)
	if len(members.Members) != 1 {
		t.Fatalf("alice's org has %d members after bob's attempts, want 1: %+v", len(members.Members), members.Members)
	}

	// Each user sees only their own org in the session payload.
	for _, tc := range []struct {
		token string
		want  string
	}{{aliceToken, alice.Org.ID}, {bobToken, bob.Org.ID}} {
		meRec := h.do(http.MethodGet, "/api/auth/me", tc.token, nil)
		me := decodeBody[struct {
			Orgs []orgMembershipView `json:"orgs"`
		}](t, meRec)
		if len(me.Orgs) != 1 || me.Orgs[0].ID != tc.want {
			t.Fatalf("GET /api/auth/me orgs = %+v, want exactly [%s]", me.Orgs, tc.want)
		}
	}
}

// TestCreateOrg_NonMemberOfAPreExistingOrgStaysOut covers the same
// boundary against an org the caller did NOT create and that was seeded
// entirely outside this route — an org with real events, ticket types and
// an owner already on it (h.newPublishedEvent). Creating an org of one's
// own must not open a door into it.
func TestCreateOrg_NonMemberOfAPreExistingOrgStaysOut(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "orgcreate-pre")

	outsiderToken, _ := h.signupUser("outsider-orgcreate@example.com", "outsider-password-123", "Outsider")
	own := h.createOrg(t, outsiderToken, map[string]any{"name": "Outsider Events"})

	// They are a fully-fledged owner — of their own org.
	rec := h.do(http.MethodGet, "/api/orgs/"+own.Org.ID+"/members", outsiderToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("outsider on own org: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}

	// And a stranger to the seeded one, including its event-scoped routes
	// (auth.CanManageEvent resolves the org from the event, a different
	// code path from the org-id-in-URL routes above).
	for _, c := range []struct {
		name   string
		method string
		path   string
	}{
		{"org members", http.MethodGet, "/api/orgs/" + fx.orgID + "/members"},
		{"org events", http.MethodGet, "/api/orgs/" + fx.orgID + "/events"},
		{"event stats", http.MethodGet, "/api/events/" + fx.eventID + "/stats"},
		{"event attendees", http.MethodGet, "/api/events/" + fx.eventID + "/attendees"},
		{"event payouts", http.MethodGet, "/api/events/" + fx.eventID + "/payouts"},
		{"scan bundle", http.MethodGet, "/api/events/" + fx.eventID + "/scan-bundle"},
	} {
		rec := h.do(c.method, c.path, outsiderToken, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("outsider -> seeded org, %s: expected 403, got %d body %s", c.name, rec.Code, rec.Body.String())
		}
	}
}

// TestCreateOrg_RequiresAuthentication — anonymous callers cannot mint
// orgs. requireUser is the ENTIRE authorisation gate on this route, so
// this assertion is load-bearing rather than routine.
func TestCreateOrg_RequiresAuthentication(t *testing.T) {
	h := newTestHarness(t)

	rec := h.do(http.MethodPost, "/api/orgs", "", map[string]any{"name": "Anonymous Inc"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous POST /api/orgs: expected 401, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)

	// An invalid/expired bearer token is treated as anonymous (see
	// authenticate's doc), not as a 500 or a silent success.
	rec = h.do(http.MethodPost, "/api/orgs", "not-a-real-session-token", map[string]any{"name": "Forged Inc"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bogus-token POST /api/orgs: expected 401, got %d body %s", rec.Code, rec.Body.String())
	}

	// Nothing was written by either attempt.
	for _, slug := range []string{"anonymous-inc", "forged-inc"} {
		if _, err := h.store.GetOrgBySlug(t.Context(), slug); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("org %q exists after an unauthenticated attempt (err=%v)", slug, err)
		}
	}
}

// TestCreateOrg_SlugIsUniqueAcrossOrgs — an org's slug is its public
// handle, unique exactly like an event's (both carry a UNIQUE constraint
// in migrations/0001_init.sql). A second org asking for a taken slug is
// refused with 409, not a 500 and not a silent overwrite, and the refusal
// applies across users (it is a global namespace, not per-account).
func TestCreateOrg_SlugIsUniqueAcrossOrgs(t *testing.T) {
	h := newTestHarness(t)
	firstToken, _ := h.signupUser("first-slug@example.com", "first-password-123", "First")
	secondToken, _ := h.signupUser("second-slug@example.com", "second-password-123", "Second")

	first := h.createOrg(t, firstToken, map[string]any{"name": "Harbour Live", "slug": "harbour-live"})

	// Same slug, different user.
	rec := h.do(http.MethodPost, "/api/orgs", secondToken, map[string]any{"name": "Harbour Live Copycat", "slug": "harbour-live"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate slug: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)
	if code := decodeBody[errorEnvelope](t, rec).Error.Code; code != codeConflict {
		t.Fatalf("duplicate slug error code = %q, want %q", code, codeConflict)
	}

	// Same slug, same user — a repeated form submit must not quietly
	// duplicate either.
	rec = h.do(http.MethodPost, "/api/orgs", firstToken, map[string]any{"name": "Harbour Live", "slug": "harbour-live"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("same-user duplicate slug: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}

	// A collision after normalisation is still a collision: "Harbour
	// Live!" and "  HARBOUR-LIVE  " both reduce to the taken slug.
	for _, raw := range []string{"Harbour Live!", "  HARBOUR-LIVE  ", "harbour--live"} {
		rec = h.do(http.MethodPost, "/api/orgs", secondToken, map[string]any{"name": "Copycat", "slug": raw})
		if rec.Code != http.StatusConflict {
			t.Fatalf("slug %q normalises to a taken slug: expected 409, got %d body %s", raw, rec.Code, rec.Body.String())
		}
	}

	// The rejected attempts wrote nothing: the second user still has no org.
	meRec := h.do(http.MethodGet, "/api/auth/me", secondToken, nil)
	me := decodeBody[struct {
		Orgs []orgMembershipView `json:"orgs"`
	}](t, meRec)
	if len(me.Orgs) != 0 {
		t.Fatalf("second user has %d org(s) after only failed attempts: %+v", len(me.Orgs), me.Orgs)
	}

	// And the original is untouched and still owned by the first user.
	got, err := h.store.GetOrgBySlug(t.Context(), "harbour-live")
	if err != nil {
		t.Fatalf("get org by slug: %v", err)
	}
	if got.ID != first.Org.ID || got.Name != "Harbour Live" {
		t.Fatalf("original org changed: %+v", got)
	}
}

// TestCreateOrg_ValidatesInput — every rejection is a 400 in the standard
// error envelope, never a 500 leaking a constraint error.
func TestCreateOrg_ValidatesInput(t *testing.T) {
	h := newTestHarness(t)
	token, _ := h.signupUser("validation-org@example.com", "validation-password-123", "Validator")

	cases := []struct {
		name string
		body any
	}{
		{"missing name", map[string]any{}},
		{"blank name", map[string]any{"name": "   "}},
		{"name too long", map[string]any{"name": repeatA(200)}},
		{"name with no sluggable characters", map[string]any{"name": "!!! ???"}},
		{"slug too short after normalisation", map[string]any{"name": "Fine Name", "slug": "a"}},
		{"unknown currency", map[string]any{"name": "Fine Name", "default_currency": "XYZ"}},
		{"malformed currency", map[string]any{"name": "Fine Name", "default_currency": "not-a-currency"}},
	}

	for _, c := range cases {
		rec := h.do(http.MethodPost, "/api/orgs", token, c.body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d body %s", c.name, rec.Code, rec.Body.String())
		}
		assertErrorShape(t, rec)
	}

	// A non-JSON body is a 400 too, not a panic-recovered 500.
	rec := h.do(http.MethodPost, "/api/orgs", token, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}

	// None of the above created anything.
	meRec := h.do(http.MethodGet, "/api/auth/me", token, nil)
	me := decodeBody[struct {
		Orgs []orgMembershipView `json:"orgs"`
	}](t, meRec)
	if len(me.Orgs) != 0 {
		t.Fatalf("invalid requests created %d org(s): %+v", len(me.Orgs), me.Orgs)
	}
}

// repeatA returns a string of n 'a's — a helper for the over-length
// name case, kept explicit so the test reads as "too long" rather than
// relying on a magic literal.
func repeatA(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

// TestCreateOrgWithOwner_IsAtomic proves the org row and its owner
// membership land together or not at all, at the store layer where the
// transaction actually lives. An org with no owner is permanently
// unmanageable (nobody can invite, promote, or change payouts — see
// orgs.ErrLastOwner), so a half-written create is worse than a failed one.
//
// The forced failure is a membership insert that cannot succeed: a
// user_id with no users row, which org_members' FOREIGN KEY rejects
// (foreign_keys pragma is on — see store.Open's DSN). If the insert order
// were ever reversed, or the two writes split out of one transaction, the
// orgs row would survive and this goes red.
func TestCreateOrgWithOwner_IsAtomic(t *testing.T) {
	h := newTestHarness(t)

	org := &store.Org{Name: "Orphan Org", Slug: "orphan-org"}
	err := h.store.CreateOrgWithOwner(t.Context(), org, "no-such-user-id")
	if err == nil {
		t.Fatal("CreateOrgWithOwner with a nonexistent owner: expected an error, got nil")
	}

	if _, err := h.store.GetOrgBySlug(t.Context(), "orphan-org"); err == nil {
		t.Fatal("the org row survived a failed membership insert — the two writes are not in one transaction")
	}

	// The happy path against the same store still works afterwards (the
	// rolled-back transaction left nothing behind, including the slug).
	_, ownerID := h.signupUser("atomic-owner@example.com", "atomic-password-123", "Atomic Owner")
	good := &store.Org{Name: "Orphan Org", Slug: "orphan-org"}
	if err := h.store.CreateOrgWithOwner(t.Context(), good, ownerID); err != nil {
		t.Fatalf("CreateOrgWithOwner after a rollback: %v", err)
	}
	members, err := h.store.ListOrgMembers(t.Context(), good.ID)
	if err != nil {
		t.Fatalf("list org members: %v", err)
	}
	if len(members) != 1 || members[0].UserID != ownerID || members[0].Role != "owner" {
		t.Fatalf("members = %+v, want exactly one owner (%s)", members, ownerID)
	}
}
