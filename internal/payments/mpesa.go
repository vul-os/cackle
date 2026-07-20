// Package payments: M-Pesa (Safaricom Daraja) adapter (Kenya).
//
// Reference: https://developer.safaricom.co.ke/APIs/MpesaExpressSimulate
// (Lipa na M-Pesa Online / "STK Push"), and the Daraja "M-Pesa Express
// Query" endpoint for status. OAuth: Basic-auth client-credentials grant
// against /oauth/v1/generate.
//
// Confidence: HIGH on the overall STK Push + OAuth + Query shape (this is
// Safaricom's single most consistently documented integration and the
// flow has been stable for years); this file has not been run against a
// real Daraja sandbox app.
//
// M-Pesa is NOT a card processor and does not fit the redirect/webhook
// shape the other adapters in this package share — it is modelled
// honestly rather than forced into that shape:
//
//   - Begin does not return a redirect URL. It triggers an STK push (a
//     PIN prompt on the buyer's own phone) and returns Charge.Reference =
//     the Safaricom CheckoutRequestID, with Instructions telling the
//     caller to poll Verify or wait for the callback.
//   - Daraja's STK callback (what would be "Webhook" for every other
//     adapter) carries NO cryptographic signature at all — Safaricom does
//     not sign these deliveries. Treating that push body as authoritative
//     would mean anyone who could reach the callback URL could fabricate
//     a "paid" notification. Webhook() in this file therefore does NOT
//     trust the callback's own ResultCode/Amount fields for anything
//     beyond "here is a CheckoutRequestID to go check" — it extracts that
//     id and then calls Safaricom's own authenticated Query API
//     server-to-server to get the actual settlement status before
//     returning any Result. This converts an unauthenticated push into a
//     verified pull, which is the only honest way to give this adapter a
//     real fail-closed guarantee given Daraja's actual security model.
//   - Amounts: the STK Push "Amount" field is a bare whole number of KES
//     shillings — Safaricom does not support fractional cents on this
//     API. Since KES is a 2-decimal currency in Cackle's own model (see
//     internal/money / currency.go), Begin REJECTS any order whose
//     AmountMinor is not an exact multiple of 100 (i.e. has a non-zero
//     cents remainder) rather than silently rounding — rounding either
//     direction would mean charging the buyer something other than the
//     order's actual total.
package payments

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProviderNameMpesa is the stable Name() this provider registers under.
const ProviderNameMpesa = "mpesa"

const (
	EnvMpesaConsumerKey    = "CACKLE_MPESA_CONSUMER_KEY"
	EnvMpesaConsumerSecret = "CACKLE_MPESA_CONSUMER_SECRET"
	EnvMpesaShortcode      = "CACKLE_MPESA_SHORTCODE"
	EnvMpesaPasskey        = "CACKLE_MPESA_PASSKEY"
	// EnvMpesaBaseURL optionally overrides the Daraja API base (e.g. to
	// point at the sandbox host https://sandbox.safaricom.co.ke instead
	// of the production host). Empty means production.
	EnvMpesaBaseURL = "CACKLE_MPESA_BASE_URL"

	mpesaProductionBase  = "https://api.safaricom.co.ke"
	mpesaHTTPTimeout     = 20 * time.Second // STK push round trips can be slow (buyer must respond on-phone)
	mpesaMaxResponseSize = 1 << 20

	// mpesaTokenSafetyMargin is subtracted from Safaricom's reported
	// expires_in so a cached OAuth token is never used right up to the
	// wire.
	mpesaTokenSafetyMargin = 60 * time.Second
)

var (
	ErrMpesaCredentialsNotConfigured = errors.New("payments: mpesa: " + EnvMpesaConsumerKey + ", " + EnvMpesaConsumerSecret + ", " + EnvMpesaShortcode + " and " + EnvMpesaPasskey + " must all be set")
	ErrMpesaUnsupportedCurrency      = errors.New("payments: mpesa: only KES is supported")
	ErrMpesaFractionalAmount         = errors.New("payments: mpesa: KES amount must be a whole number of shillings (M-Pesa does not support fractional cents)")
	ErrMpesaUnexpectedStatus         = errors.New("payments: mpesa: unexpected API response status")
	ErrMpesaMalformedResponse        = errors.New("payments: mpesa: malformed API response")
	ErrMpesaResponseTooLarge         = errors.New("payments: mpesa: response body exceeds size limit")
	ErrMpesaMissingCheckoutRequestID = errors.New("payments: mpesa: callback carried no CheckoutRequestID to verify")
	// ErrMpesaOrderLookupRequired is returned by NewMpesa when constructed
	// with a nil OrderLookup. This adapter cannot function without one —
	// see MpesaProvider.orderLookup's doc comment — so refusing to
	// construct at all (rather than constructing a permanently-broken
	// provider that fails closed on every single Verify/Webhook call) is
	// the earlier, clearer failure.
	ErrMpesaOrderLookupRequired = errors.New("payments: mpesa: a non-nil OrderLookup is required")
)

