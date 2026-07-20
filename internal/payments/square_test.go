package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testSquareWebhookKey = "square-test-webhook-signature-key"
const testSquareNotificationURL = "https://example.com/webhooks/square"

func newTestSquareProvider(ts *httptest.Server) *SquareProvider {
	return &SquareProvider{
		accessToken:         "sq0atp-test",
		webhookSignatureKey: testSquareWebhookKey,
		locationID:          "L123",
		notificationURL:     testSquareNotificationURL,
		httpClient:          ts.Client(),
		baseURL:             ts.URL,
	}
}

func signSquareBody(key, notificationURL string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(notificationURL))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestNewSquare_RequiresAllFiveEnvVars(t *testing.T) {
	t.Setenv(EnvSquareAccessToken, "")
	t.Setenv(EnvSquareWebhookSignatureKey, "")
	t.Setenv(EnvSquareLocationID, "")
	t.Setenv(EnvSquareNotificationURL, "")
	t.Setenv(EnvSquareAPIBaseURL, "")
	if _, err := NewSquare(); !errors.Is(err, ErrSquareAccessTokenNotConfigured) {
		t.Fatalf("err = %v, want ErrSquareAccessTokenNotConfigured", err)
	}
	t.Setenv(EnvSquareAccessToken, "tok")
	if _, err := NewSquare(); !errors.Is(err, ErrSquareWebhookKeyNotConfigured) {
		t.Fatalf("err = %v, want ErrSquareWebhookKeyNotConfigured", err)
	}
	t.Setenv(EnvSquareWebhookSignatureKey, "key")
	if _, err := NewSquare(); !errors.Is(err, ErrSquareLocationNotConfigured) {
		t.Fatalf("err = %v, want ErrSquareLocationNotConfigured", err)
	}
	t.Setenv(EnvSquareLocationID, "L1")
	if _, err := NewSquare(); !errors.Is(err, ErrSquareNotificationURLMissing) {
		t.Fatalf("err = %v, want ErrSquareNotificationURLMissing", err)
	}
	t.Setenv(EnvSquareNotificationURL, "https://example.com/webhook")
	if _, err := NewSquare(); !errors.Is(err, ErrSquareBaseURLNotConfigured) {
		t.Fatalf("err = %v, want ErrSquareBaseURLNotConfigured", err)
	}
	t.Setenv(EnvSquareAPIBaseURL, "https://connect.squareupsandbox.com")
	p, err := NewSquare()
	if err != nil {
		t.Fatalf("NewSquare() = %v, want nil", err)
	}
	if p.Name() != ProviderNameSquare {
		t.Fatalf("Name() = %q", p.Name())
	}
}

// --- currency table -----------------------------------------------------

func TestSquareAmount_OrdinaryAndZeroDecimalPassThrough(t *testing.T) {
	got, err := squareAmount(5000, "USD")
	if err != nil || got != 5000 {
		t.Fatalf("squareAmount(5000, USD) = %d, %v; want 5000, nil", got, err)
	}
	got, err = squareAmount(1000, "JPY")
	if err != nil || got != 1000 {
		t.Fatalf("squareAmount(1000, JPY) = %d, %v; want 1000, nil", got, err)
	}
}

func TestSquareAmount_ThreeDecimalCurrencyRefused(t *testing.T) {
	_, err := squareAmount(1000, "KWD")
	if !errors.Is(err, ErrSquareUnsupportedCurrency) {
		t.Fatalf("squareAmount(1000, KWD) err = %v, want ErrSquareUnsupportedCurrency", err)
	}
}

// --- Begin --------------------------------------------------------------

func TestSquareBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/online-checkout/payment-links" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sq0atp-test" {
			t.Fatalf("missing bearer token")
		}
		w.Write([]byte(`{"payment_link":{"id":"PLINK1","url":"https://square.link/u/PLINK1","order_id":"ORDER1"}}`))
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	charge, err := p.Begin(context.Background(), Order{
		Reference:   "ord_1",
		AmountMinor: 5000,
		Currency:    "USD",
		CallbackURL: "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL != "https://square.link/u/PLINK1" {
		t.Fatalf("RedirectURL = %q", charge.RedirectURL)
	}
}

func TestSquareBegin_RefusesThreeDecimalCurrency(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "KWD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrSquareUnsupportedCurrency) {
		t.Fatalf("Begin() err = %v, want ErrSquareUnsupportedCurrency", err)
	}
}

func TestSquareBegin_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errors":[{"category":"API_ERROR","code":"INTERNAL_SERVER_ERROR","detail":"boom"}]}`))
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrSquareUnexpectedStatus) {
		t.Fatalf("Begin() err = %v, want ErrSquareUnexpectedStatus", err)
	}
}

func TestSquareBegin_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrSquareMalformedResponse) {
		t.Fatalf("Begin() err = %v, want ErrSquareMalformedResponse", err)
	}
}

func TestSquareBegin_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Begin(ctx, Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if err == nil {
		t.Fatal("Begin() = nil error, want timeout error")
	}
}

// --- Verify ---------------------------------------------------------------

func TestSquareVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v2/payments/pay_1") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"payment":{"id":"pay_1","status":"COMPLETED","reference_id":"ord_1","amount_money":{"amount":5000,"currency":"USD"}}}`))
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	result, err := p.Verify(context.Background(), "pay_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 5000 || result.Currency != "USD" || result.Reference != "ord_1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestSquareVerify_PendingIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"payment":{"id":"pay_1","status":"PENDING","reference_id":"ord_1","amount_money":{"amount":5000,"currency":"USD"}}}`))
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	result, err := p.Verify(context.Background(), "pay_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for PENDING, want failed")
	}
}

func TestSquareVerify_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	_, err := p.Verify(context.Background(), "pay_1")
	if !errors.Is(err, ErrSquareUnexpectedStatus) {
		t.Fatalf("Verify() err = %v, want ErrSquareUnexpectedStatus", err)
	}
}

func TestSquareVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	_, err := p.Verify(context.Background(), "pay_1")
	if !errors.Is(err, ErrSquareMalformedResponse) {
		t.Fatalf("Verify() err = %v, want ErrSquareMalformedResponse", err)
	}
}

func TestSquareVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestSquareProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Verify(ctx, "pay_1")
	if err == nil {
		t.Fatal("Verify() = nil error, want timeout error")
	}
}

// --- Webhook --------------------------------------------------------------

func squarePaymentUpdatedEvent(eventID, paymentID, ref, currency string, amount int64, status string) []byte {
	return []byte(fmt.Sprintf(
		`{"event_id":%q,"type":"payment.updated","data":{"object":{"payment":{"id":%q,"status":%q,"reference_id":%q,"amount_money":{"amount":%d,"currency":%q}}}}}`,
		eventID, paymentID, status, ref, amount, currency,
	))
}

func TestSquareWebhook_Success(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "COMPLETED")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
	req.Header.Set("x-square-hmacsha256-signature", signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 5000 || result.Currency != "USD" || result.Reference != "ord_1" {
		t.Fatalf("result = %+v", result)
	}
	if result.EventID != "evt_1" {
		t.Fatalf("EventID = %q, want evt_1", result.EventID)
	}
}

func TestSquareWebhook_MissingSignatureFailsClosed(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "COMPLETED")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrSquareMissingSignature) {
		t.Fatalf("Webhook() err = %v, want ErrSquareMissingSignature", err)
	}
}

func TestSquareWebhook_TamperedSignatureFailsClosed(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "COMPLETED")
	sig := signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body)
	tampered := strings.Replace(string(body), `"amount":5000`, `"amount":1`, 1)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(tampered))
	req.Header.Set("x-square-hmacsha256-signature", sig)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrSquareInvalidSignature) {
		t.Fatalf("Webhook() err = %v, want ErrSquareInvalidSignature", err)
	}
}

func TestSquareWebhook_WrongNotificationURLFailsClosed(t *testing.T) {
	// The signature depends on the configured notification URL matching
	// exactly what Square signed against — a mismatch here must fail
	// closed just like a wrong key would.
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "COMPLETED")
	sig := signSquareBody(testSquareWebhookKey, "https://wrong.example.com/hook", body)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
	req.Header.Set("x-square-hmacsha256-signature", sig)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrSquareInvalidSignature) {
		t.Fatalf("Webhook() err = %v, want ErrSquareInvalidSignature", err)
	}
}

func TestSquareWebhook_MalformedJSONFailsClosed(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := []byte(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
	req.Header.Set("x-square-hmacsha256-signature", signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrSquareMalformedResponse) {
		t.Fatalf("Webhook() err = %v, want ErrSquareMalformedResponse", err)
	}
}

func TestSquareWebhook_UnhandledEventType(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := []byte(`{"event_id":"evt_1","type":"refund.updated","data":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
	req.Header.Set("x-square-hmacsha256-signature", signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() err = %v, want ErrUnhandledEvent", err)
	}
}

func TestSquareWebhook_NonCompletedPaymentIsUnhandled(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "PENDING")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
	req.Header.Set("x-square-hmacsha256-signature", signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() err = %v, want ErrUnhandledEvent (payment not yet completed)", err)
	}
}

func TestSquareWebhook_AmountMismatchFailsClosedViaReconcile(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "COMPLETED")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
	req.Header.Set("x-square-hmacsha256-signature", signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 999900, Currency: "USD"}); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrAmountMismatch", err)
	}
}

func TestSquareWebhook_CurrencyMismatchFailsClosedViaReconcile(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "COMPLETED")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
	req.Header.Set("x-square-hmacsha256-signature", signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 5000, Currency: "EUR"}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrCurrencyMismatch", err)
	}
}

func TestSquareWebhook_ReplayedEventProducesStableEventID(t *testing.T) {
	p := newTestSquareProvider(httptest.NewServer(nil))
	body := squarePaymentUpdatedEvent("evt_1", "pay_1", "ord_1", "USD", 5000, "COMPLETED")
	sig := signSquareBody(testSquareWebhookKey, testSquareNotificationURL, body)

	var ids []string
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/square", strings.NewReader(string(body)))
		req.Header.Set("x-square-hmacsha256-signature", sig)
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
