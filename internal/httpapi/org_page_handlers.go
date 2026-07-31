package httpapi

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/pages"
)

// The ORGANISATION page: GET /o/{ref}.
//
// # The gap this fills
//
// GET /h/{ref} resolves ONE EVENT (page_handlers.go's handleHostPage ->
// resolveViewableEvent -> resolveEventRef -> Events.GetBySlug). Until this file
// there was no route anywhere that server-rendered a page for an ORGANISATION,
// and GET /api/events?host=<slug> only narrowed a JSON listing — nothing
// rendered it. So an organisation was named in three places (the browse page's
// org chrome, an event's organiser attribution, and federation's rule that a
// borrowed listing links back to its publisher's own box) with nowhere to send
// anyone who wanted "everything by this organiser".
//
// # Why /o/{ref} and not something under /h/
//
// An event slug is validated only as non-empty — internal/events.Service.Create
// rejects an empty slug and nothing else, and Update the same. There is no
// character set, no prefix rule, no reserved word. So no convention inside /h/
// can be reserved for organisations: /h/@acme, /h/org:acme and /h/~acme are all
// slugs an event may legitimately already hold, and any of them would resolve
// to that event first (or, worse, stop resolving to it the day the reservation
// was added). A collision cannot be made impossible inside a namespace whose
// keys are unconstrained.
//
// A separate top-level prefix makes the collision structurally impossible
// rather than resolved by precedence: event slugs are only ever looked up under
// /h/, organisation references only ever under /o/, and the two resolvers never
// see each other's input. Nothing has to be reserved and no rule has to be
// remembered.
//
// /o/ is free at both layers: it is not on chi's root router (see deps.go) and
// not in the SPA's route table (web/src/routes.jsx), so the only thing it would
// otherwise have hit is the SPA catch-all — and chi resolves the longest static
// prefix first, exactly as it does for /h/ and /media/, so registering it here
// takes it back from the catch-all and nothing else. It is one letter, matching
// /h/'s existing convention.
//
// WITHIN /o/, a reference is a slug OR an id, and the tie is broken the same
// way the JSON listing breaks it — see resolveScopedOrg.
//
// # What this route may show
//
// Three separate guards, at three separate layers, and each one is
// mutation-tested on its own in org_page_test.go:
//
//  1. IN SCOPE. The organisation must already be in this host's display scope
//     (narrowToHostOrg over hostScope's set). Out of scope answers exactly as
//     nonexistent does, so the route cannot enumerate a box.
//  2. THAT ORG ONLY. The event query is restricted to the resolved org's id.
//  3. PUBLISHED ONLY. store.ListPublishedEvents enforces status='published' in
//     SQL, so a draft and a cancelled event are both absent — from a stranger
//     and from the organisation's own owner alike, because this page has no
//     preview mode and no caller identity at all.
//
// There is deliberately NO draft-preview counterpart to resolveViewableEvent's
// here. An event page has one, because a host has to see the page they are
// building before they publish it. An organisation page is a list of what is on
// sale; there is nothing to build and nothing to preview, so this route never
// looks at the session, and cannot leak a draft to anyone by getting an
// authorisation check wrong — it does not make one.

// orgPageMaxEvents caps the programme. store.ListPublishedEvents caps at 200
// itself; this asks for that ceiling explicitly rather than inheriting the
// listing route's default of 50, because this page has no pagination and
// silently stopping at 50 would quietly hide an organiser's later events.
const orgPageMaxEvents = 200

// discardedResponse absorbs a response nobody will read.
//
// narrowToHostOrg (host_scope.go) both DECIDES whether a reference names an
// in-scope organisation and WRITES the JSON 404 when it does not. That decision
// is the anti-enumeration property this page needs to share exactly, and this
// route's 404 has to be HTML rather than a JSON envelope — a person following a
// link is owed a page, not an error object.
//
// Rather than reimplement the matching rule (two copies of an anti-enumeration
// check is how the two stop agreeing), the call is made with its response
// thrown away and the HTML 404 written here instead. The BOOLEAN is what this
// route consumes. The consequence, and the point: if the matching rule in
// host_scope.go ever changes, this page changes with it, and it is not possible
// for the page to accept an organisation the listing rejects.
type discardedResponse struct{ h http.Header }

func (d discardedResponse) Header() http.Header         { return d.h }
func (d discardedResponse) Write(b []byte) (int, error) { return len(b), nil }
func (d discardedResponse) WriteHeader(int)             {}

