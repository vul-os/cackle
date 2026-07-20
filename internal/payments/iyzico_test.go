package payments

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestIyzicoProvider(ts *httptest.Server) *IyzicoProvider {
	return &IyzicoProvider{
		apiKey:     "test-api-key",
		secretKey:  "test-secret-key",
		httpClient: ts.Client(),
		baseURL:    ts.URL,
	}
}

func iyzicoCallbackRequest(token string) *http.Request {
	form := url.Values{"token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/iyzico", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestNewIyzico_RequiresCredentials(t *testing.T) {
	t.Setenv(EnvIyzicoAPIKey, "")
	t.Setenv(EnvIyzicoSecretKey, "")
	if _, err := NewIyzico(); !errors.Is(err, ErrIyzicoCredentialsNotConfigured) {
		t.Fatalf("NewIyzico() = %v, want ErrIyzicoCredentialsNotConfigured", err)
	}
}

func TestIyzicoBegin_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" || r.Header.Get("x-iyzi-rnd") == "" {
			t.Fatal("missing IYZWS auth headers")
		}
		w.Write([]byte(`{"status":"success","token":"tok_abc","paymentPageUrl":"https://sandbox-cpp.iyzipay.com/tok_abc"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "TRY", BuyerEmail: "a@b.com"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.Reference != "tok_abc" || charge.RedirectURL == "" {
		t.Fatalf("unexpected charge: %+v", charge)
	}
}

func TestIyzicoBegin_FailureStatusFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"failure","errorMessage":"invalid signature"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "TRY", BuyerEmail: "a@b.com"})
	if !errors.Is(err, ErrIyzicoUnexpectedStatus) {
		t.Fatalf("Begin() = %v, want ErrIyzicoUnexpectedStatus", err)
	}
}

func TestIyzicoVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","paymentStatus":"SUCCESS","token":"tok_abc","paymentId":"pay_1","paidPrice":"100.00","currency":"TRY","basketId":"ord_1"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	result, err := p.Verify(context.Background(), "tok_abc")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 10000 || result.Reference != "tok_abc" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestIyzicoVerify_FailurePaymentStatusIsNotPaid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","paymentStatus":"FAILURE","token":"tok_abc","currency":"TRY"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	result, err := p.Verify(context.Background(), "tok_abc")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for FAILURE paymentStatus, want not-paid")
	}
}

func TestIyzicoVerify_APIFailureStatusFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"failure","errorMessage":"token not found"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	_, err := p.Verify(context.Background(), "tok_missing")
	if !errors.Is(err, ErrIyzicoUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrIyzicoUnexpectedStatus", err)
	}
}

func TestIyzicoVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	_, err := p.Verify(context.Background(), "tok_abc")
	if !errors.Is(err, ErrIyzicoMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrIyzicoMalformedResponse", err)
	}
}

func TestIyzicoVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	_, err := p.Verify(context.Background(), "tok_abc")
	if !errors.Is(err, ErrIyzicoUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrIyzicoUnexpectedStatus", err)
	}
}

func TestIyzicoVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"status":"success","paymentStatus":"SUCCESS"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Verify(context.Background(), "tok_abc")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

// TestIyzicoWebhook_UnsignedCallbackAloneCannotForgeSuccess guards the same
// property several unsigned-callback adapters rely on: the callback itself
// carries no verifiable status, only a token, so a forged callback claiming
// success must not matter -- only the authenticated retrieve call's answer does.
func TestIyzicoWebhook_UnsignedCallbackAloneCannotForgeSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Authenticated retrieve says FAILURE...
		w.Write([]byte(`{"status":"success","paymentStatus":"FAILURE","token":"tok_abc","currency":"TRY"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)

	req := iyzicoCallbackRequest("tok_abc")
	result, err := p.Webhook(context.Background(), req)
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Webhook() reported paid based on the callback alone -- SECURITY REGRESSION")
	}
}

func TestIyzicoWebhook_LegitimateCallbackSucceeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","paymentStatus":"SUCCESS","token":"tok_abc","paymentId":"pay_1","paidPrice":"100.00","currency":"TRY"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	result, err := p.Webhook(context.Background(), iyzicoCallbackRequest("tok_abc"))
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 10000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestIyzicoWebhook_MissingTokenFailsClosed(t *testing.T) {
	p := &IyzicoProvider{apiKey: "x", secretKey: "y"}
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrIyzicoMissingToken) {
		t.Fatalf("Webhook() = %v, want ErrIyzicoMissingToken", err)
	}
}

func TestIyzicoWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","paymentStatus":"SUCCESS","token":"tok_777","paymentId":"pay_777","paidPrice":"100.00","currency":"TRY"}`))
	}))
	defer ts.Close()
	p := newTestIyzicoProvider(ts)
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"tok_777": {ID: "tok_777", AmountMinor: 10000, Currency: "TRY"}}}

	req := func() *http.Request { return iyzicoCallbackRequest("tok_777") }
	_, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, req(), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestIyzicoWebhook_OversizedBodyRejected(t *testing.T) {
	p := &IyzicoProvider{apiKey: "x", secretKey: "y"}
	junk := strings.Repeat("a", iyzicoMaxResponseSize+1024)
	body := "token=tok_abc&note=" + junk
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrIyzicoResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrIyzicoResponseTooLarge", err)
	}
}
