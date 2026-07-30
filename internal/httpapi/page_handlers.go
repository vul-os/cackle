package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/pages"
	"github.com/vul-os/cackle/internal/store"
)

// Host pages: the public HTML page for an event, and the API a host manages it
// through. See docs/HOST-PAGES.md for the format and the reasoning; this file
// is the HTTP surface and the tenancy enforcement.
//
// The three properties this file is responsible for, none of which
// internal/pages can enforce on its own because it has no database:
//
//  1. A host may only write the page of an event whose org they administer
//     (auth.CanManageEvent, RoleAdmin — the same bar as editing the event).
//  2. A page may only reference images from ITS OWN event's gallery, checked
//     against the database before the document is stored. internal/pages
//     checks it again at render time against the gallery it is handed, so a
//     foreign id cannot become a /media/ URL even if it somehow reached the
//     column.
//  3. The PUBLIC page exists only for a published event. A draft's page — and
//     therefore a draft's very existence, title and line-up — is served only to
//     an admin/owner of its own org, as a preview, and is reported as 404 (not
//     403) to everyone else.
//
// The persistence below is narrow SQL against Store.DB(), the same pattern and
// for the same reason as dbutil.go: internal/store does not need to grow
// route-shaped methods for one table, and internal/pages is deliberately pure
// (no store import at all — see its package doc).

// hostPageStore is the whole persistence surface for host pages.
type hostPageStore struct{ db *sql.DB }

type storedPage struct {
	Document  []byte
	Version   int
	UpdatedAt time.Time
	CreatedAt time.Time
}

// get returns the stored page for eventID, or store.ErrNotFound if the event
// has never had one saved (which is not an error condition — it means the
// event uses the zero-configuration default page).
func (s hostPageStore) get(ctx context.Context, eventID string) (storedPage, error) {
	var (
		p                    storedPage
		doc                  string
		createdAt, updatedAt string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT document, version, created_at, updated_at FROM event_pages WHERE event_id = ?`,
		eventID,
	).Scan(&doc, &p.Version, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return storedPage{}, store.ErrNotFound
	}
	if err != nil {
		return storedPage{}, err
	}
	p.Document = []byte(doc)
	// RFC3339/UTC, matching internal/store.timeToText — every other timestamp
	// column in this schema is written that way, and a column that agreed with
	// nothing else would be a trap for the first person to join across it.
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return p, nil
}

// put inserts or replaces eventID's page. document must already be
// pages.Document.Canonical() output — this method does not validate, and the
// only caller that reaches it has just parsed and validated.
func (s hostPageStore) put(ctx context.Context, eventID string, version int, document []byte, updatedBy string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO event_pages (event_id, version, document, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(event_id) DO UPDATE SET
			version    = excluded.version,
			document   = excluded.document,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`,
		eventID, version, string(document), updatedBy, now, now)
	return err
}

func (s hostPageStore) delete(ctx context.Context, eventID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM event_pages WHERE event_id = ?`, eventID)
	return err
}

// galleryIDs returns the set of image ids belonging to eventID. This is the
// authority for "may this page reference this image": it is scoped by
// event_id, so an id from another event — or another org — simply is not in
// the set.
func (s hostPageStore) galleryIDs(ctx context.Context, eventID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM images WHERE event_id = ?`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *server) pageStore() hostPageStore { return hostPageStore{db: s.deps.Store.DB()} }

