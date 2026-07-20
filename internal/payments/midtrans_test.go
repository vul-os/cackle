package payments

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testMidtransServerKey = "SB-Mid-server-fake-key"

func newTestMidtransProvider(ts *httptest.Server) *MidtransProvider {
	return &MidtransProvider{
		serverKey:   testMidtransServerKey,
		snapClient:  ts.Client(),
		coreClient:  ts.Client(),
		snapBaseURL: ts.URL,
		coreBaseURL: ts.URL,
	}
}

func signMidtransNotification(orderID, statusCode, grossAmount string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + testMidtransServerKey))
	return hex.EncodeToString(sum[:])
}

func midtransWebhookRequest(payload map[string]any) *http.Request {
	b, _ := json.Marshal(payload)
	return httptest.NewRequest(http.MethodPost, "/api/payments/webhook/midtrans", strings.NewReader(string(b)))
}

func TestNewMidtrans_RequiresServerKey(t *testing.T) {
	t.Setenv(EnvMidtransServerKey, "")
	if _, err := NewMidtrans(); !errors.Is(err, ErrMidtransServerKeyNotConfigured) {
		t.Fatalf("NewMidtrans() = %v, want ErrMidtransServerKeyNotConfigured", err)
	}
}

func TestMidtransBegin_RejectsNonIDR(t *testing.T) {
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "USD"})
	if !errors.Is(err, ErrMidtransUnsupportedCurrency) {
		t.Fatalf("Begin() = %v, want ErrMidtransUnsupportedCurrency", err)
	}
}

func TestMidtransBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok_abc","redirect_url":"https://app.midtrans.com/snap/v3/redirection/tok_abc"}`))
	}))
	defer ts.Close()
	p := newTestMidtransProvider(ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "IDR", BuyerEmail: "a@b.com"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" {
		t.Fatal("empty RedirectURL")
	}
}

func TestMidtransVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/ord_1/status") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"order_id":"ord_1","transaction_id":"txn_1","transaction_status":"settlement","gross_amount":"10000.00","currency":"IDR","status_code":"200","settlement_time":"2026-07-20 10:00:00"}`))
	}))
	defer ts.Close()
	p := newTestMidtransProvider(ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 1000000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMidtransVerify_CaptureWithoutFraudAcceptIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"order_id":"ord_1","transaction_id":"txn_1","transaction_status":"capture","fraud_status":"challenge","gross_amount":"10000.00","currency":"IDR"}`))
	}))
	defer ts.Close()
	p := newTestMidtransProvider(ts)
	result, err := p.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for capture+challenge, want not-paid")
	}
}

func TestMidtransVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestMidtransProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrMidtransMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrMidtransMalformedResponse", err)
	}
}

func TestMidtransVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status_message":"error"}`))
	}))
	defer ts.Close()
	p := newTestMidtransProvider(ts)
	_, err := p.Verify(context.Background(), "ord_1")
	if !errors.Is(err, ErrMidtransUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrMidtransUnexpectedStatus", err)
	}
}

func TestMidtransVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"order_id":"ord_1","transaction_status":"settlement","gross_amount":"10000.00"}`))
	}))
	defer ts.Close()
	p := newTestMidtransProvider(ts)
	p.coreClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Verify(context.Background(), "ord_1")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

func TestMidtransWebhook_ValidSignatureSucceeds(t *testing.T) {
	sig := signMidtransNotification("ord_1", "200", "10000.00")
	req := midtransWebhookRequest(map[string]any{
		"order_id": "ord_1", "transaction_id": "txn_1", "transaction_status": "settlement",
		"gross_amount": "10000.00", "currency": "IDR", "status_code": "200", "signature_key": sig,
		"settlement_time": "2026-07-20 10:00:00",
	})
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.Reference != "ord_1" || result.AmountMinor != 1000000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMidtransWebhook_MissingSignatureFailsClosed(t *testing.T) {
	req := midtransWebhookRequest(map[string]any{
		"order_id": "ord_1", "transaction_status": "settlement", "gross_amount": "10000.00", "status_code": "200",
	})
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMidtransMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrMidtransMissingSignature", err)
	}
}

func TestMidtransWebhook_TamperedAmountFailsClosed(t *testing.T) {
	// Signature was computed for gross_amount 10000.00; attacker changes
	// the amount after the fact -- signature must no longer validate.
	sig := signMidtransNotification("ord_1", "200", "10000.00")
	req := midtransWebhookRequest(map[string]any{
		"order_id": "ord_1", "transaction_status": "settlement", "gross_amount": "1.00",
		"status_code": "200", "signature_key": sig,
	})
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMidtransInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrMidtransInvalidSignature", err)
	}
}

func TestMidtransWebhook_WrongServerKeyFailsClosed(t *testing.T) {
	sum := sha512.Sum512([]byte("ord_1" + "200" + "10000.00" + "some-other-key"))
	sig := hex.EncodeToString(sum[:])
	req := midtransWebhookRequest(map[string]any{
		"order_id": "ord_1", "transaction_status": "settlement", "gross_amount": "10000.00",
		"status_code": "200", "signature_key": sig,
	})
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMidtransInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrMidtransInvalidSignature", err)
	}
}

func TestMidtransWebhook_MalformedJSONFailsClosed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{not valid`))
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMidtransMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrMidtransMalformedResponse", err)
	}
}

func TestMidtransWebhook_DenyStatusIsNotPaid(t *testing.T) {
	sig := signMidtransNotification("ord_1", "200", "10000.00")
	req := midtransWebhookRequest(map[string]any{
		"order_id": "ord_1", "transaction_status": "deny", "gross_amount": "10000.00",
		"status_code": "200", "signature_key": sig,
	})
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for deny, want not-paid")
	}
}

func TestMidtransWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	sig := signMidtransNotification("ord_1", "200", "10000.00")
	req := func() *http.Request {
		return midtransWebhookRequest(map[string]any{
			"order_id": "ord_1", "transaction_id": "txn_777", "transaction_status": "settlement",
			"gross_amount": "10000.00", "currency": "IDR", "status_code": "200", "signature_key": sig,
		})
	}
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 1000000, Currency: "IDR"}}}

	_, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, req(), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestMidtransWebhook_OversizedBodyRejected(t *testing.T) {
	junk := strings.Repeat("a", midtransMaxResponseSize+1024)
	req := midtransWebhookRequest(map[string]any{
		"order_id": "ord_1", "transaction_status": "settlement", "gross_amount": "10000.00",
		"status_code": "200", "signature_key": "x", "note": junk,
	})
	p := &MidtransProvider{serverKey: testMidtransServerKey}
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMidtransResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrMidtransResponseTooLarge", err)
	}
}
