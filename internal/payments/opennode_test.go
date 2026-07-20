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
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestOpenNode(t *testing.T, srv *httptest.Server) *OpenNodeProvider {
	t.Helper()
	return &OpenNodeProvider{
		baseURL:    srv.URL,
		apiKey:     "test-api-key",
		httpClient: srv.Client(),
	}
}

func signOpenNode(apiKey, chargeID string) string {
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(chargeID))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestOpenNodeBegin_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charges", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "test-api-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["amount"] != 12.34 {
			t.Fatalf("expected amount 12.34, got %v", body["amount"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"unpaid","amount":12.34,"currency":"USD","hosted_checkout_url":"https://checkout.opennode.com/charge_1"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	charge, err := p.Begin(context.Background(), Order{Reference: "order_1", AmountMinor: 1234, Currency: "USD"})
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	if charge.Reference != "charge_1" {
		t.Fatalf("expected reference charge_1, got %q", charge.Reference)
	}
	if charge.RedirectURL != "https://checkout.opennode.com/charge_1" {
		t.Fatalf("unexpected redirect url: %q", charge.RedirectURL)
	}
}

func TestOpenNodeBegin_RejectsNonPositiveAmount(t *testing.T) {
	p := &OpenNodeProvider{baseURL: "http://unused", apiKey: "k", httpClient: http.DefaultClient}
	if _, err := p.Begin(context.Background(), Order{Reference: "o1", AmountMinor: 0, Currency: "USD"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestOpenNodeVerify_PaidMapsToPaid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"paid","amount":12.34,"currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
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

func TestOpenNodeVerify_StringAmountAlsoParses(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"paid","amount":"12.34","currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	result, err := p.Verify(context.Background(), "charge_1")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.AmountMinor != 1234 {
		t.Fatalf("expected 1234 minor units from a string amount, got %d", result.AmountMinor)
	}
}

func TestOpenNodeVerify_UnderpaidNeverSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"underpaid","amount":12.34,"currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	result, err := p.Verify(context.Background(), "charge_1")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("an underpaid charge must never report StatusPaid")
	}
}

func TestOpenNodeVerify_ExpiredNeverSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"expired","amount":12.34,"currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	result, err := p.Verify(context.Background(), "charge_1")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("expected StatusFailed for an expired quote, got %v", result.Status)
	}
}

func TestOpenNodeVerify_RefundedNeverReportsPaid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"refunded","amount":12.34,"currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	result, err := p.Verify(context.Background(), "charge_1")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("a refunded charge must never report StatusPaid")
	}
}

func TestOpenNodeVerify_MalformedAmountFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"paid","amount":null,"currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	if _, err := p.Verify(context.Background(), "charge_1"); err == nil {
		t.Fatal("expected error for a null amount")
	}
}

func TestOpenNodeVerify_MalformedJSONFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	if _, err := p.Verify(context.Background(), "charge_1"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestOpenNodeVerify_ServerErrorFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	if _, err := p.Verify(context.Background(), "charge_1"); err == nil {
		t.Fatal("expected error for a 500 response")
	}
}

func TestOpenNodeVerify_TimeoutFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Verify(ctx, "charge_1"); err == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestOpenNodeWebhook_ValidSignatureRefetchesAndSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"paid","amount":12.34,"currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	form := url.Values{
		"id":           {"charge_1"},
		"hashed_order": {signOpenNode(p.apiKey, "charge_1")},
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook/opennode", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("expected StatusPaid, got %v", result.Status)
	}
}

func TestOpenNodeWebhook_MissingSignatureRejected(t *testing.T) {
	p := newTestOpenNode(t, httptest.NewServer(http.NotFoundHandler()))
	form := url.Values{"id": {"charge_1"}}
	req := httptest.NewRequest(http.MethodPost, "/webhook/opennode", strings.NewReader(form.Encode()))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrOpenNodeMissingSignature) {
		t.Fatalf("expected ErrOpenNodeMissingSignature, got %v", err)
	}
}

func TestOpenNodeWebhook_TamperedSignatureRejected(t *testing.T) {
	p := newTestOpenNode(t, httptest.NewServer(http.NotFoundHandler()))
	// hashed_order computed for a DIFFERENT charge id than the one submitted.
	form := url.Values{
		"id":           {"charge_1"},
		"hashed_order": {signOpenNode(p.apiKey, "charge_evil")},
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook/opennode", strings.NewReader(form.Encode()))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrOpenNodeInvalidSignature) {
		t.Fatalf("expected ErrOpenNodeInvalidSignature, got %v", err)
	}
}

func TestOpenNodeWebhook_WrongAPIKeyRejected(t *testing.T) {
	p := newTestOpenNode(t, httptest.NewServer(http.NotFoundHandler()))
	form := url.Values{
		"id":           {"charge_1"},
		"hashed_order": {signOpenNode("wrong-key", "charge_1")},
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook/opennode", strings.NewReader(form.Encode()))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrOpenNodeInvalidSignature) {
		t.Fatalf("expected ErrOpenNodeInvalidSignature, got %v", err)
	}
}

func TestOpenNodeWebhook_MalformedHexSignatureRejected(t *testing.T) {
	p := newTestOpenNode(t, httptest.NewServer(http.NotFoundHandler()))
	form := url.Values{"id": {"charge_1"}, "hashed_order": {"not-hex!!"}}
	req := httptest.NewRequest(http.MethodPost, "/webhook/opennode", strings.NewReader(form.Encode()))

	if _, err := p.Webhook(context.Background(), req); !errors.Is(err, ErrOpenNodeInvalidSignature) {
		t.Fatalf("expected ErrOpenNodeInvalidSignature, got %v", err)
	}
}

func TestOpenNodeWebhook_SpoofedStatusCannotFabricateSettlement(t *testing.T) {
	// A validly-signed webhook still must not be trusted for status: the
	// adapter always refetches. Here the real charge is still unpaid.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/charge/charge_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"charge_1","status":"unpaid","amount":12.34,"currency":"USD"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestOpenNode(t, srv)
	form := url.Values{
		"id":           {"charge_1"},
		"hashed_order": {signOpenNode(p.apiKey, "charge_1")},
		"status":       {"paid"}, // attacker-controlled field the adapter must ignore
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook/opennode", strings.NewReader(form.Encode()))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook failed: %v", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("webhook body claimed paid but the refetched charge is unpaid — must not report StatusPaid")
	}
}

func TestNewOpenNode_RequiresAPIKey(t *testing.T) {
	t.Setenv(EnvOpenNodeAPIKey, "")
	if _, err := NewOpenNode(); !errors.Is(err, ErrOpenNodeNotConfigured) {
		t.Fatalf("expected ErrOpenNodeNotConfigured, got %v", err)
	}

	t.Setenv(EnvOpenNodeAPIKey, "key")
	p, err := NewOpenNode()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if p.Name() != ProviderNameOpenNode {
		t.Fatalf("unexpected name: %s", p.Name())
	}
	if p.baseURL != opennodeDefaultBaseURL {
		t.Fatalf("expected default base URL, got %s", p.baseURL)
	}
}

func TestOpenNodeCapabilities(t *testing.T) {
	p := &OpenNodeProvider{}
	caps := p.Capabilities()
	if caps.Flow != FlowInvoice {
		t.Fatalf("expected FlowInvoice, got %v", caps.Flow)
	}
}
