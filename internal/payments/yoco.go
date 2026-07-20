// Package payments: Yoco adapter (South Africa).
//
// Reference: https://developer.yoco.com/online/resources/checkouts
// (Checkouts API — create + retrieve) and
// https://developer.yoco.com/online/resources/webhooks (webhook
// verification: Yoco uses the Svix webhook standard — headers
// webhook-id, webhook-timestamp, webhook-signature; secret format
// "whsec_<base64>"; signed content is "{id}.{timestamp}.{body}" joined by
// periods, HMAC-SHA256'd with the base64-decoded secret, base64-encoded,
// and compared against any "v1,<base64-sig>" entry in the
// space-separated webhook-signature header).
//
// Confidence: MEDIUM-HIGH. Yoco explicitly documents using the Svix
// webhook standard, which this file implements faithfully including the
// signed-content template and the timestamp-tolerance replay guard Svix
// recommends. The Checkouts API request/response field names are
// implemented from Yoco's own docs but have not been run against a real
// Yoco sandbox account.
//
// Yoco is ZAR-only — its amount field is already integer minor units
// (cents), matching Cackle's own AmountMinor directly. No currency
// conversion is needed or attempted; any other currency is rejected.
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
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderNameYoco is the stable Name() this provider registers under.
const ProviderNameYoco = "yoco"

const (
	EnvYocoSecretKey     = "CACKLE_YOCO_SECRET_KEY"
	EnvYocoWebhookSecret = "CACKLE_YOCO_WEBHOOK_SECRET" // format: whsec_<base64>
	yocoAPIBase          = "https://payments.yoco.com/api"
	yocoHTTPTimeout      = 15 * time.Second
	yocoMaxResponseSize  = 1 << 20
	// yocoWebhookTolerance is the maximum age (either direction) Svix
	// recommends allowing for a webhook-timestamp before treating the
	// delivery as stale/replayed, independent of the caller's own
	// SeenStore-based replay protection.
	yocoWebhookTolerance = 5 * time.Minute
)

var (
	ErrYocoSecretNotConfigured        = errors.New("payments: yoco: " + EnvYocoSecretKey + " not set")
	ErrYocoWebhookSecretNotConfigured = errors.New("payments: yoco: " + EnvYocoWebhookSecret + " not set")
	ErrYocoUnsupportedCurrency        = errors.New("payments: yoco: only ZAR is supported")
	ErrYocoMissingSignature           = errors.New("payments: yoco: missing webhook-id/webhook-timestamp/webhook-signature headers")
	ErrYocoInvalidSignature           = errors.New("payments: yoco: invalid webhook signature")
	ErrYocoStaleTimestamp             = errors.New("payments: yoco: webhook timestamp outside tolerance window")
	ErrYocoUnexpectedStatus           = errors.New("payments: yoco: unexpected API response status")
	ErrYocoMalformedResponse          = errors.New("payments: yoco: malformed API response")
	ErrYocoResponseTooLarge           = errors.New("payments: yoco: response body exceeds size limit")
)

// YocoProvider implements Provider against the Yoco Checkouts API.
type YocoProvider struct {
	secretKey     string
	webhookSecret []byte // decoded from the whsec_ base64 payload
	httpClient    *http.Client
	baseURL       string
}

// NewYoco constructs a YocoProvider from EnvYocoSecretKey and
// EnvYocoWebhookSecret (expected in Yoco/Svix's "whsec_<base64>" format).
func NewYoco() (*YocoProvider, error) {
	secret := strings.TrimSpace(os.Getenv(EnvYocoSecretKey))
	if secret == "" {
		return nil, ErrYocoSecretNotConfigured
	}
	whsec := strings.TrimSpace(os.Getenv(EnvYocoWebhookSecret))
	if whsec == "" {
		return nil, ErrYocoWebhookSecretNotConfigured
	}
	decoded, err := decodeYocoWebhookSecret(whsec)
	if err != nil {
		return nil, fmt.Errorf("payments: yoco: %s: %w", EnvYocoWebhookSecret, err)
	}
	return &YocoProvider{
		secretKey:     secret,
		webhookSecret: decoded,
		httpClient:    &http.Client{Timeout: yocoHTTPTimeout},
		baseURL:       yocoAPIBase,
	}, nil
}

func decodeYocoWebhookSecret(whsec string) ([]byte, error) {
	payload := strings.TrimPrefix(whsec, "whsec_")
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("not valid base64 after whsec_ prefix: %w", err)
	}
	return decoded, nil
}

// Name implements Provider.
func (p *YocoProvider) Name() string { return ProviderNameYoco }

// Capabilities implements Provider.
func (p *YocoProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    []string{"ZAR"},
		Countries:     []string{"ZA"},
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: false, // ZAR only; never exercised against 0/3-decimal currencies
	}
}

