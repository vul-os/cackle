package payments

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

// --- test doubles -----------------------------------------------------

// fakeProvider is a minimal, fully-controllable Provider for exercising
// the orchestration helpers (HandleWebhook, HandleVerify) without going
// through real HTTP/HMAC machinery — that's covered separately in
// paystack_test.go.
type fakeProvider struct {
	name string

	webhookResult Result
	webhookErr    error

	verifyResult Result
	verifyErr    error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	return Charge{}, errors.New("fakeProvider: Begin not used in these tests")
}

func (f *fakeProvider) Verify(ctx context.Context, reference string) (Result, error) {
	return f.verifyResult, f.verifyErr
}

func (f *fakeProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	return f.webhookResult, f.webhookErr
}

// fakeSeenStore is an in-memory SeenStore for replay-protection tests.
type fakeSeenStore struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newFakeSeenStore() *fakeSeenStore {
	return &fakeSeenStore{seen: make(map[string]bool)}
}

func (s *fakeSeenStore) MarkSeen(ctx context.Context, provider, eventID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := provider + ":" + eventID
	if s.seen[key] {
		return false, nil
	}
	s.seen[key] = true
	return true, nil
}

// erroringSeenStore always fails, to test that HandleWebhook fails closed
// when the replay check itself cannot be performed.
type erroringSeenStore struct{}

func (erroringSeenStore) MarkSeen(ctx context.Context, provider, eventID string) (bool, error) {
	return false, errors.New("simulated seen-store failure")
}

// fakeOrderLookup is an in-memory OrderLookup.
type fakeOrderLookup struct {
	orders map[string]OrderRef
	err    error
}

func (f *fakeOrderLookup) Lookup(ctx context.Context, reference string) (OrderRef, error) {
	if f.err != nil {
		return OrderRef{}, f.err
	}
	o, ok := f.orders[reference]
	if !ok {
		return OrderRef{}, errors.New("fakeOrderLookup: order not found")
	}
	return o, nil
}

// --- Reconcile ----------------------------------------------------------

func TestReconcile_Success(t *testing.T) {
	result := Result{Reference: "ord_1", Status: StatusPaid, AmountCents: 5000, Currency: "ZAR"}
	want := OrderRef{ID: "ord_1", TotalCents: 5000, Currency: "ZAR"}
	if err := Reconcile(result, want); err != nil {
		t.Fatalf("Reconcile() = %v, want nil", err)
	}
}

func TestReconcile_AmountMismatchRejected(t *testing.T) {
	result := Result{Reference: "ord_1", Status: StatusPaid, AmountCents: 100, Currency: "ZAR"}
	want := OrderRef{ID: "ord_1", TotalCents: 5000, Currency: "ZAR"}
	err := Reconcile(result, want)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("Reconcile() = %v, want ErrAmountMismatch", err)
	}
}

func TestReconcile_CurrencyMismatchRejected(t *testing.T) {
	result := Result{Reference: "ord_1", Status: StatusPaid, AmountCents: 5000, Currency: "USD"}
	want := OrderRef{ID: "ord_1", TotalCents: 5000, Currency: "ZAR"}
	err := Reconcile(result, want)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Reconcile() = %v, want ErrCurrencyMismatch", err)
	}
}

func TestReconcile_ReferenceMismatchRejected(t *testing.T) {
	result := Result{Reference: "ord_evil", Status: StatusPaid, AmountCents: 5000, Currency: "ZAR"}
	want := OrderRef{ID: "ord_1", TotalCents: 5000, Currency: "ZAR"}
	err := Reconcile(result, want)
	if !errors.Is(err, ErrReferenceMismatch) {
		t.Fatalf("Reconcile() = %v, want ErrReferenceMismatch", err)
	}
}

func TestReconcile_NotPaidRejected(t *testing.T) {
	for _, status := range []Status{StatusPending, StatusFailed, ""} {
		result := Result{Reference: "ord_1", Status: status, AmountCents: 5000, Currency: "ZAR"}
		want := OrderRef{ID: "ord_1", TotalCents: 5000, Currency: "ZAR"}
		err := Reconcile(result, want)
		if !errors.Is(err, ErrNotPaid) {
			t.Fatalf("Reconcile() with status=%q = %v, want ErrNotPaid", status, err)
		}
	}
}

