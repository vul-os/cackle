package payments

// BTCPay Server adapter — the flagship crypto provider for Cackle.
//
// BTCPay Server (https://btcpayserver.org) is self-hosted and non-custodial:
// the organiser runs their own BTCPay instance (or uses a host that runs one
// for them) against their OWN on-chain wallet / Lightning node. Cackle only
// ever talks to the organiser's BTCPay instance over its API; funds move
// directly from buyer to organiser wallet. This matches Cackle's own
// never-hold-funds design exactly, which is why it's the flagship adapter
// in this group rather than a hosted custodial service.
//
// Built against BTCPay Server's documented Greenfield API v1:
//   - API reference / auth:  https://docs.btcpayserver.org/API/Greenfield/v1/
//   - Invoices endpoint:     https://docs.btcpayserver.org/API/Greenfield/v1/#tag/Invoices
//   - Webhooks:              https://docs.btcpayserver.org/Webhooks/
//
// Confidence: HIGH for the invoice create/fetch shape (id, amount, currency,
// status, checkoutLink, additionalStatus) and for the webhook signing scheme
// (BTCPay-Sig: sha256=<hex hmac-sha256 of the raw body>, keyed by a
// per-webhook shared secret) — both are called out explicitly in the payments
// contract and match this author's recollection of the Greenfield API.
// MODERATE confidence on the exact `additionalStatus` enum values
// (None/Marked/Invalid/PaidPartial/PaidOver/PaidLate) used below to detect
// under/overpayment — verify these against your BTCPay Server version before
// relying on the PaidOver/PaidPartial distinction in production. This
// adapter is unit-tested only (no sandbox BTCPay instance was available) —
// see btcpay_test.go.
//
// # Confirmations
//
// This adapter deliberately does NOT re-implement a confirmation counter.
// BTCPay Server already enforces a per-store "SpeedPolicy" (0-conf / 1-conf
// / N-conf, configurable in the BTCPay admin UI) before it will ever move an
// invoice to status "Settled". This adapter trusts that invariant: only
// status=="Settled" is ever reported as StatusPaid. If you need a different
// confirmation threshold than your store's SpeedPolicy, configure it in
// BTCPay itself — Cackle has no additional confirmation-count knob for this
// adapter because BTCPay's own invoice status already encodes it, and
// guessing at an undocumented per-payment confirmation field felt riskier
// than leaning on the status BTCPay itself already computes.
//
// # Underpayment
//
// An invoice that never reaches full payment stays in "New" or "Processing"
// (or expires into "Invalid"/"Expired") — it never reaches "Settled", so it
// is reported as StatusPending (or StatusFailed once expired/invalid), never
// StatusPaid. A partially-paid invoice structurally cannot settle an order
// through this adapter.
//
// # Overpayment
//
// If BTCPay reports additionalStatus=="PaidOver" this adapter refuses to
// guess the actual overpaid amount (the top-level invoice object's
// documented fields don't include a confident "amount actually received"
// figure distinct from the invoice's own ask) and returns
// ErrBTCPayOverpaid instead of a Result — a flagged, fail-closed condition
// that requires a human to check the BTCPay dashboard and refund/credit the
// difference, rather than silently accepting it as a normal settlement.
//
// # FX / quote expiry
//
// BTCPay prices and locks the exchange rate for the fiat amount requested at
// invoice-creation time, and invoices expire (default ~15-60 minutes,
// configurable per store) — after which status becomes "Expired" and this
// adapter reports StatusFailed. There is no field on Charge to carry the
// expiry timestamp explicitly (the v2 Provider contract's Charge type only
// has Reference/RedirectURL/Instructions), so Begin puts the invoice's
// expiration time into Charge.Instructions as human-readable text. Once
// expired, BTCPay will not settle the invoice at the old rate — a fresh
// invoice (and fresh quote) is required, which is exactly the "never let
// rate drift silently change what the buyer owed" behaviour the contract
// asks for.

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

// ---------------------------------------------------------------------
// Shared helpers for the crypto adapter group (btcpay/lnbits/opennode/
// coinbasecommerce/stablecoin).
//
// Minor<->major currency conversion (the "1234 minor units of USD is
// '12.34'" logic, correct for zero- and three-decimal currencies too) is
// NOT duplicated here: currency.go already provides
// minorUnitExponent/minorToMajorString/majorStringToMinor as this
// package's shared, independently-tested conversion helpers, and its own
// doc comment says every adapter needing this MUST route through it. All
// five crypto adapters use those directly.
//
// What IS declared once here, for this file group only, is an HTTP
// timeout and a response-size cap — httpshared.go's boundedRead covers the
// bounded-read mechanics (reused directly), but there's no shared timeout
// constant yet, and crypto processors (self-hosted BTCPay in particular)
// warrant a wider margin than Paystack's 15s.
// ---------------------------------------------------------------------

