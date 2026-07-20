package payments

// Coinbase Commerce adapter — a hosted, CUSTODIAL crypto checkout service,
// same tier as opennode.go in the payments contract's priority ordering
// ("hosted custodial services... as conveniences", behind the self-hosted
// BTCPay/LNbits adapters). Coinbase Commerce briefly touches funds before
// paying the organiser's own Coinbase/bank account out; Cackle itself still
// never holds funds.
//
// Built against Coinbase Commerce's documented API:
//   - API reference:      https://docs.cloud.coinbase.com/commerce/reference
//   - Create a charge:    https://docs.cloud.coinbase.com/commerce/reference/createcharge
//   - Webhook security:   https://docs.cloud.coinbase.com/commerce/docs/webhooks-security
//
// Confidence: HIGH for authentication (X-CC-Api-Key + X-CC-Version request
// headers), the create/fetch charge shape (POST /charges, GET /charges/{id},
// a top-level "data" envelope with "id", "hosted_url", "timeline", and
// "local_price": {"amount", "currency"}), and the webhook signing scheme
// (X-CC-Webhook-Signature: hex HMAC-SHA256 of the raw body, keyed by a
// per-endpoint shared secret from the Coinbase Commerce dashboard) — these
// match this author's recollection of Coinbase Commerce's long-stable
// integration docs. MODERATE confidence on the exact timeline status enum
// beyond NEW/PENDING/COMPLETED/EXPIRED (this adapter also handles
// UNRESOLVED/RESOLVED/CANCELED, which is where under/overpayment surfaces —
// verify these against current docs/your account before relying on them).
// Note: Coinbase Commerce stopped onboarding new merchants at points in its
// history — confirm it is still available for new integrations before
// depending on it. Unit-tested only (no sandbox Coinbase Commerce account
// was available) — see coinbasecommerce_test.go.
//
// # Underpayment / overpayment
//
// Coinbase Commerce's charge timeline can move into an "UNRESOLVED" state
// (optionally carrying more detail on WHY — under/overpaid, multiple
// payments, a delayed confirmation, etc) rather than silently settling or
// silently failing. This adapter treats UNRESOLVED (and its post-manual-
// review successor, "RESOLVED" — which does not by itself guarantee the
// resolution was "paid in full") as a distinct, fail-closed condition: it
// returns ErrCoinbaseCommerceRequiresManualReview instead of a Result, so
// an under/overpayment is FLAGGED for a human to check the Coinbase
// Commerce dashboard, never silently accepted as paid and never silently
// discarded as an ordinary failure either.
//
// # Confirmations
//
// Not independently configurable via this adapter. Coinbase Commerce
// manages required confirmations internally as part of its own
// PENDING -> COMPLETED transition; this adapter trusts that transition
// rather than re-deriving a confirmation count itself.
//
// # FX / quote expiry
//
// Coinbase Commerce locks the exchange rate at charge-creation time; a
// charge that isn't paid within its window moves to "EXPIRED", which this
// adapter reports as StatusFailed — never a stale-rate late settlement.
import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProviderNameCoinbaseCommerce is the stable Name() this provider registers
// under.
const ProviderNameCoinbaseCommerce = "coinbasecommerce"

// coinbaseCommerceDefaultBaseURL is Coinbase Commerce's production API
// base — a public hostname, not a secret, so (like opennode.go) it has a
// sensible default rather than requiring self-hosted-style configuration.
const coinbaseCommerceDefaultBaseURL = "https://api.commerce.coinbase.com"

// coinbaseCommerceAPIVersion is sent as the required X-CC-Version header.
// Coinbase Commerce versions its API by date; this is the last version this
// author has direct knowledge of. Confirm against current docs.
const coinbaseCommerceAPIVersion = "2018-03-22"

