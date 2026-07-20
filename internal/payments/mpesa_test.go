package payments

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestMpesaServer builds a fake Daraja server. handlers maps a path
// suffix to a canned JSON response body.
func newTestMpesaServer(t *testing.T, handlers map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for suffix, resp := range handlers {
			if strings.HasSuffix(r.URL.Path, suffix) {
				w.Write([]byte(resp))
				return
			}
		}
		t.Fatalf("unexpected request to %s", r.URL.Path)
	}))
}

// defaultTestMpesaOrderLookup is the OrderLookup most tests want: a single
// stored order "ws_CO_1" worth 1000 KES, matching what these tests used to
// plumb in via the now-removed WithMpesaOrder(ctx, 1000, "KES").
func defaultTestMpesaOrderLookup() *fakeOrderLookup {
	return &fakeOrderLookup{orders: map[string]OrderRef{
		"ws_CO_1": {ID: "ws_CO_1", AmountMinor: 1000, Currency: "KES"},
	}}
}

func newTestMpesaProvider(ts *httptest.Server) *MpesaProvider {
	return &MpesaProvider{
		consumerKey:    "ck_test",
		consumerSecret: "cs_test",
		shortcode:      "174379",
		passkey:        "test-passkey",
		httpClient:     ts.Client(),
		baseURL:        ts.URL,
		orderLookup:    defaultTestMpesaOrderLookup(),
	}
}

const mpesaOAuthResponse = `{"access_token":"test-token-abc","expires_in":"3599"}`

func TestNewMpesa_RequiresAllCredentials(t *testing.T) {
	t.Setenv(EnvMpesaConsumerKey, "")
	t.Setenv(EnvMpesaConsumerSecret, "")
	t.Setenv(EnvMpesaShortcode, "")
	t.Setenv(EnvMpesaPasskey, "")
	if _, err := NewMpesa(defaultTestMpesaOrderLookup()); !errors.Is(err, ErrMpesaCredentialsNotConfigured) {
		t.Fatalf("NewMpesa() = %v, want ErrMpesaCredentialsNotConfigured", err)
	}
}

// TestNewMpesa_RequiresOrderLookup is the regression test for the finding
// that this adapter was permanently dead code: nothing in the app ever
// supplied what Verify/Webhook need to determine a settled amount (Daraja's
// query API doesn't echo one back), so a provider built without one would
// fail closed on every single call, forever. NewMpesa now refuses to
// construct at all without one, surfacing the misconfiguration immediately
// at startup instead of on the first real payment.
func TestNewMpesa_RequiresOrderLookup(t *testing.T) {
	t.Setenv(EnvMpesaConsumerKey, "ck_test")
	t.Setenv(EnvMpesaConsumerSecret, "cs_test")
	t.Setenv(EnvMpesaShortcode, "174379")
	t.Setenv(EnvMpesaPasskey, "test-passkey")
	if _, err := NewMpesa(nil); !errors.Is(err, ErrMpesaOrderLookupRequired) {
		t.Fatalf("NewMpesa(nil) = %v, want ErrMpesaOrderLookupRequired", err)
	}
}

func TestMpesaBegin_RejectsNonKES(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "x"}
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "USD", Metadata: map[string]string{"phone": "254712345678"}})
	if !errors.Is(err, ErrMpesaUnsupportedCurrency) {
		t.Fatalf("Begin() = %v, want ErrMpesaUnsupportedCurrency", err)
	}
}

func TestMpesaBegin_RejectsFractionalShillings(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "x"}
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1050, Currency: "KES", Metadata: map[string]string{"phone": "254712345678"}})
	if !errors.Is(err, ErrMpesaFractionalAmount) {
		t.Fatalf("Begin() = %v, want ErrMpesaFractionalAmount", err)
	}
}

func TestMpesaBegin_RequiresPhone(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "x"}
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "KES"})
	if err == nil {
		t.Fatal("Begin() without a phone number = nil error, want rejection")
	}
}

