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
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderNamePaystack is the stable Name() this provider registers under.
const ProviderNamePaystack = "paystack"

// EnvPaystackSecretKey is the ONLY place the Paystack secret key may come
// from: an environment variable. There is no default, it is never
// committed, and it must never appear in a log line or error message.
const EnvPaystackSecretKey = "CACKLE_PAYSTACK_SECRET_KEY"

const (
	paystackAPIBase = "https://api.paystack.co"

	// defaultHTTPTimeout bounds every outbound call to Paystack. It is
	// applied even if the caller's context has no deadline of its own —
	// an outbound payment provider call must never be allowed to hang a
	// request indefinitely.
	defaultHTTPTimeout = 15 * time.Second

	// maxResponseBodyBytes caps how much of any HTTP body (Paystack API
	// responses, and incoming webhook request bodies) this package will
	// read. Paystack payloads are small JSON documents; anything larger
	// is refused rather than read into memory unbounded.
	maxResponseBodyBytes = 1 << 20 // 1 MiB

	// paystackAmountSubunitFactor is the number of subunits per major
	// currency unit that Paystack expects in its `amount` field, for
	// every currency Cackle currently supports (ZAR). Since Cackle's own
	// internal representation is ALSO integer cents (subunits), this
	// factor is 1:1 — no multiplication needed anywhere in this file.
	// This constant exists purely as documentation of that assumption; if
	// Cackle ever adds a currency where Paystack's subunit factor differs
	// from "cents == subunits" (none exist today), this is the file to
	// revisit.
	paystackAmountSubunitFactor = 1

	// defaultRecipientType is the Paystack transfer-recipient "type" for
	// South African bank accounts (as opposed to "nuban" for Nigeria,
	// "mobile_money" for Ghana, etc). Cackle targets the South African
	// market first.
	defaultRecipientType = "basa"
)

// Sentinel errors specific to the Paystack provider. Callers should match
// with errors.Is; error strings never contain the secret key.
var (
	ErrPaystackSecretNotConfigured = errors.New("payments: paystack: " + EnvPaystackSecretKey + " not set")
	ErrMissingSignature            = errors.New("payments: paystack: missing webhook signature")
	ErrInvalidSignature            = errors.New("payments: paystack: invalid webhook signature")
	ErrUnexpectedStatus            = errors.New("payments: paystack: unexpected API response status")
	ErrMalformedResponse           = errors.New("payments: paystack: malformed API response")
	ErrResponseTooLarge            = errors.New("payments: paystack: response body exceeds size limit")
)

// PaystackProvider implements Provider against the Paystack API
// (https://paystack.com), for South African merchant accounts (Paystack
// pays out directly to the organiser's own bank account — Cackle never
// touches the money).
type PaystackProvider struct {
	secretKey  string
	httpClient *http.Client
	baseURL    string // overridable in tests only (unexported, same-package tests use it directly)
}

// NewPaystack constructs a PaystackProvider using the secret key from
// EnvPaystackSecretKey. It returns ErrPaystackSecretNotConfigured if unset
// — there is no default and this package will never silently run without
// a secret.
func NewPaystack() (*PaystackProvider, error) {
	secret := strings.TrimSpace(os.Getenv(EnvPaystackSecretKey))
	if secret == "" {
		return nil, ErrPaystackSecretNotConfigured
	}
	return &PaystackProvider{
		secretKey:  secret,
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		baseURL:    paystackAPIBase,
	}, nil
}

// realPaystackSecretConfigured reports whether a real Paystack secret is
// present in the environment. Used by the stub provider and the Registry
// as a defense-in-depth guard against ever running the auto-settling stub
// somewhere a real provider is also configured.
func realPaystackSecretConfigured() bool {
	return strings.TrimSpace(os.Getenv(EnvPaystackSecretKey)) != ""
}

// Name implements Provider.
func (p *PaystackProvider) Name() string { return ProviderNamePaystack }

// Begin initializes a Paystack transaction and returns its hosted payment
// page URL.
func (p *PaystackProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.ID) == "" {
		return Charge{}, errors.New("payments: paystack: order id is required as the provider reference")
	}
	if o.TotalCents <= 0 {
		return Charge{}, errors.New("payments: paystack: total_cents must be positive")
	}
	if strings.TrimSpace(o.BuyerEmail) == "" {
		return Charge{}, errors.New("payments: paystack: buyer email is required")
	}
	currency := o.Currency
	if currency == "" {
		currency = "ZAR"
	}

	reqBody := map[string]any{
		"email":     o.BuyerEmail,
		"amount":    o.TotalCents * paystackAmountSubunitFactor,
		"reference": o.ID,
		"currency":  currency,
	}
	if o.CallbackURL != "" {
		reqBody["callback_url"] = o.CallbackURL
	}
	meta := map[string]string{}
	if o.EventID != "" {
		meta["event_id"] = o.EventID
	}
	for k, v := range o.Metadata {
		meta[k] = v
	}
	if len(meta) > 0 {
		reqBody["metadata"] = meta
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/transaction/initialize", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyPaystackError(status, respBody)
	}

	var parsed struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AuthorizationURL string `json:"authorization_url"`
			AccessCode       string `json:"access_code"`
			Reference        string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if !parsed.Status || parsed.Data.AuthorizationURL == "" {
		return Charge{}, fmt.Errorf("%w: status=false or empty authorization_url", ErrMalformedResponse)
	}

	ref := parsed.Data.Reference
	if ref == "" {
		ref = o.ID
	}
	return Charge{
		Provider:    ProviderNamePaystack,
		Reference:   ref,
		RedirectURL: parsed.Data.AuthorizationURL,
	}, nil
}

