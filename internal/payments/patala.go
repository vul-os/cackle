//go:build patala

// This file is the ONLY place in Cackle that imports patala-go
// (github.com/vul-os/patala/patala-go/bindings/patala), the cgo Go binding
// over patala's Rust core — see the sibling patala repo's PATALA.md and
// patala-go/README.md. It exists so a real payment processor (Stripe,
// Paystack, Adyen, BTCPay, lnbits, and the rest of patala-fiat's 20
// adapters) can be served by patala's substrate instead of a hand-rolled
// Cackle adapter — see docs/PAYMENTS.md "The patala path" and ROADMAP.md's
// payments-migration entry.
//
// # Building this in
//
// The default Cackle build (`make build`, plain `go build`/`go test`)
// never compiles this file — no `//go:build patala` file is reachable
// without passing `-tags patala` explicitly, so the offline gate/scanner
// (internal/tickets, internal/scan) and every other pure-Go, CGO_ENABLED=0
// build stay completely unaffected. Building WITH this file requires all
// of:
//
//  1. The sibling patala repo checked out next to this one
//     (`../patala` relative to this module root — see go.mod's `replace`
//     directive) with its Go bindings generated against a cdylib built
//     with every fiat processor compiled in:
//
//     cd ../patala/patala-go && make FEATURES=fiat-all generate
//
//  2. `CGO_ENABLED=1` and a C toolchain (cc/clang/gcc) — cgo is mandatory
//     for anything that imports patala-go; see that package's README.md
//     "The cgo cost" section for the full, honest tradeoff (this breaks a
//     pure-Go static binary, requires a matching cross C toolchain to
//     cross-compile, etc).
//
//  3. `go build -tags patala ./cmd/... ./internal/...` (or `go test`),
//     with `CGO_LDFLAGS`/`DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH` pointed at
//     the generated `../patala/patala-go/bindings/patala/` directory,
//     exactly as patala-go's own Makefile does for its examples. See
//     Makefile's `build-patala`/`test-patala` targets in this repo for the
//     whole recipe as one command.
//
// If a self-hoster does not want cgo at all but still wants real
// processors, patala-sidecar (a loopback-only HTTP server over the same
// patala-core surface) is the documented alternative — see
// patala-go/README.md's "The cgo cost" section. Cackle does not integrate
// the sidecar today; this file is the cgo-binding path only.
//
// # manual stays native
//
// ProviderNameManual is never routed through this file, deliberately, for
// two independent reasons: (1) it needs no network, no config, and no cgo
// at all — routing it through patala would only add a dependency for zero
// benefit; (2) patala's generic FFI surface (PatalaRailNewFiat only
// exposes quote/charge/verify) cannot drive `ManualRail`'s
// `mark_paid`/`mark_failed` operator actions in the first place — those
// are inherent methods outside the `PaymentRail` trait UniFFI exports (see
// patala-go/README.md's "What a cackle consumer needs to know"). manual.go
// is untouched by this migration.
//
// # What this adapter can and cannot do
//
// patala_core::PaymentRail (the trait patala-fiat's 20 adapters implement,
// and the only thing PatalaRailNewFiat hands back) has exactly three
// methods reachable here: quote, charge, verify. It has NO webhook
// concept at all — webhook signature verification is provider-specific
// Rust code (patala-fiat's own `<provider>::webhook` modules) that is
// NOT part of the UniFFI-exported surface. So PatalaFiatProvider.Webhook
// below unconditionally fails — a patala-backed provider can only ever be
// confirmed by polling Verify, never by a provider's push webhook, until
// (unless) patala grows a webhook export. This is a real, current gap in
// what the binding exposes, not a Cackle shortcut — see docs/PAYMENTS.md.
//
// verify() also returns only a bool, never the amount/currency it actually
// observed at the provider — see (*PatalaFiatProvider).Verify's own doc
// comment for how this adapter still gets Cackle's full
// pay-10-claim-1000 anti-fraud guarantee out of that bool.
package payments

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	patala "github.com/vul-os/patala/patala-go/bindings/patala"
)

// ErrPatalaNoWebhook is returned unconditionally by
// PatalaFiatProvider.Webhook — see this file's module doc comment ("What
// this adapter can and cannot do").
var ErrPatalaNoWebhook = errors.New("payments: this provider settles via patala, which has no webhook surface through its Go binding yet -- poll Verify instead")