// Environment variables CoinbaseCommerceProvider reads from.
const (
	EnvCoinbaseCommerceAPIKey        = "CACKLE_COINBASECOMMERCE_API_KEY"        // required
	EnvCoinbaseCommerceWebhookSecret = "CACKLE_COINBASECOMMERCE_WEBHOOK_SECRET" // required
	EnvCoinbaseCommerceBaseURL       = "CACKLE_COINBASECOMMERCE_BASE_URL"       // optional, defaults to coinbaseCommerceDefaultBaseURL
)

// Sentinel errors specific to the Coinbase Commerce adapter.
var (
	ErrCoinbaseCommerceNotConfigured     = errors.New("payments: coinbasecommerce: " + EnvCoinbaseCommerceAPIKey + " and " + EnvCoinbaseCommerceWebhookSecret + " must both be set")
	ErrCoinbaseCommerceMissingSignature  = errors.New("payments: coinbasecommerce: missing X-CC-Webhook-Signature header")
	ErrCoinbaseCommerceInvalidSignature  = errors.New("payments: coinbasecommerce: invalid X-CC-Webhook-Signature")
	ErrCoinbaseCommerceUnexpectedStatus  = errors.New("payments: coinbasecommerce: unexpected API response status")
	ErrCoinbaseCommerceMalformedResponse = errors.New("payments: coinbasecommerce: malformed API response")
	ErrCoinbaseCommerceResponseTooLarge  = errors.New("payments: coinbasecommerce: response body exceeds size limit")
	// ErrCoinbaseCommerceRequiresManualReview is returned instead of a
	// Result when the charge timeline is UNRESOLVED or RESOLVED — see the
	// file doc comment on under/overpayment.
	ErrCoinbaseCommerceRequiresManualReview = errors.New("payments: coinbasecommerce: charge requires manual review in the Coinbase Commerce dashboard (unresolved/resolved timeline state) — this adapter will not guess whether it was paid in full")
)

// CoinbaseCommerceProvider implements Provider against Coinbase Commerce's
// hosted checkout API. See the file-level doc comment for API references
// and confidence notes.
type CoinbaseCommerceProvider struct {
	baseURL       string
	apiKey        string
	webhookSecret string
	httpClient    *http.Client
}

// NewCoinbaseCommerce constructs a CoinbaseCommerceProvider from
// EnvCoinbaseCommerceAPIKey and EnvCoinbaseCommerceWebhookSecret (both
// required), plus the optional EnvCoinbaseCommerceBaseURL.
func NewCoinbaseCommerce() (*CoinbaseCommerceProvider, error) {
	key := strings.TrimSpace(os.Getenv(EnvCoinbaseCommerceAPIKey))
	secret := strings.TrimSpace(os.Getenv(EnvCoinbaseCommerceWebhookSecret))
	if key == "" || secret == "" {
		return nil, ErrCoinbaseCommerceNotConfigured
	}
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv(EnvCoinbaseCommerceBaseURL)), "/")
	if base == "" {
		base = coinbaseCommerceDefaultBaseURL
	}
	return &CoinbaseCommerceProvider{
		baseURL:       base,
		apiKey:        key,
		webhookSecret: secret,
		httpClient:    &http.Client{Timeout: cryptoDefaultHTTPTimeout},
	}, nil
}

// Name implements Provider.
func (p *CoinbaseCommerceProvider) Name() string { return ProviderNameCoinbaseCommerce }

// Capabilities implements Provider.
func (p *CoinbaseCommerceProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil,
		Countries:     nil,
		Flow:          FlowInvoice,
		Refunds:       false,
		Payouts:       true, // Coinbase Commerce pays merchants out independent of Cackle
		Webhooks:      true,
		ZeroDecimalOK: true,
	}
}

// coinbaseCommerceTimelineEntry is one entry in a charge's status timeline.
// Context is only populated for UNRESOLVED entries and, per the file doc
// comment, is only moderately confident — this adapter surfaces it in error
// messages for a human to read, never parses it into an automated decision.
type coinbaseCommerceTimelineEntry struct {
	Time    string `json:"time"`
	Status  string `json:"status"`
	Context string `json:"context,omitempty"`
}

