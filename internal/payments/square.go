// Package payments — Square adapter.
//
// Built against Square's DOCUMENTED public API, verified live against
// developer.squareup.com during this change. No sandbox/live account was
// used — see the test file, which exercises this adapter entirely against
// an httptest fake server, and the HONESTY notes below on the parts of
// this file that could not be fully confirmed.
//
// Doc sources:
//   - Payment Links (Checkout API): https://developer.squareup.com/reference/square/checkout-api/create-payment-link
//   - Money object:                 https://developer.squareup.com/reference/square/objects/Money
//   - Payment object / statuses:    https://developer.squareup.com/reference/square/objects/Payment
//   - Webhook signature:            https://developer.squareup.com/docs/webhooks/step3validate
//   - Webhook events:                https://developer.squareup.com/docs/webhooks/v2webhook-events-tech-ref
//   - Monetary amounts / zero-decimal: https://developer.squareup.com/docs/build-basics/common-data-types/working-with-monetary-amounts
//
// See stripe.go's package doc comment for the assumed v2 Order/Result field
// shape every P1 adapter in this change codes against.
//
// # HONESTY notes
//
//  1. Square's webhook signature IS confirmed to be an HMAC-SHA256 over
//     "the signature key, the notification URL, and the raw body", per
//     Square's own prose docs — but the EXACT concatenation order/encoding
//     (notification_url + raw_body, base64-encoded HMAC output) was only
//     corroborated via widely-known SDK behaviour, not quoted verbatim
//     from Square's prose. This file implements that widely-known
//     construction; if wrong, every signature check fails closed (genuine
//     webhooks get rejected) rather than accepting a forged one.
//  2. This file could not confirm Square's exact minor-unit handling for
//     ISO-4217 three-decimal currencies (KWD etc) or any Square-specific
//     zero-decimal exception list beyond the single confirmed JPY example.
//     Three-decimal currencies are refused (see ErrSquareUnsupportedCurrency)
//     rather than guessed at, matching this package's pattern elsewhere.
//  3. Square's Payment Links API creates an underlying Order but the
//     Payment id is only known once the buyer actually pays — there is no
//     confirmed "look up by our own reference" endpoint independent of
//     that. See Verify's doc comment for the same reference-identity
//     caveat documented in stripe.go/checkoutcom.go.
package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

// ProviderNameSquare is the stable Name() this provider registers under.
const ProviderNameSquare = "square"

// Env vars. No defaults. CACKLE_SQUARE_API_BASE_URL must be either
// https://connect.squareup.com (production) or
// https://connect.squareupsandbox.com (sandbox) — required explicitly so a
// misconfiguration can never silently point at the wrong environment.
// CACKLE_SQUARE_NOTIFICATION_URL must be the EXACT URL configured for this
// webhook subscription in Square's dashboard: Square's signature covers
// that URL, so a mismatch here would make every signature check fail
// closed (safe, but useless) rather than silently skip verification.
const (
	EnvSquareAccessToken         = "CACKLE_SQUARE_ACCESS_TOKEN"
	EnvSquareWebhookSignatureKey = "CACKLE_SQUARE_WEBHOOK_SIGNATURE_KEY"
	EnvSquareLocationID          = "CACKLE_SQUARE_LOCATION_ID"
	EnvSquareNotificationURL     = "CACKLE_SQUARE_NOTIFICATION_URL"
	EnvSquareAPIBaseURL          = "CACKLE_SQUARE_API_BASE_URL"
)

const (
	squareHTTPTimeout  = 15 * time.Second
	squareMaxBodyBytes = 1 << 20 // 1 MiB
)

