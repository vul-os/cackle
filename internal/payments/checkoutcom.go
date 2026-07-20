// Package payments — Checkout.com adapter.
//
// Built against Checkout.com's DOCUMENTED public API, verified live against
// checkout.com/docs during this change. No sandbox/live account was used —
// see the test file, which exercises this adapter entirely against an
// httptest fake server, and the HONESTY note below on the one part of this
// file (the exact Hosted Payments Page request field list) that could not
// be fully confirmed line-by-line.
//
// Doc sources:
//   - Hosted Payments Page: https://checkout.com/docs/payments/accept-payments/accept-a-payment-on-a-hosted-page/manage-your-hosted-payments-page
//   - Get payment details:  https://checkout.com/docs/payments/manage-payments/get-payment-details
//   - Webhooks setup:       https://checkout.com/docs/developer-resources/webhooks/manage-webhooks/set-up-your-webhook-receiver
//   - payment_captured event: https://checkout.com/docs/developer-resources/webhooks/webhook-event-types/payment_captured
//   - Currency minor units: https://checkout.com/docs/developer-resources/testing/codes/calculating-the-amount
//
// See stripe.go's package doc comment for the assumed v2 Order/Result field
// shape every P1 adapter in this change codes against.
//
// # HONESTY note: Hosted Payments Page request fields
//
// Checkout.com's interactive API reference for POST /hosted-payments is
// JS-rendered and could not be fetched field-by-field during this change.
// The request fields below (amount, currency, reference, success_url,
// failure_url, cancel_url) are Checkout.com's standard, well-documented
// naming convention used consistently across their Payments API elsewhere
// (https://checkout.com/docs/payments/accept-payments), but were not
// independently confirmed against the Hosted Payments Page endpoint's own
// schema page specifically. The response shape (`id`, `_links.redirect.href`)
// WAS confirmed directly. If field names differ once this is checked
// against a sandbox, Begin is the only method that needs adjusting — Verify
// and Webhook were confirmed against real documented examples and are
// higher confidence.
package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProviderNameCheckoutCom is the stable Name() this provider registers under.
const ProviderNameCheckoutCom = "checkoutcom"

// Env vars. No defaults. CACKLE_CHECKOUTCOM_API_BASE_URL must be either
// https://api.checkout.com (live) or https://api.sandbox.checkout.com
// (sandbox) — required explicitly rather than assumed, so a
// misconfiguration can never silently point at the wrong environment.
const (
	EnvCheckoutComSecretKey  = "CACKLE_CHECKOUTCOM_SECRET_KEY"
	EnvCheckoutComWebhookKey = "CACKLE_CHECKOUTCOM_WEBHOOK_SECRET"
	EnvCheckoutComAPIBaseURL = "CACKLE_CHECKOUTCOM_API_BASE_URL"
)

const (
	checkoutComHTTPTimeout  = 15 * time.Second
	checkoutComMaxBodyBytes = 1 << 20 // 1 MiB
)

// Sentinel errors specific to the Checkout.com adapter. Error strings
// never contain the secret key or webhook secret.
var (
	ErrCheckoutComSecretNotConfigured  = errors.New("payments: checkoutcom: " + EnvCheckoutComSecretKey + " not set")
	ErrCheckoutComWebhookKeyMissing    = errors.New("payments: checkoutcom: " + EnvCheckoutComWebhookKey + " not set")
	ErrCheckoutComBaseURLNotConfigured = errors.New("payments: checkoutcom: " + EnvCheckoutComAPIBaseURL + " not set")
	ErrCheckoutComMissingSignature     = errors.New("payments: checkoutcom: missing Cko-Signature header")
	ErrCheckoutComInvalidSignature     = errors.New("payments: checkoutcom: invalid webhook signature")
	ErrCheckoutComUnexpectedStatus     = errors.New("payments: checkoutcom: unexpected API response status")
	ErrCheckoutComMalformedResponse    = errors.New("payments: checkoutcom: malformed API response")
	ErrCheckoutComResponseTooLarge     = errors.New("payments: checkoutcom: response body exceeds size limit")
)

