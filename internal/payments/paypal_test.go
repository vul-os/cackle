package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestPayPalProvider builds a PayPalProvider pointed at a single fake
// server that plays the role of BOTH api-m.paypal.com endpoints this
// adapter calls (oauth2/token, orders, notifications/verify-webhook-signature)
// — the test's handler dispatches on path.
func newTestPayPalProvider(ts *httptest.Server) *PayPalProvider {
	return &PayPalProvider{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		webhookID:    "WH-TEST-1",
		httpClient:   ts.Client(),
		baseURL:      ts.URL,
	}
}

func paypalTokenHandler(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/v1/oauth2/token" {
		return false
	}
	w.Write([]byte(`{"access_token":"A21AAtest","token_type":"Bearer","expires_in":32400}`))
	return true
}

func TestNewPayPal_RequiresAllFourSettings(t *testing.T) {
	t.Setenv(EnvPayPalClientID, "")
	t.Setenv(EnvPayPalClientSecret, "")
	t.Setenv(EnvPayPalWebhookID, "")
	t.Setenv(EnvPayPalEnv, "")
	if _, err := NewPayPal(); !errors.Is(err, ErrPayPalClientIDNotConfigured) {
		t.Fatalf("err = %v, want ErrPayPalClientIDNotConfigured", err)
	}
	t.Setenv(EnvPayPalClientID, "cid")
	if _, err := NewPayPal(); !errors.Is(err, ErrPayPalClientSecretNotConfigured) {
		t.Fatalf("err = %v, want ErrPayPalClientSecretNotConfigured", err)
	}
	t.Setenv(EnvPayPalClientSecret, "secret")
	if _, err := NewPayPal(); !errors.Is(err, ErrPayPalWebhookIDNotConfigured) {
		t.Fatalf("err = %v, want ErrPayPalWebhookIDNotConfigured", err)
	}
	t.Setenv(EnvPayPalWebhookID, "WH-1")
	if _, err := NewPayPal(); !errors.Is(err, ErrPayPalEnvNotConfigured) {
		t.Fatalf("err = %v, want ErrPayPalEnvNotConfigured (missing)", err)
	}
	t.Setenv(EnvPayPalEnv, "production") // not "live" or "sandbox"
	if _, err := NewPayPal(); !errors.Is(err, ErrPayPalEnvNotConfigured) {
		t.Fatalf("err = %v, want ErrPayPalEnvNotConfigured (invalid value)", err)
	}
	t.Setenv(EnvPayPalEnv, "sandbox")
	p, err := NewPayPal()
	if err != nil {
		t.Fatalf("NewPayPal() = %v, want nil", err)
	}
	if p.Name() != ProviderNamePayPal {
		t.Fatalf("Name() = %q", p.Name())
	}
	if p.baseURL != paypalSandboxBaseURL {
		t.Fatalf("baseURL = %q, want sandbox base URL", p.baseURL)
	}
}

// --- currency formatting --------------------------------------------------

func TestPayPalAmountValue_OrdinaryCurrency(t *testing.T) {
	got, err := paypalAmountValue(5000, "USD")
	if err != nil || got != "50.00" {
		t.Fatalf("paypalAmountValue(5000, USD) = %q, %v; want \"50.00\", nil", got, err)
	}
}

func TestPayPalAmountValue_ZeroDecimalCurrency(t *testing.T) {
	got, err := paypalAmountValue(1000, "JPY")
	if err != nil || got != "1000" {
		t.Fatalf("paypalAmountValue(1000, JPY) = %q, %v; want \"1000\", nil (no decimal point)", got, err)
	}
}

func TestPayPalAmountValue_ThreeDecimalCurrencyRefused(t *testing.T) {
	_, err := paypalAmountValue(1000, "KWD")
	if !errors.Is(err, ErrPayPalUnsupportedCurrency) {
		t.Fatalf("paypalAmountValue(1000, KWD) err = %v, want ErrPayPalUnsupportedCurrency", err)
	}
}

func TestPayPalAmountValueToMinor_RoundTrips(t *testing.T) {
	got, err := paypalAmountValueToMinor("50.00", "USD")
	if err != nil || got != 5000 {
		t.Fatalf("paypalAmountValueToMinor(50.00, USD) = %d, %v; want 5000, nil", got, err)
	}
	got, err = paypalAmountValueToMinor("1000", "JPY")
	if err != nil || got != 1000 {
		t.Fatalf("paypalAmountValueToMinor(1000, JPY) = %d, %v; want 1000, nil", got, err)
	}
}

// --- Begin --------------------------------------------------------------

func TestPayPalBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		if r.URL.Path != "/v2/checkout/orders" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer A21AAtest" {
			t.Fatalf("missing bearer token: %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		units := body["purchase_units"].([]any)
		amt := units[0].(map[string]any)["amount"].(map[string]any)
		if amt["value"] != "50.00" || amt["currency_code"] != "USD" {
			t.Fatalf("unexpected amount in request: %v", amt)
		}
		w.Write([]byte(`{"id":"5O190127TN364715T","status":"CREATED","links":[{"rel":"approve","href":"https://www.paypal.com/checkoutnow?token=5O190127TN364715T"}]}`))
	}))
	defer ts.Close()

	p := newTestPayPalProvider(ts)
	charge, err := p.Begin(context.Background(), Order{
		Reference:   "ord_1",
		AmountMinor: 5000,
		Currency:    "USD",
		CallbackURL: "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL != "https://www.paypal.com/checkoutnow?token=5O190127TN364715T" {
		t.Fatalf("RedirectURL = %q", charge.RedirectURL)
	}
}

func TestPayPalBegin_NoApproveLinkFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		w.Write([]byte(`{"id":"ORDER1","status":"CREATED","links":[{"rel":"self","href":"https://api.paypal.com/v2/checkout/orders/ORDER1"}]}`))
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrPayPalMalformedResponse) {
		t.Fatalf("Begin() err = %v, want ErrPayPalMalformedResponse", err)
	}
}

func TestPayPalBegin_RefusesThreeDecimalCurrency(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for a refused currency")
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "KWD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrPayPalUnsupportedCurrency) {
		t.Fatalf("Begin() err = %v, want ErrPayPalUnsupportedCurrency", err)
	}
}

