package payments

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestPayFastProvider(validateTS *httptest.Server) *PayFastProvider {
	p := &PayFastProvider{
		merchantID:  "10000100",
		merchantKey: "46f0cd694581a",
		passphrase:  "test-passphrase",
		httpClient:  http.DefaultClient,
		processURL:  payFastProcessURL,
		validateURL: payFastValidateURL,
	}
	if validateTS != nil {
		p.httpClient = validateTS.Client()
		p.validateURL = validateTS.URL
	}
	return p
}

// buildSignedPayFastBody constructs a raw x-www-form-urlencoded ITN body
// with a valid signature for the given provider, in field order (PayFast
// signs in the order fields were sent, not alphabetically).
func buildSignedPayFastBody(p *PayFastProvider, fields []payFastKV) []byte {
	sig := p.payFastSignature(fields)
	var b strings.Builder
	for i, kv := range fields {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(kv.key)
		b.WriteByte('=')
		b.WriteString(kv.value)
	}
	b.WriteString("&signature=")
	b.WriteString(sig)
	return []byte(b.String())
}

func payFastWebhookRequest(body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/payfast", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestNewPayFast_RequiresCredentials(t *testing.T) {
	t.Setenv(EnvPayFastMerchantID, "")
	t.Setenv(EnvPayFastMerchantKey, "")
	if _, err := NewPayFast(); !errors.Is(err, ErrPayFastCredentialsNotConfigured) {
		t.Fatalf("NewPayFast() = %v, want ErrPayFastCredentialsNotConfigured", err)
	}
}

func TestPayFastBegin_RejectsNonZAR(t *testing.T) {
	p := newTestPayFastProvider(nil)
	_, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 1000, Currency: "USD"})
	if !errors.Is(err, ErrPayFastUnsupportedCurrency) {
		t.Fatalf("Begin() = %v, want ErrPayFastUnsupportedCurrency", err)
	}
}

func TestPayFastBegin_Success(t *testing.T) {
	p := newTestPayFastProvider(nil)
	charge, err := p.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 10000, Currency: "ZAR", CallbackURL: "https://example.com/return"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.RedirectURL == "" {
		t.Fatal("empty RedirectURL")
	}
	if !strings.Contains(charge.Instructions, "signature=") {
		t.Fatal("Instructions should carry a signed field set for a form POST")
	}
	if !strings.Contains(charge.Instructions, "amount=100.00") {
		t.Fatalf("Instructions = %q, want amount=100.00", charge.Instructions)
	}
}

func TestPayFastWebhook_ValidSignatureAndValidateSucceeds(t *testing.T) {
	validateTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("VALID"))
	}))
	defer validateTS.Close()
	p := newTestPayFastProvider(validateTS)

	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"pf_payment_id", "pf_123"},
		{"payment_status", "COMPLETE"},
		{"amount_gross", "100.00"},
	}
	body := buildSignedPayFastBody(p, fields)
	result, err := p.Webhook(context.Background(), payFastWebhookRequest(body))
	if err != nil {
		t.Fatalf("Webhook() = %v", err)
	}
	if result.Status != StatusPaid || result.Reference != "ord_1" || result.AmountMinor != 10000 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestPayFastWebhook_MissingSignatureFailsClosed(t *testing.T) {
	p := newTestPayFastProvider(nil)
	body := []byte("m_payment_id=ord_1&payment_status=COMPLETE&amount_gross=100.00")
	_, err := p.Webhook(context.Background(), payFastWebhookRequest(body))
	if !errors.Is(err, ErrPayFastMissingSignature) {
		t.Fatalf("Webhook() = %v, want ErrPayFastMissingSignature", err)
	}
}

func TestPayFastWebhook_TamperedFieldFailsClosed(t *testing.T) {
	p := newTestPayFastProvider(nil)
	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"payment_status", "COMPLETE"},
		{"amount_gross", "100.00"},
	}
	sig := p.payFastSignature(fields)
	// Attacker changes amount_gross after the signature was computed.
	tampered := "m_payment_id=ord_1&payment_status=COMPLETE&amount_gross=999999.00&signature=" + sig
	_, err := p.Webhook(context.Background(), payFastWebhookRequest([]byte(tampered)))
	if !errors.Is(err, ErrPayFastInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrPayFastInvalidSignature", err)
	}
}

func TestPayFastWebhook_WrongPassphraseFailsClosed(t *testing.T) {
	signer := newTestPayFastProvider(nil)
	signer.passphrase = "different-passphrase"
	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"payment_status", "COMPLETE"},
		{"amount_gross", "100.00"},
	}
	body := buildSignedPayFastBody(signer, fields)

	verifier := newTestPayFastProvider(nil) // passphrase "test-passphrase"
	_, err := verifier.Webhook(context.Background(), payFastWebhookRequest(body))
	if !errors.Is(err, ErrPayFastInvalidSignature) {
		t.Fatalf("Webhook() = %v, want ErrPayFastInvalidSignature", err)
	}
}

