// Package payments defines Cackle's payment provider seam and the built-in
// providers that implement it.
//
// The central design constraint: Cackle never holds funds. The organiser's
// own payment account (Paystack merchant account, bank EFT details, etc)
// receives the money directly; Cackle only initiates the charge, records
// what happened, and verifies it out-of-band before issuing tickets. There
// is no escrow, no platform wallet, no "hold and release" step anywhere in
// this package.
//
// Providers are never hardcoded into callers. Everything goes through the
// Provider interface and is looked up by name from a Registry, so a
// self-hoster can plug in their own provider (a different gateway, a manual
// EFT-only flow, whatever) without touching anyone else's code.
//
// # Money representation
//
// Amounts are integer cents (subunits) throughout this package, matching
// the rest of Cackle's schema (money is never a float). Paystack's API
// takes amounts in the smallest currency subunit too, and for every
// currency Cackle currently supports (ZAR — South African cents) that
// subunit factor is 100, i.e. R1.00 == 100. That means Order.TotalCents
// and Result.AmountCents map onto Paystack's `amount` field with NO
// conversion required — see the comment on paystackAmountSubunitFactor in
// paystack.go for the one place this assumption is pinned down. This would
// NOT hold for a zero-decimal currency (Paystack itself has none today,
// but if Cackle ever adds a currency where major-unit == subunit, this
// package would need a per-currency table before trusting the 1:1 mapping
// blindly).
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
	"sort"
	"strings"
	"sync"
	"time"
)

// Status is the settlement state of a charge.
type Status string

const (
	// StatusPending means the charge was started but has not settled yet
	// (the buyer may still be on the payment page).
	StatusPending Status = "pending"
	// StatusPaid means the provider has confirmed funds settled. This is
	// the ONLY status that should ever cause tickets to be issued.
	StatusPaid Status = "paid"
	// StatusFailed covers abandoned, declined, reversed, or otherwise
	// unsuccessful charges.
	StatusFailed Status = "failed"
)

// Order is the minimal, storage-agnostic view of an order that a Provider
// needs to begin a charge. Callers (internal/orders, internal/httpapi)
// build this from their own persisted order; this package never touches
// the database itself.
type Order struct {
	// ID is the caller's own order identifier (a ULID in Cackle's schema).
	// It is used as the provider reference so a charge can always be
	// traced back to exactly one order, and reconciled against it later.
	ID string
	// EventID is the event the order belongs to, carried through as
	// provider metadata for support/audit purposes only — never trusted
	// for reconciliation.
	EventID string
	// BuyerEmail and BuyerName identify the buyer to the provider (e.g.
	// Paystack requires an email to initialize a transaction).
	BuyerEmail string
	BuyerName  string
	// TotalCents is the authoritative order total, in integer cents. This
	// is the amount that will later be reconciled against whatever the
	// provider reports as settled — see Reconcile.
	TotalCents int64
	// Currency is an ISO 4217 code, e.g. "ZAR". Empty defaults to "ZAR"
	// in providers that need a currency (Cackle's schema default).
	Currency string
	// CallbackURL is where the buyer's browser should land after leaving
	// the provider's hosted payment page. Optional.
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
	// instructions instead (manual EFT, the stub provider).
	RedirectURL string
	// Instructions is human-readable text for non-redirect flows (manual
	// EFT reference, PayShap instructions, "this is a demo" notices).
	Instructions string
}

// Result is what a Provider reports about a charge's settlement state,
// whether from Verify (polled) or Webhook (pushed).
type Result struct {
	// Provider is the provider's Name().
	Provider string
	// Reference is the order reference this result is about. Callers
	// MUST use this — not anything else in the request — to look up the
	// order before trusting AmountCents/Currency/Status. See Reconcile.
	Reference string
	// EventID is a provider-specific identifier that uniquely identifies
	// THIS settlement notification (e.g. a Paystack transaction id). It
	// is used for webhook replay protection — see SeenStore — and must
	// be non-empty whenever Status is StatusPaid.
	EventID string
	// Status is the settlement state. Only StatusPaid may ever result in
	// tickets being issued.
	Status Status
	// AmountCents is what the PROVIDER reports as settled, in integer
	// cents. This is provider-reported data, echoed back over the
	// network — it must be reconciled against the caller's own stored
	// order total (Reconcile) before being trusted for anything.
	AmountCents int64
	// Currency is what the provider reports settled in. Same caveat as
	// AmountCents: reconcile before trusting.
	Currency string
	// PaidAt is when the provider says the charge settled. Zero value if
	// unknown or not yet paid.
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
	// under (e.g. "paystack", "stub"). It is persisted in orders.provider
	// and used as the {provider} path segment in the webhook route.
	Name() string
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
	Webhook(ctx context.Context, r *http.Request) (Result, error)
}

// OrderRef is the minimal, storage-agnostic view of a STORED order that
// Reconcile checks a provider Result against. Callers construct this from
// their own database read — this package never queries storage itself.
type OrderRef struct {
	ID         string
	TotalCents int64
	Currency   string
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
	// exactly how ticketing platforms get robbed (pay R10, claim R1000)
	// — always reject rather than clamp or "trust the higher one".
	ErrAmountMismatch = errors.New("payments: settled amount does not match order total")
	// ErrCurrencyMismatch: settled currency differs from the order's
	// stored currency.
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
// amount/currency that match the order's own stored total exactly. Never
// call this with a "want" built from anything the client sent — want must
// come from the caller's own database read of the order, keyed by
// result.Reference.
func Reconcile(result Result, want OrderRef) error {
	if result.Reference == "" || result.Reference != want.ID {
		return fmt.Errorf("%w: result reference %q, order %q", ErrReferenceMismatch, result.Reference, want.ID)
	}
	if result.Status != StatusPaid {
		return fmt.Errorf("%w: status=%q", ErrNotPaid, result.Status)
	}
	if result.AmountCents != want.TotalCents {
		return fmt.Errorf("%w: provider reported %d, order total is %d", ErrAmountMismatch, result.AmountCents, want.TotalCents)
	}
	wantCurrency := want.Currency
	if wantCurrency == "" {
		wantCurrency = "ZAR"
	}
	gotCurrency := result.Currency
	if gotCurrency == "" {
		gotCurrency = "ZAR"
	}
	if !strings.EqualFold(gotCurrency, wantCurrency) {
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

// ErrUnhandledEvent is returned by a Provider's Webhook method (see
// paystack.go) for a validly-signed webhook whose event type this build
// does not treat as a settlement (e.g. Paystack's transfer.* events).
// Callers wiring the HTTP route should treat this specific error as "ack
// with 200, do nothing" — NOT as a hard failure — so the provider doesn't
// retry forever, while every other error from Webhook remains a hard
// failure that must not be acknowledged as success.
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

// Registry looks up providers by name so callers never hardcode one.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds p under p.Name(). It refuses a nil provider, an empty
// name, a duplicate name, and — as defense in depth alongside the checks
// already inside NewStub — refuses to register anything named
// ProviderNameStub if a real Paystack secret is configured in this
// environment, so the demo/test provider can't end up live even if
// something upstream constructed it incorrectly.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return errors.New("payments: cannot register a nil provider")
	}
	name := p.Name()
	if strings.TrimSpace(name) == "" {
		return errors.New("payments: provider Name() must not be empty")
	}
	if name == ProviderNameStub && realPaystackSecretConfigured() {
		return ErrStubRefusesRealSecret
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("payments: provider %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get looks up a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Names returns the registered provider names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
