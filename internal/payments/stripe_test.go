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

const testStripeSecretKey = "sk_test_fake_secret_for_unit_tests"
const testStripeWebhookSecret = "whsec_fake_secret_for_unit_tests"

func newTestStripeProvider(ts *httptest.Server) *StripeProvider {
	return &StripeProvider{
		secretKey:     testStripeSecretKey,
		webhookSecret: testStripeWebhookSecret,
		httpClient:    ts.Client(),
		baseURL:       ts.URL,
		now:           time.Now,
	}
}

func signStripeBody(secret string, timestamp int64, body []byte) string {
	signedPayload := fmt.Sprintf("%d.%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	return hex.EncodeToString(mac.Sum(nil))
}

func stripeSigHeader(secret string, timestamp int64, body []byte) string {
	return fmt.Sprintf("t=%d,v1=%s", timestamp, signStripeBody(secret, timestamp, body))
}

// --- construction -----------------------------------------------------

func TestNewStripe_RequiresSecretKey(t *testing.T) {
	t.Setenv(EnvStripeSecretKey, "")
	t.Setenv(EnvStripeWebhookSecret, "whsec_x")
	_, err := NewStripe()
	if !errors.Is(err, ErrStripeSecretNotConfigured) {
		t.Fatalf("NewStripe() = %v, want ErrStripeSecretNotConfigured", err)
	}
}

func TestNewStripe_RequiresWebhookSecret(t *testing.T) {
	t.Setenv(EnvStripeSecretKey, "sk_test_x")
	t.Setenv(EnvStripeWebhookSecret, "")
	_, err := NewStripe()
	if !errors.Is(err, ErrStripeWebhookSecretMissing) {
		t.Fatalf("NewStripe() = %v, want ErrStripeWebhookSecretMissing", err)
	}
}

func TestNewStripe_Succeeds(t *testing.T) {
	t.Setenv(EnvStripeSecretKey, "sk_test_x")
	t.Setenv(EnvStripeWebhookSecret, "whsec_x")
	p, err := NewStripe()
	if err != nil {
		t.Fatalf("NewStripe() = %v, want nil", err)
	}
	if p.Name() != ProviderNameStripe {
		t.Fatalf("Name() = %q", p.Name())
	}
}

// --- zero-decimal / currency conversion --------------------------------

func TestStripeAmount_OrdinaryCurrencyPassesThrough(t *testing.T) {
	got, err := stripeAmount(5000, "USD")
	if err != nil || got != 5000 {
		t.Fatalf("stripeAmount(5000, USD) = %d, %v; want 5000, nil", got, err)
	}
}

func TestStripeAmount_ZeroDecimalCurrencyPassesThrough(t *testing.T) {
	got, err := stripeAmount(1000, "JPY")
	if err != nil || got != 1000 {
		t.Fatalf("stripeAmount(1000, JPY) = %d, %v; want 1000, nil (no x100 for zero-decimal)", got, err)
	}
}

func TestStripeAmount_ISKForcedTwoDecimal(t *testing.T) {
	// ISK: ISO-4217 exponent 0 (AmountMinor=500 means 500 ISK), but
	// Stripe's own docs say to charge 5 ISK you pass amount=500 — i.e.
	// Stripe wants the ISK count x100, unlike plain zero-decimal
	// currencies. This is THE gotcha this file exists to get right.
	got, err := stripeAmount(500, "ISK")
	if err != nil || got != 50000 {
		t.Fatalf("stripeAmount(500, ISK) = %d, %v; want 50000, nil", got, err)
	}
}

func TestStripeAmount_UGXForcedTwoDecimal(t *testing.T) {
	got, err := stripeAmount(500, "UGX")
	if err != nil || got != 50000 {
		t.Fatalf("stripeAmount(500, UGX) = %d, %v; want 50000, nil", got, err)
	}
}

func TestStripeAmount_ThreeDecimalCurrencyRefused(t *testing.T) {
	_, err := stripeAmount(1000, "KWD")
	if !errors.Is(err, ErrStripeUnsupportedCurrency) {
		t.Fatalf("stripeAmount(1000, KWD) err = %v, want ErrStripeUnsupportedCurrency", err)
	}
}

func TestStripeAmountToMinor_RoundTrips(t *testing.T) {
	for _, tc := range []struct {
		amt      int64
		currency string
		want     int64
	}{
		{5000, "USD", 5000},
		{1000, "JPY", 1000},
		{50000, "ISK", 500},
		{50000, "UGX", 500},
	} {
		got, err := stripeAmountToMinor(tc.amt, tc.currency)
		if err != nil || got != tc.want {
			t.Fatalf("stripeAmountToMinor(%d, %s) = %d, %v; want %d, nil", tc.amt, tc.currency, got, err, tc.want)
		}
	}
}

func TestStripeAmountToMinor_ISKNotMultipleOf100Rejected(t *testing.T) {
	_, err := stripeAmountToMinor(555, "ISK")
	if !errors.Is(err, ErrStripeMalformedResponse) {
		t.Fatalf("stripeAmountToMinor(555, ISK) err = %v, want ErrStripeMalformedResponse", err)
	}
}

// --- Begin --------------------------------------------------------------

func TestStripeBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkout/sessions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.PostForm.Get("client_reference_id") != "ord_1" {
			t.Fatalf("client_reference_id = %q", r.PostForm.Get("client_reference_id"))
		}
		if r.PostForm.Get("line_items[0][price_data][unit_amount]") != "5000" {
			t.Fatalf("unit_amount = %q", r.PostForm.Get("line_items[0][price_data][unit_amount]"))
		}
		w.Write([]byte(`{"id":"cs_test_1","url":"https://checkout.stripe.com/pay/cs_test_1"}`))
	}))
	defer ts.Close()

	p := newTestStripeProvider(ts)
	charge, err := p.Begin(context.Background(), Order{
		Reference:   "ord_1",
		AmountMinor: 5000,
		Currency:    "USD",
		BuyerEmail:  "buyer@example.com",
		CallbackURL: "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL != "https://checkout.stripe.com/pay/cs_test_1" {
		t.Fatalf("RedirectURL = %q", charge.RedirectURL)
	}
	if charge.Reference != "ord_1" {
		t.Fatalf("Reference = %q", charge.Reference)
	}
}

