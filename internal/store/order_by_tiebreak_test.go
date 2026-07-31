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
// timeToText (store.go:208), which formats with time.RFC3339 — whole-second
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
//
// AND WATCH FOR AN INDEX DOING THE JOB BY ACCIDENT. Two of these queries came
// out correctly ordered against the BROKEN code, because the index SQLite
// chose already fed the sorter in id order: idx_events_org_status_id from
// migration 0008 for the org-filtered listing, and org_members' primary-key
// autoindex for the member listings. Both are accidents of which index is
// cheapest today. They are covered instead by tests that create a rival
// ordering index mid-test and require the answer not to move —
// TestListPublishedEventsPageIsNotDecidedByTheQueryPlan and
// TestOrgMemberListingIsNotDecidedByTheQueryPlan.

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
// is handed one paidAt (orders/orders.go:482) and stamps every ticket in the
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

// --- the remaining listings ------------------------------------------------

// tiedAt is the one second every row in the batch tests below is written into.
var tiedAt = timeToText(time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC))

// idA/idB/idC sort in that order as text. They are written into each table in
// REVERSE (C, B, A) so that rowid order is the opposite of id order and cannot
// stand in for a real tiebreaker.
const (
	idA = "01ROW00000000000000000000A"
	idB = "01ROW00000000000000000000B"
	idC = "01ROW00000000000000000000C"
)

func ascIDs() []string  { return []string{idA, idB, idC} }
func descIDs() []string { return []string{idC, idB, idA} }

// reversed returns want backwards. Every case seeds in this order, so the
// physical/rowid order of the rows is the exact OPPOSITE of the answer, and no
// listing can come out right by accident of insertion order.
func reversed(want []string) []string {
	out := make([]string, len(want))
	for i, v := range want {
		out[len(want)-1-i] = v
	}
	return out
}

// exec runs one statement and fails the test on error.
func exec(t *testing.T, st *Store, stmt string, args ...any) {
	t.Helper()
	if _, err := st.db.Exec(stmt, args...); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

// tiebreakFixture is one org, one user and one event, all created through the
// real API, for the raw inserts below to hang off.
type tiebreakFixture struct {
	orgID   string
	userID  string
	eventID string
}

func newTiebreakFixture(t *testing.T, st *Store) tiebreakFixture {
	t.Helper()
	ev, _ := mustEventWithKey(t, st)
	return tiebreakFixture{orgID: ev.OrgID, userID: mustTiebreakUser(t, st), eventID: ev.ID}
}

// assertSequence fails unless got is exactly want.
func assertSequence(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s returned %d rows %v, want %d %v", what, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s returned %v, want %v — same-second rows come back in an undefined order", what, got, want)
		}
	}
}

