package payments

// LNbits / Lightning adapter — small amounts, instant settlement, well
// suited to on-site/at-the-door spend (see the payments contract's ordering:
// this is priority 2, right after BTCPay).
//
// LNbits (https://lnbits.com, self-hostable, MIT-licensed) is a thin
// wallet/accounts layer in front of a Lightning node (or its own funding
// source).
// Like BTCPay, an organiser runs their own LNbits instance (or wallet on a
// host they trust) against their OWN node/channels — Cackle never custodies
// sats or holds any Lightning channel state.
//
// Built against LNbits' documented core Payments API:
//   - API guide:  https://legend.lnbits.com/guide/api.html
//   - Source / OpenAPI spec: https://github.com/lnbits/lnbits
//
// Confidence: HIGH for creating a fixed-amount invoice
// (POST /api/v1/payments, {"out":false,"amount":<sats>,"memo":...} ->
// {"payment_hash":...,"payment_request":<bolt11>}) and for polling status
// (GET /api/v1/payments/{payment_hash} -> {"paid": bool, ...}) — these are
// the oldest, most stable parts of LNbits' core API and are widely used in
// third-party integrations. MODERATE confidence on the fiat-denominated
// `unit`/`amount` request fields (LNbits can price an invoice directly in a
// fiat currency and convert to sats itself using its own rate source, but
// this has evolved across LNbits versions and this author is not fully
// certain of the exact field names in your specific LNbits version) — this
// is called out again below. This adapter is unit-tested only (no sandbox
// LNbits instance was available) — see lnbits_test.go.
//
// # No webhook signature — a compensating control, not a guess
//
// Unlike BTCPay/Paystack, LNbits' native webhook delivery (the `webhook`
// field on invoice creation) has no built-in cryptographic signature at
// all — it just POSTs the payment object to the configured URL. Rather than
// guess at a signing scheme LNbits doesn't document, this adapter:
//  1. requires the webhook URL you register with LNbits to embed an
//     operator-chosen shared secret as a query parameter
//     (?secret=<CACKLE_LNBITS_WEBHOOK_SECRET>), checked in constant time;
//  2. NEVER trusts the webhook body for the actual settlement data — it
//     only extracts which payment_hash to ask about, then re-verifies via
//     LNbits' own authenticated GET /api/v1/payments/{hash} before
//     reporting a Result.
//
// A forged POST to the webhook URL therefore cannot fabricate a settlement:
// worst case, with the correct shared secret, it triggers one extra
// authenticated read that reports the true (unpaid) status.
//
// # Confirmations
//
// Not applicable. Lightning payments settle via HTLC the instant the
// recipient's node releases the preimage — there is no block-confirmation
// wait, which is exactly why the contract calls this out as suited to
// "on-site spend". This adapter has no confirmation-count configuration.
//
// # Underpayment / overpayment
//
// This adapter always creates FIXED-AMOUNT invoices (never LNbits'
// amountless-invoice mode). A BOLT11 invoice with a fixed amount settles for
// EXACTLY that amount or does not settle at all — a Lightning HTLC cannot
// partially capture or overpay a fixed-amount invoice. That protocol-level
// guarantee is what makes underpayment structurally impossible here, not
// extra logic in this file. LNbits' status endpoint only reports a paid
// bool (not a settled amount), so this adapter cannot independently
// re-detect an overpayment the way btcpay.go does — it relies on the
// Lightning protocol invariant above instead. If you need amountless
// invoices for some other reason, do not reuse this adapter for them.
//
// # FX / quote expiry
//
// This adapter records the fiat amount/currency it asked LNbits to invoice
// (see the in-memory invoiceRecord below) at Begin time, and a caller-
// configurable expiry window (CACKLE_LNBITS_QUOTE_TTL_SECONDS, default 900)
// is both requested as the BOLT11 invoice's own `expiry` and enforced
// independently by this adapter: once that window has passed with no
// payment, Verify/Webhook report StatusFailed rather than let a very late
// payment settle at a rate that may no longer be intended.
//
// # A real limitation, stated plainly (mitigated by NewLNbitsWithStore)
//
// Because LNbits' payment-status endpoint does not echo back the ORIGINAL
// fiat currency/amount an invoice was priced in (only its sat amount), this
// adapter has to remember that association itself. It does so with a
// small in-memory map keyed by payment_hash, populated in Begin and read in
// Verify/Webhook. Constructed via plain NewLNbits, this works only within a
// single running Cackle process: it does NOT survive a process restart or
// a multi-replica deployment behind a load balancer — a restart between
// Begin and Verify/Webhook will make this adapter report "unknown
// reference" (a fail-closed error, never a false settlement) rather than
// silently losing track of the fiat amount.
//
// Constructed via NewLNbitsWithStore instead, Begin persists that
// association to the supplied RecordStore, and Verify/Webhook fall back to
// it on an in-memory cache miss — so a restart (single replica) no longer
// loses the association. A genuine multi-replica deployment behind a load
// balancer still wants each replica's in-memory cache warmed from the same
// store, or requests for a given reference routed consistently — this
// adapter does not implement cross-replica cache invalidation itself.
import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProviderNameLNbits is the stable Name() this provider registers under.
const ProviderNameLNbits = "lnbits"

