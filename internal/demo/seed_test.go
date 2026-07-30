package demo

import (
	"context"
	"testing"

	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/orgs"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
	"github.com/vul-os/cackle/internal/store/keyvault"
)

// seedForTest wires up the same in-memory store + services the --demo path
// uses, seeds it once, and returns the store. The stub provider (auto-settle)
// is the one the seed's orders run through.
func seedForTest(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	// Event signing keys are encrypted at rest (internal/store/keyvault): a
	// freshly opened Store is LOCKED and refuses to create or use one. Tests
	// supply fixed key material explicitly — there is no plaintext mode for a
	// test to fall back to, and a test that quietly took one would be exactly
	// the reason the plaintext mode survived as long as it did.
	kvSrc, err := keyvault.Keyfile([]byte("cackle-test-event-key-material-32b!!"))
	if err != nil {
		t.Fatalf("keyvault.Keyfile: %v", err)
	}
	if err := st.UnlockKeyVault(context.Background(), kvSrc); err != nil {
		t.Fatalf("unlock event key vault: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ev := events.New(st)
	registry := payments.NewRegistry()
	stub, err := payments.NewStub(true)
	if err != nil {
		t.Fatalf("new stub provider: %v", err)
	}
	if err := registry.Register(stub); err != nil {
		t.Fatalf("register stub provider: %v", err)
	}
	or := orders.New(st, ev, registry)
	og := orgs.New(st, nil) // nil BankingProvider is valid (see orgs.BankingProvider)

	if err := Seed(ctx, st, ev, or, og); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	return st
}

// TestSeedPopulatesDefaultEventAnalytics guards the demo's sales data. The
// organiser analytics / attendees / scanner screens open by default on the
// SOONEST event (cape-town-pub-quiz-championship). If that event has no seeded
// orders, those screens — and their committed screenshots — render all zeros,
// an empty and unconvincing showcase of a real feature. This asserts the seed
// actually moves inventory through the real order -> settle -> admit path for
// that default event, so a future change to eventsWithOrders (or to which
// event sorts first) can't silently empty the analytics again.
func TestSeedPopulatesDefaultEventAnalytics(t *testing.T) {
	ctx := context.Background()
	st := seedForTest(t)

	ev, err := st.GetEventBySlug(ctx, "cape-town-pub-quiz-championship")
	if err != nil {
		t.Fatalf("get default demo event: %v", err)
	}

	stats, err := st.TicketTypeStatsForEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ticket-type stats: %v", err)
	}
	var sold int
	var revenue int64
	for _, s := range stats {
		sold += s.Sold
		revenue += s.RevenueMinor
	}
	if sold <= 0 {
		t.Fatalf("default demo event has no seeded sales (sold=%d); analytics/attendees would render empty", sold)
	}
	if revenue <= 0 {
		t.Fatalf("default demo event has zero revenue (%d) despite %d sold", revenue, sold)
	}

	admitted, err := st.CountAdmittedForEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("count admitted: %v", err)
	}
	if admitted <= 0 {
		t.Fatalf("default demo event has no admissions (admitted=%d); the scanner/admission stats would render empty", admitted)
	}
	if admitted > sold {
		t.Fatalf("admitted (%d) exceeds sold (%d) — a ticket was admitted that was never issued", admitted, sold)
	}
}
