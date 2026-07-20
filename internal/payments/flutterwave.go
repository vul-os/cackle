// Package payments: Flutterwave adapter.
//
// Reference: https://developer.flutterwave.com/docs/collecting-payments/standard
// (Standard payment initialize + verify) and
// https://developer.flutterwave.com/docs/integration-guides/webhooks
// (webhook verification).
//
// Confidence: MEDIUM. Flutterwave's v3 API shape (Bearer secret key,
// /v3/payments to initialize, /v3/transactions/verify_by_reference to
// verify, tx_ref as the merchant reference) is well established and
// widely documented, and is implemented here from that documentation —
// but this has NOT been run against a real Flutterwave sandbox account.
// The one point most worth double-checking before production use: unlike
// Paystack/Razorpay/Yoco, Flutterwave's `amount` field is a plain JSON
// number in MAJOR units (e.g. 100 means ₦100, not 100 kobo) — see
// minorToMajorString/majorStringToMinor in currency.go for the exact
// conversion this file relies on.
//
// Flutterwave's webhook verification is NOT an HMAC: it is a static
// shared secret ("hash") you configure in the Flutterwave dashboard,
// echoed back verbatim in the `verif-hash` header on every webhook
// delivery. This file still compares it in constant time (hmac.Equal)
// to avoid a timing side-channel, even though it isn't a MAC.
package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderNameFlutterwave is the stable Name() this provider registers under.
const ProviderNameFlutterwave = "flutterwave"

// EnvFlutterwaveSecretKey is the Flutterwave secret key (Bearer token for
// the API). EnvFlutterwaveWebhookHash is the static "hash" value
// configured in the Flutterwave dashboard's webhook settings, echoed back
// in the verif-hash header on every webhook delivery. Both come ONLY from
// the environment — no defaults, never logged.
const (
	EnvFlutterwaveSecretKey    = "CACKLE_FLUTTERWAVE_SECRET_KEY"
	EnvFlutterwaveWebhookHash  = "CACKLE_FLUTTERWAVE_WEBHOOK_HASH"
	flutterwaveAPIBase         = "https://api.flutterwave.com/v3"
	flutterwaveHTTPTimeout     = 15 * time.Second
	flutterwaveMaxResponseSize = 1 << 20 // 1 MiB
)

var flutterwaveCurrencies = []string{"NGN", "GHS", "KES", "UGX", "TZS", "ZAR", "USD", "XOF", "XAF", "RWF"}
var flutterwaveCountries = []string{"NG", "GH", "KE", "UG", "TZ", "ZA", "RW", "CI", "CM"}

var (
	ErrFlutterwaveSecretNotConfigured = errors.New("payments: flutterwave: " + EnvFlutterwaveSecretKey + " not set")
	ErrFlutterwaveHashNotConfigured   = errors.New("payments: flutterwave: " + EnvFlutterwaveWebhookHash + " not set")
	ErrFlutterwaveMissingSignature    = errors.New("payments: flutterwave: missing verif-hash header")
	ErrFlutterwaveInvalidSignature    = errors.New("payments: flutterwave: invalid verif-hash")
	ErrFlutterwaveUnexpectedStatus    = errors.New("payments: flutterwave: unexpected API response status")
	ErrFlutterwaveMalformedResponse   = errors.New("payments: flutterwave: malformed API response")
	ErrFlutterwaveResponseTooLarge    = errors.New("payments: flutterwave: response body exceeds size limit")
)

// FlutterwaveProvider implements Provider against the Flutterwave v3 API.
type FlutterwaveProvider struct {
	secretKey   string
	webhookHash string
	httpClient  *http.Client
	baseURL     string
}

// NewFlutterwave constructs a FlutterwaveProvider from
// EnvFlutterwaveSecretKey and EnvFlutterwaveWebhookHash. Both are
// required: without the webhook hash, incoming webhooks could never be
// verified, so this fails closed at construction time rather than at the
// first webhook delivery.
func NewFlutterwave() (*FlutterwaveProvider, error) {
	secret := strings.TrimSpace(os.Getenv(EnvFlutterwaveSecretKey))
	if secret == "" {
		return nil, ErrFlutterwaveSecretNotConfigured
	}
	hash := strings.TrimSpace(os.Getenv(EnvFlutterwaveWebhookHash))
	if hash == "" {
		return nil, ErrFlutterwaveHashNotConfigured
	}
	return &FlutterwaveProvider{
		secretKey:   secret,
		webhookHash: hash,
		httpClient:  &http.Client{Timeout: flutterwaveHTTPTimeout},
		baseURL:     flutterwaveAPIBase,
	}, nil
}

// Name implements Provider.
func (p *FlutterwaveProvider) Name() string { return ProviderNameFlutterwave }

// Capabilities implements Provider.
func (p *FlutterwaveProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    flutterwaveCurrencies,
		Countries:     flutterwaveCountries,
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: false, // untested against a zero-decimal currency
	}
}