// Environment variables LNbitsProvider reads from.
const (
	EnvLNbitsBaseURL          = "CACKLE_LNBITS_BASE_URL"          // e.g. https://lnbits.example.com
	EnvLNbitsAPIKey           = "CACKLE_LNBITS_API_KEY"           // an invoice/read key (never an admin key)
	EnvLNbitsWebhookSecret    = "CACKLE_LNBITS_WEBHOOK_SECRET"    // Cackle's own compensating shared secret (see file doc comment)
	EnvLNbitsWebhookURL       = "CACKLE_LNBITS_WEBHOOK_URL"       // optional: registered with LNbits as the invoice's webhook target
	EnvLNbitsQuoteTTLSeconds  = "CACKLE_LNBITS_QUOTE_TTL_SECONDS" // optional, default 900 (15 minutes)
	lnbitsDefaultQuoteTTLSecs = 900
)

// Sentinel errors specific to the LNbits adapter.
var (
	ErrLNbitsNotConfigured     = errors.New("payments: lnbits: " + EnvLNbitsBaseURL + ", " + EnvLNbitsAPIKey + " and " + EnvLNbitsWebhookSecret + " must all be set")
	ErrLNbitsMissingSignature  = errors.New("payments: lnbits: missing ?secret= query parameter on webhook request")
	ErrLNbitsInvalidSignature  = errors.New("payments: lnbits: webhook ?secret= does not match " + EnvLNbitsWebhookSecret)
	ErrLNbitsUnexpectedStatus  = errors.New("payments: lnbits: unexpected API response status")
	ErrLNbitsMalformedResponse = errors.New("payments: lnbits: malformed API response")
	ErrLNbitsResponseTooLarge  = errors.New("payments: lnbits: response body exceeds size limit")
	// ErrLNbitsUnknownReference is returned by Verify/Webhook for a
	// payment_hash this process has no record of creating (see the file
	// doc comment's "real limitation" section) — fail closed rather than
	// guess the fiat amount/currency.
	ErrLNbitsUnknownReference = errors.New("payments: lnbits: unknown payment_hash (not created by this process, or the process restarted since)")
)

// lnbitsInvoiceRecord is what Begin remembers about an invoice it created,
// so Verify/Webhook can report the fiat amount LNbits itself doesn't track.
type lnbitsInvoiceRecord struct {
	amountMinor int64
	currency    string
	createdAt   time.Time
}

// LNbitsProvider implements Provider against a self-hosted (or
// self-operated) LNbits wallet. See the file-level doc comment for API
// references, the no-signature compensating control, and a stated
// limitation around the in-memory invoice record.
type LNbitsProvider struct {
	baseURL       string
	apiKey        string
	webhookSecret string
	webhookURL    string
	quoteTTL      time.Duration
	httpClient    *http.Client

	mu       sync.Mutex
	invoices map[string]lnbitsInvoiceRecord
	// store, if non-nil, makes the invoice/fiat-amount association this
	// provider creates in Begin survive a process restart — see
	// NewLNbitsWithStore. nil (the default from NewLNbits) preserves the
	// original in-memory-only behaviour documented above and exercised by
	// every existing test in this file.
	store RecordStore
}

// NewLNbits constructs an LNbitsProvider (in-memory-only invoice state —
// does not survive a process restart) from EnvLNbitsBaseURL,
// EnvLNbitsAPIKey and EnvLNbitsWebhookSecret (all required), plus the
// optional EnvLNbitsWebhookURL and EnvLNbitsQuoteTTLSeconds.
func NewLNbits() (*LNbitsProvider, error) {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv(EnvLNbitsBaseURL)), "/")
	key := strings.TrimSpace(os.Getenv(EnvLNbitsAPIKey))
	secret := strings.TrimSpace(os.Getenv(EnvLNbitsWebhookSecret))
	if base == "" || key == "" || secret == "" {
		return nil, ErrLNbitsNotConfigured
	}
	ttl := lnbitsDefaultQuoteTTLSecs
	if v := strings.TrimSpace(os.Getenv(EnvLNbitsQuoteTTLSeconds)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("payments: lnbits: %s must be a positive integer number of seconds", EnvLNbitsQuoteTTLSeconds)
		}
		ttl = n
	}
	return &LNbitsProvider{
		baseURL:       base,
		apiKey:        key,
		webhookSecret: secret,
		webhookURL:    strings.TrimSpace(os.Getenv(EnvLNbitsWebhookURL)),
		quoteTTL:      time.Duration(ttl) * time.Second,
		httpClient:    &http.Client{Timeout: cryptoDefaultHTTPTimeout},
		invoices:      make(map[string]lnbitsInvoiceRecord),
	}, nil
}