// cryptoDefaultHTTPTimeout bounds every outbound call any adapter in this
// file group makes, regardless of the caller's own context deadline.
const cryptoDefaultHTTPTimeout = 20 * time.Second

// cryptoMaxBodyBytes caps every HTTP body (API responses and incoming
// webhook bodies) this file group will read, via httpshared.go's
// boundedRead, so nothing ever does an unbounded read of a remote response.
const cryptoMaxBodyBytes int64 = 1 << 20 // 1 MiB

// ---------------------------------------------------------------------
// BTCPay adapter proper
// ---------------------------------------------------------------------

// ProviderNameBTCPay is the stable Name() this provider registers under.
const ProviderNameBTCPay = "btcpay"

// Environment variables BTCPayProvider reads from — and the ONLY place its
// secrets may come from. There is no default for any of these: BTCPay is
// self-hosted, so there is no "the" BTCPay instance to point at.
const (
	EnvBTCPayBaseURL       = "CACKLE_BTCPAY_BASE_URL"       // e.g. https://btcpay.example.com
	EnvBTCPayAPIKey        = "CACKLE_BTCPAY_API_KEY"        // Greenfield API key
	EnvBTCPayStoreID       = "CACKLE_BTCPAY_STORE_ID"       // the BTCPay store to invoice against
	EnvBTCPayWebhookSecret = "CACKLE_BTCPAY_WEBHOOK_SECRET" // per-webhook shared secret configured in BTCPay
)

// Sentinel errors specific to the BTCPay adapter. Match with errors.Is;
// none of these ever include the API key or webhook secret.
var (
	ErrBTCPayNotConfigured     = errors.New("payments: btcpay: " + EnvBTCPayBaseURL + ", " + EnvBTCPayAPIKey + ", " + EnvBTCPayStoreID + " and " + EnvBTCPayWebhookSecret + " must all be set")
	ErrBTCPayMissingSignature  = errors.New("payments: btcpay: missing BTCPay-Sig webhook header")
	ErrBTCPayInvalidSignature  = errors.New("payments: btcpay: invalid BTCPay-Sig webhook signature")
	ErrBTCPayUnexpectedStatus  = errors.New("payments: btcpay: unexpected API response status")
	ErrBTCPayMalformedResponse = errors.New("payments: btcpay: malformed API response")
	ErrBTCPayResponseTooLarge  = errors.New("payments: btcpay: response body exceeds size limit")
	// ErrBTCPayOverpaid is returned instead of a Result when BTCPay reports
	// additionalStatus=="PaidOver". This is a deliberate fail-closed flag,
	// not a guess: this adapter is not confident enough in the exact
	// "amount actually received" field to report a trustworthy overpaid
	// figure, so it refuses to synthesize one and asks a human to check the
	// BTCPay dashboard instead.
	ErrBTCPayOverpaid = errors.New("payments: btcpay: invoice reports an overpayment (additionalStatus=PaidOver) — check the BTCPay dashboard and refund/credit the difference manually; this adapter will not guess the received amount")
	// ErrBTCPayInconsistentStatus covers a status/additionalStatus
	// combination this adapter doesn't expect (e.g. Settled+PaidPartial) —
	// fail closed rather than guess which field to trust.
	ErrBTCPayInconsistentStatus = errors.New("payments: btcpay: invoice reported an inconsistent status/additionalStatus combination")
)

// BTCPayProvider implements Provider against a self-hosted BTCPay Server
// instance. See the file-level doc comment for API references and
// confidence notes.
type BTCPayProvider struct {
	baseURL       string
	storeID       string
	apiKey        string
	webhookSecret string
	httpClient    *http.Client
}

// NewBTCPay constructs a BTCPayProvider from EnvBTCPayBaseURL,
// EnvBTCPayAPIKey, EnvBTCPayStoreID and EnvBTCPayWebhookSecret. All four are
// required — there is no default BTCPay instance.
func NewBTCPay() (*BTCPayProvider, error) {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv(EnvBTCPayBaseURL)), "/")
	key := strings.TrimSpace(os.Getenv(EnvBTCPayAPIKey))
	store := strings.TrimSpace(os.Getenv(EnvBTCPayStoreID))
	secret := strings.TrimSpace(os.Getenv(EnvBTCPayWebhookSecret))
	if base == "" || key == "" || store == "" || secret == "" {
		return nil, ErrBTCPayNotConfigured
	}
	return &BTCPayProvider{
		baseURL:       base,
		storeID:       store,
		apiKey:        key,
		webhookSecret: secret,
		httpClient:    &http.Client{Timeout: cryptoDefaultHTTPTimeout},
	}, nil
}

