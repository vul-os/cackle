// Package payments — Mollie adapter.
//
// Built against Mollie's DOCUMENTED public API, verified live against
// docs.mollie.com during this change. No sandbox/live account was used —
// see the test file, which exercises this adapter entirely against an
// httptest fake server, and the HONESTY note below on the one part of
// this file (decimal-string formatting for non-2-decimal currencies) that
// this adapter deliberately refuses rather than guesses at.
//
// Doc sources:
//   - Create payment: https://docs.mollie.com/reference/create-payment
//   - Webhooks:       https://docs.mollie.com/reference/webhooks
//
// See stripe.go's package doc comment for the assumed v2 Order/Result field
// shape every P1 adapter in this change codes against.
//
// # Mollie's webhook design is unusually — and deliberately — simple
//
// Mollie's classic webhook (the `webhookUrl` field on Create Payment) POSTs
// a SINGLE form-encoded parameter, `id`, and carries NO signature at all.
// Mollie's own docs are explicit about why this is safe: "the script
// behind your webhook URL should use that ID to fetch the payment status
// and act accordingly" — i.e. verification IS the authenticated
// server-to-server re-fetch of GET /v2/payments/{id}, not a signature
// check. A forged webhook call can, at most, make this adapter re-check a
// real payment's real status early — it can never fabricate a paid result,
// because the body of the webhook is never trusted for anything beyond
// "go look up this id". This file's Webhook method does exactly that: it
// never inspects r's body for anything but the id, and defers entirely to
// the same authenticated lookup Verify uses.
//
// (Mollie has since added an opt-in "next-gen webhooks" beta with a real
// HMAC signature over a richer payload — https://docs.mollie.com/reference/webhooks-new
// — but that is a different, separate subscription mechanism from the
// classic webhookUrl field this adapter uses, and was not built here since
// the classic mechanism above is already fail-closed by construction.)
//
// # HONESTY note: non-2-decimal currency formatting
//
// Mollie's amount.value field is a decimal string (e.g. "10.00"), and this
// adapter is fully confident about the ordinary 2-decimal case. It could
// NOT confirm, word-for-word against Mollie's own reference page, whether
// a zero-decimal currency like JPY must be sent as "1000" (no decimal
// point) or "1000.00" (padded) — secondary/SDK evidence suggests "1000",
// but that's corroborating evidence, not a primary-doc confirmation.
// Rather than guess and risk a 100x amount error, this adapter REFUSES
// zero-decimal and three-decimal ISO-4217 currencies outright — see
// ErrMollieUnsupportedCurrency. Ordinary 2-decimal currencies (the large
// majority: USD, EUR, GBP, ZAR, ...) are fully supported.
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

// ProviderNameMollie is the stable Name() this provider registers under.
const ProviderNameMollie = "mollie"

// EnvMollieAPIKey is the ONLY place the Mollie API key may come from. A
// single key covers both test (test_...) and live (live_...) modes —
// Mollie encodes environment in the key prefix itself, not a separate base
// URL, so there is only one env var here (unlike Adyen/Checkout.com/
// PayPal/Square which need an explicit base-URL/env selector).
const EnvMollieAPIKey = "CACKLE_MOLLIE_API_KEY"

// EnvMollieWebhookURL is the absolute, public HTTPS URL Mollie should call
// back on payment status changes (the Create Payment `webhookUrl` field).
// Mollie requires this be a real reachable URL at payment-creation time,
// so it is configuration, not a secret, but is still required with no
// default (a missing webhook URL would silently create payments Cackle is
// never told about).
const EnvMollieWebhookURL = "CACKLE_MOLLIE_WEBHOOK_URL"

const (
	mollieAPIBase      = "https://api.mollie.com/v2"
	mollieHTTPTimeout  = 15 * time.Second
	mollieMaxBodyBytes = 1 << 20 // 1 MiB
	// mollieFormBodyMaxBytes bounds the classic webhook's form-encoded
	// body, which is just "id=tr_...." — far smaller than
	// mollieMaxBodyBytes, but read through the same limited reader.
	mollieFormBodyMaxBytes = 1 << 16 // 64 KiB
)

// Sentinel errors specific to the Mollie adapter. Error strings never
// contain the API key.
var (
	ErrMollieAPIKeyNotConfigured = errors.New("payments: mollie: " + EnvMollieAPIKey + " not set")
	ErrMollieWebhookURLMissing   = errors.New("payments: mollie: " + EnvMollieWebhookURL + " not set")
	ErrMollieMissingID           = errors.New("payments: mollie: webhook body has no id parameter")
	ErrMollieUnexpectedStatus    = errors.New("payments: mollie: unexpected API response status")
	ErrMollieMalformedResponse   = errors.New("payments: mollie: malformed API response")
	ErrMollieResponseTooLarge    = errors.New("payments: mollie: response body exceeds size limit")
	// ErrMollieUnsupportedCurrency covers non-2-decimal ISO-4217
	// currencies — see file doc comment HONESTY note.
	ErrMollieUnsupportedCurrency = errors.New("payments: mollie: non-2-decimal ISO-4217 currency's amount.value string format is not verified against Mollie's documented semantics; refusing rather than guessing")
)

