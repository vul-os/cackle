package orders

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
	"github.com/vul-os/cackle/internal/tickets"
)

// --- fixtures -------------------------------------------------------------

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

func mustStub(t *testing.T) *payments.Registry {
	t.Helper()
	reg := payments.NewRegistry()
	stub, err := payments.NewStub(true)
	if err != nil {
		t.Fatalf("payments.NewStub: %v", err)
	}
	if err := reg.Register(stub); err != nil {
		t.Fatalf("register stub: %v", err)
	}
	return reg
}

// fixture bundles a fully wired events+orders test environment around one
// published event with one ticket type.
type fixture struct {
	st       *store.Store
	events   *events.Service
	orders   *Service
	org      *store.Org
	event    *events.Event
	ticketTy *events.TicketType
}

func newFixture(t *testing.T, quantityTotal int, priceCents int64) *fixture {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()

	org := &store.Org{Name: "Fixture Promoters", Slug: "fixture-" + store.NewID()}
	if err := st.CreateOrg(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}

	evSvc := events.New(st)
	starts := time.Now().Add(24 * time.Hour)
	ev, err := evSvc.Create(ctx, org.ID, events.CreateEventInput{
		Slug:     "fixture-event-" + store.NewID(),
		Title:    "Fixture Event",
		StartsAt: starts,
		EndsAt:   starts.Add(4 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := evSvc.Publish(ctx, ev.ID); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	tt, err := evSvc.CreateTicketType(ctx, ev.ID, events.TicketTypeInput{
		Name:          "General",
		PriceCents:    priceCents,
		QuantityTotal: quantityTotal,
	})
	if err != nil {
		t.Fatalf("create ticket type: %v", err)
	}

	reg := mustStub(t)
	ordSvc := New(st, evSvc, reg)

	return &fixture{st: st, events: evSvc, orders: ordSvc, org: org, event: ev, ticketTy: tt}
}

// --- price forgery ---------------------------------------------------------

func TestCreateIgnoresClientPrice_UsesDatabasePrice(t *testing.T) {
	fx := newFixture(t, 10, 12345)
	ctx := context.Background()

	// OrderItemInput has no price field at all: the only way to spend
	// money is ticket_type_id + quantity. Verify the resulting total is
	// exactly what the database ticket type charges, regardless of the
	// ticket type's price having been set AFTER the client would have
	// seen any stale price.
	order, charge, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID:    fx.event.ID,
		BuyerEmail: "buyer@example.com",
		BuyerName:  "Buyer",
		Items:      []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 3}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	wantTotal := int64(12345 * 3)
	if order.TotalCents != wantTotal {
		t.Fatalf("TotalCents = %d, want %d (price must come from DB, never the client)", order.TotalCents, wantTotal)
	}
	if len(order.Items) != 1 || order.Items[0].UnitPriceCents != 12345 {
		t.Fatalf("order item unit price = %+v, want 12345", order.Items)
	}
	if charge == nil || charge.Reference != order.ID {
		t.Fatalf("charge = %+v, want reference %q", charge, order.ID)
	}

	// Now change the ticket type's price. A NEW order must use the new
	// price; the order created above must be untouched (its
	// order_items.unit_price_cents is frozen at purchase time).
	if _, err := fx.events.UpdateTicketType(ctx, fx.ticketTy.ID, events.TicketTypeInput{
		Name: "General", PriceCents: 99999, QuantityTotal: 10,
	}); err != nil {
		t.Fatalf("update price: %v", err)
	}

	reloaded, err := fx.orders.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reloaded.TotalCents != wantTotal {
		t.Fatalf("existing order total changed after price update: got %d, want %d", reloaded.TotalCents, wantTotal)
	}

	order2, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID:    fx.event.ID,
		BuyerEmail: "buyer2@example.com",
		Items:      []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	if order2.TotalCents != 99999 {
		t.Fatalf("new order total = %d, want 99999 (new DB price)", order2.TotalCents)
	}
}

// --- oversell / concurrent race ---------------------------------------------

func TestCreate_OversellRejected(t *testing.T) {
	fx := newFixture(t, 2, 1000)
	ctx := context.Background()

	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "a@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 3}},
	}); !errors.Is(err, ErrSoldOut) {
		t.Fatalf("single order exceeding quantity_total: got %v, want ErrSoldOut", err)
	}

	// Exactly 2 remain: buy them...
	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "b@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 2}},
	}); err != nil {
		t.Fatalf("buy remaining 2: %v", err)
	}

	// ...then a third buyer must be rejected, not oversold.
	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "c@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	}); !errors.Is(err, ErrSoldOut) {
		t.Fatalf("buying after sold out: got %v, want ErrSoldOut", err)
	}

	tt, err := fx.st.GetTicketTypeByID(ctx, fx.ticketTy.ID)
	if err != nil {
		t.Fatalf("GetTicketTypeByID: %v", err)
	}
	if tt.QuantitySold != 2 {
		t.Fatalf("quantity_sold = %d, want exactly 2 (no oversell)", tt.QuantitySold)
	}
}

