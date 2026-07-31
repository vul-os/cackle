package pages

import (
	"bytes"
	"embed"
	"html/template"
	"net/url"
	"time"
)

// The ORGANISATION page: one public, server-rendered page listing everything
// an organisation currently has on sale on this box.
//
// # Why this is a separate renderer and not another Document
//
// The event page (page.html, render.go) renders a HOST-AUTHORED document: a
// validated block format a host submits through the API, with host-chosen
// colours, fonts and copy. Almost all of internal/pages exists to make that
// safe.
//
// An organisation page has no author. Nobody submits one, there is no document
// format for it, and there is nothing on it a host chose — it is Cackle's own
// chrome around a list of rows the database already holds. Modelling it as a
// Document would have meant either inventing an org-page document format
// (a second format, a second validator, a second attack surface, for a page
// with no editor) or synthesising a links-block Document at request time — and
// that second option is not even available: validateHref admits only absolute
// http/https/mailto URLs, so a links block cannot point at "/h/{slug}" at all.
//
// So this file renders a fixed page from fixed fields. That is the whole
// difference: the event page is "render what a host wrote, safely", and this is
// "render what the box knows". It shares the CSP, the headers, the escaping
// discipline and the visual language, and shares NO trust assumption, because
// there is no host input to trust.
//
// The rules org.html follows are the same five page.html follows — no script,
// no other origin, every value a plain {{pipeline}} in its real context, no
// inline style attribute, and no logic in the template beyond ranging and
// branching on booleans decided here in Go.

//go:embed org.html
var orgTemplateFS embed.FS

// orgTemplate is parsed once at init, for the same reason pageTemplate is: a
// template that would fail to parse takes the process down at boot rather than
// on a visitor's page load. It registers no template functions either.
var orgTemplate = template.Must(template.New("org.html").ParseFS(orgTemplateFS, "org.html"))

// Organisation is the subset of an organisation its page renders. It is a
// NAME and nothing else — no member, no bank account, no default currency, no
// created_at, no id. This page is unauthenticated, and an organisation here is
// known by the events it has published and by nothing else. Adding a field to
// this struct is adding a field to a public page; do it deliberately.
type Organisation struct {
	Name string
}

// OrgEvent is one published event as the organisation page lists it.
//
// There is no Status field, and that is deliberate: the caller has already
// restricted the set to published events (internal/store.ListPublishedEvents
// enforces status='published' in SQL). A status field here would invite a
// renderer-level filter, which would mean two places decide what is visible
// and one day they would disagree. The visibility rule lives at the query, and
// this type cannot express anything else.
type OrgEvent struct {
	Slug      string
	Title     string
	Summary   string
	VenueName string
	StartsAt  time.Time
	EndsAt    time.Time
	Timezone  string
}

// OrgRenderInput is everything RenderOrg needs. As with RenderInput, every
// field is supplied by the caller: RenderOrg performs no lookups, reads no
// clock and touches no network.
type OrgRenderInput struct {
	Org    Organisation
	Events []OrgEvent
	// Nonce is a fresh, per-response CSP nonce for the single inline <style>
	// element. RenderOrg does not generate it — the same value has to appear
	// in the Content-Security-Policy header, and only the caller writing that
	// header can guarantee they match.
	Nonce string
}

// RenderOrg turns an organisation and its published events into a complete
// HTML document.
func RenderOrg(in OrgRenderInput) ([]byte, error) {
	var buf bytes.Buffer
	if err := orgTemplate.Execute(&buf, newOrgView(in)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type orgView struct {
	Nonce  string
	Name   string
	Intro  string
	Empty  bool
	Vacant string
	Events []orgEventView
}

type orgEventView struct {
	Href    string
	Title   string
	Summary string
	Venue   string
	When    timeRange
}

// newOrgView flattens the input into the render-ready shape, and is where every
// word of chrome on this page is decided.
//
// The copy is deliberately the same copy the React storefront uses for the same
// situations (web/src/lib/host.js): "Tickets sold direct by X", "Nothing on
// sale right now", "X has no tickets on sale at the moment". A visitor who
// follows a link from the app to this page must not be met with a different
// account of what they are looking at.
//
// It claims nothing beyond this box. There is no directory, nothing to search,
// nobody nearby, and no other organisation named — this page is one organiser's
// own programme, and it says only that.
func newOrgView(in OrgRenderInput) orgView {
	v := orgView{
		Nonce: in.Nonce,
		Name:  in.Org.Name,
		Intro: "Tickets sold direct by " + in.Org.Name + ".",
		Empty: len(in.Events) == 0,
		// An organisation between programmes is still that organisation. This
		// is the honest empty state, not "check back soon, new events are
		// added all the time" — nobody adds events to somebody else's box.
		Vacant: in.Org.Name + " has no tickets on sale at the moment.",
	}
	for _, e := range in.Events {
		v.Events = append(v.Events, orgEventView{
			// url.PathEscape, not raw concatenation: an event slug is
			// validated only as non-empty (internal/events.Service.Create),
			// so it can contain '/', '?' or '#'. Escaping keeps such a slug
			// inside its own path segment instead of letting it rewrite the
			// URL. For every slug the app itself produces this is the
			// identity function.
			Href:    "/h/" + url.PathEscape(e.Slug),
			Title:   e.Title,
			Summary: e.Summary,
			Venue:   e.VenueName,
			When:    formatRange(e.StartsAt, e.EndsAt, e.Timezone),
		})
	}
	return v
}
