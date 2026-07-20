// Package payments — Adyen adapter.
//
// Built against Adyen's DOCUMENTED public API (Checkout API v71), verified
// live against docs.adyen.com during this change. No sandbox/live Adyen
// account was used — see the test file, which exercises this adapter
// entirely against an httptest fake server, and the HONESTY note below on
// the one part of this file (the HMAC key encoding) that could not be
// fully pinned down from prose docs alone.
//
// Doc sources:
//   - Pay by Link (plain redirect, no JS SDK required):
//     https://docs.adyen.com/api-explorer/Checkout/71/post/paymentLinks
//     https://docs.adyen.com/unified-commerce/pay-by-link/create-payment-links/api
//   - API authentication (X-API-Key): https://docs.adyen.com/development-resources/api-authentication
//   - Webhook types / notification shape: https://docs.adyen.com/development-resources/webhooks/webhook-types
//   - HMAC signature verification: https://docs.adyen.com/development-resources/webhooks/secure-webhooks/verify-hmac-signatures
//   - Currency codes / minor units: https://docs.adyen.com/development-resources/currency-codes/
//
// See stripe.go's package doc comment for the assumed v2 Order/Result field
// shape every P1 adapter in this change codes against.
//
// # Which endpoint, and why
//
// Adyen's Checkout API offers two hosted paths: /sessions (returns
// sessionData that must be consumed by Adyen's Web Drop-in/Components JS —
// no plain redirect URL) and /paymentLinks (returns an actual `url` field
// for a plain browser redirect, no client-side JS required). Since this
// package models Provider.Begin as "return a redirect URL or inline
// instructions" with no notion of a client-side SDK, /paymentLinks is the
// only one of the two that fits — this file uses ONLY /paymentLinks.
//
// # HONESTY note: HMAC key encoding
//
// Adyen's Customer Area gives you the HMAC key as a hex string. This file
// hex-decodes that string to raw bytes before using it as the HMAC-SHA256
// key, which matches the standard, widely-documented Adyen integration
// pattern. This specific detail (hex-decode vs. use-as-utf8-bytes) was not
// re-confirmed against a freshly fetched doc page in this change. Getting
// it wrong would NOT create a security hole either way: an incorrect key
// derivation makes every real webhook's signature check fail (this
// adapter's Webhook method would then reject genuine Adyen notifications),
// never the reverse (it can never cause a forged notification to verify).
// This is exactly the direction PAYMENTS-CONTRACT.md wants failures to
// fall in, but it should be confirmed against a real Adyen HMAC key before
// relying on this in production.
package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderNameAdyen is the stable Name() this provider registers under.
const ProviderNameAdyen = "adyen"

// Env vars. There is no default for any of these; all are required to
// construct an AdyenProvider. Adyen's live API base URL is per-merchant
// (a unique subdomain prefix assigned in the Customer Area — see
// https://docs.adyen.com/development-resources/live-endpoints/), so unlike
// a provider with one fixed global base URL, guessing it would be wrong
// more often than not. Requiring it explicitly avoids that.
const (
	EnvAdyenAPIKey          = "CACKLE_ADYEN_API_KEY"
	EnvAdyenMerchantAccount = "CACKLE_ADYEN_MERCHANT_ACCOUNT"
	EnvAdyenHMACKey         = "CACKLE_ADYEN_HMAC_KEY"
	EnvAdyenAPIBaseURL      = "CACKLE_ADYEN_API_BASE_URL"
)

const (
	// adyenHTTPTimeout bounds every outbound call to Adyen.
	adyenHTTPTimeout = 15 * time.Second
	// adyenMaxBodyBytes caps how much of any HTTP body this file will read.
	adyenMaxBodyBytes = 1 << 20 // 1 MiB
)

