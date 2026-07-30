package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/store"
)

// The host display scope: whose events the public listing shows.
//
// Cackle is self-hosted by an organiser. A root page that reads "browse all
// events" describes a global marketplace, and there isn't one — there is a box,
// run by somebody, with their own events on it. This file is the part of that
// correction that lives on the wire: GET /api/events answers with WHOSE events
// these are, not just with a list, so no client has to guess and none can
// imply more than exists.
//
// The setting itself is internal/config.HostScope (CACKLE_HOST_SCOPE).

// hostOrgView is one organisation on this box, as the public listing names it.
// It carries the org's public handle and nothing else: no member, no bank
// account, no default currency, no created_at. This is an unauthenticated
// route, and an org here has only published events to be known by.
type hostOrgView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// hostView is the "whose events are these" envelope on GET /api/events.
type hostView struct {
	// Scope is the configured host display scope: "own", "single" or
	// "peers". See docs/CONFIGURATION.md.
	Scope string `json:"scope"`
	// Name is the operator's own name for this box (CACKLE_HOST_NAME), or
	// in "single" scope the organisation's own name when none is set.
	// Empty means the operator has not named their host and the client must
	// NOT invent one — a heading with no name is correct; a made-up one is
	// not.
	Name string `json:"name,omitempty"`
	// Organisations is every organisation whose events this listing can
	// contain. Never null: an empty array means this box has nothing
	// published.
	Organisations []hostOrgView `json:"organisations"`
	// MultiOrg reports that this box hosts more than one organisation, and
	// is therefore the switch for per-event organisation labelling in the
	// UI. It is deliberately NOT derived from the events on the current
	// page of results — a search that happens to match one organisation
	// must not make a multi-organisation box present as a single venue,
	// and a single venue must never be dressed up as a directory.
	MultiOrg bool `json:"multi_org"`
	// PeersIncluded reports whether any event in this listing came from
	// another box. It is false in every scope, including "peers", because
	// no peer event source exists in this binary yet. See
	// config.HostScope.IncludesPeerEvents.
	PeersIncluded bool `json:"peers_included"`
	// Org, when set, is the single organisation this listing was narrowed
	// to by ?host=. Absent otherwise.
	Org *hostOrgView `json:"org,omitempty"`
}

// errHostOrgUnresolved means CACKLE_HOST_ORG names no organisation on this
// box. It is a configuration fault, reported as a 500, never as an empty
// listing and never by widening to every organisation.
var errHostOrgUnresolved = errors.New("httpapi: CACKLE_HOST_ORG names no organisation on this host")

// hostScope resolves the configured scope into the set of organisations the
// public listing may show, and the org ids to filter events by.
//
// The returned orgIDs slice is NEVER nil: it is a whitelist, and "restricted
// to nothing" must stay distinguishable from "unrestricted" all the way down
// to the SQL (see store.ListPublishedEvents). That is why "own" passes the
// resolved ids rather than nil even though the filter is a no-op today —
// every event row on this box belongs to an org on this box, so it changes no
// result, and it means the listing is structurally a whitelist rather than
// "whatever is in the table".
func (s *server) hostScope(ctx context.Context) (scope config.HostScope, orgs []hostOrgView, orgIDs []string, err error) {
	scope = config.HostScopeOwn
	if s.deps.Config != nil && s.deps.Config.HostScope != "" {
		scope = s.deps.Config.HostScope
	}

	switch scope {
	case config.HostScopeSingle:
		ref := s.deps.Config.HostOrg
		org, err := s.deps.Store.GetOrgBySlug(ctx, ref)
		if errors.Is(err, store.ErrNotFound) {
			org, err = s.deps.Store.GetOrgByID(ctx, ref)
		}
		if errors.Is(err, store.ErrNotFound) {
			return scope, nil, nil, fmt.Errorf("%w: %q", errHostOrgUnresolved, ref)
		}
		if err != nil {
			return scope, nil, nil, err
		}
		// The configured organisation is listed even with nothing on. A venue
		// between programmes is still that venue; answering with an empty
		// organisation list would leave the page with no name to put at the
		// top of "nothing on right now".
		return scope, []hostOrgView{{ID: org.ID, Name: org.Name, Slug: org.Slug}}, []string{org.ID}, nil

	default:
		// "own" and "peers" are the same set today. "peers" is the seam: when
		// an opt-in peer feed exists, THIS is where its organisations join the
		// list and where its events would be merged into the result — and
		// config.HostScope.IncludesPeerEvents is the one predicate that has to
		// start returning true. Until then the two scopes are identical, and
		// the response says so via peers_included:false rather than leaving a
		// client to assume.
		rows, err := s.deps.Store.ListPublicHostOrgs(ctx)
		if err != nil {
			return scope, nil, nil, err
		}
		orgs = make([]hostOrgView, 0, len(rows))
		orgIDs = make([]string, 0, len(rows))
		for _, o := range rows {
			orgs = append(orgs, hostOrgView{ID: o.ID, Name: o.Name, Slug: o.Slug})
			orgIDs = append(orgIDs, o.ID)
		}
		return scope, orgs, orgIDs, nil
	}
}

// hostDisplayName is what the listing calls this host. CACKLE_HOST_NAME wins;
// in "single" scope the organisation's own name is a truthful fallback,
// because the box IS that organisation. In every other scope an unset name
// stays empty — a box hosting several organisations has no name of its own
// unless its operator gave it one, and inventing one ("Cackle", the hostname)
// would put a name on a page that nobody chose.
func hostDisplayName(cfg *config.Config, scope config.HostScope, orgs []hostOrgView) string {
	if cfg != nil && cfg.HostName != "" {
		return cfg.HostName
	}
	if scope == config.HostScopeSingle && len(orgs) == 1 {
		return orgs[0].Name
	}
	return ""
}

// narrowToHostOrg applies ?host=<slug-or-id>, the link from an event's
// organisation label to that organisation's own events.
//
// The reference must name an organisation ALREADY IN SCOPE. Resolving it
// against the whole orgs table instead would turn a public listing filter into
// a probe for which organisations exist on a box — including ones that have
// published nothing — so an out-of-scope reference is reported as not found,
// exactly as a nonexistent one is.
//
// It writes the error response itself and reports whether the caller may
// proceed.
func narrowToHostOrg(w http.ResponseWriter, ref string, view *hostView, orgIDs *[]string) bool {
	if ref == "" {
		return true
	}
	for _, o := range view.Organisations {
		if o.Slug == ref || o.ID == ref {
			match := o
			view.Org = &match
			*orgIDs = []string{o.ID}
			return true
		}
	}
	notFound(w, "no such organisation on this host")
	return false
}
