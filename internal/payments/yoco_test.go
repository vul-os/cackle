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
	"strconv"
	"strings"
	"testing"
	"time"
)

var testYocoWebhookSecretRaw = []byte("0123456789abcdef0123456789abcdef")

func newTestYocoProvider(ts *httptest.Server) *YocoProvider {
	return &YocoProvider{
		secretKey:     "sk_test_fake",
		webhookSecret: testYocoWebhookSecretRaw,
		httpClient:    ts.Client(),
		baseURL:       ts.URL,
	}
}

func signYocoWebhook(id, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, testYocoWebhookSecretRaw)
	mac.Write([]byte(id + "." + timestamp + "." + string(body)))
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func yocoWebhookRequest(body []byte, id, timestamp, sig string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/yoco", strings.NewReader(string(body)))
	if id != "" {
		req.Header.Set("webhook-id", id)
	}
	if timestamp != "" {
		req.Header.Set("webhook-timestamp", timestamp)
	}
	if sig != "" {
		req.Header.Set("webhook-signature", sig)
	}
	return req
}

func nowUnixString() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

func TestNewYoco_RequiresSecretAndWebhookSecret(t *testing.T) {
	t.Setenv(EnvYocoSecretKey, "")
	t.Setenv(EnvYocoWebhookSecret, "")
	if _, err := NewYoco(); !errors.Is(err, ErrYocoSecretNotConfigured) {
		t.Fatalf("NewYoco() = %v, want ErrYocoSecretNotConfigured", err)
	}
	t.Setenv(EnvYocoSecretKey, "sk_test_x")
	if _, err := NewYoco(); !errors.Is(err, ErrYocoWebhookSecretNotConfigured) {
		t.Fatalf("NewYoco() = %v, want ErrYocoWebhookSecretNotConfigured", err)
	}
	t.Setenv(EnvYocoWebhookSecret, "whsec_"+base64.StdEncoding.EncodeToString(testYocoWebhookSecretRaw))
	p, err := NewYoco()
	if err != nil {
		t.Fatalf("NewYoco() = %v, want nil", err)
	}
	if p.Name() != ProviderNameYoco {
		t.Fatalf("Name() = %q", p.Name())
	}
}

func TestYocoBegin_RejectsNonZAR(t *testing.T) {
	p := &YocoProvider{secretKey: "x"}
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "USD"})
	if !errors.Is(err, ErrYocoUnsupportedCurrency) {
		t.Fatalf("Begin() = %v, want ErrYocoUnsupportedCurrency", err)
	}
}

func TestYocoBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"chk_abc","redirectUrl":"https://pay.yoco.com/chk_abc"}`))
	}))
	defer ts.Close()
	p := newTestYocoProvider(ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "ZAR"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" || charge.Reference != "chk_abc" {
		t.Fatalf("unexpected charge: %+v", charge)
	}
}

func TestYocoVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/checkouts/chk_abc") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR"}`))
	}))
	defer ts.Close()
	p := newTestYocoProvider(ts)
	result, err := p.Verify(context.Background(), "chk_abc")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 1000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestYocoVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestYocoProvider(ts)
	_, err := p.Verify(context.Background(), "chk_abc")
	if !errors.Is(err, ErrYocoMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrYocoMalformedResponse", err)
	}
}

func TestYocoVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"oops"}`))
	}))
	defer ts.Close()
	p := newTestYocoProvider(ts)
	_, err := p.Verify(context.Background(), "chk_abc")
	if !errors.Is(err, ErrYocoUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrYocoUnexpectedStatus", err)
	}
}

func TestYocoVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR"}`))
	}))
	defer ts.Close()
	p := newTestYocoProvider(ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Verify(context.Background(), "chk_abc")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

func TestYocoWebhook_ValidSignatureSucceeds(t *testing.T) {
	body := []byte(`{"type":"payment.succeeded","payload":{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR"}}`)
	ts := nowUnixString()
	sig := signYocoWebhook("msg_1", ts, body)
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	result, err := p.Webhook(context.Background(), yocoWebhookRequest(body, "msg_1", ts, sig))
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 1000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestYocoWebhook_MissingHeadersFailsClosed(t *testing.T) {
	body := []byte(`{"type":"payment.succeeded","payload":{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR"}}`)
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	_, err := p.Webhook(context.Background(), yocoWebhookRequest(body, "", "", ""))
	if !errors.Is(err, ErrYocoMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrYocoMissingSignature", err)
	}
}

func TestYocoWebhook_TamperedBodyFailsClosed(t *testing.T) {
	body := []byte(`{"type":"payment.succeeded","payload":{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR"}}`)
	ts := nowUnixString()
	sig := signYocoWebhook("msg_1", ts, body)
	tampered := []byte(`{"type":"payment.succeeded","payload":{"id":"chk_abc","status":"completed","amount":999999999,"currency":"ZAR"}}`)
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	_, err := p.Webhook(context.Background(), yocoWebhookRequest(tampered, "msg_1", ts, sig))
	if !errors.Is(err, ErrYocoInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrYocoInvalidSignature", err)
	}
}

func TestYocoWebhook_WrongSecretFailsClosed(t *testing.T) {
	body := []byte(`{"type":"payment.succeeded","payload":{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR"}}`)
	ts := nowUnixString()
	mac := hmac.New(sha256.New, []byte("some-other-secret-that-is-32-bytes!"))
	mac.Write([]byte("msg_1." + ts + "." + string(body)))
	sig := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	_, err := p.Webhook(context.Background(), yocoWebhookRequest(body, "msg_1", ts, sig))
	if !errors.Is(err, ErrYocoInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrYocoInvalidSignature", err)
	}
}

func TestYocoWebhook_StaleTimestampFailsClosed(t *testing.T) {
	body := []byte(`{"type":"payment.succeeded","payload":{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR"}}`)
	staleTs := strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)
	sig := signYocoWebhook("msg_1", staleTs, body)
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	_, err := p.Webhook(context.Background(), yocoWebhookRequest(body, "msg_1", staleTs, sig))
	if !errors.Is(err, ErrYocoStaleTimestamp) {
		t.Fatalf("Webhook() = %v, want ErrYocoStaleTimestamp", err)
	}
}

func TestYocoWebhook_UnhandledEvent(t *testing.T) {
	body := []byte(`{"type":"payment.failed","payload":{"id":"chk_abc","status":"failed","amount":1000,"currency":"ZAR"}}`)
	ts := nowUnixString()
	sig := signYocoWebhook("msg_1", ts, body)
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	_, err := p.Webhook(context.Background(), yocoWebhookRequest(body, "msg_1", ts, sig))
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook() = %v, want ErrUnhandledEvent", err)
	}
}

func TestYocoWebhook_MalformedJSONFailsClosed(t *testing.T) {
	body := []byte(`{not valid json`)
	ts := nowUnixString()
	sig := signYocoWebhook("msg_1", ts, body)
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	_, err := p.Webhook(context.Background(), yocoWebhookRequest(body, "msg_1", ts, sig))
	if !errors.Is(err, ErrYocoMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrYocoMalformedResponse", err)
	}
}

func TestYocoWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	body := []byte(`{"type":"payment.succeeded","payload":{"id":"chk_777","status":"completed","amount":1000,"currency":"ZAR"}}`)
	ts := nowUnixString()
	sig := signYocoWebhook("msg_777", ts, body)
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"chk_777": {ID: "chk_777", AmountMinor: 1000, Currency: "ZAR"}}}

	req := func() *http.Request { return yocoWebhookRequest(body, "msg_777", ts, sig) }
	_, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, req(), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestYocoWebhook_OversizedBodyRejected(t *testing.T) {
	junk := strings.Repeat("a", yocoMaxResponseSize+1024)
	body := []byte(fmt.Sprintf(`{"type":"payment.succeeded","payload":{"id":"chk_abc","status":"completed","amount":1000,"currency":"ZAR","note":"%s"}}`, junk))
	ts := nowUnixString()
	p := &YocoProvider{webhookSecret: testYocoWebhookSecretRaw}
	_, err := p.Webhook(context.Background(), yocoWebhookRequest(body, "msg_1", ts, "v1,irrelevant"))
	if !errors.Is(err, ErrYocoResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrYocoResponseTooLarge", err)
	}
}
