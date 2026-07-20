// Package payments defines Cackle's payment provider seam and the built-in
// providers that implement it.
//
// The central design constraint: Cackle never holds funds. The organiser's
// own payment account (a card processor's merchant account, a bank
// account, a self-hosted BTCPay/LNbits node, whatever) receives the money
// directly; Cackle only initiates the charge, records what happened, and
// verifies it out-of-band before issuing tickets. There is no escrow, no
// platform wallet, no "hold and release" step anywhere in this package.
//
// Providers are never hardcoded into callers. Everything goes through the
// Provider interface and is looked up by name from a Registry (see
// registry.go), so a self-hoster can plug in their own provider (a
// different gateway, a bank-transfer-only flow, whatever) without touching
// anyone else's code. The DEFAULT provider is "manual" (see manual.go): no
// network calls, no API keys, works in every country — every other
// provider is an optional, off-by-default adapter on top of it.
//
// # Money representation
//
// Cackle is country- and currency-agnostic: there is no privileged
// country, currency, or processor. Amounts in this package are always an
// integer count of the currency's own minor units (AmountMinor) plus an
// ISO-4217 currency code — NEVER "cents", because that word is wrong for
// roughly a dozen real currencies (JPY/KRW/VND/CLP/ISK have zero decimal
// places, KWD/BHD/JOD/OMR/TND have three). The authoritative exponent
// table lives in internal/money; this package never assumes 2. Provider
// adapters are responsible for converting AmountMinor to/from whatever
// subunit convention their own API uses (most match ISO-4217 minor units,
// but check each one — see the per-adapter file for the citation).
//
// # Fail-closed discipline
//
// Every method on Provider fails closed: a transport error, a timeout, an
// unparseable response, a missing/invalid webhook signature, or an amount
// that doesn't reconcile against the stored order must never be reported
// as a successful payment. When in doubt, return an error. Ticketing
// platforms get robbed by treating an unverifiable "probably fine" as
// "paid" — this package is written to make that class of bug structurally
// hard to introduce.
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Status is the settlement state of a charge.
type Status string

const (
	// StatusPending means the charge was started but has not settled yet
	// (the buyer may still be on the payment page, or an organiser
	// hasn't marked a manual order paid yet).
	StatusPending Status = "pending"
	// StatusPaid means the provider (or, for the manual provider, the
	// organiser) has confirmed funds settled. This is the ONLY status
	// that should ever cause tickets to be issued.
	StatusPaid Status = "paid"
	// StatusFailed covers abandoned, declined, reversed, cancelled, or
	// otherwise unsuccessful charges.
	StatusFailed Status = "failed"
)

// Flow describes the shape of the buyer-facing interaction a Provider
// uses to collect payment. Callers (checkout UI) use this to decide how
// to present a provider — send the buyer somewhere, embed something
// inline, or just show instructions/an invoice.
type Flow string

const (
	// FlowRedirect sends the buyer's browser to a hosted payment page
	// (Charge.RedirectURL) and back via Order.CallbackURL.
	FlowRedirect Flow = "redirect"
	// FlowInline settles without leaving the checkout page (an embedded
	// widget, a direct API call the client makes itself, etc).
	FlowInline Flow = "inline"
	// FlowManual means there is no payment page or widget at all: the
	// buyer is shown instructions (bank details, pay-at-the-door, an
	// invoice reference) and an organiser later marks the order paid.
	// See manual.go — this is Cackle's default flow.
	FlowManual Flow = "manual"
	// FlowInvoice means the provider generates a payable invoice/request
	// (common for crypto and some B2B gateways) rather than a page or an
	// inline widget.
	FlowInvoice Flow = "invoice"
)

// Capabilities describes what a Provider can do, so callers (the
// Registry, checkout UI, org settings) can filter and validate providers
// without hardcoding per-provider knowledge anywhere else.
type Capabilities struct {
	// Currencies is the set of ISO-4217 codes this provider's configured
	// merchant account(s) can settle. Empty means "broad support / ask
	// the provider at Begin time" — callers should not treat an empty
	// slice as "supports everything" without also validating the
	// specific currency via internal/money and, where possible, the
	// provider's own error at Begin.
	Currencies []string
	// Countries is the set of ISO-3166-1 alpha-2 countries this
	// provider's merchant account(s) are registered in. Empty means
	// broad/unspecified.
	Countries []string
	// Flow is the buyer-facing interaction shape — see the Flow
	// constants.
	Flow Flow
	// Refunds reports whether this provider exposes a refund API
	// (whether or not Cackle has wired it up yet).
	Refunds bool
	// Payouts reports whether this provider can pay an organiser out
	// directly (transfers/recipients), independent of Cackle.
	Payouts bool
	// Webhooks reports whether Webhook is actually supported. false
	// means Webhook always returns an error (see manual.go and stub.go)
	// and callers must rely on Verify (polling) instead.
	Webhooks bool
	// ZeroDecimalOK reports whether this provider correctly handles
	// zero- and three-decimal currencies (JPY, KWD, ...) rather than
	// assuming every currency has two decimal places.
	ZeroDecimalOK bool
}

