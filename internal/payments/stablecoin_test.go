package payments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeStablecoinAllocator is an in-memory StablecoinAddressAllocator for
// tests, standing in for whatever persistent address pool a real deployment
// would wire up.
type fakeStablecoinAllocator struct {
	mu      sync.Mutex
	next    []string
	byOrder map[string]StablecoinAllocation
	byAddr  map[string]StablecoinAllocation
}

func newFakeStablecoinAllocator(addresses ...string) *fakeStablecoinAllocator {
	return &fakeStablecoinAllocator{
		next:    addresses,
		byOrder: make(map[string]StablecoinAllocation),
		byAddr:  make(map[string]StablecoinAllocation),
	}
}

func (f *fakeStablecoinAllocator) Allocate(ctx context.Context, orderReference string, amountMinor int64, currency string) (StablecoinAllocation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.next) == 0 {
		return StablecoinAllocation{}, errors.New("no addresses left in pool")
	}
	addr := f.next[0]
	f.next = f.next[1:]
	alloc := StablecoinAllocation{Address: addr, AmountMinor: amountMinor, Currency: currency, AllocatedAt: time.Now()}
	f.byOrder[orderReference] = alloc
	f.byAddr[strings.ToLower(addr)] = alloc
	return alloc, nil
}

func (f *fakeStablecoinAllocator) Lookup(ctx context.Context, address string) (StablecoinAllocation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	alloc, ok := f.byAddr[strings.ToLower(address)]
	if !ok {
		return StablecoinAllocation{}, fmt.Errorf("no allocation for address %q", address)
	}
	return alloc, nil
}

func newTestStablecoin(t *testing.T, srv *httptest.Server, allocator StablecoinAddressAllocator) *StablecoinProvider {
	t.Helper()
	return &StablecoinProvider{
		indexerBaseURL: srv.URL,
		indexerAPIKey:  "test-indexer-key",
		tokenContract:  "0xTokenContract",
		tokenDecimals:  6,
		quoteCurrency:  "USD",
		minConfirms:    12,
		orderTTL:       time.Hour,
		httpClient:     srv.Client(),
		allocator:      allocator,
	}
}

func tokenTxJSON(to, value string, decimals, confirmations int, unixTime int64) string {
	return fmt.Sprintf(`{"to":"%s","value":"%s","tokenDecimal":"%d","confirmations":"%d","timeStamp":"%d","hash":"0xabc"}`,
		to, value, decimals, confirmations, unixTime)
}

func TestStablecoinBegin_Success(t *testing.T) {
	allocator := newFakeStablecoinAllocator("0xReceiveAddress1")
	p := newTestStablecoin(t, httptest.NewServer(http.NotFoundHandler()), allocator)

	charge, err := p.Begin(context.Background(), Order{Reference: "order_1", AmountMinor: 1234, Currency: "USD"})
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if charge.Reference != "0xReceiveAddress1" {
		t.Fatalf("expected the allocated address as reference, got %q", charge.Reference)
	}
	if !strings.Contains(charge.Instructions, "0xReceiveAddress1") {
		t.Fatalf("expected instructions to include the address, got: %q", charge.Instructions)
	}
}

func TestStablecoinBegin_RejectsWrongCurrency(t *testing.T) {
	allocator := newFakeStablecoinAllocator("0xAddr")
	p := newTestStablecoin(t, httptest.NewServer(http.NotFoundHandler()), allocator)

	_, err := p.Begin(context.Background(), Order{Reference: "order_1", AmountMinor: 1234, Currency: "EUR"})
	if !errors.Is(err, ErrStablecoinUnsupportedCurrency) {
		t.Fatalf("expected ErrStablecoinUnsupportedCurrency, got %v", err)
	}
}

