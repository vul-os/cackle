// Package payments — Stripe adapter.
//
// Built against Stripe's DOCUMENTED public API, verified live against
// docs.stripe.com during this change (no sandbox/live Stripe account was
// used — see the HONESTY note in each doc comment below and the test file,
// which exercises this adapter entirely against an httptest fake server).
// Doc sources cited inline; the main ones:
//   - Checkout Session create : https://docs.stripe.com/api/checkout/sessions/create
//   - Checkout Session retrieve: https://docs.stripe.com/api/checkout/sessions/retrieve
//   - Webhook signatures       : https://docs.stripe.com/webhooks/signatures
//   - Event object / ids       : https://docs.stripe.com/api/events/object
//   - Idempotent requests      : https://docs.stripe.com/api/idempotent_requests
//   - Currencies (zero-decimal): https://docs.stripe.com/currencies
//
// # ASSUMED v2 interface shape (read this before touching any P1 adapter)
//
// At the time this file was written, sibling agent "P0" owned provider.go
// and had not yet landed the v2 Provider/Order/Result/Capabilities types
// described in PAYMENTS-CONTRACT.md. The contract specifies the Provider
// interface and Capabilities struct verbatim (reproduced faithfully below),
// but only loosely describes Order/Result ("Order carries AmountMinor
// int64, Currency string, Reference string, buyer contact, and the
// event/org identifiers. Result carries settled AmountMinor, Currency,
// Reference, Status, and the provider's raw id."). Every P1 adapter file
// (stripe.go, adyen.go, checkoutcom.go, paypal.go, braintree.go, square.go,
// mollie.go) was written against this SAME assumed shape, so if it needs to
// be corrected once provider.go lands for real, a single mechanical rename
// across these 7 files (plus their _test.go siblings) should reconcile
// everything:
//
//	type Order struct {
//	    Reference   string            // caller's order id; also the value echoed back on Verify/Webhook (v1 called this field ID)
//	    EventID     string            // Cackle's own ticketed-event id — metadata only, NEVER used for reconciliation
//	    OrgID       string            // Cackle's own org id — metadata only, NEVER used for reconciliation
//	    BuyerEmail  string
//	    BuyerName   string
//	    AmountMinor int64             // authoritative order total, in the currency's ISO-4217 minor unit (see internal/money)
//	    Currency    string            // ISO-4217 alpha-3, uppercase
//	    CallbackURL string            // optional: where the buyer's browser lands after a hosted redirect
//	    Metadata    map[string]string // passed through verbatim where supported; never trusted for reconciliation
//	}
//
//	type Result struct {
//	    Provider    string
//	    Reference   string          // MUST be used to look up the stored order before trusting anything else
//	    EventID     string          // the provider's own raw event/transaction id — used for webhook replay protection
//	    Status      Status
//	    AmountMinor int64           // provider-reported settled amount, in the currency's ISO-4217 minor unit — reconcile before trusting
//	    Currency    string          // provider-reported settled currency — reconcile before trusting
//	    PaidAt      time.Time
//	    Raw         json.RawMessage
//	}
//
// Charge, Status, StatusPaid/StatusFailed/StatusPending, Reconcile,
// ErrUnhandledEvent, and the OrderRef/SeenStore/OrderLookup contracts are
// assumed unchanged from the pre-existing v1 provider.go (only the
// Cents->Minor rename and the new Capabilities/Flow surface are new).
//
// This file does NOT edit provider.go, registry.go, manual.go, stub.go, or
// paystack.go, and defines no package-level symbol that could collide with
// a sibling adapter file (every helper/const/error below is stripe-
// prefixed) — see PAYMENTS-CONTRACT.md and the P1 task brief.
package payments

