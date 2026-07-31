package httpapi

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/vul-os/cackle/internal/config"
)

// Organisation-page tests: GET /o/{ref}.
//
// This route is the only public page on the box that is addressed by an
// ORGANISATION rather than by an event, so it is the only one that could be
// walked to find out which organisations live here. Most of what follows is
// about that: three separate guards, disabled one at a time, each of which must
// redden a test on its own.
//
// The trap these are written to avoid: a guard that looks tested because a
// DIFFERENT guard would have hidden the row anyway. So the fixtures are built
// so that each test has exactly one thing standing between it and a leak —
// TestOrgPage_HidesDraftAndCancelledEventsOfAnOrgThatIsOtherwiseOnDisplay puts
// a draft in an org that is INSIDE the display scope, so the scope filter
// cannot be what hides it, and the published-only rule has to hold alone.

// publishEvent moves a draft to published.
func (h *testHarness) publishEvent(t *testing.T, eventID, token string) {
	t.Helper()
	if rec := h.do(http.MethodPost, "/api/events/"+eventID+"/publish", token, nil); rec.Code != http.StatusOK {
		t.Fatalf("publish event: status %d body %s", rec.Code, rec.Body.String())
	}
}

// cancelEvent moves a published event to cancelled.
func (h *testHarness) cancelEvent(t *testing.T, eventID, token string) {
	t.Helper()
	rec := h.do(http.MethodPatch, "/api/events/"+eventID, token, map[string]any{"status": "cancelled"})
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel event: status %d body %s", rec.Code, rec.Body.String())
	}
}

func (h *testHarness) orgPage(path string) *httptest.ResponseRecorder {
	return h.do(http.MethodGet, path, "", nil)
}

// The happy path, and the first two guards together: the page names the
// organisation and lists its published events.
func TestOrgPage_ShowsTheOrganisationAndItsPublishedEvents(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "orgpage")

	rec := h.orgPage("/o/test-org-orgpage")
	if rec.Code != http.StatusOK {
		t.Fatalf("org page: status %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type = %q, want text/html", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Test Org orgpage") {
		t.Error("the organisation page does not name the organisation")
	}
	if !strings.Contains(body, "Test Event orgpage") {
		t.Error("the organisation page does not list the organisation's published event")
	}
	if !strings.Contains(body, `href="/h/test-event-orgpage"`) {
		t.Error("the listed event does not link to its own host page")
	}
	// It is a page, not an API resource, and it is reachable without a
	// session — the whole point is somewhere to send a stranger.
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("the organisation page is served without a Content-Security-Policy")
	}
	_ = fx
}

// Reachable by the organisation's id as well as its slug: an event's organiser
// attribution has the org id to hand (events carry org_id) and should not have
// to resolve a slug first.
func TestOrgPage_ResolvesBySlugAndByID(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "byref")

	for _, ref := range []string{"test-org-byref", fx.orgID} {
		rec := h.orgPage("/o/" + ref)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /o/%s: status %d body %s", ref, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Test Org byref") {
			t.Errorf("GET /o/%s did not resolve to the organisation", ref)
		}
	}
}

// GUARD 3, ON ITS OWN. The organisation here is firmly inside the display
// scope — it has a published event, so nothing about the scope filter can be
// what hides its OTHER events. A draft and a cancelled event in that same org
// must still be absent, which leaves the published-only rule as the only thing
// that can be holding them back.
func TestOrgPage_HidesDraftAndCancelledEventsOfAnOrgThatIsOtherwiseOnDisplay(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "mixed")

	// A draft in the same org.
	h.newDraftEvent(t, fx.orgID, fx.ownerToken, "unannounced-fundraiser")
	// A published-then-cancelled event in the same org.
	cancelled := h.newDraftEvent(t, fx.orgID, fx.ownerToken, "washed-out-picnic")
	h.publishEvent(t, cancelled, fx.ownerToken)
	h.cancelEvent(t, cancelled, fx.ownerToken)

	rec := h.orgPage("/o/test-org-mixed")
	if rec.Code != http.StatusOK {
		t.Fatalf("org page: status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// The control: the org's published event IS there, so a page that hid
	// everything cannot pass this test by hiding everything.
	if !strings.Contains(body, "test-event-mixed") {
		t.Fatal("the org's published event is missing; this test cannot prove anything about what is hidden")
	}
	if strings.Contains(body, "unannounced-fundraiser") || strings.Contains(body, "Draft unannounced-fundraiser") {
		t.Error("a DRAFT event is on the public organisation page")
	}
	if strings.Contains(body, "washed-out-picnic") || strings.Contains(body, "Draft washed-out-picnic") {
		t.Error("a CANCELLED event is listed as something the organisation has on")
	}
}

// GUARD 2, ON ITS OWN. Two organisations, both in scope, both with published
// events. One organisation's page must not carry the other's programme.
func TestOrgPage_ShowsOnlyThatOrganisationsEvents(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "alpha")
	h.newPublishedEvent(t, "beta")

	rec := h.orgPage("/o/test-org-alpha")
	if rec.Code != http.StatusOK {
		t.Fatalf("org page: status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "test-event-alpha") {
		t.Fatal("the organisation's own event is missing; this test cannot prove anything about what is excluded")
	}
	if strings.Contains(body, "test-event-beta") || strings.Contains(body, "Test Org beta") {
		t.Error("one organisation's page carries another organisation's events")
	}
}

// An organisation that does not exist. The baseline the two leak tests below
// compare against.
func TestOrgPage_NonexistentOrgIs404(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "present")

	rec := h.orgPage("/o/no-such-organisation")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type = %q; a browser following a link is owed a page, not a JSON envelope", ct)
	}
	if strings.Contains(rec.Body.String(), "organisation") {
		t.Error("the 404 body names what this namespace holds; that is a hint to someone walking it")
	}
}

