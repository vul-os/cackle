// Package payments: Razorpay adapter (India).
//
// Reference: https://razorpay.com/docs/api/orders/ (Create an Order),
// https://razorpay.com/docs/api/payments/fetch-payments-for-order
// (list payments for an order — used by Verify), and
// https://razorpay.com/docs/webhooks/validate-test/ (webhook signature:
// HMAC-SHA256 hex over the raw request body, header X-Razorpay-Signature).
//
// Confidence: HIGH on the security-critical paths (order creation and
// webhook signature verification are Razorpay's most heavily used,
// consistently documented endpoints), MEDIUM on some response field
// completeness, since this has not been run against a real Razorpay test
// account.
//
// Razorpay's Standard Checkout is an INLINE flow, not a redirect: Begin
// creates an Order server-side and returns its id; the buyer-facing
// client is expected to open Razorpay's Checkout.js widget with that
// order id and the (non-secret) key_id, which is why Capabilities().Flow
// is FlowInline and Charge.RedirectURL is always empty here — the caller
// must render Checkout.js itself using Charge.Reference (the Razorpay
// order id) and Charge.Instructions.
//
// Amounts are integer minor units (paise for INR) matching Cackle's own
// AmountMinor directly — no conversion needed, same convention as
// Stripe/Paystack.
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
	"net/http"
	"os"
	"strings"
	"time"
)

// ProviderNameRazorpay is the stable Name() this provider registers under.
const ProviderNameRazorpay = "razorpay"

const (
	EnvRazorpayKeyID         = "CACKLE_RAZORPAY_KEY_ID"
	EnvRazorpayKeySecret     = "CACKLE_RAZORPAY_KEY_SECRET"
	EnvRazorpayWebhookSecret = "CACKLE_RAZORPAY_WEBHOOK_SECRET"
	razorpayAPIBase          = "https://api.razorpay.com/v1"
	razorpayHTTPTimeout      = 15 * time.Second
	razorpayMaxResponseSize  = 1 << 20
)

var (
	ErrRazorpayCredentialsNotConfigured   = errors.New("payments: razorpay: " + EnvRazorpayKeyID + " and " + EnvRazorpayKeySecret + " must both be set")
	ErrRazorpayWebhookSecretNotConfigured = errors.New("payments: razorpay: " + EnvRazorpayWebhookSecret + " not set")
	ErrRazorpayMissingSignature           = errors.New("payments: razorpay: missing X-Razorpay-Signature header")
	ErrRazorpayInvalidSignature           = errors.New("payments: razorpay: invalid webhook signature")
	ErrRazorpayUnexpectedStatus           = errors.New("payments: razorpay: unexpected API response status")
	ErrRazorpayMalformedResponse          = errors.New("payments: razorpay: malformed API response")
	ErrRazorpayResponseTooLarge           = errors.New("payments: razorpay: response body exceeds size limit")
	ErrRazorpayNoCapturedPayment          = errors.New("payments: razorpay: no captured payment found for order")
)

// RazorpayProvider implements Provider against the Razorpay Orders/Payments API.
type RazorpayProvider struct {
	keyID         string
	keySecret     string
	webhookSecret string
	httpClient    *http.Client
	baseURL       string
}

// NewRazorpay constructs a RazorpayProvider from EnvRazorpayKeyID,
// EnvRazorpayKeySecret, and EnvRazorpayWebhookSecret. All three are
// required up front.
func NewRazorpay() (*RazorpayProvider, error) {
	keyID := strings.TrimSpace(os.Getenv(EnvRazorpayKeyID))
	keySecret := strings.TrimSpace(os.Getenv(EnvRazorpayKeySecret))
	if keyID == "" || keySecret == "" {
		return nil, ErrRazorpayCredentialsNotConfigured
	}
	webhookSecret := strings.TrimSpace(os.Getenv(EnvRazorpayWebhookSecret))
	if webhookSecret == "" {
		return nil, ErrRazorpayWebhookSecretNotConfigured
	}
	return &RazorpayProvider{
		keyID:         keyID,
		keySecret:     keySecret,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: razorpayHTTPTimeout},
		baseURL:       razorpayAPIBase,
	}, nil
}

// Name implements Provider.
func (p *RazorpayProvider) Name() string { return ProviderNameRazorpay }

// Capabilities implements Provider.
func (p *RazorpayProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    []string{"INR"},
		Countries:     []string{"IN"},
		Flow:          FlowInline,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: false, // untested; INR is 2-decimal, no zero/three-decimal case exercised
	}
}

