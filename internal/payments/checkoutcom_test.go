package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testCheckoutComWebhookSecret = "cko_test_webhook_secret"

func newTestCheckoutComProvider(ts *httptest.Server) *CheckoutComProvider {
	return &CheckoutComProvider{
		secretKey:     "sk_test_fake",
		webhookSecret: testCheckoutComWebhookSecret,
		httpClient:    ts.Client(),
		baseURL:       ts.URL,
	}
}

func signCheckoutComBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestNewCheckoutCom_RequiresAllThreeEnvVars(t *testing.T) {
	t.Setenv(EnvCheckoutComSecretKey, "")
	t.Setenv(EnvCheckoutComWebhookKey, "")
	t.Setenv(EnvCheckoutComAPIBaseURL, "")
	if _, err := NewCheckoutCom(); !errors.Is(err, ErrCheckoutComSecretNotConfigured) {
		t.Fatalf("err = %v, want ErrCheckoutComSecretNotConfigured", err)
	}
	t.Setenv(EnvCheckoutComSecretKey, "sk_test_x")
	if _, err := NewCheckoutCom(); !errors.Is(err, ErrCheckoutComWebhookKeyMissing) {
		t.Fatalf("err = %v, want ErrCheckoutComWebhookKeyMissing", err)
	}
	t.Setenv(EnvCheckoutComWebhookKey, "wh_x")
	if _, err := NewCheckoutCom(); !errors.Is(err, ErrCheckoutComBaseURLNotConfigured) {
		t.Fatalf("err = %v, want ErrCheckoutComBaseURLNotConfigured", err)
	}
	t.Setenv(EnvCheckoutComAPIBaseURL, "https://api.sandbox.checkout.com")
	p, err := NewCheckoutCom()
	if err != nil {
		t.Fatalf("NewCheckoutCom() = %v, want nil", err)
	}
	if p.Name() != ProviderNameCheckoutCom {
		t.Fatalf("Name() = %q", p.Name())
	}
}

// --- currency table -----------------------------------------------------

func TestCheckoutComAmount_OrdinaryAndZeroDecimalPassThrough(t *testing.T) {
	if got := checkoutComAmount(5000, "USD"); got != 5000 {
		t.Fatalf("checkoutComAmount(5000, USD) = %d, want 5000", got)
	}
	if got := checkoutComAmount(1000, "JPY"); got != 1000 {
		t.Fatalf("checkoutComAmount(1000, JPY) = %d, want 1000", got)
	}
	if got := checkoutComAmount(1000, "ISK"); got != 1000 {
		t.Fatalf("checkoutComAmount(1000, ISK) = %d, want 1000 (Checkout.com treats ISK as zero-decimal, unlike Stripe)", got)
	}
}

func TestCheckoutComAmount_CLPForcedTwoDecimal(t *testing.T) {
	got := checkoutComAmount(500, "CLP")
	if got != 50000 {
		t.Fatalf("checkoutComAmount(500, CLP) = %d, want 50000", got)
	}
}

func TestCheckoutComAmountToMinor_RoundTrips(t *testing.T) {
	got, err := checkoutComAmountToMinor(50000, "CLP")
	if err != nil || got != 500 {
		t.Fatalf("checkoutComAmountToMinor(50000, CLP) = %d, %v; want 500, nil", got, err)
	}
	_, err = checkoutComAmountToMinor(555, "CLP")
	if !errors.Is(err, ErrCheckoutComMalformedResponse) {
		t.Fatalf("checkoutComAmountToMinor(555, CLP) err = %v, want ErrCheckoutComMalformedResponse", err)
	}
}

// --- Begin --------------------------------------------------------------

func TestCheckoutComBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hosted-payments" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk_test_fake" {
			t.Fatalf("missing/incorrect Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"id":"hp_test_1","_links":{"redirect":{"href":"https://pay.checkout.com/hp_test_1"}}}`))
	}))
	defer ts.Close()

	p := newTestCheckoutComProvider(ts)
	charge, err := p.Begin(context.Background(), Order{
		Reference:   "ord_1",
		AmountMinor: 5000,
		Currency:    "USD",
		CallbackURL: "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL != "https://pay.checkout.com/hp_test_1" {
		t.Fatalf("RedirectURL = %q", charge.RedirectURL)
	}
}

func TestCheckoutComBegin_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error_type":"processing_error","error_codes":["internal_error"]}`))
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrCheckoutComUnexpectedStatus) {
		t.Fatalf("Begin() err = %v, want ErrCheckoutComUnexpectedStatus", err)
	}
}

func TestCheckoutComBegin_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrCheckoutComMalformedResponse) {
		t.Fatalf("Begin() err = %v, want ErrCheckoutComMalformedResponse", err)
	}
}

func TestCheckoutComBegin_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Begin(ctx, Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if err == nil {
		t.Fatal("Begin() = nil error, want timeout error")
	}
}

// --- Verify ---------------------------------------------------------------

func TestCheckoutComVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/payments/pay_1") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":"pay_1","status":"Captured","amount":5000,"currency":"USD","reference":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	result, err := p.Verify(context.Background(), "pay_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 5000 || result.Currency != "USD" {
		t.Fatalf("result = %+v, want paid/5000/USD", result)
	}
}