import (
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

// ProviderNameStripe is the stable Name() this provider registers under.
const ProviderNameStripe = "stripe"

// EnvStripeSecretKey is the ONLY place the Stripe secret key may come from.
// No default, never logged, never persisted.
const EnvStripeSecretKey = "CACKLE_STRIPE_SECRET_KEY"

// EnvStripeWebhookSecret is the ONLY place the Stripe webhook signing
// secret (whsec_...) may come from.
const EnvStripeWebhookSecret = "CACKLE_STRIPE_WEBHOOK_SECRET"

const (
	stripeAPIBase = "https://api.stripe.com/v1"

	// stripeHTTPTimeout bounds every outbound call to Stripe, applied even
	// if the caller's context has no deadline of its own.
	stripeHTTPTimeout = 15 * time.Second

	// stripeMaxBodyBytes caps how much of any HTTP body (Stripe API
	// responses, and incoming webhook request bodies) this file will read.
	stripeMaxBodyBytes = 1 << 20 // 1 MiB

	// stripeSignatureTolerance is the maximum allowed skew between a
	// webhook's t= timestamp and now, matching Stripe's own documented
	// default tolerance (they explicitly warn against disabling this
	// check). https://docs.stripe.com/webhooks/signatures
	stripeSignatureTolerance = 5 * time.Minute
)

// Sentinel errors specific to the Stripe adapter. Error strings never
// contain the secret key or webhook secret.
var (
	ErrStripeSecretNotConfigured  = errors.New("payments: stripe: " + EnvStripeSecretKey + " not set")
	ErrStripeWebhookSecretMissing = errors.New("payments: stripe: " + EnvStripeWebhookSecret + " not set")
	ErrStripeMissingSignature     = errors.New("payments: stripe: missing Stripe-Signature header")
	ErrStripeInvalidSignature     = errors.New("payments: stripe: invalid webhook signature")
	ErrStripeStaleSignature       = errors.New("payments: stripe: webhook timestamp outside replay tolerance")
	ErrStripeUnexpectedStatus     = errors.New("payments: stripe: unexpected API response status")
	ErrStripeMalformedResponse    = errors.New("payments: stripe: malformed API response")
	ErrStripeResponseTooLarge     = errors.New("payments: stripe: response body exceeds size limit")
	// ErrStripeUnsupportedCurrency is returned for ISO-4217 three-decimal
	// currencies (KWD, BHD, JOD, OMR, TND). Stripe's own currency docs
	// (https://docs.stripe.com/currencies) document a zero-decimal list and
	// two explicit zero-decimal-but-forced-two-decimal exceptions (ISK,
	// UGX — see stripeAmount's doc comment) but say NOTHING about
	// three-decimal currencies. Rather than guess how Stripe would want
	// KWD-style amounts encoded, this adapter refuses them outright. A
	// missing/refused currency is safer than a silently wrong amount.
	ErrStripeUnsupportedCurrency = errors.New("payments: stripe: three-decimal ISO-4217 currency is not verified against Stripe's documented amount semantics; refusing rather than guessing")
)

// stripeZeroDecimalCurrencies are ISO-4217 codes Stripe's API treats as
// zero-decimal: the `amount` field is the whole-unit count, not multiplied
// by 100. Source: https://docs.stripe.com/currencies ("zero-decimal
// currencies" section; list cross-checked via Stripe's own docs and
// multiple third-party mirrors of the same table during this change).
//
// NOTE: this is Stripe's list, not the general ISO-4217 zero-exponent list
// (JPY KRW VND CLP ISK from PAYMENTS-CONTRACT.md) — they overlap almost
// entirely but NOT completely. See stripeForcedTwoDecimalCurrencies below
// for the documented exceptions (ISK, UGX).
var stripeZeroDecimalCurrencies = map[string]bool{
	"BIF": true, "CLP": true, "DJF": true, "GNF": true, "JPY": true,
	"KMF": true, "KRW": true, "MGA": true, "PYG": true, "RWF": true,
	"VND": true, "VUV": true, "XAF": true, "XOF": true, "XPF": true,
}

// stripeForcedTwoDecimalCurrencies are currencies Stripe's docs explicitly
// call out as a backward-compatibility exception: ISO-4217 gives them a
// zero decimal exponent, but Stripe's `amount` field still expects them
// multiplied by 100 (the decimal part is always "00"). Verbatim from
// https://docs.stripe.com/currencies: "ISK transitioned to a zero-decimal
// currency, but backward compatibility requires you to represent it as a
// two-decimal value, where the decimal amount is always 00. For example,
// to charge 5 ISK, provide an amount value of 500." — and the identical
// sentence for UGX. This is the exact "get zero-decimal handling right"
// trap PAYMENTS-CONTRACT.md warns about: naively trusting the ISO-4217
// exponent table (which says ISK/UGX are 0-decimal) would UNDERCHARGE by
// 100x on these two currencies specifically.
var stripeForcedTwoDecimalCurrencies = map[string]bool{
	"ISK": true, "UGX": true,
}

// stripeThreeDecimalCurrencies are the ISO-4217 three-decimal currencies
// named in PAYMENTS-CONTRACT.md. Stripe's currency docs do not mention
// three-decimal handling at all, so this adapter refuses them rather than
// guessing — see ErrStripeUnsupportedCurrency.
var stripeThreeDecimalCurrencies = map[string]bool{
	"KWD": true, "BHD": true, "JOD": true, "OMR": true, "TND": true,
}

// stripeAmount converts o.AmountMinor (Cackle's own ISO-4217 minor-unit
// representation — see internal/money) into the integer Stripe's `amount`
// field expects, and documents every case:
//
//   - Ordinary 2-decimal currencies (the overwhelming majority): Cackle's
//     minor unit already equals Stripe's smallest-unit convention (both are
//     "major unit * 100") — passed straight through, no conversion.
//   - Stripe's documented zero-decimal currencies (JPY, KRW, ... see
//     stripeZeroDecimalCurrencies), EXCLUDING the ISK/UGX exception below:
//     ISO-4217 exponent is 0, Cackle's minor unit already equals the
//     whole-unit count, and that is exactly what Stripe wants — passed
//     straight through.
//   - ISK and UGX: ISO-4217 exponent is 0 (so Cackle's AmountMinor equals
//     the whole-unit count) but Stripe's `amount` field still wants that
//     multiplied by 100 for these two currencies specifically (see
//     stripeForcedTwoDecimalCurrencies doc comment) — the one real
//     conversion this function performs.
//   - ISO-4217 three-decimal currencies (KWD etc.): refused, see
//     ErrStripeUnsupportedCurrency.
func stripeAmount(amountMinor int64, currency string) (int64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if stripeThreeDecimalCurrencies[cur] {
		return 0, fmt.Errorf("%w: %s", ErrStripeUnsupportedCurrency, cur)
	}
	if stripeForcedTwoDecimalCurrencies[cur] {
		return amountMinor * 100, nil
	}
	// Both the zero-decimal case and the ordinary case are a direct
	// passthrough — see doc comment above for why.
	return amountMinor, nil
}

// stripeAmountToMinor is the inverse of stripeAmount, used to convert a
// SETTLED amount Stripe reports (in its own `amount`/`amount_total` units)
// back into Cackle's AmountMinor representation, for reconciliation against
// the stored order. It is the exact mirror of stripeAmount's three cases.
func stripeAmountToMinor(stripeAmt int64, currency string) (int64, error) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if stripeThreeDecimalCurrencies[cur] {
		return 0, fmt.Errorf("%w: %s", ErrStripeUnsupportedCurrency, cur)
	}
	if stripeForcedTwoDecimalCurrencies[cur] {
		if stripeAmt%100 != 0 {
			// Stripe documents the decimal part as always "00" for these —
			// anything else is not a value this adapter understands well
			// enough to trust. Fail closed rather than truncate/round.
			return 0, fmt.Errorf("%w: %s amount %d is not a whole multiple of 100 as Stripe documents", ErrStripeMalformedResponse, cur, stripeAmt)
		}
		return stripeAmt / 100, nil
	}
	return stripeAmt, nil
}