// resolveScopedOrg resolves a slug-or-id reference to an organisation that is
// in this host's display scope, or reports that there is none.
//
// The slug-or-id tie is broken by narrowToHostOrg's own order — it walks the
// in-scope organisations and takes the first whose Slug OR ID equals the
// reference, over a list hostScope returns in a fixed order (by name, then id,
// from store.ListPublicHostOrgs; or the single configured org). An org slug is
// unique and an org id is a ULID, so a reference can match at most one row in
// practice; the loop makes the outcome deterministic even if that ever stopped
// being true.
//
// The second return is false BOTH when the reference names nothing and when it
// names an organisation outside this host's scope. The caller cannot tell the
// two apart, and neither can a visitor: that is the anti-enumeration property.
func (s *server) resolveScopedOrg(r *http.Request, ref string) (hostOrgView, bool, error) {
	scope, orgs, _, err := s.hostScope(r.Context())
	if err != nil {
		return hostOrgView{}, false, err
	}
	view := hostView{Scope: string(scope), Organisations: orgs}
	orgIDs := []string{}
	if !narrowToHostOrg(discardedResponse{h: http.Header{}}, ref, &view, &orgIDs) {
		return hostOrgView{}, false, nil
	}
	// An empty reference is "no narrowing" to the listing route, which
	// answers with the whole box. There is no whole-box organisation page, so
	// it is not found here. (chi does not route /o/ to /o/{ref} today; this
	// does not depend on that staying true.)
	if view.Org == nil {
		return hostOrgView{}, false, nil
	}
	return *view.Org, true, nil
}

// handleOrgPage serves GET /o/{slugOrID} — the public, server-rendered
// organisation page. No auth, no session, no JavaScript, no third-party
// request, and it carries the same locked-down headers as an event page.
func (s *server) handleOrgPage(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")

	org, ok, err := s.resolveScopedOrg(r, ref)
	if err != nil {
		internalError(w, s.log(), "org page: host scope", err)
		return
	}
	if !ok {
		s.writeOrgPageNotFound(w)
		return
	}

	// Restricted to this organisation, and published-only in the SQL itself.
	// Note the org id comes from the RESOLVED in-scope organisation, never
	// from the request — a reference that reached here has already been
	// matched against the scope set, so this cannot be widened by the URL.
	evs, err := s.deps.Store.ListPublishedEvents(r.Context(), "", "", []string{org.ID}, nil, nil, orgPageMaxEvents)
	if err != nil {
		internalError(w, s.log(), "org page: list events", err)
		return
	}

	nonce, err := newCSPNonce()
	if err != nil {
		internalError(w, s.log(), "org page: nonce", err)
		return
	}

	in := pages.OrgRenderInput{Org: pages.Organisation{Name: org.Name}, Nonce: nonce}
	for _, ev := range evs {
		in.Events = append(in.Events, pages.OrgEvent{
			Slug:      ev.Slug,
			Title:     ev.Title,
			Summary:   ev.Summary,
			VenueName: ev.VenueName,
			StartsAt:  ev.StartsAt,
			EndsAt:    ev.EndsAt,
			Timezone:  ev.Timezone,
		})
	}

	html, err := pages.RenderOrg(in)
	if err != nil {
		internalError(w, s.log(), "org page: render", err)
		return
	}

	// The same header set an event page gets, including the sandbox that puts
	// the document in an opaque origin. This page carries no host-authored
	// content at all, so it needs the policy less than /h/ does — but serving
	// it under a weaker one would mean two public HTML surfaces with two
	// policies, and the weaker would become the one an attacker aims at.
	s.setHostPageHeaders(w, nonce)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

// writeOrgPageNotFound is the single answer to BOTH "no such organisation" and
// "that organisation exists but is not shown on this host". Byte-for-byte
// identical in both cases, with identical headers and an identical status — the
// moment the two differ in any observable way, /o/ becomes a probe for which
// organisations live on a box, including ones that have published nothing.
//
// It is HTML rather than the API's JSON error envelope for the same reason
// writeHostPageNotFound is: this route is reached by a browser following a
// link, and a raw JSON body is not an answer to a person. It is written here
// rather than shared with the event 404 only so the sentence can be true — an
// organisation page is not "a published event at this address".
func (s *server) writeOrgPageNotFound(w http.ResponseWriter) {
	nonce, err := newCSPNonce()
	if err != nil {
		internalError(w, s.log(), "org page: nonce", err)
		return
	}
	s.setHostPageHeaders(w, nonce)
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">"+
		"<title>Not found</title></head><body><h1>Not found</h1>"+
		"<p>There is nothing published at this address.</p></body></html>\n")
}

// compile-time proof that the discard writer really is an http.ResponseWriter,
// so narrowToHostOrg's signature cannot drift away from this call site silently.
var _ http.ResponseWriter = discardedResponse{}
