// Package payments: iyzico adapter (Turkey).
//
// Reference: https://docs.iyzico.com/en/checkout-form (Checkout Form
// initialize + retrieve) and https://docs.iyzico.com/en/authentication
// (classic "IYZWS" HMAC-SHA1 request signing).
//
// Confidence: SPLIT, and this file is deliberately conservative about
// which half it asks you to trust:
//
//   - MEDIUM-HIGH on the security-critical shape: iyzico's Checkout Form
//     callback carries NO signature of its own — the buyer's browser is
//     redirected back with a bare `token`, and iyzico's documentation is
//     explicit that the integrator MUST call retrieveCheckoutForm
//     server-to-server with that token to learn the real outcome. This
//     file's Webhook does exactly that and nothing else: the token is
//     used purely as a lookup key, never trusted for status or amount.
//     Getting the OUTBOUND auth wrong (below) makes that retrieve call
//     fail closed (401/malformed), never silently "succeed" — so this
//     half's fail-closed guarantee holds regardless of the next point.
//   - LOW-MEDIUM, EXPLICITLY FLAGGED, on the exact IYZWS request-signing
//     byte sequence used for the outbound Initialize/Retrieve calls
//     (hashStr = apiKey + randomKey + secretKey + compact-JSON-body,
//     SHA1, base64, header "Authorization: IYZWS {apiKey}:{signature}"
//     plus "x-iyzi-rnd: {randomKey}"). This is iyzico's long-standing
//     "classic" v1 scheme as implemented by their official SDKs; iyzico
//     has since introduced a newer HMACSHA256-based auth ("IYZWSv2") for
//     some merchants, and this file has NOT been verified against either
//     scheme with a real sandbox account — if requests come back
//     unauthorized, check which scheme your merchant account actually
//     requires before assuming this file's logic is broken elsewhere.
//   - NOT ATTEMPTED: iyzico's mandatory buyer/address/basket-item fields
//     (identityNumber, registrationAddress, city, ip, shippingAddress,
//     billingAddress, basketItems...) are extensive and Cackle's Order
//     type does not carry most of them. Begin sends the fields it can
//     populate honestly from Order and leaves the rest out rather than
//     fabricating placeholder identity/address data — iyzico will likely
//     reject a real request until a caller extends Order (via Metadata)
//     or wires up the missing fields directly. This is a functional gap,
//     not a security one: an incomplete/rejected request never reports a
//     false "paid".
package payments

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
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

// ProviderNameIyzico is the stable Name() this provider registers under.
const ProviderNameIyzico = "iyzico"

const (
	EnvIyzicoAPIKey    = "CACKLE_IYZICO_API_KEY"
	EnvIyzicoSecretKey = "CACKLE_IYZICO_SECRET_KEY"
	// EnvIyzicoBaseURL optionally overrides the API host (e.g.
	// https://sandbox-api.iyzipay.com for testing). Empty means production.
	EnvIyzicoBaseURL      = "CACKLE_IYZICO_BASE_URL"
	iyzicoProductionBase  = "https://api.iyzipay.com"
	iyzicoHTTPTimeout     = 15 * time.Second
	iyzicoMaxResponseSize = 1 << 20
)

var (
	ErrIyzicoCredentialsNotConfigured = errors.New("payments: iyzico: " + EnvIyzicoAPIKey + " and " + EnvIyzicoSecretKey + " must both be set")
	ErrIyzicoUnexpectedStatus         = errors.New("payments: iyzico: unexpected API response status")
	ErrIyzicoMalformedResponse        = errors.New("payments: iyzico: malformed API response")
	ErrIyzicoResponseTooLarge         = errors.New("payments: iyzico: response body exceeds size limit")
	ErrIyzicoMissingToken             = errors.New("payments: iyzico: callback carried no token")
)

// IyzicoProvider implements Provider against iyzico's Checkout Form API.
type IyzicoProvider struct {
	apiKey     string
	secretKey  string
	httpClient *http.Client
	baseURL    string
}