// patalaKeyOverrides documents the small number of cases where Cackle's
// existing CACKLE_<PROVIDER>_<SUFFIX> environment variable name does not
// literally lower-case into patala-fiat's own config map key for that
// provider (see patala-py/src/fiat.rs's build_<provider> functions, the
// authoritative key list this must match). Every other provider's keys
// happen to already match a plain lower-casing of Cackle's own suffix —
// see docs/PAYMENTS.md's "The patala path" table for the full mapping,
// verified by hand against fiat.rs.
var patalaKeyOverrides = map[string]map[string]string{
	"adyen":  {"HMAC_KEY": "hmac_key_hex"},
	"lnbits": {"QUOTE_TTL_SECONDS": "quote_ttl_secs"},
}

// PatalaConfigFromEnv builds patala-fiat's map[string]string config for
// provider from this deployment's CACKLE_<PROVIDER>_* environment
// variables, reusing every removed native adapter's own variable names
// verbatim (see docs/PAYMENTS.md's adapter table) so an operator moving
// onto this path does not need to rename anything. An empty return value
// means "no CACKLE_<PROVIDER>_* variable is set at all" — callers (see
// cmd/cackle/patala_register.go) treat that as "not configured", the same
// convention main.go already used for cfg.PaystackSecretKey != "".
//
// Fields patala-fiat also accepts but Cackle has no existing env var for
// (requires_kyc, currencies, settlement_days/settlement_seconds,
// timeout_secs) are simply left unset here — every build_<provider>
// function in patala-py/src/fiat.rs applies the same sane default
// patala-fiat's own from_env() would (see PORTING.md §4), so omitting them
// is correct, not a gap.
func PatalaConfigFromEnv(provider string) map[string]string {
	prefix := "CACKLE_" + strings.ToUpper(provider) + "_"
	overrides := patalaKeyOverrides[provider]
	cfg := make(map[string]string)
	for _, kv := range os.Environ() {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, prefix) {
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		suffix := strings.TrimPrefix(name, prefix)
		key := strings.ToLower(suffix)
		if overrides != nil {
			if mapped, ok := overrides[suffix]; ok {
				key = mapped
			}
		}
		cfg[key] = value
	}
	return cfg
}

// PatalaFiatProviderNames returns every processor name reachable via
// patala.PatalaRailNewFiat IN THIS SPECIFIC BUILD of the patala cdylib
// (i.e. whatever `FEATURES=...` it was generated with — see this file's
// build comment). Includes "manual", which callers should skip (see
// cmd/cackle/patala_register.go): Cackle always uses its own native
// manual.go instead.
func PatalaFiatProviderNames() []string {
	return patala.PatalaFiatProviders()
}

// patalaChargeProof is the JSON shape every hosted-checkout rail in
// patala-fiat embeds in Receipt.proof for its redirect URL (see e.g.
// patala-fiat/src/stripe/proof.rs's ChargeProof, which this mirrors
// field-for-field for the one field this generic adapter cares about).
// patala_core documents `proof` as fully opaque, so this is a best-effort,
// convention-based read, not a formal contract patala_core itself
// promises — a provider whose proof has no such key, or isn't JSON at
// all, simply leaves RedirectURL empty (see (*PatalaFiatProvider).Begin);
// that never affects correctness, only whether a checkout UI has a link
// to send the buyer to.
type patalaChargeProof struct {
	RedirectURL string `json:"redirect_url"`
}

// PatalaFiatProvider adapts ANY ONE of patala-fiat's processor rails
// (Stripe, Paystack, Adyen, BTCPay, lnbits, ...) to Cackle's Provider
// interface, via patala-go's single by-name constructor
// (patala.PatalaRailNewFiat). One Go type serves every provider
// patala-fiat ships — there is no per-provider Go code in this file,
// mirroring exactly why patala-py/src/fiat.rs itself chose one exported
// constructor over twenty typed ones (see that file's own module docs).
type PatalaFiatProvider struct {
	name  string
	rail  *patala.PatalaRail
	store RecordStore
}

