// Package payments — PayPal adapter (Orders v2 API).
//
// Built against PayPal's DOCUMENTED public API, verified live against
// developer.paypal.com during this change. No sandbox/live PayPal account
// was used — see the test file, which exercises this adapter entirely
// against an httptest fake server, and the HONESTY notes below on the
// parts of this file that could not be fully confirmed against primary
// docs.
//
// Doc sources:
//   - Orders v2 API:              https://developer.paypal.com/docs/api/orders/v2/
//   - OAuth2 client credentials:  https://developer.paypal.com/api/rest/authentication/
//   - Currency codes (zero-decimal): https://developer.paypal.com/api/rest/reference/currency-codes/
//   - Verify webhook signature:   https://developer.paypal.com/api/rest/webhooks/rest/
//
// See stripe.go's package doc comment for the assumed v2 Order/Result field
// shape every P1 adapter in this change codes against.
//
// # HONESTY notes
//
//  1. The full Get-Order status enum (CREATED, PAYER_ACTION_REQUIRED,
//     APPROVED, COMPLETED — and possibly SAVED/VOIDED) was not confirmed
//     exhaustively against a fetched schema page. This file only special-
//     cases the values confirmed with reasonable confidence and treats
//     everything else — recognised or not — as "not yet paid" (never as
//     paid), so an incomplete enum can only make this adapter too
//     conservative, never wrongly permissive.
//  2. The exact top-level webhook event JSON shape
//     ({id, event_type, resource: {...}}) is PayPal's long-standing,
//     widely documented convention, but this specific change corroborated
//     it via secondary summaries rather than a freshly fetched official
//     sample payload. Recommend confirming against PayPal's Webhooks
//     Simulator (https://developer.paypal.com/api/rest/webhooks/simulator/)
//     before production use.
//  3. Three-decimal ISO-4217 currencies (KWD, BHD, JOD, OMR, TND) are not
//     mentioned anywhere in PayPal's fetched currency-codes reference —
//     this adapter refuses them rather than guessing at a decimal-string
//     format PayPal might reject or misinterpret.
package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderNamePayPal is the stable Name() this provider registers under.
const ProviderNamePayPal = "paypal"

// Env vars. No defaults for secrets. CACKLE_PAYPAL_ENV must be exactly
// "live" or "sandbox" — required explicitly (not defaulted either way) so
// a forgotten/misspelled value can never silently point at the wrong
// PayPal environment, which would be a real-money-vs-play-money mistake in
// either direction.
const (
	EnvPayPalClientID     = "CACKLE_PAYPAL_CLIENT_ID"
	EnvPayPalClientSecret = "CACKLE_PAYPAL_CLIENT_SECRET"
	EnvPayPalWebhookID    = "CACKLE_PAYPAL_WEBHOOK_ID"
	EnvPayPalEnv          = "CACKLE_PAYPAL_ENV"
)

const (
	paypalLiveBaseURL    = "https://api-m.paypal.com"
	paypalSandboxBaseURL = "https://api-m.sandbox.paypal.com"

	paypalHTTPTimeout  = 15 * time.Second
	paypalMaxBodyBytes = 1 << 20 // 1 MiB
)

// Sentinel errors specific to the PayPal adapter. Error strings never
// contain the client secret or an access token.
var (
	ErrPayPalClientIDNotConfigured     = errors.New("payments: paypal: " + EnvPayPalClientID + " not set")
	ErrPayPalClientSecretNotConfigured = errors.New("payments: paypal: " + EnvPayPalClientSecret + " not set")
	ErrPayPalWebhookIDNotConfigured    = errors.New("payments: paypal: " + EnvPayPalWebhookID + " not set")
	ErrPayPalEnvNotConfigured          = errors.New("payments: paypal: " + EnvPayPalEnv + " must be exactly \"live\" or \"sandbox\"")
	ErrPayPalMissingSignatureHeaders   = errors.New("payments: paypal: missing one or more PAYPAL-TRANSMISSION-* headers")
	ErrPayPalInvalidSignature          = errors.New("payments: paypal: webhook signature verification failed")
	ErrPayPalUnexpectedStatus          = errors.New("payments: paypal: unexpected API response status")
	ErrPayPalMalformedResponse         = errors.New("payments: paypal: malformed API response")
	ErrPayPalResponseTooLarge          = errors.New("payments: paypal: response body exceeds size limit")
	// ErrPayPalUnsupportedCurrency covers ISO-4217 three-decimal currencies
	// — see file doc comment HONESTY note 3.
	ErrPayPalUnsupportedCurrency = errors.New("payments: paypal: three-decimal ISO-4217 currency is not verified against PayPal's documented amount semantics; refusing rather than guessing")
)