// hostPageCSP is the Content-Security-Policy served with every host page. It
// is a great deal stricter than the app-wide policy in middleware.go, and it
// replaces it outright on this route.
//
// Read it as a list of things a host page is structurally incapable of doing:
//
//	default-src 'none'      nothing loads unless a directive below allows it.
//	script-src  'none'      no script, ever. internal/pages cannot emit one —
//	                        this makes that a browser-enforced fact rather
//	                        than a property of our template.
//	style-src   'nonce-..'  the single inline stylesheet, and nothing else.
//	                        NOT 'unsafe-inline': the page carries no style=""
//	                        attribute, so the nonce alone is sufficient, and a
//	                        style attribute introduced later fails visibly
//	                        instead of quietly widening the policy.
//	img-src     'self'      images come from /media/{id} on this origin only.
//	connect-src 'none'      nothing to talk to; there is no script to talk.
//	font-src    'none'      the font stacks are local names, never a webfont —
//	                        this is requirement (f), no third-party fetch,
//	                        enforced by the browser and not just by review.
//	form-action 'none'      a host cannot put a working credential form on a
//	                        page served from Cackle's origin. This is the
//	                        phishing directive, and it is the reason a host
//	                        page is not merely "safe because no script".
//	base-uri    'none'      no rewriting what relative URLs resolve against.
//	frame-src / frame-ancestors 'none'   no framing, in either direction.
//	sandbox                 see hostPageSandbox below.
const hostPageCSPTemplate = "default-src 'none'; " +
	"script-src 'none'; " +
	"style-src 'nonce-%s'; " +
	"img-src 'self'; " +
	"connect-src 'none'; " +
	"font-src 'none'; " +
	"object-src 'none'; " +
	"media-src 'none'; " +
	"frame-src 'none'; " +
	"form-action 'none'; " +
	"base-uri 'none'; " +
	"frame-ancestors 'none'; " +
	"sandbox " + hostPageSandbox

// hostPageSandbox is origin isolation without a second hostname.
//
// The brief asks whether host content should be served from a separate origin
// so that a host cannot reach another host's organiser session. It should —
// but requiring a second hostname would mean a second DNS record and a second
// certificate before a self-hoster can publish their first event page, which
// contradicts "works with zero configuration". CSP's sandbox directive buys
// most of the same isolation for free: the browser puts the document in an
// OPAQUE ORIGIN, so it has no access to document.cookie, no access to
// localStorage or IndexedDB for the real origin, and no same-origin
// relationship with the organiser app — even though it is served from the same
// hostname.
//
// The two capabilities granted back are the minimum a page of links needs:
//
//	allow-top-navigation-by-user-activation   a link the visitor CLICKS works.
//	                                          A page that navigates on its own
//	                                          does not (and cannot: no script).
//	allow-popups                              target=_blank links open.
//
// Deliberately NOT granted: allow-scripts, allow-forms, allow-same-origin,
// allow-modals, allow-downloads, allow-popups-to-escape-sandbox. Any of those
// would hand back part of what the sandbox is here to take away.
//
// A browser that does not implement the sandbox directive ignores it and is
// still left with script-src 'none' and form-action 'none'; this hardens the
// design, it is not load-bearing for it. An operator who wants true origin
// separation as well can reverse-proxy /h/ onto its own hostname — that is a
// deployment choice, documented in docs/HOST-PAGES.md, and nothing here
// assumes it either way.
const hostPageSandbox = "allow-top-navigation-by-user-activation allow-popups"

// newCSPNonce mints the per-response nonce shared between the CSP header and
// the page's single <style> element. 16 bytes from crypto/rand: a guessable
// nonce is the one way a nonce-based policy degrades to no policy at all.
//
// URL-safe base64 (alphabet A-Za-z0-9-_), NOT the standard alphabet. CSP's
// base64-value grammar accepts both, but html/template escapes a '+' inside an
// attribute value to "&#43;" — so a standard-alphabet nonce silently stops
// matching the header roughly three times in four, and the page's stylesheet is
// then blocked by its own policy. The alphabet is the fix; the test that
// compares the two values is what keeps it fixed.
func newCSPNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// resolveEventRef resolves a slug-or-id reference to an event of any status.
func (s *server) resolveEventRef(ctx context.Context, ref string) (*events.Event, error) {
	ev, err := s.deps.Events.GetBySlug(ctx, ref)
	if errors.Is(err, store.ErrNotFound) {
		ev, err = s.deps.Events.Get(ctx, ref)
	}
	return ev, err
}

