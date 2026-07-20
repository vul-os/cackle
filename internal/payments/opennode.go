package payments

// OpenNode adapter — a hosted, CUSTODIAL Bitcoin/Lightning checkout
// service. Unlike btcpay.go/lnbits.go, OpenNode is not self-hosted: the
// organiser holds an OpenNode merchant account and OpenNode itself briefly
// touches the funds before paying the organiser out. That's why the
// payments contract ranks this as priority 3 ("hosted custodial services...
// as conveniences") behind BTCPay/LNbits, not the flagship. Cackle still
// never holds funds itself — money moves buyer -> OpenNode -> organiser's
// own OpenNode/bank payout, same as Paystack.
//
// Built against OpenNode's documented API:
//   - API reference:    https://developers.opennode.com/reference
//   - Create a charge:  https://developers.opennode.com/reference/create-charge
//   - Charge webhooks:  https://developers.opennode.com/docs/webhooks
//
// Confidence: HIGH for the create/fetch charge shape (POST /v1/charges,
// GET /v1/charge/{id}, {"data": {"id", "status", "amount", "currency",
// "hosted_checkout_url"}}) and the documented status enum (unpaid,
// processing, paid, underpaid, expired, refunded) — these are stable,
// long-documented parts of OpenNode's API. MODERATE confidence on the exact
// webhook signing scheme: OpenNode's callback historically POSTs
// form-encoded fields including `hashed_order`, verified as
// HMAC-SHA256(key=API key, message=charge id) — this author's recollection
// of that construction is not 100% certain and callback verification
// schemes have shifted across API versions historically. To compensate,
// this adapter does NOT stop at the signature check: after verifying it,
// it always refetches the charge from OpenNode's authenticated API before
// building a Result, so even a subtly-wrong signature construction cannot
// fabricate a settlement — at worst it would (with a still-guessable-only-
// by-brute-force id) cause an extra authenticated read. Treat the webhook
// signature check here as defense in depth on top of that refetch, not the
// sole gate, until you've confirmed the exact scheme against current
// OpenNode docs. Unit-tested only (no sandbox OpenNode account was
// available) — see opennode_test.go.
//
// # Underpayment
//
// OpenNode's documented status enum has an explicit "underpaid" state,
// used directly below: it is mapped to StatusFailed, never StatusPaid. A
// partially-paid charge cannot settle an order through this adapter.
//
// # Overpayment
//
// This author is not aware of an explicit "overpaid" status in OpenNode's
// documented enum (unlike BTCPay's additionalStatus=="PaidOver"). If
// OpenNode itself tolerates/settles a modest overpayment silently as
// "paid", this adapter reports whatever amount OpenNode's charge object
// says was settled — the generic Reconcile() step (in provider.go) is the
// backstop: it rejects any amount that doesn't exactly equal the stored
// order total, so an overpayment can surface as ErrAmountMismatch on an
// otherwise StatusPaid result, distinguishing it from ordinary tampering
// for a caller inspecting both fields together, exactly as documented on
// btcpay.go's overpayment handling.
//
// # Confirmations
//
// Not independently configurable via this adapter. OpenNode manages
// required on-chain confirmations internally as part of computing when a
// charge's status becomes "paid" (vs "processing"); this adapter trusts
// that status transition rather than re-deriving a confirmation count
// itself, the same trust boundary documented on btcpay.go.
//
// # FX / quote expiry
//
// OpenNode prices and locks the exchange rate at charge-creation time and
// charges expire ("expired" status) after a window OpenNode controls. This
// adapter reports StatusFailed for an expired charge — a fresh charge (and
// fresh rate) is required, never a stale-rate late settlement.
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
	"strconv"
	"strings"
	"time"
)

// ProviderNameOpenNode is the stable Name() this provider registers under.
const ProviderNameOpenNode = "opennode"

// opennodeDefaultBaseURL is OpenNode's production API base. It is not a
// secret (see rule 5 in the payments contract — that rule governs API
// keys/webhook secrets, not a well-known public API hostname), so unlike
// BTCPay/LNbits (genuinely self-hosted, no "the" instance) this has a
// sensible default, overridable for OpenNode's dev/sandbox environment.
const opennodeDefaultBaseURL = "https://api.opennode.com"

// Environment variables OpenNodeProvider reads from.
const (
	EnvOpenNodeAPIKey  = "CACKLE_OPENNODE_API_KEY"  // required, no default
	EnvOpenNodeBaseURL = "CACKLE_OPENNODE_BASE_URL" // optional, defaults to opennodeDefaultBaseURL
)

