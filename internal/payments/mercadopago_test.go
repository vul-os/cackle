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

const testMercadoPagoWebhookSecret = "test-mp-webhook-secret"

func newTestMercadoPagoProvider(ts *httptest.Server) *MercadoPagoProvider {
	return &MercadoPagoProvider{
		accessToken:   "APP_USR-test-token",
		webhookSecret: testMercadoPagoWebhookSecret,
		httpClient:    ts.Client(),
		baseURL:       ts.URL,
	}
}

func signMercadoPagoManifest(dataID, requestID, ts string) string {
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", dataID, requestID, ts)
	mac := hmac.New(sha256.New, []byte(testMercadoPagoWebhookSecret))
	mac.Write([]byte(manifest))
	return hex.EncodeToString(mac.Sum(nil))
}

func mercadoPagoWebhookRequest(body []byte, requestID, xSignature string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/mercadopago", strings.NewReader(string(body)))
	if requestID != "" {
		req.Header.Set("x-request-id", requestID)
	}
	if xSignature != "" {
		req.Header.Set("x-signature", xSignature)
	}
	return req
}

func TestNewMercadoPago_RequiresTokenAndSecret(t *testing.T) {
	t.Setenv(EnvMercadoPagoAccessToken, "")
	t.Setenv(EnvMercadoPagoWebhookSecret, "")
	if _, err := NewMercadoPago(); !errors.Is(err, ErrMercadoPagoTokenNotConfigured) {
		t.Fatalf("NewMercadoPago() = %v, want ErrMercadoPagoTokenNotConfigured", err)
	}
	t.Setenv(EnvMercadoPagoAccessToken, "APP_USR-x")
	if _, err := NewMercadoPago(); !errors.Is(err, ErrMercadoPagoWebhookSecretNotSet) {
		t.Fatalf("NewMercadoPago() = %v, want ErrMercadoPagoWebhookSecretNotSet", err)
	}
}

func TestMercadoPagoBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"pref_1","init_point":"https://www.mercadopago.com/checkout/v1/redirect?pref_id=pref_1"}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10050, Currency: "ARS", BuyerEmail: "a@b.com"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" {
		t.Fatal("empty RedirectURL")
	}
}

func TestMercadoPagoVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.String(), "external_reference=ord_1") {
			t.Fatalf("unexpected query %s", r.URL.String())
		}
		w.Write([]byte(`{"results":[{"id":123,"status":"approved","transaction_amount":100.50,"currency_id":"ARS","external_reference":"ord_1","date_approved":"2026-07-20T10:00:00.000-04:00"}]}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 10050 || result.Currency != "ARS" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMercadoPagoVerify_NotFoundFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[]}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	_, err := p.Verify(context.Background(), "ord_missing")
	if !errors.Is(err, ErrMercadoPagoPaymentNotFound) {
		t.Fatalf("Verify() = %v, want ErrMercadoPagoPaymentNotFound", err)
	}
}

func TestMercadoPagoVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrMercadoPagoMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrMercadoPagoMalformedResponse", err)
	}
}

func TestMercadoPagoVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"error"}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrMercadoPagoUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrMercadoPagoUnexpectedStatus", err)
	}
}

func TestMercadoPagoVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"results":[]}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Verify(context.Background(), "ord_1")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

func TestMercadoPagoWebhook_ValidSignatureFetchesAndSucceeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/payments/555") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":555,"status":"approved","transaction_amount":100.50,"currency_id":"ARS","external_reference":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)

	body := []byte(`{"action":"payment.created","type":"payment","data":{"id":"555"}}`)
	sig := "ts=1700000000,v1=" + signMercadoPagoManifest("555", "req-1", "1700000000")
	req := mercadoPagoWebhookRequest(body, "req-1", sig)
	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.Reference != "ord_1" || result.AmountMinor != 10050 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMercadoPagoWebhook_MissingSignatureFailsClosed(t *testing.T) {
	p := &MercadoPagoProvider{webhookSecret: testMercadoPagoWebhookSecret}
	body := []byte(`{"action":"payment.created","type":"payment","data":{"id":"555"}}`)
	_, err := p.Webhook(context.Background(), mercadoPagoWebhookRequest(body, "", ""))
	if !errors.Is(err, ErrMercadoPagoMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrMercadoPagoMissingSignature", err)
	}
}

func TestMercadoPagoWebhook_TamperedRequestIDFailsClosed(t *testing.T) {
	p := &MercadoPagoProvider{webhookSecret: testMercadoPagoWebhookSecret}
	body := []byte(`{"action":"payment.created","type":"payment","data":{"id":"555"}}`)
	sig := "ts=1700000000,v1=" + signMercadoPagoManifest("555", "req-1", "1700000000")
	// Attacker (or a proxy bug) changes x-request-id after the signature
	// was computed for "req-1" -- manifest no longer matches.
	req := mercadoPagoWebhookRequest(body, "req-EVIL", sig)
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMercadoPagoInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrMercadoPagoInvalidSignature", err)
	}
}

func TestMercadoPagoWebhook_WrongSecretFailsClosed(t *testing.T) {
	p := &MercadoPagoProvider{webhookSecret: testMercadoPagoWebhookSecret}
	body := []byte(`{"action":"payment.created","type":"payment","data":{"id":"555"}}`)
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", "555", "req-1", "1700000000")
	mac := hmac.New(sha256.New, []byte("some-other-secret"))
	mac.Write([]byte(manifest))
	sig := "ts=1700000000,v1=" + hex.EncodeToString(mac.Sum(nil))
	req := mercadoPagoWebhookRequest(body, "req-1", sig)
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMercadoPagoInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrMercadoPagoInvalidSignature", err)
	}
}

func TestMercadoPagoWebhook_MalformedJSONFailsClosed(t *testing.T) {
	p := &MercadoPagoProvider{webhookSecret: testMercadoPagoWebhookSecret}
	body := []byte(`{not valid json`)
	req := mercadoPagoWebhookRequest(body, "req-1", "ts=1700000000,v1=irrelevant")
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMercadoPagoMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrMercadoPagoMalformedResponse", err)
	}
}

func TestMercadoPagoWebhook_UnhandledType(t *testing.T) {
	p := &MercadoPagoProvider{webhookSecret: testMercadoPagoWebhookSecret}
	body := []byte(`{"action":"created","type":"merchant_order","data":{"id":"555"}}`)
	sig := "ts=1700000000,v1=" + signMercadoPagoManifest("555", "req-1", "1700000000")
	req := mercadoPagoWebhookRequest(body, "req-1", sig)
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() = %v, want ErrUnhandledEvent", err)
	}
}

func TestMercadoPagoWebhook_FetchedPaymentIDMismatchFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server returns a DIFFERENT payment id than requested -- never trust it.
		w.Write([]byte(`{"id":999,"status":"approved","transaction_amount":100.50,"currency_id":"ARS","external_reference":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	body := []byte(`{"action":"payment.created","type":"payment","data":{"id":"555"}}`)
	sig := "ts=1700000000,v1=" + signMercadoPagoManifest("555", "req-1", "1700000000")
	req := mercadoPagoWebhookRequest(body, "req-1", sig)
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMercadoPagoMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrMercadoPagoMalformedResponse", err)
	}
}

func TestMercadoPagoWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":777,"status":"approved","transaction_amount":100.50,"currency_id":"ARS","external_reference":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestMercadoPagoProvider(ts)
	body := []byte(`{"action":"payment.created","type":"payment","data":{"id":"777"}}`)
	sig := "ts=1700000000,v1=" + signMercadoPagoManifest("777", "req-1", "1700000000")
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 10050, Currency: "ARS"}}}

	req := func() *http.Request { return mercadoPagoWebhookRequest(body, "req-1", sig) }
	_, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, req(), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestMercadoPagoWebhook_OversizedBodyRejected(t *testing.T) {
	p := &MercadoPagoProvider{webhookSecret: testMercadoPagoWebhookSecret}
	junk := strings.Repeat("a", mercadoPagoMaxResponseSize+1024)
	body := []byte(`{"action":"payment.created","type":"payment","data":{"id":"555"},"note":"` + junk + `"}`)
	req := mercadoPagoWebhookRequest(body, "req-1", "ts=1700000000,v1=irrelevant")
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMercadoPagoResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrMercadoPagoResponseTooLarge", err)
	}
}