// resolveViewableEvent resolves a reference to an event the CALLER may see the
// page of, and reports whether this is a draft preview.
//
// A published (or cancelled) event's page is public. A DRAFT's page is visible
// only to an admin/owner of its org — which is not a loophole but the feature
// that makes the rest workable: without it a host cannot look at the page they
// are building until they publish the event, and "publish it to see it" is how
// you get half-finished events on a public listing.
//
// A draft the caller may not see is reported as store.ErrNotFound, never as
// forbidden. "This draft exists but you may not see it" is itself the leak.
func (s *server) resolveViewableEvent(r *http.Request, ref string) (ev *events.Event, preview bool, err error) {
	ev, err = s.resolveEventRef(r.Context(), ref)
	if err != nil {
		return nil, false, err
	}
	if ev.Status != "draft" {
		return ev, false, nil
	}

	user, ok := userFromContext(r.Context())
	if !ok {
		return nil, false, store.ErrNotFound
	}
	allowed, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, ev.ID, auth.RoleAdmin)
	if err != nil {
		return nil, false, err
	}
	if !allowed {
		return nil, false, store.ErrNotFound
	}
	return ev, true, nil
}

// handleHostPage serves GET /h/{slugOrID} — the public, server-rendered host
// page. No auth, no session, no JavaScript, no third-party request.
//
// It is registered on the root router (like /media/{id}) rather than under
// /api, because it is a page rather than an API resource; chi resolves the
// longest static prefix first, so it can never be shadowed by, or shadow, the
// SPA catch-all.
func (s *server) handleHostPage(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")

	ev, preview, err := s.resolveViewableEvent(r, ref)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeHostPageNotFound(w)
			return
		}
		internalError(w, s.log(), "host page: resolve event", err)
		return
	}

	doc, err := s.loadPageDocument(r.Context(), ev)
	if err != nil {
		internalError(w, s.log(), "host page: load document", err)
		return
	}

	types, err := s.deps.Events.ListTicketTypes(r.Context(), ev.ID)
	if err != nil {
		internalError(w, s.log(), "host page: ticket types", err)
		return
	}
	images, err := s.deps.Store.ListImagesByEvent(r.Context(), ev.ID)
	if err != nil {
		internalError(w, s.log(), "host page: gallery", err)
		return
	}

	nonce, err := newCSPNonce()
	if err != nil {
		internalError(w, s.log(), "host page: nonce", err)
		return
	}

	in := pages.RenderInput{
		Doc:     doc,
		Event:   toPagesEvent(ev),
		Tickets: toPagesTickets(types),
		BuyHref: "/events/" + ev.Slug,
		Nonce:   nonce,
	}
	for _, img := range images {
		in.AllowedImages = append(in.AllowedImages, pages.Image{ID: img.ID, Width: img.Width, Height: img.Height})
	}

	html, err := pages.Render(in)
	if err != nil {
		internalError(w, s.log(), "host page: render", err)
		return
	}

	s.setHostPageHeaders(w, nonce)
	if preview {
		// A draft preview is authenticated content. It must not be stored by
		// a shared cache, and it must not be reachable from the address bar
		// of anyone who is not the organiser.
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Cackle-Page-State", "draft-preview")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

// setHostPageHeaders replaces the app-wide security headers with the much
// tighter host-page set. Set() (not Add()) on every one of them, so the value
// from middleware.go is REPLACED rather than joined — two CSP headers would be
// intersected by the browser, which happens to be safe here, but relying on
// that would be relying on an accident.
func (s *server) setHostPageHeaders(w http.ResponseWriter, nonce string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", fmt.Sprintf(hostPageCSPTemplate, nonce))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	// The app-wide policy allows the camera same-origin for the gate scanner.
	// A host page has no use for any of it.
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), midi=()")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	// Host content is user-generated and is not a search-ranking asset of the
	// deployment that happens to host it.
	h.Set("X-Robots-Tag", "noindex")
	h.Set("Cache-Control", "public, max-age=0, must-revalidate")
}

// writeHostPageNotFound answers with HTML rather than the API's JSON error
// envelope: this route is reached by a browser following a link, and a raw
// JSON body is not an answer to a person. It carries the same locked-down
// headers as a real page, and it says nothing about whether the event exists
// as a draft.
func (s *server) writeHostPageNotFound(w http.ResponseWriter) {
	nonce, err := newCSPNonce()
	if err != nil {
		internalError(w, s.log(), "host page: nonce", err)
		return
	}
	s.setHostPageHeaders(w, nonce)
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">"+
		"<title>Not found</title></head><body><h1>Not found</h1>"+
		"<p>There is no published event at this address.</p></body></html>\n")
}