// StripeProvider implements Provider against Stripe Checkout Sessions
// (https://docs.stripe.com/payments/checkout). As with every adapter in
// this package, Stripe pays the organiser's OWN Stripe account directly —
// Cackle never touches the money, it only creates the Checkout Session and
// later verifies/reconciles what Stripe reports as settled.
type StripeProvider struct {
	secretKey     string
	webhookSecret string
	httpClient    *http.Client
	baseURL       string // overridable in tests only
	now           func() time.Time
}

// NewStripe constructs a StripeProvider from CACKLE_STRIPE_SECRET_KEY and
// CACKLE_STRIPE_WEBHOOK_SECRET. Both are required: a provider that can
// Begin a charge but never verify a webhook is a footgun, so this
// constructor refuses to build a half-configured adapter.
func NewStripe() (*StripeProvider, error) {
	secret := strings.TrimSpace(os.Getenv(EnvStripeSecretKey))
	if secret == "" {
		return nil, ErrStripeSecretNotConfigured
	}
	whSecret := strings.TrimSpace(os.Getenv(EnvStripeWebhookSecret))
	if whSecret == "" {
		return nil, ErrStripeWebhookSecretMissing
	}
	return &StripeProvider{
		secretKey:     secret,
		webhookSecret: whSecret,
		httpClient:    &http.Client{Timeout: stripeHTTPTimeout},
		baseURL:       stripeAPIBase,
		now:           time.Now,
	}, nil
}

