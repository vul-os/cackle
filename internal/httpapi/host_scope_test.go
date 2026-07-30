package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/events"
)

// These tests are about ONE claim: the public listing says whose events it is
// showing, and shows only those. Cackle is self-hosted by an organiser, so
// "every published row in the table" was never the honest answer to GET
// /api/events — it just happened to be the only one available.

type listingResponse struct {
	Events []events.Event `json:"events"`
	Host   struct {
		Scope         string `json:"scope"`
		Name          string `json:"name"`
		MultiOrg      bool   `json:"multi_org"`
		PeersIncluded bool   `json:"peers_included"`
		Organisations []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"organisations"`
		Org *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"org"`
	} `json:"host"`
}

func (h *testHarness) listing(t *testing.T, path string) listingResponse {
	t.Helper()
	rec := h.do(http.MethodGet, path, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d body %s", path, rec.Code, rec.Body.String())
	}
	return decodeBody[listingResponse](t, rec)
}

func (r listingResponse) slugs() []string {
	out := make([]string, 0, len(r.Events))
	for _, e := range r.Events {
		out = append(out, e.Slug)
	}
	return out
}

func (r listingResponse) orgSlugs() []string {
	out := make([]string, 0, len(r.Host.Organisations))
	for _, o := range r.Host.Organisations {
		out = append(out, o.Slug)
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

// One organisation on the box: the listing must NOT dress it up as a
// directory. multi_org false is the switch a client uses to leave out every
// bit of per-organisation chrome.
func TestHostScope_SingleOrgBoxIsNotADirectory(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "solo")

	got := h.listing(t, "/api/events")
	if got.Host.Scope != string(config.HostScopeOwn) {
		t.Errorf("scope = %q, want %q (the default)", got.Host.Scope, config.HostScopeOwn)
	}
	if got.Host.MultiOrg {
		t.Error("multi_org = true on a box hosting one organisation")
	}
	if len(got.Host.Organisations) != 1 {
		t.Fatalf("organisations = %v, want exactly one", got.orgSlugs())
	}
	if got.Host.Organisations[0].Name != "Test Org solo" {
		t.Errorf("organisation name = %q, want %q", got.Host.Organisations[0].Name, "Test Org solo")
	}
	if got.Host.Name != "" {
		t.Errorf("host name = %q; an unnamed host must stay unnamed, not be invented", got.Host.Name)
	}
}

// Several organisations: the listing must say so, and must carry enough to
// label each event with the organisation it belongs to.
func TestHostScope_MultiOrgBoxNamesEveryOrganisation(t *testing.T) {
	h := newTestHarness(t)
	a := h.newPublishedEvent(t, "alpha")
	b := h.newPublishedEvent(t, "beta")

	got := h.listing(t, "/api/events")
	if !got.Host.MultiOrg {
		t.Error("multi_org = false on a box hosting two organisations")
	}
	if len(got.Host.Organisations) != 2 {
		t.Fatalf("organisations = %v, want two", got.orgSlugs())
	}
	// Every returned event's org_id must be resolvable from the envelope —
	// otherwise a client cannot label it and would have to guess or omit.
	known := map[string]bool{}
	for _, o := range got.Host.Organisations {
		known[o.ID] = true
	}
	for _, e := range got.Events {
		if !known[e.OrgID] {
			t.Errorf("event %q belongs to org %q, which the host envelope does not name", e.Slug, e.OrgID)
		}
	}
	if !known[a.orgID] || !known[b.orgID] {
		t.Errorf("organisations %v do not cover both fixtures", got.orgSlugs())
	}
}

// An organisation with only drafts has published nothing, so naming it on a
// public, unauthenticated route would disclose that it exists and what it is
// called.
func TestHostScope_DraftOnlyOrgIsNotNamedPublicly(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "onstage")

	token, userID := h.signupUser("quiet@example.com", "owner-password-123", "Quiet")
	orgID := h.newOrgWithOwner("Not Announced Yet", "not-announced-yet", userID)
	h.newDraftEvent(t, orgID, token, "secret-draft")

	got := h.listing(t, "/api/events")
	if contains(got.orgSlugs(), "not-announced-yet") {
		t.Errorf("a draft-only organisation is named publicly: %v", got.orgSlugs())
	}
	if got.Host.MultiOrg {
		t.Error("multi_org = true because of an organisation that has published nothing")
	}
}

// single scope: the box presents as ONE organisation, and the other
// organisation's published events are not listed.
func TestHostScope_SinglePresentsOneOrgAndHidesTheRest(t *testing.T) {
	h := newTestHarness(t)
	mine := h.newPublishedEvent(t, "bijou")
	h.newPublishedEvent(t, "lodger")

	h.cfg.HostScope = config.HostScopeSingle
	h.cfg.HostOrg = "test-org-bijou"

	got := h.listing(t, "/api/events")
	if got.Host.Scope != string(config.HostScopeSingle) {
		t.Errorf("scope = %q, want %q", got.Host.Scope, config.HostScopeSingle)
	}
	if got.Host.MultiOrg {
		t.Error("multi_org = true in single scope")
	}
	if len(got.Host.Organisations) != 1 || got.Host.Organisations[0].ID != mine.orgID {
		t.Fatalf("organisations = %v, want only the configured one", got.orgSlugs())
	}
	if !contains(got.slugs(), "test-event-bijou") {
		t.Errorf("configured organisation's own event is missing: %v", got.slugs())
	}
	if contains(got.slugs(), "test-event-lodger") {
		t.Errorf("another organisation's event is listed in single scope: %v", got.slugs())
	}
	// With no CACKLE_HOST_NAME, the organisation's own name is the truthful
	// fallback — the box IS that organisation.
	if got.Host.Name != "Test Org bijou" {
		t.Errorf("host name = %q, want the organisation's own name", got.Host.Name)
	}
}

