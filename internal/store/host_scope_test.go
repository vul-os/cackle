package store

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// seedPublishedEvent creates an org and one event of the given status,
// returning the org.
func seedPublishedEvent(t *testing.T, st *Store, orgName, orgSlug, eventSlug, status string) *Org {
	t.Helper()
	ctx := context.Background()

	org := &Org{Name: orgName, Slug: orgSlug}
	if err := st.CreateOrg(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ev := &Event{
		OrgID: org.ID, Slug: eventSlug, Title: eventSlug, Status: status,
		StartsAt: time.Now().Add(time.Hour), EndsAt: time.Now().Add(2 * time.Hour),
	}
	if err := st.CreateEventWithKey(ctx, ev, &EventKey{PublicKey: pub, PrivateKey: priv}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	return org
}

func listedSlugs(t *testing.T, rows []Event) []string {
	t.Helper()
	out := make([]string, 0, len(rows))
	for _, e := range rows {
		out = append(out, e.Slug)
	}
	return out
}

// nil and empty-but-not-nil are opposite instructions, and the difference is
// the whole safety property of the host display scope: a scope that resolves
// to no organisation must return NOTHING, never the whole box.
func TestListPublishedEvents_OrgFilterNilVersusEmpty(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	a := seedPublishedEvent(t, st, "Alpha", "alpha", "alpha-show", "published")
	seedPublishedEvent(t, st, "Beta", "beta", "beta-show", "published")

	all, err := st.ListPublishedEvents(ctx, "", "", nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("nil filter returned %v, want both events", listedSlugs(t, all))
	}

	none, err := st.ListPublishedEvents(ctx, "", "", []string{}, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("an empty org filter returned %v; a scope restricted to no organisation must return nothing", listedSlugs(t, none))
	}

	one, err := st.ListPublishedEvents(ctx, "", "", []string{a.ID}, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Slug != "alpha-show" {
		t.Fatalf("org filter returned %v, want only alpha-show", listedSlugs(t, one))
	}
}

// The org filter composes with the other filters rather than replacing them.
func TestListPublishedEvents_OrgFilterComposesWithQuery(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	a := seedPublishedEvent(t, st, "Alpha", "alpha", "alpha-show", "published")
	seedPublishedEvent(t, st, "Beta", "beta", "beta-show", "published")

	rows, err := st.ListPublishedEvents(ctx, "beta", "", []string{a.ID}, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %v; the org filter and the search term must BOTH apply", listedSlugs(t, rows))
	}
}

// A draft is not published, so its org has published nothing and must not be
// named on the public listing.
func TestListPublicHostOrgs_OnlyOrgsWithAPublishedEvent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	seedPublishedEvent(t, st, "Onstage", "onstage", "onstage-show", "published")
	seedPublishedEvent(t, st, "Rehearsing", "rehearsing", "rehearsing-show", "draft")
	seedPublishedEvent(t, st, "Called Off", "called-off", "called-off-show", "cancelled")

	orgs, err := st.ListPublicHostOrgs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].Slug != "onstage" {
		got := make([]string, 0, len(orgs))
		for _, o := range orgs {
			got = append(got, o.Slug)
		}
		t.Fatalf("orgs = %v, want only onstage", got)
	}
}

// An org with nothing at all in it is not on the public listing either.
func TestListPublicHostOrgs_EmptyBox(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.CreateOrg(ctx, &Org{Name: "Nothing Yet", Slug: "nothing-yet"}); err != nil {
		t.Fatal(err)
	}
	orgs, err := st.ListPublicHostOrgs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 0 {
		t.Fatalf("orgs = %d, want 0", len(orgs))
	}
}

func TestPlaceholders(t *testing.T) {
	cases := map[int]string{1: "?", 2: "?, ?", 3: "?, ?, ?"}
	for n, want := range cases {
		if got := placeholders(n); got != want {
			t.Errorf("placeholders(%d) = %q, want %q", n, got, want)
		}
	}
}