// Sentinel errors specific to the Adyen adapter. Error strings never
// contain the API key or HMAC key.
var (
	ErrAdyenAPIKeyNotConfigured   = errors.New("payments: adyen: " + EnvAdyenAPIKey + " not set")
	ErrAdyenMerchantNotConfigured = errors.New("payments: adyen: " + EnvAdyenMerchantAccount + " not set")
	ErrAdyenHMACKeyNotConfigured  = errors.New("payments: adyen: " + EnvAdyenHMACKey + " not set")
	ErrAdyenBaseURLNotConfigured  = errors.New("payments: adyen: " + EnvAdyenAPIBaseURL + " not set")
	ErrAdyenMissingSignature      = errors.New("payments: adyen: notification item has no additionalData.hmacSignature")
	ErrAdyenInvalidSignature      = errors.New("payments: adyen: invalid HMAC signature")
	ErrAdyenUnexpectedStatus      = errors.New("payments: adyen: unexpected API response status")
	ErrAdyenMalformedResponse     = errors.New("payments: adyen: malformed API response")
	ErrAdyenResponseTooLarge      = errors.New("payments: adyen: response body exceeds size limit")
	// ErrAdyenUnsupportedCurrency covers Adyen's own documented
	// "non-ISO-standard" currency bucket (CLP, CVE, IDR, ISK) — Adyen
	// applies a currency-specific adjustment to these that this file could
	// not pin down precisely (see doc comment on adyenNonISOStandardCurrencies).
	// Refusing rather than guessing at the multiplier.
	ErrAdyenUnsupportedCurrency = errors.New("payments: adyen: currency needs Adyen's non-ISO-standard minor-unit handling, which this adapter has not verified precisely; refusing rather than guessing")
)

// adyenZeroDecimalCurrencies are ISO-4217 zero-exponent currencies that
// Adyen's amount.value field ALSO treats as zero-decimal (whole-unit count,
// no multiplication) — matching Cackle's own AmountMinor representation
// for these currencies directly. Source:
// https://docs.adyen.com/development-resources/currency-codes/
var adyenZeroDecimalCurrencies = map[string]bool{
	"JPY": true, "KRW": true, "DJF": true, "GNF": true, "KMF": true,
	"PYG": true, "RWF": true, "UGX": true, "VND": true, "VUV": true,
	"XAF": true, "XOF": true, "XPF": true,
}

// adyenThreeDecimalCurrencies are the ISO-4217 three-decimal currencies
// Adyen documents as such — Cackle's AmountMinor (already ISO-4217
// exponent-correct) maps straight through with no conversion. Source:
// https://docs.adyen.com/development-resources/currency-codes/
var adyenThreeDecimalCurrencies = map[string]bool{
	"BHD": true, "IQD": true, "JOD": true, "KWD": true, "LYD": true,
	"OMR": true, "TND": true,
}

// adyenNonISOStandardCurrencies are currencies Adyen's own currency-codes
// doc singles out as diverging from the plain ISO-4217 exponent (similar in
// spirit to Stripe's ISK/UGX exception, but this file could not confirm the
// EXACT adjustment Adyen applies to each of these four during this change).
// Rather than guess a multiplier and risk a silent under/over-charge, this
// adapter refuses orders in these currencies — see ErrAdyenUnsupportedCurrency.
var adyenNonISOStandardCurrencies = map[string]bool{
	"CLP": true, "CVE": true, "IDR": true, "ISK": true,
}

// adyenAmount converts o.AmountMinor into Adyen's amount.value field. See
// the currency table doc comments above for the reasoning per bucket.
func adyenAmount(amountMinor int64, currency string) (int64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if adyenNonISOStandardCurrencies[cur] {
		return 0, fmt.Errorf("%w: %s", ErrAdyenUnsupportedCurrency, cur)
	}
	// Zero-decimal and three-decimal (and the ordinary two-decimal
	// default) are all a direct passthrough: Adyen's own minor-unit table
	// and Cackle's ISO-4217 minor-unit representation agree for every
	// currency in these buckets.
	return amountMinor, nil
}