// TestCreate_ConcurrentRaceForLastTicket races N goroutines for exactly 1
// remaining ticket and asserts exactly one wins — the cardinal no-oversell
// guarantee under real concurrency (not just sequential calls).
func TestCreate_ConcurrentRaceForLastTicket(t *testing.T) {
	const attempts = 12
	fx := newFixture(t, 1, 500)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var successes int
	var soldOutCount int
	var otherErrs []error

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _, err := fx.orders.Create(ctx, CreateOrderInput{
				EventID:    fx.event.ID,
				BuyerEmail: "racer@example.com",
				Items:      []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrSoldOut):
				soldOutCount++
			default:
				otherErrs = append(otherErrs, err)
			}
		}(i)
	}
	wg.Wait()

	if len(otherErrs) > 0 {
		t.Fatalf("unexpected errors during race: %v", otherErrs)
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1 (oversold or undersold the last ticket)", successes)
	}
	if soldOutCount != attempts-1 {
		t.Fatalf("sold-out rejections = %d, want %d", soldOutCount, attempts-1)
	}

	tt, err := fx.st.GetTicketTypeByID(ctx, fx.ticketTy.ID)
	if err != nil {
		t.Fatalf("GetTicketTypeByID: %v", err)
	}
	if tt.QuantitySold != 1 {
		t.Fatalf("quantity_sold = %d, want exactly 1 after the race", tt.QuantitySold)
	}
}

// --- validation --------------------------------------------------------

func TestCreate_ValidationErrors(t *testing.T) {
	fx := newFixture(t, 10, 1000)
	ctx := context.Background()

	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{EventID: fx.event.ID, BuyerEmail: "x@example.com"}); !errors.Is(err, ErrEmptyOrder) {
		t.Fatalf("empty items: got %v, want ErrEmptyOrder", err)
	}

	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "x@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 0}},
	}); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("zero quantity: got %v, want ErrInvalidQuantity", err)
	}

	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "x@example.com",
		Items: []OrderItemInput{{TicketTypeID: "nonexistent", Quantity: 1}},
	}); !errors.Is(err, ErrTicketTypeNotFound) {
		t.Fatalf("nonexistent ticket type: got %v, want ErrTicketTypeNotFound", err)
	}

	// A ticket type belonging to a DIFFERENT event must not be usable.
	otherEvSvc := events.New(fx.st)
	starts := time.Now().Add(48 * time.Hour)
	otherEv, err := otherEvSvc.Create(ctx, fx.org.ID, events.CreateEventInput{
		Slug: "other-event-" + store.NewID(), Title: "Other", StartsAt: starts, EndsAt: starts.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create other event: %v", err)
	}
	if _, err := otherEvSvc.Publish(ctx, otherEv.ID); err != nil {
		t.Fatalf("publish other event: %v", err)
	}
	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: otherEv.ID, BuyerEmail: "x@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	}); !errors.Is(err, ErrTicketTypeNotFound) {
		t.Fatalf("cross-event ticket type: got %v, want ErrTicketTypeNotFound", err)
	}

	// A draft (unpublished) event must reject orders.
	draftEv, err := otherEvSvc.Create(ctx, fx.org.ID, events.CreateEventInput{
		Slug: "draft-event-" + store.NewID(), Title: "Draft", StartsAt: starts, EndsAt: starts.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create draft event: %v", err)
	}
	draftTT, err := otherEvSvc.CreateTicketType(ctx, draftEv.ID, events.TicketTypeInput{Name: "GA", PriceCents: 100, QuantityTotal: 5})
	if err != nil {
		t.Fatalf("create ticket type for draft event: %v", err)
	}
	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: draftEv.ID, BuyerEmail: "x@example.com",
		Items: []OrderItemInput{{TicketTypeID: draftTT.ID, Quantity: 1}},
	}); !errors.Is(err, ErrEventNotPublished) {
		t.Fatalf("draft event order: got %v, want ErrEventNotPublished", err)
	}

	// max_per_order enforcement.
	cappedTT, err := fx.events.CreateTicketType(ctx, fx.event.ID, events.TicketTypeInput{
		Name: "Capped", PriceCents: 100, QuantityTotal: 10, MaxPerOrder: 2,
	})
	if err != nil {
		t.Fatalf("create capped ticket type: %v", err)
	}
	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "x@example.com",
		Items: []OrderItemInput{{TicketTypeID: cappedTT.ID, Quantity: 3}},
	}); !errors.Is(err, ErrMaxPerOrderExceeded) {
		t.Fatalf("over max_per_order: got %v, want ErrMaxPerOrderExceeded", err)
	}
}