// MpesaProvider implements Provider against the Safaricom Daraja STK Push
// (Lipa na M-Pesa Online) API.
type MpesaProvider struct {
	consumerKey    string
	consumerSecret string
	shortcode      string
	passkey        string
	httpClient     *http.Client
	baseURL        string

	// orderLookup resolves the STORED order's amount/currency by
	// reference (== Daraja's CheckoutRequestID, since Cackle's order ID
	// IS the provider reference — see Order's doc comment). This adapter
	// needs it for a reason none of its siblings do: Daraja's STK Push
	// Query API (what Verify/Webhook call to authenticate a settlement)
	// does not echo back a settled amount at all, only a ResultCode — see
	// resultFromQuery's doc comment for why echoing the STORED amount
	// back is safe specifically here. Every call site MUST supply this at
	// construction (NewMpesa refuses a nil one, see
	// ErrMpesaOrderLookupRequired) — there is no working fallback.
	orderLookup OrderLookup

	tokenMu      sync.Mutex
	cachedToken  string
	tokenExpires time.Time
}

// NewMpesa constructs an MpesaProvider from EnvMpesaConsumerKey,
// EnvMpesaConsumerSecret, EnvMpesaShortcode, and EnvMpesaPasskey.
// EnvMpesaBaseURL is optional (defaults to the production API host).
// lookup MUST be non-nil (ErrMpesaOrderLookupRequired otherwise) — see
// MpesaProvider.orderLookup's doc comment for why this adapter, uniquely
// among this package's providers, cannot Verify/Webhook without one.
func NewMpesa(lookup OrderLookup) (*MpesaProvider, error) {
	key := strings.TrimSpace(os.Getenv(EnvMpesaConsumerKey))
	secret := strings.TrimSpace(os.Getenv(EnvMpesaConsumerSecret))
	shortcode := strings.TrimSpace(os.Getenv(EnvMpesaShortcode))
	passkey := strings.TrimSpace(os.Getenv(EnvMpesaPasskey))
	if key == "" || secret == "" || shortcode == "" || passkey == "" {
		return nil, ErrMpesaCredentialsNotConfigured
	}
	if lookup == nil {
		return nil, ErrMpesaOrderLookupRequired
	}
	base := strings.TrimSpace(os.Getenv(EnvMpesaBaseURL))
	if base == "" {
		base = mpesaProductionBase
	}
	return &MpesaProvider{
		consumerKey:    key,
		consumerSecret: secret,
		shortcode:      shortcode,
		passkey:        passkey,
		httpClient:     &http.Client{Timeout: mpesaHTTPTimeout},
		baseURL:        base,
		orderLookup:    lookup,
	}, nil
}

// Name implements Provider.
func (p *MpesaProvider) Name() string { return ProviderNameMpesa }

// Capabilities implements Provider.
func (p *MpesaProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies: []string{"KES"},
		Countries:  []string{"KE"},
		Flow:       FlowInvoice, // STK push: buyer confirms on their own phone, no redirect/widget
		Refunds:    false,
		Payouts:    false,
		// Webhooks is true in the sense that Webhook() exists and is
		// wired up, but see the file header: it never trusts the
		// callback body directly, always re-verifying via the
		// authenticated Query API before returning a Result.
		Webhooks:      true,
		ZeroDecimalOK: false, // KES is 2-decimal; M-Pesa's own API just doesn't support the fractional part
	}
}

// mpesaTimestamp is Daraja's required "yyyyMMddHHmmss" format, always in
// the API's expected (Nairobi-local, per Safaricom's own examples, but
// Daraja accepts UTC in practice for the password hash to validate) form.
// This uses UTC for determinism; Safaricom's password check only cares
// that Timestamp matches what's embedded in Password.
func mpesaTimestamp(t time.Time) string {
	return t.UTC().Format("20060102150405")
}

