package payments

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestPayUProvider(ts *httptest.Server) *PayUProvider {
	p := &PayUProvider{
		merchantKey: "gtKFFx",
		salt:        "eCwWELxi",
		httpClient:  http.DefaultClient,
		checkoutURL: payUCheckoutURL,
		verifyURL:   payUVerifyURL,
	}
	if ts != nil {
		p.httpClient = ts.Client()
		p.verifyURL = ts.URL
	}
	return p
}

func payUWebhookRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/payu", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestNewPayU_RequiresCredentials(t *testing.T) {
	t.Setenv(EnvPayUMerchantKey, "")
	t.Setenv(EnvPayUSalt, "")
	if _, err := NewPayU(); !errors.Is(err, ErrPayUCredentialsNotConfigured) {
		t.Fatalf("NewPayU() = %v, want ErrPayUCredentialsNotConfigured", err)
	}
}

func TestPayUBegin_RejectsNonINR(t *testing.T) {
	p := newTestPayUProvider(nil)
	_, err := p.Begin(context.Background(), Order{Reference: "txn_1", AmountMinor: 1000, Currency: "USD", BuyerEmail: "a@b.com"})
	if err == nil {
		t.Fatal("Begin() with non-INR currency = nil error, want rejection")
	}
}

func TestPayUBegin_Success(t *testing.T) {
	p := newTestPayUProvider(nil)
	charge, err := p.Begin(context.Background(), Order{Reference: "txn_1", AmountMinor: 10000, Currency: "INR", BuyerEmail: "a@b.com", BuyerName: "Jane"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" {
		t.Fatal("empty RedirectURL")
	}
	values, err := url.ParseQuery(charge.Instructions)
	if err != nil {
		t.Fatalf("Instructions did not parse as a query string: %v", err)
	}
	if values.Get("hash") == "" {
		t.Fatal("Instructions missing hash field")
	}
	if values.Get("amount") != "100.00" {
		t.Fatalf("amount = %q, want 100.00", values.Get("amount"))
	}
}

func TestPayUWebhook_ValidHashSucceeds(t *testing.T) {
	p := newTestPayUProvider(nil)
	var udf [5]string
	hash := p.responseHash("success", "txn_1", "100.00", "Order txn_1", "Jane", "a@b.com", udf)
	form := url.Values{
		"status": {"success"}, "txnid": {"txn_1"}, "amount": {"100.00"},
		"productinfo": {"Order txn_1"}, "firstname": {"Jane"}, "email": {"a@b.com"},
		"mihpayid": {"mihpay123"}, "hash": {hash},
	}
	result, err := p.Webhook(context.Background(), payUWebhookRequest(form.Encode()))
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.Reference != "txn_1" || result.AmountMinor != 10000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestPayUWebhook_MissingHashFailsClosed(t *testing.T) {
	p := newTestPayUProvider(nil)
	form := url.Values{"status": {"success"}, "txnid": {"txn_1"}, "amount": {"100.00"}}
	_, err := p.Webhook(context.Background(), payUWebhookRequest(form.Encode()))
	if !errors.Is(err, ErrPayUMissingHash) {
		t.Fatalf("Webhook() = %v, want ErrPayUMissingHash", err)
	}
}

func TestPayUWebhook_TamperedAmountFailsClosed(t *testing.T) {
	p := newTestPayUProvider(nil)
	var udf [5]string
	hash := p.responseHash("success", "txn_1", "100.00", "Order txn_1", "Jane", "a@b.com", udf)
	// Attacker changes amount after the hash was computed.
	form := url.Values{
		"status": {"success"}, "txnid": {"txn_1"}, "amount": {"999999.00"},
		"productinfo": {"Order txn_1"}, "firstname": {"Jane"}, "email": {"a@b.com"},
		"mihpayid": {"mihpay123"}, "hash": {hash},
	}
	_, err := p.Webhook(context.Background(), payUWebhookRequest(form.Encode()))
	if !errors.Is(err, ErrPayUInvalidHash) {
		t.Fatalf("Webhook() = %v, want ErrPayUInvalidHash", err)
	}
}

func TestPayUWebhook_WrongSaltFailsClosed(t *testing.T) {
	signer := newTestPayUProvider(nil)
	signer.salt = "different-salt"
	var udf [5]string
	hash := signer.responseHash("success", "txn_1", "100.00", "Order txn_1", "Jane", "a@b.com", udf)
	form := url.Values{
		"status": {"success"}, "txnid": {"txn_1"}, "amount": {"100.00"},
		"productinfo": {"Order txn_1"}, "firstname": {"Jane"}, "email": {"a@b.com"},
		"mihpayid": {"mihpay123"}, "hash": {hash},
	}
	verifier := newTestPayUProvider(nil) // salt "eCwWELxi"
	_, err := verifier.Webhook(context.Background(), payUWebhookRequest(form.Encode()))
	if !errors.Is(err, ErrPayUInvalidHash) {
		t.Fatalf("Webhook() = %v, want ErrPayUInvalidHash", err)
	}
}

func TestPayUWebhook_FailureStatusIsNotPaid(t *testing.T) {
	p := newTestPayUProvider(nil)
	var udf [5]string
	hash := p.responseHash("failure", "txn_1", "100.00", "Order txn_1", "Jane", "a@b.com", udf)
	form := url.Values{
		"status": {"failure"}, "txnid": {"txn_1"}, "amount": {"100.00"},
		"productinfo": {"Order txn_1"}, "firstname": {"Jane"}, "email": {"a@b.com"},
		"hash": {hash},
	}
	result, err := p.Webhook(context.Background(), payUWebhookRequest(form.Encode()))
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for failure, want not-paid")
	}
}

func TestPayUWebhook_MalformedBodyFailsClosed(t *testing.T) {
	p := newTestPayUProvider(nil)
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("%zzzzz"))
	_, err := p.Webhook(context.Background(), req)
	if err == nil {
		t.Fatal("Webhook() with malformed form body = nil error, want failure")
	}
}

func TestPayUWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	p := newTestPayUProvider(nil)
	var udf [5]string
	hash := p.responseHash("success", "txn_1", "100.00", "Order txn_1", "Jane", "a@b.com", udf)
	form := url.Values{
		"status": {"success"}, "txnid": {"txn_1"}, "amount": {"100.00"},
		"productinfo": {"Order txn_1"}, "firstname": {"Jane"}, "email": {"a@b.com"},
		"mihpayid": {"mihpay_777"}, "hash": {hash},
	}
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"txn_1": {ID: "txn_1", AmountMinor: 10000, Currency: "INR"}}}

	req := func() *http.Request { return payUWebhookRequest(form.Encode()) }
	_, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, req(), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestPayUWebhook_AmountMismatchFailsClosed(t *testing.T) {
	p := newTestPayUProvider(nil)
	var udf [5]string
	hash := p.responseHash("success", "txn_1", "100.00", "Order txn_1", "Jane", "a@b.com", udf)
	form := url.Values{
		"status": {"success"}, "txnid": {"txn_1"}, "amount": {"100.00"},
		"productinfo": {"Order txn_1"}, "firstname": {"Jane"}, "email": {"a@b.com"},
		"mihpayid": {"mihpay_778"}, "hash": {hash},
	}
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"txn_1": {ID: "txn_1", AmountMinor: 1, Currency: "INR"}}}
	_, err := HandleWebhook(context.Background(), p, payUWebhookRequest(form.Encode()), nil, lookup)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrAmountMismatch", err)
	}
}

func TestPayUWebhook_OversizedBodyRejected(t *testing.T) {
	p := newTestPayUProvider(nil)
	junk := strings.Repeat("a", payUMaxResponseSize+1024)
	body := "status=success&txnid=txn_1&amount=100.00&note=" + junk + "&hash=irrelevant"
	_, err := p.Webhook(context.Background(), payUWebhookRequest(body))
	if !errors.Is(err, ErrPayUResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrPayUResponseTooLarge", err)
	}
}

func TestPayUVerify_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":1,"transaction_details":{"txn_1":{"mihpayid":"mihpay123","status":"success","txnid":"txn_1","amt":"100.00"}}}`))
	}))
	defer ts.Close()
	p := newTestPayUProvider(ts)
	result, err := p.Verify(context.Background(), "txn_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 10000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestPayUVerify_NotFoundFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":1,"transaction_details":{}}`))
	}))
	defer ts.Close()
	p := newTestPayUProvider(ts)
	_, err := p.Verify(context.Background(), "txn_missing")
	if !errors.Is(err, ErrPayUTransactionNotFound) {
		t.Fatalf("Verify() = %v, want ErrPayUTransactionNotFound", err)
	}
}

func TestPayUVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	p := newTestPayUProvider(ts)
	_, err := p.Verify(context.Background(), "txn_1")
	if !errors.Is(err, ErrPayUMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrPayUMalformedResponse", err)
	}
}

func TestPayUVerify_Provider500FailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	p := newTestPayUProvider(ts)
	_, err := p.Verify(context.Background(), "txn_1")
	if !errors.Is(err, ErrPayUUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrPayUUnexpectedStatus", err)
	}
}
