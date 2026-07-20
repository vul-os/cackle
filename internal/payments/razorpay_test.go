package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testRazorpayWebhookSecret = "test-razorpay-webhook-secret"

func newTestRazorpayProvider(ts *httptest.Server) *RazorpayProvider {
	return &RazorpayProvider{
		keyID:         "rzp_test_fake",
		keySecret:     "test_secret",
		webhookSecret: testRazorpayWebhookSecret,
		httpClient:    ts.Client(),
		baseURL:       ts.URL,
	}
}

func signRazorpayBody(body []byte) string {
	mac := hmac.New(sha256.New, []byte(testRazorpayWebhookSecret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func razorpayWebhookRequest(body []byte, sig string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/razorpay", strings.NewReader(string(body)))
	if sig != "" {
		req.Header.Set("X-Razorpay-Signature", sig)
	}
	return req
}

func TestNewRazorpay_RequiresCredentials(t *testing.T) {
	t.Setenv(EnvRazorpayKeyID, "")
	t.Setenv(EnvRazorpayKeySecret, "")
	t.Setenv(EnvRazorpayWebhookSecret, "")
	if _, err := NewRazorpay(); !errors.Is(err, ErrRazorpayCredentialsNotConfigured) {
		t.Fatalf("NewRazorpay() = %v, want ErrRazorpayCredentialsNotConfigured", err)
	}
	t.Setenv(EnvRazorpayKeyID, "rzp_test_x")
	t.Setenv(EnvRazorpayKeySecret, "secret")
	if _, err := NewRazorpay(); !errors.Is(err, ErrRazorpayWebhookSecretNotConfigured) {
		t.Fatalf("NewRazorpay() = %v, want ErrRazorpayWebhookSecretNotConfigured", err)
	}
}

func TestRazorpayBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ := r.BasicAuth()
		if user != "rzp_test_fake" {
			t.Fatalf("expected basic auth, got %q", user)
		}
		w.Write([]byte(`{"id":"order_abc123","status":"created"}`))
	}))
	defer ts.Close()
	p := newTestRazorpayProvider(ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "INR"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.Reference != "order_abc123" {
		t.Fatalf("Reference = %q, want order_abc123", charge.Reference)
	}
	if charge.RedirectURL != "" {
		t.Fatal("RedirectURL should be empty for an inline-flow provider")
	}
	if charge.Instructions == "" {
		t.Fatal("Instructions should not be empty (client needs key_id + order_id)")
	}
}

func TestRazorpayBegin_ProviderErrorFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"description":"bad request"}}`))
	}))
	defer ts.Close()
	p := newTestRazorpayProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "INR"})
	if !errors.Is(err, ErrRazorpayUnexpectedStatus) {
		t.Fatalf("Begin() = %v, want ErrRazorpayUnexpectedStatus", err)
	}
}

func TestRazorpayVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/orders/order_abc123/payments") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"items":[{"id":"pay_1","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured","created_at":1753000000}]}`))
	}))
	defer ts.Close()
	p := newTestRazorpayProvider(ts)
	result, err := p.Verify(context.Background(), "order_abc123")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 10000 || result.Currency != "INR" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRazorpayVerify_NoCapturedPaymentFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"id":"pay_1","order_id":"order_abc123","amount":10000,"currency":"INR","status":"authorized"}]}`))
	}))
	defer ts.Close()
	p := newTestRazorpayProvider(ts)
	_, err := p.Verify(context.Background(), "order_abc123")
	if !errors.Is(err, ErrRazorpayNoCapturedPayment) {
		t.Fatalf("Verify() = %v, want ErrRazorpayNoCapturedPayment", err)
	}
}

func TestRazorpayVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestRazorpayProvider(ts)
	_, err := p.Verify(context.Background(), "order_abc123")
	if !errors.Is(err, ErrRazorpayMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrRazorpayMalformedResponse", err)
	}
}

func TestRazorpayVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"description":"oops"}}`))
	}))
	defer ts.Close()
	p := newTestRazorpayProvider(ts)
	_, err := p.Verify(context.Background(), "order_abc123")
	if !errors.Is(err, ErrRazorpayUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrRazorpayUnexpectedStatus", err)
	}
}

func TestRazorpayVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"items":[]}`))
	}))
	defer ts.Close()
	p := newTestRazorpayProvider(ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Verify(context.Background(), "order_abc123")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

func TestRazorpayWebhook_ValidSignatureSucceeds(t *testing.T) {
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured","created_at":1753000000}}}}`)
	sig := signRazorpayBody(body)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	result, err := p.Webhook(context.Background(), razorpayWebhookRequest(body, sig))
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.Reference != "order_abc123" || result.AmountMinor != 10000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRazorpayWebhook_MissingSignatureFailsClosed(t *testing.T) {
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured"}}}}`)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	_, err := p.Webhook(context.Background(), razorpayWebhookRequest(body, ""))
	if !errors.Is(err, ErrRazorpayMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrRazorpayMissingSignature", err)
	}
}

func TestRazorpayWebhook_TamperedSignatureFailsClosed(t *testing.T) {
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured"}}}}`)
	sig := signRazorpayBody(body)
	tampered := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_abc123","amount":999999999,"currency":"INR","status":"captured"}}}}`)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	_, err := p.Webhook(context.Background(), razorpayWebhookRequest(tampered, sig))
	if !errors.Is(err, ErrRazorpayInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrRazorpayInvalidSignature", err)
	}
}

func TestRazorpayWebhook_WrongSecretFailsClosed(t *testing.T) {
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured"}}}}`)
	mac := hmac.New(sha256.New, []byte("wrong-secret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	_, err := p.Webhook(context.Background(), razorpayWebhookRequest(body, sig))
	if !errors.Is(err, ErrRazorpayInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrRazorpayInvalidSignature", err)
	}
}

func TestRazorpayWebhook_InvalidHexSignatureFailsClosed(t *testing.T) {
	body := []byte(`{"event":"payment.captured","payload":{}}`)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	_, err := p.Webhook(context.Background(), razorpayWebhookRequest(body, "not-hex-zzz"))
	if !errors.Is(err, ErrRazorpayInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrRazorpayInvalidSignature", err)
	}
}

func TestRazorpayWebhook_UnhandledEvent(t *testing.T) {
	body := []byte(`{"event":"payment.failed","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_abc123","amount":10000,"currency":"INR","status":"failed"}}}}`)
	sig := signRazorpayBody(body)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	_, err := p.Webhook(context.Background(), razorpayWebhookRequest(body, sig))
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() = %v, want ErrUnhandledEvent", err)
	}
}

func TestRazorpayWebhook_MalformedJSONFailsClosed(t *testing.T) {
	body := []byte(`{not valid json`)
	sig := signRazorpayBody(body)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	_, err := p.Webhook(context.Background(), razorpayWebhookRequest(body, sig))
	if !errors.Is(err, ErrRazorpayMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrRazorpayMalformedResponse", err)
	}
}

func TestRazorpayWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_777","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured"}}}}`)
	sig := signRazorpayBody(body)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"order_abc123": {ID: "order_abc123", AmountMinor: 10000, Currency: "INR"}}}

	_, err := HandleWebhook(context.Background(), p, razorpayWebhookRequest(body, sig), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, razorpayWebhookRequest(body, sig), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestRazorpayWebhook_AmountMismatchFailsClosed(t *testing.T) {
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_778","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured"}}}}`)
	sig := signRazorpayBody(body)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"order_abc123": {ID: "order_abc123", AmountMinor: 1, Currency: "INR"}}}
	_, err := HandleWebhook(context.Background(), p, razorpayWebhookRequest(body, sig), nil, lookup)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrAmountMismatch", err)
	}
}

func TestRazorpayWebhook_OversizedBodyRejected(t *testing.T) {
	junk := strings.Repeat("a", razorpayMaxResponseSize+1024)
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_abc123","amount":10000,"currency":"INR","status":"captured","note":"` + junk + `"}}}}`)
	p := &RazorpayProvider{webhookSecret: testRazorpayWebhookSecret}
	_, err := p.Webhook(context.Background(), razorpayWebhookRequest(body, "irrelevant"))
	if !errors.Is(err, ErrRazorpayResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrRazorpayResponseTooLarge", err)
	}
}