// mollieNonTwoDecimalCurrencies are the ISO-4217 currencies whose exponent
// is not 2 (the zero-decimal set from PAYMENTS-CONTRACT.md plus the
// three-decimal set) — refused, see file doc comment HONESTY note.
var mollieNonTwoDecimalCurrencies = map[string]bool{
	"JPY": true, "KRW": true, "VND": true, "CLP": true, "ISK": true,
	"KWD": true, "BHD": true, "JOD": true, "OMR": true, "TND": true,
}

// mollieAmountValue formats o.AmountMinor as Mollie's decimal-string
// amount.value, for the ordinary 2-decimal case this adapter is confident
// about. Non-2-decimal currencies are refused — see file doc comment.
func mollieAmountValue(amountMinor int64, currency string) (string, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if mollieNonTwoDecimalCurrencies[cur] {
		return "", fmt.Errorf("%w: %s", ErrMollieUnsupportedCurrency, cur)
	}
	neg := amountMinor < 0
	v := amountMinor
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d.%02d", v/100, v%100)
	if neg {
		s = "-" + s
	}
	return s, nil
}

// mollieAmountValueToMinor is the inverse of mollieAmountValue, for
// reconciling a settled amount Mollie reports back into Cackle's
// AmountMinor.
func mollieAmountValueToMinor(value, currency string) (int64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if mollieNonTwoDecimalCurrencies[cur] {
		return 0, fmt.Errorf("%w: %s", ErrMollieUnsupportedCurrency, cur)
	}
	parts := strings.SplitN(strings.TrimSpace(value), ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: amount %q has an invalid whole part", ErrMollieMalformedResponse, value)
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
			return 0, fmt.Errorf("%w: amount %q has an invalid fractional part", ErrMollieMalformedResponse, value)
		}
	}
	sign := int64(1)
	if whole < 0 {
		sign = -1
		whole = -whole
	}
	return sign * (whole*100 + frac), nil
}

// MollieProvider implements Provider against Mollie's Payments API. As
// with every adapter in this package, Mollie pays the organiser's OWN
// Mollie account directly — Cackle never touches the money.
type MollieProvider struct {
	apiKey     string
	webhookURL string
	httpClient *http.Client
	baseURL    string // overridable in tests only
}

// NewMollie constructs a MollieProvider from CACKLE_MOLLIE_API_KEY and
// CACKLE_MOLLIE_WEBHOOK_URL. Both are required.
func NewMollie() (*MollieProvider, error) {
	apiKey := strings.TrimSpace(os.Getenv(EnvMollieAPIKey))
	if apiKey == "" {
		return nil, ErrMollieAPIKeyNotConfigured
	}
	webhookURL := strings.TrimSpace(os.Getenv(EnvMollieWebhookURL))
	if webhookURL == "" {
		return nil, ErrMollieWebhookURLMissing
	}
	return &MollieProvider{
		apiKey:     apiKey,
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: mollieHTTPTimeout},
		baseURL:    mollieAPIBase,
	}, nil
}

// Name implements Provider.
func (p *MollieProvider) Name() string { return ProviderNameMollie }

// Capabilities implements Provider.
func (p *MollieProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil,
		Countries:     nil,
		Flow:          FlowRedirect,
		Refunds:       true,
		Payouts:       true,
		Webhooks:      true,
		ZeroDecimalOK: false, // see file HONESTY note: non-2-decimal currencies are refused, not handled
	}
}