// Begin creates a Yoco Checkout and returns its hosted redirectUrl.
func (p *YocoProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: yoco: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: yoco: amount_minor must be positive")
	}
	if !strings.EqualFold(o.Currency, "ZAR") {
		return Charge{}, fmt.Errorf("%w: got %q", ErrYocoUnsupportedCurrency, o.Currency)
	}

	reqBody := map[string]any{
		"amount":   o.AmountMinor,
		"currency": "ZAR",
		"metadata": map[string]string{"reference": o.Reference},
	}
	if o.CallbackURL != "" {
		reqBody["successUrl"] = o.CallbackURL
		reqBody["cancelUrl"] = o.CallbackURL
		reqBody["failureUrl"] = o.CallbackURL
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/checkouts", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyYocoError(status, respBody)
	}
	var parsed struct {
		ID          string `json:"id"`
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrYocoMalformedResponse, err)
	}
	if parsed.RedirectURL == "" || parsed.ID == "" {
		return Charge{}, fmt.Errorf("%w: empty id or redirectUrl", ErrYocoMalformedResponse)
	}
	return Charge{
		Provider:    ProviderNameYoco,
		Reference:   parsed.ID,
		RedirectURL: parsed.RedirectURL,
	}, nil
}

type yocoCheckout struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Metadata struct {
		Reference string `json:"reference"`
	} `json:"metadata"`
}

func (c yocoCheckout) toResult(raw []byte) (Result, error) {
	if c.ID == "" {
		return Result{}, fmt.Errorf("%w: missing id", ErrYocoMalformedResponse)
	}
	result := Result{
		Provider:    ProviderNameYoco,
		Reference:   c.ID,
		EventID:     c.ID,
		AmountMinor: c.Amount,
		Currency:    c.Currency,
		Raw:         json.RawMessage(raw),
	}
	switch strings.ToLower(c.Status) {
	case "completed", "succeeded":
		if c.Amount <= 0 {
			return Result{}, fmt.Errorf("%w: completed status with non-positive amount", ErrYocoMalformedResponse)
		}
		result.Status = StatusPaid
	case "cancelled", "failed", "expired":
		result.Status = StatusFailed
	default:
		result.Status = StatusFailed
	}
	return result, nil
}

// Verify retrieves a Checkout by its Yoco-assigned id (Cackle's
// reference, returned by Begin).
func (p *YocoProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: yoco: reference is required")
	}
	respBody, status, err := p.do(ctx, http.MethodGet, "/checkouts/"+reference, nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyYocoError(status, respBody)
	}
	var parsed yocoCheckout
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrYocoMalformedResponse, err)
	}
	if parsed.ID != "" && parsed.ID != reference {
		return Result{}, fmt.Errorf("%w: returned id %q for requested %q", ErrYocoMalformedResponse, parsed.ID, reference)
	}
	return parsed.toResult(respBody)
}

// Webhook validates Yoco's Svix-standard signature (webhook-id,
// webhook-timestamp, webhook-signature headers) and returns the settled
// result for a payment.succeeded event.
func (p *YocoProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	id := strings.TrimSpace(r.Header.Get("webhook-id"))
	timestamp := strings.TrimSpace(r.Header.Get("webhook-timestamp"))
	sigHeader := strings.TrimSpace(r.Header.Get("webhook-signature"))
	if id == "" || timestamp == "" || sigHeader == "" {
		return Result{}, ErrYocoMissingSignature
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return Result{}, fmt.Errorf("%w: malformed webhook-timestamp", ErrYocoInvalidSignature)
	}
	age := time.Since(time.Unix(ts, 0))
	if age < 0 {
		age = -age
	}
	if age > yocoWebhookTolerance {
		return Result{}, ErrYocoStaleTimestamp
	}

	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrYocoMalformedResponse)
	}
	body, err := boundedRead(r.Body, yocoMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrYocoResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: yoco: read webhook body: %w", err)
	}

	signedContent := id + "." + timestamp + "." + string(body)
	mac := hmac.New(sha256.New, p.webhookSecret)
	mac.Write([]byte(signedContent))
	expected := mac.Sum(nil)

	valid := false
	for _, entry := range strings.Fields(sigHeader) {
		version, sig, ok := strings.Cut(entry, ",")
		if !ok || version != "v1" {
			continue
		}
		given, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			continue
		}
		if hmac.Equal(expected, given) {
			valid = true
			break
		}
	}
	if !valid {
		return Result{}, ErrYocoInvalidSignature
	}

	var envelope struct {
		Type    string       `json:"type"`
		Payload yocoCheckout `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrYocoMalformedResponse, err)
	}
	if envelope.Type != "payment.succeeded" {
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Type)
	}
	if envelope.Payload.Status == "" {
		envelope.Payload.Status = "completed"
	}
	return envelope.Payload.toResult(body)
}

func (p *YocoProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, yocoHTTPTimeout)
	defer cancel()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: yoco: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: yoco: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: yoco: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, yocoMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrYocoResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: yoco: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyYocoError(status int, body []byte) error {
	var env struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrYocoUnexpectedStatus, status, msg)
}