func TestStripeBegin_RequiresCallbackURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD"})
	if err == nil {
		t.Fatal("Begin() = nil error, want error for missing callback_url")
	}
}

func TestStripeBegin_RefusesThreeDecimalCurrency(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for a refused currency")
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "KWD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrStripeUnsupportedCurrency) {
		t.Fatalf("Begin() err = %v, want ErrStripeUnsupportedCurrency", err)
	}
}

func TestStripeBegin_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"internal error","type":"api_error"}}`))
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrStripeUnexpectedStatus) {
		t.Fatalf("Begin() err = %v, want ErrStripeUnexpectedStatus", err)
	}
}

func TestStripeBegin_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if !errors.Is(err, ErrStripeMalformedResponse) {
		t.Fatalf("Begin() err = %v, want ErrStripeMalformedResponse", err)
	}
}

func TestStripeBegin_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"id":"cs_test_1","url":"https://checkout.stripe.com/pay/cs_test_1"}`))
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Begin(ctx, Order{Reference: "ord_1", AmountMinor: 5000, Currency: "USD", CallbackURL: "https://example.com"})
	if err == nil {
		t.Fatal("Begin() = nil error, want timeout error")
	}
}

// --- Verify ---------------------------------------------------------------

func TestStripeVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/checkout/sessions/cs_test_1") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":"cs_test_1","payment_status":"paid","amount_total":5000,"currency":"usd","client_reference_id":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	result, err := p.Verify(context.Background(), "cs_test_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("Status = %q, want paid", result.Status)
	}
	if result.AmountMinor != 5000 {
		t.Fatalf("AmountMinor = %d, want 5000", result.AmountMinor)
	}
	if result.Currency != "USD" {
		t.Fatalf("Currency = %q, want USD", result.Currency)
	}
	if result.Reference != "ord_1" {
		t.Fatalf("Reference = %q, want ord_1", result.Reference)
	}
}