// CACKLE_HOST_NAME wins over the organisation's own name, in every scope.
func TestHostScope_OperatorNameWins(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "named")
	h.cfg.HostName = "The Bijou"

	if got := h.listing(t, "/api/events"); got.Host.Name != "The Bijou" {
		t.Errorf("host name = %q, want %q", got.Host.Name, "The Bijou")
	}

	h.cfg.HostScope = config.HostScopeSingle
	h.cfg.HostOrg = "test-org-named"
	if got := h.listing(t, "/api/events"); got.Host.Name != "The Bijou" {
		t.Errorf("single scope host name = %q, want %q", got.Host.Name, "The Bijou")
	}
}

// The failure that matters: a single-scope box whose configured organisation
// cannot be resolved must NOT fall back to listing everybody. It fails.
func TestHostScope_UnresolvableSingleOrgFailsClosed(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "elsewhere")

	h.cfg.HostScope = config.HostScopeSingle
	h.cfg.HostOrg = "no-such-org"

	rec := h.do(http.MethodGet, "/api/events", "", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d body %s; want 500 — a scope that cannot be resolved must fail, never widen",
			rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "test-event-elsewhere") {
		t.Errorf("the failure response leaked an event it was configured not to show: %s", body)
	}
}

// ?host= narrows to one organisation — the link behind an event's
// organisation label on a multi-organisation box.
func TestHostScope_HostParamNarrowsToOneOrganisation(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "one")
	h.newPublishedEvent(t, "two")

	got := h.listing(t, "/api/events?host=test-org-one")
	if got.Host.Org == nil || got.Host.Org.Slug != "test-org-one" {
		t.Fatalf("host.org = %+v, want the requested organisation", got.Host.Org)
	}
	if !got.Host.MultiOrg {
		t.Error("multi_org = false; narrowing the view must not rewrite what the box IS")
	}
	if len(got.Host.Organisations) != 2 {
		t.Errorf("organisations = %v, want both — the box still hosts two", got.orgSlugs())
	}
	if !contains(got.slugs(), "test-event-one") || contains(got.slugs(), "test-event-two") {
		t.Errorf("events = %v, want only the requested organisation's", got.slugs())
	}
}

// An organisation that is out of scope, or that does not exist, answers the
// same way: not found. Otherwise ?host= becomes a probe for which
// organisations live on a box.
func TestHostScope_HostParamOutOfScopeIs404(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "here")
	h.newPublishedEvent(t, "there")

	h.cfg.HostScope = config.HostScopeSingle
	h.cfg.HostOrg = "test-org-here"

	for _, ref := range []string{"test-org-there", "does-not-exist-at-all"} {
		rec := h.do(http.MethodGet, "/api/events?host="+ref, "", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /api/events?host=%s: status %d, want 404", ref, rec.Code)
		}
	}
}

// The peers scope is a NAME today, not a feature. It must behave exactly as
// own and must say, on the wire, that no peer event is in the list. If this
// test starts failing because peer events became real, docs/CONFIGURATION.md's
// claim that they are not has to change in the same commit.
func TestHostScope_PeersBehavesExactlyAsOwnAndSaysSo(t *testing.T) {
	h := newTestHarness(t)
	h.newPublishedEvent(t, "local-a")
	h.newPublishedEvent(t, "local-b")

	own := h.listing(t, "/api/events")

	h.cfg.HostScope = config.HostScopePeers
	peers := h.listing(t, "/api/events")

	if peers.Host.Scope != string(config.HostScopePeers) {
		t.Errorf("scope = %q, want %q", peers.Host.Scope, config.HostScopePeers)
	}
	if peers.Host.PeersIncluded {
		t.Error("peers_included = true, but this binary has no peer event source")
	}
	if own.Host.PeersIncluded {
		t.Error("peers_included = true in own scope")
	}
	if len(peers.Events) != len(own.Events) {
		t.Fatalf("peers scope returned %d events, own returned %d; they must be identical today",
			len(peers.Events), len(own.Events))
	}
	for i := range peers.Events {
		if peers.Events[i].ID != own.Events[i].ID {
			t.Fatalf("peers scope event %d = %q, own = %q; they must be identical today",
				i, peers.Events[i].Slug, own.Events[i].Slug)
		}
	}
	if len(peers.Host.Organisations) != len(own.Host.Organisations) {
		t.Errorf("peers scope names %d organisations, own names %d",
			len(peers.Host.Organisations), len(own.Host.Organisations))
	}
}

// An empty box answers with an empty ARRAY, not null: a client that renders
// host.organisations must not have to special-case a missing one.
func TestHostScope_EmptyBoxAnswersWithAnEmptyArray(t *testing.T) {
	h := newTestHarness(t)

	rec := h.do(http.MethodGet, "/api/events", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"organisations":[]`) {
		t.Errorf("body %s does not carry an empty organisations array", body)
	}
}