// Sentinel errors specific to the OpenNode adapter.
var (
	ErrOpenNodeNotConfigured     = errors.New("payments: opennode: " + EnvOpenNodeAPIKey + " must be set")
	ErrOpenNodeMissingSignature  = errors.New("payments: opennode: missing hashed_order in webhook callback")
	ErrOpenNodeInvalidSignature  = errors.New("payments: opennode: invalid hashed_order in webhook callback")
	ErrOpenNodeUnexpectedStatus  = errors.New("payments: opennode: unexpected API response status")
	ErrOpenNodeMalformedResponse = errors.New("payments: opennode: malformed API response")
	ErrOpenNodeResponseTooLarge  = errors.New("payments: opennode: response body exceeds size limit")
)

// OpenNodeProvider implements Provider against OpenNode's hosted checkout
// API. See the file-level doc comment for API references and confidence
// notes.
type OpenNodeProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenNode constructs an OpenNodeProvider from EnvOpenNodeAPIKey
// (required) and EnvOpenNodeBaseURL (optional).
func NewOpenNode() (*OpenNodeProvider, error) {
	key := strings.TrimSpace(os.Getenv(EnvOpenNodeAPIKey))
	if key == "" {
		return nil, ErrOpenNodeNotConfigured
	}
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv(EnvOpenNodeBaseURL)), "/")
	if base == "" {
		base = opennodeDefaultBaseURL
	}
	return &OpenNodeProvider{
		baseURL:    base,
		apiKey:     key,
		httpClient: &http.Client{Timeout: cryptoDefaultHTTPTimeout},
	}, nil
}

// Name implements Provider.
func (p *OpenNodeProvider) Name() string { return ProviderNameOpenNode }

// Capabilities implements Provider.
func (p *OpenNodeProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil,
		Countries:     nil,
		Flow:          FlowInvoice,
		Refunds:       false,
		Payouts:       true, // OpenNode pays merchants out to their own bank/wallet independent of Cackle
		Webhooks:      true,
		ZeroDecimalOK: true,
	}
}

// opennodeCharge mirrors the fields this adapter reads from OpenNode's
// charge object (both create and fetch responses share this shape, nested
// under a top-level "data" key).
type opennodeCharge struct {
	ID                string          `json:"id"`
	Status            string          `json:"status"` // unpaid | processing | paid | underpaid | expired | refunded
	Amount            json.RawMessage `json:"amount"`
	Currency          string          `json:"currency"`
	HostedCheckoutURL string          `json:"hosted_checkout_url"`
}

type opennodeEnvelope struct {
	Data opennodeCharge `json:"data"`
}

// Begin creates an OpenNode charge priced in the order's fiat currency and
// returns its hosted checkout URL.
func (p *OpenNodeProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: opennode: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: opennode: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: opennode: currency is required")
	}
	amountStr, err := minorToMajorString(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: opennode: %w", err)
	}
	amountFloat, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: opennode: could not render amount as a number: %w", err)
	}

	reqBody := map[string]any{
		"amount":      amountFloat,
		"currency":    currency,
		"order_id":    o.Reference,
		"description": "Cackle order " + o.Reference,
	}
	if o.BuyerEmail != "" {
		reqBody["customer_email"] = o.BuyerEmail
	}
	if o.CallbackURL != "" {
		reqBody["success_url"] = o.CallbackURL
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/v1/charges", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyOpenNodeError(status, respBody)
	}

	var envelope opennodeEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrOpenNodeMalformedResponse, err)
	}
	if envelope.Data.ID == "" || envelope.Data.HostedCheckoutURL == "" {
		return Charge{}, fmt.Errorf("%w: empty id or hosted_checkout_url", ErrOpenNodeMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNameOpenNode,
		Reference:   envelope.Data.ID,
		RedirectURL: envelope.Data.HostedCheckoutURL,
	}, nil
}

// Verify fetches the charge directly from OpenNode and reports its
// settlement state.
func (p *OpenNodeProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: opennode: reference is required")
	}
	charge, err := p.fetchCharge(ctx, reference)
	if err != nil {
		return Result{}, err
	}
	return opennodeResultFromCharge(charge)
}

func (p *OpenNodeProvider) fetchCharge(ctx context.Context, id string) (opennodeCharge, error) {
	respBody, status, err := p.do(ctx, http.MethodGet, "/v1/charge/"+url.PathEscape(id), nil)
	if err != nil {
		return opennodeCharge{}, err
	}
	if status < 200 || status >= 300 {
		return opennodeCharge{}, classifyOpenNodeError(status, respBody)
	}
	var envelope opennodeEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return opennodeCharge{}, fmt.Errorf("%w: %v", ErrOpenNodeMalformedResponse, err)
	}
	if envelope.Data.ID == "" {
		return opennodeCharge{}, fmt.Errorf("%w: empty charge id", ErrOpenNodeMalformedResponse)
	}
	return envelope.Data, nil
}