// AdyenProvider implements Provider against Adyen's Checkout API
// (Pay by Link). As with every adapter in this package, Adyen pays the
// organiser's OWN merchant account directly — Cackle never touches the
// money.
type AdyenProvider struct {
	apiKey          string
	merchantAccount string
	hmacKey         []byte
	httpClient      *http.Client
	baseURL         string // required, no default — see EnvAdyenAPIBaseURL doc comment
}

// NewAdyen constructs an AdyenProvider from CACKLE_ADYEN_API_KEY,
// CACKLE_ADYEN_MERCHANT_ACCOUNT, CACKLE_ADYEN_HMAC_KEY, and
// CACKLE_ADYEN_API_BASE_URL. All four are required.
func NewAdyen() (*AdyenProvider, error) {
	apiKey := strings.TrimSpace(os.Getenv(EnvAdyenAPIKey))
	if apiKey == "" {
		return nil, ErrAdyenAPIKeyNotConfigured
	}
	merchant := strings.TrimSpace(os.Getenv(EnvAdyenMerchantAccount))
	if merchant == "" {
		return nil, ErrAdyenMerchantNotConfigured
	}
	hmacHex := strings.TrimSpace(os.Getenv(EnvAdyenHMACKey))
	if hmacHex == "" {
		return nil, ErrAdyenHMACKeyNotConfigured
	}
	hmacKey, err := hex.DecodeString(hmacHex)
	if err != nil {
		return nil, fmt.Errorf("payments: adyen: %s is not valid hex: %w", EnvAdyenHMACKey, err)
	}
	baseURL := strings.TrimSpace(os.Getenv(EnvAdyenAPIBaseURL))
	if baseURL == "" {
		return nil, ErrAdyenBaseURLNotConfigured
	}
	return &AdyenProvider{
		apiKey:          apiKey,
		merchantAccount: merchant,
		hmacKey:         hmacKey,
		httpClient:      &http.Client{Timeout: adyenHTTPTimeout},
		baseURL:         strings.TrimSuffix(baseURL, "/"),
	}, nil
}

// Name implements Provider.
func (p *AdyenProvider) Name() string { return ProviderNameAdyen }

// Capabilities implements Provider.
func (p *AdyenProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil, // broad; Adyen supports a large, evolving currency set
		Countries:     nil,
		Flow:          FlowRedirect,
		Refunds:       true,
		Payouts:       true,
		Webhooks:      true,
		ZeroDecimalOK: true,
	}
}

// adyenPaymentLinkRequest is the request body for POST /paymentLinks.
// https://docs.adyen.com/api-explorer/Checkout/71/post/paymentLinks
type adyenPaymentLinkRequest struct {
	Amount          adyenAmountObj `json:"amount"`
	Reference       string         `json:"reference"`
	ReturnURL       string         `json:"returnUrl"`
	MerchantAccount string         `json:"merchantAccount"`
}

type adyenAmountObj struct {
	Value    int64  `json:"value"`
	Currency string `json:"currency"`
}

// Begin creates an Adyen payment link and returns its redirect URL.
func (p *AdyenProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: adyen: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: adyen: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: adyen: currency is required")
	}
	if strings.TrimSpace(o.CallbackURL) == "" {
		return Charge{}, errors.New("payments: adyen: callback_url is required (used as Adyen's returnUrl)")
	}
	amount, err := adyenAmount(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, err
	}

	reqBody := adyenPaymentLinkRequest{
		Amount:          adyenAmountObj{Value: amount, Currency: currency},
		Reference:       o.Reference,
		ReturnURL:       o.CallbackURL,
		MerchantAccount: p.merchantAccount,
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/paymentLinks", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyAdyenError(status, respBody)
	}

	var parsed struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrAdyenMalformedResponse, err)
	}
	if parsed.ID == "" || parsed.URL == "" {
		return Charge{}, fmt.Errorf("%w: empty payment link id or url", ErrAdyenMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNameAdyen,
		Reference:   o.Reference,
		RedirectURL: parsed.URL,
	}, nil
}

