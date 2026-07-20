package payments

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestMollieProvider(ts *httptest.Server) *MollieProvider {
	return &MollieProvider{
		apiKey:     "test_apikey",
		webhookURL: "https://example.com/webhooks/mollie",
		httpClient: ts.Client(),
		baseURL:    ts.URL,
	}
}

func TestNewMollie_RequiresAPIKeyAndWebhookURL(t *testing.T) {
	t.Setenv(EnvMollieAPIKey, "")
	t.Setenv(EnvMollieWebhookURL, "")
	if _, err := NewMollie(); !errors.Is(err, ErrMollieAPIKeyNotConfigured) {
		t.Fatalf("err = %v, want ErrMollieAPIKeyNotConfigured", err)
	}
	t.Setenv(EnvMollieAPIKey, "test_x")
	if _, err := NewMollie(); !errors.Is(err, ErrMollieWebhookURLMissing) {
		t.Fatalf("err = %v, want ErrMollieWebhookURLMissing", err)
	}
	t.Setenv(EnvMollieWebhookURL, "https://example.com/hook")
	p, err := NewMollie()
	if err != nil {
		t.Fatalf("NewMollie() = %v, want nil", err)
	}
	if p.Name() != ProviderNameMollie {
		t.Fatalf("Name() = %q", p.Name())
	}
}

// --- currency formatting --------------------------------------------------

func TestMollieAmountValue_OrdinaryCurrency(t *testing.T) {
	got, err := mollieAmountValue(5000, "EUR")
	if err != nil || got != "50.00" {
		t.Fatalf("mollieAmountValue(5000, EUR) = %q, %v; want \"50.00\", nil", got, err)
	}
}

func TestMollieAmountValue_ZeroDecimalCurrencyRefused(t *testing.T) {
	_, err := mollieAmountValue(1000, "JPY")
	if !errors.Is(err, ErrMollieUnsupportedCurrency) {
		t.Fatalf("mollieAmountValue(1000, JPY) err = %v, want ErrMollieUnsupportedCurrency", err)
	}
}

func TestMollieAmountValue_ThreeDecimalCurrencyRefused(t *testing.T) {
	_, err := mollieAmountValue(1000, "KWD")
	if !errors.Is(err, ErrMollieUnsupportedCurrency) {
		t.Fatalf("mollieAmountValue(1000, KWD) err = %v, want ErrMollieUnsupportedCurrency", err)
	}
}

func TestMollieAmountValueToMinor_RoundTrips(t *testing.T) {
	got, err := mollieAmountValueToMinor("50.00", "EUR")
	if err != nil || got != 5000 {
		t.Fatalf("mollieAmountValueToMinor(50.00, EUR) = %d, %v; want 5000, nil", got, err)
	}
}

// --- Begin --------------------------------------------------------------

func TestMollieBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payments" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test_apikey" {
			t.Fatalf("missing bearer token")
		}
		w.Write([]byte(`{"id":"tr_test1","_links":{"checkout":{"href":"https://www.mollie.com/checkout/tr_test1"}}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	charge, err := p.Begin(context.Background(), Order{
		Reference:   "ord_1",
		AmountMinor: 5000,
		Currency:    "EUR",
		CallbackURL: "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL != "https://www.mollie.com/checkout/tr_test1" {
		t.Fatalf("RedirectURL = %q", charge.RedirectURL)
	}
	if charge.Reference != "ord_1" {
		t.Fatalf("Reference = %q, want ord_1", charge.Reference)
	}
}

func TestMollieBegin_RefusesZeroDecimalCurrency(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "JPY", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrMollieUnsupportedCurrency) {
		t.Fatalf("Begin() err = %v, want ErrMollieUnsupportedCurrency", err)
	}
}

func TestMollieBegin_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":500,"title":"Internal Server Error","detail":"boom"}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "EUR", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrMollieUnexpectedStatus) {
		t.Fatalf("Begin() err = %v, want ErrMollieUnexpectedStatus", err)
	}
}

func TestMollieBegin_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "EUR", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrMollieMalformedResponse) {
		t.Fatalf("Begin() err = %v, want ErrMollieMalformedResponse", err)
	}
}

func TestMollieBegin_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Begin(ctx, Order{Reference: "ord_1", AmountMinor: 5000, Currency: "EUR", CallbackURL: "https://example.com"})
	if err == nil {
		t.Fatal("Begin() = nil error, want timeout error")
	}
}

// --- Verify ---------------------------------------------------------------

func TestMollieVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/payments/tr_test1") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":"tr_test1","status":"paid","amount":{"currency":"EUR","value":"50.00"},"metadata":{"cackle_reference":"ord_1"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	result, err := p.Verify(context.Background(), "tr_test1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 5000 || result.Currency != "EUR" || result.Reference != "ord_1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMollieVerify_OpenIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"tr_test1","status":"open","amount":{"currency":"EUR","value":"50.00"},"metadata":{"cackle_reference":"ord_1"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	result, err := p.Verify(context.Background(), "tr_test1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for status=open, want failed")
	}
}

func TestMollieVerify_MissingMetadataReferenceFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"tr_test1","status":"paid","amount":{"currency":"EUR","value":"50.00"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	_, err := p.Verify(context.Background(), "tr_test1")
	if !errors.Is(err, ErrMollieMalformedResponse) {
		t.Fatalf("Verify() err = %v, want ErrMollieMalformedResponse (no reference to reconcile against)", err)
	}
}

func TestMollieVerify_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	_, err := p.Verify(context.Background(), "tr_test1")
	if !errors.Is(err, ErrMollieUnexpectedStatus) {
		t.Fatalf("Verify() err = %v, want ErrMollieUnexpectedStatus", err)
	}
}

func TestMollieVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	_, err := p.Verify(context.Background(), "tr_test1")
	if !errors.Is(err, ErrMollieMalformedResponse) {
		t.Fatalf("Verify() err = %v, want ErrMollieMalformedResponse", err)
	}
}

func TestMollieVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Verify(ctx, "tr_test1")
	if err == nil {
		t.Fatal("Verify() = nil error, want timeout error")
	}
}

// --- Webhook --------------------------------------------------------------
//
// Mollie's classic webhook has no signature at all — "verification" is the
// authenticated re-fetch of the payment by id, exactly what Verify already
// does. These tests exercise that specific contract: the webhook body's
// `id` is never trusted for anything but which payment to go look up.

func TestMollieWebhook_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/payments/tr_test1") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":"tr_test1","status":"paid","amount":{"currency":"EUR","value":"50.00"},"metadata":{"cackle_reference":"ord_1"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_test1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 5000 || result.Currency != "EUR" || result.Reference != "ord_1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMollieWebhook_MissingIDFailsClosed(t *testing.T) {
	p := newTestMollieProvider(httptest.NewServer(nil))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMollieMissingID) {
		t.Fatalf("Webhook() err = %v, want ErrMollieMissingID", err)
	}
}

func TestMollieWebhook_ForgedIDNeverAcceptsWithoutRealPayment(t *testing.T) {
	// A "forged" webhook call (attacker-supplied id) can, at most, make
	// this adapter go check that id's real status — it can never
	// fabricate a paid result, because Webhook defers entirely to the
	// authenticated API lookup. Here the fake server reports the payment
	// as still open, proving the webhook body's claims (there are none
	// beyond the id) are never trusted directly.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"tr_forged","status":"open","amount":{"currency":"EUR","value":"999999.00"},"metadata":{"cackle_reference":"ord_1"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_forged"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil error (open is a valid non-settlement result)", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for a payment the API reports as open, want failed")
	}
}

func TestMollieWebhook_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_test1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMollieUnexpectedStatus) {
		t.Fatalf("Webhook() err = %v, want ErrMollieUnexpectedStatus", err)
	}
}

func TestMollieWebhook_MalformedAPIResponseFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_test1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMollieMalformedResponse) {
		t.Fatalf("Webhook() err = %v, want ErrMollieMalformedResponse", err)
	}
}

func TestMollieWebhook_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_test1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := p.Webhook(ctx, req)
	if err == nil {
		t.Fatal("Webhook() = nil error, want timeout error")
	}
}

func TestMollieWebhook_AmountMismatchFailsClosedViaReconcile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"tr_test1","status":"paid","amount":{"currency":"EUR","value":"50.00"},"metadata":{"cackle_reference":"ord_1"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_test1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 999900, Currency: "EUR"}); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrAmountMismatch", err)
	}
}

func TestMollieWebhook_CurrencyMismatchFailsClosedViaReconcile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"tr_test1","status":"paid","amount":{"currency":"EUR","value":"50.00"},"metadata":{"cackle_reference":"ord_1"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_test1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 5000, Currency: "USD"}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrCurrencyMismatch", err)
	}
}

func TestMollieWebhook_ReplayedEventProducesStableEventID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"tr_test1","status":"paid","amount":{"currency":"EUR","value":"50.00"},"metadata":{"cackle_reference":"ord_1"}}`))
	}))
	defer ts.Close()
	p := newTestMollieProvider(ts)

	var ids []string
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/mollie", strings.NewReader("id=tr_test1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		result, err := p.Webhook(context.Background(), req)
		if err != nil {
			t.Fatalf("Webhook() call %d = %v", i, err)
		}
		ids = append(ids, result.EventID)
	}
	if ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("EventIDs = %v, want equal and non-empty", ids)
	}
}
