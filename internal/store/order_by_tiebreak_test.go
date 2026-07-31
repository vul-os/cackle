package store

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sort"
	"testing"
	"time"
)

// Guards for the ORDER BY tiebreakers in this package.
//
// WHY THEY ARE NEEDED AT ALL. Every timestamp column here is written by
// timeToText (store.go:207), which formats with time.RFC3339 — whole-second
// resolution, and always the fixed-width `Z` form because .UTC() is applied
// first. So the stored text never suffers the trailing-zero lexicographic
// inversion that bit internal/scan; what it suffers instead is TIES. Any two
// rows written in the same second sort equal, and an ORDER BY that names only
// a timestamp has, at that point, specified nothing.
//
// HOW THE TESTS FORCE A TIE, since a test that merely hopes for one proves
// nothing: they write the timestamps themselves, either passing the identical
// time.Time to both rows or passing two times inside one second, which is the
// same thing once through timeToText.
//
// HOW THEY MAKE THE TIEBREAKER THE ONLY DECIDER. A tied query returns rows in
// whatever order the plan happens to produce, which for a temp b-tree sort is
// the order they were scanned, which is rowid order. Insert ids in ascending
// order and rowid order coincides with id order, and the test passes with no
// tiebreaker at all — vacuously. So these tests insert the rows with their ids
// in the WRONG physical order, which is a state a restore, a VACUUM, a
// replicated write or any future insert path can produce, and which SettleOrder
// is one loop-reversal away from producing itself.

// mustOrderWithTickets creates an org, event, ticket type and one paid order,
// then settles it with len(offsets) tickets. Ticket i is given id ids[i] and
// issued_at base+offsets[i]; rows are INSERTED in slice order, so passing
// descending ids puts rowid order and id order deliberately at odds.
func mustOrderWithTickets(t *testing.T, st *Store, holder *string, base time.Time, ids []string, offsets []time.Duration) (orderID string) {
	t.Helper()
	ctx := context.Background()

	ev, _ := mustEventWithKey(t, st)
	tt := &TicketType{EventID: ev.ID, Name: "General", PriceMinor: 1000, QuantityTotal: 100, Status: "on_sale"}
	if err := st.CreateTicketType(ctx, tt); err != nil {
		t.Fatalf("CreateTicketType: %v", err)
	}
	ord := &Order{
		EventID: ev.ID, UserID: holder, BuyerEmail: "buyer@example.test", BuyerName: "Buyer",
		Status: "pending", SubtotalMinor: 1000, TotalMinor: 1000, Currency: "ZAR", Provider: "test",
		CreatedAt: base,
	}
	if _, err := st.CreateOrderWithItems(ctx, ord, []OrderLine{{TicketTypeID: tt.ID, Quantity: len(ids), UnitPriceMinor: 1000}}); err != nil {
		t.Fatalf("CreateOrderWithItems: %v", err)
	}

	tks := make([]Ticket, 0, len(ids))
	for i, id := range ids {
		tks = append(tks, Ticket{
			ID: id, OrderID: ord.ID, EventID: ev.ID, TicketTypeID: tt.ID,
			HolderUserID: holder, HolderName: "Buyer", Serial: id,
			Capability: "cap-" + id, Status: "valid", IssuedAt: base.Add(offsets[i]),
		})
	}
	settled, err := st.SettleOrder(ctx, ord.ID, base, tks)
	if err != nil {
		t.Fatalf("SettleOrder: %v", err)
	}
	if !settled {
		t.Fatal("SettleOrder reported the order was already settled")
	}
	return ord.ID
}

// ticketIDs projects a result set down to the sequence the caller sees.
func ticketIDs(rows []Ticket) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

