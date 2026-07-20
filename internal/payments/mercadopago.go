// Package payments: Mercado Pago adapter (Latin America).
//
// Reference: https://www.mercadopago.com/developers/en/reference/preferences/_checkout_preferences/post
// (Checkout Pro — create a Preference, unit_price as a JSON number in
// MAJOR units, init_point as the hosted redirect), and
// https://www.mercadopago.com/developers/en/docs/checkout-api/additional-content/security/signature
// (webhook signature verification: header x-signature carries "ts=...,
// v1=..."; the signed manifest is the literal string
// "id:{data.id};request-id:{x-request-id};ts:{ts};" — note the trailing
// semicolon after each field, including the last — HMAC-SHA256'd with the
// webhook secret and compared to v1 in hex).
//
// Confidence: MEDIUM-HIGH on the webhook signature manifest template
// (specific enough, and distinctive enough — three semicolon-terminated
// fields in that exact order — that this is implemented with real
// confidence from Mercado Pago's own documentation), MEDIUM on the
// Preferences API request/response shape. This has not been run against a
// real Mercado Pago test account.
//
// Mercado Pago's webhook notification body does NOT carry the settled
// amount — only {action, data:{id}, type}. This file's Webhook,
// consistent with the rest of this adapter's honest security model, MUST
// therefore make an authenticated server-to-server call (GET
// /v1/payments/{id}) to fetch the actual amount/currency/status before
// returning any Result — never trusts the push body for anything beyond
// "go look this payment id up".
//
// Amounts (unit_price on preferences, transaction_amount on payments) are
// JSON numbers in MAJOR units (e.g. 100.50 meaning $100.50), converted
// via currency.go's minorToMajorString/majorStringToMinor.
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
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderNameMercadoPago is the stable Name() this provider registers under.
const ProviderNameMercadoPago = "mercadopago"

const (
	EnvMercadoPagoAccessToken   = "CACKLE_MERCADOPAGO_ACCESS_TOKEN"
	EnvMercadoPagoWebhookSecret = "CACKLE_MERCADOPAGO_WEBHOOK_SECRET"
	mercadoPagoAPIBase          = "https://api.mercadopago.com"
	mercadoPagoHTTPTimeout      = 15 * time.Second
	mercadoPagoMaxResponseSize  = 1 << 20
)

var mercadoPagoCurrencies = []string{"ARS", "BRL", "CLP", "COP", "MXN", "PEN", "UYU"}
var mercadoPagoCountries = []string{"AR", "BR", "CL", "CO", "MX", "PE", "UY"}

var (
	ErrMercadoPagoTokenNotConfigured  = errors.New("payments: mercadopago: " + EnvMercadoPagoAccessToken + " not set")
	ErrMercadoPagoWebhookSecretNotSet = errors.New("payments: mercadopago: " + EnvMercadoPagoWebhookSecret + " not set")
	ErrMercadoPagoMissingSignature    = errors.New("payments: mercadopago: missing x-signature/x-request-id headers")
	ErrMercadoPagoInvalidSignature    = errors.New("payments: mercadopago: invalid webhook signature")
	ErrMercadoPagoUnexpectedStatus    = errors.New("payments: mercadopago: unexpected API response status")
	ErrMercadoPagoMalformedResponse   = errors.New("payments: mercadopago: malformed API response")
	ErrMercadoPagoResponseTooLarge    = errors.New("payments: mercadopago: response body exceeds size limit")
	ErrMercadoPagoPaymentNotFound     = errors.New("payments: mercadopago: no payment found for external_reference")
)

// MercadoPagoProvider implements Provider against the Mercado Pago
// Preferences (Checkout Pro) and Payments APIs.
type MercadoPagoProvider struct {
	accessToken   string
	webhookSecret string
	httpClient    *http.Client
	baseURL       string
}

// NewMercadoPago constructs a MercadoPagoProvider from
// EnvMercadoPagoAccessToken and EnvMercadoPagoWebhookSecret.
func NewMercadoPago() (*MercadoPagoProvider, error) {
	token := strings.TrimSpace(os.Getenv(EnvMercadoPagoAccessToken))
	if token == "" {
		return nil, ErrMercadoPagoTokenNotConfigured
	}
	secret := strings.TrimSpace(os.Getenv(EnvMercadoPagoWebhookSecret))
	if secret == "" {
		return nil, ErrMercadoPagoWebhookSecretNotSet
	}
	return &MercadoPagoProvider{
		accessToken:   token,
		webhookSecret: secret,
		httpClient:    &http.Client{Timeout: mercadoPagoHTTPTimeout},
		baseURL:       mercadoPagoAPIBase,
	}, nil
}