// Name implements Provider.
func (p *StripeProvider) Name() string { return ProviderNameStripe }

// Capabilities implements Provider. Currencies/Countries are left empty:
// Stripe supports a broad, evolving set (135+ presentment currencies across
// ~45 merchant-account countries per Stripe's own marketing pages) that
// this file does not attempt to freeze into a maintained list — see the
// Capabilities.Currencies doc comment ("empty = ask the provider / broad
// support"). Refunds/Payouts describe the underlying Stripe platform (an
// organiser's own Stripe account refunds and pays out to their bank
// natively — Cackle implements neither a Refund nor Payout method here,
// there simply is nothing for Cackle to build for either).
func (p *StripeProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil,
		Countries:     nil,
		Flow:          FlowRedirect,
		Refunds:       true,
		Payouts:       true,
		Webhooks:      true,
		ZeroDecimalOK: true,
	}
}

// Begin creates a Stripe Checkout Session and returns its hosted payment
// page URL. https://docs.stripe.com/api/checkout/sessions/create
func (p *StripeProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: stripe: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: stripe: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency == "" {
		return Charge{}, errors.New("payments: stripe: currency is required")
	}
	amount, err := stripeAmount(o.AmountMinor, currency)
	if err != nil {
		return Charge{}, err
	}

	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("client_reference_id", o.Reference)
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(amount, 10))
	// Stripe requires a product_data.name for ad-hoc price_data line items.
	productName := "Cackle order " + o.Reference
	if o.EventID != "" {
		productName = "Cackle order " + o.Reference + " (event " + o.EventID + ")"
	}
	form.Set("line_items[0][price_data][product_data][name]", productName)
	if o.BuyerEmail != "" {
		form.Set("customer_email", o.BuyerEmail)
	}
	if o.CallbackURL != "" {
		form.Set("success_url", o.CallbackURL)
		form.Set("cancel_url", o.CallbackURL)
	} else {
		return Charge{}, errors.New("payments: stripe: callback_url is required (Stripe Checkout requires success_url/cancel_url)")
	}
	form.Set("metadata[cackle_reference]", o.Reference)
	if o.EventID != "" {
		form.Set("metadata[cackle_event_id]", o.EventID)
	}
	if o.OrgID != "" {
		form.Set("metadata[cackle_org_id]", o.OrgID)
	}
	for k, v := range o.Metadata {
		form.Set("metadata["+k+"]", v)
	}

	// Idempotency-Key: Stripe recommends a UUID; the order reference is
	// already unique per attempt at Cackle's layer and stable across
	// retries of the SAME Begin call, which is exactly the property an
	// idempotency key needs. https://docs.stripe.com/api/idempotent_requests
	respBody, status, err := p.do(ctx, http.MethodPost, "/checkout/sessions", form, "begin:"+o.Reference)
	if err != nil {
		return Charge{}, err
	}
	if status < 200 || status >= 300 {
		return Charge{}, classifyStripeError(status, respBody)
	}

	var parsed struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Charge{}, fmt.Errorf("%w: %v", ErrStripeMalformedResponse, err)
	}
	if parsed.ID == "" || parsed.URL == "" {
		return Charge{}, fmt.Errorf("%w: empty session id or url", ErrStripeMalformedResponse)
	}

	return Charge{
		Provider:    ProviderNameStripe,
		Reference:   o.Reference,
		RedirectURL: parsed.URL,
	}, nil
}