// checkoutComZeroDecimalCurrencies: amount is the whole-unit count, no
// multiplication — matches Cackle's own AmountMinor for these currencies
// directly. Source: https://checkout.com/docs/developer-resources/testing/codes/calculating-the-amount
//
// NOTE this list includes ISK, which Stripe (see stripe.go) treats
// DIFFERENTLY (forced two-decimal, ×100) — providers genuinely disagree on
// this currency, which is exactly why this file keeps its own independent
// table rather than sharing one across adapters.
var checkoutComZeroDecimalCurrencies = map[string]bool{
	"BIF": true, "DJF": true, "GNF": true, "ISK": true, "JPY": true,
	"KMF": true, "KRW": true, "PYG": true, "RWF": true, "UGX": true,
	"VUV": true, "VND": true, "XAF": true, "XOF": true, "XPF": true,
}

// checkoutComThreeDecimalCurrencies: amount is minor-unit/1000ths, matching
// the plain ISO-4217 three-decimal exponent — Cackle's AmountMinor maps
// straight through. Source: same page as above.
var checkoutComThreeDecimalCurrencies = map[string]bool{
	"BHD": true, "IQD": true, "JOD": true, "KWD": true, "LYD": true,
	"OMR": true, "TND": true,
}

// checkoutComForcedTwoDecimalCurrencies mirrors Stripe's ISK/UGX exception
// but for Checkout.com's OWN documented special case: CLP. ISO-4217 gives
// CLP a zero exponent, but Checkout.com's docs specifically note "the last
// two digits must be 00" for CLP — i.e. CLP is sent as an ordinary ×100
// amount, restricted to whole-peso values (cents are meaningless for CLP,
// hence the forced "00" tail). Source: same calculating-the-amount page.
var checkoutComForcedTwoDecimalCurrencies = map[string]bool{
	"CLP": true,
}

// checkoutComAmount converts o.AmountMinor into Checkout.com's `amount`
// field. See the currency table doc comments above.
func checkoutComAmount(amountMinor int64, currency string) int64 {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if checkoutComForcedTwoDecimalCurrencies[cur] {
		return amountMinor * 100
	}
	// Zero-decimal, three-decimal, and the ordinary two-decimal default
	// are all a direct passthrough.
	return amountMinor
}

// checkoutComAmountToMinor is the inverse, for reconciling a settled
// amount Checkout.com reports back into Cackle's AmountMinor.
func checkoutComAmountToMinor(amt int64, currency string) (int64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if checkoutComForcedTwoDecimalCurrencies[cur] {
		if amt%100 != 0 {
			return 0, fmt.Errorf("%w: %s amount %d is not a whole multiple of 100 as Checkout.com documents", ErrCheckoutComMalformedResponse, cur, amt)
		}
		return amt / 100, nil
	}
	return amt, nil
}

// CheckoutComProvider implements Provider against Checkout.com's Hosted
// Payments Page. As with every adapter in this package, Checkout.com pays
// the organiser's OWN account directly — Cackle never touches the money.
type CheckoutComProvider struct {
	secretKey     string
	webhookSecret string
	httpClient    *http.Client
	baseURL       string // required, no default
}

// NewCheckoutCom constructs a CheckoutComProvider from
// CACKLE_CHECKOUTCOM_SECRET_KEY, CACKLE_CHECKOUTCOM_WEBHOOK_SECRET, and
// CACKLE_CHECKOUTCOM_API_BASE_URL. All three are required.
func NewCheckoutCom() (*CheckoutComProvider, error) {
	secret := strings.TrimSpace(os.Getenv(EnvCheckoutComSecretKey))
	if secret == "" {
		return nil, ErrCheckoutComSecretNotConfigured
	}
	whSecret := strings.TrimSpace(os.Getenv(EnvCheckoutComWebhookKey))
	if whSecret == "" {
		return nil, ErrCheckoutComWebhookKeyMissing
	}
	baseURL := strings.TrimSpace(os.Getenv(EnvCheckoutComAPIBaseURL))
	if baseURL == "" {
		return nil, ErrCheckoutComBaseURLNotConfigured
	}
	return &CheckoutComProvider{
		secretKey:     secret,
		webhookSecret: whSecret,
		httpClient:    &http.Client{Timeout: checkoutComHTTPTimeout},
		baseURL:       strings.TrimSuffix(baseURL, "/"),
	}, nil
}

// Name implements Provider.
func (p *CheckoutComProvider) Name() string { return ProviderNameCheckoutCom }