// NewLNbitsWithStore constructs an LNbitsProvider exactly like NewLNbits,
// then wires rs as its durability seam: every invoice Begin creates is
// persisted, and Verify/Webhook fall back to rs on an in-memory cache miss
// (e.g. after a restart) instead of failing closed with
// ErrLNbitsUnknownReference — turning this file's previously-documented
// "does not survive a restart" limitation into "survives a restart when rs
// is configured". Pass nil rs for NewLNbits' original behaviour.
func NewLNbitsWithStore(rs RecordStore) (*LNbitsProvider, error) {
	p, err := NewLNbits()
	if err != nil {
		return nil, err
	}
	p.store = rs
	return p, nil
}

// Name implements Provider.
func (p *LNbitsProvider) Name() string { return ProviderNameLNbits }

// Capabilities implements Provider.
func (p *LNbitsProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil, // whatever fiat currencies your LNbits instance's rate source supports
		Countries:     nil,
		Flow:          FlowInvoice,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: true,
	}
}

// Begin creates a fixed-amount BOLT11 invoice for the order and remembers
// its fiat amount/currency (see file doc comment) so Verify/Webhook can
// report it later.
func (p *LNbitsProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: lnbits: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: lnbits: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: lnbits: currency is required")
	}
	amountStr, err := minorToMajorString(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: lnbits: %w", err)
	}
	amountFloat, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: lnbits: could not render amount as a number: %w", err)
	}

	memo := "Cackle order " + o.Reference
	reqBody := map[string]any{
		"out":    false,
		"amount": amountFloat,
		"unit":   strings.ToLower(currency),
		"memo":   memo,
		"expiry": int(p.quoteTTL.Seconds()),
	}
	if p.webhookURL != "" {
		sep := "?"
		if strings.Contains(p.webhookURL, "?") {
			sep = "&"
		}
		reqBody["webhook"] = p.webhookURL + sep + "secret=" + url.QueryEscape(p.webhookSecret)
	}

	respBody, status, err := p.do(ctx, http.MethodPost, "/api/v1/payments", reqBody)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyLNbitsError(status, respBody)
	}

	var parsed struct {
		PaymentHash    string `json:"payment_hash"`
		PaymentRequest string `json:"payment_request"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrLNbitsMalformedResponse, err)
	}
	if parsed.PaymentHash == "" || parsed.PaymentRequest == "" {
		return Charge{}, fmt.Errorf("%w: empty payment_hash or payment_request", ErrLNbitsMalformedResponse)
	}

	now := time.Now().UTC()
	rec := lnbitsInvoiceRecord{
		amountMinor: o.AmountMinor,
		currency:    currency,
		createdAt:   now,
	}
	if p.store != nil {
		err := p.store.PutPaymentRecord(ctx, PaymentRecord{
			Provider:    ProviderNameLNbits,
			Reference:   parsed.PaymentHash,
			AmountMinor: rec.amountMinor,
			Currency:    rec.currency,
			Status:      StatusPending,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			// The LNbits invoice already exists on the node at this point
			// — that side effect can't be undone — but we must not hand
			// the buyer a reference this process (or a restart of it)
			// might later fail to recognise. Fail closed; the buyer
			// retries and gets a fresh invoice.
			return Charge{}, fmt.Errorf("payments: lnbits: persist invoice record: %w", err)
		}
	}

	p.mu.Lock()
	p.invoices[parsed.PaymentHash] = rec
	p.mu.Unlock()

	return Charge{
		Provider:  ProviderNameLNbits,
		Reference: parsed.PaymentHash,
		Instructions: fmt.Sprintf(
			"Pay this Lightning invoice within %s: %s",
			p.quoteTTL, parsed.PaymentRequest,
		),
	}, nil
}

// Verify polls LNbits for the payment's status. Fails closed if this
// process has no memory of creating reference (see file doc comment), if
// the quote has expired, or on any transport/parse error.
func (p *LNbitsProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: lnbits: reference is required")
	}
	record, ok, err := p.lookupInvoice(ctx, reference)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, ErrLNbitsUnknownReference
	}
	return p.verifyAgainstRecord(ctx, reference, record)
}

// lookupInvoice checks the in-memory cache first, then — if a RecordStore
// is configured — falls back to it on a cache miss (e.g. this process
// restarted since Begin created the invoice), repopulating the cache so
// subsequent lookups on this replica don't need another store round trip.
func (p *LNbitsProvider) lookupInvoice(ctx context.Context, reference string) (lnbitsInvoiceRecord, bool, error) {
	p.mu.Lock()
	record, ok := p.invoices[reference]
	p.mu.Unlock()
	if ok {
		return record, true, nil
	}
	if p.store == nil {
		return lnbitsInvoiceRecord{}, false, nil
	}
	rec, ok, err := p.store.GetPaymentRecord(ctx, ProviderNameLNbits, reference)
	if err != nil {
		return lnbitsInvoiceRecord{}, false, fmt.Errorf("payments: lnbits: load persisted invoice: %w", err)
	}
	if !ok {
		return lnbitsInvoiceRecord{}, false, nil
	}
	record = lnbitsInvoiceRecord{
		amountMinor: rec.AmountMinor,
		currency:    rec.Currency,
		createdAt:   rec.CreatedAt,
	}
	p.mu.Lock()
	p.invoices[reference] = record
	p.mu.Unlock()
	return record, true, nil
}

func (p *LNbitsProvider) verifyAgainstRecord(ctx context.Context, reference string, record lnbitsInvoiceRecord) (Result, error) {
	respBody, status, err := p.do(ctx, http.MethodGet, "/api/v1/payments/"+url.PathEscape(reference), nil)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyLNbitsError(status, respBody)
	}

	var parsed struct {
		Paid bool `json:"paid"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrLNbitsMalformedResponse, err)
	}

	result := Result{
		Provider:    ProviderNameLNbits,
		Reference:   reference,
		EventID:     reference, // a BOLT11 invoice settles at most once; the payment_hash is a stable dedupe key
		AmountMinor: record.amountMinor,
		Currency:    record.currency,
		Raw:         json.RawMessage(respBody),
	}

	expired := time.Since(record.createdAt) > p.quoteTTL
	switch {
	case parsed.Paid:
		result.Status = StatusPaid
		result.PaidAt = time.Now().UTC()
	case expired:
		// The quote window has passed with no payment — never let a very
		// late payment settle later at a rate that's no longer intended.
		result.Status = StatusFailed
	default:
		result.Status = StatusPending
	}
	return result, nil
}