func TestCheckoutComVerify_DeclinedIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"pay_1","status":"Declined","amount":5000,"currency":"USD","reference":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	result, err := p.Verify(context.Background(), "pay_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for Declined, want failed")
	}
}

func TestCheckoutComVerify_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	_, err := p.Verify(context.Background(), "pay_1")
	if !errors.Is(err, ErrCheckoutComUnexpectedStatus) {
		t.Fatalf("Verify() err = %v, want ErrCheckoutComUnexpectedStatus", err)
	}
}

func TestCheckoutComVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	_, err := p.Verify(context.Background(), "pay_1")
	if !errors.Is(err, ErrCheckoutComMalformedResponse) {
		t.Fatalf("Verify() err = %v, want ErrCheckoutComMalformedResponse", err)
	}
}

func TestCheckoutComVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestCheckoutComProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Verify(ctx, "pay_1")
	if err == nil {
		t.Fatal("Verify() = nil error, want timeout error")
	}
}

// --- Webhook --------------------------------------------------------------

func checkoutComCapturedEvent(id, paymentID, ref, currency string, amount int64) []byte {
	return []byte(fmt.Sprintf(
		`{"id":%q,"type":"payment_captured","data":{"id":%q,"status":"Captured","amount":%d,"currency":%q,"reference":%q}}`,
		id, paymentID, amount, currency, ref,
	))
}

func TestCheckoutComWebhook_Success(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := checkoutComCapturedEvent("evt_1", "pay_1", "ord_1", "USD", 5000)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))
	req.Header.Set("Cko-Signature", signCheckoutComBody(testCheckoutComWebhookSecret, body))

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

func TestCheckoutComWebhook_MissingSignatureFailsClosed(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := checkoutComCapturedEvent("evt_1", "pay_1", "ord_1", "USD", 5000)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrCheckoutComMissingSignature) {
		t.Fatalf("Webhook() err = %v, want ErrCheckoutComMissingSignature", err)
	}
}

func TestCheckoutComWebhook_TamperedSignatureFailsClosed(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := checkoutComCapturedEvent("evt_1", "pay_1", "ord_1", "USD", 5000)
	sig := signCheckoutComBody(testCheckoutComWebhookSecret, body)
	tampered := strings.Replace(string(body), `"amount":5000`, `"amount":1`, 1)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(tampered))
	req.Header.Set("Cko-Signature", sig)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrCheckoutComInvalidSignature) {
		t.Fatalf("Webhook() err = %v, want ErrCheckoutComInvalidSignature", err)
	}
}

func TestCheckoutComWebhook_WrongSecretFailsClosed(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := checkoutComCapturedEvent("evt_1", "pay_1", "ord_1", "USD", 5000)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))
	req.Header.Set("Cko-Signature", signCheckoutComBody("wrong-secret", body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrCheckoutComInvalidSignature) {
		t.Fatalf("Webhook() err = %v, want ErrCheckoutComInvalidSignature", err)
	}
}

func TestCheckoutComWebhook_MalformedJSONFailsClosed(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := []byte(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))
	req.Header.Set("Cko-Signature", signCheckoutComBody(testCheckoutComWebhookSecret, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrCheckoutComMalformedResponse) {
		t.Fatalf("Webhook() err = %v, want ErrCheckoutComMalformedResponse", err)
	}
}

func TestCheckoutComWebhook_UnhandledEventType(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := []byte(`{"id":"evt_1","type":"payment_approved","data":{"id":"pay_1","status":"Authorized","amount":5000,"currency":"USD","reference":"ord_1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))
	req.Header.Set("Cko-Signature", signCheckoutComBody(testCheckoutComWebhookSecret, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() err = %v, want ErrUnhandledEvent", err)
	}
}

func TestCheckoutComWebhook_AmountMismatchFailsClosedViaReconcile(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := checkoutComCapturedEvent("evt_1", "pay_1", "ord_1", "USD", 5000)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))
	req.Header.Set("Cko-Signature", signCheckoutComBody(testCheckoutComWebhookSecret, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 999900, Currency: "USD"}); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrAmountMismatch", err)
	}
}

func TestCheckoutComWebhook_CurrencyMismatchFailsClosedViaReconcile(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := checkoutComCapturedEvent("evt_1", "pay_1", "ord_1", "USD", 5000)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))
	req.Header.Set("Cko-Signature", signCheckoutComBody(testCheckoutComWebhookSecret, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 5000, Currency: "EUR"}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrCurrencyMismatch", err)
	}
}

func TestCheckoutComWebhook_ReplayedEventProducesStableEventID(t *testing.T) {
	p := newTestCheckoutComProvider(httptest.NewServer(nil))
	body := checkoutComCapturedEvent("evt_1", "pay_1", "ord_1", "USD", 5000)
	sig := signCheckoutComBody(testCheckoutComWebhookSecret, body)

	var ids []string
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/checkoutcom", strings.NewReader(string(body)))
		req.Header.Set("Cko-Signature", sig)
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