// --- settle idempotency -------------------------------------------------

func TestSettle_IssuesTicketsAndIsIdempotent(t *testing.T) {
	fx := newFixture(t, 10, 2500)
	ctx := context.Background()

	order, charge, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "buyer@example.com", BuyerName: "Buyer",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 3}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stubProvider, ok := fx.orders.payments.Get(payments.ProviderNameStub)
	if !ok {
		t.Fatal("stub provider not registered")
	}
	result, err := stubProvider.Verify(ctx, charge.Reference)
	if err != nil {
		t.Fatalf("stub Verify: %v", err)
	}

	settledOrder1, tickets1, err := fx.orders.Settle(ctx, result)
	if err != nil {
		t.Fatalf("Settle (first delivery): %v", err)
	}
	if settledOrder1.Status != "paid" {
		t.Fatalf("order status after settle = %q, want paid", settledOrder1.Status)
	}
	if len(tickets1) != 3 {
		t.Fatalf("issued %d tickets, want 3 (one per unit purchased)", len(tickets1))
	}
	seen := map[string]bool{}
	for _, tk := range tickets1 {
		if tk.Capability == "" {
			t.Fatal("issued ticket has empty capability")
		}
		if seen[tk.ID] {
			t.Fatalf("duplicate ticket id %s within a single settle call", tk.ID)
		}
		seen[tk.ID] = true
	}

	// Deliver the SAME webhook result a second time — must not double-issue.
	settledOrder2, tickets2, err := fx.orders.Settle(ctx, result)
	if err != nil {
		t.Fatalf("Settle (second/replayed delivery): %v", err)
	}
	if settledOrder2.Status != "paid" {
		t.Fatalf("order status after second settle = %q, want paid", settledOrder2.Status)
	}
	if len(tickets2) != 3 {
		t.Fatalf("second settle returned %d tickets, want 3 (the same ones)", len(tickets2))
	}
	gotIDs := map[string]bool{}
	for _, tk := range tickets2 {
		gotIDs[tk.ID] = true
	}
	for id := range seen {
		if !gotIDs[id] {
			t.Fatalf("second settle is missing ticket %s issued by the first", id)
		}
	}

	all, err := fx.st.ListTicketsForOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListTicketsForOrder: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("total ticket rows in DB for order = %d, want exactly 3 (no duplicates persisted)", len(all))
	}
}