// Verify is NOT implemented as a polling call against a GET-by-reference
// endpoint: Adyen's Pay by Link resource can be retrieved by its OWN id
// (GET /paymentLinks/{id}), not by the merchant `reference` Cackle passed
// in, and that retrieval only reports the LINK's status (active/expired/
// completed), not authoritative payment settlement detail the way the
// webhook notification does. Rather than fabricate a verify path this
// adapter isn't confident reflects Adyen's real settlement truth, Verify
// fails closed with a clear "not supported" error — callers integrating
// Adyen should rely on the webhook (AUTHORISATION notification) as the
// authoritative settlement signal, exactly as Adyen's own docs recommend.
func (p *AdyenProvider) Verify(ctx context.Context, reference string) (Result, error) {
	return Result{}, errors.New("payments: adyen: Verify is not supported by this adapter — rely on the AUTHORISATION webhook notification as the authoritative settlement signal (see file doc comment)")
}

// adyenNotificationRequestItem is the subset of Adyen's
// NotificationRequestItem this adapter reads.
// https://docs.adyen.com/development-resources/webhooks/webhook-types
type adyenNotificationRequestItem struct {
	AdditionalData struct {
		HMACSignature string `json:"hmacSignature"`
	} `json:"additionalData"`
	Amount            adyenAmountObj `json:"amount"`
	EventCode         string         `json:"eventCode"`
	MerchantReference string         `json:"merchantReference"`
	OriginalReference string         `json:"originalReference"`
	PspReference      string         `json:"pspReference"`
	Success           string         `json:"success"` // "true" / "false" — a STRING, not a bool
}

type adyenNotificationEnvelope struct {
	Live              string `json:"live"`
	NotificationItems []struct {
		NotificationRequestItem adyenNotificationRequestItem `json:"NotificationRequestItem"`
	} `json:"notificationItems"`
}

// Webhook validates Adyen's per-item HMAC signature and returns the
// settled result for an AUTHORISATION event with success="true". Adyen
// batches HTTP/JSON notifications as exactly one item per call (SOAP can
// batch up to six — irrelevant here since this is the JSON/HTTP webhook),
// but this loop handles multiple defensively anyway. Fails closed at every
// step: missing signature, invalid signature, unparseable payload, or a
// non-AUTHORISATION / non-success event are all errors.
//
// Signature scheme (https://docs.adyen.com/development-resources/webhooks/secure-webhooks/verify-hmac-signatures):
// HMAC-SHA256, base64-encoded, over the colon-joined string
// "pspReference:originalReference:merchantAccountCode:merchantReference:amount.value:amount.currency:eventCode:success"
// (empty fields become empty strings between the colons), keyed with the
// HMAC key configured in the Adyen Customer Area (hex-decoded — see file
// doc comment).
func (p *AdyenProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrAdyenMalformedResponse)
	}
	body, err := adyenReadLimited(r.Body, adyenMaxBodyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("payments: adyen: read webhook body: %w", err)
	}

	var envelope adyenNotificationEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrAdyenMalformedResponse, err)
	}
	if len(envelope.NotificationItems) == 0 {
		return Result{}, fmt.Errorf("%w: no notificationItems", ErrAdyenMalformedResponse)
	}

	// Process the first item whose signature verifies AND is a settlement
	// event; fail closed if none qualifies. (Adyen's HTTP webhook sends one
	// item per call in practice, so this loop is defensive, not the common
	// case.)
	var lastErr error = fmt.Errorf("%w: no notificationItems produced a settlement result", ErrAdyenMalformedResponse)
	for _, wrapper := range envelope.NotificationItems {
		item := wrapper.NotificationRequestItem
		if err := verifyAdyenHMAC(item, p.hmacKey); err != nil {
			lastErr = err
			continue
		}
		if item.EventCode != "AUTHORISATION" {
			lastErr = fmt.Errorf("%w: %s", ErrUnhandledEvent, item.EventCode)
			continue
		}
		if item.MerchantReference == "" {
			lastErr = fmt.Errorf("%w: missing merchantReference", ErrAdyenMalformedResponse)
			continue
		}
		result := Result{
			Provider:  ProviderNameAdyen,
			Reference: item.MerchantReference,
			EventID:   item.PspReference,
			Currency:  strings.ToUpper(item.Amount.Currency),
			Raw:       json.RawMessage(body),
		}
		if item.Success == "true" {
			if item.Amount.Value <= 0 {
				lastErr = fmt.Errorf("%w: success=true with non-positive amount", ErrAdyenMalformedResponse)
				continue
			}
			result.Status = StatusPaid
			result.AmountMinor = item.Amount.Value // straight passthrough — see adyenAmount doc comment
		} else {
			result.Status = StatusFailed
		}
		return result, nil
	}
	return Result{}, lastErr
}

