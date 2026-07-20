// Package payments: Midtrans adapter (Indonesia).
//
// Reference: https://docs.midtrans.com/reference/charge-transactions-1
// (Snap API — create transaction, gross_amount as a numeric-string in
// IDR), https://docs.midtrans.com/reference/get-transaction-status
// (server-to-server status check), and
// https://docs.midtrans.com/docs/https-notification-webhooks (webhook
// signature: signature_key = SHA512(order_id + status_code + gross_amount
// + ServerKey), carried INSIDE the JSON body itself, not an HTTP header).
//
// Confidence: MEDIUM-HIGH. Midtrans is IDR-only in practice (it does not
// document broad multi-currency support the way Xendit/Flutterwave do),
// so this file rejects any other currency outright rather than guessing
// at unverified subunit behaviour.
//
// IMPORTANT currency-exponent note, because this is exactly the class of
// bug PAYMENTS-CONTRACT.md warns about: ISO 4217 formally assigns IDR a
// 2-decimal exponent (Cackle's internal/money agrees — sen are the
// notional minor unit, even though nobody uses them), so Order.AmountMinor
// for an IDR order is in sen, NOT whole rupiah. Midtrans's own wire
// format, however, is documented and observed to carry gross_amount as
// the plain whole-rupiah face value (e.g. "10000.00" meaning Rp10.000,
// not Rp100). This file's job is exactly to bridge that gap: it converts
// Cackle's 2-decimal AmountMinor to/from Midtrans's major-unit decimal
// string via minorToMajorString/majorStringToMinor (currency.go) using
// IDR's real (2-decimal) exponent — it does NOT treat IDR as zero-decimal
// anywhere, because Cackle's own ledger doesn't. Getting this exponent
// wrong in either direction is a 100x bug.
//
// The signature_key construction and Snap API request/response shape are
// implemented from Midtrans's own documentation but have not been run
// against a real Midtrans sandbox account.
package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ProviderNameMidtrans is the stable Name() this provider registers under.
const ProviderNameMidtrans = "midtrans"

const (
	// EnvMidtransServerKey is Midtrans's server key: used both as the
	// HTTP Basic auth credential for outbound API calls AND as the
	// ingredient in the webhook signature_key computation.
	EnvMidtransServerKey    = "CACKLE_MIDTRANS_SERVER_KEY"
	midtransSnapAPIBase     = "https://app.midtrans.com/snap/v1"
	midtransCoreAPIBase     = "https://api.midtrans.com/v2"
	midtransHTTPTimeout     = 15 * time.Second
	midtransMaxResponseSize = 1 << 20
)

var (
	ErrMidtransServerKeyNotConfigured = errors.New("payments: midtrans: " + EnvMidtransServerKey + " not set")
	ErrMidtransUnsupportedCurrency    = errors.New("payments: midtrans: only IDR is supported")
	ErrMidtransMissingSignature       = errors.New("payments: midtrans: webhook body has no signature_key")
	ErrMidtransInvalidSignature       = errors.New("payments: midtrans: invalid signature_key")
	ErrMidtransUnexpectedStatus       = errors.New("payments: midtrans: unexpected API response status")
	ErrMidtransMalformedResponse      = errors.New("payments: midtrans: malformed API response")
	ErrMidtransResponseTooLarge       = errors.New("payments: midtrans: response body exceeds size limit")
)

// MidtransProvider implements Provider against the Midtrans Snap + Core APIs.
type MidtransProvider struct {
	serverKey   string
	snapClient  *http.Client
	coreClient  *http.Client
	snapBaseURL string
	coreBaseURL string
}

// NewMidtrans constructs a MidtransProvider from EnvMidtransServerKey.
func NewMidtrans() (*MidtransProvider, error) {
	key := strings.TrimSpace(os.Getenv(EnvMidtransServerKey))
	if key == "" {
		return nil, ErrMidtransServerKeyNotConfigured
	}
	client := &http.Client{Timeout: midtransHTTPTimeout}
	return &MidtransProvider{
		serverKey:   key,
		snapClient:  client,
		coreClient:  client,
		snapBaseURL: midtransSnapAPIBase,
		coreBaseURL: midtransCoreAPIBase,
	}, nil
}