// TestSettle_ConcurrentDoubleDeliveryIssuesOnce simulates two webhook
// deliveries racing each other for the SAME order, concurrently, and
// asserts the final ticket count is still exactly right.
func TestSettle_ConcurrentDoubleDeliveryIssuesOnce(t *testing.T) {
	fx := newFixture(t, 10, 1500)
	ctx := context.Background()

	order, charge, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "buyer@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stubProvider, _ := fx.orders.payments.Get(payments.ProviderNameStub)
	result, err := stubProvider.Verify(ctx, charge.Reference)
	if err != nil {
		t.Fatalf("stub Verify: %v", err)
	}

	const deliveries = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ticketCounts []int
	var errs []error

	for i := 0; i < deliveries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, tks, err := fx.orders.Settle(ctx, result)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ticketCounts = append(ticketCounts, len(tks))
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors during concurrent settle: %v", errs)
	}
	for _, n := range ticketCounts {
		if n != 2 {
			t.Fatalf("a concurrent Settle call returned %d tickets, want 2", n)
		}
	}

	all, err := fx.st.ListTicketsForOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListTicketsForOrder: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("total ticket rows in DB = %d, want exactly 2 (concurrent double delivery must not double-issue)", len(all))
	}
}

// TestSettle_RejectsAmountMismatch confirms Settle refuses to issue
// tickets if the provider's reported amount doesn't match the order total
// — the anti-fraud check inherited from internal/payments.Reconcile.
func TestSettle_RejectsAmountMismatch(t *testing.T) {
	fx := newFixture(t, 10, 1000)
	ctx := context.Background()

	order, charge, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "buyer@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	forged := payments.Result{
		Provider:    payments.ProviderNameStub,
		Reference:   charge.Reference,
		EventID:     "forged-event-id",
		Status:      payments.StatusPaid,
		AmountCents: 1, // forged: order total is 1000
		Currency:    "ZAR",
		PaidAt:      time.Now(),
	}
	if _, _, err := fx.orders.Settle(ctx, forged); !errors.Is(err, payments.ErrAmountMismatch) {
		t.Fatalf("forged amount: got %v, want ErrAmountMismatch", err)
	}

	all, err := fx.st.ListTicketsForOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListTicketsForOrder: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("tickets issued despite amount mismatch: %d", len(all))
	}
}

// --- ticket capability round-trip ---------------------------------------