// SupportsCurrency reports whether code (case-insensitive, any
// whitespace-trimmed ISO-4217 code) is usable with this provider. An empty
// Currencies list is treated as "unrestricted" — callers still validate
// code itself via internal/money separately.
func (c Capabilities) SupportsCurrency(code string) bool {
	if len(c.Currencies) == 0 {
		return true
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, cur := range c.Currencies {
		if strings.EqualFold(cur, code) {
			return true
		}
	}
	return false
}

// SupportsCountry reports whether cc (case-insensitive ISO-3166-1 alpha-2)
// is usable with this provider. An empty Countries list is treated as
// "unrestricted".
func (c Capabilities) SupportsCountry(cc string) bool {
	if len(c.Countries) == 0 {
		return true
	}
	cc = strings.ToUpper(strings.TrimSpace(cc))
	for _, country := range c.Countries {
		if strings.EqualFold(country, cc) {
			return true
		}
	}
	return false
}

// Order is the minimal, storage-agnostic view of an order that a Provider
// needs to begin a charge. Callers (internal/orders, internal/httpapi)
// build this from their own persisted order; this package never touches
// the database itself.
type Order struct {
	// Reference is the caller's own order identifier (a ULID in Cackle's
	// schema). It is handed to the provider as ITS charge reference so a
	// charge can always be traced back to exactly one order, and
	// reconciled against it later — see Reconcile and OrderRef.ID, which
	// this must equal.
	Reference string
	// EventID is the event the order belongs to, carried through as
	// provider metadata for support/audit purposes only — never trusted
	// for reconciliation.
	EventID string
	// OrgID is the organiser's own identifier, also carried through as
	// metadata only — the org whose own payment account should receive
	// this money.
	OrgID string
	// BuyerEmail and BuyerName identify the buyer to the provider (e.g.
	// most hosted-page providers require an email to initialize a
	// transaction).
	BuyerEmail string
	BuyerName  string
	// AmountMinor is the authoritative order total, expressed in the
	// SMALLEST unit of Currency (its ISO-4217 minor unit — see
	// internal/money; this is NOT always "cents": 0 for JPY, 3 for KWD).
	// This is the amount that will later be reconciled against whatever
	// the provider reports as settled — see Reconcile.
	AmountMinor int64
	// Currency is an ISO-4217 alpha-3 code. Callers must set this
	// explicitly — Cackle has no privileged default currency; validate
	// with internal/money before constructing an Order.
	Currency string
	// CallbackURL is where the buyer's browser should land after leaving
	// the provider's hosted payment page. Optional; only meaningful for
	// FlowRedirect providers.
	CallbackURL string
	// Metadata is passed through to the provider verbatim where
	// supported, for support/debugging. Never used for reconciliation.
	Metadata map[string]string
}

// Charge is what a Provider hands back after Begin: enough for the caller
// to send the buyer to go pay, or to show them instructions.
type Charge struct {
	// Provider is the provider's Name(), so callers persisting this can
	// record which provider a given order used.
	Provider string
	// Reference is the value that will be echoed back on Verify and in
	// Webhook payloads. Callers must persist this against the order.
	Reference string
	// RedirectURL is a hosted payment page the buyer's browser should be
	// sent to. Empty for providers that settle inline or give
	// instructions instead (FlowManual, FlowInvoice, the stub provider).
	RedirectURL string
	// Instructions is human-readable text for non-redirect flows (manual
	// payment instructions, an invoice reference, "this is a demo"
	// notices).
	Instructions string
}

// Result is what a Provider reports about a charge's settlement state,
// whether from Verify (polled) or Webhook (pushed).
type Result struct {
	// Provider is the provider's Name().
	Provider string
	// Reference is the order reference this result is about. Callers
	// MUST use this — not anything else in the request — to look up the
	// order before trusting AmountMinor/Currency/Status. See Reconcile.
	Reference string
	// EventID is a provider-specific identifier that uniquely identifies
	// THIS settlement notification (e.g. a processor's transaction id).
	// It is used for webhook replay protection — see SeenStore — and
	// must be non-empty whenever Status is StatusPaid.
	EventID string
	// Status is the settlement state. Only StatusPaid may ever result in
	// tickets being issued.
	Status Status
	// AmountMinor is what the PROVIDER reports as settled, in the
	// currency's own minor units (see Order.AmountMinor). This is
	// provider-reported data, echoed back over the network — it must be
	// reconciled against the caller's own stored order total (Reconcile)
	// before being trusted for anything.
	AmountMinor int64
	// Currency is what the provider reports settled in. Same caveat as
	// AmountMinor: reconcile before trusting.
	Currency string
	// PaidAt is when the provider (or organiser, for manual) says the
	// charge settled. Zero value if unknown or not yet paid.
	PaidAt time.Time
	// Raw is the provider's raw response/webhook payload, kept for audit
	// logging. It is response data, not a secret — safe to log/store,
	// but callers should still avoid dumping it somewhere untrusted users
	// can read arbitrary provider internals.
	Raw json.RawMessage
}

// Provider is the payment seam every gateway integration implements.
// Cackle never hardcodes a provider anywhere outside a Registry lookup by
// name, so a self-hoster can add their own without touching shared code.
type Provider interface {
	// Name is the stable, lowercase identifier this provider registers
	// under (e.g. "manual", "stripe", "paystack"). It is persisted in
	// orders.provider and used as the {provider} path segment in the
	// webhook route.
	Name() string
	// Capabilities describes what this provider supports — see the
	// Capabilities doc comment. It must be safe to call at any time
	// (including before Begin) and MUST NOT make a network call.
	Capabilities() Capabilities
	// Begin returns a redirect URL (or inline instructions) for an order.
	// It must not mutate any state Cackle considers authoritative for
	// ticket issuance — issuance only happens after Verify or Webhook
	// reports StatusPaid.
	Begin(ctx context.Context, o Order) (Charge, error)
	// Verify confirms a reference out-of-band, by asking the provider
	// directly (never trusting anything the caller claims about the
	// order). Fail CLOSED on any transport, parse, or ambiguous-status
	// error: return an error rather than a guessed Result.
	Verify(ctx context.Context, reference string) (Result, error)
	// Webhook validates the provider's signature on r and returns the
	// settled result. Fail CLOSED: a missing or invalid signature, or a
	// malformed payload, must be an error — never a best-effort Result.
	// Providers whose Capabilities().Webhooks is false must always
	// return an error here (never silently succeed) — see manual.go and
	// stub.go.
	Webhook(ctx context.Context, r *http.Request) (Result, error)
}

// OrderRef is the minimal, storage-agnostic view of a STORED order that
// Reconcile checks a provider Result against. Callers construct this from
// their own database read — this package never queries storage itself.
type OrderRef struct {
	ID          string
	AmountMinor int64
	Currency    string
}

// Sentinel errors for Reconcile and the webhook/verify orchestration
// helpers below. Callers should match with errors.Is.
var (
	// ErrReferenceMismatch means the result's reference does not match
	// the order it was looked up against — this should be structurally
	// impossible if callers look orders up BY the result's reference, and
	// existing only as a defensive double-check.
	ErrReferenceMismatch = errors.New("payments: reference does not match order")
	// ErrAmountMismatch is the core anti-fraud check: the provider's
	// settled amount does not equal the order's stored total. This is
	// exactly how ticketing platforms get robbed (pay 10, claim 1000) —
	// always reject rather than clamp or "trust the higher one".
	ErrAmountMismatch = errors.New("payments: settled amount does not match order total")
	// ErrCurrencyMismatch: settled currency differs from the order's
	// stored currency. Cackle never treats two different currencies as
	// interchangeable, even ones that look numerically close.
	ErrCurrencyMismatch = errors.New("payments: settled currency does not match order currency")
	// ErrNotPaid: the provider result is not (yet, or no longer) a
	// successful settlement. Callers must not issue tickets.
	ErrNotPaid = errors.New("payments: provider result is not a settled payment")
	// ErrReplayed: a webhook event with this provider+event id has
	// already been processed once. Reported by HandleWebhook when a
	// SeenStore is supplied.
	ErrReplayed = errors.New("payments: webhook event already processed (replay)")
)

// Reconcile fails closed unless result is an actual settled payment for
// exactly the order described by want: same reference, StatusPaid, and an
// amount/currency that match the order's own stored total EXACTLY. Never
// call this with a "want" built from anything the client sent — want must
// come from the caller's own database read of the order, keyed by
// result.Reference.
//
// Unlike the pre-v2 version of this package, there is no implicit
// currency default here: Cackle is currency-agnostic, so an empty
// Currency on either side is compared literally (empty == empty), not
// silently treated as any particular currency. Callers are responsible
// for always populating a real ISO-4217 currency on both the stored order
// and the provider Result.
func Reconcile(result Result, want OrderRef) error {
	if result.Reference == "" || result.Reference != want.ID {
		return fmt.Errorf("%w: result reference %q, order %q", ErrReferenceMismatch, result.Reference, want.ID)
	}
	if result.Status != StatusPaid {
		return fmt.Errorf("%w: status=%q", ErrNotPaid, result.Status)
	}
	if result.AmountMinor != want.AmountMinor {
		return fmt.Errorf("%w: provider reported %d, order total is %d", ErrAmountMismatch, result.AmountMinor, want.AmountMinor)
	}
	if !strings.EqualFold(result.Currency, want.Currency) {
		return fmt.Errorf("%w: provider reported %q, order currency is %q", ErrCurrencyMismatch, result.Currency, want.Currency)
	}
	return nil
}

// SeenStore records whether a provider webhook event has already been
// processed, for replay protection. internal/store owns the actual
// persistence; this package only needs this minimal contract so it never
// has to import internal/store directly.
type SeenStore interface {
	// MarkSeen atomically records eventID as processed for provider and
	// reports whether this is the first time it has been seen (true), or
	// a replay (false). Implementations MUST be safe under concurrent
	// webhook delivery (e.g. a unique constraint + INSERT-or-detect, not
	// a racy read-then-write).
	MarkSeen(ctx context.Context, provider, eventID string) (firstSeen bool, err error)
}

// OrderLookup fetches the stored truth about an order by the reference a
// provider result claims to be about. internal/orders owns the actual
// query; this package only needs this minimal contract.
type OrderLookup interface {
	Lookup(ctx context.Context, reference string) (OrderRef, error)
}

// ErrUnhandledEvent is returned by a Provider's Webhook method for a
// validly-signed webhook whose event type this build does not treat as a
// settlement (e.g. a transfer.* event on a provider that also emits
// payout webhooks). Callers wiring the HTTP route should treat this
// specific error as "ack with 200, do nothing" — NOT as a hard failure —
// so the provider doesn't retry forever, while every other error from
// Webhook remains a hard failure that must not be acknowledged as
// success.
var ErrUnhandledEvent = errors.New("payments: unhandled webhook event type")

// HandleWebhook is the recommended orchestration for a webhook HTTP route:
// it validates the provider signature and payload (via p.Webhook), then
// enforces replay protection (if seen is non-nil) and reconciliation
// against the stored order (if lookup is non-nil), failing closed at every
// step. Both seen and lookup may be nil for narrow tests that only care
// about signature handling, but production wiring should always supply
// both.
func HandleWebhook(ctx context.Context, p Provider, r *http.Request, seen SeenStore, lookup OrderLookup) (Result, error) {
	result, err := p.Webhook(ctx, r)
	if err != nil {
		return Result{}, err
	}
	if err := checkReplay(ctx, seen, p.Name(), result); err != nil {
		return Result{}, err
	}
	if lookup != nil {
		want, err := lookup.Lookup(ctx, result.Reference)
		if err != nil {
			return Result{}, fmt.Errorf("payments: order lookup failed: %w", err)
		}
		if err := Reconcile(result, want); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

// HandleVerify is the recommended orchestration for the poll-based verify
// endpoint: it asks the provider directly (via p.Verify — never trusting
// anything the client sent beyond the reference to look up), then
// reconciles against the stored order if lookup is non-nil.
func HandleVerify(ctx context.Context, p Provider, reference string, lookup OrderLookup) (Result, error) {
	result, err := p.Verify(ctx, reference)
	if err != nil {
		return Result{}, err
	}
	if lookup != nil {
		want, err := lookup.Lookup(ctx, result.Reference)
		if err != nil {
			return Result{}, fmt.Errorf("payments: order lookup failed: %w", err)
		}
		if err := Reconcile(result, want); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

// checkReplay fails closed: if a SeenStore is configured, an event with no
// id, or one already marked seen, is rejected. A nil seen store is only
// acceptable for callers that implement replay protection some other way
// (or narrow tests) — production wiring should always pass one.
func checkReplay(ctx context.Context, seen SeenStore, provider string, result Result) error {
	if seen == nil {
		return nil
	}
	if result.Status != StatusPaid {
		// Only settled payments need replay protection; there is nothing
		// to double-spend on a non-paid result.
		return nil
	}
	if strings.TrimSpace(result.EventID) == "" {
		return fmt.Errorf("payments: refusing to process a paid webhook result with no event id (cannot dedupe)")
	}
	first, err := seen.MarkSeen(ctx, provider, result.EventID)
	if err != nil {
		return fmt.Errorf("payments: replay check failed: %w", err)
	}
	if !first {
		return fmt.Errorf("%w: provider=%s event=%s", ErrReplayed, provider, result.EventID)
	}
	return nil
}