// Sentinel errors specific to the Square adapter. Error strings never
// contain the access token or webhook signature key.
var (
	ErrSquareAccessTokenNotConfigured = errors.New("payments: square: " + EnvSquareAccessToken + " not set")
	ErrSquareWebhookKeyNotConfigured  = errors.New("payments: square: " + EnvSquareWebhookSignatureKey + " not set")
	ErrSquareLocationNotConfigured    = errors.New("payments: square: " + EnvSquareLocationID + " not set")
	ErrSquareNotificationURLMissing   = errors.New("payments: square: " + EnvSquareNotificationURL + " not set")
	ErrSquareBaseURLNotConfigured     = errors.New("payments: square: " + EnvSquareAPIBaseURL + " not set")
	ErrSquareMissingSignature         = errors.New("payments: square: missing x-square-hmacsha256-signature header")
	ErrSquareInvalidSignature         = errors.New("payments: square: invalid webhook signature")
	ErrSquareUnexpectedStatus         = errors.New("payments: square: unexpected API response status")
	ErrSquareMalformedResponse        = errors.New("payments: square: malformed API response")
	ErrSquareResponseTooLarge         = errors.New("payments: square: response body exceeds size limit")
	// ErrSquareUnsupportedCurrency covers ISO-4217 three-decimal
	// currencies — see file doc comment HONESTY note 2.
	ErrSquareUnsupportedCurrency = errors.New("payments: square: three-decimal ISO-4217 currency is not verified against Square's documented Money semantics; refusing rather than guessing")
)

var squareThreeDecimalCurrencies = map[string]bool{
	"KWD": true, "BHD": true, "JOD": true, "OMR": true, "TND": true,
}

// squareAmount validates and passes through o.AmountMinor for Square's
// Money.amount field. Square's Money object documents amount as "the
// smallest denomination of the currency" (matching ISO-4217 minor units)
// with JPY confirmed as a zero-decimal example — the same convention
// Cackle's own AmountMinor already uses, so ordinary and zero-decimal
// currencies are a direct passthrough. Three-decimal currencies are
// refused (HONESTY note 2).
func squareAmount(amountMinor int64, currency string) (int64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if squareThreeDecimalCurrencies[cur] {
		return 0, fmt.Errorf("%w: %s", ErrSquareUnsupportedCurrency, cur)
	}
	return amountMinor, nil
}

// SquareProvider implements Provider against Square's Payment Links API.
// As with every adapter in this package, Square pays the organiser's OWN
// Square account directly — Cackle never touches the money.
type SquareProvider struct {
	accessToken         string
	webhookSignatureKey string
	locationID          string
	notificationURL     string
	httpClient          *http.Client
	baseURL             string
}

// NewSquare constructs a SquareProvider from CACKLE_SQUARE_ACCESS_TOKEN,
// CACKLE_SQUARE_WEBHOOK_SIGNATURE_KEY, CACKLE_SQUARE_LOCATION_ID,
// CACKLE_SQUARE_NOTIFICATION_URL, and CACKLE_SQUARE_API_BASE_URL. All five
// are required.
func NewSquare() (*SquareProvider, error) {
	token := strings.TrimSpace(os.Getenv(EnvSquareAccessToken))
	if token == "" {
		return nil, ErrSquareAccessTokenNotConfigured
	}
	key := strings.TrimSpace(os.Getenv(EnvSquareWebhookSignatureKey))
	if key == "" {
		return nil, ErrSquareWebhookKeyNotConfigured
	}
	loc := strings.TrimSpace(os.Getenv(EnvSquareLocationID))
	if loc == "" {
		return nil, ErrSquareLocationNotConfigured
	}
	notifyURL := strings.TrimSpace(os.Getenv(EnvSquareNotificationURL))
	if notifyURL == "" {
		return nil, ErrSquareNotificationURLMissing
	}
	baseURL := strings.TrimSpace(os.Getenv(EnvSquareAPIBaseURL))
	if baseURL == "" {
		return nil, ErrSquareBaseURLNotConfigured
	}
	return &SquareProvider{
		accessToken:         token,
		webhookSignatureKey: key,
		locationID:          loc,
		notificationURL:     notifyURL,
		httpClient:          &http.Client{Timeout: squareHTTPTimeout},
		baseURL:             strings.TrimSuffix(baseURL, "/"),
	}, nil
}

