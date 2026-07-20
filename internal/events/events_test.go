package events

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/store"
	"github.com/vul-os/cackle/internal/tickets"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "cackle.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mustOrg(t *testing.T, st *store.Store) *store.Org {
	t.Helper()
	o := &store.Org{Name: "Test Promoters", Slug: "test-promoters-" + store.NewID()}
	if err := st.CreateOrg(context.Background(), o); err != nil {
		t.Fatalf("create org: %v", err)
	}
	return o
}

func validCreateInput(slug string) CreateEventInput {
	starts := time.Now().Add(24 * time.Hour)
	return CreateEventInput{
		Slug:      slug,
		Title:     "Test Festival",
		VenueName: "Test Grounds",
		StartsAt:  starts,
		EndsAt:    starts.Add(6 * time.Hour),
	}
}

func TestCreateGeneratesOwnIssuerKey(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	ev, err := svc.Create(ctx, org.ID, validCreateInput("festival-1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ev.Status != "draft" {
		t.Fatalf("status = %q, want draft", ev.Status)
	}
	// Currency has no privileged hardcoded default (e.g. "ZAR") — an event
	// with no explicit currency inherits its owning org's default_currency
	// (mustOrg leaves this unset, so store.CreateOrg's own USD fallback
	// applies here; TestCreate_CurrencyDefaultsFromOrgNotHardcoded below
	// proves this with a non-USD org).
	if ev.Timezone != "UTC" || ev.Currency != org.DefaultCurrency {
		t.Fatalf("unexpected defaults: tz=%q currency=%q, want currency=%q (org default)", ev.Timezone, ev.Currency, org.DefaultCurrency)
	}

	ring, err := svc.IssuerPublicKeys(ctx, ev.ID)
	if err != nil {
		t.Fatalf("IssuerPublicKeys: %v", err)
	}
	if len(ring.Keys) != 1 {
		t.Fatalf("expected exactly 1 issuer key, got %d", len(ring.Keys))
	}

	// A second event must get a DIFFERENT key — never a shared/global one.
	ev2, err := svc.Create(ctx, org.ID, validCreateInput("festival-2"))
	if err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	ring2, err := svc.IssuerPublicKeys(ctx, ev2.ID)
	if err != nil {
		t.Fatalf("IssuerPublicKeys #2: %v", err)
	}
	for kid := range ring.Keys {
		if _, dup := ring2.Keys[kid]; dup {
			t.Fatalf("event 2 shares issuer key %q with event 1 — per-event key isolation broken", kid)
		}
	}

	// The signing key actually works end to end: sign with it, verify
	// with the public key from IssuerPublicKeys.
	kid, priv, err := svc.signingKey(ctx, ev.ID)
	if err != nil {
		t.Fatalf("signingKey: %v", err)
	}
	tok, err := tickets.Issue(tickets.Payload{TID: "t1", EID: ev.ID, TT: "tt1", KID: kid}, priv)
	if err != nil {
		t.Fatalf("tickets.Issue: %v", err)
	}
	if _, err := tickets.VerifyWithRing(tok, ring, time.Now()); err != nil {
		t.Fatalf("VerifyWithRing: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	if _, err := svc.Create(ctx, org.ID, CreateEventInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty input: got %v, want ErrInvalidInput", err)
	}

	bad := validCreateInput("bad-times")
	bad.EndsAt = bad.StartsAt.Add(-time.Hour)
	if _, err := svc.Create(ctx, org.ID, bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ends before starts: got %v, want ErrInvalidInput", err)
	}

	if _, err := svc.Create(ctx, "nonexistent-org", validCreateInput("orphan")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("nonexistent org: got %v, want ErrNotFound", err)
	}
}

// TestCreate_CurrencyDefaultsFromOrgNotHardcoded proves Cackle has no
// privileged default currency: an event with no explicit currency
// inherits whatever ISO-4217 code its OWN org picked — including
// zero-decimal (JPY) and three-decimal (KWD) currencies — and an event
// can still override that default explicitly. An invalid/unknown code,
// on either the org or the event, is rejected rather than silently
// coerced to some fallback.
func TestCreate_CurrencyDefaultsFromOrgNotHardcoded(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	svc := New(st)

	jpyOrg := &store.Org{Name: "Tokyo Promoters", Slug: "tokyo-promoters-" + store.NewID(), DefaultCurrency: "jpy"}
	if err := st.CreateOrg(ctx, jpyOrg); err != nil {
		t.Fatalf("create JPY org: %v", err)
	}
	if jpyOrg.DefaultCurrency != "JPY" {
		t.Fatalf("DefaultCurrency = %q, want normalized to JPY", jpyOrg.DefaultCurrency)
	}

	// No explicit event currency: inherits the org's own default (JPY,
	// zero decimal places) — never a hardcoded ZAR/USD/anything.
	in := validCreateInput("tokyo-fest")
	ev, err := svc.Create(ctx, jpyOrg.ID, in)
	if err != nil {
		t.Fatalf("Create (inherit org default): %v", err)
	}
	if ev.Currency != "JPY" {
		t.Fatalf("Currency = %q, want JPY (inherited from org default_currency)", ev.Currency)
	}

	// Explicit override to a three-decimal currency works too.
	kwdIn := validCreateInput("kuwait-fest")
	kwdIn.Currency = "kwd"
	ev2, err := svc.Create(ctx, jpyOrg.ID, kwdIn)
	if err != nil {
		t.Fatalf("Create (explicit override): %v", err)
	}
	if ev2.Currency != "KWD" {
		t.Fatalf("Currency = %q, want KWD (explicit override, normalized)", ev2.Currency)
	}

	// An unknown currency code is rejected outright, not coerced.
	badIn := validCreateInput("bad-currency")
	badIn.Currency = "XYZ"
	if _, err := svc.Create(ctx, jpyOrg.ID, badIn); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown currency: got %v, want ErrInvalidInput", err)
	}

	// Update can change an event's currency, validated the same way.
	updated, err := svc.Update(ctx, ev.ID, UpdateEventInput{Currency: strPtr("usd")})
	if err != nil {
		t.Fatalf("Update currency: %v", err)
	}
	if updated.Currency != "USD" {
		t.Fatalf("Currency after update = %q, want USD", updated.Currency)
	}
	if _, err := svc.Update(ctx, ev.ID, UpdateEventInput{Currency: strPtr("not-a-currency")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update with invalid currency: got %v, want ErrInvalidInput", err)
	}
}

func strPtr(s string) *string { return &s }

func TestPublishTransitions(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	ev, err := svc.Create(ctx, org.ID, validCreateInput("publish-me"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ev.Status != "draft" {
		t.Fatalf("new event status = %q, want draft", ev.Status)
	}

	published, err := svc.Publish(ctx, ev.ID)
	if err != nil {
		t.Fatalf("Publish (draft->published): %v", err)
	}
	if published.Status != "published" {
		t.Fatalf("status after publish = %q, want published", published.Status)
	}

	// Publishing an already-published event is idempotent.
	again, err := svc.Publish(ctx, ev.ID)
	if err != nil {
		t.Fatalf("Publish (already published): %v", err)
	}
	if again.Status != "published" {
		t.Fatalf("status after re-publish = %q, want published", again.Status)
	}

	// Cancel it via Update, then Publish must reject the transition.
	cancelled := "cancelled"
	c, err := svc.Update(ctx, ev.ID, UpdateEventInput{Status: &cancelled})
	if err != nil {
		t.Fatalf("Update to cancelled: %v", err)
	}
	if c.Status != "cancelled" {
		t.Fatalf("status after cancel = %q, want cancelled", c.Status)
	}
	if _, err := svc.Publish(ctx, ev.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Publish cancelled event: got %v, want ErrInvalidTransition", err)
	}

	// Update cannot be used to jump straight to "published" — must go
	// through Publish.
	ev2, err := svc.Create(ctx, org.ID, validCreateInput("publish-me-2"))
	if err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	published2 := "published"
	if _, err := svc.Update(ctx, ev2.ID, UpdateEventInput{Status: &published2}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Update to published: got %v, want ErrInvalidTransition", err)
	}
}

func TestUpdateEventPartial(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	ev, err := svc.Create(ctx, org.ID, validCreateInput("updatable"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newTitle := "Renamed Festival"
	updated, err := svc.Update(ctx, ev.ID, UpdateEventInput{Title: &newTitle})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != newTitle {
		t.Fatalf("title = %q, want %q", updated.Title, newTitle)
	}
	if updated.Slug != ev.Slug {
		t.Fatalf("slug changed unexpectedly: %q -> %q", ev.Slug, updated.Slug)
	}
	if !updated.UpdatedAt.After(ev.UpdatedAt) && !updated.UpdatedAt.Equal(ev.UpdatedAt) {
		t.Fatalf("updated_at did not advance")
	}
}

func TestListPublicOnlyReturnsPublished(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	draft, err := svc.Create(ctx, org.ID, validCreateInput("draft-event"))
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	pub, err := svc.Create(ctx, org.ID, validCreateInput("published-event"))
	if err != nil {
		t.Fatalf("create pub: %v", err)
	}
	if _, err := svc.Publish(ctx, pub.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	list, err := svc.ListPublic(ctx, PublicFilter{})
	if err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if len(list) != 1 || list[0].ID != pub.ID {
		t.Fatalf("ListPublic = %+v, want exactly the published event", list)
	}
	for _, e := range list {
		if e.ID == draft.ID {
			t.Fatal("ListPublic leaked a draft event")
		}
	}
}

func TestTicketTypeLifecycleAndInvariants(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	ev, err := svc.Create(ctx, org.ID, validCreateInput("tt-event"))
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	tt, err := svc.CreateTicketType(ctx, ev.ID, TicketTypeInput{
		Name:          "General",
		PriceMinor:    5000,
		QuantityTotal: 10,
		MaxPerOrder:   4,
	})
	if err != nil {
		t.Fatalf("CreateTicketType: %v", err)
	}
	if tt.QuantitySold != 0 {
		t.Fatalf("new ticket type quantity_sold = %d, want 0", tt.QuantitySold)
	}
	if tt.Status != "active" {
		t.Fatalf("default status = %q, want active", tt.Status)
	}

	list, err := svc.ListTicketTypes(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ListTicketTypes: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListTicketTypes = %d entries, want 1", len(list))
	}

	updated, err := svc.UpdateTicketType(ctx, tt.ID, TicketTypeInput{
		Name:          "General Admission",
		PriceMinor:    6000,
		QuantityTotal: 20,
		MaxPerOrder:   4,
	})
	if err != nil {
		t.Fatalf("UpdateTicketType: %v", err)
	}
	if updated.PriceMinor != 6000 || updated.QuantityTotal != 20 {
		t.Fatalf("update did not apply: %+v", updated)
	}

	// Reserve some inventory directly at the store level to simulate a
	// sale, then verify UpdateTicketType refuses to shrink below it.
	if _, err := st.CreateOrderWithItems(ctx, &store.Order{
		EventID: ev.ID, BuyerEmail: "x@example.com", Status: "pending", Currency: "ZAR",
	}, []store.OrderLine{{TicketTypeID: tt.ID, Quantity: 5, UnitPriceMinor: 6000}}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if _, err := svc.UpdateTicketType(ctx, tt.ID, TicketTypeInput{
		Name: "General Admission", PriceMinor: 6000, QuantityTotal: 3, MaxPerOrder: 4,
	}); !errors.Is(err, ErrQuantityBelowSold) {
		t.Fatalf("shrink below sold: got %v, want ErrQuantityBelowSold", err)
	}

	// DeleteTicketType must refuse now that inventory is reserved.
	if err := svc.DeleteTicketType(ctx, tt.ID); !errors.Is(err, ErrTicketTypeHasSales) {
		t.Fatalf("delete with sales: got %v, want ErrTicketTypeHasSales", err)
	}

	// A fresh, untouched ticket type can be deleted.
	tt2, err := svc.CreateTicketType(ctx, ev.ID, TicketTypeInput{Name: "VIP", PriceMinor: 10000, QuantityTotal: 5})
	if err != nil {
		t.Fatalf("CreateTicketType #2: %v", err)
	}
	if err := svc.DeleteTicketType(ctx, tt2.ID); err != nil {
		t.Fatalf("DeleteTicketType (unsold): %v", err)
	}
}

func TestStatsCorrectness(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	ev, err := svc.Create(ctx, org.ID, validCreateInput("stats-event"))
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}
	tt, err := svc.CreateTicketType(ctx, ev.ID, TicketTypeInput{Name: "General", PriceMinor: 1000, QuantityTotal: 100})
	if err != nil {
		t.Fatalf("CreateTicketType: %v", err)
	}

	// A pending (unpaid) order reserves inventory but must NOT count
	// toward Sold/RevenueMinor.
	pendingOrd := &store.Order{
		EventID: ev.ID, BuyerEmail: "pending@example.com", Status: "pending", Currency: "ZAR", TotalMinor: 3000,
	}
	if _, err := st.CreateOrderWithItems(ctx, pendingOrd, []store.OrderLine{{TicketTypeID: tt.ID, Quantity: 3, UnitPriceMinor: 1000}}); err != nil {
		t.Fatalf("create pending order: %v", err)
	}

	stats, err := svc.Stats(ctx, ev.ID)
	if err != nil {
		t.Fatalf("Stats (pending only): %v", err)
	}
	if stats.Sold != 0 || stats.RevenueMinor != 0 {
		t.Fatalf("pending order counted in stats: %+v", stats)
	}

	// A paid order (created directly via store + marked paid, with a real
	// signed ticket issued for it) DOES count.
	ord := &store.Order{EventID: ev.ID, BuyerEmail: "paid@example.com", Status: "pending", Currency: "ZAR", TotalMinor: 2000}
	if _, err := st.CreateOrderWithItems(ctx, ord, []store.OrderLine{{TicketTypeID: tt.ID, Quantity: 2, UnitPriceMinor: 1000}}); err != nil {
		t.Fatalf("create paid-to-be order: %v", err)
	}
	tid1 := store.NewID()
	tid2 := store.NewID()
	cap1, _, err := svc.IssueTicket(ctx, ev.ID, tickets.Payload{TID: tid1, EID: ev.ID, TT: tt.ID})
	if err != nil {
		t.Fatalf("IssueTicket 1: %v", err)
	}
	cap2, _, err := svc.IssueTicket(ctx, ev.ID, tickets.Payload{TID: tid2, EID: ev.ID, TT: tt.ID})
	if err != nil {
		t.Fatalf("IssueTicket 2: %v", err)
	}
	now := time.Now()
	settled, err := st.SettleOrder(ctx, ord.ID, now, []store.Ticket{
		{ID: tid1, OrderID: ord.ID, EventID: ev.ID, TicketTypeID: tt.ID, Serial: tid1, Capability: cap1, IssuedAt: now},
		{ID: tid2, OrderID: ord.ID, EventID: ev.ID, TicketTypeID: tt.ID, Serial: tid2, Capability: cap2, IssuedAt: now},
	})
	if err != nil || !settled {
		t.Fatalf("SettleOrder: settled=%v err=%v", settled, err)
	}

	// Record one admission against the real ticket that was just issued
	// (admissions.ticket_id has a foreign key into tickets).
	if err := admit(ctx, st, ev.ID, tid1); err != nil {
		t.Fatalf("seed admission: %v", err)
	}

	stats, err = svc.Stats(ctx, ev.ID)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Sold != 2 {
		t.Fatalf("Sold = %d, want 2", stats.Sold)
	}
	if stats.RevenueMinor != 2000 {
		t.Fatalf("RevenueMinor = %d, want 2000", stats.RevenueMinor)
	}
	if stats.Admitted != 1 {
		t.Fatalf("Admitted = %d, want 1", stats.Admitted)
	}
	if len(stats.ByType) != 1 || stats.ByType[0].Sold != 2 || stats.ByType[0].RevenueMinor != 2000 {
		t.Fatalf("ByType = %+v", stats.ByType)
	}
}

func TestListByOrgIncludesEveryStatus(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	draft, err := svc.Create(ctx, org.ID, validCreateInput("org-draft"))
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	pub, err := svc.Create(ctx, org.ID, validCreateInput("org-published"))
	if err != nil {
		t.Fatalf("create pub: %v", err)
	}
	if _, err := svc.Publish(ctx, pub.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// A second org's events must never leak into the first org's listing.
	otherOrg := mustOrg(t, st)
	if _, err := svc.Create(ctx, otherOrg.ID, validCreateInput("other-org-draft")); err != nil {
		t.Fatalf("create other org event: %v", err)
	}

	list, err := svc.ListByOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListByOrg: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByOrg = %d events, want 2 (draft + published)", len(list))
	}
	var sawDraft, sawPublished bool
	for _, e := range list {
		if e.OrgID != org.ID {
			t.Fatalf("ListByOrg leaked an event from another org: %+v", e)
		}
		if e.ID == draft.ID {
			sawDraft = true
		}
		if e.ID == pub.ID {
			sawPublished = true
		}
	}
	if !sawDraft {
		t.Fatal("ListByOrg did not include the draft event")
	}
	if !sawPublished {
		t.Fatal("ListByOrg did not include the published event")
	}
}

func TestDeleteRefusesEventWithIssuedTickets(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	org := mustOrg(t, st)
	svc := New(st)

	ev, err := svc.Create(ctx, org.ID, validCreateInput("delete-event"))
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}
	tt, err := svc.CreateTicketType(ctx, ev.ID, TicketTypeInput{Name: "General", PriceMinor: 1000, QuantityTotal: 10})
	if err != nil {
		t.Fatalf("CreateTicketType: %v", err)
	}

	// A pending (never-paid) order has NO issued tickets yet — deleting the
	// event must still succeed and take the order/order_items with it.
	pendingOrd := &store.Order{EventID: ev.ID, BuyerEmail: "pending@example.com", Status: "pending", Currency: "ZAR"}
	if _, err := st.CreateOrderWithItems(ctx, pendingOrd, []store.OrderLine{{TicketTypeID: tt.ID, Quantity: 1, UnitPriceMinor: 1000}}); err != nil {
		t.Fatalf("seed pending order: %v", err)
	}

	ev2, err := svc.Create(ctx, org.ID, validCreateInput("delete-event-2"))
	if err != nil {
		t.Fatalf("Create event 2: %v", err)
	}

	// ev2 has a genuinely settled order with an issued ticket — must refuse.
	tt2, err := svc.CreateTicketType(ctx, ev2.ID, TicketTypeInput{Name: "General", PriceMinor: 1000, QuantityTotal: 10})
	if err != nil {
		t.Fatalf("CreateTicketType 2: %v", err)
	}
	ord := &store.Order{EventID: ev2.ID, BuyerEmail: "buyer@example.com", Status: "pending", Currency: "ZAR", TotalMinor: 1000}
	if _, err := st.CreateOrderWithItems(ctx, ord, []store.OrderLine{{TicketTypeID: tt2.ID, Quantity: 1, UnitPriceMinor: 1000}}); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	tid := store.NewID()
	capToken, _, err := svc.IssueTicket(ctx, ev2.ID, tickets.Payload{TID: tid, EID: ev2.ID, TT: tt2.ID})
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	if settled, err := st.SettleOrder(ctx, ord.ID, time.Now(), []store.Ticket{
		{ID: tid, OrderID: ord.ID, EventID: ev2.ID, TicketTypeID: tt2.ID, Serial: tid, Capability: capToken, IssuedAt: time.Now()},
	}); err != nil || !settled {
		t.Fatalf("SettleOrder: settled=%v err=%v", settled, err)
	}

	if err := svc.Delete(ctx, ev2.ID); !errors.Is(err, ErrEventHasTickets) {
		t.Fatalf("delete event with issued ticket: got %v, want ErrEventHasTickets", err)
	}
	// Nothing was removed.
	if _, err := svc.Get(ctx, ev2.ID); err != nil {
		t.Fatalf("event with issued ticket should still exist: %v", err)
	}

	// ev (only ever had a pending, never-paid order) CAN be deleted.
	if err := svc.Delete(ctx, ev.ID); err != nil {
		t.Fatalf("Delete (no issued tickets): %v", err)
	}
	if _, err := svc.Get(ctx, ev.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted event should be gone: got %v", err)
	}

	// Deleting a nonexistent event is ErrNotFound, not a generic error.
	if err := svc.Delete(ctx, "nonexistent-id"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete nonexistent event: got %v, want ErrNotFound", err)
	}
}

// admit inserts a raw admissions row directly, since internal/scan (which
// owns writing that table in production) is out of this package's scope.
func admit(ctx context.Context, st *store.Store, eventID, ticketID string) error {
	_, err := st.DB().ExecContext(ctx, `
		INSERT INTO admissions (id, ticket_id, event_id, gate_id, device_id, scanned_at, result)
		VALUES (?, ?, ?, 'gate-1', 'device-1', ?, 'admitted')`,
		store.NewID(), ticketID, eventID, time.Now().Format(time.RFC3339))
	return err
}