func TestReconcile_CaseInsensitiveCurrency(t *testing.T) {
	result := Result{Reference: "ord_1", Status: StatusPaid, AmountCents: 5000, Currency: "zar"}
	want := OrderRef{ID: "ord_1", TotalCents: 5000, Currency: "ZAR"}
	if err := Reconcile(result, want); err != nil {
		t.Fatalf("Reconcile() = %v, want nil (currency should be case-insensitive)", err)
	}
}

// --- HandleWebhook orchestration -----------------------------------------

func TestHandleWebhook_ProviderErrorPropagates(t *testing.T) {
	p := &fakeProvider{name: "fake", webhookErr: ErrInvalidSignature}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	_, err := HandleWebhook(context.Background(), p, req, nil, nil)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("HandleWebhook() = %v, want ErrInvalidSignature", err)
	}
}

func TestHandleWebhook_ReplayRejected(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		webhookResult: Result{
			Reference: "ord_1", EventID: "evt_1", Status: StatusPaid,
			AmountCents: 5000, Currency: "ZAR",
		},
	}
	seen := newFakeSeenStore()
	req, _ := http.NewRequest(http.MethodPost, "/", nil)

	if _, err := HandleWebhook(context.Background(), p, req, seen, nil); err != nil {
		t.Fatalf("first HandleWebhook() = %v, want nil", err)
	}
	_, err := HandleWebhook(context.Background(), p, req, seen, nil)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestHandleWebhook_ReplayCheckFailureFailsClosed(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		webhookResult: Result{
			Reference: "ord_1", EventID: "evt_1", Status: StatusPaid,
			AmountCents: 5000, Currency: "ZAR",
		},
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	_, err := HandleWebhook(context.Background(), p, req, erroringSeenStore{}, nil)
	if err == nil {
		t.Fatal("HandleWebhook() = nil error, want failure when the seen-store itself errors")
	}
}

func TestHandleWebhook_EmptyEventIDRejectedWhenPaid(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		webhookResult: Result{
			Reference: "ord_1", EventID: "", Status: StatusPaid,
			AmountCents: 5000, Currency: "ZAR",
		},
	}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	_, err := HandleWebhook(context.Background(), p, req, newFakeSeenStore(), nil)
	if err == nil {
		t.Fatal("HandleWebhook() = nil error, want failure for a paid result with no event id")
	}
}

func TestHandleWebhook_AmountMismatchRejectedViaLookup(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		webhookResult: Result{
			Reference: "ord_1", EventID: "evt_1", Status: StatusPaid,
			AmountCents: 100, Currency: "ZAR", // attacker/provider claims R1
		},
	}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{
		"ord_1": {ID: "ord_1", TotalCents: 500000, Currency: "ZAR"}, // real order is R5000
	}}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	_, err := HandleWebhook(context.Background(), p, req, newFakeSeenStore(), lookup)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrAmountMismatch", err)
	}
}

func TestHandleWebhook_CurrencyMismatchRejectedViaLookup(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		webhookResult: Result{
			Reference: "ord_1", EventID: "evt_1", Status: StatusPaid,
			AmountCents: 5000, Currency: "USD",
		},
	}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{
		"ord_1": {ID: "ord_1", TotalCents: 5000, Currency: "ZAR"},
	}}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	_, err := HandleWebhook(context.Background(), p, req, newFakeSeenStore(), lookup)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrCurrencyMismatch", err)
	}
}

func TestHandleWebhook_OrderLookupFailureFailsClosed(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		webhookResult: Result{
			Reference: "ord_missing", EventID: "evt_1", Status: StatusPaid,
			AmountCents: 5000, Currency: "ZAR",
		},
	}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{}}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	_, err := HandleWebhook(context.Background(), p, req, newFakeSeenStore(), lookup)
	if err == nil {
		t.Fatal("HandleWebhook() = nil error, want failure when the order can't be looked up")
	}
}