// Name implements Provider.
func (p *MercadoPagoProvider) Name() string { return ProviderNameMercadoPago }

// Capabilities implements Provider.
func (p *MercadoPagoProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    mercadoPagoCurrencies,
		Countries:     mercadoPagoCountries,
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: false, // CLP is 0-decimal per ISO 4217 but not independently verified here
	}
}

// Begin creates a Mercado Pago Checkout Pro Preference and returns its
// hosted init_point.
func (p *MercadoPagoProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: mercadopago: order reference is required as external_reference")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: mercadopago: amount_minor must be positive")
	}
	if strings.TrimSpace(o.Currency) == "" {
		return Charge{}, errors.New("payments: mercadopago: currency is required")
	}
	majorAmount, err := minorToMajorString(o.AmountMinor, o.Currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: mercadopago: %w", err)
	}
	unitPrice, err := strconv.ParseFloat(majorAmount, 64)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: mercadopago: unparseable amount %q: %w", majorAmount, err)
	}

	reqBody := map[string]any{
		"external_reference": o.Reference,
		"items": []map[string]any{
			{
				"title":       "Order " + o.Reference,
				"quantity":    1,
				"unit_price":  unitPrice,
				"currency_id": strings.ToUpper(o.Currency),
			},
		},
	}
	if o.BuyerEmail != "" {
		reqBody["payer"] = map[string]string{"email": o.BuyerEmail}
	}
	if o.CallbackURL != "" {
		reqBody["back_urls"] = map[string]string{
			"success": o.CallbackURL,
			"failure": o.CallbackURL,
			"pending": o.CallbackURL,
		}
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/checkout/preferences", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyMercadoPagoError(status, respBody)
	}
	var parsed struct {
		ID        string `json:"id"`
		InitPoint string `json:"init_point"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrMercadoPagoMalformedResponse, err)
	}
	if parsed.InitPoint == "" {
		return Charge{}, fmt.Errorf("%w: empty init_point", ErrMercadoPagoMalformedResponse)
	}
	return Charge{
		Provider:    ProviderNameMercadoPago,
		Reference:   o.Reference,
		RedirectURL: parsed.InitPoint,
	}, nil
}

type mercadoPagoPayment struct {
	ID                int64   `json:"id"`
	Status            string  `json:"status"`
	TransactionAmount float64 `json:"transaction_amount"`
	CurrencyID        string  `json:"currency_id"`
	ExternalReference string  `json:"external_reference"`
	DateApproved      string  `json:"date_approved"`
}

func (pay mercadoPagoPayment) toResult(raw []byte) (Result, error) {
	if pay.ExternalReference == "" {
		return Result{}, fmt.Errorf("%w: missing external_reference", ErrMercadoPagoMalformedResponse)
	}
	amountMinor, err := majorStringToMinor(strconv.FormatFloat(pay.TransactionAmount, 'f', -1, 64), pay.CurrencyID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: transaction_amount: %v", ErrMercadoPagoMalformedResponse, err)
	}
	result := Result{
		Provider:    ProviderNameMercadoPago,
		Reference:   pay.ExternalReference,
		EventID:     strconv.FormatInt(pay.ID, 10),
		AmountMinor: amountMinor,
		Currency:    strings.ToUpper(pay.CurrencyID),
		Raw:         json.RawMessage(raw),
	}
	switch pay.Status {
	case "approved":
		if amountMinor <= 0 || pay.ID == 0 {
			return Result{}, fmt.Errorf("%w: approved with non-positive amount or no id", ErrMercadoPagoMalformedResponse)
		}
		result.Status = StatusPaid
		if pay.DateApproved != "" {
			if t, err := time.Parse(time.RFC3339, pay.DateApproved); err == nil {
				result.PaidAt = t
			}
		}
	case "rejected", "cancelled", "refunded", "charged_back":
		result.Status = StatusFailed
	default:
		// "pending", "in_process", "in_mediation", and anything
		// unrecognised fail closed as not-paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// Verify searches for a payment by external_reference (Cackle's order
// reference).
func (p *MercadoPagoProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: mercadopago: reference is required")
	}
	respBody, status, err := p.do(ctx, http.MethodGet, "/v1/payments/search?external_reference="+url.QueryEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyMercadoPagoError(status, respBody)
	}
	var parsed struct {
		Results []mercadoPagoPayment `json:"results"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMercadoPagoMalformedResponse, err)
	}
	if len(parsed.Results) == 0 {
		return Result{}, ErrMercadoPagoPaymentNotFound
	}
	for _, pay := range parsed.Results {
		if pay.ExternalReference != reference {
			continue
		}
		if pay.Status == "approved" {
			return pay.toResult(respBody)
		}
	}
	// No approved payment; report the most recent one's (non-paid) state
	// rather than erroring outright, matching the pattern of other
	// adapters ("not yet paid" is not itself an error).
	return parsed.Results[0].toResult(respBody)
}