func TestStripeVerify_UnpaidIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"cs_test_1","payment_status":"unpaid","amount_total":0,"currency":"usd","client_reference_id":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	result, err := p.Verify(context.Background(), "cs_test_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error (unpaid is a valid, non-paid result)", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for payment_status=unpaid, want failed")
	}
}

func TestStripeVerify_ZeroDecimalAmountRoundTrips(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"cs_test_1","payment_status":"paid","amount_total":1000,"currency":"jpy","client_reference_id":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	result, err := p.Verify(context.Background(), "cs_test_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.AmountMinor != 1000 {
		t.Fatalf("AmountMinor = %d, want 1000 (JPY passthrough)", result.AmountMinor)
	}
}

func TestStripeVerify_HTTP500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	_, err := p.Verify(context.Background(), "cs_test_1")
	if !errors.Is(err, ErrStripeUnexpectedStatus) {
		t.Fatalf("Verify() err = %v, want ErrStripeUnexpectedStatus", err)
	}
}

func TestStripeVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	_, err := p.Verify(context.Background(), "cs_test_1")
	if !errors.Is(err, ErrStripeMalformedResponse) {
		t.Fatalf("Verify() err = %v, want ErrStripeMalformedResponse", err)
	}
}

func TestStripeVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()
	p := newTestStripeProvider(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := p.Verify(ctx, "cs_test_1")
	if err == nil {
		t.Fatal("Verify() = nil error, want timeout error")
	}
}

// --- Webhook --------------------------------------------------------------

func stripeCheckoutCompletedEvent(id, sessionID, ref, currency string, amountTotal int64, paymentStatus string) []byte {
	return []byte(fmt.Sprintf(
		`{"id":%q,"type":"checkout.session.completed","data":{"object":{"id":%q,"payment_status":%q,"amount_total":%d,"currency":%q,"client_reference_id":%q}}}`,
		id, sessionID, paymentStatus, amountTotal, currency, ref,
	))
}

func newStripeWebhookRequest(t *testing.T, body []byte, sigHeader string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(body)))
	if sigHeader != "" {
		req.Header.Set("Stripe-Signature", sigHeader)
	}
	return req
}

func TestStripeWebhook_Success(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	ts := time.Now().Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader(testStripeWebhookSecret, ts, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("Status = %q, want paid", result.Status)
	}
	if result.AmountMinor != 5000 || result.Currency != "USD" {
		t.Fatalf("AmountMinor/Currency = %d/%s, want 5000/USD", result.AmountMinor, result.Currency)
	}
	if result.Reference != "ord_1" {
		t.Fatalf("Reference = %q, want ord_1", result.Reference)
	}
	if result.EventID != "evt_1" {
		t.Fatalf("EventID = %q, want evt_1", result.EventID)
	}
}

func TestStripeWebhook_MissingSignatureFailsClosed(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	req := newStripeWebhookRequest(t, body, "")

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrStripeMissingSignature) {
		t.Fatalf("Webhook() err = %v, want ErrStripeMissingSignature", err)
	}
}

func TestStripeWebhook_TamperedSignatureFailsClosed(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	ts := time.Now().Unix()
	// Sign the ORIGINAL body, then tamper with the body afterward — the
	// signature no longer matches what's actually being sent.
	sig := stripeSigHeader(testStripeWebhookSecret, ts, body)
	tampered := append([]byte{}, body...)
	tampered = []byte(strings.Replace(string(tampered), `"amount_total":5000`, `"amount_total":1`, 1))
	req := newStripeWebhookRequest(t, tampered, sig)

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrStripeInvalidSignature) {
		t.Fatalf("Webhook() err = %v, want ErrStripeInvalidSignature", err)
	}
}

func TestStripeWebhook_WrongSecretFailsClosed(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	ts := time.Now().Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader("whsec_wrong_secret", ts, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrStripeInvalidSignature) {
		t.Fatalf("Webhook() err = %v, want ErrStripeInvalidSignature", err)
	}
}