// Name implements Provider.
func (p *MidtransProvider) Name() string { return ProviderNameMidtrans }

// Capabilities implements Provider.
func (p *MidtransProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    []string{"IDR"},
		Countries:     []string{"ID"},
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		// false: IDR is formally 2-decimal in Cackle's own model (see the
		// file header) and this adapter has no zero- or three-decimal
		// currency to test the exponent-aware conversion against anyway
		// (Midtrans is IDR-only here).
		ZeroDecimalOK: false,
	}
}

// Begin creates a Midtrans Snap transaction and returns its hosted
// checkout redirect_url.
func (p *MidtransProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: midtrans: order reference is required as order_id")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: midtrans: amount_minor must be positive")
	}
	if !strings.EqualFold(o.Currency, "IDR") {
		return Charge{}, fmt.Errorf("%w: got %q", ErrMidtransUnsupportedCurrency, o.Currency)
	}
	grossAmount, err := minorToMajorString(o.AmountMinor, "IDR")
	if err != nil {
		return Charge{}, fmt.Errorf("payments: midtrans: %w", err)
	}

	reqBody := map[string]any{
		"transaction_details": map[string]any{
			"order_id":     o.Reference,
			"gross_amount": mustParseJSONInt(grossAmount),
		},
	}
	if o.BuyerEmail != "" {
		reqBody["customer_details"] = map[string]string{"email": o.BuyerEmail, "first_name": o.BuyerName}
	}

	respBody, status, err := p.do(ctx, p.snapClient, p.snapBaseURL, http.MethodPost, "/transactions", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyMidtransError(status, respBody)
	}
	var parsed struct {
		Token       string `json:"token"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrMidtransMalformedResponse, err)
	}
	if parsed.RedirectURL == "" {
		return Charge{}, fmt.Errorf("%w: empty redirect_url", ErrMidtransMalformedResponse)
	}
	return Charge{
		Provider:    ProviderNameMidtrans,
		Reference:   o.Reference,
		RedirectURL: parsed.RedirectURL,
	}, nil
}

// mustParseJSONInt wraps a decimal-string amount (as produced by
// minorToMajorString) as a JSON number for the Snap API request body,
// rather than a quoted string, matching Midtrans's documented examples
// (e.g. "gross_amount": 10000). In the ordinary case where the order
// total is a whole number of rupiah (no sen), that string has no
// fractional part; a fractional value (uncommon but not rejected) is
// still passed through as a valid JSON number.
func mustParseJSONInt(s string) json.Number {
	return json.Number(s)
}

type midtransTransactionStatus struct {
	OrderID           string `json:"order_id"`
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	GrossAmount       string `json:"gross_amount"`
	Currency          string `json:"currency"`
	StatusCode        string `json:"status_code"`
	SignatureKey      string `json:"signature_key"`
	SettlementTime    string `json:"settlement_time"`
	TransactionTime   string `json:"transaction_time"`
}

func (s midtransTransactionStatus) currency() string {
	if s.Currency != "" {
		return s.Currency
	}
	return "IDR"
}

func (s midtransTransactionStatus) toResult(raw []byte) (Result, error) {
	if s.OrderID == "" {
		return Result{}, fmt.Errorf("%w: missing order_id", ErrMidtransMalformedResponse)
	}
	amountMinor, err := majorStringToMinor(s.GrossAmount, s.currency())
	if err != nil {
		return Result{}, fmt.Errorf("%w: gross_amount %q: %v", ErrMidtransMalformedResponse, s.GrossAmount, err)
	}
	result := Result{
		Provider:    ProviderNameMidtrans,
		Reference:   s.OrderID,
		EventID:     s.TransactionID,
		AmountMinor: amountMinor,
		Currency:    s.currency(),
		Raw:         json.RawMessage(raw),
	}
	switch s.TransactionStatus {
	case "capture":
		// A card "capture" is only a genuine settlement when
		// fraud_status is "accept" — Midtrans's own documented
		// distinction. "challenge" or anything else must not be paid.
		if s.FraudStatus != "accept" {
			result.Status = StatusFailed
			return result, nil
		}
		if amountMinor <= 0 || s.TransactionID == "" {
			return Result{}, fmt.Errorf("%w: capture/accept with non-positive amount or no transaction_id", ErrMidtransMalformedResponse)
		}
		result.Status = StatusPaid
		result.PaidAt = parseMidtransTime(s.SettlementTime, s.TransactionTime)
	case "settlement":
		if amountMinor <= 0 || s.TransactionID == "" {
			return Result{}, fmt.Errorf("%w: settlement with non-positive amount or no transaction_id", ErrMidtransMalformedResponse)
		}
		result.Status = StatusPaid
		result.PaidAt = parseMidtransTime(s.SettlementTime, s.TransactionTime)
	case "deny", "cancel", "expire", "failure":
		result.Status = StatusFailed
	default:
		// pending, and anything unrecognised, fail closed as not-paid.
		result.Status = StatusFailed
	}
	return result, nil
}

func parseMidtransTime(candidates ...string) time.Time {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		// Midtrans timestamps look like "2026-07-20 10:00:00" in the
		// Asia/Jakarta timezone, without an explicit offset in most
		// examples in their docs.
		if t, err := time.Parse("2006-01-02 15:04:05", c); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Verify calls Midtrans's server-to-server transaction status endpoint.
func (p *MidtransProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: midtrans: reference is required")
	}
	respBody, status, err := p.do(ctx, p.coreClient, p.coreBaseURL, http.MethodGet, "/"+reference+"/status", nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyMidtransError(status, respBody)
	}
	var parsed midtransTransactionStatus
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMidtransMalformedResponse, err)
	}
	if parsed.OrderID != "" && parsed.OrderID != reference {
		return Result{}, fmt.Errorf("%w: returned order_id %q for requested %q", ErrMidtransMalformedResponse, parsed.OrderID, reference)
	}
	return parsed.toResult(respBody)
}

// Webhook validates the signature_key embedded in Midtrans's notification
// body (SHA512(order_id + status_code + gross_amount + ServerKey)) and
// returns the settled result. Fails closed on any missing field, mismatch,
// or unparseable body.
func (p *MidtransProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrMidtransMalformedResponse)
	}
	body, err := boundedRead(r.Body, midtransMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrMidtransResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: midtrans: read webhook body: %w", err)
	}
	var parsed midtransTransactionStatus
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMidtransMalformedResponse, err)
	}
	if parsed.SignatureKey == "" {
		return Result{}, ErrMidtransMissingSignature
	}
	if parsed.OrderID == "" || parsed.StatusCode == "" || parsed.GrossAmount == "" {
		return Result{}, fmt.Errorf("%w: missing order_id/status_code/gross_amount", ErrMidtransMalformedResponse)
	}
	sum := sha512.Sum512([]byte(parsed.OrderID + parsed.StatusCode + parsed.GrossAmount + p.serverKey))
	expected := hex.EncodeToString(sum[:])
	// signature_key is plain hex text, not a MAC in the cryptographic
	// sense, but comparing it in constant time is still the right
	// primitive to avoid a timing side-channel on the comparison itself.
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(parsed.SignatureKey))) {
		return Result{}, ErrMidtransInvalidSignature
	}
	return parsed.toResult(body)
}

func (p *MidtransProvider) do(ctx context.Context, client *http.Client, baseURL, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, midtransHTTPTimeout)
	defer cancel()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: midtrans: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: midtrans: build request: %w", err)
	}
	req.SetBasicAuth(p.serverKey, "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: midtrans: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, midtransMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrMidtransResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: midtrans: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyMidtransError(status int, body []byte) error {
	var env struct {
		StatusMessage string `json:"status_message"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.StatusMessage
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrMidtransUnexpectedStatus, status, msg)
}