type coinbaseCommerceCharge struct {
	ID        string                          `json:"id"`
	HostedURL string                          `json:"hosted_url"`
	Timeline  []coinbaseCommerceTimelineEntry `json:"timeline"`
	Pricing   struct {
		Local struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"local"`
	} `json:"pricing"`
}

type coinbaseCommerceEnvelope struct {
	Data coinbaseCommerceCharge `json:"data"`
}

// Begin creates a Coinbase Commerce charge priced in the order's fiat
// currency and returns its hosted checkout URL.
func (p *CoinbaseCommerceProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: coinbasecommerce: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: coinbasecommerce: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: coinbasecommerce: currency is required")
	}
	amountStr, err := minorToMajorString(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: coinbasecommerce: %w", err)
	}

	meta := map[string]string{"order_id": o.Reference}
	if o.EventID != "" {
		meta["event_id"] = o.EventID
	}
	if o.OrgID != "" {
		meta["org_id"] = o.OrgID
	}

	reqBody := map[string]any{
		"name":         "Cackle order " + o.Reference,
		"description":  "Cackle order " + o.Reference,
		"pricing_type": "fixed_price",
		"local_price": map[string]string{
			"amount":   amountStr,
			"currency": currency,
		},
		"metadata": meta,
	}
	if o.CallbackURL != "" {
		reqBody["redirect_url"] = o.CallbackURL
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/charges", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyCoinbaseCommerceError(status, respBody)
	}

	var envelope coinbaseCommerceEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrCoinbaseCommerceMalformedResponse, err)
	}
	if envelope.Data.ID == "" || envelope.Data.HostedURL == "" {
		return Charge{}, fmt.Errorf("%w: empty id or hosted_url", ErrCoinbaseCommerceMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNameCoinbaseCommerce,
		Reference:   envelope.Data.ID,
		RedirectURL: envelope.Data.HostedURL,
	}, nil
}

// Verify fetches the charge directly from Coinbase Commerce and reports its
// settlement state, derived from the LATEST entry in its status timeline.
func (p *CoinbaseCommerceProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: coinbasecommerce: reference is required")
	}
	charge, err := p.fetchCharge(ctx, reference)
	if err != nil {
		return Result{}, err
	}
	return coinbaseCommerceResultFromCharge(charge)
}

func (p *CoinbaseCommerceProvider) fetchCharge(ctx context.Context, id string) (coinbaseCommerceCharge, error) {
	respBody, status, err := p.do(ctx, http.MethodGet, "/charges/"+url.PathEscape(id), nil)
	if err != nil {
		return coinbaseCommerceCharge{}, err
	}
	if status < 200 || status >= 300 {
		return coinbaseCommerceCharge{}, classifyCoinbaseCommerceError(status, respBody)
	}
	var envelope coinbaseCommerceEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return coinbaseCommerceCharge{}, fmt.Errorf("%w: %v", ErrCoinbaseCommerceMalformedResponse, err)
	}
	if envelope.Data.ID == "" {
		return coinbaseCommerceCharge{}, fmt.Errorf("%w: empty charge id", ErrCoinbaseCommerceMalformedResponse)
	}
	return envelope.Data, nil
}

// coinbaseCommerceResultFromCharge maps a charge's LATEST timeline entry
// onto a Result, failing closed on anything malformed, unresolved, or
// unrecognised.
func coinbaseCommerceResultFromCharge(charge coinbaseCommerceCharge) (Result, error) {
	if len(charge.Timeline) == 0 {
		return Result{}, fmt.Errorf("%w: empty timeline", ErrCoinbaseCommerceMalformedResponse)
	}
	latest := charge.Timeline[len(charge.Timeline)-1]

	amountMinor, err := majorStringToMinor(charge.Pricing.Local.Amount, charge.Pricing.Local.Currency)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrCoinbaseCommerceMalformedResponse, err)
	}

	raw, _ := json.Marshal(charge)
	result := Result{
		Provider:    ProviderNameCoinbaseCommerce,
		Reference:   charge.ID,
		EventID:     charge.ID, // a charge settles at most once; the charge id is a stable dedupe key
		AmountMinor: amountMinor,
		Currency:    charge.Pricing.Local.Currency,
		Raw:         json.RawMessage(raw),
	}

	switch latest.Status {
	case "COMPLETED":
		result.Status = StatusPaid
		result.PaidAt = time.Now().UTC()
	case "NEW", "PENDING":
		result.Status = StatusPending
	case "EXPIRED", "CANCELED":
		result.Status = StatusFailed
	case "UNRESOLVED", "RESOLVED":
		if latest.Context != "" {
			return Result{}, fmt.Errorf("%w: context=%s", ErrCoinbaseCommerceRequiresManualReview, latest.Context)
		}
		return Result{}, ErrCoinbaseCommerceRequiresManualReview
	default:
		// Fail closed: an unrecognised status is never treated as paid.
		return Result{}, fmt.Errorf("%w: unrecognised timeline status %q", ErrCoinbaseCommerceMalformedResponse, latest.Status)
	}
	return result, nil
}

// Webhook validates Coinbase Commerce's HMAC-SHA256 signature (header
// X-CC-Webhook-Signature, hex, computed over the raw request body with the
// configured webhook shared secret), then refetches the charge from
// Coinbase Commerce's authenticated API rather than trust the (also
// present) embedded charge/timeline data in the webhook payload — the same
// defense-in-depth pattern used by every adapter in this file group.
func (p *CoinbaseCommerceProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	sigHeader := strings.TrimSpace(r.Header.Get("X-CC-Webhook-Signature"))
	if sigHeader == "" {
		return Result{}, ErrCoinbaseCommerceMissingSignature
	}

	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrCoinbaseCommerceMalformedResponse)
	}
	body, err := boundedRead(r.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrCoinbaseCommerceResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: coinbasecommerce: read webhook body: %w", err)
	}

	given, err := hex.DecodeString(sigHeader)
	if err != nil {
		return Result{}, fmt.Errorf("%w: signature is not valid hex", ErrCoinbaseCommerceInvalidSignature)
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, given) {
		return Result{}, ErrCoinbaseCommerceInvalidSignature
	}

	var envelope struct {
		Event struct {
			Type string `json:"type"`
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrCoinbaseCommerceMalformedResponse, err)
	}
	if !strings.HasPrefix(envelope.Event.Type, "charge:") {
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Event.Type)
	}
	if envelope.Event.Data.ID == "" {
		return Result{}, fmt.Errorf("%w: missing event.data.id", ErrCoinbaseCommerceMalformedResponse)
	}

	charge, err := p.fetchCharge(ctx, envelope.Event.Data.ID)
	if err != nil {
		return Result{}, err
	}
	return coinbaseCommerceResultFromCharge(charge)
}

// do issues an authenticated Coinbase Commerce API request, bounding it
// with cryptoDefaultHTTPTimeout and capping the response body it reads.
func (p *CoinbaseCommerceProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, cryptoDefaultHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: coinbasecommerce: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: coinbasecommerce: build request: %w", err)
	}
	req.Header.Set("X-CC-Api-Key", p.apiKey)
	req.Header.Set("X-CC-Version", coinbaseCommerceAPIVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: coinbasecommerce: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrCoinbaseCommerceResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: coinbasecommerce: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// classifyCoinbaseCommerceError builds an error for a non-2xx Coinbase
// Commerce response, best-effort including its own message without ever
// including request headers or the API key.
func classifyCoinbaseCommerceError(status int, body []byte) error {
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env) // best-effort; body may not be JSON
	msg := env.Error.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrCoinbaseCommerceUnexpectedStatus, status, msg)
}