// TestRemainingListingsBreakTimestampTies is the batch guard for every other
// timestamp-ordered listing in this package. Each case writes three rows into
// one stored second — so the ORDER BY's timestamp column is identical on all
// three — with the ids inserted in reverse, and then requires the listing to
// come back in id order.
//
// MUTATION: remove the trailing id/user_id column from any one of the ORDER BY
// clauses named in the subtests and that subtest fails.
func TestRemainingListingsBreakTimestampTies(t *testing.T) {
	cases := []struct {
		name string // <file>:<method>
		seed func(t *testing.T, st *Store, f tiebreakFixture, ids []string)
		list func(t *testing.T, st *Store, f tiebreakFixture) []string
		want []string
	}{
		{
			name: "orders.go ListOrdersForUser",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				for _, id := range ids {
					exec(t, st, `INSERT INTO orders (id, event_id, user_id, buyer_email, buyer_name, status,
						subtotal_minor, fee_minor, total_minor, currency, provider, provider_ref, created_at, paid_at)
						VALUES (?, ?, ?, 'b@example.test', 'B', 'pending', 0, 0, 0, 'ZAR', 'test', NULL, ?, NULL)`,
						id, f.eventID, f.userID, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListOrdersForUser(context.Background(), f.userID)
				if err != nil {
					t.Fatalf("ListOrdersForUser: %v", err)
				}
				out := make([]string, len(rows))
				for i, r := range rows {
					out[i] = r.ID
				}
				return out
			},
			want: descIDs(),
		},
		{
			name: "orders.go ListOrdersForEvent",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				for _, id := range ids {
					exec(t, st, `INSERT INTO orders (id, event_id, user_id, buyer_email, buyer_name, status,
						subtotal_minor, fee_minor, total_minor, currency, provider, provider_ref, created_at, paid_at)
						VALUES (?, ?, NULL, 'b@example.test', 'B', 'pending', 0, 0, 0, 'ZAR', 'test', NULL, ?, NULL)`,
						id, f.eventID, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListOrdersForEvent(context.Background(), f.eventID)
				if err != nil {
					t.Fatalf("ListOrdersForEvent: %v", err)
				}
				out := make([]string, len(rows))
				for i, r := range rows {
					out[i] = r.ID
				}
				return out
			},
			want: descIDs(),
		},
		{
			name: "events.go ListEventsByOrg",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				exec(t, st, `DELETE FROM events WHERE id = ?`, f.eventID)
				for _, id := range ids {
					exec(t, st, `INSERT INTO events (id, org_id, slug, title, summary, description, venue_name, address,
						lat, lng, starts_at, ends_at, timezone, cover_image, status, currency, category, cover_image_id, created_at, updated_at)
						VALUES (?, ?, ?, 'T', '', '', '', '', NULL, NULL, ?, ?, '', '', 'draft', 'ZAR', '', NULL, ?, ?)`,
						id, f.orgID, "slug-"+id, tiedAt, tiedAt, tiedAt, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListEventsByOrg(context.Background(), f.orgID)
				if err != nil {
					t.Fatalf("ListEventsByOrg: %v", err)
				}
				return eventIDs(rows)
			},
			want: descIDs(),
		},
		{
			name: "images.go ListImagesByEvent",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				for _, id := range ids {
					exec(t, st, `INSERT INTO images (id, event_id, format, width, height, size_bytes, uploaded_by, created_at)
						VALUES (?, ?, 'png', 1, 1, 1, NULL, ?)`, id, f.eventID, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListImagesByEvent(context.Background(), f.eventID)
				if err != nil {
					t.Fatalf("ListImagesByEvent: %v", err)
				}
				out := make([]string, len(rows))
				for i, r := range rows {
					out[i] = r.ID
				}
				return out
			},
			want: ascIDs(),
		},
		{
			name: "org_invites.go ListPendingOrgInvites",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				for _, id := range ids {
					exec(t, st, `INSERT INTO org_invites (id, org_id, email, role, token_hash, invited_by, expires_at, accepted_at, created_at)
						VALUES (?, ?, 'i@example.test', 'scanner', ?, NULL, ?, NULL, ?)`,
						id, f.orgID, "hash-"+id, tiedAt, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListPendingOrgInvites(context.Background(), f.orgID)
				if err != nil {
					t.Fatalf("ListPendingOrgInvites: %v", err)
				}
				out := make([]string, len(rows))
				for i, r := range rows {
					out[i] = r.ID
				}
				return out
			},
			want: descIDs(),
		},
		{
			name: "payouts.go ListPayoutsForEvent",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				for _, id := range ids {
					exec(t, st, `INSERT INTO payouts (id, event_id, org_id, amount_minor, currency, status, provider_ref, created_at, paid_at)
						VALUES (?, ?, ?, 100, 'ZAR', 'pending', NULL, ?, NULL)`, id, f.eventID, f.orgID, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListPayoutsForEvent(context.Background(), f.eventID)
				if err != nil {
					t.Fatalf("ListPayoutsForEvent: %v", err)
				}
				out := make([]string, len(rows))
				for i, r := range rows {
					out[i] = r.ID
				}
				return out
			},
			want: descIDs(),
		},
		{
			name: "compensating_payments.go ListCompensatingPaymentAudits",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				for _, id := range ids {
					exec(t, st, `INSERT INTO compensating_payment_audits
						(id, original_reference, rail_id, destination, status, amount_minor, currency, created_at)
						VALUES (?, 'ref-1', 'rail', 'dest', 'pending', 100, 'ZAR', ?)`, id, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListCompensatingPaymentAudits(context.Background(), "ref-1")
				if err != nil {
					t.Fatalf("ListCompensatingPaymentAudits: %v", err)
				}
				out := make([]string, len(rows))
				for i, r := range rows {
					out[i] = r.ID
				}
				return out
			},
			want: descIDs(),
		},
		{
			name: "event_keys.go ListEventKeys and ActiveEventKeys",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				exec(t, st, `DELETE FROM event_keys WHERE event_id = ?`, f.eventID)
				for _, id := range ids {
					exec(t, st, `INSERT INTO event_keys (id, event_id, public_key, sealed_private_key, sealed_nonce, created_at, revoked_at)
						VALUES (?, ?, ?, ?, ?, ?, NULL)`, id, f.eventID, []byte("pub"), []byte("sealed"), []byte("nonce"), tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				ctx := context.Background()
				all, err := st.ListEventKeys(ctx, f.eventID)
				if err != nil {
					t.Fatalf("ListEventKeys: %v", err)
				}
				active, err := st.ActiveEventKeys(ctx, f.eventID)
				if err != nil {
					t.Fatalf("ActiveEventKeys: %v", err)
				}
				out := make([]string, 0, len(all))
				for i, k := range all {
					if active[i].ID != k.ID {
						t.Fatalf("ActiveEventKeys disagrees with ListEventKeys at %d: %q vs %q", i, active[i].ID, k.ID)
					}
					out = append(out, k.ID)
				}
				return out
			},
			want: ascIDs(),
		},
		{
			name: "orgs.go ListOrgsForUser",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				exec(t, st, `DELETE FROM org_members WHERE user_id = ?`, f.userID)
				for _, id := range ids {
					exec(t, st, `INSERT INTO orgs (id, name, slug, created_at, default_currency) VALUES (?, ?, ?, ?, 'ZAR')`,
						id, "Org "+id, "org-"+id, tiedAt)
					exec(t, st, `INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?, ?, 'admin', ?)`,
						id, f.userID, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				rows, err := st.ListOrgsForUser(context.Background(), f.userID)
				if err != nil {
					t.Fatalf("ListOrgsForUser: %v", err)
				}
				out := make([]string, len(rows))
				for i, r := range rows {
					out[i] = r.ID
				}
				return out
			},
			want: ascIDs(),
		},
		{
			name: "orgs.go ListOrgMembersWithUser and ListOrgMembers",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				exec(t, st, `DELETE FROM org_members WHERE org_id = ?`, f.orgID)
				for _, id := range ids {
					exec(t, st, `INSERT INTO users (id, email, password_hash, name, created_at, email_verified_at)
						VALUES (?, ?, 'h', 'U', ?, NULL)`, id, "u-"+id+"@example.test", tiedAt)
					exec(t, st, `INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?, ?, 'scanner', ?)`,
						f.orgID, id, tiedAt)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				ctx := context.Background()
				withUser, err := st.ListOrgMembersWithUser(ctx, f.orgID)
				if err != nil {
					t.Fatalf("ListOrgMembersWithUser: %v", err)
				}
				plain, err := st.ListOrgMembers(ctx, f.orgID)
				if err != nil {
					t.Fatalf("ListOrgMembers: %v", err)
				}
				out := make([]string, 0, len(withUser))
				for i, m := range withUser {
					if plain[i].UserID != m.UserID {
						t.Fatalf("ListOrgMembers disagrees with ListOrgMembersWithUser at %d: %q vs %q", i, plain[i].UserID, m.UserID)
					}
					out = append(out, m.UserID)
				}
				return out
			},
			want: ascIDs(),
		},
		{
			name: "ticket_types.go ListTicketTypesForEvent and TicketTypeStatsForEvent",
			seed: func(t *testing.T, st *Store, f tiebreakFixture, ids []string) {
				// sort_order defaults to 0 and name is not unique, so two
				// identically-named types at the default sort order tie on BOTH
				// existing sort columns. This one is not a timestamp tie at all,
				// which is why the survey missed it.
				for _, id := range ids {
					exec(t, st, `INSERT INTO ticket_types (id, event_id, name, description, price_minor,
						quantity_total, quantity_sold, sales_start, sales_end, max_per_order, status, sort_order)
						VALUES (?, ?, 'General', '', 100, 10, 0, NULL, NULL, 0, 'on_sale', 0)`, id, f.eventID)
				}
			},
			list: func(t *testing.T, st *Store, f tiebreakFixture) []string {
				ctx := context.Background()
				types, err := st.ListTicketTypesForEvent(ctx, f.eventID)
				if err != nil {
					t.Fatalf("ListTicketTypesForEvent: %v", err)
				}
				stats, err := st.TicketTypeStatsForEvent(ctx, f.eventID)
				if err != nil {
					t.Fatalf("TicketTypeStatsForEvent: %v", err)
				}
				out := make([]string, 0, len(types))
				for i, tt := range types {
					if stats[i].TicketTypeID != tt.ID {
						t.Fatalf("TicketTypeStatsForEvent disagrees with ListTicketTypesForEvent at %d: %q vs %q", i, stats[i].TicketTypeID, tt.ID)
					}
					out = append(out, tt.ID)
				}
				return out
			},
			want: ascIDs(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			f := newTiebreakFixture(t, st)
			tc.seed(t, st, f, reversed(tc.want))
			assertSequence(t, tc.name, tc.list(t, st, f), tc.want)
		})
	}
}

// TestOrgMemberListingIsNotDecidedByTheQueryPlan is the org_members case,
// which needs its own test because a BACKSTOP hides it in the batch above.
// org_members' PRIMARY KEY is (org_id, user_id), and SQLite serves the listing
// from that implicit index, so the tied rows reach the sorter already in
// user_id order and come out right with no tiebreaker at all. That is an
// accident of which index is cheapest, not a guarantee: give SQLite an index
// that satisfies the ORDER BY directly and the same query returns the members
// in the opposite order.
//
// MUTATION: drop `, user_id` from ListOrgMembers / `, m.user_id ASC` from
// ListOrgMembersWithUser and the two plans disagree.
func TestOrgMemberListingIsNotDecidedByTheQueryPlan(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	f := newTiebreakFixture(t, st)

	exec(t, st, `DELETE FROM org_members WHERE org_id = ?`, f.orgID)
	for _, id := range reversed(ascIDs()) {
		exec(t, st, `INSERT INTO users (id, email, password_hash, name, created_at, email_verified_at)
			VALUES (?, ?, 'h', 'U', ?, NULL)`, id, "u-"+id+"@example.test", tiedAt)
		exec(t, st, `INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?, ?, 'scanner', ?)`,
			f.orgID, id, tiedAt)
	}

	memberIDs := func(label string) []string {
		t.Helper()
		rows, err := st.ListOrgMembers(ctx, f.orgID)
		if err != nil {
			t.Fatalf("ListOrgMembers (%s): %v", label, err)
		}
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.UserID
		}
		return out
	}

	viaAutoindex := memberIDs("primary-key autoindex")
	exec(t, st, `CREATE INDEX tmp_org_members_created ON org_members(org_id, created_at)`)
	viaOrderingIndex := memberIDs("ordering index")

	assertSequence(t, "ListOrgMembers under the primary-key autoindex", viaAutoindex, ascIDs())
	assertSequence(t, "ListOrgMembers under an ordering index", viaOrderingIndex, ascIDs())
}

// TestAdmissionClaimsBreakScannedAtTies covers a query the survey did not list:
// ListAdmissionClaimsForEvent, which orders by (ticket_id, scanned_at,
// device_id). Three columns look like plenty until you notice nothing stops one
// device recording two scans of one ticket inside one second — the admissions
// insert path is a plain INSERT with no dedupe on that triple, and the only
// unique index on the table is one 'admitted' row per ticket. Both rows then
// tie on all three sort columns, and this is the organiser's after-the-fact
// record of a contested admission, where the order the door happened is the
// entire point of the view.
//
// MUTATION: drop `, id` from ListAdmissionClaimsForEvent's ORDER BY and this
// fails, reporting the duplicate before the admission.
func TestAdmissionClaimsBreakScannedAtTies(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	f := newTiebreakFixture(t, st)

	const ticketID = "01TICKETCONTESTED000000000"
	exec(t, st, `INSERT INTO ticket_types (id, event_id, name, description, price_minor,
		quantity_total, quantity_sold, sales_start, sales_end, max_per_order, status, sort_order)
		VALUES ('tt-1', ?, 'General', '', 100, 10, 0, NULL, NULL, 0, 'on_sale', 0)`, f.eventID)
	exec(t, st, `INSERT INTO orders (id, event_id, user_id, buyer_email, buyer_name, status,
		subtotal_minor, fee_minor, total_minor, currency, provider, provider_ref, created_at, paid_at)
		VALUES ('ord-1', ?, NULL, 'b@example.test', 'B', 'paid', 100, 0, 100, 'ZAR', 'test', NULL, ?, ?)`,
		f.eventID, tiedAt, tiedAt)
	exec(t, st, `INSERT INTO tickets (id, order_id, event_id, ticket_type_id, holder_user_id, holder_name,
		serial, capability, status, issued_at, voided_at)
		VALUES (?, 'ord-1', ?, 'tt-1', NULL, 'B', ?, 'cap', 'valid', ?, NULL)`,
		ticketID, f.eventID, ticketID, tiedAt)

	// Two claims from ONE device in one second — identical on every existing
	// sort column — inserted with the later one first. A third device makes the
	// ticket contested, which is what this query selects on.
	admission := func(id, deviceID, result, reported, note string) {
		exec(t, st, `INSERT INTO admissions (id, ticket_id, event_id, gate_id, scanned_by, device_id,
			scanned_at, result, reported_result, note)
			VALUES (?, ?, ?, 'gate-1', NULL, ?, ?, ?, ?, ?)`,
			id, ticketID, f.eventID, deviceID, tiedAt, result, reported, note)
	}
	admission("01ADMISSION0000000000000B", "dev-1", "duplicate", "admitted", "second")
	admission("01ADMISSION0000000000000A", "dev-1", "admitted", "admitted", "first")
	admission("01ADMISSION0000000000000C", "dev-2", "duplicate", "admitted", "other gate")

	claims, err := st.ListAdmissionClaimsForEvent(ctx, f.eventID)
	if err != nil {
		t.Fatalf("ListAdmissionClaimsForEvent: %v", err)
	}
	got := make([]string, len(claims))
	for i, c := range claims {
		got[i] = c.Note
	}
	assertSequence(t, "ListAdmissionClaimsForEvent", got, []string{"first", "second", "other gate"})
}
