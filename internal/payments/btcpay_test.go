package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestBTCPay(t *testing.T, srv *httptest.Server) *BTCPayProvider {
	t.Helper()
	return &BTCPayProvider{
		baseURL:       srv.URL,
		storeID:       "store1",
		apiKey:        "test-api-key",
		webhookSecret: "test-webhook-secret",
		httpClient:    srv.Client(),
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestBTCPayBegin_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "token test-api-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["amount"] != "12.34" {
			t.Fatalf("expected amount 12.34, got %v", body["amount"])
		}
		if body["currency"] != "USD" {
			t.Fatalf("expected currency USD, got %v", body["currency"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"inv_123","storeId":"store1","amount":"12.34","currency":"USD","status":"New","additionalStatus":"None","checkoutLink":"https://btcpay.example.com/i/inv_123","expirationTime":9999999999}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	charge, err := p.Begin(context.Background(), Order{
		Reference:   "order_1",
		AmountMinor: 1234,
		Currency:    "USD",
		EventID:     "evt_1",
	})
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if charge.Reference != "inv_123" {
		t.Fatalf("expected reference inv_123, got %q", charge.Reference)
	}
	if charge.RedirectURL != "https://btcpay.example.com/i/inv_123" {
		t.Fatalf("unexpected redirect url: %q", charge.RedirectURL)
	}
	if !strings.Contains(charge.Instructions, "2286-11-20") {
		t.Fatalf("expected instructions to mention the expiry timestamp, got: %q", charge.Instructions)
	}
}

func TestBTCPayBegin_RejectsNonPositiveAmount(t *testing.T) {
	p := &BTCPayProvider{baseURL: "http://unused", storeID: "s", apiKey: "k", webhookSecret: "w", httpClient: http.DefaultClient}
	if _, err := p.Begin(context.Background(), Order{Reference: "o1", AmountMinor: 0, Currency: "USD"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := p.Begin(context.Background(), Order{Reference: "o1", AmountMinor: -100, Currency: "USD"}); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestBTCPayVerify_SettledMapsToPaid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_123","amount":"12.34","currency":"USD","status":"Settled","additionalStatus":"None"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	result, err := p.Verify(context.Background(), "inv_123")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
	if result.AmountMinor != 1234 {
		t.Fatalf("expected 1234 minor units, got %d", result.AmountMinor)
	}
	if result.Currency != "USD" {
		t.Fatalf("expected USD, got %s", result.Currency)
	}
	if result.EventID != "inv_123" {
		t.Fatalf("expected event id inv_123, got %s", result.EventID)
	}
}

func TestBTCPayVerify_UnderpaymentStaysNotPaid(t *testing.T) {
	for _, status := range []string{"New", "Processing"} {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/stores/store1/invoices/inv_under", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"inv_under","amount":"12.34","currency":"USD","status":"` + status + `","additionalStatus":"None"}`))
		})
		srv := httptest.NewServer(mux)

		p := newTestBTCPay(t, srv)
		result, err := p.Verify(context.Background(), "inv_under")
		srv.Close()
		if err != nil {
			t.Fatalf("status=%s: Verify failed: %v", status, err)
		}
		if result.Status == StatusPaid {
			t.Fatalf("status=%s: an unsettled invoice must never report StatusPaid", status)
		}
	}
}

func TestBTCPayVerify_ExpiredIsFailedNeverPaid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_exp", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_exp","amount":"12.34","currency":"USD","status":"Expired","additionalStatus":"None"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	result, err := p.Verify(context.Background(), "inv_exp")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("expected StatusFailed for an expired quote, got %v", result.Status)
	}
}

func TestBTCPayVerify_InvalidUnderpaidNeverSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_partial", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_partial","amount":"12.34","currency":"USD","status":"Invalid","additionalStatus":"PaidPartial"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	result, err := p.Verify(context.Background(), "inv_partial")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("an underpaid+invalid invoice must never be reported as paid, got %v", result.Status)
	}
}

func TestBTCPayVerify_OverpaidIsFlaggedNotAccepted(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_over", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_over","amount":"12.34","currency":"USD","status":"Settled","additionalStatus":"PaidOver"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	_, err := p.Verify(context.Background(), "inv_over")
	if err == nil {
		t.Fatal("expected an error for an overpaid invoice, got nil (silently accepted)")
	}
	if !errors.Is(err, ErrBTCPayOverpaid) {
		t.Fatalf("expected ErrBTCPayOverpaid, got %v", err)
	}
}

func TestBTCPayVerify_InconsistentSettledPartialFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_weird", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_weird","amount":"12.34","currency":"USD","status":"Settled","additionalStatus":"PaidPartial"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	_, err := p.Verify(context.Background(), "inv_weird")
	if err == nil {
		t.Fatal("expected an error for an inconsistent Settled+PaidPartial invoice")
	}
}

func TestBTCPayVerify_MalformedAmountFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_bad", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_bad","amount":"not-a-number","currency":"USD","status":"Settled","additionalStatus":"None"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	if _, err := p.Verify(context.Background(), "inv_bad"); err == nil {
		t.Fatal("expected error for a malformed amount string")
	}
}

func TestBTCPayVerify_TooMuchPrecisionRejected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_prec", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_prec","amount":"12.345","currency":"USD","status":"Settled","additionalStatus":"None"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	if _, err := p.Verify(context.Background(), "inv_prec"); err == nil {
		t.Fatal("expected error: USD only has 2 decimals, 12.345 has 3 and must not be silently rounded")
	}
}

func TestBTCPayVerify_NonJSONResponseFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_bad2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	if _, err := p.Verify(context.Background(), "inv_bad2"); err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
}

func TestBTCPayVerify_ServerErrorFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_500", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal error"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	if _, err := p.Verify(context.Background(), "inv_500"); err == nil {
		t.Fatal("expected error for a 500 response")
	}
}

