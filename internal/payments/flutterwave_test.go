package payments

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestFlutterwaveProvider(ts *httptest.Server) *FlutterwaveProvider {
	return &FlutterwaveProvider{
		secretKey:   "sk_test_fake",
		webhookHash: "test-webhook-hash",
		httpClient:  ts.Client(),
		baseURL:     ts.URL,
	}
}

func flutterwaveWebhookRequest(body []byte, hash string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/flutterwave", strings.NewReader(string(body)))
	if hash != "" {
		req.Header.Set("verif-hash", hash)
	}
	return req
}

func TestNewFlutterwave_RequiresSecretAndHash(t *testing.T) {
	t.Setenv(EnvFlutterwaveSecretKey, "")
	t.Setenv(EnvFlutterwaveWebhookHash, "")
	if _, err := NewFlutterwave(); !errors.Is(err, ErrFlutterwaveSecretNotConfigured) {
		t.Fatalf("NewFlutterwave() = %v, want ErrFlutterwaveSecretNotConfigured", err)
	}
	t.Setenv(EnvFlutterwaveSecretKey, "sk_test_x")
	if _, err := NewFlutterwave(); !errors.Is(err, ErrFlutterwaveHashNotConfigured) {
		t.Fatalf("NewFlutterwave() = %v, want ErrFlutterwaveHashNotConfigured", err)
	}
	t.Setenv(EnvFlutterwaveWebhookHash, "hash")
	p, err := NewFlutterwave()
	if err != nil {
		t.Fatalf("NewFlutterwave() = %v, want nil", err)
	}
	if p.Name() != ProviderNameFlutterwave {
		t.Fatalf("Name() = %q", p.Name())
	}
}

func TestFlutterwaveBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","message":"ok","data":{"link":"https://checkout.flutterwave.com/pay/abc"}}`))
	}))
	defer ts.Close()

	p := newTestFlutterwaveProvider(ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10050, Currency: "NGN", BuyerEmail: "a@b.com"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" {
		t.Fatal("charge.RedirectURL is empty")
	}
	if charge.Reference != "ord_1" {
		t.Fatalf("Reference = %q, want ord_1", charge.Reference)
	}
}

func TestFlutterwaveBegin_ProviderErrorFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","message":"invalid key"}`))
	}))
	defer ts.Close()
	p := newTestFlutterwaveProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "NGN", BuyerEmail: "a@b.com"})
	if !errors.Is(err, ErrFlutterwaveUnexpectedStatus) {
		t.Fatalf("Begin() = %v, want ErrFlutterwaveUnexpectedStatus", err)
	}
}

func TestFlutterwaveVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.String(), "verify_by_reference") {
			t.Fatalf("unexpected path %s", r.URL.String())
		}
		w.Write([]byte(`{"status":"success","data":{"id":123,"tx_ref":"ord_1","flw_ref":"FLW-REF","amount":100.50,"currency":"NGN","status":"successful"}}`))
	}))
	defer ts.Close()
	p := newTestFlutterwaveProvider(ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("Status = %q, want paid", result.Status)
	}
	if result.AmountMinor != 10050 {
		t.Fatalf("AmountMinor = %d, want 10050", result.AmountMinor)
	}
	if result.Currency != "NGN" {
		t.Fatalf("Currency = %q", result.Currency)
	}
}

func TestFlutterwaveVerify_FailedStatusIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","data":{"id":1,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"failed"}}`))
	}))
	defer ts.Close()
	p := newTestFlutterwaveProvider(ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil (failed is a valid non-paid result)", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid, want failed")
	}
}

func TestFlutterwaveVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	}))
	defer ts.Close()
	p := newTestFlutterwaveProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrFlutterwaveMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrFlutterwaveMalformedResponse", err)
	}
}

func TestFlutterwaveVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","message":"oops"}`))
	}))
	defer ts.Close()
	p := newTestFlutterwaveProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrFlutterwaveUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrFlutterwaveUnexpectedStatus", err)
	}
}

func TestFlutterwaveVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"status":"success","data":{"id":1,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful"}}`))
	}))
	defer ts.Close()
	p := newTestFlutterwaveProvider(ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Verify(context.Background(), "ord_1")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

func TestFlutterwaveWebhook_ValidHashSucceeds(t *testing.T) {
	body := []byte(`{"event":"charge.completed","data":{"id":9,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	result, err := p.Webhook(context.Background(), flutterwaveWebhookRequest(body, "test-webhook-hash"))
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.Reference != "ord_1" || result.AmountMinor != 10000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFlutterwaveWebhook_MissingHashFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.completed","data":{"id":9,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	_, err := p.Webhook(context.Background(), flutterwaveWebhookRequest(body, ""))
	if !errors.Is(err, ErrFlutterwaveMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrFlutterwaveMissingSignature", err)
	}
}

func TestFlutterwaveWebhook_WrongHashFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.completed","data":{"id":9,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	_, err := p.Webhook(context.Background(), flutterwaveWebhookRequest(body, "wrong-hash"))
	if !errors.Is(err, ErrFlutterwaveInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrFlutterwaveInvalidSignature", err)
	}
}

func TestFlutterwaveWebhook_UnhandledEvent(t *testing.T) {
	body := []byte(`{"event":"transfer.completed","data":{"id":9,"tx_ref":"trf_1","amount":100,"currency":"NGN","status":"successful"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	_, err := p.Webhook(context.Background(), flutterwaveWebhookRequest(body, "test-webhook-hash"))
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() = %v, want ErrUnhandledEvent", err)
	}
}

func TestFlutterwaveWebhook_MalformedJSONFailsClosed(t *testing.T) {
	body := []byte(`{not valid json`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	_, err := p.Webhook(context.Background(), flutterwaveWebhookRequest(body, "test-webhook-hash"))
	if !errors.Is(err, ErrFlutterwaveMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrFlutterwaveMalformedResponse", err)
	}
}

func TestFlutterwaveWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	body := []byte(`{"event":"charge.completed","data":{"id":777,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 10000, Currency: "NGN"}}}

	_, err := HandleWebhook(context.Background(), p, flutterwaveWebhookRequest(body, "test-webhook-hash"), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, flutterwaveWebhookRequest(body, "test-webhook-hash"), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestFlutterwaveWebhook_AmountMismatchFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.completed","data":{"id":778,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 99999, Currency: "NGN"}}}
	_, err := HandleWebhook(context.Background(), p, flutterwaveWebhookRequest(body, "test-webhook-hash"), nil, lookup)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrAmountMismatch", err)
	}
}

func TestFlutterwaveWebhook_CurrencyMismatchFailsClosed(t *testing.T) {
	body := []byte(`{"event":"charge.completed","data":{"id":779,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 10000, Currency: "GHS"}}}
	_, err := HandleWebhook(context.Background(), p, flutterwaveWebhookRequest(body, "test-webhook-hash"), nil, lookup)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrCurrencyMismatch", err)
	}
}

func TestFlutterwaveWebhook_OversizedBodyRejected(t *testing.T) {
	junk := strings.Repeat("a", flutterwaveMaxResponseSize+1024)
	body := []byte(`{"event":"charge.completed","data":{"id":9,"tx_ref":"ord_1","amount":100,"currency":"NGN","status":"successful","note":"` + junk + `"}}`)
	p := &FlutterwaveProvider{webhookHash: "test-webhook-hash"}
	_, err := p.Webhook(context.Background(), flutterwaveWebhookRequest(body, "test-webhook-hash"))
	if !errors.Is(err, ErrFlutterwaveResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrFlutterwaveResponseTooLarge", err)
	}
}