// paypalZeroDecimalCurrencies is PayPal's own documented zero-decimal set
// (the amount.value decimal string carries NO decimal point at all, e.g.
// "100" not "100.00" or "100.0"). Source:
// https://developer.paypal.com/api/rest/reference/currency-codes/
var paypalZeroDecimalCurrencies = map[string]bool{
	"JPY": true, "HUF": true, "TWD": true,
}

var paypalThreeDecimalCurrencies = map[string]bool{
	"KWD": true, "BHD": true, "JOD": true, "OMR": true, "TND": true,
}

// paypalAmountValue formats o.AmountMinor (Cackle's ISO-4217 minor-unit
// integer) as the decimal string PayPal's amount.value field expects.
// AmountMinor is already scaled to the currency's real exponent (0 for
// JPY/HUF/TWD, 2 for everything else this function accepts), so this is
// purely a string-formatting exercise, not a numeric conversion: insert a
// decimal point two digits from the right for ordinary currencies, or
// none at all for PayPal's documented zero-decimal set.
func paypalAmountValue(amountMinor int64, currency string) (string, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if paypalThreeDecimalCurrencies[cur] {
		return "", fmt.Errorf("%w: %s", ErrPayPalUnsupportedCurrency, cur)
	}
	if paypalZeroDecimalCurrencies[cur] {
		return strconv.FormatInt(amountMinor, 10), nil
	}
	neg := amountMinor < 0
	v := amountMinor
	if neg {
		v = -v
	}
	whole := v / 100
	frac := v % 100
	s := fmt.Sprintf("%d.%02d", whole, frac)
	if neg {
		s = "-" + s
	}
	return s, nil
}

// paypalAmountValueToMinor is the inverse of paypalAmountValue, for
// reconciling a settled amount PayPal reports back into Cackle's
// AmountMinor.
func paypalAmountValueToMinor(value, currency string) (int64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if paypalThreeDecimalCurrencies[cur] {
		return 0, fmt.Errorf("%w: %s", ErrPayPalUnsupportedCurrency, cur)
	}
	value = strings.TrimSpace(value)
	if paypalZeroDecimalCurrencies[cur] {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: zero-decimal amount %q is not an integer", ErrPayPalMalformedResponse, value)
		}
		return n, nil
	}
	parts := strings.SplitN(value, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: amount %q has an invalid whole part", ErrPayPalMalformedResponse, value)
	}
	frac := int64(0)
	if len(parts) == 2 {
		fracStr := parts[1]
		for len(fracStr) < 2 {
			fracStr += "0"
		}
		if len(fracStr) > 2 {
			fracStr = fracStr[:2]
		}
		frac, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: amount %q has an invalid fractional part", ErrPayPalMalformedResponse, value)
		}
	}
	sign := int64(1)
	if whole < 0 {
		sign = -1
		whole = -whole
	}
	return sign * (whole*100 + frac), nil
}

// PayPalProvider implements Provider against PayPal's Orders v2 API. As
// with every adapter in this package, PayPal pays the organiser's OWN
// PayPal account directly — Cackle never touches the money.
type PayPalProvider struct {
	clientID     string
	clientSecret string
	webhookID    string
	httpClient   *http.Client
	baseURL      string
}