// NewPatalaFiat builds a Cackle Provider for name (must be one of
// PatalaFiatProviderNames(), e.g. "stripe", "paystack", "btcpay") from
// this deployment's CACKLE_<NAME>_* environment variables (see
// PatalaConfigFromEnv). store is REQUIRED, unlike some native adapters'
// nil-store fallback (e.g. manual.go's NewManual): patala's Verify takes a
// whole Receipt back, not a bare reference string, so this adapter MUST
// persist enough state itself to reconstruct one later — see
// (*PatalaFiatProvider).Verify.
func NewPatalaFiat(name string, store RecordStore) (*PatalaFiatProvider, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, errors.New("payments: patala: provider name must not be empty")
	}
	if store == nil {
		return nil, fmt.Errorf("payments: patala provider %q requires a non-nil RecordStore (see NewPatalaFiat doc comment)", name)
	}
	cfg := PatalaConfigFromEnv(name)
	rail, err := patala.PatalaRailNewFiat(name, cfg)
	if err != nil {
		return nil, fmt.Errorf("payments: patala rail %q: %w", name, err)
	}
	return &PatalaFiatProvider{name: name, rail: rail, store: store}, nil
}

func (p *PatalaFiatProvider) Name() string { return p.name }

// Capabilities maps patala_core::RailCapabilities onto Cackle's own
// Capabilities shape. Several fields have no equivalent in either
// direction — see patala-fiat/PORTING.md §4 for the authoritative list of
// gaps this mirrors:
//   - Countries: RailCapabilities has no country field at all (dropped).
//   - Flow: RailCapabilities has no flow field either. Every processor
//     patala-fiat ships is CustodialReversible and, in Cackle's own
//     original per-provider adapters, overwhelmingly FlowRedirect (a
//     hosted checkout page) — Razorpay was the one native exception
//     (FlowInline, a Checkout.js widget). This generic adapter cannot
//     distinguish that per-provider nuance through patala's coarser
//     surface, so it reports FlowRedirect for every provider; a checkout
//     UI driving a patala-backed "razorpay" should be aware this is an
//     approximation, disclosed here rather than silently assumed.
//   - Refunds: patala_core::PaymentRail does have a refund() method, but
//     this adapter does not call it (Cackle's Provider interface has no
//     Refund method at all to expose it through) — always false here.
//   - Webhooks: always false — see ErrPatalaNoWebhook and this file's
//     module doc comment.
//   - ZeroDecimalOK: always true. Every patala-fiat adapter routes its
//     amount conversion through patala-fiat's own currency table
//     (PORTING.md §8), which is the whole point of this migration.
func (p *PatalaFiatProvider) Capabilities() Capabilities {
	caps := p.rail.Capabilities()
	return Capabilities{
		Currencies:    caps.Currencies,
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      false,
		ZeroDecimalOK: true,
	}
}

// Begin maps cackle's Order onto patala_core::PayRequest and calls
// Charge. See docs/PAYMENTS.md "The patala path" for the
// PayRequest.Destination reinterpretation this relies on: every
// hosted-checkout patala-fiat rail treats `destination` as an opaque,
// rail-specific single string (a callback/return URL for Stripe/Adyen/
// Checkout.com/..., a buyer email for Paystack — see each rail's own
// rail.rs module docs in patala-fiat). This adapter always sends
// Order.CallbackURL as destination, the closest single field Cackle's own
// Order carries that is broadly applicable across rails; a provider that
// instead needs an email will fail closed with a clear patala-side error
// rather than silently misusing a URL as an email address.
func (p *PatalaFiatProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, fmt.Errorf("payments: patala %s: order reference is required", p.name)
	}
	if o.AmountMinor <= 0 {
		return Charge{}, fmt.Errorf("payments: patala %s: order amount must be positive", p.name)
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	req := patala.PayRequest{
		AmountMinor: uint64(o.AmountMinor),
		Currency:    currency,
		Destination: o.CallbackURL,
		Reference:   o.Reference,
	}
	receipt, err := p.rail.Charge(req)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: patala %s: charge: %w", p.name, err)
	}

	now := time.Now()
	rec := PaymentRecord{
		Provider: p.name,
		Reference: o.Reference,
		// Persist the ORDER's real total/currency, not receipt.AmountMinor
		// (always 0 here — see patala_core::Receipt's honest
		// pending-lifecycle contract, PORTING.md §5: nothing has settled
		// yet at charge time). See (*PatalaFiatProvider).Verify for why
		// this is exactly what makes Cackle's own anti-fraud guarantee
		// hold up through a generic bool-only verify() surface.
		AmountMinor: o.AmountMinor,
		Currency:    currency,
		Status:      StatusPending,
		// Instructions is repurposed here to carry patala's own opaque
		// Receipt.proof bytes (base64), NOT human-readable buyer text —
		// see PaymentRecord's doc comment; this is the same durability
		// seam every RecordStore-backed native adapter (manual.go) uses,
		// just storing a different (still opaque) payload for this
		// provider family.
		Instructions: base64.StdEncoding.EncodeToString(receipt.Proof),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := p.store.PutPaymentRecord(ctx, rec); err != nil {
		return Charge{}, fmt.Errorf("payments: patala %s: persisting charge record: %w", p.name, err)
	}

	// Best-effort redirect URL — see patalaChargeProof's doc comment.
	redirectURL := ""
	var proof patalaChargeProof
	if json.Unmarshal(receipt.Proof, &proof) == nil {
		redirectURL = proof.RedirectURL
	}

	return Charge{
		Provider:     p.name,
		Reference:    o.Reference,
		RedirectURL:  redirectURL,
		Instructions: fmt.Sprintf("Payment started via patala (%s). Waiting for confirmation — the organiser or this deployment will need to poll for settlement (see docs/PAYMENTS.md: this path has no webhook yet).", p.name),
	}, nil
}