func TestPayFastWebhook_ValidateNotValidFailsClosed(t *testing.T) {
	validateTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("INVALID"))
	}))
	defer validateTS.Close()
	p := newTestPayFastProvider(validateTS)

	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"payment_status", "COMPLETE"},
		{"amount_gross", "100.00"},
	}
	body := buildSignedPayFastBody(p, fields)
	_, err := p.Webhook(context.Background(), payFastWebhookRequest(body))
	if !errors.Is(err, ErrPayFastNotValidated) {
		t.Fatalf("Webhook() = %v, want ErrPayFastNotValidated", err)
	}
}

func TestPayFastWebhook_Validate500FailsClosed(t *testing.T) {
	validateTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer validateTS.Close()
	p := newTestPayFastProvider(validateTS)

	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"payment_status", "COMPLETE"},
		{"amount_gross", "100.00"},
	}
	body := buildSignedPayFastBody(p, fields)
	_, err := p.Webhook(context.Background(), payFastWebhookRequest(body))
	if !errors.Is(err, ErrPayFastUnexpectedStatus) {
		t.Fatalf("Webhook() = %v, want ErrPayFastUnexpectedStatus", err)
	}
}

func TestPayFastWebhook_FailedStatusIsNotPaid(t *testing.T) {
	validateTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("VALID"))
	}))
	defer validateTS.Close()
	p := newTestPayFastProvider(validateTS)

	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"payment_status", "FAILED"},
		{"amount_gross", "100.00"},
	}
	body := buildSignedPayFastBody(p, fields)
	result, err := p.Webhook(context.Background(), payFastWebhookRequest(body))
	if err != nil {
		t.Fatalf("Webhook() = %v, want nil", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("Status = paid for FAILED, want not-paid")
	}
}

func TestPayFastWebhook_ReplayedThroughHandleWebhook(t *testing.T) {
	validateTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("VALID"))
	}))
	defer validateTS.Close()
	p := newTestPayFastProvider(validateTS)

	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"pf_payment_id", "pf_777"},
		{"payment_status", "COMPLETE"},
		{"amount_gross", "100.00"},
	}
	body := buildSignedPayFastBody(p, fields)
	seen := newFakeSeenStore()
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 10000, Currency: "ZAR"}}}

	req := func() *http.Request { return payFastWebhookRequest(body) }
	_, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: HandleWebhook() = %v, want nil", err)
	}
	_, err = HandleWebhook(context.Background(), p, req(), seen, lookup)
	if !errors.Is(err, ErrReplayed) {
		t.Fatalf("second delivery: HandleWebhook() = %v, want ErrReplayed", err)
	}
}

func TestPayFastWebhook_AmountMismatchFailsClosed(t *testing.T) {
	validateTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("VALID"))
	}))
	defer validateTS.Close()
	p := newTestPayFastProvider(validateTS)

	fields := []payFastKV{
		{"m_payment_id", "ord_1"},
		{"pf_payment_id", "pf_778"},
		{"payment_status", "COMPLETE"},
		{"amount_gross", "100.00"},
	}
	body := buildSignedPayFastBody(p, fields)
	lookup := &fakeOrderLookup{orders: map[string]OrderRef{"ord_1": {ID: "ord_1", AmountMinor: 1, Currency: "ZAR"}}}
	_, err := HandleWebhook(context.Background(), p, payFastWebhookRequest(body), nil, lookup)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("HandleWebhook() = %v, want ErrAmountMismatch", err)
	}
}

func TestPayFastWebhook_OversizedBodyRejected(t *testing.T) {
	p := newTestPayFastProvider(nil)
	junk := strings.Repeat("a", payFastMaxResponseSize+1024)
	body := "m_payment_id=ord_1&payment_status=COMPLETE&amount_gross=100.00&note=" + junk + "&signature=irrelevant"
	_, err := p.Webhook(context.Background(), payFastWebhookRequest([]byte(body)))
	if !errors.Is(err, ErrPayFastResponseTooLarge) {
		t.Fatalf("Webhook() = %v, want ErrPayFastResponseTooLarge", err)
	}
}

func TestPayFastVerify_NotSupported(t *testing.T) {
	p := newTestPayFastProvider(nil)
	_, err := p.Verify(context.Background(), "ord_1")
	if err == nil {
		t.Fatal("Verify() = nil error, want an explicit not-supported error")
	}
}