// Name implements Provider.
func (p *BTCPayProvider) Name() string { return ProviderNameBTCPay }

// Capabilities implements Provider.
func (p *BTCPayProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil, // broad: whatever fiat currencies the store's configured rate sources support
		Countries:     nil, // self-hosted, not tied to any merchant country
		Flow:          FlowInvoice,
		Refunds:       false, // BTCPay has a refund API this adapter does not implement (see file doc comment)
		Payouts:       false, // funds settle directly to the organiser's own wallet; there is nothing to "pay out"
		Webhooks:      true,
		ZeroDecimalOK: true,
	}
}

// btcpayInvoice mirrors the fields this adapter reads from BTCPay's
// Greenfield invoice object (both the create-invoice response and the
// get-invoice response share this shape).
type btcpayInvoice struct {
	ID               string `json:"id"`
	StoreID          string `json:"storeId"`
	Amount           string `json:"amount"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`           // New | Processing | Settled | Expired | Invalid
	AdditionalStatus string `json:"additionalStatus"` // None | Marked | Invalid | PaidPartial | PaidOver | PaidLate
	CheckoutLink     string `json:"checkoutLink"`
	CreatedTime      int64  `json:"createdTime"`
	ExpirationTime   int64  `json:"expirationTime"`
}

// Begin creates a BTCPay invoice priced in the order's fiat currency and
// returns its hosted checkout link.
func (p *BTCPayProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: btcpay: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: btcpay: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: btcpay: currency is required")
	}

	meta := map[string]string{"orderId": o.Reference}
	if o.EventID != "" {
		meta["eventId"] = o.EventID
	}
	if o.OrgID != "" {
		meta["orgId"] = o.OrgID
	}

	amountStr, err := minorToMajorString(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: btcpay: %w", err)
	}
	reqBody := map[string]any{
		"amount":   amountStr,
		"currency": currency,
		"metadata": meta,
	}
	if o.CallbackURL != "" {
		reqBody["checkout"] = map[string]any{"redirectURL": o.CallbackURL}
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/api/v1/stores/"+url.PathEscape(p.storeID)+"/invoices", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyBTCPayError(status, respBody)
	}

	var inv btcpayInvoice
	if err := json.Unmarshal(respBody, &inv); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrBTCPayMalformedResponse, err)
	}
	if inv.ID == "" || inv.CheckoutLink == "" {
		return Charge{}, fmt.Errorf("%w: empty id or checkoutLink", ErrBTCPayMalformedResponse)
	}

	instructions := "Pay the BTCPay invoice before it expires — the quoted BTC/Lightning rate is only locked for this invoice's expiry window."
	if inv.ExpirationTime > 0 {
		instructions = fmt.Sprintf("Pay before %s — BTCPay locks the exchange rate only until then; after that a new invoice (and a fresh rate) is required.",
			time.Unix(inv.ExpirationTime, 0).UTC().Format(time.RFC3339))
	}

	return Charge{
		Provider:     ProviderNameBTCPay,
		Reference:    inv.ID,
		RedirectURL:  inv.CheckoutLink,
		Instructions: instructions,
	}, nil
}

// Verify fetches the invoice directly from BTCPay and reports its
// settlement state. Fails closed on any transport, parse, or inconsistent
// status.
func (p *BTCPayProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: btcpay: reference is required")
	}
	inv, err := p.fetchInvoice(ctx, reference)
	if err != nil {
		return Result{}, err
	}
	return btcpayResultFromInvoice(inv, nil)
}

// fetchInvoice GETs the invoice by id, used by both Verify and Webhook (the
// latter refetches rather than trusting webhook body fields — see file doc
// comment).
func (p *BTCPayProvider) fetchInvoice(ctx context.Context, invoiceID string) (btcpayInvoice, error) {
	respBody, status, err := p.do(ctx, http.MethodGet, "/api/v1/stores/"+url.PathEscape(p.storeID)+"/invoices/"+url.PathEscape(invoiceID), nil)
	if err != nil {
		return btcpayInvoice{}, err
	}
	if status < 200 || status >= 300 {
		return btcpayInvoice{}, classifyBTCPayError(status, respBody)
	}
	var inv btcpayInvoice
	if err := json.Unmarshal(respBody, &inv); err != nil {
		return btcpayInvoice{}, fmt.Errorf("%w: %v", ErrBTCPayMalformedResponse, err)
	}
	if inv.ID == "" {
		return btcpayInvoice{}, fmt.Errorf("%w: empty invoice id", ErrBTCPayMalformedResponse)
	}
	return inv, nil
}