// Capabilities implements Provider.
func (p *CheckoutComProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil,
		Countries:     nil,
		Flow:          FlowRedirect,
		Refunds:       true,
		Payouts:       true,
		Webhooks:      true,
		ZeroDecimalOK: true,
	}
}

// Begin creates a Checkout.com Hosted Payments Page session. See the file
// doc comment's HONESTY note on the request field names.
func (p *CheckoutComProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: checkoutcom: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: checkoutcom: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: checkoutcom: currency is required")
	}
	if strings.TrimSpace(o.CallbackURL) == "" {
		return Charge{}, errors.New("payments: checkoutcom: callback_url is required")
	}

	reqBody := map[string]any{
		"amount":      checkoutComAmount(o.AmountMinor, currency),
		"currency":    currency,
		"reference":   o.Reference,
		"success_url": o.CallbackURL,
		"failure_url": o.CallbackURL,
		"cancel_url":  o.CallbackURL,
	}
	if o.BuyerEmail != "" {
		reqBody["customer"] = map[string]string{"email": o.BuyerEmail}
	}
	meta := map[string]string{"cackle_reference": o.Reference}
	if o.EventID != "" {
		meta["cackle_event_id"] = o.EventID
	}
	if o.OrgID != "" {
		meta["cackle_org_id"] = o.OrgID
	}
	for k, v := range o.Metadata {
		meta[k] = v
	}
	reqBody["metadata"] = meta

	respBody, status, err := p.do(ctx, http.MethodPost, "/hosted-payments", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyCheckoutComError(status, respBody)
	}

	var parsed struct {
		ID    string `json:"id"`
		Links struct {
			Redirect struct {
				HREF string `json:"href"`
			} `json:"redirect"`
		} `json:"_links"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrCheckoutComMalformedResponse, err)
	}
	if parsed.ID == "" || parsed.Links.Redirect.HREF == "" {
		return Charge{}, fmt.Errorf("%w: empty session id or _links.redirect.href", ErrCheckoutComMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNameCheckoutCom,
		Reference:   o.Reference,
		RedirectURL: parsed.Links.Redirect.HREF,
	}, nil
}

// checkoutComPaymentPayload is the subset of a Checkout.com Payment object
// (from GET /payments/{id} or an event's `data`) this adapter reads.
type checkoutComPaymentPayload struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	Reference string            `json:"reference"`
	Metadata  map[string]string `json:"metadata"`
}

// Verify retrieves a payment directly from Checkout.com by id and reports
// its settlement state. Fails closed on any transport, parse, or ambiguous
// status. https://checkout.com/docs/payments/manage-payments/get-payment-details
//
// reference here is treated as a Checkout.com payment id (pay_...), NOT
// Cackle's own order reference — Checkout.com's API does not expose a
// documented "look up a payment by an arbitrary merchant reference" GET
// endpoint (only a payment-request search API with different semantics),
// so callers should persist Checkout.com's own payment/session id as
// Charge.Reference in preference to o.Reference, mirroring the same
// documented caveat in stripe.go's Verify.
func (p *CheckoutComProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: checkoutcom: reference is required")
	}

	respBody, status, err := p.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyCheckoutComError(status, respBody)
	}

	result, err := parseCheckoutComPayment(respBody)
	if err != nil {
		return Result{}, err
	}
	result.Raw = json.RawMessage(respBody)
	return result, nil
}

// parseCheckoutComPayment turns a Payment JSON body into a Result, failing
// closed on anything malformed or ambiguous. Only "Captured" is ever
// treated as StatusPaid — the confirmed status values from Checkout.com's
// docs are Captured/Authorized/Pending/Declined (not exhaustively
// confirmed — see file doc comment); anything else, known or not, is
// StatusFailed rather than guessed as paid or left ambiguous.
func parseCheckoutComPayment(body []byte) (Result, error) {
	var payload checkoutComPaymentPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrCheckoutComMalformedResponse, err)
	}
	if payload.ID == "" {
		return Result{}, fmt.Errorf("%w: missing payment id", ErrCheckoutComMalformedResponse)
	}
	ref := payload.Reference
	if ref == "" {
		ref = payload.Metadata["cackle_reference"]
	}
	if ref == "" {
		return Result{}, fmt.Errorf("%w: payment has no reference or cackle_reference metadata to reconcile against", ErrCheckoutComMalformedResponse)
	}

	result := Result{
		Provider:  ProviderNameCheckoutCom,
		Reference: ref,
		EventID:   payload.ID,
		Currency:  strings.ToUpper(payload.Currency),
	}

	if payload.Status == "Captured" {
		if payload.Amount <= 0 {
			return Result{}, fmt.Errorf("%w: status=Captured with non-positive amount", ErrCheckoutComMalformedResponse)
		}
		minor, err := checkoutComAmountToMinor(payload.Amount, result.Currency)
		if err != nil {
			return Result{}, err
		}
		result.AmountMinor = minor
		result.Status = StatusPaid
	} else {
		result.Status = StatusFailed
	}
	return result, nil
}

// checkoutComWebhookEnvelope is Checkout.com's webhook event envelope.
// https://checkout.com/docs/developer-resources/webhooks/webhook-event-types/payment_captured
type checkoutComWebhookEnvelope struct {
	ID   string                    `json:"id"`
	Type string                    `json:"type"`
	Data checkoutComPaymentPayload `json:"data"`
}

// Webhook validates Checkout.com's HMAC-SHA256 signature (header
// Cko-Signature, hex-encoded, over the raw request body) and returns the
// settled result for a payment_captured event.
// https://checkout.com/docs/developer-resources/webhooks/manage-webhooks/set-up-your-webhook-receiver
func (p *CheckoutComProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	sigHeader := strings.TrimSpace(r.Header.Get("Cko-Signature"))
	if sigHeader == "" {
		return Result{}, ErrCheckoutComMissingSignature
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrCheckoutComMalformedResponse)
	}
	body, err := checkoutComReadLimited(r.Body, checkoutComMaxBodyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("payments: checkoutcom: read webhook body: %w", err)
	}

	given, err := hex.DecodeString(sigHeader)
	if err != nil {
		return Result{}, fmt.Errorf("%w: signature header is not valid hex", ErrCheckoutComInvalidSignature)
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, given) {
		return Result{}, ErrCheckoutComInvalidSignature
	}

	var envelope checkoutComWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrCheckoutComMalformedResponse, err)
	}
	if envelope.Type != "payment_captured" {
		// payment_approved (pre-capture) and everything else is not treated
		// as a final settlement by this build.
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Type)
	}

	payloadBytes, err := json.Marshal(envelope.Data)
	if err != nil {
		return Result{}, fmt.Errorf("%w: re-encode event data: %v", ErrCheckoutComMalformedResponse, err)
	}
	result, err := parseCheckoutComPayment(payloadBytes)
	if err != nil {
		return Result{}, err
	}
	if result.Status != StatusPaid {
		return Result{}, fmt.Errorf("%w: payment_captured event carried a non-Captured status", ErrCheckoutComMalformedResponse)
	}
	if envelope.ID != "" {
		result.EventID = envelope.ID // prefer the event's own unique id for replay dedup
	}
	result.Raw = json.RawMessage(body)
	return result, nil
}

// do issues an authenticated JSON request against the Checkout.com API,
// bounding it with checkoutComHTTPTimeout regardless of the caller's own
// context, and caps the response body it reads.
func (p *CheckoutComProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, checkoutComHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: checkoutcom: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: checkoutcom: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: checkoutcom: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := checkoutComReadLimited(resp.Body, checkoutComMaxBodyBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("payments: checkoutcom: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// checkoutComReadLimited reads at most limit bytes from r, returning
// ErrCheckoutComResponseTooLarge if there was more.
func checkoutComReadLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrCheckoutComResponseTooLarge
	}
	return b, nil
}

// checkoutComErrorEnvelope is Checkout.com's documented error response
// shape: {"request_id":"...", "error_type":"...", "error_codes":[...]}.
type checkoutComErrorEnvelope struct {
	ErrorType  string   `json:"error_type"`
	ErrorCodes []string `json:"error_codes"`
}

// classifyCheckoutComError builds an error for a non-2xx Checkout.com
// response, best-effort including Checkout.com's own error codes without
// ever including request headers or the secret key.
func classifyCheckoutComError(status int, body []byte) error {
	var env checkoutComErrorEnvelope
	_ = json.Unmarshal(body, &env)
	return fmt.Errorf("%w: http %d: %s %v", ErrCheckoutComUnexpectedStatus, status, env.ErrorType, env.ErrorCodes)
}