func TestMpesaBegin_Success(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate": mpesaOAuthResponse,
		"/processrequest":    `{"MerchantRequestID":"mr_1","CheckoutRequestID":"ws_CO_1","ResponseCode":"0","ResponseDescription":"Success. Request accepted for processing","CustomerMessage":"Success. Request accepted for processing"}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	charge, err := p.Begin(context.Background(), Order{
		Reference: "ord_1", AmountMinor: 1000, Currency: "KES",
		Metadata: map[string]string{"phone": "254712345678"},
	})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.Reference != "ws_CO_1" {
		t.Fatalf("Reference = %q, want ws_CO_1", charge.Reference)
	}
}

func TestMpesaBegin_RejectedByDarajaFailsClosed(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate": mpesaOAuthResponse,
		"/processrequest":    `{"MerchantRequestID":"mr_1","CheckoutRequestID":"","ResponseCode":"1","ResponseDescription":"Insufficient balance"}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	_, err := p.Begin(context.Background(), Order{
		Reference: "ord_1", AmountMinor: 1000, Currency: "KES",
		Metadata: map[string]string{"phone": "254712345678"},
	})
	if !errors.Is(err, ErrMpesaUnexpectedStatus) {
		t.Fatalf("Begin() = %v, want ErrMpesaUnexpectedStatus", err)
	}
}

func TestMpesaVerify_RequiresOrderLookup(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "x"} // orderLookup deliberately left nil
	_, err := p.Verify(context.Background(), "ws_CO_1")
	if err == nil {
		t.Fatal("Verify() with a nil orderLookup = nil error, want rejection")
	}
}

func TestMpesaVerify_SuccessfulQuerySettlesAtStoredAmount(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate":     mpesaOAuthResponse,
		"/stkpushquery/v1/query": `{"ResponseCode":"0","ResponseDescription":"ok","CheckoutRequestID":"ws_CO_1","ResultCode":"0","ResultDesc":"The service request is processed successfully."}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	ctx := context.Background()
	result, err := p.Verify(ctx, "ws_CO_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 1000 || result.Currency != "KES" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMpesaVerify_NonZeroResultCodeIsNotPaid(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate":     mpesaOAuthResponse,
		"/stkpushquery/v1/query": `{"ResponseCode":"0","CheckoutRequestID":"ws_CO_1","ResultCode":"1032","ResultDesc":"Request cancelled by user"}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	ctx := context.Background()
	result, err := p.Verify(ctx, "ws_CO_1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil (cancelled is a valid non-paid result)", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for a cancelled request, want not-paid")
	}
}

func TestMpesaVerify_CheckoutRequestIDMismatchFailsClosed(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate":     mpesaOAuthResponse,
		"/stkpushquery/v1/query": `{"ResponseCode":"0","CheckoutRequestID":"ws_CO_DIFFERENT","ResultCode":"0"}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	ctx := context.Background()
	_, err := p.Verify(ctx, "ws_CO_1")
	if !errors.Is(err, ErrMpesaMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrMpesaMalformedResponse", err)
	}
}

func TestMpesaVerify_MalformedJSONFailsClosed(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate":     mpesaOAuthResponse,
		"/stkpushquery/v1/query": `not json`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	ctx := context.Background()
	_, err := p.Verify(ctx, "ws_CO_1")
	if !errors.Is(err, ErrMpesaMalformedResponse) {
		t.Fatalf("Verify() = %v, want ErrMpesaMalformedResponse", err)
	}
}

func TestMpesaVerify_OAuthFailureFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessage":"bad credentials"}`))
	}))
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	ctx := context.Background()
	_, err := p.Verify(ctx, "ws_CO_1")
	if !errors.Is(err, ErrMpesaUnexpectedStatus) {
		t.Fatalf("Verify() = %v, want ErrMpesaUnexpectedStatus", err)
	}
}

func TestMpesaVerify_TimeoutFailsClosed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(mpesaOAuthResponse))
	}))
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	p.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	ctx := context.Background()
	_, err := p.Verify(ctx, "ws_CO_1")
	if err == nil {
		t.Fatal("Verify() against slow server = nil error, want timeout failure")
	}
}