func TestPayPalBegin_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"name":"INTERNAL_SERVER_ERROR","message":"boom"}`))
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrPayPalUnexpectedStatus) {
		t.Fatalf("Begin() err = %v, want ErrPayPalUnexpectedStatus", err)
	}
}

func TestPayPalBegin_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrPayPalMalformedResponse) {
		t.Fatalf("Begin() err = %v, want ErrPayPalMalformedResponse", err)
	}
}

func TestPayPalBegin_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			paypalTokenHandler(w, r)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Begin(ctx, Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if err == nil {
		t.Fatal("Begin() = nil error, want timeout error")
	}
}

// --- Verify (get + capture) ------------------------------------------------

func TestPayPalVerify_ApprovedThenCaptured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v2/checkout/orders/ORDER1"):
			w.Write([]byte(`{"id":"ORDER1","status":"APPROVED","purchase_units":[]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v2/checkout/orders/ORDER1/capture"):
			w.Write([]byte(`{"id":"ORDER1","status":"COMPLETED","purchase_units":[{"reference_id":"ord_1","custom_id":"ord_1","payments":{"captures":[{"id":"CAP1","status":"COMPLETED","amount":{"currency_code":"USD","value":"50.00"}}]}}]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	p := newTestPayPalProvider(ts)
	result, err := p.Verify(context.Background(), "ORDER1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 5000 || result.Currency != "USD" || result.Reference != "ord_1" {
		t.Fatalf("result = %+v", result)
	}
	if result.EventID != "CAP1" {
		t.Fatalf("EventID = %q, want CAP1", result.EventID)
	}
}

func TestPayPalVerify_CreatedIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		w.Write([]byte(`{"id":"ORDER1","status":"CREATED","purchase_units":[]}`))
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	result, err := p.Verify(context.Background(), "ORDER1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for CREATED order, want failed")
	}
}

func TestPayPalVerify_CaptureNotCompletedFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet:
			w.Write([]byte(`{"id":"ORDER1","status":"APPROVED","purchase_units":[]}`))
		case r.Method == http.MethodPost:
			w.Write([]byte(`{"id":"ORDER1","status":"PENDING","purchase_units":[]}`))
		}
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	result, err := p.Verify(context.Background(), "ORDER1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for a capture that did not complete, want failed")
	}
}

func TestPayPalVerify_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	_, err := p.Verify(context.Background(), "ORDER1")
	if !errors.Is(err, ErrPayPalUnexpectedStatus) {
		t.Fatalf("Verify() err = %v, want ErrPayPalUnexpectedStatus", err)
	}
}

func TestPayPalVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	_, err := p.Verify(context.Background(), "ORDER1")
	if !errors.Is(err, ErrPayPalMalformedResponse) {
		t.Fatalf("Verify() err = %v, want ErrPayPalMalformedResponse", err)
	}
}

func TestPayPalVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			paypalTokenHandler(w, r)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Verify(ctx, "ORDER1")
	if err == nil {
		t.Fatal("Verify() = nil error, want timeout error")
	}
}

// --- Webhook --------------------------------------------------------------

func paypalCaptureCompletedEvent(id, captureID, ref, currency, value string) []byte {
	return []byte(fmt.Sprintf(
		`{"id":%q,"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":%q,"status":"COMPLETED","custom_id":%q,"amount":{"currency_code":%q,"value":%q}}}`,
		id, captureID, ref, currency, value,
	))
}

func newPayPalWebhookRequest(body []byte, valid bool) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", strings.NewReader(string(body)))
	if valid {
		req.Header.Set("Paypal-Transmission-Id", "tx-1")
		req.Header.Set("Paypal-Transmission-Time", "2026-07-20T10:00:00Z")
		req.Header.Set("Paypal-Cert-Url", "https://api.paypal.com/cert.pem")
		req.Header.Set("Paypal-Auth-Algo", "SHA256withRSA")
		req.Header.Set("Paypal-Transmission-Sig", "fake-sig")
	}
	return req
}

func paypalVerifyHandler(verificationStatus string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		if r.URL.Path != "/v1/notifications/verify-webhook-signature" {
			return
		}
		w.Write([]byte(fmt.Sprintf(`{"verification_status":%q}`, verificationStatus)))
	}
}

func TestPayPalWebhook_Success(t *testing.T) {
	ts := httptest.NewServer(paypalVerifyHandler("SUCCESS"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := paypalCaptureCompletedEvent("WH-EVT-1", "CAP1", "ord_1", "USD", "50.00")
	req := newPayPalWebhookRequest(body, true)

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 5000 || result.Currency != "USD" || result.Reference != "ord_1" {
		t.Fatalf("result = %+v", result)
	}
	if result.EventID != "WH-EVT-1" {
		t.Fatalf("EventID = %q, want WH-EVT-1", result.EventID)
	}
}

func TestPayPalWebhook_MissingTransmissionHeadersFailsClosed(t *testing.T) {
	ts := httptest.NewServer(paypalVerifyHandler("SUCCESS"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := paypalCaptureCompletedEvent("WH-EVT-1", "CAP1", "ord_1", "USD", "50.00")
	req := newPayPalWebhookRequest(body, false)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrPayPalMissingSignatureHeaders) {
		t.Fatalf("Webhook() err = %v, want ErrPayPalMissingSignatureHeaders", err)
	}
}

func TestPayPalWebhook_VerificationFailureFailsClosed(t *testing.T) {
	// This is the PayPal-equivalent of a "tampered signature": PayPal's
	// own verify-webhook-signature endpoint reports FAILURE.
	ts := httptest.NewServer(paypalVerifyHandler("FAILURE"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := paypalCaptureCompletedEvent("WH-EVT-1", "CAP1", "ord_1", "USD", "50.00")
	req := newPayPalWebhookRequest(body, true)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrPayPalInvalidSignature) {
		t.Fatalf("Webhook() err = %v, want ErrPayPalInvalidSignature", err)
	}
}

func TestPayPalWebhook_VerifyEndpointHTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if paypalTokenHandler(w, r) {
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := paypalCaptureCompletedEvent("WH-EVT-1", "CAP1", "ord_1", "USD", "50.00")
	req := newPayPalWebhookRequest(body, true)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrPayPalUnexpectedStatus) {
		t.Fatalf("Webhook() err = %v, want ErrPayPalUnexpectedStatus", err)
	}
}

func TestPayPalWebhook_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(paypalVerifyHandler("SUCCESS"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	req := newPayPalWebhookRequest([]byte(`not json`), true)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrPayPalMalformedResponse) {
		t.Fatalf("Webhook() err = %v, want ErrPayPalMalformedResponse", err)
	}
}

func TestPayPalWebhook_UnhandledEventType(t *testing.T) {
	ts := httptest.NewServer(paypalVerifyHandler("SUCCESS"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := []byte(`{"id":"WH-EVT-1","event_type":"PAYMENT.CAPTURE.REFUNDED","resource":{}}`)
	req := newPayPalWebhookRequest(body, true)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() err = %v, want ErrUnhandledEvent", err)
	}
}

func TestPayPalWebhook_AmountMismatchFailsClosedViaReconcile(t *testing.T) {
	ts := httptest.NewServer(paypalVerifyHandler("SUCCESS"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := paypalCaptureCompletedEvent("WH-EVT-1", "CAP1", "ord_1", "USD", "50.00")
	req := newPayPalWebhookRequest(body, true)

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 999900, Currency: "USD"}); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrAmountMismatch", err)
	}
}

func TestPayPalWebhook_CurrencyMismatchFailsClosedViaReconcile(t *testing.T) {
	ts := httptest.NewServer(paypalVerifyHandler("SUCCESS"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := paypalCaptureCompletedEvent("WH-EVT-1", "CAP1", "ord_1", "USD", "50.00")
	req := newPayPalWebhookRequest(body, true)

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 5000, Currency: "EUR"}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrCurrencyMismatch", err)
	}
}

func TestPayPalWebhook_ReplayedEventProducesStableEventID(t *testing.T) {
	ts := httptest.NewServer(paypalVerifyHandler("SUCCESS"))
	defer ts.Close()
	p := newTestPayPalProvider(ts)
	body := paypalCaptureCompletedEvent("WH-EVT-1", "CAP1", "ord_1", "USD", "50.00")

	var ids []string
	for i := 0; i < 2; i++ {
		req := newPayPalWebhookRequest(body, true)
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