// Name implements Provider.
func (p *SquareProvider) Name() string { return ProviderNameSquare }

// Capabilities implements Provider.
func (p *SquareProvider) Capabilities() Capabilities {
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

// Begin creates a Square payment link and returns its redirect URL.
// https://developer.squareup.com/reference/square/checkout-api/create-payment-link
func (p *SquareProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: square: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: square: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: square: currency is required")
	}
	if strings.TrimSpace(o.CallbackURL) == "" {
		return Charge{}, errors.New("payments: square: callback_url is required")
	}
	amount, err := squareAmount(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, err
	}

	itemName := "Cackle order " + o.Reference
	reqBody := map[string]any{
		"idempotency_key": o.Reference,
		"order": map[string]any{
			"location_id":  p.locationID,
			"reference_id": o.Reference,
			"line_items": []map[string]any{
				{
					"name":     itemName,
					"quantity": "1",
					"base_price_money": map[string]any{
						"amount":   amount,
						"currency": currency,
					},
				},
			},
		},
		"checkout_options": map[string]any{
			"redirect_url": o.CallbackURL,
		},
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/v2/online-checkout/payment-links", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifySquareError(status, respBody)
	}

	var parsed struct {
		PaymentLink struct {
			ID      string `json:"id"`
			URL     string `json:"url"`
			OrderID string `json:"order_id"`
		} `json:"payment_link"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrSquareMalformedResponse, err)
	}
	if parsed.PaymentLink.ID == "" || parsed.PaymentLink.URL == "" {
		return Charge{}, fmt.Errorf("%w: empty payment link id or url", ErrSquareMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNameSquare,
		Reference:   o.Reference,
		RedirectURL: parsed.PaymentLink.URL,
	}, nil
}

// squarePaymentPayload is the subset of a Square Payment object this
// adapter reads. https://developer.squareup.com/reference/square/objects/Payment
type squarePaymentPayload struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ReferenceID string `json:"reference_id"`
	OrderID     string `json:"order_id"`
	AmountMoney struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	} `json:"amount_money"`
}

// Verify retrieves a Payment directly from Square by id. Fails closed on
// any transport, parse, or ambiguous status: only status COMPLETED is
// ever StatusPaid (APPROVED, PENDING, CANCELED, FAILED, or anything
// unrecognised are StatusFailed).
// https://developer.squareup.com/reference/square/payments-api/get-payment
//
// reference is a Square PAYMENT id, not Cackle's own order reference or
// the payment link id Begin returns — Square's Payment Links API does not
// expose a confirmed "look up by our own reference_id" endpoint
// independent of the underlying Order, so callers should capture the
// payment id from the webhook and persist THAT as Charge.Reference,
// mirroring the same caveat documented in stripe.go/checkoutcom.go's
// Verify methods.
func (p *SquareProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: square: reference is required")
	}

	respBody, status, err := p.do(ctx, http.MethodGet, "/v2/payments/"+url.PathEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifySquareError(status, respBody)
	}

	var parsed struct {
		Payment squarePaymentPayload `json:"payment"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrSquareMalformedResponse, err)
	}
	result, err := parseSquarePayment(parsed.Payment)
	if err != nil {
		return Result{}, err
	}
	result.Raw = json.RawMessage(respBody)
	return result, nil
}

func parseSquarePayment(payment squarePaymentPayload) (Result, error) {
	if payment.ID == "" {
		return Result{}, fmt.Errorf("%w: missing payment id", ErrSquareMalformedResponse)
	}
	ref := payment.ReferenceID
	if ref == "" {
		return Result{}, fmt.Errorf("%w: payment has no reference_id to reconcile against", ErrSquareMalformedResponse)
	}

	result := Result{
		Provider:  ProviderNameSquare,
		Reference: ref,
		EventID:   payment.ID,
		Currency:  strings.ToUpper(payment.AmountMoney.Currency),
	}
	if payment.Status == "COMPLETED" {
		if payment.AmountMoney.Amount <= 0 {
			return Result{}, fmt.Errorf("%w: status=COMPLETED with non-positive amount", ErrSquareMalformedResponse)
		}
		result.Status = StatusPaid
		result.AmountMinor = payment.AmountMoney.Amount
	} else {
		result.Status = StatusFailed
	}
	return result, nil
}