// NewIyzico constructs an IyzicoProvider from EnvIyzicoAPIKey and
// EnvIyzicoSecretKey.
func NewIyzico() (*IyzicoProvider, error) {
	apiKey := strings.TrimSpace(os.Getenv(EnvIyzicoAPIKey))
	secretKey := strings.TrimSpace(os.Getenv(EnvIyzicoSecretKey))
	if apiKey == "" || secretKey == "" {
		return nil, ErrIyzicoCredentialsNotConfigured
	}
	base := strings.TrimSpace(os.Getenv(EnvIyzicoBaseURL))
	if base == "" {
		base = iyzicoProductionBase
	}
	return &IyzicoProvider{
		apiKey:     apiKey,
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: iyzicoHTTPTimeout},
		baseURL:    base,
	}, nil
}

// Name implements Provider.
func (p *IyzicoProvider) Name() string { return ProviderNameIyzico }

// Capabilities implements Provider.
func (p *IyzicoProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    []string{"TRY", "USD", "EUR", "GBP"},
		Countries:     []string{"TR"},
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: false, // all supported currencies here are 2-decimal; never exercised otherwise
	}
}

// iyzicoAuthHeaders computes the classic "IYZWS" v1 HMAC-SHA1 signature
// for a request body. See the file header for this scheme's confidence
// level.
func (p *IyzicoProvider) iyzicoAuthHeaders(randomKey string, body []byte) (authorization, xIyziRnd string) {
	h := sha1.New()
	h.Write([]byte(p.apiKey))
	h.Write([]byte(randomKey))
	h.Write([]byte(p.secretKey))
	h.Write(body)
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return "IYZWS " + p.apiKey + ":" + signature, randomKey
}

