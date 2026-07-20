// Package payments: Xendit adapter (Indonesia / Southeast Asia).
//
// Reference: https://developers.xendit.co/api-reference/#create-invoice
// (Invoices API — create + list-by-external_id) and
// https://developers.xendit.co/docs/webhooks/ (webhook verification via a
// static per-account "callback token" echoed in the x-callback-token
// header — not an HMAC).
//
// Confidence: MEDIUM. The Invoices API shape (HTTP Basic auth with the
// secret key as username, external_id as the merchant reference,
// invoice_url as the hosted checkout link, "PAID"/"EXPIRED" statuses) is
// well documented and implemented here from that documentation, but this
// has NOT been run against a real Xendit sandbox account.
//
// Currency-exponent note (see PAYMENTS-CONTRACT.md's warning about this
// exact class of bug): ISO 4217 — and Cackle's own internal/money — treat
// IDR as a 2-decimal currency (sen, unused in practice) and VND as
// genuinely 0-decimal. Xendit's own wire format, however, is documented
// and observed to carry `amount` as the plain MAJOR-unit face value for
// every currency it supports (e.g. "amount": 10000 means Rp10.000, not
// Rp100 — Xendit does not use IDR sen at all). This file bridges that gap
// via minorToMajorString/majorStringToMinor (currency.go), which convert
// Cackle's own AmountMinor to/from major units using each currency's REAL
// exponent (2 for IDR, 0 for VND, etc) — it does not special-case IDR as
// zero-decimal anywhere. The IDR/VND conversion (Xendit's primary,
// best-documented markets) is implemented with confidence; the other
// supported 2-decimal currencies (PHP, THB, MYR) are NOT independently
// verified against a real invoice.
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
	"strings"
	"time"
)

// ProviderNameXendit is the stable Name() this provider registers under.
const ProviderNameXendit = "xendit"

const (
	EnvXenditSecretKey    = "CACKLE_XENDIT_SECRET_KEY"
	EnvXenditWebhookToken = "CACKLE_XENDIT_WEBHOOK_TOKEN"
	xenditAPIBase         = "https://api.xendit.co"
	xenditHTTPTimeout     = 15 * time.Second
	xenditMaxResponseSize = 1 << 20
)

var xenditCurrencies = []string{"IDR", "PHP", "VND", "THB", "MYR"}
var xenditCountries = []string{"ID", "PH", "VN", "TH", "MY"}

var (
	ErrXenditSecretNotConfigured = errors.New("payments: xendit: " + EnvXenditSecretKey + " not set")
	ErrXenditTokenNotConfigured  = errors.New("payments: xendit: " + EnvXenditWebhookToken + " not set")
	ErrXenditMissingSignature    = errors.New("payments: xendit: missing x-callback-token header")
	ErrXenditInvalidSignature    = errors.New("payments: xendit: invalid x-callback-token")
	ErrXenditUnexpectedStatus    = errors.New("payments: xendit: unexpected API response status")
	ErrXenditMalformedResponse   = errors.New("payments: xendit: malformed API response")
	ErrXenditResponseTooLarge    = errors.New("payments: xendit: response body exceeds size limit")
	ErrXenditInvoiceNotFound     = errors.New("payments: xendit: no invoice found for external_id")
)

// XenditProvider implements Provider against the Xendit Invoices API.
type XenditProvider struct {
	secretKey    string
	webhookToken string
	httpClient   *http.Client
	baseURL      string
}

// NewXendit constructs a XenditProvider from EnvXenditSecretKey and
// EnvXenditWebhookToken. Both are required up front — a provider that
// can't verify webhooks must not be constructible at all.
func NewXendit() (*XenditProvider, error) {
	secret := strings.TrimSpace(os.Getenv(EnvXenditSecretKey))
	if secret == "" {
		return nil, ErrXenditSecretNotConfigured
	}
	token := strings.TrimSpace(os.Getenv(EnvXenditWebhookToken))
	if token == "" {
		return nil, ErrXenditTokenNotConfigured
	}
	return &XenditProvider{
		secretKey:    secret,
		webhookToken: token,
		httpClient:   &http.Client{Timeout: xenditHTTPTimeout},
		baseURL:      xenditAPIBase,
	}, nil
}

// Name implements Provider.
func (p *XenditProvider) Name() string { return ProviderNameXendit }

// Capabilities implements Provider.
func (p *XenditProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    xenditCurrencies,
		Countries:     xenditCountries,
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: true, // IDR/VND are Xendit's primary, well-verified markets
	}
}

type xenditInvoice struct {
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	Status     string `json:"status"`
	Amount     any    `json:"amount"`
	PaidAmount any    `json:"paid_amount"`
	Currency   string `json:"currency"`
	InvoiceURL string `json:"invoice_url"`
	PaidAt     string `json:"paid_at"`
}

// xenditNumberToString converts Xendit's amount field, which may be encoded as
// either a JSON number or a JSON string depending on endpoint, into a
// decimal string suitable for majorStringToMinor.
func xenditNumberToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// Xendit amounts are whole numbers for its zero-decimal markets;
		// formatting via %v on a float64 risks scientific notation for
		// very large values, so use a fixed-point format instead.
		return xenditTrimTrailingZeros(t)
	default:
		return ""
	}
}

func xenditTrimTrailingZeros(f float64) string {
	s := fmt.Sprintf("%.6f", f)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return s
}