// assertSameIssuedAt fails unless every named ticket stored byte-identical
// issued_at text — i.e. unless the tie this test depends on actually happened.
func assertSameIssuedAt(t *testing.T, st *Store, ids ...string) {
	t.Helper()
	var want string
	for i, id := range ids {
		var got string
		if err := st.db.QueryRow(`SELECT issued_at FROM tickets WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("read issued_at of %s: %v", id, err)
		}
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("the tie was not forced: issued_at %q vs %q — this test proves nothing unless they are equal", want, got)
		}
	}
}

// TestListTicketsForOrderBreaksIssuedAtTies covers ListTicketsForOrder.
//
// The tie here is not a same-second coincidence, it is STRUCTURAL: SettleOrder
// is handed one paidAt (orders/orders.go:483) and stamps every ticket in the
// order with it (orders/orders.go:513), so EVERY row this query sorts has the
// identical issued_at, at full nanosecond resolution, before RFC3339
// truncation is even reached. `ORDER BY issued_at ASC` on its own therefore
// orders an order's tickets by nothing whatsoever.
//
// MUTATION: drop `, id ASC` from ListTicketsForOrder and this fails.
func TestListTicketsForOrderBreaksIssuedAtTies(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	base := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	ids := []string{"01TICKET000000000000000004", "01TICKET000000000000000003", "01TICKET000000000000000002", "01TICKET000000000000000001"}
	orderID := mustOrderWithTickets(t, st, nil, base, ids, []time.Duration{0, 0, 0, 0})
	assertSameIssuedAt(t, st, ids...)

	rows, err := st.ListTicketsForOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("ListTicketsForOrder: %v", err)
	}
	want := append([]string(nil), ids...)
	sort.Strings(want)
	got := ticketIDs(rows)
	if len(got) != len(want) {
		t.Fatalf("got %d tickets, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListTicketsForOrder returned %v, want %v — an order's ticket list has no defined order", got, want)
		}
	}
}

// TestListTicketsForUserBreaksIssuedAtTies covers ListTicketsForUser, the
// query behind GET /api/tickets. Here the tie is the ordinary same-second one:
// two tickets issued 400ms apart store byte-identical issued_at.
//
// MUTATION: drop `, id DESC` from ListTicketsForUser and this fails.
func TestListTicketsForUserBreaksIssuedAtTies(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	user := mustTiebreakUser(t, st)
	base := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	ids := []string{"01TICKET000000000000000001", "01TICKET000000000000000002", "01TICKET000000000000000003"}
	mustOrderWithTickets(t, st, &user, base, ids, []time.Duration{0, 400 * time.Millisecond, 900 * time.Millisecond})
	assertSameIssuedAt(t, st, ids...)

	rows, err := st.ListTicketsForUser(ctx, user)
	if err != nil {
		t.Fatalf("ListTicketsForUser: %v", err)
	}
	want := []string{ids[2], ids[1], ids[0]} // most recently issued first
	got := ticketIDs(rows)
	if len(got) != len(want) {
		t.Fatalf("got %d tickets, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListTicketsForUser returned %v, want %v — a buyer's ticket list can reorder between refreshes", got, want)
		}
	}
}

// mustTiebreakUser creates a user to hang tickets and orders off.
func mustTiebreakUser(t *testing.T, st *Store) string {
	t.Helper()
	u := &User{Email: "holder-" + NewID() + "@example.test", Name: "Holder", PasswordHash: "not-a-real-hash"}
	if err := st.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u.ID
}

// mustPublishedEventsAt creates one org and a published event per id, every
// one starting at exactly the same instant, INSERTED in the order given. Pass
// descending ids to put rowid order and id order at odds.
func mustPublishedEventsAt(t *testing.T, st *Store, startsAt time.Time, ids []string) (orgID string) {
	t.Helper()
	ctx := context.Background()

	org := &Org{Name: "Tiebreak Ltd", Slug: "tiebreak-" + NewID()}
	if err := st.CreateOrg(ctx, org); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	for _, id := range ids {
		ev := &Event{
			ID: id, OrgID: org.ID, Slug: "ev-" + id, Title: "Event " + id, Status: "published",
			StartsAt: startsAt, EndsAt: startsAt.Add(time.Hour), Currency: "ZAR",
		}
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		if err := st.CreateEventWithKey(ctx, ev, &EventKey{PublicKey: pub, PrivateKey: priv}); err != nil {
			t.Fatalf("CreateEventWithKey(%s): %v", id, err)
		}
	}
	return org.ID
}

func eventIDs(rows []Event) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

// TestListPublishedEventsBreaksStartsAtTies covers ListPublishedEvents, where
// the missing tiebreaker does something worse than reorder a list: combined
// with LIMIT it leaves WHICH ROWS ARE ON THE PAGE unspecified.
//
// starts_at ties are not an edge case for this table the way they are for an
// audit log. It is an organiser-chosen wall-clock time, so it is almost always
// a round one, and every event on a box that starts at 19:00 on the same
// evening sorts equal. This route has a LIMIT and no offset — there is no page
// two — so an event losing a coin toss is not shown later, it is not shown.
//
// A BACKSTOP TO AVOID. Passing an org filter makes SQLite serve this from
// idx_events_org_status_id (migration 0008), whose third column is `id`, so the
// scan feeds the sorter in id order and the answer comes out right WITHOUT any
// tiebreaker — vacuously green. This test therefore uses the unfiltered
// whole-box listing, which is both the default and the case SQLite serves from
// idx_events_status, in rowid order. The plan-dependence test below covers the
// filtered case by showing that accident is only an accident.
//
// MUTATION: drop `, id ASC` from ListPublishedEvents and this fails, returning
// the two events that should have been below the cut.
func TestListPublishedEventsBreaksStartsAtTies(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	startsAt := time.Date(2026, 9, 9, 19, 0, 0, 0, time.UTC)
	ids := []string{"01EVENT0000000000000000004", "01EVENT0000000000000000003", "01EVENT0000000000000000002", "01EVENT0000000000000000001"}
	mustPublishedEventsAt(t, st, startsAt, ids)

	var rowCount, distinct int
	if err := st.db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT starts_at) FROM events WHERE status = 'published'`).Scan(&rowCount, &distinct); err != nil {
		t.Fatalf("count distinct starts_at: %v", err)
	}
	if rowCount != len(ids) || distinct != 1 {
		t.Fatalf("the tie was not forced: %d published events over %d distinct starts_at values, want %d over 1", rowCount, distinct, len(ids))
	}

	rows, err := st.ListPublishedEvents(ctx, "", "", nil, nil, nil, 2)
	if err != nil {
		t.Fatalf("ListPublishedEvents: %v", err)
	}
	want := []string{"01EVENT0000000000000000001", "01EVENT0000000000000000002"}
	got := eventIDs(rows)
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the limited page contained %v, want %v — which events are listed at all is undefined", got, want)
		}
	}
}