func TestTicketCapabilityRoundTripsThroughIssuance(t *testing.T) {
	fx := newFixture(t, 5, 4200)
	ctx := context.Background()

	order, charge, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "buyer@example.com", BuyerName: "Holder Name",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stubProvider, _ := fx.orders.payments.Get(payments.ProviderNameStub)
	result, err := stubProvider.Verify(ctx, charge.Reference)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	_, tks, err := fx.orders.Settle(ctx, result)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if len(tks) != 1 {
		t.Fatalf("issued %d tickets, want 1", len(tks))
	}
	issued := tks[0]

	ring, err := fx.events.IssuerPublicKeys(ctx, fx.event.ID)
	if err != nil {
		t.Fatalf("IssuerPublicKeys: %v", err)
	}

	payload, err := tickets.VerifyWithRing(issued.Capability, ring, time.Now())
	if err != nil {
		t.Fatalf("VerifyWithRing: %v", err)
	}
	if payload.TID != issued.ID {
		t.Fatalf("payload.TID = %q, want %q", payload.TID, issued.ID)
	}
	if payload.EID != fx.event.ID {
		t.Fatalf("payload.EID = %q, want %q", payload.EID, fx.event.ID)
	}
	if payload.TT != fx.ticketTy.ID {
		t.Fatalf("payload.TT = %q, want %q", payload.TT, fx.ticketTy.ID)
	}
	if payload.Name != "Holder Name" {
		t.Fatalf("payload.Name = %q, want %q", payload.Name, "Holder Name")
	}

	// A capability signed under one event's key must not verify against a
	// different event's ring (proves per-event key isolation end to end).
	otherEvSvc := events.New(fx.st)
	starts := time.Now().Add(72 * time.Hour)
	otherEv, err := otherEvSvc.Create(ctx, fx.org.ID, events.CreateEventInput{
		Slug: "isolation-event-" + store.NewID(), Title: "Isolation", StartsAt: starts, EndsAt: starts.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create other event: %v", err)
	}
	otherRing, err := otherEvSvc.IssuerPublicKeys(ctx, otherEv.ID)
	if err != nil {
		t.Fatalf("IssuerPublicKeys (other): %v", err)
	}
	if _, err := tickets.VerifyWithRing(issued.Capability, otherRing, time.Now()); err == nil {
		t.Fatal("ticket verified against a different event's key ring — per-event key isolation broken")
	}

	// Sanity: order is fully paid.
	gotOrder, err := fx.orders.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotOrder.Status != "paid" {
		t.Fatalf("order status = %q, want paid", gotOrder.Status)
	}
}

// --- ListForUser / TicketsForUser ---------------------------------------

func TestListForUserAndTicketsForUser(t *testing.T) {
	fx := newFixture(t, 5, 1000)
	ctx := context.Background()

	userID := store.NewID()
	// CreateUser isn't strictly required by FK (orders.user_id has no
	// NOT NULL requirement against a real row check beyond FK), but
	// tickets.holder_user_id references users(id) too, so create a real
	// user for referential integrity.
	if err := fx.st.CreateUser(ctx, &store.User{ID: userID, Email: "user-" + userID + "@example.com", PasswordHash: "x"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	order, charge, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, UserID: userID, BuyerEmail: "user@example.com", BuyerName: "User",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stubProvider, _ := fx.orders.payments.Get(payments.ProviderNameStub)
	result, err := stubProvider.Verify(ctx, charge.Reference)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if _, _, err := fx.orders.Settle(ctx, result); err != nil {
		t.Fatalf("Settle: %v", err)
	}

	orders, err := fx.orders.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != order.ID {
		t.Fatalf("ListForUser = %+v, want exactly one order %q", orders, order.ID)
	}

	tks, err := fx.orders.TicketsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("TicketsForUser: %v", err)
	}
	if len(tks) != 2 {
		t.Fatalf("TicketsForUser = %d tickets, want 2", len(tks))
	}
	for _, tk := range tks {
		if tk.HolderUserID != userID {
			t.Fatalf("ticket holder = %q, want %q", tk.HolderUserID, userID)
		}
	}

	single, err := fx.orders.Ticket(ctx, tks[0].ID)
	if err != nil {
		t.Fatalf("Ticket: %v", err)
	}
	if single.ID != tks[0].ID {
		t.Fatalf("Ticket lookup mismatch")
	}
}

// --- provider resolution --------------------------------------------------

func TestCreate_ProviderRequiredWhenAmbiguous(t *testing.T) {
	fx := newFixture(t, 5, 1000)
	ctx := context.Background()

	// Register a second provider so there is no unambiguous default.
	second, err := payments.NewStub(true)
	if err != nil {
		t.Fatalf("NewStub: %v", err)
	}
	// Registering a second provider under the same name is refused, so
	// register a distinct name via a thin wrapper implementing Provider.
	if err := fx.orders.payments.Register(nameOverride{second, "manual-eft"}); err != nil {
		t.Fatalf("register second provider: %v", err)
	}

	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "x@example.com",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	}); !errors.Is(err, ErrProviderRequired) {
		t.Fatalf("ambiguous provider: got %v, want ErrProviderRequired", err)
	}

	// Explicit provider selection still works.
	order, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "x@example.com", Provider: "manual-eft",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("Create with explicit provider: %v", err)
	}
	if order.Provider != "manual-eft" {
		t.Fatalf("order.Provider = %q, want manual-eft", order.Provider)
	}

	if _, _, err := fx.orders.Create(ctx, CreateOrderInput{
		EventID: fx.event.ID, BuyerEmail: "x@example.com", Provider: "does-not-exist",
		Items: []OrderItemInput{{TicketTypeID: fx.ticketTy.ID, Quantity: 1}},
	}); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown provider: got %v, want ErrUnknownProvider", err)
	}
}

// nameOverride wraps a Provider under a different Name(), purely so tests
// can register two independent providers without the stub's fixed name
// colliding.
type nameOverride struct {
	payments.Provider
	name string
}

func (n nameOverride) Name() string { return n.name }
