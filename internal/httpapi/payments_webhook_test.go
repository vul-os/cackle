package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vul-os/cackle/internal/payments"
)

// TestWebhook_UnknownProviderIs404 and the tests below exercise the "fail
// closed" contract on POST /api/payments/webhook/{provider}: a missing or
// invalid signature must never be treated as a settled payment.
func TestWebhook_UnknownProviderIs404(t *testing.T) {
	h := newTestHarness(t)
	rec := h.do(http.MethodPost, "/api/payments/webhook/does-not-exist", "", map[string]any{"event": "charge.success"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown provider webhook: expected 404, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestWebhook_StubProviderAlwaysFailsClosed: the stub provider settles
// synchronously inside Begin and has no webhook mechanism — Webhook always
// errors (see internal/payments/stub.go). The route must reject it, never
// acknowledge a fabricated success.
func TestWebhook_StubProviderAlwaysFailsClosed(t *testing.T) {
	h := newTestHarness(t)
	rec := h.do(http.MethodPost, "/api/payments/webhook/stub", "", map[string]any{"event": "charge.success"})
	if rec.Code == http.StatusOK {
		t.Fatalf("stub webhook: must never succeed, got 200 body %s", rec.Body.String())
	}
}

// TestWebhook_PaystackMissingOrBadSignatureFailsClosed proves the raw-body
// HMAC check runs and rejects tampered/missing signatures before any order
// is ever touched.
func TestWebhook_PaystackMissingOrBadSignatureFailsClosed(t *testing.T) {
	h := newTestHarness(t)
	t.Setenv("CACKLE_PAYSTACK_SECRET_KEY", "test-paystack-secret")
	ps, err := payments.NewPaystack()
	if err != nil {
		t.Fatalf("new paystack provider: %v", err)
	}
	if err := h.payments.Register(ps); err != nil {
		t.Fatalf("register paystack: %v", err)
	}

	body := []byte(`{"event":"charge.success","data":{"reference":"nonexistent","status":"success","amount":100,"currency":"ZAR"}}`)

	// No signature header at all.
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/paystack", bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.20:1"
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("missing signature: must never succeed, got 200 body %s", rec.Body.String())
	}

	// Wrong signature (right shape, wrong key).
	badMAC := hmac.New(sha512.New, []byte("wrong-secret"))
	badMAC.Write(body)
	req = httptest.NewRequest(http.MethodPost, "/api/payments/webhook/paystack", bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.21:1"
	req.Header.Set("X-Paystack-Signature", hex.EncodeToString(badMAC.Sum(nil)))
	rec = httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("bad signature: must never succeed, got 200 body %s", rec.Body.String())
	}
}

// TestWebhook_PaystackValidSignatureUnhandledEventIsAcked proves a
// correctly-signed webhook for an event this build doesn't treat as a
// settlement (per ErrUnhandledEvent) is acknowledged with 200 (so the
// provider stops retrying) without ever calling Settle.
func TestWebhook_PaystackValidSignatureUnhandledEventIsAcked(t *testing.T) {
	h := newTestHarness(t)
	secret := "test-paystack-secret-2"
	t.Setenv("CACKLE_PAYSTACK_SECRET_KEY", secret)
	ps, err := payments.NewPaystack()
	if err != nil {
		t.Fatalf("new paystack provider: %v", err)
	}
	if err := h.payments.Register(ps); err != nil {
		t.Fatalf("register paystack: %v", err)
	}

	body := []byte(`{"event":"transfer.success","data":{"reference":"whatever"}}`)
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)

	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/paystack", bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.22:1"
	req.Header.Set("X-Paystack-Signature", hex.EncodeToString(mac.Sum(nil)))
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("validly-signed unhandled event: expected 200 ack, got %d body %s", rec.Code, rec.Body.String())
	}
}