// Verify reconstructs the patala_core::Receipt this rail issued at Begin
// time and asks patala to re-check it against the real processor.
//
// The AmountMinor on the reconstructed receipt is deliberately the
// ORDER's real expected total (persisted at Begin, not the 0
// charge()/Begin itself returned) — patala_core::verify's own documented
// contract (patala-fiat/PORTING.md §6) fails closed unless the provider's
// freshly-refetched settled amount is >= receipt.AmountMinor. Passing the
// real expected total here turns that check into EXACTLY Cackle's own
// Reconcile anti-fraud guarantee ("pay 10, claim 1000" must be rejected)
// at the one place in this seam it can still be enforced: verify()
// returns only a bool, never the amount it actually observed, so there is
// nowhere else afterwards to fold that check in.
func (p *PatalaFiatProvider) Verify(ctx context.Context, reference string) (Result, error) {
	rec, ok, err := p.store.GetPaymentRecord(ctx, p.name, reference)
	if err != nil {
		return Result{}, fmt.Errorf("payments: patala %s: loading record: %w", p.name, err)
	}
	if !ok {
		return Result{}, fmt.Errorf("payments: patala %s: unknown reference %q", p.name, reference)
	}
	proof, err := base64.StdEncoding.DecodeString(rec.Instructions)
	if err != nil {
		return Result{}, fmt.Errorf("payments: patala %s: stored proof is corrupt: %w", p.name, err)
	}

	receipt := patala.Receipt{
		RailId:        p.name,
		AmountMinor:   uint64(rec.AmountMinor),
		Currency:      rec.Currency,
		Reference:     reference,
		Proof:         proof,
		SettledAtUnix: 0,
	}
	settled, err := p.rail.Verify(receipt)
	if err != nil {
		return Result{}, fmt.Errorf("payments: patala %s: verify: %w", p.name, err)
	}
	if !settled {
		return Result{Provider: p.name, Reference: reference, Status: StatusPending}, nil
	}

	now := time.Now()
	if rec.Status != StatusPaid {
		rec.Status = StatusPaid
		rec.UpdatedAt = now
		if err := p.store.PutPaymentRecord(ctx, rec); err != nil {
			return Result{}, fmt.Errorf("payments: patala %s: persisting settlement: %w", p.name, err)
		}
	}

	return Result{
		Provider:  p.name,
		Reference: reference,
		// EventID: patala_core::verify returns only a bool -- there is no
		// per-event provider transaction id available through this
		// generic surface to use as a replay-protection key (contrast
		// this with every removed native adapter's Webhook, which had a
		// real provider event id). The order reference is unique per
		// order per provider, so it is reused here: a second successful
		// Verify call for an already-settled reference is exactly the
		// "replay" case SeenStore exists to catch (see provider.go's
		// checkReplay) — a coarser key than a true per-event id, but
		// still the correct behaviour (don't re-issue tickets).
		EventID:     reference,
		Status:      StatusPaid,
		AmountMinor: rec.AmountMinor,
		Currency:    rec.Currency,
		PaidAt:      now,
	}, nil
}

// Webhook always fails — see this file's module doc comment ("What this
// adapter can and cannot do") and ErrPatalaNoWebhook.
func (p *PatalaFiatProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	return Result{}, ErrPatalaNoWebhook
}