// Verify confirms a transaction reference directly against Paystack's API.
// It fails closed: any transport error, non-2xx response, malformed
// payload, or ambiguous status is returned as an error, never as a Result
// claiming payment succeeded.
func (p *PaystackProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: paystack: reference is required")
	}

	respBody, status, err := p.do(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyPaystackError(status, respBody)
	}

	var parsed struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			ID        int64  `json:"id"`
			Status    string `json:"status"`
			Reference string `json:"reference"`
			Amount    int64  `json:"amount"`
			Currency  string `json:"currency"`
			PaidAt    string `json:"paid_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if !parsed.Status {
		return Result{}, fmt.Errorf("%w: paystack returned status=false: %s", ErrUnexpectedStatus, parsed.Message)
	}
	if parsed.Data.Reference != "" && parsed.Data.Reference != reference {
		return Result{}, fmt.Errorf("%w: paystack returned reference %q for requested %q", ErrMalformedResponse, parsed.Data.Reference, reference)
	}

	result := Result{
		Provider:    ProviderNamePaystack,
		Reference:   reference,
		EventID:     strconv.FormatInt(parsed.Data.ID, 10),
		AmountCents: parsed.Data.Amount / paystackAmountSubunitFactor,
		Currency:    parsed.Data.Currency,
		Raw:         json.RawMessage(respBody),
	}

	switch parsed.Data.Status {
	case "success":
		if result.AmountCents <= 0 {
			// Defensive: never call a non-positive amount "paid".
			return Result{}, fmt.Errorf("%w: success status with non-positive amount", ErrMalformedResponse)
		}
		result.Status = StatusPaid
		if parsed.Data.PaidAt != "" {
			if t, err := time.Parse(time.RFC3339, parsed.Data.PaidAt); err == nil {
				result.PaidAt = t
			}
		}
	case "abandoned", "failed", "reversed":
		result.Status = StatusFailed
	default:
		// Fail closed: any status we don't explicitly recognise as
		// "success" is never treated as paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// Webhook validates Paystack's HMAC-SHA512 signature (header
// X-Paystack-Signature, hex-encoded, computed over the exact raw request
// body using the secret key) and returns the settled result for a
// charge.success event. It fails closed at every step: missing signature,
// invalid signature, unparseable payload, or an event this build doesn't
// treat as a settlement are all returned as errors — never as a guessed
// Result.
func (p *PaystackProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	sigHeader := strings.TrimSpace(r.Header.Get("X-Paystack-Signature"))
	if sigHeader == "" {
		return Result{}, ErrMissingSignature
	}

	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrMalformedResponse)
	}
	body, err := readLimited(r.Body, maxResponseBodyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("payments: paystack: read webhook body: %w", err)
	}

	given, err := hex.DecodeString(sigHeader)
	if err != nil {
		return Result{}, fmt.Errorf("%w: signature header is not valid hex", ErrInvalidSignature)
	}
	mac := hmac.New(sha512.New, []byte(p.secretKey))
	mac.Write(body)
	expected := mac.Sum(nil)
	// hmac.Equal is constant-time; never compare signatures with == or
	// bytes.Equal.
	if !hmac.Equal(expected, given) {
		return Result{}, ErrInvalidSignature
	}

	var envelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	if envelope.Event != "charge.success" {
		// A validly-signed webhook for an event we don't treat as a
		// settlement (e.g. transfer.success/failed). See ErrUnhandledEvent
		// doc comment for how callers should handle this.
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Event)
	}

	var data struct {
		ID        int64  `json:"id"`
		Status    string `json:"status"`
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Currency  string `json:"currency"`
		PaidAt    string `json:"paid_at"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return Result{}, fmt.Errorf("%w: charge data: %v", ErrMalformedResponse, err)
	}
	if data.Status != "success" {
		// A charge.success event whose nested data disagrees is
		// inconsistent — refuse rather than guess which field to trust.
		return Result{}, fmt.Errorf("%w: charge.success event carried data.status=%q", ErrMalformedResponse, data.Status)
	}
	if data.Reference == "" || data.Amount <= 0 {
		return Result{}, fmt.Errorf("%w: missing reference or non-positive amount", ErrMalformedResponse)
	}

	result := Result{
		Provider:    ProviderNamePaystack,
		Reference:   data.Reference,
		EventID:     strconv.FormatInt(data.ID, 10),
		Status:      StatusPaid,
		AmountCents: data.Amount / paystackAmountSubunitFactor,
		Currency:    data.Currency,
		Raw:         json.RawMessage(body),
	}
	if data.PaidAt != "" {
		if t, err := time.Parse(time.RFC3339, data.PaidAt); err == nil {
			result.PaidAt = t
		}
	}
	return result, nil
}

