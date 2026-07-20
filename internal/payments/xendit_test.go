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

func newTestXenditProvider(ts *httptest.Server) *XenditProvider {
	return &XenditProvider{
		secretKey:    "xnd_test_fake",
		webhookToken: "test-callback-token",
		httpClient:   ts.Client(),
		baseURL:      ts.URL,
	}
}

func xenditWebhookRequest(body []byte, token string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/xendit", strings.NewReader(string(body)))
	if token != "" {
		req.Header.Set("x-callback-token", token)
	}
	return req
}

func TestNewXendit_RequiresSecretAndToken(t *testing.T) {
	t.Setenv(EnvXenditSecretKey, "")
	t.Setenv(EnvXenditWebhookToken, "")
	if _, err := NewXendit(); !errors.Is(err, ErrXenditSecretNotConfigured) {
		t.Fatalf("NewXendit() = %v, want ErrXenditSecretNotConfigured", err)
	}
	t.Setenv(EnvXenditSecretKey, "xnd_x")
	if _, err := NewXendit(); !errors.Is(err, ErrXenditTokenNotConfigured) {
		t.Fatalf("NewXendit() = %v, want ErrXenditTokenNotConfigured", err)
	}
}

func TestXenditBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ := r.BasicAuth()
		if user != "xnd_test_fake" {
			t.Fatalf("expected basic auth user, got %q", user)
		}
		w.Write([]byte(`{"id":"inv_1","external_id":"ord_1","status":"PENDING","amount":10000,"currency":"IDR","invoice_url":"https://checkout.xendit.co/web/inv_1"}`))
	}))
	defer ts.Close()
	p := newTestXenditProvider(ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "IDR", BuyerEmail: "a@b.com"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" {
		t.Fatal("empty RedirectURL")
	}
}

func TestXenditVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.String(), "external_id=ord_1") {
			t.Fatalf("unexpected query %s", r.URL.String())
		}
		w.Write([]byte(`[{"id":"inv_1","external_id":"ord_1","status":"PAID","amount":10000,"paid_amount":10000,"currency":"IDR","paid_at":"2026-07-20T10:00:00.000Z"}]`))
	}))
	defer ts.Close()
	p := newTestXenditProvider(ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 1000000 || result.Currency != "IDR" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestXenditVerify_NotFoundFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()
	p := newTestXenditProvider(ts)
	_, err := p.Verify(context.Background(), "ord_missing")
	if !errors.Is(err, ErrXenditInvoiceNotFound) {
		t.Fatalf("Verify() = %v, want ErrXenditInvoiceNotFound", err)
	}
}

func TestXenditVerify_PendingIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"inv_1","external_id":"ord_1","status":"PENDING","amount":10000,"currency":"IDR"}]`))
	}))
	defer ts.Close()
	p := newTestXenditProvider(ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid, want not-paid for PENDING invoice")
	}
}

func TestXenditVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestXenditProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrXenditMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrXenditMalformedResponse", err)
	}
}

func TestXenditVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`))
	}))
	defer ts.Close()
	p := newTestXenditProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrXenditUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrXenditUnexpectedStatus", err)
	}
}

func TestXenditVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`[{"id":"inv_1","external_id":"ord_1","status":"PAID","amount":10000,"currency":"IDR"}]`))
	}))
	defer ts.Close()
	p := newTestXenditProvider(ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Verify(context.Background(), "ord_1")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

func TestXenditWebhook_ValidTokenSucceeds(t *testing.T) {
	body := []byte(`{"id":"inv_9","external_id":"ord_1","status":"PAID","amount":10000,"paid_amount":10000,"currency":"IDR"}`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	result, err := p.Webhook(context.Background(), xenditWebhookRequest(body, "test-callback-token"))
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.Reference != "ord_1" || result.AmountMinor != 1000000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestXenditWebhook_MissingTokenFailsClosed(t *testing.T) {
	body := []byte(`{"id":"inv_9","external_id":"ord_1","status":"PAID","amount":10000,"currency":"IDR"}`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	_, err := p.Webhook(context.Background(), xenditWebhookRequest(body, ""))
	if !errors.Is(err, ErrXenditMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrXenditMissingSignature", err)
	}
}

func TestXenditWebhook_WrongTokenFailsClosed(t *testing.T) {
	body := []byte(`{"id":"inv_9","external_id":"ord_1","status":"PAID","amount":10000,"currency":"IDR"}`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	_, err := p.Webhook(context.Background(), xenditWebhookRequest(body, "wrong-token"))
	if !errors.Is(err, ErrXenditInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrXenditInvalidSignature", err)
	}
}

func TestXenditWebhook_PendingStatusUnhandled(t *testing.T) {
	body := []byte(`{"id":"inv_9","external_id":"ord_1","status":"PENDING","amount":10000,"currency":"IDR"}`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	_, err := p.Webhook(context.Background(), xenditWebhookRequest(body, "test-callback-token"))
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() = %v, want ErrUnhandledEvent", err)
	}
}

func TestXenditWebhook_MalformedJSONFailsClosed(t *testing.T) {
	body := []byte(`{not valid`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	_, err := p.Webhook(context.Background(), xenditWebhookRequest(body, "test-callback-token"))
	if !errors.Is(err, ErrXenditMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrXenditMalformedResponse", err)
	}
}

func TestXenditWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	body := []byte(`{"id":"inv_777","external_id":"ord_1","status":"PAID","amount":10000,"paid_amount":10000,"currency":"IDR"}`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 1000000, Currency: "IDR"}}}

	_, err := HandleWebhook(context.Background(), p, xenditWebhookRequest(body, "test-callback-token"), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, xenditWebhookRequest(body, "test-callback-token"), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestXenditWebhook_AmountMismatchFailsClosed(t *testing.T) {
	body := []byte(`{"id":"inv_778","external_id":"ord_1","status":"PAID","amount":10000,"paid_amount":10000,"currency":"IDR"}`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 99999, Currency: "IDR"}}}
	_, err := HandleWebhook(context.Background(), p, xenditWebhookRequest(body, "test-callback-token"), nil, lookup)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrAmountMismatch", err)
	}
}

func TestXenditWebhook_OversizedBodyRejected(t *testing.T) {
	junk := strings.Repeat("a", xenditMaxResponseSize+1024)
	body := []byte(`{"id":"inv_9","external_id":"ord_1","status":"PAID","amount":10000,"currency":"IDR","note":"` + junk + `"}`)
	p := &XenditProvider{webhookToken: "test-callback-token"}
	_, err := p.Webhook(context.Background(), xenditWebhookRequest(body, "test-callback-token"))
	if !errors.Is(err, ErrXenditResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrXenditResponseTooLarge", err)
	}
}