// Begin creates a Mollie payment and returns its hosted checkout URL.
// https://docs.mollie.com/reference/create-payment
func (p *MollieProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: mollie: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: mollie: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: mollie: currency is required")
	}
	if strings.TrimSpace(o.CallbackURL) == "" {
		return Charge{}, errors.New("payments: mollie: callback_url is required")
	}
	value, err := mollieAmountValue(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, err
	}

	description := "Cackle order " + o.Reference
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
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: mollie: encode metadata: %w", err)
	}

	reqBody := map[string]any{
		"amount": map[string]string{
			"currency": currency,
			"value":    value,
		},
		"description": description,
		"redirectUrl": o.CallbackURL,
		"webhookUrl":  p.webhookURL,
		"metadata":    json.RawMessage(metaJSON),
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/payments", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyMollieError(status, respBody)
	}

	var parsed struct {
		ID    string `json:"id"`
		Links struct {
			Checkout struct {
				HREF string `json:"href"`
			} `json:"checkout"`
		} `json:"_links"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrMollieMalformedResponse, err)
	}
	if parsed.ID == "" || parsed.Links.Checkout.HREF == "" {
		return Charge{}, fmt.Errorf("%w: empty payment id or _links.checkout.href", ErrMollieMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNameMollie,
		Reference:   o.Reference,
		RedirectURL: parsed.Links.Checkout.HREF,
	}, nil
}

// molliePaymentPayload is the subset of a Mollie Payment object this
// adapter reads.
type molliePaymentPayload struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount struct {
		Currency string `json:"currency"`
		Value    string `json:"value"`
	} `json:"amount"`
	Metadata json.RawMessage `json:"metadata"`
}

// Verify retrieves a payment directly from Mollie by id — this IS Mollie's
// own documented verification mechanism (see file doc comment): the
// payment's authoritative state only ever comes from this authenticated
// GET, never from anything a webhook call claims. Fails closed on any
// transport, parse, or ambiguous status: only status "paid" is ever
// StatusPaid. https://docs.mollie.com/reference/get-payment
//
// reference is a Mollie payment id (tr_...), which is exactly what
// Charge.Reference is set to by Begin above — unlike several other
// adapters in this package, there is no reference-identity ambiguity here.
func (p *MollieProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: mollie: reference is required")
	}

	respBody, status, err := p.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyMollieError(status, respBody)
	}

	result, err := parseMolliePayment(respBody)
	if err != nil {
		return Result{}, err
	}
	result.Raw = json.RawMessage(respBody)
	return result, nil
}

func parseMolliePayment(body []byte) (Result, error) {
	var payload molliePaymentPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMollieMalformedResponse, err)
	}
	if payload.ID == "" {
		return Result{}, fmt.Errorf("%w: missing payment id", ErrMollieMalformedResponse)
	}
	ref := mollieMetadataReference(payload.Metadata)
	if ref == "" {
		return Result{}, fmt.Errorf("%w: payment has no cackle_reference metadata to reconcile against", ErrMollieMalformedResponse)
	}

	currency := strings.ToUpper(payload.Amount.Currency)
	result := Result{
		Provider:  ProviderNameMollie,
		Reference: ref,
		EventID:   payload.ID,
		Currency:  currency,
	}

	if payload.Status == "paid" {
		minor, err := mollieAmountValueToMinor(payload.Amount.Value, currency)
		if err != nil {
			return Result{}, err
		}
		if minor <= 0 {
			return Result{}, fmt.Errorf("%w: status=paid with non-positive amount", ErrMollieMalformedResponse)
		}
		result.AmountMinor = minor
		result.Status = StatusPaid
	} else {
		// open, pending, authorized, canceled, expired, failed, or
		// anything unrecognised: never paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// mollieMetadataReference extracts the cackle_reference field this
// adapter's Begin stores in the payment's metadata, tolerating metadata
// being absent or not an object (never trusted for reconciliation itself —
// only used to know WHICH order to reconcile against; Reconcile still
// checks the authoritative amount/currency/status separately).
func mollieMetadataReference(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return m["cackle_reference"]
}

// Webhook implements Mollie's classic webhook contract: read the id
// parameter from the form-encoded body, and IMMEDIATELY re-fetch that
// payment's authoritative status via the same code path Verify uses. See
// the file doc comment for why this is fail-closed by construction even
// though there is no signature: nothing from the request body is EVER
// trusted except which id to go look up.
// https://docs.mollie.com/reference/webhooks
func (p *MollieProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrMollieMalformedResponse)
	}
	body, err := mollieReadLimited(r.Body, mollieFormBodyMaxBytes)
	if err != nil {
		return Result{}, fmt.Errorf("payments: mollie: read webhook body: %w", err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return Result{}, fmt.Errorf("%w: body is not valid form encoding: %v", ErrMollieMalformedResponse, err)
	}
	id := strings.TrimSpace(values.Get("id"))
	if id == "" {
		return Result{}, ErrMollieMissingID
	}
	return p.Verify(ctx, id)
}

// do issues an authenticated JSON request against the Mollie API, bounding
// it with mollieHTTPTimeout regardless of the caller's own context, and
// caps the response body it reads.
func (p *MollieProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, mollieHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: mollie: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: mollie: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: mollie: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := mollieReadLimited(resp.Body, mollieMaxBodyBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("payments: mollie: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// mollieReadLimited reads at most limit bytes from r, returning
// ErrMollieResponseTooLarge if there was more.
func mollieReadLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrMollieResponseTooLarge
	}
	return b, nil
}

// mollieErrorEnvelope is Mollie's documented error response shape:
// {"status":..., "title":"...", "detail":"..."}.
type mollieErrorEnvelope struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// classifyMollieError builds an error for a non-2xx Mollie response,
// best-effort including Mollie's own message without ever including
// request headers or the API key.
func classifyMollieError(status int, body []byte) error {
	var env mollieErrorEnvelope
	_ = json.Unmarshal(body, &env)
	msg := env.Detail
	if msg == "" {
		msg = env.Title
	}
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrMollieUnexpectedStatus, status, msg)
}