// Webhook validates Mercado Pago's x-signature header against the manifest
// "id:{data.id};request-id:{x-request-id};ts:{ts};", then fetches the
// authoritative payment record server-to-server (GET /v1/payments/{id})
// before returning any Result — the push body alone never carries a
// trustworthy settled amount.
func (p *MercadoPagoProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	xSignature := strings.TrimSpace(r.Header.Get("x-signature"))
	xRequestID := strings.TrimSpace(r.Header.Get("x-request-id"))
	if xSignature == "" || xRequestID == "" {
		return Result{}, ErrMercadoPagoMissingSignature
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrMercadoPagoMalformedResponse)
	}
	body, err := boundedRead(r.Body, mercadoPagoMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrMercadoPagoResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: mercadopago: read webhook body: %w", err)
	}
	var envelope struct {
		Action string `json:"action"`
		Type   string `json:"type"`
		Data   struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMercadoPagoMalformedResponse, err)
	}
	if envelope.Data.ID == "" {
		return Result{}, fmt.Errorf("%w: missing data.id", ErrMercadoPagoMalformedResponse)
	}

	ts, sigV1, err := parseMercadoPagoSignatureHeader(xSignature)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMercadoPagoInvalidSignature, err)
	}
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", envelope.Data.ID, xRequestID, ts)
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write([]byte(manifest))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sigV1)) {
		return Result{}, ErrMercadoPagoInvalidSignature
	}

	if envelope.Type != "payment" {
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Type)
	}

	// Signature verified; now fetch the authoritative record. This
	// outbound call uses ctx (with its own timeout layered on top by
	// p.do), so it still respects the caller's cancellation.
	respBody, status, err := p.do(ctx, http.MethodGet, "/v1/payments/"+url.PathEscape(envelope.Data.ID), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyMercadoPagoError(status, respBody)
	}
	var pay mercadoPagoPayment
	if err := json.Unmarshal(respBody, &pay); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMercadoPagoMalformedResponse, err)
	}
	if strconv.FormatInt(pay.ID, 10) != envelope.Data.ID {
		return Result{}, fmt.Errorf("%w: fetched payment id %d does not match webhook data.id %q", ErrMercadoPagoMalformedResponse, pay.ID, envelope.Data.ID)
	}
	return pay.toResult(respBody)
}

// parseMercadoPagoSignatureHeader splits Mercado Pago's "x-signature"
// header, e.g. "ts=1234567890,v1=abcdef...", into its ts and v1 parts.
func parseMercadoPagoSignatureHeader(header string) (ts, v1 string, err error) {
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "ts":
			ts = strings.TrimSpace(v)
		case "v1":
			v1 = strings.TrimSpace(v)
		}
	}
	if ts == "" || v1 == "" {
		return "", "", errors.New("x-signature header missing ts or v1")
	}
	return ts, v1, nil
}

func (p *MercadoPagoProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, mercadoPagoHTTPTimeout)
	defer cancel()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: mercadopago: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: mercadopago: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: mercadopago: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, mercadoPagoMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrMercadoPagoResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: mercadopago: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyMercadoPagoError(status int, body []byte) error {
	var env struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrMercadoPagoUnexpectedStatus, status, msg)
}