// Webhook requires the operator-configured shared secret in the request's
// ?secret= query parameter (constant-time compared — see file doc comment
// for why this exists instead of a cryptographic signature), then extracts
// ONLY the payment_hash from the body and re-verifies via LNbits' own API —
// it never trusts the webhook body for settlement data.
func (p *LNbitsProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	given := r.URL.Query().Get("secret")
	if given == "" {
		return Result{}, ErrLNbitsMissingSignature
	}
	if subtle.ConstantTimeCompare([]byte(given), []byte(p.webhookSecret)) != 1 {
		return Result{}, ErrLNbitsInvalidSignature
	}

	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrLNbitsMalformedResponse)
	}
	body, err := boundedRead(r.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrLNbitsResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: lnbits: read webhook body: %w", err)
	}

	var envelope struct {
		PaymentHash string `json:"payment_hash"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrLNbitsMalformedResponse, err)
	}
	if envelope.PaymentHash == "" {
		return Result{}, fmt.Errorf("%w: missing payment_hash", ErrLNbitsMalformedResponse)
	}

	record, ok, err := p.lookupInvoice(ctx, envelope.PaymentHash)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, ErrLNbitsUnknownReference
	}
	return p.verifyAgainstRecord(ctx, envelope.PaymentHash, record)
}

// do issues an authenticated LNbits API request, bounding it with
// cryptoDefaultHTTPTimeout and capping the response body it reads.
func (p *LNbitsProvider) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, cryptoDefaultHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("payments: lnbits: encode request: %w", err)
		}
		reqBody = strings.NewReader(string(b))
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: lnbits: build request: %w", err)
	}
	req.Header.Set("X-Api-Key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: lnbits: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrLNbitsResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: lnbits: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// classifyLNbitsError builds an error for a non-2xx LNbits response,
// best-effort including LNbits' own message without ever including
// request headers or the API key.
func classifyLNbitsError(status int, body []byte) error {
	var env struct {
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &env) // best-effort; body may not be JSON
	msg := env.Detail
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrLNbitsUnexpectedStatus, status, msg)
}