func TestHandleWebhook_SuccessPassesThrough(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		webhookResult: Result{
			Reference: "ord_1", EventID: "evt_1", Status: StatusPaid,
			AmountCents: 5000, Currency: "ZAR",
		},
	}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{
		"ord_1": {ID: "ord_1", TotalCents: 5000, Currency: "ZAR"},
	}}
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	result, err := HandleWebhook(context.Background(), p, req, newFakeSeenStore(), lookup)
	if err != nil {
		t.Fatalf("HandleWebhook() = %v, want nil", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("result.Status = %q, want paid", result.Status)
	}
}

// --- HandleVerify orchestration -------------------------------------------

func TestHandleVerify_ProviderErrorPropagates(t *testing.T) {
	p := &fakeProvider{name: "fake", verifyErr: errors.New("boom")}
	_, err := HandleVerify(context.Background(), p, "ord_1", nil)
	if err == nil {
		t.Fatal("HandleVerify() = nil error, want propagated provider error")
	}
}

func TestHandleVerify_AmountMismatchRejected(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		verifyResult: Result{
			Reference: "ord_1", Status: StatusPaid, AmountCents: 1, Currency: "ZAR",
		},
	}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{
		"ord_1": {ID: "ord_1", TotalCents: 5000, Currency: "ZAR"},
	}}
	_, err := HandleVerify(context.Background(), p, "ord_1", lookup)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("HandleVerify() = %v, want ErrAmountMismatch", err)
	}
}

func TestHandleVerify_Success(t *testing.T) {
	p := &fakeProvider{
		name: "fake",
		verifyResult: Result{
			Reference: "ord_1", Status: StatusPaid, AmountCents: 5000, Currency: "ZAR",
			PaidAt: time.Now(),
		},
	}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{
		"ord_1": {ID: "ord_1", TotalCents: 5000, Currency: "ZAR"},
	}}
	result, err := HandleVerify(context.Background(), p, "ord_1", lookup)
	if err != nil {
		t.Fatalf("HandleVerify() = %v, want nil", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("result.Status = %q, want paid", result.Status)
	}
}

// --- Registry -------------------------------------------------------------

func TestRegistry_RegisterGetNames(t *testing.T) {
	r := NewRegistry()
	stub, err := NewStub(true)
	if err != nil {
		t.Fatalf("NewStub() = %v", err)
	}
	if err := r.Register(stub); err != nil {
		t.Fatalf("Register() = %v", err)
	}
	got, ok := r.Get(ProviderNameStub)
	if !ok || got.Name() != ProviderNameStub {
		t.Fatalf("Get(%q) = %v, %v", ProviderNameStub, got, ok)
	}
	if names := r.Names(); len(names) != 1 || names[0] != ProviderNameStub {
		t.Fatalf("Names() = %v", names)
	}
}

func TestRegistry_DuplicateRejected(t *testing.T) {
	r := NewRegistry()
	stub, _ := NewStub(true)
	if err := r.Register(stub); err != nil {
		t.Fatalf("Register() = %v", err)
	}
	stub2, _ := NewStub(true)
	if err := r.Register(stub2); err == nil {
		t.Fatal("second Register() with the same name = nil error, want rejection")
	}
}

func TestRegistry_NilProviderRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatal("Register(nil) = nil error, want rejection")
	}
}

func TestRegistry_RefusesStubWhenRealSecretPresent(t *testing.T) {
	t.Setenv(EnvPaystackSecretKey, "sk_test_shouldnotmatter")
	r := NewRegistry()
	// Construct a StubProvider directly (bypassing NewStub's own guard)
	// to prove the Registry has its own independent, defense-in-depth
	// check rather than relying solely on the constructor.
	stub := &StubProvider{settled: make(map[string]Result)}
	err := r.Register(stub)
	if !errors.Is(err, ErrStubRefusesRealSecret) {
		t.Fatalf("Register() = %v, want ErrStubRefusesRealSecret", err)
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nonexistent"); ok {
		t.Fatal("Get() on empty registry = ok, want not-found")
	}
}