func TestBTCPayVerify_TimeoutFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Verify(ctx, "inv_slow"); err == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestBTCPayWebhook_ValidSignatureRefetchesAndSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_wh", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_wh","amount":"12.34","currency":"USD","status":"Settled","additionalStatus":"None"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	body := []byte(`{"type":"InvoiceSettled","invoiceId":"inv_wh","storeId":"store1"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(body)))
	req.Header.Set("BTCPay-Sig", sign(p.webhookSecret, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
	if result.AmountMinor != 1234 || result.Currency != "USD" {
		t.Fatalf("unexpected amount/currency: %d %s", result.AmountMinor, result.Currency)
	}
}

func TestBTCPayWebhook_MissingSignatureRejected(t *testing.T) {
	p := newTestBTCPay(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"type":"InvoiceSettled","invoiceId":"inv_wh"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(body)))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrBTCPayMissingSignature) {
		t.Fatalf("expected ErrBTCPayMissingSignature, got %v", err)
	}
}

func TestBTCPayWebhook_TamperedSignatureRejected(t *testing.T) {
	p := newTestBTCPay(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"type":"InvoiceSettled","invoiceId":"inv_wh"}`)
	tamperedBody := []byte(`{"type":"InvoiceSettled","invoiceId":"inv_wh_evil"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(tamperedBody)))
	// Signature computed over the ORIGINAL body, but the request carries a
	// tampered body — signature must not validate.
	req.Header.Set("BTCPay-Sig", sign(p.webhookSecret, body))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrBTCPayInvalidSignature) {
		t.Fatalf("expected ErrBTCPayInvalidSignature, got %v", err)
	}
}

func TestBTCPayWebhook_WrongSecretRejected(t *testing.T) {
	p := newTestBTCPay(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"type":"InvoiceSettled","invoiceId":"inv_wh"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(body)))
	req.Header.Set("BTCPay-Sig", sign("wrong-secret", body))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrBTCPayInvalidSignature) {
		t.Fatalf("expected ErrBTCPayInvalidSignature, got %v", err)
	}
}

func TestBTCPayWebhook_MalformedJSONRejected(t *testing.T) {
	p := newTestBTCPay(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`not json at all`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(body)))
	req.Header.Set("BTCPay-Sig", sign(p.webhookSecret, body))

	if _, err := p.Webhook(context.Background(), req); err == nil {
		t.Fatal("expected error for malformed JSON body")
	}
}

func TestBTCPayWebhook_UnhandledEventType(t *testing.T) {
	p := newTestBTCPay(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"type":"SomeUnrelatedEvent","invoiceId":"inv_wh"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(body)))
	req.Header.Set("BTCPay-Sig", sign(p.webhookSecret, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("expected ErrUnhandledEvent, got %v", err)
	}
}

func TestBTCPayWebhook_OverpaidFlaggedViaRefetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_over_wh", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_over_wh","amount":"12.34","currency":"USD","status":"Settled","additionalStatus":"PaidOver"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	body := []byte(`{"type":"InvoiceSettled","invoiceId":"inv_over_wh"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(body)))
	req.Header.Set("BTCPay-Sig", sign(p.webhookSecret, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrBTCPayOverpaid) {
		t.Fatalf("expected ErrBTCPayOverpaid, got %v", err)
	}
}

func TestBTCPayWebhook_SpoofedBodyCannotFabricateSettlement(t *testing.T) {
	// Even if an attacker somehow got a validly-signed webhook through
	// (e.g. secret leaked) claiming settlement, the adapter never trusts
	// webhook body fields for the actual amount/status — it always
	// refetches from BTCPay. Here the "real" invoice is still New, so even
	// a signed "InvoiceSettled" webhook must not report StatusPaid.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stores/store1/invoices/inv_not_really", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_not_really","amount":"12.34","currency":"USD","status":"New","additionalStatus":"None"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestBTCPay(t, srv)
	body := []byte(`{"type":"InvoiceSettled","invoiceId":"inv_not_really"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/btcpay", strings.NewReader(string(body)))
	req.Header.Set("BTCPay-Sig", sign(p.webhookSecret, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("webhook body claimed settlement but the refetched invoice is still New — must not report StatusPaid")
	}
}

func TestNewBTCPay_RequiresAllEnvVars(t *testing.T) {
	t.Setenv(EnvBTCPayBaseURL, "")
	t.Setenv(EnvBTCPayAPIKey, "")
	t.Setenv(EnvBTCPayStoreID, "")
	t.Setenv(EnvBTCPayWebhookSecret, "")
	if _, err := NewBTCPay(); !errors.Is(err, ErrBTCPayNotConfigured) {
		t.Fatalf("expected ErrBTCPayNotConfigured, got %v", err)
	}

	t.Setenv(EnvBTCPayBaseURL, "https://btcpay.example.com")
	t.Setenv(EnvBTCPayAPIKey, "key")
	t.Setenv(EnvBTCPayStoreID, "store1")
	t.Setenv(EnvBTCPayWebhookSecret, "secret")
	p, err := NewBTCPay()
	if err != nil {
		t.Fatalf("expected success once all env vars are set, got %v", err)
	}
	if p.Name() != ProviderNameBTCPay {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestBTCPayCapabilities(t *testing.T) {
	p := &BTCPayProvider{}
	caps := p.Capabilities()
	if caps.Flow != FlowInvoice {
		t.Fatalf("expected FlowInvoice, got %v", caps.Flow)
	}
	if !caps.Webhooks {
		t.Fatal("expected Webhooks capability true")
	}
	if !caps.ZeroDecimalOK {
		t.Fatal("expected ZeroDecimalOK true")
	}
}