func newIyzicoRandomKey() string {
	// iyzico's own SDKs typically use a timestamp-based nonce here; a
	// monotonically-increasing, per-request-unique value is what matters
	// for the hash to be freshly computed each call (this is NOT a
	// replay-protection nonce on iyzico's side as far as this file's
	// documentation review found — replay protection for THIS package's
	// purposes comes from Cackle's own SeenStore, keyed on the payment
	// token, not from anything iyzico does with randomKey).
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

// Begin initializes an iyzico Checkout Form and returns its hosted
// paymentPageUrl.
func (p *IyzicoProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: iyzico: order reference is required as conversationId")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: iyzico: amount_minor must be positive")
	}
	if strings.TrimSpace(o.Currency) == "" {
		return Charge{}, errors.New("payments: iyzico: currency is required")
	}
	price, err := minorToMajorString(o.AmountMinor, o.Currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: iyzico: %w", err)
	}

	reqBody := map[string]any{
		"locale":         "en",
		"conversationId": o.Reference,
		"price":          price,
		"paidPrice":      price,
		"currency":       strings.ToUpper(o.Currency),
		"basketId":       o.Reference,
		"paymentGroup":   "PRODUCT",
		"callbackUrl":    o.CallbackURL,
		// buyer/address/basketItems are intentionally left for the
		// caller to extend (see file header) — iyzico requires more
		// identity/address detail than Order carries today.
	}
	if o.BuyerEmail != "" {
		reqBody["buyer"] = map[string]any{
			"id":    o.Reference,
			"email": o.BuyerEmail,
			"name":  o.BuyerName,
		}
	}

	respBody, status, err := p.do(ctx, "/payment/iyzipos/checkoutform/initialize/auth/ecom", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyIyzicoError(status, respBody)
	}
	var parsed struct {
		Status         string `json:"status"`
		Token          string `json:"token"`
		PaymentPageURL string `json:"paymentPageUrl"`
		ErrorMessage   string `json:"errorMessage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrIyzicoMalformedResponse, err)
	}
	if parsed.Status != "success" || parsed.Token == "" || parsed.PaymentPageURL == "" {
		msg := parsed.ErrorMessage
		if msg == "" {
			msg = "status=" + parsed.Status
		}
		return Charge{}, fmt.Errorf("%w: %s", ErrIyzicoUnexpectedStatus, msg)
	}
	return Charge{
		Provider:    ProviderNameIyzico,
		Reference:   parsed.Token,
		RedirectURL: parsed.PaymentPageURL,
	}, nil
}

type iyzicoCheckoutFormResult struct {
	Status        string `json:"status"`
	PaymentStatus string `json:"paymentStatus"`
	Token         string `json:"token"`
	PaymentID     string `json:"paymentId"`
	PaidPrice     string `json:"paidPrice"`
	Currency      string `json:"currency"`
	BasketID      string `json:"basketId"`
	ErrorMessage  string `json:"errorMessage"`
}

func (res iyzicoCheckoutFormResult) toResult(reference string, raw []byte) (Result, error) {
	if res.Status != "success" {
		// A transport-level failure from iyzico's own API (bad request,
		// invalid token, etc) — distinct from "PaymentStatus=FAILURE",
		// which is a legitimate not-paid outcome.
		msg := res.ErrorMessage
		if msg == "" {
			msg = "status=" + res.Status
		}
		return Result{}, fmt.Errorf("%w: %s", ErrIyzicoUnexpectedStatus, msg)
	}
	// The reference this package tracks orders by is the Checkout Form
	// token (Charge.Reference); basketId doubles as the caller's own
	// order id but token is authoritative here since that's what was
	// looked up.
	result := Result{
		Provider:  ProviderNameIyzico,
		Reference: reference,
		EventID:   res.PaymentID,
		Currency:  strings.ToUpper(res.Currency),
		Raw:       json.RawMessage(raw),
	}
	if res.PaidPrice != "" {
		amountMinor, err := majorStringToMinor(res.PaidPrice, res.Currency)
		if err != nil {
			return Result{}, fmt.Errorf("%w: paidPrice %q: %v", ErrIyzicoMalformedResponse, res.PaidPrice, err)
		}
		result.AmountMinor = amountMinor
	}
	switch res.PaymentStatus {
	case "SUCCESS":
		if result.AmountMinor <= 0 || res.PaymentID == "" {
			return Result{}, fmt.Errorf("%w: SUCCESS status with non-positive amount or no paymentId", ErrIyzicoMalformedResponse)
		}
		result.Status = StatusPaid
	case "FAILURE":
		result.Status = StatusFailed
	default:
		result.Status = StatusFailed
	}
	return result, nil
}

// Verify calls iyzico's retrieveCheckoutForm API with the Checkout Form
// token (Cackle's reference, as returned by Begin).
func (p *IyzicoProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: iyzico: reference is required")
	}
	reqBody := map[string]any{
		"locale":         "en",
		"conversationId": reference,
		"token":          reference,
	}
	respBody, status, err := p.do(ctx, "/payment/iyzipos/checkoutform/auth/ecom/detail", reqBody)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyIyzicoError(status, respBody)
	}
	var parsed iyzicoCheckoutFormResult
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrIyzicoMalformedResponse, err)
	}
	return parsed.toResult(reference, respBody)
}

// Webhook handles iyzico's Checkout Form callback: an UNSIGNED POST
// carrying only a token. Per the file header, the token is used purely
// as a lookup key — this always calls the authenticated
// retrieveCheckoutForm API (identical to Verify) before returning any
// Result, never trusting anything else in the callback body.
func (p *IyzicoProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrIyzicoMalformedResponse)
	}
	body, err := boundedRead(r.Body, iyzicoMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrIyzicoResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: iyzico: read callback body: %w", err)
	}
	token := extractIyzicoToken(r, body)
	if token == "" {
		return Result{}, ErrIyzicoMissingToken
	}
	return p.Verify(ctx, token)
}

// extractIyzicoToken pulls "token" from either a form-urlencoded POST
// body (iyzico's documented callback shape) or, defensively, a JSON body,
// so this works whether the caller's HTTP route parsed the content type
// for us or not.
func extractIyzicoToken(r *http.Request, body []byte) string {
	if ct := r.Header.Get("Content-Type"); strings.Contains(ct, "application/json") {
		var payload struct {
			Token string `json:"token"`
		}
		if json.Unmarshal(body, &payload) == nil && payload.Token != "" {
			return payload.Token
		}
		return ""
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}
	return values.Get("token")
}

func (p *IyzicoProvider) do(ctx context.Context, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, iyzicoHTTPTimeout)
	defer cancel()

	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: iyzico: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, 0, fmt.Errorf("payments: iyzico: build request: %w", err)
	}
	randomKey := newIyzicoRandomKey()
	authorization, xIyziRnd := p.iyzicoAuthHeaders(randomKey, b)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("x-iyzi-rnd", xIyziRnd)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: iyzico: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, iyzicoMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrIyzicoResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: iyzico: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyIyzicoError(status int, body []byte) error {
	var env struct {
		ErrorMessage string `json:"errorMessage"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.ErrorMessage
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrIyzicoUnexpectedStatus, status, msg)
}
