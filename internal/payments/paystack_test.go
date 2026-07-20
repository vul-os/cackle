package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testPaystackSecret = "sk_test_fake_secret_for_unit_tests"

// newTestPaystackProvider builds a PaystackProvider pointed at ts, using
// unexported field access (this is an internal, same-package test file) —
// no exported test-only knob is needed on the production type.
func newTestPaystackProvider(t *testing.T, ts *httptest.Server) *PaystackProvider {
	t.Helper()
	return &PaystackProvider{
		secretKey:  testPaystackSecret,
		httpClient: ts.Client(),
		baseURL:    ts.URL,
	}
}

func signPaystackBody(t *testing.T, body []byte) string {
	t.Helper()
	mac := hmac.New(sha512.New, []byte(testPaystackSecret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestNewPaystack_RequiresSecret(t *testing.T) {
	t.Setenv(EnvPaystackSecretKey, "")
	_, err := NewPaystack()
	if !errors.Is(err, ErrPaystackSecretNotConfigured) {
		t.Fatalf("NewPaystack() = %v, want ErrPaystackSecretNotConfigured", err)
	}
}

func TestNewPaystack_SucceedsWithSecret(t *testing.T) {
	t.Setenv(EnvPaystackSecretKey, "sk_test_x")
	p, err := NewPaystack()
	if err != nil {
		t.Fatalf("NewPaystack() = %v, want nil", err)
	}
	if p.Name() != ProviderNamePaystack {
		t.Fatalf("Name() = %q", p.Name())
	}
}

// --- Verify ---------------------------------------------------------------

func TestPaystackVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/transaction/verify/ord_1") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"status":true,"message":"ok","data":{"id":9001,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR","paid_at":"2026-07-20T10:00:00.000Z"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("Status = %q, want paid", result.Status)
	}
	if result.AmountMinor != 5000 {
		t.Fatalf("AmountMinor = %d, want 5000", result.AmountMinor)
	}
	if result.Currency != "ZAR" {
		t.Fatalf("Currency = %q, want ZAR", result.Currency)
	}
	if result.EventID != "9001" {
		t.Fatalf("EventID = %q, want 9001", result.EventID)
	}
	if result.PaidAt.IsZero() {
		t.Fatal("PaidAt is zero, want parsed timestamp")
	}
}

func TestPaystackVerify_AbandonedIsFailedNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":true,"message":"ok","data":{"id":1,"status":"abandoned","reference":"ord_1","amount":5000,"currency":"ZAR"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error (abandoned is a valid, non-paid result)", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid, want failed for an abandoned transaction")
	}
}

func TestPaystackVerify_UnknownStatusFailsClosedAsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":true,"message":"ok","data":{"id":1,"status":"some-new-paystack-status","reference":"ord_1","amount":5000,"currency":"ZAR"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil error", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for an unrecognised status string, want fail-closed to failed")
	}
}

func TestPaystackVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":false,"message":"internal error"}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrUnexpectedStatus", err)
	}
}

func TestPaystackVerify_StatusFalseFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":false,"message":"Transaction reference not found"}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrUnexpectedStatus", err)
	}
}

func TestPaystackVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not valid json`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrMalformedResponse", err)
	}
}

func TestPaystackVerify_ReferenceMismatchFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server claims a different reference than what we asked about —
		// must never be trusted.
		w.Write([]byte(`{"status":true,"message":"ok","data":{"id":1,"status":"success","reference":"ord_evil","amount":5000,"currency":"ZAR"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrMalformedResponse for a reference mismatch", err)
	}
}

func TestPaystackVerify_SuccessWithNonPositiveAmountFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":true,"message":"ok","data":{"id":1,"status":"success","reference":"ord_1","amount":0,"currency":"ZAR"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrMalformedResponse for success+zero-amount", err)
	}
}

func TestPaystackVerify_EmptyReferenceRejected(t *testing.T) {
	p := &PaystackProvider{secretKey: testPaystackSecret, httpClient: http.DefaultClient, baseURL: "http://unused.invalid"}
	_, err := p.Verify(context.Background(), "  ")
	if err == nil {
		t.Fatal("Verify(\"\") = nil error, want rejection before any network call")
	}
}

func TestPaystackVerify_ProviderTimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"status":true,"data":{"id":1,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}

	_, err := p.Verify(context.Background(), "ord_1")
	if err == nil {
		t.Fatal("Verify() against a slow server = nil error, want a timeout failure")
	}
}