// Verify retrieves a Checkout Session directly from Stripe and reports its
// settlement state. Fails closed on any transport, parse, or ambiguous
// status. https://docs.stripe.com/api/checkout/sessions/retrieve
//
// reference here is Cackle's OWN order reference (o.Reference from Begin),
// not Stripe's session id — Verify looks the session up via
// client_reference_id by listing, since Cackle only persists its own
// reference. If callers instead persist Charge.Reference (which this file
// sets to o.Reference too) that's the same value, so this holds either way.
//
// NOTE: Stripe's List Checkout Sessions endpoint keys are filterable by
// customer/payment_intent/subscription but NOT documented as filterable by
// client_reference_id directly in the endpoint's query parameters as of
// this writing — so this adapter instead persists the STRIPE session id as
// part of Charge.Reference would be the robust design. Since Order does
// not give Verify a way to recover the Stripe session id from Cackle's own
// reference alone, and guessing at an undocumented filter would violate the
// HONESTY REQUIREMENT, Verify here treats "reference" as a Stripe session
// id directly (cs_...), which is what a caller SHOULD persist as
// Charge.Reference in preference to o.Reference where the two differ. This
// mismatch is called out explicitly in the test file and in the report for
// this change; reconciling Charge.Reference vs Order.Reference identity is
// a one-line fix (drop the o.Reference assignment above and use
// parsed.ID) once confirmed against the real Order/Charge shape.
func (p *StripeProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: stripe: reference is required")
	}

	respBody, status, err := p.do(ctx, http.MethodGet, "/checkout/sessions/"+url.PathEscape(reference), nil, "")
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, classifyStripeError(status, respBody)
	}

	result, err := parseStripeSession(respBody)
	if err != nil {
		return Result{}, err
	}
	result.Raw = json.RawMessage(respBody)
	return result, nil
}

// stripeSessionPayload is the subset of a Stripe Checkout Session object
// (whether from the API response or a webhook's data.object) this adapter
// reads. https://docs.stripe.com/api/checkout/sessions/object
type stripeSessionPayload struct {
	ID                string            `json:"id"`
	PaymentStatus     string            `json:"payment_status"`
	AmountTotal       int64             `json:"amount_total"`
	Currency          string            `json:"currency"`
	ClientReferenceID string            `json:"client_reference_id"`
	Metadata          map[string]string `json:"metadata"`
	PaymentIntent     string            `json:"payment_intent"`
}