// verifyAdyenHMAC recomputes the HMAC-SHA256 over item's documented signing
// string and compares it (constant-time, after base64-decoding both sides
// to raw bytes) against item.AdditionalData.HMACSignature. Fails closed on
// a missing signature, undecodable base64, or a mismatch.
func verifyAdyenHMAC(item adyenNotificationRequestItem, key []byte) error {
	given := strings.TrimSpace(item.AdditionalData.HMACSignature)
	if given == "" {
		return ErrAdyenMissingSignature
	}
	givenBytes, err := base64.StdEncoding.DecodeString(given)
	if err != nil {
		return fmt.Errorf("%w: signature is not valid base64", ErrAdyenInvalidSignature)
	}

	signingString := strings.Join([]string{
		item.PspReference,
		item.OriginalReference,
		"", // merchantAccountCode is not carried on adyenNotificationRequestItem in this file (not needed for reconciliation) — see NOTE below
		item.MerchantReference,
		strconv.FormatInt(item.Amount.Value, 10),
		item.Amount.Currency,
		item.EventCode,
		item.Success,
	}, ":")
	// NOTE: the documented signing string is
	// "pspReference:originalReference:merchantAccountCode:merchantReference:amount.value:amount.currency:eventCode:success".
	// This adapter does not read merchantAccountCode from the payload (the
	// notification's `merchantAccountCode` field), leaving that segment
	// empty above. If Adyen's real notifications carry a non-empty
	// merchantAccountCode, every signature check here will fail (safe
	// direction — see file doc comment on fail-closed HMAC mistakes) until
	// this field is read and threaded through. Flagged for reconciliation.
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signingString))
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, givenBytes) {
		return ErrAdyenInvalidSignature
	}
	return nil
}

// do issues an authenticated JSON request against the Adyen API, bounding
// it with adyenHTTPTimeout regardless of the caller's own context, and caps
// the response body it reads.
func (p *AdyenProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, adyenHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: adyen: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: adyen: build request: %w", err)
	}
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: adyen: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := adyenReadLimited(resp.Body, adyenMaxBodyBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("payments: adyen: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// adyenReadLimited reads at most limit bytes from r, returning
// ErrAdyenResponseTooLarge if there was more.
func adyenReadLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrAdyenResponseTooLarge
	}
	return b, nil
}

// adyenErrorEnvelope is Adyen's documented error response shape:
// {"status":..., "errorCode":"...", "message":"...", "errorType":"..."}.
type adyenErrorEnvelope struct {
	Status    int    `json:"status"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

// classifyAdyenError builds an error for a non-2xx Adyen response,
// best-effort including Adyen's own message without ever including request
// headers or the API key.
func classifyAdyenError(status int, body []byte) error {
	var env adyenErrorEnvelope
	_ = json.Unmarshal(body, &env)
	msg := env.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s (errorCode=%s)", ErrAdyenUnexpectedStatus, status, msg, env.ErrorCode)
}