// NewPayPal constructs a PayPalProvider from CACKLE_PAYPAL_CLIENT_ID,
// CACKLE_PAYPAL_CLIENT_SECRET, CACKLE_PAYPAL_WEBHOOK_ID, and
// CACKLE_PAYPAL_ENV ("live" or "sandbox", required explicitly).
func NewPayPal() (*PayPalProvider, error) {
	clientID := strings.TrimSpace(os.Getenv(EnvPayPalClientID))
	if clientID == "" {
		return nil, ErrPayPalClientIDNotConfigured
	}
	clientSecret := strings.TrimSpace(os.Getenv(EnvPayPalClientSecret))
	if clientSecret == "" {
		return nil, ErrPayPalClientSecretNotConfigured
	}
	webhookID := strings.TrimSpace(os.Getenv(EnvPayPalWebhookID))
	if webhookID == "" {
		return nil, ErrPayPalWebhookIDNotConfigured
	}
	var baseURL string
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvPayPalEnv))) {
	case "live":
		baseURL = paypalLiveBaseURL
	case "sandbox":
		baseURL = paypalSandboxBaseURL
	default:
		return nil, ErrPayPalEnvNotConfigured
	}
	return &PayPalProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		webhookID:    webhookID,
		httpClient:   &http.Client{Timeout: paypalHTTPTimeout},
		baseURL:      baseURL,
	}, nil
}

// Name implements Provider.
func (p *PayPalProvider) Name() string { return ProviderNamePayPal }