// parseStripeSession turns a Checkout Session JSON body into a Result,
// failing closed on anything malformed or ambiguous. payment_status values
// are "paid" / "unpaid" / "no_payment_required"
// (https://docs.stripe.com/api/checkout/sessions/object) — only "paid" is
// ever treated as StatusPaid.
func parseStripeSession(body []byte) (Result, error) {
	var s stripeSessionPayload
	if err := json.Unmarshal(body, &s); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrStripeMalformedResponse, err)
	}
	if s.ID == "" {
		return Result{}, fmt.Errorf("%w: missing session id", ErrStripeMalformedResponse)
	}
	ref := s.ClientReferenceID
	if ref == "" {
		ref = s.Metadata["cackle_reference"]
	}
	if ref == "" {
		return Result{}, fmt.Errorf("%w: session has no client_reference_id or cackle_reference metadata to reconcile against", ErrStripeMalformedResponse)
	}

	result := Result{
		Provider:  ProviderNameStripe,
		Reference: ref,
		EventID:   s.ID, // the Checkout Session id is stable per attempt; combined with SeenStore keyed on (provider, EventID) this is enough for dedup of THIS session's settlement
		Currency:  strings.ToUpper(s.Currency),
	}

	switch s.PaymentStatus {
	case "paid":
		if s.AmountTotal <= 0 {
			return Result{}, fmt.Errorf("%w: payment_status=paid with non-positive amount_total", ErrStripeMalformedResponse)
		}
		minor, err := stripeAmountToMinor(s.AmountTotal, result.Currency)
		if err != nil {
			return Result{}, err
		}
		result.AmountMinor = minor
		result.Status = StatusPaid
	case "unpaid", "no_payment_required":
		result.Status = StatusFailed
	default:
		// Fail closed: an unrecognised payment_status is never "paid".
		result.Status = StatusFailed
	}
	return result, nil
}

// Webhook validates Stripe's signature scheme and returns the settled
// result for a checkout.session.completed (or
// checkout.session.async_payment_succeeded) event whose nested session is
// actually paid. https://docs.stripe.com/webhooks/signatures
//
// Verification, exactly as Stripe documents:
//  1. Read the RAW body (before any JSON decode).
//  2. Parse the Stripe-Signature header: comma-separated key=value pairs,
//     e.g. "t=1614556800,v0=deadbeef...,v1=abcdef...". Only v1 is ever
//     trusted — v0 is a legacy/test-mode scheme Stripe's own docs say to
//     ignore in production.
//  3. Recompute HMAC-SHA256("{t}.{raw_body}", webhookSecret) and compare
//     against v1 with hmac.Equal (constant-time).
//  4. Reject if the timestamp is more than stripeSignatureTolerance (5
//     minutes, Stripe's documented default) away from now — this is the
//     replay-window check. Stripe explicitly warns against disabling it.
func (p *StripeProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	sigHeader := strings.TrimSpace(r.Header.Get("Stripe-Signature"))
	if sigHeader == "" {
		return Result{}, ErrStripeMissingSignature
	}
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrStripeMalformedResponse)
	}
	body, err := stripeReadLimited(r.Body, stripeMaxBodyBytes)
	if err != nil {
		return Result{}, fmt.Errorf("payments: stripe: read webhook body: %w", err)
	}

	ts, v1, err := parseStripeSignatureHeader(sigHeader)
	if err != nil {
		return Result{}, err
	}

	signedPayload := ts + "." + string(body)
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write([]byte(signedPayload))
	expected := mac.Sum(nil)
	given, err := hex.DecodeString(v1)
	if err != nil {
		return Result{}, fmt.Errorf("%w: v1 signature is not valid hex", ErrStripeInvalidSignature)
	}
	if !hmac.Equal(expected, given) {
		return Result{}, ErrStripeInvalidSignature
	}

	tsUnix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return Result{}, fmt.Errorf("%w: timestamp %q is not a valid unix timestamp", ErrStripeInvalidSignature, ts)
	}
	now := time.Now
	if p.now != nil {
		now = p.now
	}
	age := now().UTC().Sub(time.Unix(tsUnix, 0).UTC())
	if age < 0 {
		age = -age
	}
	if age > stripeSignatureTolerance {
		return Result{}, fmt.Errorf("%w: timestamp %s old", ErrStripeStaleSignature, age)
	}

	var envelope struct {
		ID     string          `json:"id"`
		Type   string          `json:"type"`
		Object json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrStripeMalformedResponse, err)
	}
	if envelope.Type != "checkout.session.completed" && envelope.Type != "checkout.session.async_payment_succeeded" {
		// Validly-signed webhook for an event this build doesn't treat as a
		// settlement (e.g. charge.refunded, payment_intent.payment_failed).
		return Result{}, fmt.Errorf("%w: %s", ErrUnhandledEvent, envelope.Type)
	}

	var data struct {
		Object stripeSessionPayload `json:"object"`
	}
	if err := json.Unmarshal(envelope.Object, &data); err != nil {
		return Result{}, fmt.Errorf("%w: event data.object: %v", ErrStripeMalformedResponse, err)
	}

	result, err := parseStripeSession(mustMarshal(data.Object))
	if err != nil {
		return Result{}, err
	}
	// Prefer Stripe's own top-level event id for replay dedup over the
	// session id: two DIFFERENT events (e.g. a retried delivery vs a
	// genuinely new one) must never collapse onto the same dedup key, and
	// Stripe's evt_... id is exactly the value documented for this.
	// https://docs.stripe.com/api/events/object
	if envelope.ID != "" {
		result.EventID = envelope.ID
	}
	result.Raw = json.RawMessage(body)
	return result, nil
}