func TestStablecoinBegin_RejectsNonPositiveAmount(t *testing.T) {
	allocator := newFakeStablecoinAllocator("0xAddr")
	p := newTestStablecoin(t, httptest.NewServer(http.NotFoundHandler()), allocator)

	if _, err := p.Begin(context.Background(), Order{Reference: "o1", AmountMinor: 0, Currency: "USD"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestNewStablecoin_RequiresAllocator(t *testing.T) {
	if _, err := NewStablecoin(nil); err == nil {
		t.Fatal("expected error for a nil allocator")
	}
}

func TestStablecoinVerify_ExactPaymentSettles(t *testing.T) {
	allocator := newFakeStablecoinAllocator("0xAddr")
	now := time.Now()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: now.Add(-time.Minute)}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 1234 minor USD units (2 decimals) == 12.34 USD == 12_340_000 raw
		// units of a 6-decimal token, at the 1:1 peg this adapter assumes.
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
			tokenTxJSON("0xAddr", "12340000", 6, 20, now.Unix()) + `]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
	if result.AmountMinor != 1234 {
		t.Fatalf("expected 1234 minor units received, got %d", result.AmountMinor)
	}
	if result.EventID == "" {
		t.Fatal("expected a non-empty EventID for a paid result")
	}
}

func TestStablecoinVerify_UnderpaymentStaysPending(t *testing.T) {
	allocator := newFakeStablecoinAllocator()
	now := time.Now()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: now.Add(-time.Minute)}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only 5000000 raw units == 5.00 USD, less than the 12.34 USD ask.
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
			tokenTxJSON("0xAddr", "5000000", 6, 20, now.Unix()) + `]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusPending {
		t.Fatalf("expected StatusPending for an underpayment, got %v", result.Status)
	}
}

func TestStablecoinVerify_OverpaymentIsFlaggedNotSilentlyAccepted(t *testing.T) {
	allocator := newFakeStablecoinAllocator()
	now := time.Now()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: now.Add(-time.Minute)}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 20000000 raw units == 20.00 USD, MORE than the 12.34 USD ask.
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
			tokenTxJSON("0xAddr", "20000000", 6, 20, now.Unix()) + `]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	// This adapter reports StatusPaid with the ACTUAL received amount;
	// it's the generic Reconcile() (in provider.go) that then rejects the
	// mismatch against the stored order total — see file doc comment.
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid (with the larger actual amount) so Reconcile can flag it, got %v", result.Status)
	}
	if result.AmountMinor != 2000 {
		t.Fatalf("expected the actual received 2000 minor units, got %d", result.AmountMinor)
	}
	want := OrderRef{ID: "0xAddr", AmountMinor: 1234, Currency: "USD"}
	if err := Reconcile(result, want); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("expected Reconcile to flag the overpayment as ErrAmountMismatch, got %v", err)
	}
}

func TestStablecoinVerify_InsufficientConfirmationsNotCounted(t *testing.T) {
	allocator := newFakeStablecoinAllocator()
	now := time.Now()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: now.Add(-time.Minute)}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Full amount, but only 2 confirmations when 12 are required.
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
			tokenTxJSON("0xAddr", "12340000", 6, 2, now.Unix()) + `]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("a transfer below the required confirmation count must never settle the order")
	}
}

func TestStablecoinVerify_ZeroConfNeverSettles(t *testing.T) {
	allocator := newFakeStablecoinAllocator()
	now := time.Now()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: now.Add(-time.Minute)}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
			tokenTxJSON("0xAddr", "12340000", 6, 0, now.Unix()) + `]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("a zero-conf transfer must never settle the order")
	}
}

func TestStablecoinVerify_TransferBeforeAllocationIgnored(t *testing.T) {
	// Guards against crediting a stale/reused-address transfer that
	// predates this order — see "address reuse" hazard in the file doc
	// comment.
	allocator := newFakeStablecoinAllocator()
	allocatedAt := time.Now()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: allocatedAt}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		oldTime := allocatedAt.Add(-time.Hour).Unix()
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
			tokenTxJSON("0xAddr", "12340000", 6, 20, oldTime) + `]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("a transfer that predates this order's allocation must not settle it")
	}
}

func TestStablecoinVerify_ExpiredOrderFailsClosed(t *testing.T) {
	allocator := newFakeStablecoinAllocator()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: time.Now().Add(-2 * time.Hour)}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"0","message":"No transactions found","result":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	p.orderTTL = time.Hour
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("expected StatusFailed for an order past its TTL with no payment, got %v", result.Status)
	}
}

func TestStablecoinVerify_EmptyResultQuirkIsNotAnError(t *testing.T) {
	// The documented Etherscan-family quirk: an empty result set comes
	// back as status="0" with message="No transactions found" — this must
	// NOT be treated as a hard error.
	allocator := newFakeStablecoinAllocator()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: time.Now()}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"0","message":"No transactions found","result":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	result, err := p.Verify(context.Background(), "0xAddr")
	if err != nil {
		t.Fatalf("expected the empty-result quirk to be handled without error, got %v", err)
	}
	if result.Status != StatusPending {
		t.Fatalf("expected StatusPending, got %v", result.Status)
	}
}