// opennodeResultFromCharge maps an OpenNode charge's status onto a Result,
// failing closed on anything malformed or unrecognised.
func opennodeResultFromCharge(charge opennodeCharge) (Result, error) {
	amountStr, err := flexibleJSONAmountToString(charge.Amount)
	if err != nil {
		return Result{}, fmt.Errorf("%w: amount: %v", ErrOpenNodeMalformedResponse, err)
	}
	amountMinor, err := majorStringToMinor(amountStr, charge.Currency)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrOpenNodeMalformedResponse, err)
	}

	raw, _ := json.Marshal(charge)
	result := Result{
		Provider:    ProviderNameOpenNode,
		Reference:   charge.ID,
		EventID:     charge.ID, // a charge settles at most once; the charge id is a stable dedupe key
		AmountMinor: amountMinor,
		Currency:    charge.Currency,
		Raw:         json.RawMessage(raw),
	}

	switch charge.Status {
	case "paid":
		result.Status = StatusPaid
		result.PaidAt = time.Now().UTC()
	case "unpaid", "processing":
		result.Status = StatusPending
	case "underpaid", "expired", "refunded":
		// underpaid: never settles, by contract requirement.
		// expired: the quote window closed unpaid.
		// refunded: money is no longer with the organiser.
		result.Status = StatusFailed
	default:
		// Fail closed: an unrecognised status is never treated as paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// flexibleJSONAmountToString extracts a decimal amount string from a JSON
// value that might be a bare number token OR a quoted string, without ever
// round-tripping through float64 (which would risk a precision bug in a
// money amount). This author is not fully certain whether OpenNode's API
// returns "amount" as a JSON number or a JSON string in every version, so
// both are handled explicitly rather than assumed.
func flexibleJSONAmountToString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("empty amount")
	}
	// Try as a bare JSON number first: json.Number preserves the EXACT
	// textual digits of the token, so this never loses precision the way
	// unmarshalling into float64 would.
	var num json.Number
	if err := json.Unmarshal(raw, &num); err == nil {
		return num.String(), nil
	}
	// Fall back to a quoted JSON string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	return "", fmt.Errorf("amount is neither a JSON number nor a JSON string: %s", string(raw))
}

// Webhook verifies OpenNode's documented callback signature (hashed_order =
// HMAC-SHA256(key=API key, message=charge id), form-encoded body), then —
// as defense in depth given this author's moderate confidence in that
// exact construction (see file doc comment) — refetches the charge from
// OpenNode's authenticated API before reporting a Result, rather than
// trust any other field in the callback body.
func (p *OpenNodeProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrOpenNodeMalformedResponse)
	}
	body, err := boundedRead(r.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrOpenNodeResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: opennode: read webhook body: %w", err)
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return Result{}, fmt.Errorf("%w: could not parse form-encoded body: %v", ErrOpenNodeMalformedResponse, err)
	}
	id := values.Get("id")
	hashedOrder := values.Get("hashed_order")
	if id == "" {
		return Result{}, fmt.Errorf("%w: missing id", ErrOpenNodeMalformedResponse)
	}
	if hashedOrder == "" {
		return Result{}, ErrOpenNodeMissingSignature
	}

	given, err := hex.DecodeString(hashedOrder)
	if err != nil {
		return Result{}, fmt.Errorf("%w: hashed_order is not valid hex", ErrOpenNodeInvalidSignature)
	}
	mac := hmac.New(sha256.New, []byte(p.apiKey))
	mac.Write([]byte(id))
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, given) {
		return Result{}, ErrOpenNodeInvalidSignature
	}

	charge, err := p.fetchCharge(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return opennodeResultFromCharge(charge)
}

// do issues an authenticated OpenNode API request, bounding it with
// cryptoDefaultHTTPTimeout and capping the response body it reads.
func (p *OpenNodeProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, cryptoDefaultHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: opennode: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: opennode: build request: %w", err)
	}
	req.Header.Set("Authorization", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: opennode: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrOpenNodeResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: opennode: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// classifyOpenNodeError builds an error for a non-2xx OpenNode response,
// best-effort including OpenNode's own message without ever including
// request headers or the API key.
func classifyOpenNodeError(status int, body []byte) error {
	var env struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &env) // best-effort; body may not be JSON
	msg := env.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrOpenNodeUnexpectedStatus, status, msg)
}