// Capabilities implements Provider.
func (p *PayPalProvider) Capabilities() Capabilities {
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

// fetchAccessToken gets a client-credentials OAuth2 token. Not cached: a
// fresh token is fetched per Begin/Verify/Webhook call. This is simpler
// and unambiguously correct; a production deployment handling significant
// volume may want to cache tokens until their documented expiry, but that
// is an optimisation, not a correctness requirement, and is left as a
// follow-up rather than adding untested caching/locking logic here.
// https://developer.paypal.com/api/rest/authentication/
func (p *PayPalProvider) fetchAccessToken(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, paypalHTTPTimeout)
	defer cancel()

	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("payments: paypal: build token request: %w", err)
	}
	req.SetBasicAuth(p.clientID, p.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("payments: paypal: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := paypalReadLimited(resp.Body, paypalMaxBodyBytes)
	if err != nil {
		return "", fmt.Errorf("payments: paypal: read token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: token endpoint http %d", ErrPayPalUnexpectedStatus, resp.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("%w: token response: %v", ErrPayPalMalformedResponse, err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("%w: empty access_token", ErrPayPalMalformedResponse)
	}
	return parsed.AccessToken, nil
}

// Begin creates a PayPal Order (intent=CAPTURE) and returns the buyer
// approval redirect URL. https://developer.paypal.com/docs/api/orders/v2/#orders_create
func (p *PayPalProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: paypal: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: paypal: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: paypal: currency is required")
	}
	if strings.TrimSpace(o.CallbackURL) == "" {
		return Charge{}, errors.New("payments: paypal: callback_url is required")
	}
	value, err := paypalAmountValue(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, err
	}

	token, err := p.fetchAccessToken(ctx)
	if err != nil {
		return Charge{}, err
	}

	reqBody := map[string]any{
		"intent": "CAPTURE",
		"purchase_units": []map[string]any{
			{
				"reference_id": o.Reference,
				"custom_id":    o.Reference,
				"amount": map[string]string{
					"currency_code": currency,
					"value":         value,
				},
			},
		},
		"application_context": map[string]string{
			"return_url": o.CallbackURL,
			"cancel_url": o.CallbackURL,
		},
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/v2/checkout/orders", reqBody, token)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyPayPalError(status, respBody)
	}

	var parsed struct {
		ID    string `json:"id"`
		Links []struct {
			Rel  string `json:"rel"`
			HREF string `json:"href"`
		} `json:"links"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrPayPalMalformedResponse, err)
	}
	if parsed.ID == "" {
		return Charge{}, fmt.Errorf("%w: empty order id", ErrPayPalMalformedResponse)
	}
	var approveURL string
	for _, l := range parsed.Links {
		if l.Rel == "approve" {
			approveURL = l.HREF
			break
		}
	}
	if approveURL == "" {
		return Charge{}, fmt.Errorf("%w: no approve link in order response", ErrPayPalMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNamePayPal,
		Reference:   o.Reference,
		RedirectURL: approveURL,
	}, nil
}

// paypalPurchaseUnit is the subset of a PayPal Order's purchase_units
// entry this adapter reads, across both Get Order and Capture Order
// responses.
type paypalPurchaseUnit struct {
	ReferenceID string `json:"reference_id"`
	CustomID    string `json:"custom_id"`
	Amount      struct {
		CurrencyCode string `json:"currency_code"`
		Value        string `json:"value"`
	} `json:"amount"`
	Payments struct {
		Captures []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Amount struct {
				CurrencyCode string `json:"currency_code"`
				Value        string `json:"value"`
			} `json:"amount"`
		} `json:"captures"`
	} `json:"payments"`
}

// Verify looks up a PayPal Order by id, and if the buyer has approved it
// (status APPROVED), captures it — this is the point at which money
// actually moves, so Verify is where this adapter performs the capture
// step a client-side PayPal Buttons integration would otherwise trigger.
// Fails closed on any transport, parse, or ambiguous status: only a
// capture with status COMPLETED is ever reported as StatusPaid.
// https://developer.paypal.com/docs/api/orders/v2/#orders_get
// https://developer.paypal.com/docs/api/orders/v2/#orders_capture
//
// reference is a PayPal order id (not Cackle's own order reference) — see
// the same Verify-identity caveat documented in stripe.go.
func (p *PayPalProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: paypal: reference is required")
	}

	token, err := p.fetchAccessToken(ctx)
	if err != nil {
		return Result{}, err
	}

	respBody, status, err := p.do(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(reference), nil, token)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyPayPalError(status, respBody)
	}

	var order struct {
		ID            string               `json:"id"`
		Status        string               `json:"status"`
		PurchaseUnits []paypalPurchaseUnit `json:"purchase_units"`
	}
	if err := json.Unmarshal(respBody, &order); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrPayPalMalformedResponse, err)
	}
	if order.ID == "" {
		return Result{}, fmt.Errorf("%w: missing order id", ErrPayPalMalformedResponse)
	}

	switch order.Status {
	case "COMPLETED":
		return paypalResultFromCaptures(order.ID, order.PurchaseUnits, respBody)
	case "APPROVED":
		// Buyer has approved — capture now.
		capRespBody, capStatus, err := p.do(ctx, http.MethodPost, "/v2/checkout/orders/"+url.PathEscape(reference)+"/capture", map[string]any{}, token)
		if err != nil {
			return Result{}, err
		}
		if capStatus < 200 || capStatus >= 300 {
			return Result{}, classifyPayPalError(capStatus, capRespBody)
		}
		var captured struct {
			ID            string               `json:"id"`
			Status        string               `json:"status"`
			PurchaseUnits []paypalPurchaseUnit `json:"purchase_units"`
		}
		if err := json.Unmarshal(capRespBody, &captured); err != nil {
			return Result{}, fmt.Errorf("%w: capture response: %v", ErrPayPalMalformedResponse, err)
		}
		if captured.Status != "COMPLETED" {
			// Fail closed: a capture call that didn't complete is not paid,
			// whatever status it did come back with.
			return Result{Provider: ProviderNamePayPal, Reference: reference, Status: StatusFailed, Raw: json.RawMessage(capRespBody)}, nil
		}
		return paypalResultFromCaptures(captured.ID, captured.PurchaseUnits, capRespBody)
	default:
		// CREATED, PAYER_ACTION_REQUIRED, VOIDED, or anything unrecognised:
		// never paid. (See file doc comment HONESTY note 1 on the enum.)
		return Result{Provider: ProviderNamePayPal, Reference: reference, Status: StatusFailed, Raw: json.RawMessage(respBody)}, nil
	}
}

// paypalResultFromCaptures builds a Result from the first captured payment
// found in units, failing closed if there is none or its amount is
// unreadable.
func paypalResultFromCaptures(orderID string, units []paypalPurchaseUnit, raw []byte) (Result, error) {
	for _, u := range units {
		for _, c := range u.Payments.Captures {
			if c.Status != "COMPLETED" {
				continue
			}
			ref := u.CustomID
			if ref == "" {
				ref = u.ReferenceID
			}
			if ref == "" {
				return Result{}, fmt.Errorf("%w: completed capture has no custom_id/reference_id", ErrPayPalMalformedResponse)
			}
			currency := strings.ToUpper(c.Amount.CurrencyCode)
			minor, err := paypalAmountValueToMinor(c.Amount.Value, currency)
			if err != nil {
				return Result{}, err
			}
			if minor <= 0 {
				return Result{}, fmt.Errorf("%w: completed capture with non-positive amount", ErrPayPalMalformedResponse)
			}
			return Result{
				Provider:    ProviderNamePayPal,
				Reference:   ref,
				EventID:     c.ID,
				Status:      StatusPaid,
				AmountMinor: minor,
				Currency:    currency,
				Raw:         json.RawMessage(raw),
			}, nil
		}
	}
	return Result{}, fmt.Errorf("%w: order %s has no COMPLETED capture", ErrPayPalMalformedResponse, orderID)
}

// paypalWebhookEvent is PayPal's standard webhook event envelope. See file
// doc comment HONESTY note 2.
type paypalWebhookEvent struct {
	ID        string          `json:"id"`
	EventType string          `json:"event_type"`
	Resource  json.RawMessage `json:"resource"`
}

// Webhook verifies a PayPal webhook by calling PayPal's OWN
// verify-webhook-signature endpoint (PayPal does not use a simple local
// HMAC — verification is a server-to-server round trip) and returns the
// settled result for a PAYMENT.CAPTURE.COMPLETED event. Fails closed at
// every step: missing transmission headers, a non-SUCCESS verification
// result, a transport/parse error, or an unrecognised event type are all
// errors. https://developer.paypal.com/api/rest/webhooks/rest/
func (p *PayPalProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	transmissionID := r.Header.Get("Paypal-Transmission-Id")
	transmissionTime := r.Header.Get("Paypal-Transmission-Time")
	certURL := r.Header.Get("Paypal-Cert-Url")
	authAlgo := r.Header.Get("Paypal-Auth-Algo")
	transmissionSig := r.Header.Get("Paypal-Transmission-Sig")
	if transmissionID == "" || transmissionTime == "" || certURL == "" || authAlgo == "" || transmissionSig == "" {
		return Result{}, ErrPayPalMissingSignatureHeaders
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrPayPalMalformedResponse)
	}
	body, err := paypalReadLimited(r.Body, paypalMaxBodyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("payments: paypal: read webhook body: %w", err)
	}
	if !json.Valid(body) {
		// Fail closed before spending an outbound call on PayPal's
		// verify-webhook-signature endpoint: a body that isn't even valid
		// JSON can never be a genuine PayPal event, and embedding it in a
		// json.RawMessage for the verify request would otherwise surface
		// as an opaque request-encoding error instead of a clear,
		// classified one.
		return Result{}, fmt.Errorf("%w: webhook body is not valid JSON", ErrPayPalMalformedResponse)
	}

	token, err := p.fetchAccessToken(ctx)
	if err != nil {
		return Result{}, err
	}

	// webhook_event MUST be the raw, unmodified body — PayPal's docs
	// explicitly warn that re-serializing it breaks verification.
	// json.RawMessage's MarshalJSON returns its bytes verbatim, so
	// embedding `body` this way never re-formats it.
	verifyReq := struct {
		TransmissionID   string          `json:"transmission_id"`
		TransmissionTime string          `json:"transmission_time"`
		CertURL          string          `json:"cert_url"`
		AuthAlgo         string          `json:"auth_algo"`
		TransmissionSig  string          `json:"transmission_sig"`
		WebhookID        string          `json:"webhook_id"`
		WebhookEvent     json.RawMessage `json:"webhook_event"`
	}{
		TransmissionID:   transmissionID,
		TransmissionTime: transmissionTime,
		CertURL:          certURL,
		AuthAlgo:         authAlgo,
		TransmissionSig:  transmissionSig,
		WebhookID:        p.webhookID,
		WebhookEvent:     json.RawMessage(body),
	}

	verifyRespBody, status, err := p.do(ctx, http.MethodPost, "/v1/notifications/verify-webhook-signature", verifyReq, token)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyPayPalError(status, verifyRespBody)
	}
	var verifyResp struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.Unmarshal(verifyRespBody, &verifyResp); err != nil {
		return Result{}, fmt.Errorf("%w: verify response: %v", ErrPayPalMalformedResponse, err)
	}
	if verifyResp.VerificationStatus != "SUCCESS" {
		return Result{}, fmt.Errorf("%w: verification_status=%q", ErrPayPalInvalidSignature, verifyResp.VerificationStatus)
	}

	var event paypalWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrPayPalMalformedResponse, err)
	}
	if event.EventType != "PAYMENT.CAPTURE.COMPLETED" {
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, event.EventType)
	}

	var capture struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		CustomID string `json:"custom_id"`
		Amount   struct {
			CurrencyCode string `json:"currency_code"`
			Value        string `json:"value"`
		} `json:"amount"`
		SupplementaryData struct {
			RelatedIDs struct {
				OrderID string `json:"order_id"`
			} `json:"related_ids"`
		} `json:"supplementary_data"`
	}
	if err := json.Unmarshal(event.Resource, &capture); err != nil {
		return Result{}, fmt.Errorf("%w: event resource: %v", ErrPayPalMalformedResponse, err)
	}
	if capture.Status != "" && capture.Status != "COMPLETED" {
		return Result{}, fmt.Errorf("%w: PAYMENT.CAPTURE.COMPLETED event carried resource.status=%q", ErrPayPalMalformedResponse, capture.Status)
	}
	ref := capture.CustomID
	if ref == "" {
		return Result{}, fmt.Errorf("%w: capture resource has no custom_id to reconcile against", ErrPayPalMalformedResponse)
	}
	currency := strings.ToUpper(capture.Amount.CurrencyCode)
	minor, err := paypalAmountValueToMinor(capture.Amount.Value, currency)
	if err != nil {
		return Result{}, err
	}
	if minor <= 0 {
		return Result{}, fmt.Errorf("%w: non-positive amount", ErrPayPalMalformedResponse)
	}

	eventID := event.ID
	if eventID == "" {
		eventID = capture.ID
	}
	return Result{
		Provider:    ProviderNamePayPal,
		Reference:   ref,
		EventID:     eventID,
		Status:      StatusPaid,
		AmountMinor: minor,
		Currency:    currency,
		Raw:         json.RawMessage(body),
	}, nil
}

// do issues an authenticated JSON request against the PayPal API, bounding
// it with paypalHTTPTimeout regardless of the caller's own context, and
// caps the response body it reads.
func (p *PayPalProvider) do(ctx context.Context, method, path string, body any, accessToken string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, paypalHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: paypal: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: paypal: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: paypal: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := paypalReadLimited(resp.Body, paypalMaxBodyBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("payments: paypal: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// paypalReadLimited reads at most limit bytes from r, returning
// ErrPayPalResponseTooLarge if there was more.
func paypalReadLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrPayPalResponseTooLarge
	}
	return b, nil
}

// paypalErrorEnvelope is PayPal's documented error response shape:
// {"name":"...","message":"...","debug_id":"..."}.
type paypalErrorEnvelope struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// classifyPayPalError builds an error for a non-2xx PayPal response,
// best-effort including PayPal's own message without ever including
// request headers, the client secret, or an access token.
func classifyPayPalError(status int, body []byte) error {
	var env paypalErrorEnvelope
	_ = json.Unmarshal(body, &env)
	msg := env.Message
	if msg == "" {
		msg = env.Name
	}
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrPayPalUnexpectedStatus, status, msg)
}