// btcpayResultFromInvoice maps a BTCPay invoice's status/additionalStatus
// onto a Result, failing closed at every ambiguous combination. raw, if
// non-nil, is stored on the Result for audit purposes (the webhook's own
// raw body, when called from Webhook; Verify has no separate raw body to
// attach beyond the invoice JSON itself).
func btcpayResultFromInvoice(inv btcpayInvoice, raw []byte) (Result, error) {
	if raw == nil {
		b, err := json.Marshal(inv)
		if err == nil {
			raw = b
		}
	}
	result := Result{
		Provider:  ProviderNameBTCPay,
		Reference: inv.ID,
		EventID:   inv.ID, // one invoice settles at most once; the invoice id is a stable dedupe key
		Currency:  inv.Currency,
		Raw:       json.RawMessage(raw),
	}
	amountMinor, err := majorStringToMinor(inv.Amount, inv.Currency)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrBTCPayMalformedResponse, err)
	}
	result.AmountMinor = amountMinor

	switch inv.Status {
	case "Settled":
		switch inv.AdditionalStatus {
		case "", "None", "Marked", "PaidLate":
			result.Status = StatusPaid
			result.PaidAt = time.Now().UTC()
		case "PaidOver":
			return Result{}, ErrBTCPayOverpaid
		default:
			// Settled+PaidPartial (or anything else unrecognised) is an
			// inconsistent combination we don't trust enough to call paid.
			return Result{}, fmt.Errorf("%w: status=Settled additionalStatus=%q", ErrBTCPayInconsistentStatus, inv.AdditionalStatus)
		}
	case "New", "Processing":
		result.Status = StatusPending
	case "Expired", "Invalid":
		// Covers both a genuinely expired quote and an underpaid invoice
		// that never completed within its window — neither ever settles.
		result.Status = StatusFailed
	default:
		// Fail closed: an unrecognised status is never treated as paid.
		result.Status = StatusFailed
	}
	return result, nil
}

// Webhook validates BTCPay's HMAC-SHA256 signature (header BTCPay-Sig,
// "sha256=<hex>", computed over the raw request body with the configured
// webhook secret), then — rather than trust any settlement fields that may
// or may not be present in the webhook payload itself — refetches the
// invoice from BTCPay's API and reports ITS authoritative status. This
// means a forged webhook POST can, at worst, trigger one extra authenticated
// GET; it can never fabricate a settlement.
func (p *BTCPayProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	sigHeader := strings.TrimSpace(r.Header.Get("BTCPay-Sig"))
	if sigHeader == "" {
		return Result{}, ErrBTCPayMissingSignature
	}
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return Result{}, fmt.Errorf("%w: expected \"sha256=<hex>\" format", ErrBTCPayInvalidSignature)
	}
	hexSig := strings.TrimPrefix(sigHeader, "sha256=")

	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrBTCPayMalformedResponse)
	}
	body, err := boundedRead(r.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrBTCPayResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: btcpay: read webhook body: %w", err)
	}

	given, err := hex.DecodeString(hexSig)
	if err != nil {
		return Result{}, fmt.Errorf("%w: signature is not valid hex", ErrBTCPayInvalidSignature)
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, given) {
		return Result{}, ErrBTCPayInvalidSignature
	}

	var envelope struct {
		Type      string `json:"type"`
		InvoiceID string `json:"invoiceId"`
		StoreID   string `json:"storeId"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrBTCPayMalformedResponse, err)
	}
	if !strings.HasPrefix(envelope.Type, "Invoice") {
		// A validly-signed webhook for an event type this build doesn't
		// treat as invoice-related at all (defined in provider.go — see
		// its doc comment for how callers should handle this: ack 200, do
		// nothing).
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Type)
	}
	if envelope.InvoiceID == "" {
		return Result{}, fmt.Errorf("%w: missing invoiceId", ErrBTCPayMalformedResponse)
	}

	inv, err := p.fetchInvoice(ctx, envelope.InvoiceID)
	if err != nil {
		return Result{}, err
	}
	return btcpayResultFromInvoice(inv, body)
}

// do issues an authenticated Greenfield API request, bounding it with
// cryptoDefaultHTTPTimeout regardless of the caller's own context, and caps
// the response body it reads.
func (p *BTCPayProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, cryptoDefaultHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: btcpay: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: btcpay: build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: btcpay: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrBTCPayResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: btcpay: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// classifyBTCPayError builds an error for a non-2xx BTCPay response,
// best-effort including BTCPay's own message without ever including
// request headers or the API key.
func classifyBTCPayError(status int, body []byte) error {
	var env struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &env) // best-effort; body may not be JSON
	msg := env.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrBTCPayUnexpectedStatus, status, msg)
}