// Begin initializes a Flutterwave Standard payment and returns its hosted
// checkout link.
func (p *FlutterwaveProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: flutterwave: order id is required as tx_ref")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: flutterwave: amount_minor must be positive")
	}
	if strings.TrimSpace(o.Currency) == "" {
		return Charge{}, errors.New("payments: flutterwave: currency is required")
	}
	if strings.TrimSpace(o.BuyerEmail) == "" {
		return Charge{}, errors.New("payments: flutterwave: buyer email is required")
	}
	majorAmount, err := minorToMajorString(o.AmountMinor, o.Currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: flutterwave: %w", err)
	}

	reqBody := map[string]any{
		"tx_ref":       o.Reference,
		"amount":       majorAmount,
		"currency":     strings.ToUpper(o.Currency),
		"redirect_url": o.CallbackURL,
		"customer": map[string]string{
			"email": o.BuyerEmail,
			"name":  o.BuyerName,
		},
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/payments", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyFlutterwaveError(status, respBody)
	}

	var parsed struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Link string `json:"link"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrFlutterwaveMalformedResponse, err)
	}
	if parsed.Status != "success" || parsed.Data.Link == "" {
		return Charge{}, fmt.Errorf("%w: status=%q or empty link", ErrFlutterwaveMalformedResponse, parsed.Status)
	}

	return Charge{
		Provider:    ProviderNameFlutterwave,
		Reference:   o.Reference,
		RedirectURL: parsed.Data.Link,
	}, nil
}

// flutterwaveTransactionPayload is the common shape of a Flutterwave
// transaction object, returned both by verify_by_reference and inside a
// charge.completed webhook's `data`.
type flutterwaveTransactionPayload struct {
	ID       int64       `json:"id"`
	TxRef    string      `json:"tx_ref"`
	FlwRef   string      `json:"flw_ref"`
	Amount   json.Number `json:"amount"`
	Currency string      `json:"currency"`
	Status   string      `json:"status"`
}

func (t flutterwaveTransactionPayload) toResult(raw []byte) (Result, error) {
	if t.TxRef == "" {
		return Result{}, fmt.Errorf("%w: missing tx_ref", ErrFlutterwaveMalformedResponse)
	}
	amountMinor, err := majorStringToMinor(t.Amount.String(), t.Currency)
	if err != nil {
		return Result{}, fmt.Errorf("%w: amount %q: %v", ErrFlutterwaveMalformedResponse, t.Amount.String(), err)
	}
	result := Result{
		Provider:    ProviderNameFlutterwave,
		Reference:   t.TxRef,
		EventID:     strconv.FormatInt(t.ID, 10),
		AmountMinor: amountMinor,
		Currency:    t.Currency,
		Raw:         json.RawMessage(raw),
	}
	switch t.Status {
	case "successful":
		if amountMinor <= 0 {
			return Result{}, fmt.Errorf("%w: successful status with non-positive amount", ErrFlutterwaveMalformedResponse)
		}
		result.Status = StatusPaid
	case "failed", "cancelled":
		result.Status = StatusFailed
	default:
		// Fail closed: anything not explicitly "successful" is never paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// Verify confirms a tx_ref directly against Flutterwave's
// verify_by_reference endpoint.
func (p *FlutterwaveProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: flutterwave: reference is required")
	}
	respBody, status, err := p.do(ctx, http.MethodGet, "/transactions/verify_by_reference?tx_ref="+url.QueryEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyFlutterwaveError(status, respBody)
	}
	var parsed struct {
		Status string                        `json:"status"`
		Data   flutterwaveTransactionPayload `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrFlutterwaveMalformedResponse, err)
	}
	if parsed.Status != "success" {
		return Result{}, fmt.Errorf("%w: status=%q", ErrFlutterwaveUnexpectedStatus, parsed.Status)
	}
	if parsed.Data.TxRef != "" && parsed.Data.TxRef != reference {
		return Result{}, fmt.Errorf("%w: returned tx_ref %q for requested %q", ErrFlutterwaveMalformedResponse, parsed.Data.TxRef, reference)
	}
	return parsed.Data.toResult(respBody)
}

// Webhook validates Flutterwave's verif-hash header (a static shared
// secret, compared in constant time) and returns the settled result for a
// charge.completed event.
func (p *FlutterwaveProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	given := strings.TrimSpace(r.Header.Get("verif-hash"))
	if given == "" {
		return Result{}, ErrFlutterwaveMissingSignature
	}
	if !hmac.Equal([]byte(given), []byte(p.webhookHash)) {
		return Result{}, ErrFlutterwaveInvalidSignature
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrFlutterwaveMalformedResponse)
	}
	body, err := boundedRead(r.Body, flutterwaveMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrFlutterwaveResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: flutterwave: read webhook body: %w", err)
	}

	var envelope struct {
		Event string                        `json:"event"`
		Data  flutterwaveTransactionPayload `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrFlutterwaveMalformedResponse, err)
	}
	if envelope.Event != "charge.completed" {
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Event)
	}
	return envelope.Data.toResult(body)
}

func (p *FlutterwaveProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, flutterwaveHTTPTimeout)
	defer cancel()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: flutterwave: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: flutterwave: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: flutterwave: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, flutterwaveMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrFlutterwaveResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: flutterwave: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyFlutterwaveError(status int, body []byte) error {
	var env struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrFlutterwaveUnexpectedStatus, status, msg)
}