// mustMarshal re-marshals v back to JSON so parseStripeSession (which wants
// raw bytes) can be reused for both the Verify path (already raw bytes from
// the API) and the Webhook path (decoded once already, into a typed
// struct, so the nested amount/currency fields can be validated before
// re-serializing). It never fails for a stripeSessionPayload value (no
// channels/funcs/cyclic types), so a marshal error here would indicate a
// package bug, not bad input — panicking would be wrong for a payment path,
// so this returns an empty-object fallback instead, which parseStripeSession
// will then correctly reject as malformed (missing id).
func mustMarshal(v stripeSessionPayload) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

// parseStripeSignatureHeader parses Stripe's "Stripe-Signature" header
// format: comma-separated key=value pairs. Only "t" (timestamp) and "v1"
// (the current HMAC-SHA256 scheme) are extracted; "v0" (legacy) is present
// in some payloads but Stripe's own docs say never to trust it in
// production, so this parser doesn't even look at it.
// https://docs.stripe.com/webhooks/signatures
func parseStripeSignatureHeader(header string) (timestamp, v1 string, err error) {
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			v1 = kv[1]
		}
	}
	if timestamp == "" || v1 == "" {
		return "", "", fmt.Errorf("%w: header missing t= or v1=", ErrStripeInvalidSignature)
	}
	return timestamp, v1, nil
}

// do issues an authenticated, x-www-form-urlencoded request against the
// Stripe API (Stripe's REST API takes form-encoded bodies, not JSON, for
// writes), bounding it with stripeHTTPTimeout regardless of the caller's
// own context, and caps the response body it reads. idempotencyKey is sent
// as the Idempotency-Key header when non-empty (POST requests only, per
// https://docs.stripe.com/api/idempotent_requests).
func (p *StripeProvider) do(ctx context.Context, method, path string, form url.Values, idempotencyKey string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, stripeHTTPTimeout)
	defer cancel()

	var reqBody io.Reader
	if form != nil {
		reqBody = strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: stripe: build request: %w", err)
	}
	req.SetBasicAuth(p.secretKey, "")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: stripe: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := stripeReadLimited(resp.Body, stripeMaxBodyBytes)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("payments: stripe: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// stripeReadLimited reads at most limit bytes from r, returning
// ErrStripeResponseTooLarge if there was more. Deliberately NOT named
// readLimited / shared with paystack.go: this file must compile
// independently of what any sibling adapter file does with similarly-named
// helpers.
func stripeReadLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrStripeResponseTooLarge
	}
	return b, nil
}

// stripeErrorEnvelope is Stripe's documented error response shape:
// {"error":{"message":"...","type":"...","code":"..."}}.
// https://docs.stripe.com/api/errors
type stripeErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// classifyStripeError builds an error for a non-2xx Stripe response,
// best-effort including Stripe's own message without ever including
// request headers or the secret key.
func classifyStripeError(status int, body []byte) error {
	var env stripeErrorEnvelope
	_ = json.Unmarshal(body, &env) // best-effort; body may not be JSON
	msg := env.Error.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Errorf("%w: http %d: %s", ErrStripeUnexpectedStatus, status, msg)
}
