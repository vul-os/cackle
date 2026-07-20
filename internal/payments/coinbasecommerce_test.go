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

func newTestCoinbaseCommerce(t *testing.T, srv *httptest.Server) *CoinbaseCommerceProvider {
	t.Helper()
	return &CoinbaseCommerceProvider{
		baseURL:       srv.URL,
		apiKey:        "test-api-key",
		webhookSecret: "test-webhook-secret",
		httpClient:    srv.Client(),
	}
}

func signCoinbaseCommerce(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestCoinbaseCommerceBegin_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("X-CC-Api-Key"); got != "test-api-key" {
			t.Fatalf("unexpected X-CC-Api-Key header: %q", got)
		}
		if got := r.Header.Get("X-CC-Version"); got == "" {
			t.Fatal("expected X-CC-Version header to be set")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		localPrice, ok := body["local_price"].(map[string]any)
		if !ok || localPrice["amount"] != "12.34" || localPrice["currency"] != "USD" {
			t.Fatalf("unexpected local_price: %v", body["local_price"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","hosted_url":"https://commerce.coinbase.com/charges/charge_1","timeline":[{"time":"2024-01-01T00:00:00Z","status":"NEW"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	charge, err := p.Begin(context.Background(), Order{Reference: "order_1", AmountMinor: 1234, Currency: "USD"})
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if charge.Reference != "charge_1" {
		t.Fatalf("expected reference charge_1, got %q", charge.Reference)
	}
	if charge.RedirectURL != "https://commerce.coinbase.com/charges/charge_1" {
		t.Fatalf("unexpected redirect url: %q", charge.RedirectURL)
	}
}

func TestCoinbaseCommerceBegin_RejectsNonPositiveAmount(t *testing.T) {
	p := &CoinbaseCommerceProvider{baseURL: "http://unused", apiKey: "k", webhookSecret: "w", httpClient: http.DefaultClient}
	if _, err := p.Begin(context.Background(), Order{Reference: "o1", AmountMinor: 0, Currency: "USD"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestCoinbaseCommerceVerify_CompletedMapsToPaid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[{"status":"NEW"},{"status":"PENDING"},{"status":"COMPLETED"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	result, err := p.Verify(context.Background(), "charge_1")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
	if result.AmountMinor != 1234 {
		t.Fatalf("expected 1234 minor units, got %d", result.AmountMinor)
	}
}

func TestCoinbaseCommerceVerify_PendingStaysNotPaid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[{"status":"NEW"},{"status":"PENDING"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	result, err := p.Verify(context.Background(), "charge_1")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("a PENDING charge must never report StatusPaid")
	}
}

func TestCoinbaseCommerceVerify_ExpiredNeverSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[{"status":"NEW"},{"status":"EXPIRED"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	result, err := p.Verify(context.Background(), "charge_1")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("expected StatusFailed for an expired quote, got %v", result.Status)
	}
}

func TestCoinbaseCommerceVerify_UnresolvedIsFlaggedNotAccepted(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[{"status":"NEW"},{"status":"UNRESOLVED","context":"OVERPAID"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	_, err := p.Verify(context.Background(), "charge_1")
	if !errors.Is(err, ErrCoinbaseCommerceRequiresManualReview) {
		t.Fatalf("expected ErrCoinbaseCommerceRequiresManualReview, got %v", err)
	}
	if !strings.Contains(err.Error(), "OVERPAID") {
		t.Fatalf("expected the context to be surfaced in the error, got: %v", err)
	}
}

func TestCoinbaseCommerceVerify_UnrecognisedStatusFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[{"status":"SOME_FUTURE_STATUS"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	if _, err := p.Verify(context.Background(), "charge_1"); err == nil {
		t.Fatal("expected error for an unrecognised timeline status")
	}
}

func TestCoinbaseCommerceVerify_EmptyTimelineFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	if _, err := p.Verify(context.Background(), "charge_1"); err == nil {
		t.Fatal("expected error for an empty timeline")
	}
}

func TestCoinbaseCommerceVerify_MalformedJSONFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	if _, err := p.Verify(context.Background(), "charge_1"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestCoinbaseCommerceVerify_ServerErrorFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	if _, err := p.Verify(context.Background(), "charge_1"); err == nil {
		t.Fatal("expected error for a 500 response")
	}
}

func TestCoinbaseCommerceVerify_TimeoutFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Verify(ctx, "charge_1"); err == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestCoinbaseCommerceWebhook_ValidSignatureRefetchesAndSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[{"status":"COMPLETED"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	body := []byte(`{"event":{"type":"charge:confirmed","data":{"id":"charge_1"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/coinbasecommerce", strings.NewReader(string(body)))
	req.Header.Set("X-CC-Webhook-Signature", signCoinbaseCommerce(p.webhookSecret, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
}

func TestCoinbaseCommerceWebhook_MissingSignatureRejected(t *testing.T) {
	p := newTestCoinbaseCommerce(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"event":{"type":"charge:confirmed","data":{"id":"charge_1"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/coinbasecommerce", strings.NewReader(string(body)))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrCoinbaseCommerceMissingSignature) {
		t.Fatalf("expected ErrCoinbaseCommerceMissingSignature, got %v", err)
	}
}

func TestCoinbaseCommerceWebhook_TamperedSignatureRejected(t *testing.T) {
	p := newTestCoinbaseCommerce(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"event":{"type":"charge:confirmed","data":{"id":"charge_1"}}}`)
	tampered := []byte(`{"event":{"type":"charge:confirmed","data":{"id":"charge_evil"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/coinbasecommerce", strings.NewReader(string(tampered)))
	req.Header.Set("X-CC-Webhook-Signature", signCoinbaseCommerce(p.webhookSecret, body))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrCoinbaseCommerceInvalidSignature) {
		t.Fatalf("expected ErrCoinbaseCommerceInvalidSignature, got %v", err)
	}
}

func TestCoinbaseCommerceWebhook_WrongSecretRejected(t *testing.T) {
	p := newTestCoinbaseCommerce(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"event":{"type":"charge:confirmed","data":{"id":"charge_1"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/coinbasecommerce", strings.NewReader(string(body)))
	req.Header.Set("X-CC-Webhook-Signature", signCoinbaseCommerce("wrong-secret", body))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrCoinbaseCommerceInvalidSignature) {
		t.Fatalf("expected ErrCoinbaseCommerceInvalidSignature, got %v", err)
	}
}

func TestCoinbaseCommerceWebhook_MalformedJSONRejected(t *testing.T) {
	p := newTestCoinbaseCommerce(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`not json at all`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/coinbasecommerce", strings.NewReader(string(body)))
	req.Header.Set("X-CC-Webhook-Signature", signCoinbaseCommerce(p.webhookSecret, body))

	if _, err := p.Webhook(context.Background(), req); err == nil {
		t.Fatal("expected error for malformed JSON body")
	}
}

func TestCoinbaseCommerceWebhook_UnhandledEventType(t *testing.T) {
	p := newTestCoinbaseCommerce(t, httptest.NewServer(http.NotFoundHandler()))
	body := []byte(`{"event":{"type":"some:other:event","data":{"id":"charge_1"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/coinbasecommerce", strings.NewReader(string(body)))
	req.Header.Set("X-CC-Webhook-Signature", signCoinbaseCommerce(p.webhookSecret, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("expected ErrUnhandledEvent, got %v", err)
	}
}

func TestCoinbaseCommerceWebhook_SpoofedBodyCannotFabricateSettlement(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/charges/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","timeline":[{"status":"NEW"}],"pricing":{"local":{"amount":"12.34","currency":"USD"}}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestCoinbaseCommerce(t, srv)
	body := []byte(`{"event":{"type":"charge:confirmed","data":{"id":"charge_1","timeline":[{"status":"COMPLETED"}]}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/coinbasecommerce", strings.NewReader(string(body)))
	req.Header.Set("X-CC-Webhook-Signature", signCoinbaseCommerce(p.webhookSecret, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("webhook body embedded a COMPLETED timeline but the refetched charge is still NEW — must not report StatusPaid")
	}
}

func TestNewCoinbaseCommerce_RequiresEnvVars(t *testing.T) {
	t.Setenv(EnvCoinbaseCommerceAPIKey, "")
	t.Setenv(EnvCoinbaseCommerceWebhookSecret, "")
	if _, err := NewCoinbaseCommerce(); !errors.Is(err, ErrCoinbaseCommerceNotConfigured) {
		t.Fatalf("expected ErrCoinbaseCommerceNotConfigured, got %v", err)
	}

	t.Setenv(EnvCoinbaseCommerceAPIKey, "key")
	t.Setenv(EnvCoinbaseCommerceWebhookSecret, "secret")
	p, err := NewCoinbaseCommerce()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if p.Name() != ProviderNameCoinbaseCommerce {
		t.Fatalf("unexpected name: %s", p.Name())
	}
	if p.baseURL != coinbaseCommerceDefaultBaseURL {
		t.Fatalf("expected default base URL, got %s", p.baseURL)
	}
}

func TestCoinbaseCommerceCapabilities(t *testing.T) {
	p := &CoinbaseCommerceProvider{}
	caps := p.Capabilities()
	if caps.Flow != FlowInvoice {
		t.Fatalf("expected FlowInvoice, got %v", caps.Flow)
	}
}
