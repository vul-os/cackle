package payments

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeRecordStore is an in-memory stand-in for internal/store's
// RecordStore implementation, used to prove manual/lnbits actually go
// through the durability seam (persist on write, reload on "restart")
// without needing a real database in this package's tests.
type fakeRecordStore struct {
	mu      sync.Mutex
	records map[string]PaymentRecord // key: provider + "\x00" + reference
	// failPut, if set, makes every PutPaymentRecord fail — used to prove
	// providers fail closed rather than silently keep an in-memory-only
	// mutation that was never actually made durable.
	failPut bool
}

func newFakeRecordStore() *fakeRecordStore {
	return &fakeRecordStore{records: make(map[string]PaymentRecord)}
}

func (f *fakeRecordStore) key(provider, reference string) string { return provider + "\x00" + reference }

func (f *fakeRecordStore) GetPaymentRecord(ctx context.Context, provider, reference string) (PaymentRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[f.key(provider, reference)]
	return rec, ok, nil
}

func (f *fakeRecordStore) PutPaymentRecord(ctx context.Context, rec PaymentRecord) error {
	if f.failPut {
		return errors.New("fakeRecordStore: forced Put failure")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[f.key(rec.Provider, rec.Reference)] = rec
	return nil
}

func (f *fakeRecordStore) ListPaymentRecords(ctx context.Context, provider string) ([]PaymentRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []PaymentRecord
	for _, rec := range f.records {
		if rec.Provider == provider {
			out = append(out, rec)
		}
	}
	return out, nil
}

func TestManualWithStore_SurvivesRestart(t *testing.T) {
	ctx := context.Background()
	rs := newFakeRecordStore()

	m1, err := NewManualWithStore(ctx, nil, rs)
	if err != nil {
		t.Fatalf("NewManualWithStore: %v", err)
	}
	order := Order{Reference: "ord-1", AmountMinor: 250000, Currency: "JPY", BuyerEmail: "a@b.com"}
	if _, err := m1.Begin(ctx, order); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := m1.MarkPaid(ctx, "ord-1", "owner@example.com"); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}

	// Simulate a process restart: a brand-new ManualProvider backed by the
	// SAME store must recover the full state, including the audit trail.
	m2, err := NewManualWithStore(ctx, nil, rs)
	if err != nil {
		t.Fatalf("NewManualWithStore (restart): %v", err)
	}
	result, err := m2.Verify(ctx, "ord-1")
	if err != nil {
		t.Fatalf("Verify after restart: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("Status after restart = %q, want paid", result.Status)
	}
	if result.AmountMinor != 250000 || result.Currency != "JPY" {
		t.Fatalf("amount/currency after restart = %d %s, want 250000 JPY", result.AmountMinor, result.Currency)
	}
	rec, ok := m2.Record("ord-1")
	if !ok {
		t.Fatal("Record after restart: not found")
	}
	if rec.MarkedBy != "owner@example.com" {
		t.Fatalf("MarkedBy after restart = %q, want owner@example.com (audit trail must survive)", rec.MarkedBy)
	}
}

func TestManualWithStore_BeginFailsClosedOnPersistError(t *testing.T) {
	ctx := context.Background()
	rs := newFakeRecordStore()
	rs.failPut = true

	m, err := NewManualWithStore(ctx, nil, rs)
	if err != nil {
		t.Fatalf("NewManualWithStore: %v", err)
	}
	_, err = m.Begin(ctx, Order{Reference: "ord-1", AmountMinor: 100, Currency: "USD"})
	if err == nil {
		t.Fatal("Begin with a failing store: want error, got nil (durability failure must not look like success)")
	}
	if _, ok := m.Record("ord-1"); ok {
		t.Fatal("Begin with a failing store must not leave a dangling in-memory-only record")
	}
}

func TestManualWithStore_MarkPaidFailsClosedAndRollsBack(t *testing.T) {
	ctx := context.Background()
	rs := newFakeRecordStore()

	m, err := NewManualWithStore(ctx, nil, rs)
	if err != nil {
		t.Fatalf("NewManualWithStore: %v", err)
	}
	if _, err := m.Begin(ctx, Order{Reference: "ord-1", AmountMinor: 100, Currency: "USD"}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	rs.failPut = true
	if _, err := m.MarkPaid(ctx, "ord-1", "owner@example.com"); err == nil {
		t.Fatal("MarkPaid with a failing store: want error, got nil")
	}

	// The in-memory mutation must have been rolled back — this replica's
	// view must not silently disagree with what was actually persisted.
	rec, ok := m.Record("ord-1")
	if !ok {
		t.Fatal("Record: not found")
	}
	if rec.Status != StatusPending {
		t.Fatalf("Status after failed MarkPaid = %q, want still pending (rolled back)", rec.Status)
	}
}

// Note: this file used to also cover LNbitsProvider's use of the same
// RecordStore seam (TestLNbitsWithStore_SurvivesRestart /
// TestLNbitsWithStore_UnknownReferenceStillFailsClosed). lnbits.go was
// removed when payment processing migrated onto the patala substrate (see
// docs/PAYMENTS.md "The patala path") — lnbits is now one of the 20
// processors reachable via the patala-tagged build
// (internal/payments/patala.go), which persists through the very same
// RecordStore/PaymentRecord shape exercised above via ManualProvider, so
// the seam itself is still fully covered by this file's remaining tests.