// Begin creates a Razorpay Order and returns its id (as Charge.Reference)
// for the caller to drive Razorpay's Checkout.js widget client-side.
func (p *RazorpayProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: razorpay: order reference is required as receipt")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: razorpay: amount_minor must be positive")
	}
	if strings.TrimSpace(o.Currency) == "" {
		return Charge{}, errors.New("payments: razorpay: currency is required")
	}

	reqBody := map[string]any{
		"amount":   o.AmountMinor,
		"currency": strings.ToUpper(o.Currency),
		"receipt":  o.Reference,
	}
	if o.EventID != "" {
		reqBody["notes"] = map[string]string{"event_id": o.EventID}
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/orders", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyRazorpayError(status, respBody)
	}
	var parsed struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrRazorpayMalformedResponse, err)
	}
	if parsed.ID == "" {
		return Charge{}, fmt.Errorf("%w: empty order id", ErrRazorpayMalformedResponse)
	}
	return Charge{
		Provider:  ProviderNameRazorpay,
		Reference: parsed.ID,
		Instructions: "Open Razorpay Checkout.js client-side with key_id=" + p.keyID +
			" and order_id=" + parsed.ID + " to collect payment inline.",
	}, nil
}

type razorpayPayment struct {
	ID        string `json:"id"`
	OrderID   string `json:"order_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

func razorpayPaymentToResult(pay razorpayPayment, raw []byte) (Result, error) {
	if pay.OrderID == "" {
		return Result{}, fmt.Errorf("%w: missing order_id", ErrRazorpayMalformedResponse)
	}
	result := Result{
		Provider:    ProviderNameRazorpay,
		Reference:   pay.OrderID,
		EventID:     pay.ID,
		AmountMinor: pay.Amount,
		Currency:    strings.ToUpper(pay.Currency),
		Raw:         json.RawMessage(raw),
	}
	switch pay.Status {
	case "captured":
		if pay.Amount <= 0 || pay.ID == "" {
			return Result{}, fmt.Errorf("%w: captured with non-positive amount or no payment id", ErrRazorpayMalformedResponse)
		}
		result.Status = StatusPaid
		if pay.CreatedAt > 0 {
			result.PaidAt = time.Unix(pay.CreatedAt, 0).UTC()
		}
	case "failed":
		result.Status = StatusFailed
	default:
		// "created", "authorized" (not yet captured), and anything
		// unrecognised fail closed as not-paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// Verify lists payments for the Razorpay order id (Cackle's reference)
// and returns the captured one, if any. Multiple captured payments for a
// single order should not normally happen; the first captured payment
// found is used.
func (p *RazorpayProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: razorpay: reference is required")
	}
	respBody, status, err := p.do(ctx, http.MethodGet, "/orders/"+reference+"/payments", nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyRazorpayError(status, respBody)
	}
	var parsed struct {
		Items []razorpayPayment `json:"items"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrRazorpayMalformedResponse, err)
	}
	for _, item := range parsed.Items {
		if item.OrderID != reference {
			continue // never trust an entry that disagrees on order_id
		}
		if item.Status == "captured" {
			return razorpayPaymentToResult(item, respBody)
		}
	}
	return Result{}, ErrRazorpayNoCapturedPayment
}

// Webhook validates Razorpay's X-Razorpay-Signature header (HMAC-SHA256
// hex over the raw body, using the webhook secret) and returns the
// settled result for a payment.captured event.
func (p *RazorpayProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	sigHeader := strings.TrimSpace(r.Header.Get("X-Razorpay-Signature"))
	if sigHeader == "" {
		return Result{}, ErrRazorpayMissingSignature
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrRazorpayMalformedResponse)
	}
	body, err := boundedRead(r.Body, razorpayMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrRazorpayResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: razorpay: read webhook body: %w", err)
	}
	given, err := hex.DecodeString(sigHeader)
	if err != nil {
		return Result{}, fmt.Errorf("%w: signature header is not valid hex", ErrRazorpayInvalidSignature)
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, given) {
		return Result{}, ErrRazorpayInvalidSignature
	}

	var envelope struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity razorpayPayment `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrRazorpayMalformedResponse, err)
	}
	if envelope.Event != "payment.captured" {
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Event)
	}
	pay := envelope.Payload.Payment.Entity
	if pay.Status == "" {
		pay.Status = "captured" // the event name already asserts this; tolerate a missing nested status
	}
	return razorpayPaymentToResult(pay, body)
}

func (p *RazorpayProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, razorpayHTTPTimeout)
	defer cancel()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: razorpay: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: razorpay: build request: %w", err)
	}
	req.SetBasicAuth(p.keyID, p.keySecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: razorpay: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, razorpayMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrRazorpayResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: razorpay: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyRazorpayError(status int, body []byte) error {
	var env struct {
		Error struct {
			Description string `json:"description"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.Error.Description
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrRazorpayUnexpectedStatus, status, msg)
}
