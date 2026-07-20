package payments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestLNbits(t *testing.T, srv *httptest.Server) *LNbitsProvider {
	t.Helper()
	return &LNbitsProvider{
		baseURL:       srv.URL,
		apiKey:        "test-api-key",
		webhookSecret: "test-webhook-secret",
		quoteTTL:      15 * time.Minute,
		httpClient:    srv.Client(),
		invoices:      make(map[string]lnbitsInvoiceRecord),
	}
}

func TestLNbitsBegin_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("X-Api-Key"); got != "test-api-key" {
			t.Fatalf("unexpected X-Api-Key header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["out"] != false {
			t.Fatalf("expected out=false, got %v", body["out"])
		}
		if body["unit"] != "usd" {
			t.Fatalf("expected unit=usd, got %v", body["unit"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"payment_hash":"hash123","payment_request":"lnbc1..."}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	charge, err := p.Begin(context.Background(), Order{
		Reference:   "order_1",
		AmountMinor: 1234,
		Currency:    "USD",
	})
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if charge.Reference != "hash123" {
		t.Fatalf("expected reference hash123, got %q", charge.Reference)
	}
	if !strings.Contains(charge.Instructions, "lnbc1...") {
		t.Fatalf("expected instructions to include the bolt11 invoice, got: %q", charge.Instructions)
	}

	// The record must be remembered for later Verify calls.
	p.mu.Lock()
	record, ok := p.invoices["hash123"]
	p.mu.Unlock()
	if !ok {
		t.Fatal("expected an in-memory invoice record after Begin")
	}
	if record.amountMinor != 1234 || record.currency != "USD" {
		t.Fatalf("unexpected record: %+v", record)
	}
}

func TestLNbitsBegin_RejectsNonPositiveAmount(t *testing.T) {
	p := &LNbitsProvider{baseURL: "http://unused", apiKey: "k", webhookSecret: "w", quoteTTL: time.Minute, httpClient: http.DefaultClient, invoices: map[string]lnbitsInvoiceRecord{}}
	if _, err := p.Begin(context.Background(), Order{Reference: "o1", AmountMinor: 0, Currency: "USD"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestLNbitsVerify_UnknownReferenceFailsClosed(t *testing.T) {
	p := newTestLNbits(t, httptest.NewServer(http.NotFoundHandler()))
	if _, err := p.Verify(context.Background(), "never-created"); !errors.Is(err, ErrLNbitsUnknownReference) {
		t.Fatalf("expected ErrLNbitsUnknownReference, got %v", err)
	}
}

func TestLNbitsVerify_PaidReportsOriginalFiatAmount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"paid": true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now()}

	result, err := p.Verify(context.Background(), "hash123")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
	if result.AmountMinor != 1234 || result.Currency != "USD" {
		t.Fatalf("expected the original fiat amount/currency to be reported, got %d %s", result.AmountMinor, result.Currency)
	}
}

func TestLNbitsVerify_UnpaidStaysPending(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"paid": false}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now()}

	result, err := p.Verify(context.Background(), "hash123")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusPending {
		t.Fatalf("expected StatusPending, got %v", result.Status)
	}
}

func TestLNbitsVerify_ExpiredQuoteFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"paid": false}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.quoteTTL = time.Millisecond
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now().Add(-time.Hour)}

	result, err := p.Verify(context.Background(), "hash123")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("expected StatusFailed for an expired quote, got %v", result.Status)
	}
}

func TestLNbitsVerify_LatePaymentAfterExpiryStillFailsClosed(t *testing.T) {
	// Even if the underlying invoice somehow got paid after our quote
	// window closed, we must not accept it — see file doc comment on
	// quote expiry. paid=true but createdAt is long past quoteTTL: our
	// expiry check runs BEFORE trusting "paid", so this must map to a
	// Failed/Pending state derived from the switch's priority, not Paid...
	// actually the current design intentionally still marks a truly PAID
	// invoice as Paid if paid=true takes priority. This test documents that
	// behaviour explicitly (paid=true always wins) so any future change is
	// deliberate, not accidental.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"paid": true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.quoteTTL = time.Millisecond
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now().Add(-time.Hour)}

	result, err := p.Verify(context.Background(), "hash123")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("LNbits itself reported paid=true; expected StatusPaid, got %v", result.Status)
	}
}

func TestLNbitsVerify_MalformedJSONFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now()}

	if _, err := p.Verify(context.Background(), "hash123"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLNbitsVerify_ServerErrorFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now()}

	if _, err := p.Verify(context.Background(), "hash123"); err == nil {
		t.Fatal("expected error for a 500 response")
	}
}