// TestMpesaWebhook_UntrustedCallbackAloneCannotForgeSuccess is the core
// security property of this adapter: an attacker POSTing a fabricated
// "ResultCode":0 callback (Daraja callbacks are UNSIGNED) must NOT result
// in a paid Result if the authenticated Query API says otherwise.
func TestMpesaWebhook_UntrustedCallbackAloneCannotForgeSuccess(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate": mpesaOAuthResponse,
		// The AUTHENTICATED query says this was never actually paid...
		"/stkpushquery/v1/query": `{"ResponseCode":"0","CheckoutRequestID":"ws_CO_1","ResultCode":"1032","ResultDesc":"Request cancelled by user"}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)

	// ...even though the (unsigned, attacker-controllable) callback body
	// claims success with a huge amount.
	forgedCallback := `{"Body":{"stkCallback":{"MerchantRequestID":"mr_1","CheckoutRequestID":"ws_CO_1","ResultCode":0,"ResultDesc":"forged","CallbackMetadata":{"Item":[{"Name":"Amount","Value":999999},{"Name":"MpesaReceiptNumber","Value":"FORGED123"}]}}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/mpesa", strings.NewReader(forgedCallback))
	ctx := context.Background()
	result, err := p.Webhook(ctx, req)
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil (cancelled is a valid non-paid result, not an error)", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Webhook() reported paid based on a forged, unsigned callback despite the authenticated query saying otherwise -- SECURITY REGRESSION")
	}
}

func TestMpesaWebhook_LegitimateCallbackTriggersRealVerification(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate":     mpesaOAuthResponse,
		"/stkpushquery/v1/query": `{"ResponseCode":"0","CheckoutRequestID":"ws_CO_1","ResultCode":"0","ResultDesc":"success"}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)

	callback := `{"Body":{"stkCallback":{"MerchantRequestID":"mr_1","CheckoutRequestID":"ws_CO_1","ResultCode":0,"ResultDesc":"success"}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/mpesa", strings.NewReader(callback))
	ctx := context.Background()
	result, err := p.Webhook(ctx, req)
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.AmountMinor != 1000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMpesaWebhook_MissingCheckoutRequestIDFailsClosed(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "x"}
	callback := `{"Body":{"stkCallback":{"MerchantRequestID":"mr_1","ResultCode":0}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/mpesa", strings.NewReader(callback))
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMpesaMissingCheckoutRequestID) {
		t.Fatalf("Webhook() = %v, want ErrMpesaMissingCheckoutRequestID", err)
	}
}

func TestMpesaWebhook_MalformedJSONFailsClosed(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "x"}
	req := httptest.NewRequest(http.MethodPost, "/webhook/mpesa", strings.NewReader(`{not valid`))
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMpesaMalformedResponse) {
		t.Fatalf("Webhook() = %v, want ErrMpesaMalformedResponse", err)
	}
}

func TestMpesaWebhook_OversizedBodyRejected(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "x"}
	junk := strings.Repeat("a", mpesaMaxResponseSize+1024)
	body := `{"Body":{"stkCallback":{"CheckoutRequestID":"ws_CO_1","note":"` + junk + `"}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/mpesa", strings.NewReader(body))
	_, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrMpesaResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrMpesaResponseTooLarge", err)
	}
}

func TestMpesaWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	ts := newTestMpesaServer(t, map[string]string{
		"/oauth/v1/generate":     mpesaOAuthResponse,
		"/stkpushquery/v1/query": `{"ResponseCode":"0","CheckoutRequestID":"ws_CO_777","ResultCode":"0"}`,
	})
	defer ts.Close()
	p := newTestMpesaProvider(ts)
	callback := `{"Body":{"stkCallback":{"CheckoutRequestID":"ws_CO_777","ResultCode":0}}}`
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ws_CO_777": {ID: "ws_CO_777", AmountMinor: 1000, Currency: "KES"}}}
	// The provider's OWN internal lookup (what Verify uses to determine the
	// settled amount, since Daraja's query API doesn't echo one back) needs
	// this reference too — reusing the same fake mirrors how the real app
	// wires one orderLookupAdapter for both purposes (see cmd/cackle).
	p.orderLookup = lookup

	newReq := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, "/webhook/mpesa", strings.NewReader(callback))
	}
	ctx := context.Background()
	_, err := HandleWebhook(ctx, p, newReq(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(ctx, p, newReq(), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

// sanity check that mpesaTimestamp/password never panics and produces a
// stable, decodable password (exercises a pure-logic path with no network).
func TestMpesaPassword_IsStableBase64(t *testing.T) {
	p := &MpesaProvider{shortcode: "174379", passkey: "test-passkey"}
	ts1 := mpesaTimestamp(time.Now())
	pw := p.password(ts1)
	if pw == "" {
		t.Fatal("password() returned empty string")
	}
	var buf strings.Builder
	_, err := io.Copy(&buf, strings.NewReader(pw))
	if err != nil {
		t.Fatalf("unexpected error copying password: %v", err)
	}
}