func TestStripeWebhook_StaleTimestampFailsClosed(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	staleTs := time.Now().Add(-1 * time.Hour).Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader(testStripeWebhookSecret, staleTs, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrStripeStaleSignature) {
		t.Fatalf("Webhook() err = %v, want ErrStripeStaleSignature", err)
	}
}

func TestStripeWebhook_MalformedJSONFailsClosed(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := []byte(`not json at all`)
	ts := time.Now().Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader(testStripeWebhookSecret, ts, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrStripeMalformedResponse) {
		t.Fatalf("Webhook() err = %v, want ErrStripeMalformedResponse", err)
	}
}

func TestStripeWebhook_UnhandledEventType(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := []byte(`{"id":"evt_1","type":"charge.refunded","data":{"object":{}}}`)
	ts := time.Now().Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader(testStripeWebhookSecret, ts, body))

	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() err = %v, want ErrUnhandledEvent", err)
	}
}

func TestStripeWebhook_CompletedButUnpaidFailsClosed(t *testing.T) {
	// checkout.session.completed can still be payment_status=unpaid for
	// async payment methods — must never be treated as paid.
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "unpaid")
	ts := time.Now().Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader(testStripeWebhookSecret, ts, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil error (unpaid is a valid non-settlement result)", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for payment_status=unpaid, want failed")
	}
}

func TestStripeWebhook_AmountMismatchFailsClosedViaReconcile(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	ts := time.Now().Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader(testStripeWebhookSecret, ts, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	// The order on file says the total was 999900, not 5000 — Reconcile
	// must reject this, exactly the "pay R10, claim R1000" attack this
	// package exists to prevent.
	err = Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 999900, Currency: "USD"})
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrAmountMismatch", err)
	}
}

func TestStripeWebhook_CurrencyMismatchFailsClosedViaReconcile(t *testing.T) {
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	ts := time.Now().Unix()
	req := newStripeWebhookRequest(t, body, stripeSigHeader(testStripeWebhookSecret, ts, body))

	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	err = Reconcile(result, OrderRef{ID: "ord_1", AmountMinor: 5000, Currency: "ZAR"})
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Reconcile() err = %v, want ErrCurrencyMismatch", err)
	}
}

func TestStripeWebhook_ReplayedEventProducesStableEventID(t *testing.T) {
	// Webhook() itself does not dedupe (that is the orchestration layer's
	// job via a SeenStore, see provider.go's HandleWebhook) — but replay
	// protection is only possible at all if repeated delivery of the exact
	// same event produces the exact same, non-empty EventID every time.
	// This test asserts that precondition holds for Stripe.
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	body := stripeCheckoutCompletedEvent("evt_1", "cs_test_1", "ord_1", "usd", 5000, "paid")
	ts := time.Now().Unix()
	sig := stripeSigHeader(testStripeWebhookSecret, ts, body)

	seen := map[string]bool{}
	var last Result
	for i := 0; i < 2; i++ {
		req := newStripeWebhookRequest(t, body, sig)
		result, err := p.Webhook(context.Background(), req)
		if err != nil {
			t.Fatalf("Webhook() call %d = %v", i, err)
		}
		if result.EventID == "" {
			t.Fatal("EventID is empty, cannot dedupe replays")
		}
		last = result
		if i == 1 {
			if !seen[result.EventID] {
				t.Fatal("second delivery of the identical event produced a different/unseen EventID — replay protection would not catch this")
			}
		}
		seen[result.EventID] = true
	}
	_ = last
}

func TestStripeWebhook_TruncatedBodyReadFailsClosed(t *testing.T) {
	// A request whose body is nil (e.g. already consumed) must fail
	// closed, never be treated as an empty-but-valid payload.
	p := &StripeProvider{webhookSecret: testStripeWebhookSecret, now: time.Now}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", nil)
	req.Body = nil
	req.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")

	_, err := p.Webhook(context.Background(), req)
	if err == nil {
		t.Fatal("Webhook() = nil error, want error for nil body")
	}
}