// GUARD 1, ON ITS OWN — and the property this whole route is riskiest for.
//
// In "single" scope the box presents as ONE organisation. Another organisation
// on the same box has published events, is perfectly real, and is out of
// display scope. Its page must answer exactly as a nonexistent one does: same
// status, same body, same headers. Any observable difference turns /o/ into a
// probe for which organisations live on a box.
func TestOrgPage_OutOfScopeOrgIsIndistinguishableFromNonexistent(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "bijou")
	h.newPublishedEvent(t, "lodger")

	h.cfg.HostScope = config.HostScopeSingle
	h.cfg.HostOrg = "test-org-bijou"

	// The configured organisation is on display.
	if rec := h.orgPage("/o/test-org-bijou"); rec.Code != http.StatusOK {
		t.Fatalf("the configured organisation's own page: status %d; this test cannot prove anything if nothing is visible", rec.Code)
	}

	outOfScope := h.orgPage("/o/test-org-lodger")
	nonexistent := h.orgPage("/o/no-such-organisation-at-all")

	if outOfScope.Code != http.StatusNotFound {
		t.Errorf("an out-of-scope organisation answered %d, want 404 — /o/ enumerates this box", outOfScope.Code)
	}
	if got := outOfScope.Body.String(); got != nonexistent.Body.String() {
		t.Errorf("an out-of-scope organisation answers differently from a nonexistent one:\n got: %s\nwant: %s", got, nonexistent.Body.String())
	}
	// Also by ID: knowing an org's ULID must not be a way past the scope.
	if rec := h.orgPage("/o/" + h.orgIDBySlug(t, "test-org-lodger")); rec.Code != http.StatusNotFound {
		t.Errorf("an out-of-scope organisation answered %d when addressed by id, want 404", rec.Code)
	}
	// Headers, not just the body: a differing Cache-Control or a stray
	// X-Cackle-* marker would leak the same fact.
	assertSameHeaders(t, outOfScope, nonexistent)
}

// An organisation with only drafts has published nothing. It is not on the
// public listing (store.ListPublicHostOrgs), and it must not have a page
// either — a page would disclose that it exists and what it is called.
func TestOrgPage_DraftOnlyOrgHasNoPage(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "onstage")

	token, userID := h.signupUser("quiet-orgpage@example.com", "owner-password-123", "Quiet")
	orgID := h.newOrgWithOwner("Not Announced Yet", "not-announced-yet", userID)
	h.newDraftEvent(t, orgID, token, "secret-orgpage-draft")

	bySlug := h.orgPage("/o/not-announced-yet")
	if bySlug.Code != http.StatusNotFound {
		t.Errorf("a draft-only organisation has a public page (status %d) — its very existence is the leak", bySlug.Code)
	}
	if byID := h.orgPage("/o/" + orgID); byID.Code != http.StatusNotFound {
		t.Errorf("a draft-only organisation is reachable by id (status %d)", byID.Code)
	}
	if got, want := bySlug.Body.String(), h.orgPage("/o/no-such-organisation").Body.String(); got != want {
		t.Errorf("a draft-only organisation answers differently from a nonexistent one:\n got: %s\nwant: %s", got, want)
	}
	// The org's own owner gets the same answer. This route reads no session
	// at all, and that is the property: there is no authorisation check here
	// to get wrong.
	if rec := h.do(http.MethodGet, "/o/not-announced-yet", token, nil); rec.Code != http.StatusNotFound {
		t.Errorf("the organisation's own owner got status %d; /o/ has no preview mode", rec.Code)
	}
}