// loadPageDocument returns the event's stored document, or the zero-
// configuration default if it has none.
//
// A stored document that no longer validates is treated as absent and the
// default is rendered instead. That can only happen if the rules tightened
// under an already-saved page, and the alternative — refusing to serve the
// page at all — would take a host's event offline because of a change they had
// no part in. The event's own record is always enough to render something
// truthful.
func (s *server) loadPageDocument(ctx context.Context, ev *events.Event) (pages.Document, error) {
	stored, err := s.pageStore().get(ctx, ev.ID)
	if errors.Is(err, store.ErrNotFound) {
		return pages.Default(toPagesEvent(ev)), nil
	}
	if err != nil {
		return pages.Document{}, err
	}
	doc, err := pages.Parse(stored.Document)
	if err != nil {
		s.log().Warn("host page: stored document no longer validates; serving the default page",
			"event_id", ev.ID, "error", err)
		return pages.Default(toPagesEvent(ev)), nil
	}
	return doc, nil
}

func toPagesEvent(ev *events.Event) pages.Event {
	return pages.Event{
		Slug:        ev.Slug,
		Title:       ev.Title,
		Summary:     ev.Summary,
		Description: ev.Description,
		VenueName:   ev.VenueName,
		Address:     ev.Address,
		StartsAt:    ev.StartsAt,
		EndsAt:      ev.EndsAt,
		Timezone:    ev.Timezone,
		Status:      ev.Status,
		Currency:    ev.Currency,
	}
}

// toPagesTickets maps ticket types for display. A type is shown as sold out
// when its inventory is exhausted; quantity_total of 0 means "unlimited" in
// this schema, so it is never sold out.
func toPagesTickets(types []events.TicketType) []pages.TicketType {
	out := make([]pages.TicketType, 0, len(types))
	for _, tt := range types {
		if tt.Status != "active" {
			continue
		}
		out = append(out, pages.TicketType{
			ID:          tt.ID,
			Name:        tt.Name,
			Description: tt.Description,
			PriceMinor:  tt.PriceMinor,
			SoldOut:     tt.QuantityTotal > 0 && tt.QuantitySold >= tt.QuantityTotal,
		})
	}
	return out
}