// Begin creates a Xendit invoice and returns its hosted checkout link.
func (p *XenditProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: xendit: order reference is required as external_id")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: xendit: amount_minor must be positive")
	}
	if strings.TrimSpace(o.Currency) == "" {
		return Charge{}, errors.New("payments: xendit: currency is required")
	}
	majorAmount, err := minorToMajorString(o.AmountMinor, o.Currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: xendit: %w", err)
	}

	reqBody := map[string]any{
		"external_id": o.Reference,
		"amount":      majorAmount,
		"currency":    strings.ToUpper(o.Currency),
	}
	if o.BuyerEmail != "" {
		reqBody["payer_email"] = o.BuyerEmail
	}
	if o.CallbackURL != "" {
		reqBody["success_redirect_url"] = o.CallbackURL
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/v2/invoices", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyXenditError(status, respBody)
	}
	var inv xenditInvoice
	if err := json.Unmarshal(respBody, &inv); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrXenditMalformedResponse, err)
	}
	if inv.InvoiceURL == "" {
		return Charge{}, fmt.Errorf("%w: empty invoice_url", ErrXenditMalformedResponse)
	}
	return Charge{
		Provider:    ProviderNameXendit,
		Reference:   o.Reference,
		RedirectURL: inv.InvoiceURL,
	}, nil
}

func xenditInvoiceToResult(inv xenditInvoice, raw []byte) (Result, error) {
	if inv.ExternalID == "" {
		return Result{}, fmt.Errorf("%w: missing external_id", ErrXenditMalformedResponse)
	}
	// Prefer paid_amount (what was actually settled) when present and the
	// invoice is paid; otherwise fall back to amount (the invoice face
	// value) for pending/expired invoices.
	amountField := inv.Amount
	if inv.Status == "PAID" && inv.PaidAmount != nil {
		amountField = inv.PaidAmount
	}
	amountStr := xenditNumberToString(amountField)
	if amountStr == "" {
		return Result{}, fmt.Errorf("%w: unparseable amount field", ErrXenditMalformedResponse)
	}
	amountMinor, err := majorStringToMinor(amountStr, inv.Currency)
	if err != nil {
		return Result{}, fmt.Errorf("%w: amount %q: %v", ErrXenditMalformedResponse, amountStr, err)
	}
	result := Result{
		Provider:    ProviderNameXendit,
		Reference:   inv.ExternalID,
		EventID:     inv.ID,
		AmountMinor: amountMinor,
		Currency:    inv.Currency,
		Raw:         json.RawMessage(raw),
	}
	switch inv.Status {
	case "PAID", "SETTLED":
		if amountMinor <= 0 {
			return Result{}, fmt.Errorf("%w: paid status with non-positive amount", ErrXenditMalformedResponse)
		}
		if inv.ID == "" {
			return Result{}, fmt.Errorf("%w: paid invoice with no id (cannot dedupe webhooks)", ErrXenditMalformedResponse)
		}
		result.Status = StatusPaid
		if inv.PaidAt != "" {
			if t, err := time.Parse(time.RFC3339, inv.PaidAt); err == nil {
				result.PaidAt = t
			}
		}
	case "EXPIRED":
		result.Status = StatusFailed
	default:
		// PENDING and anything unrecognised fail closed as not-paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// Verify looks up an invoice by external_id (Cackle's order reference).
func (p *XenditProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: xendit: reference is required")
	}
	respBody, status, err := p.do(ctx, http.MethodGet, "/v2/invoices?external_id="+url.QueryEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyXenditError(status, respBody)
	}
	var invoices []xenditInvoice
	if err := json.Unmarshal(respBody, &invoices); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrXenditMalformedResponse, err)
	}
	if len(invoices) == 0 {
		return Result{}, ErrXenditInvoiceNotFound
	}
	// Multiple invoices can share an external_id if the caller retried
	// Begin; take the most recently returned one (Xendit orders these
	// newest-first) but never trust a mismatched external_id.
	inv := invoices[0]
	if inv.ExternalID != reference {
		return Result{}, fmt.Errorf("%w: returned external_id %q for requested %q", ErrXenditMalformedResponse, inv.ExternalID, reference)
	}
	return xenditInvoiceToResult(inv, respBody)
}

// Webhook validates Xendit's static x-callback-token header (constant-time
// comparison — it's a shared secret, not a MAC) and returns the settled
// result for a PAID invoice callback.
func (p *XenditProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	given := strings.TrimSpace(r.Header.Get("x-callback-token"))
	if given == "" {
		return Result{}, ErrXenditMissingSignature
	}
	if !hmac.Equal([]byte(given), []byte(p.webhookToken)) {
		return Result{}, ErrXenditInvalidSignature
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrXenditMalformedResponse)
	}
	body, err := boundedRead(r.Body, xenditMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrXenditResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: xendit: read webhook body: %w", err)
	}
	var inv xenditInvoice
	if err := json.Unmarshal(body, &inv); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrXenditMalformedResponse, err)
	}
	if inv.Status != "PAID" && inv.Status != "SETTLED" {
		return Result{}, fmt.Errorf("%w: invoice status %q", ErrUnhandledEvent, inv.Status)
	}
	return xenditInvoiceToResult(inv, body)
}

func (p *XenditProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, xenditHTTPTimeout)
	defer cancel()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: xendit: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: xendit: build request: %w", err)
	}
	req.SetBasicAuth(p.secretKey, "")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: xendit: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, xenditMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrXenditResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: xendit: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyXenditError(status int, body []byte) error {
	var env struct {
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.Message
	if msg == "" {
		msg = env.ErrorCode
	}
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrXenditUnexpectedStatus, status, msg)
}