func TestStablecoinVerify_RealErrorStillFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"0","message":"Invalid API Key","result":"Invalid API Key"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	allocator := newFakeStablecoinAllocator()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: time.Now()}
	p := newTestStablecoin(t, srv, allocator)

	if _, err := p.Verify(context.Background(), "0xAddr"); err == nil {
		t.Fatal("expected error for a real indexer error (invalid API key)")
	}
}

func TestStablecoinVerify_TokenDecimalMismatchFailsClosed(t *testing.T) {
	allocator := newFakeStablecoinAllocator()
	now := time.Now()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: now.Add(-time.Minute)}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// tokenDecimal 18 (e.g. a mismatched/wrong contract) when the
		// adapter is configured for 6.
		_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
			tokenTxJSON("0xAddr", "12340000000000000000", 18, 20, now.Unix()) + `]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestStablecoin(t, srv, allocator)
	if _, err := p.Verify(context.Background(), "0xAddr"); !errors.Is(err, ErrStablecoinTokenDecimalMismatch) {
		t.Fatalf("expected ErrStablecoinTokenDecimalMismatch, got %v", err)
	}
}

func TestStablecoinVerify_UnknownAddressFailsClosed(t *testing.T) {
	allocator := newFakeStablecoinAllocator()
	p := newTestStablecoin(t, httptest.NewServer(http.NotFoundHandler()), allocator)

	if _, err := p.Verify(context.Background(), "0xNeverAllocated"); err == nil {
		t.Fatal("expected error for an address this process never allocated")
	}
}

func TestStablecoinVerify_MalformedJSONFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	allocator := newFakeStablecoinAllocator()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: time.Now()}
	p := newTestStablecoin(t, srv, allocator)

	if _, err := p.Verify(context.Background(), "0xAddr"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestStablecoinVerify_ServerErrorFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	allocator := newFakeStablecoinAllocator()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: time.Now()}
	p := newTestStablecoin(t, srv, allocator)

	if _, err := p.Verify(context.Background(), "0xAddr"); err == nil {
		t.Fatal("expected error for a 500 response")
	}
}

func TestStablecoinVerify_TimeoutFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	allocator := newFakeStablecoinAllocator()
	allocator.byAddr["0xaddr"] = StablecoinAllocation{Address: "0xAddr", AmountMinor: 1234, Currency: "USD", AllocatedAt: time.Now()}
	p := newTestStablecoin(t, srv, allocator)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Verify(ctx, "0xAddr"); err == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestStablecoinWebhook_AlwaysFails(t *testing.T) {
	p := &StablecoinProvider{}
	req := httptest.NewRequest(http.MethodPost, "/webhook/stablecoin", nil)
	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrStablecoinNoWebhook) {
		t.Fatalf("expected ErrStablecoinNoWebhook, got %v", err)
	}
}

func TestNewStablecoin_RequiresEnvVars(t *testing.T) {
	t.Setenv(EnvStablecoinIndexerBaseURL, "")
	t.Setenv(EnvStablecoinIndexerAPIKey, "")
	t.Setenv(EnvStablecoinTokenContract, "")
	t.Setenv(EnvStablecoinTokenDecimals, "")
	t.Setenv(EnvStablecoinMinConfirmations, "")
	if _, err := NewStablecoin(newFakeStablecoinAllocator()); !errors.Is(err, ErrStablecoinNotConfigured) {
		t.Fatalf("expected ErrStablecoinNotConfigured, got %v", err)
	}

	t.Setenv(EnvStablecoinIndexerBaseURL, "https://api.etherscan.io/api")
	t.Setenv(EnvStablecoinIndexerAPIKey, "key")
	t.Setenv(EnvStablecoinTokenContract, "0xContract")
	t.Setenv(EnvStablecoinTokenDecimals, "6")
	t.Setenv(EnvStablecoinMinConfirmations, "12")
	p, err := NewStablecoin(newFakeStablecoinAllocator())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if p.Name() != ProviderNameStablecoin {
		t.Fatalf("unexpected name: %s", p.Name())
	}
	if p.quoteCurrency != stablecoinDefaultQuoteCurrency {
		t.Fatalf("expected default quote currency, got %s", p.quoteCurrency)
	}
}

func TestStablecoinCapabilities(t *testing.T) {
	p := &StablecoinProvider{quoteCurrency: "USD"}
	caps := p.Capabilities()
	if caps.Flow != FlowInvoice {
		t.Fatalf("expected FlowInvoice, got %v", caps.Flow)
	}
	if caps.Webhooks {
		t.Fatal("expected Webhooks capability false (poll-only)")
	}
}