// squareWebhookEnvelope is Square's webhook event envelope.
// https://developer.squareup.com/docs/webhooks/v2webhook-events-tech-ref
type squareWebhookEnvelope struct {
	EventID string `json:"event_id"`
	Type    string `json:"type"`
	Data    struct {
		Object struct {
			Payment squarePaymentPayload `json:"payment"`
		} `json:"object"`
	} `json:"data"`
}

// Webhook validates Square's HMAC-SHA256 signature and returns the settled
// result for a payment.updated event whose nested payment is COMPLETED.
// https://developer.squareup.com/docs/webhooks/step3validate
//
// Signature: base64(HMAC-SHA256(key=webhookSignatureKey,
// message=notificationURL+rawBody)) — see file doc comment HONESTY note 1
// on this construction's confidence level.
func (p *SquareProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	sigHeader := strings.TrimSpace(r.Header.Get("x-square-hmacsha256-signature"))
	if sigHeader == "" {
		return Result{}, ErrSquareMissingSignature
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrSquareMalformedResponse)
	}
	body, err := squareReadLimited(r.Body, squareMaxBodyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("payments: square: read webhook body: %w", err)
	}

	given, err := base64.StdEncoding.DecodeString(sigHeader)
	if err != nil {
		return Result{}, fmt.Errorf("%w: signature header is not valid base64", ErrSquareInvalidSignature)
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSignatureKey))
	mac.Write([]byte(p.notificationURL))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, given) {
		return Result{}, ErrSquareInvalidSignature
	}

	var envelope squareWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrSquareMalformedResponse, err)
	}
	if envelope.Type != "payment.updated" {
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Type)
	}

	result, err := parseSquarePayment(envelope.Data.Object.Payment)
	if err != nil {
		return Result{}, err
	}
	if result.Status != StatusPaid {
		// A payment.updated event whose nested payment isn't (yet)
		// COMPLETED is not treated as a settlement by this build — it will
		// arrive again once/if the payment does complete.
		return Result{}, fmt.Errorf("%w: payment.updated with status!=COMPLETED", ErrUnhandledEvent)
	}
	if envelope.EventID != "" {
		result.EventID = envelope.EventID // prefer Square's own event_id for replay dedup
	}
	result.Raw = json.RawMessage(body)
	return result, nil
}

// do issues an authenticated JSON request against the Square API, bounding
// it with squareHTTPTimeout regardless of the caller's own context, and
// caps the response body it reads.
func (p *SquareProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, squareHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: square: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: square: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: square: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := squareReadLimited(resp.Body, squareMaxBodyBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("payments: square: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// squareReadLimited reads at most limit bytes from r, returning
// ErrSquareResponseTooLarge if there was more.
func squareReadLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrSquareResponseTooLarge
	}
	return b, nil
}

// squareErrorEnvelope is Square's documented error response shape:
// {"errors":[{"category":"...","code":"...","detail":"..."}]}.
type squareErrorEnvelope struct {
	Errors []struct {
		Category string `json:"category"`
		Code     string `json:"code"`
		Detail   string `json:"detail"`
	} `json:"errors"`
}

// classifySquareError builds an error for a non-2xx Square response,
// best-effort including Square's own error details without ever including
// request headers or the access token.
func classifySquareError(status int, body []byte) error {
	var env squareErrorEnvelope
	_ = json.Unmarshal(body, &env)
	msg := "no message"
	if len(env.Errors) > 0 {
		msg = fmt.Sprintf("%s: %s", env.Errors[0].Code, env.Errors[0].Detail)
	}
	return fmt.Errorf("%w: http %d: %s", ErrSquareUnexpectedStatus, status, msg)
}