// TestListPublishedEventsPageIsNotDecidedByTheQueryPlan is the sharper half.
// The bounded page's COMPOSITION must not change because an index appeared:
// this returns a public listing, and a would-be attendee cannot tell the
// difference between "that event is not on tonight" and "that event lost a
// tie".
//
// MUTATION: drop `, id ASC` from ListPublishedEvents and the two plans return
// different events.
func TestListPublishedEventsPageIsNotDecidedByTheQueryPlan(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	startsAt := time.Date(2026, 9, 9, 19, 0, 0, 0, time.UTC)
	ids := []string{"01EVENT0000000000000000004", "01EVENT0000000000000000003", "01EVENT0000000000000000002", "01EVENT0000000000000000001"}
	orgID := mustPublishedEventsAt(t, st, startsAt, ids)

	viaTempBTree, err := st.ListPublishedEvents(ctx, "", "", []string{orgID}, nil, nil, 2)
	if err != nil {
		t.Fatalf("ListPublishedEvents (temp b-tree plan): %v", err)
	}
	if _, err := st.db.Exec(`CREATE INDEX tmp_events_starts ON events(status, starts_at)`); err != nil {
		t.Fatalf("create probe index: %v", err)
	}
	viaIndexScan, err := st.ListPublishedEvents(ctx, "", "", []string{orgID}, nil, nil, 2)
	if err != nil {
		t.Fatalf("ListPublishedEvents (index-ordered plan): %v", err)
	}

	a, b := eventIDs(viaTempBTree), eventIDs(viaIndexScan)
	if len(a) != len(b) {
		t.Fatalf("page length changed with the query plan: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("the listed events changed with the query plan: %v without an ordering index, %v with one", a, b)
		}
	}
}