// pageView is the JSON shape of GET/PUT /api/events/{ref}/page.
type pageView struct {
	// Document is the host's stored document, or null when the event has
	// never had one saved.
	Document json.RawMessage `json:"document"`
	// IsDefault reports that Document is Cackle's zero-configuration default
	// rather than something a host wrote. A page editor uses it to decide
	// between "edit" and "start from the default".
	IsDefault bool `json:"is_default"`
	// URL is where the rendered page is served.
	URL       string     `json:"url"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// handleGetEventPage serves GET /api/events/{ref}/page — the JSON mirror of the
// HTML route, with exactly the same visibility rule: public for a published
// event, organiser-only for a draft. This is what a host fetches to render the
// page themselves, on their own origin, from their own code — and what a page
// editor loads to show the current document.
func (s *server) handleGetEventPage(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "id")

	ev, _, err := s.resolveViewableEvent(r, ref)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return
		}
		internalError(w, s.log(), "get event page: resolve event", err)
		return
	}
	s.writePageView(w, r, ev)
}

func (s *server) writePageView(w http.ResponseWriter, r *http.Request, ev *events.Event) {
	view := pageView{URL: "/h/" + ev.Slug}

	stored, err := s.pageStore().get(r.Context(), ev.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		doc := pages.Default(toPagesEvent(ev))
		canon, cerr := doc.Canonical()
		if cerr != nil {
			internalError(w, s.log(), "get event page: canonicalise default", cerr)
			return
		}
		view.Document = canon
		view.IsDefault = true
	case err != nil:
		internalError(w, s.log(), "get event page", err)
		return
	default:
		view.Document = stored.Document
		updated := stored.UpdatedAt
		view.UpdatedAt = &updated
	}

	writeJSON(w, http.StatusOK, map[string]any{"page": view})
}

// handlePutEventPage serves PUT /api/events/{id}/page — admin+ on the event's
// org, the same bar as editing the event itself.
//
// The body is the document, not a wrapper: PUT of a resource is the resource.
func (s *server) handlePutEventPage(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	ref := chi.URLParam(r, "id")

	ev, ok := s.eventForPageManagement(w, r, ref)
	if !ok {
		return
	}

	// Bounded before it is buffered. MaxBytesReader's cutoff is one byte past
	// the document limit so that a body AT the limit still reads cleanly and
	// only a body OVER it trips, letting pages.Parse produce the precise
	// error rather than a generic read failure.
	r.Body = http.MaxBytesReader(w, r.Body, pages.MaxDocumentBytes+1)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		badRequest(w, fmt.Sprintf("request body missing or larger than %d bytes", pages.MaxDocumentBytes))
		return
	}

	doc, err := pages.Parse(raw)
	if err != nil {
		if errors.Is(err, pages.ErrInvalid) {
			// pages' messages are written to be shown to a host: they name
			// the exact JSON path at fault and contain nothing internal.
			badRequest(w, err.Error())
			return
		}
		internalError(w, s.log(), "put event page: parse", err)
		return
	}

	// TENANCY. Every image the document references must be in THIS event's own
	// gallery. Without this an admin of org B could embed org A's images on
	// their own page — reading, and republishing, another host's content.
	if !s.pageImagesAreOwnedByEvent(w, r, ev.ID, doc) {
		return
	}

	canon, err := doc.Canonical()
	if err != nil {
		internalError(w, s.log(), "put event page: canonicalise", err)
		return
	}
	if err := s.pageStore().put(r.Context(), ev.ID, doc.Version, canon, user.ID); err != nil {
		internalError(w, s.log(), "put event page: store", err)
		return
	}

	s.writePageView(w, r, ev)
}

// pageImagesAreOwnedByEvent reports whether every image the document
// references belongs to eventID. It writes the error response itself when the
// answer is no.
func (s *server) pageImagesAreOwnedByEvent(w http.ResponseWriter, r *http.Request, eventID string, doc pages.Document) bool {
	ids := doc.ImageIDs()
	if len(ids) == 0 {
		return true
	}
	gallery, err := s.pageStore().galleryIDs(r.Context(), eventID)
	if err != nil {
		internalError(w, s.log(), "put event page: gallery", err)
		return false
	}
	for _, id := range ids {
		if !gallery[id] {
			// Phrased so it says nothing about whether the id exists at all
			// somewhere else — an ownership check that doubles as an
			// existence oracle for other orgs' images would be a smaller leak
			// put in place of a larger one.
			badRequest(w, fmt.Sprintf("image_id %q is not in this event's gallery; upload it to this event first", id))
			return false
		}
	}
	return true
}

// handleDeleteEventPage serves DELETE /api/events/{id}/page — admin+. It
// reverts the event to the zero-configuration default page; it never leaves the
// event without a page.
func (s *server) handleDeleteEventPage(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "id")

	ev, ok := s.eventForPageManagement(w, r, ref)
	if !ok {
		return
	}
	if err := s.pageStore().delete(r.Context(), ev.ID); err != nil {
		internalError(w, s.log(), "delete event page", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// eventForPageManagement resolves a slug-or-id reference to an event of ANY
// status (a host must be able to build a page before publishing) and checks the
// caller is an admin/owner of its org. It writes the error response itself and
// reports whether the caller may proceed.
func (s *server) eventForPageManagement(w http.ResponseWriter, r *http.Request, ref string) (*events.Event, bool) {
	user, _ := userFromContext(r.Context())

	ev, err := s.resolveEventRef(r.Context(), ref)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return nil, false
		}
		internalError(w, s.log(), "event page rbac: resolve event", err)
		return nil, false
	}

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, ev.ID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "event page rbac", err)
		return nil, false
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return nil, false
	}
	return ev, true
}