func (p *MpesaProvider) password(timestamp string) string {
	raw := p.shortcode + p.passkey + timestamp
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// accessToken returns a cached OAuth token, refreshing it if absent or
// near expiry.
func (p *MpesaProvider) accessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()
	if p.cachedToken != "" && time.Now().Before(p.tokenExpires) {
		return p.cachedToken, nil
	}

	ctx, cancel := context.WithTimeout(ctx, mpesaHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/oauth/v1/generate?grant_type=client_credentials", nil)
	if err != nil {
		return "", fmt.Errorf("payments: mpesa: build oauth request: %w", err)
	}
	req.SetBasicAuth(p.consumerKey, p.consumerSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("payments: mpesa: oauth request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := boundedRead(resp.Body, mpesaMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return "", ErrMpesaResponseTooLarge
		}
		return "", fmt.Errorf("payments: mpesa: read oauth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: oauth http %d", ErrMpesaUnexpectedStatus, resp.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("%w: oauth response: %v", ErrMpesaMalformedResponse, err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("%w: empty access_token", ErrMpesaMalformedResponse)
	}
	expiresSeconds, _ := strconv.Atoi(parsed.ExpiresIn)
	if expiresSeconds <= 0 {
		expiresSeconds = 3600
	}
	p.cachedToken = parsed.AccessToken
	p.tokenExpires = time.Now().Add(time.Duration(expiresSeconds)*time.Second - mpesaTokenSafetyMargin)
	return p.cachedToken, nil
}

// Begin triggers an STK push (a PIN prompt on the buyer's phone) via
// Daraja's processrequest endpoint. Order.BuyerName is unused; the
// M-Pesa-registered phone number is expected in Order.Metadata["phone"]
// (E.164-ish MSISDN, e.g. "2547XXXXXXXX") since Order has no dedicated
// phone field — this is documented here rather than silently defaulting
// to something wrong.
func (p *MpesaProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: mpesa: order reference is required as AccountReference")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: mpesa: amount_minor must be positive")
	}
	if !strings.EqualFold(o.Currency, "KES") {
		return Charge{}, fmt.Errorf("%w: got %q", ErrMpesaUnsupportedCurrency, o.Currency)
	}
	if o.AmountMinor%100 != 0 {
		return Charge{}, ErrMpesaFractionalAmount
	}
	phone := strings.TrimSpace(o.Metadata["phone"])
	if phone == "" {
		return Charge{}, errors.New("payments: mpesa: buyer phone number is required in Order.Metadata[\"phone\"]")
	}
	whole := o.AmountMinor / 100

	token, err := p.accessToken(ctx)
	if err != nil {
		return Charge{}, err
	}
	timestamp := mpesaTimestamp(time.Now())
	reqBody := map[string]any{
		"BusinessShortCode": p.shortcode,
		"Password":          p.password(timestamp),
		"Timestamp":         timestamp,
		"TransactionType":   "CustomerPayBillOnline",
		"Amount":            whole,
		"PartyA":            phone,
		"PartyB":            p.shortcode,
		"PhoneNumber":       phone,
		"CallBackURL":       o.CallbackURL,
		"AccountReference":  o.Reference,
		"TransactionDesc":   "Order " + o.Reference,
	}

	respBody, status, err := p.do(ctx, token, "/mpesa/stkpush/v1/processrequest", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyMpesaError(status, respBody)
	}
	var parsed struct {
		MerchantRequestID string `json:"MerchantRequestID"`
		CheckoutRequestID string `json:"CheckoutRequestID"`
		ResponseCode      string `json:"ResponseCode"`
		ResponseDesc      string `json:"ResponseDescription"`
		CustomerMessage   string `json:"CustomerMessage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrMpesaMalformedResponse, err)
	}
	if parsed.ResponseCode != "0" || parsed.CheckoutRequestID == "" {
		return Charge{}, fmt.Errorf("%w: STK push not accepted: %s", ErrMpesaUnexpectedStatus, parsed.ResponseDesc)
	}
	return Charge{
		Provider:  ProviderNameMpesa,
		Reference: parsed.CheckoutRequestID,
		Instructions: mpesaFirstNonEmpty(parsed.CustomerMessage,
			"An M-Pesa PIN prompt has been sent to the buyer's phone. Poll Verify or wait for the callback to confirm payment."),
	}, nil
}

func mpesaFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// mpesaQueryResult is the shape of both the STK Push Query API response
// and, after this file re-verifies via that same API, what Webhook
// ultimately returns.
type mpesaQueryResult struct {
	CheckoutRequestID string `json:"CheckoutRequestID"`
	ResultCode        string `json:"ResultCode"`
	ResultDesc        string `json:"ResultDesc"`
}

func (p *MpesaProvider) query(ctx context.Context, checkoutRequestID string) (mpesaQueryResult, []byte, error) {
	token, err := p.accessToken(ctx)
	if err != nil {
		return mpesaQueryResult{}, nil, err
	}
	timestamp := mpesaTimestamp(time.Now())
	reqBody := map[string]any{
		"BusinessShortCode": p.shortcode,
		"Password":          p.password(timestamp),
		"Timestamp":         timestamp,
		"CheckoutRequestID": checkoutRequestID,
	}
	respBody, status, err := p.do(ctx, token, "/mpesa/stkpushquery/v1/query", reqBody)
	if err != nil {
		return mpesaQueryResult{}, nil, err
	}
	if status < 200 || status >= 300 {
		return mpesaQueryResult{}, nil, classifyMpesaError(status, respBody)
	}
	var parsed mpesaQueryResult
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return mpesaQueryResult{}, nil, fmt.Errorf("%w: query response: %v", ErrMpesaMalformedResponse, err)
	}
	return parsed, respBody, nil
}

// resultFromQuery builds a Result from the authenticated Query API's
// response. Because the Query API's ResultCode/ResultDesc alone don't
// carry a settled amount (Daraja doesn't expose that on this endpoint,
// only on the original unauthenticated callback body), the caller-
// supplied wantAmountMinor/wantCurrency (the STORED order this
// CheckoutRequestID belongs to) is echoed back as the settled amount ONLY
// when ResultCode is "0" (success) — this is safe specifically because
// STK Push amounts cannot be altered by Safaricom or the buyer after the
// push was created with a fixed Amount; a successful ResultCode
// cryptographically-out-of-band (via TLS + OAuth-authenticated API)
// confirms exactly that fixed amount was paid, nothing more or less.
func resultFromQuery(q mpesaQueryResult, raw []byte, reference string, wantAmountMinor int64, wantCurrency string) (Result, error) {
	if q.CheckoutRequestID == "" || q.CheckoutRequestID != reference {
		return Result{}, fmt.Errorf("%w: query returned CheckoutRequestID %q for requested %q", ErrMpesaMalformedResponse, q.CheckoutRequestID, reference)
	}
	result := Result{
		Provider:  ProviderNameMpesa,
		Reference: reference,
		EventID:   reference, // CheckoutRequestID uniquely identifies this STK push attempt
		Currency:  wantCurrency,
		Raw:       json.RawMessage(raw),
	}
	if q.ResultCode == "0" {
		result.Status = StatusPaid
		result.AmountMinor = wantAmountMinor
	} else {
		result.Status = StatusFailed
	}
	return result, nil
}

// Verify polls Daraja's authenticated STK Push Query API for the given
// CheckoutRequestID (Cackle's reference, as returned by Begin).
//
// Unlike other adapters, Verify alone cannot reconcile an amount (Daraja
// doesn't echo it back on the query endpoint), so it resolves the STORED
// order's amount/currency itself via p.orderLookup (see that field's doc
// comment) before it can build a Result at all. Callers should still run
// this through HandleVerify/Reconcile with the stored order as usual (that
// catches a mismatched CURRENCY, and is what every other adapter relies on
// too) — the amount match here is guaranteed structurally (see
// resultFromQuery) rather than by comparison, precisely because it was
// read from the same stored order Reconcile will check against.
func (p *MpesaProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: mpesa: reference is required")
	}
	if p.orderLookup == nil {
		return Result{}, errors.New("payments: mpesa: Verify requires an OrderLookup (see MpesaProvider.orderLookup) because Daraja's query API does not echo back a settled amount")
	}
	ref, err := p.orderLookup.Lookup(ctx, reference)
	if err != nil {
		return Result{}, fmt.Errorf("payments: mpesa: look up stored order for %q: %w", reference, err)
	}
	q, raw, err := p.query(ctx, reference)
	if err != nil {
		return Result{}, err
	}
	return resultFromQuery(q, raw, reference, ref.AmountMinor, ref.Currency)
}

// Webhook parses Daraja's STK callback ONLY to extract CheckoutRequestID
// (a lookup key), then re-verifies via the authenticated Query API — see
// the file header for why the callback body itself is never trusted.
func (p *MpesaProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrMpesaMalformedResponse)
	}
	body, err := boundedRead(r.Body, mpesaMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrMpesaResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: mpesa: read callback body: %w", err)
	}
	var envelope struct {
		Body struct {
			StkCallback struct {
				CheckoutRequestID string `json:"CheckoutRequestID"`
			} `json:"stkCallback"`
		} `json:"Body"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMpesaMalformedResponse, err)
	}
	checkoutRequestID := strings.TrimSpace(envelope.Body.StkCallback.CheckoutRequestID)
	if checkoutRequestID == "" {
		return Result{}, ErrMpesaMissingCheckoutRequestID
	}
	// From here on this is IDENTICAL to Verify's authenticated re-check —
	// the untrusted callback contributed nothing but a lookup key.
	return p.Verify(ctx, checkoutRequestID)
}

func (p *MpesaProvider) do(ctx context.Context, token, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, mpesaHTTPTimeout)
	defer cancel()

	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: mpesa: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, 0, fmt.Errorf("payments: mpesa: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: mpesa: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, mpesaMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrMpesaResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: mpesa: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func classifyMpesaError(status int, body []byte) error {
	var env struct {
		ErrorMessage string `json:"errorMessage"`
		ResponseDesc string `json:"ResponseDescription"`
	}
	_ = json.Unmarshal(body, &env)
	msg := env.ErrorMessage
	if msg == "" {
		msg = env.ResponseDesc
	}
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrMpesaUnexpectedStatus, status, msg)
}