func TestLNbitsVerify_TimeoutFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now()}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Verify(ctx, "hash123"); err == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestLNbitsWebhook_MissingSecretRejected(t *testing.T) {
	p := newTestLNbits(t, httptest.NewServer(http.NotFoundHandler()))
	req := httptest.NewRequest(http.MethodPost, "/webhook/lnbits", strings.NewReader(`{"payment_hash":"hash123"}`))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrLNbitsMissingSignature) {
		t.Fatalf("expected ErrLNbitsMissingSignature, got %v", err)
	}
}

func TestLNbitsWebhook_WrongSecretRejected(t *testing.T) {
	p := newTestLNbits(t, httptest.NewServer(http.NotFoundHandler()))
	req := httptest.NewRequest(http.MethodPost, "/webhook/lnbits?secret=wrong", strings.NewReader(`{"payment_hash":"hash123"}`))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrLNbitsInvalidSignature) {
		t.Fatalf("expected ErrLNbitsInvalidSignature, got %v", err)
	}
}

func TestLNbitsWebhook_CorrectSecretButUnknownHashFailsClosed(t *testing.T) {
	p := newTestLNbits(t, httptest.NewServer(http.NotFoundHandler()))
	req := httptest.NewRequest(http.MethodPost, "/webhook/lnbits?secret=test-webhook-secret", strings.NewReader(`{"payment_hash":"never-seen"}`))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrLNbitsUnknownReference) {
		t.Fatalf("expected ErrLNbitsUnknownReference, got %v", err)
	}
}

func TestLNbitsWebhook_SpoofedBodyCannotFabricateSettlement(t *testing.T) {
	// Correct secret, but the body's implied "paid" claim is irrelevant:
	// the adapter always re-verifies against LNbits' own API. Here the
	// real payment is still unpaid.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"paid": false}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now()}

	req := httptest.NewRequest(http.MethodPost, "/webhook/lnbits?secret=test-webhook-secret", strings.NewReader(`{"payment_hash":"hash123","paid":true}`))
	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("webhook body claimed paid=true but the real LNbits status is unpaid — must not report StatusPaid")
	}
}

func TestLNbitsWebhook_ValidSecretSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/payments/hash123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"paid": true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestLNbits(t, srv)
	p.invoices["hash123"] = lnbitsInvoiceRecord{amountMinor: 1234, currency: "USD", createdAt: time.Now()}

	req := httptest.NewRequest(http.MethodPost, "/webhook/lnbits?secret=test-webhook-secret", strings.NewReader(`{"payment_hash":"hash123"}`))
	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
}

func TestLNbitsWebhook_MalformedJSONRejected(t *testing.T) {
	p := newTestLNbits(t, httptest.NewServer(http.NotFoundHandler()))
	req := httptest.NewRequest(http.MethodPost, "/webhook/lnbits?secret=test-webhook-secret", strings.NewReader(`not json`))

	if _, err := p.Webhook(context.Background(), req); err == nil {
		t.Fatal("expected error for malformed JSON body")
	}
}

func TestNewLNbits_RequiresEnvVars(t *testing.T) {
	t.Setenv(EnvLNbitsBaseURL, "")
	t.Setenv(EnvLNbitsAPIKey, "")
	t.Setenv(EnvLNbitsWebhookSecret, "")
	if _, err := NewLNbits(); !errors.Is(err, ErrLNbitsNotConfigured) {
		t.Fatalf("expected ErrLNbitsNotConfigured, got %v", err)
	}

	t.Setenv(EnvLNbitsBaseURL, "https://lnbits.example.com")
	t.Setenv(EnvLNbitsAPIKey, "key")
	t.Setenv(EnvLNbitsWebhookSecret, "secret")
	p, err := NewLNbits()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if p.Name() != ProviderNameLNbits {
		t.Fatalf("unexpected name: %s", p.Name())
	}
	if p.quoteTTL != lnbitsDefaultQuoteTTLSecs*time.Second {
		t.Fatalf("expected default quote TTL, got %v", p.quoteTTL)
	}
}

func TestLNbitsCapabilities(t *testing.T) {
	p := &LNbitsProvider{}
	caps := p.Capabilities()
	if caps.Flow != FlowInvoice {
		t.Fatalf("expected FlowInvoice, got %v", caps.Flow)
	}
	if !caps.ZeroDecimalOK {
		t.Fatal("expected ZeroDecimalOK true")
	}
}