func TestPaystackVerify_ContextCancelledFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"status":true,"data":{"id":1,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := p.Verify(ctx, "ord_1")
	if err == nil {
		t.Fatal("Verify() with an already-expiring context = nil error, want failure")
	}
}

// --- Begin ------------------------------------------------------------

func TestPaystackBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/transaction/initialize") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"status":true,"message":"ok","data":{"authorization_url":"https://checkout.paystack.com/abc","access_code":"abc","reference":"ord_1"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, Currency: "ZAR", BuyerEmail: "a@b.com"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" {
		t.Fatal("charge.RedirectURL is empty")
	}
	if charge.Reference != "ord_1" {
		t.Fatalf("charge.Reference = %q, want ord_1", charge.Reference)
	}
}

func TestPaystackBegin_ProviderErrorFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":false,"message":"Invalid key"}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 5000, BuyerEmail: "a@b.com"})
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("Begin() = %v, want ErrUnexpectedStatus", err)
	}
}

func TestPaystackBegin_ValidationErrors(t *testing.T) {
	p := &PaystackProvider{secretKey: testPaystackSecret, httpClient: http.DefaultClient, baseURL: "http://unused.invalid"}

	cases := []Order{
		{Reference: "", AmountMinor: 100, BuyerEmail: "a@b.com"},
		{Reference: "ord_1", AmountMinor: 0, BuyerEmail: "a@b.com"},
		{Reference: "ord_1", AmountMinor: 100, BuyerEmail: ""},
	}
	for i, o := range cases {
		if _, err := p.Begin(context.Background(), o); err == nil {
			t.Fatalf("case %d: Begin() = nil error, want validation rejection", i)
		}
	}
}

// --- Webhook ------------------------------------------------------------

func paystackWebhookRequest(t *testing.T, body []byte, sig string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/paystack", strings.NewReader(string(body)))
	if sig != "" {
		req.Header.Set("X-Paystack-Signature", sig)
	}
	return req
}

func TestPaystackWebhook_ValidSignatureSucceeds(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":555,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR","paid_at":"2026-07-20T10:00:00.000Z"}}`)
	sig := signPaystackBody(t, body)

	p := &PaystackProvider{secretKey: testPaystackSecret}
	result, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, sig))
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("Status = %q, want paid", result.Status)
	}
	if result.Reference != "ord_1" || result.AmountMinor != 5000 || result.Currency != "ZAR" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.EventID != "555" {
		t.Fatalf("EventID = %q, want 555", result.EventID)
	}
}

func TestPaystackWebhook_MissingSignatureFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR"}}`)
	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, ""))
	if !errors.Is(err, ErrMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrMissingSignature", err)
	}
}

func TestPaystackWebhook_TamperedSignatureFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR"}}`)
	sig := signPaystackBody(t, body)

	// Tamper with the body AFTER signing (simulating an attacker who
	// intercepted a legitimate webhook and changed the amount) — the
	// signature was computed for the original body, so it must no longer
	// validate.
	tamperedBody := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"ord_1","amount":999999999,"currency":"ZAR"}}`)

	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, tamperedBody, sig))
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrInvalidSignature", err)
	}
}

func TestPaystackWebhook_WrongSecretFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR"}}`)
	// Sign with a DIFFERENT secret than the provider is configured with —
	// models a forged webhook from someone who doesn't know our secret.
	mac := hmac.New(sha512.New, []byte("some-other-secret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, sig))
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrInvalidSignature", err)
	}
}

func TestPaystackWebhook_InvalidHexSignatureFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR"}}`)
	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, "not-hex-zzz"))
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrInvalidSignature", err)
	}
}

func TestPaystackWebhook_UnhandledEventType(t *testing.T) {
	body := []byte(`{"event":"transfer.success","data":{"id":1,"status":"success","reference":"trf_1","amount":5000,"currency":"ZAR"}}`)
	sig := signPaystackBody(t, body)

	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, sig))
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() = %v, want ErrUnhandledEvent", err)
	}
}

func TestPaystackWebhook_MalformedJSONWithValidSignatureFailsClosed(t *testing.T) {
	// Even a correctly-signed body must be rejected if it isn't valid
	// JSON at all -- a valid signature proves authenticity, not
	// well-formedness.
	body := []byte(`{not valid json at all`)
	sig := signPaystackBody(t, body)

	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, sig))
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrMalformedResponse", err)
	}
}