// Bank is a Paystack-supported bank, as returned by ListBanks.
type Bank struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Code     string `json:"code"`
	Currency string `json:"currency"`
	Active   bool   `json:"active"`
}

// ListBanks lists banks Paystack supports for transfers/payouts,
// restricted to ZAR (South African banks) since that's Cackle's market.
func (p *PaystackProvider) ListBanks(ctx context.Context) ([]Bank, error) {
	respBody, status, err := p.do(ctx, http.MethodGet, "/bank?currency=ZAR", nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, classifyPaystackError(status, respBody)
	}

	var parsed struct {
		Status bool   `json:"status"`
		Data   []Bank `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if !parsed.Status {
		return nil, fmt.Errorf("%w: paystack returned status=false", ErrUnexpectedStatus)
	}
	return parsed.Data, nil
}

// RecipientRequest describes an organiser's payout destination.
type RecipientRequest struct {
	// Name is the account holder name.
	Name string
	// AccountNumber is the bank account number.
	AccountNumber string
	// BankCode is the Paystack bank code (see ListBanks).
	BankCode string
	// Currency defaults to "ZAR" if empty.
	Currency string
	// Type is the Paystack recipient type; defaults to "basa" (South
	// African bank account) if empty.
	Type string
}

// Recipient is a Paystack transfer recipient, used to pay an organiser out
// directly — Cackle never holds the funds itself; this just registers
// where Paystack should send them.
type Recipient struct {
	RecipientCode string
	AccountName   string
	AccountNumber string
	BankCode      string
	BankName      string
}

// CreateRecipient registers an organiser's payout destination with
// Paystack, for later transfers. Cackle never holds funds: this only
// tells Paystack where the organiser's own money should go.
func (p *PaystackProvider) CreateRecipient(ctx context.Context, req RecipientRequest) (Recipient, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.AccountNumber) == "" || strings.TrimSpace(req.BankCode) == "" {
		return Recipient{}, errors.New("payments: paystack: name, account_number and bank_code are required")
	}
	currency := req.Currency
	if currency == "" {
		currency = "ZAR"
	}
	recipientType := req.Type
	if recipientType == "" {
		recipientType = defaultRecipientType
	}

	reqBody := map[string]any{
		"type":           recipientType,
		"name":           req.Name,
		"account_number": req.AccountNumber,
		"bank_code":      req.BankCode,
		"currency":       currency,
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/transferrecipient", reqBody)
	if err != nil {
		return Recipient{}, err
	}
	if status < 200 || status >= 300 {
		return Recipient{}, classifyPaystackError(status, respBody)
	}

	var parsed struct {
		Status bool `json:"status"`
		Data   struct {
			RecipientCode string `json:"recipient_code"`
			Details       struct {
				AccountNumber string `json:"account_number"`
				AccountName   string `json:"account_name"`
				BankCode      string `json:"bank_code"`
				BankName      string `json:"bank_name"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Recipient{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if !parsed.Status || parsed.Data.RecipientCode == "" {
		return Recipient{}, fmt.Errorf("%w: status=false or empty recipient_code", ErrMalformedResponse)
	}

	return Recipient{
		RecipientCode: parsed.Data.RecipientCode,
		AccountName:   parsed.Data.Details.AccountName,
		AccountNumber: parsed.Data.Details.AccountNumber,
		BankCode:      parsed.Data.Details.BankCode,
		BankName:      parsed.Data.Details.BankName,
	}, nil
}

// do issues an authenticated request against the Paystack API, bounding it
// with defaultHTTPTimeout regardless of the caller's own context, and caps
// the response body it reads. It returns the raw response body and status
// code so callers can decide how to parse/classify it; do never returns an
// error for a non-2xx status itself, so a caller inspecting resp bodies
// for provider error messages still can.
func (p *PaystackProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: paystack: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: paystack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		// net/http errors (timeout, connection refused, etc) never
		// include header values, so this is safe to wrap and surface —
		// but never log req or resp headers elsewhere just in case.
		return nil, 0, fmt.Errorf("payments: paystack: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimited(resp.Body, maxResponseBodyBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("payments: paystack: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// readLimited reads at most limit bytes from r, returning ErrResponseTooLarge
// if there was more, so no caller ever does an unbounded read of a
// provider response or webhook request body.
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrResponseTooLarge
	}
	return b, nil
}

// paystackErrorEnvelope is Paystack's error response shape:
// {"status":false,"message":"..."}.
type paystackErrorEnvelope struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}

// classifyPaystackError builds an error for a non-2xx Paystack response,
// best-effort including Paystack's own message without ever including
// request headers or the secret key.
func classifyPaystackError(status int, body []byte) error {
	var env paystackErrorEnvelope
	_ = json.Unmarshal(body, &env) // best-effort; body may not be JSON at all
	msg := env.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrUnexpectedStatus, status, msg)
}