// The deliberate exception, spelled out so nobody "fixes" it: in "single"
// scope the configured organisation is IN SCOPE by configuration, published
// events or not. hostScope says so in as many words — a venue between
// programmes is still that venue — so its page exists and shows the honest
// empty state rather than 404.
//
// This is not a second way past the draft-only rule: the operator has named
// this organisation as the identity of the whole box, and GET /api/events
// already returns it in host.organisations.
func TestOrgPage_SingleScopeConfiguredOrgHasAPageWithNothingOn(t *testing.T) {
	h := newTestHarness(t)
	token, userID := h.signupUser("vacant@example.com", "owner-password-123", "Vacant")
	orgID := h.newOrgWithOwner("The Empty Room", "the-empty-room", userID)
	h.newDraftEvent(t, orgID, token, "not-yet-announced")

	h.cfg.HostScope = config.HostScopeSingle
	h.cfg.HostOrg = "the-empty-room"

	rec := h.orgPage("/o/the-empty-room")
	if rec.Code != http.StatusOK {
		t.Fatalf("the configured organisation has no page: status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Nothing on sale right now") {
		t.Error("the page does not show the empty state")
	}
	if strings.Contains(body, "not-yet-announced") {
		t.Error("the draft leaked through the empty state")
	}
}

// The same locked-down headers an event page gets, and the one invariant that
// silently breaks a nonce policy: the value in the header must equal the value
// on the <style> element.
func TestOrgPage_CarriesTheHostPageSecurityHeaders(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "headers")

	rec := h.orgPage("/o/test-org-headers")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'none'", "script-src 'none'", "form-action 'none'",
		"base-uri 'none'", "frame-ancestors 'none'", "sandbox " + hostPageSandbox,
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP is missing %q; got %q", want, csp)
		}
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex" {
		t.Errorf("X-Robots-Tag = %q, want noindex", got)
	}

	nonceInHeader := regexp.MustCompile(`style-src 'nonce-([^']+)'`).FindStringSubmatch(csp)
	if nonceInHeader == nil {
		t.Fatalf("no style-src nonce in the CSP: %q", csp)
	}
	if !strings.Contains(rec.Body.String(), `nonce="`+nonceInHeader[1]+`"`) {
		t.Error("the nonce on the <style> element does not match the one in the header; the page's own stylesheet would be blocked")
	}
}

// The route does not collide with an event slug, because it cannot: /h/ and
// /o/ are separate namespaces resolved by separate handlers. An event whose
// slug is the same string as an organisation's slug resolves to the event
// under /h/ and to the organisation under /o/, with no precedence rule
// involved.
func TestOrgPage_EventSlugAndOrgSlugDoNotCollide(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "twin")

	// Give an event the ORGANISATION's slug. Nothing stops this — an event
	// slug is validated only as non-empty.
	clash := h.newDraftEvent(t, fx.orgID, fx.ownerToken, "test-org-twin")
	h.publishEvent(t, clash, fx.ownerToken)

	ev := h.do(http.MethodGet, "/h/test-org-twin", "", nil)
	if ev.Code != http.StatusOK || !strings.Contains(ev.Body.String(), "Draft test-org-twin") {
		t.Errorf("/h/test-org-twin no longer resolves to the event that owns that slug (status %d)", ev.Code)
	}
	org := h.orgPage("/o/test-org-twin")
	if org.Code != http.StatusOK || !strings.Contains(org.Body.String(), "Test Org twin") {
		t.Errorf("/o/test-org-twin does not resolve to the organisation (status %d)", org.Code)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// orgIDBySlug reads an org's id straight out of the database. Deliberately not
// over HTTP: the only public route that would give it up is the one under
// test, and a test that used it to build its own input would be circular.
func (h *testHarness) orgIDBySlug(t *testing.T, slug string) string {
	t.Helper()
	org, err := h.store.GetOrgBySlug(t.Context(), slug)
	if err != nil {
		t.Fatalf("look up org %q: %v", slug, err)
	}
	return org.ID
}

// assertSameHeaders compares two responses on every header whose value is
// deterministic. Content-Security-Policy is excluded because it carries a
// fresh random nonce per response — its SHAPE is checked in
// TestOrgPage_CarriesTheHostPageSecurityHeaders instead.
func assertSameHeaders(t *testing.T, a, b *httptest.ResponseRecorder) {
	t.Helper()
	skip := map[string]bool{"Content-Security-Policy": true, "Date": true, "Content-Length": true}
	seen := map[string]bool{}
	for k := range a.Header() {
		seen[k] = true
	}
	for k := range b.Header() {
		seen[k] = true
	}
	for k := range seen {
		if skip[k] {
			continue
		}
		if got, want := a.Header().Get(k), b.Header().Get(k); got != want {
			t.Errorf("header %s differs between an out-of-scope and a nonexistent organisation: %q vs %q", k, got, want)
		}
	}
}