func TestPaystackWebhook_InconsistentStatusFailsClosed(t *testing.T) {
	// event says charge.success but the nested data disagrees -- refuse
	// rather than guess which field to believe.
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"failed","reference":"ord_1","amount":5000,"currency":"ZAR"}}`)
	sig := signPaystackBody(t, body)

	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, sig))
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrMalformedResponse", err)
	}
}

func TestPaystackWebhook_MissingReferenceOrAmountFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"","amount":5000,"currency":"ZAR"}}`)
	sig := signPaystackBody(t, body)

	p := &PaystackProvider{secretKey: testPaystackSecret}
	_, err := p.Webhook(context.Background(), paystackWebhookRequest(t, body, sig))
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrMalformedResponse", err)
	}
}

// TestPaystackWebhook_ReplayedThroughHandleWebhook exercises the full,
// realistic path: the same valid webhook delivered twice (Paystack does
// retry) must be accepted once and rejected as a replay the second time.
func TestPaystackWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	body := []byte(`{"event":"charge.success","data":{"id":777,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR"}}`)
	sig := signPaystackBody(t, body)
	p := &PaystackProvider{secretKey: testPaystackSecret}
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 5000, Currency: "ZAR"}}}

	_, err := HandleWebhook(context.Background(), p, paystackWebhookRequest(t, body, sig), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}

	_, err = HandleWebhook(context.Background(), p, paystackWebhookRequest(t, body, sig), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second (replayed) delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

// --- ListBanks / CreateRecipient -----------------------------------------

func TestPaystackListBanks_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("currency") != "ZAR" {
			t.Fatalf("expected currency=ZAR query param, got %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"status":true,"message":"ok","data":[{"name":"First National Bank","slug":"fnb","code":"250655","currency":"ZAR","active":true}]}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	banks, err := p.ListBanks(context.Background())
	if err != nil {
		t.Fatalf("ListBanks() = %v", err)
	}
	if len(banks) != 1 || banks[0].Code != "250655" {
		t.Fatalf("unexpected banks: %+v", banks)
	}
}

func TestPaystackListBanks_ErrorFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status":false,"message":"Invalid key"}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.ListBanks(context.Background())
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("ListBanks() = %v, want ErrUnexpectedStatus", err)
	}
}

func TestPaystackCreateRecipient_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/transferrecipient") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"status":true,"message":"ok","data":{"recipient_code":"RCP_xyz","details":{"account_number":"1234567890","account_name":"Jane Organiser","bank_code":"250655","bank_name":"FNB"}}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	rec, err := p.CreateRecipient(context.Background(), RecipientRequest{
		Name: "Jane Organiser", AccountNumber: "1234567890", BankCode: "250655",
	})
	if err != nil {
		t.Fatalf("CreateRecipient() = %v", err)
	}
	if rec.RecipientCode != "RCP_xyz" {
		t.Fatalf("RecipientCode = %q, want RCP_xyz", rec.RecipientCode)
	}
}

func TestPaystackCreateRecipient_ErrorFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":false,"message":"Invalid account number"}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.CreateRecipient(context.Background(), RecipientRequest{
		Name: "Jane Organiser", AccountNumber: "bad", BankCode: "250655",
	})
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("CreateRecipient() = %v, want ErrUnexpectedStatus", err)
	}
}

func TestPaystackCreateRecipient_ValidationErrors(t *testing.T) {
	p := &PaystackProvider{secretKey: testPaystackSecret, httpClient: http.DefaultClient, baseURL: "http://unused.invalid"}
	_, err := p.CreateRecipient(context.Background(), RecipientRequest{})
	if err == nil {
		t.Fatal("CreateRecipient(empty request) = nil error, want validation rejection")
	}
}

// --- response size limit --------------------------------------------------

func TestPaystackVerify_OversizedResponseRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write far more than maxResponseBodyBytes.
		junk := strings.Repeat("a", maxResponseBodyBytes+1024)
		w.Write([]byte(`{"status":true,"data":{"id":1,"status":"success","reference":"ord_1","amount":5000,"currency":"ZAR","note":"` + junk + `"}}`))
	}))
	defer ts.Close()

	p := newTestPaystackProvider(t, ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("Verify() = %v, want ErrResponseTooLarge", err)
	}
}
