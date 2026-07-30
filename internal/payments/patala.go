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
//     (`../patala` relative to this module root) with its Go bindings
//     generated against a cdylib built with every fiat processor compiled
//     in:
//
//     cd ../patala/patala-go && make FEATURES=fiat-all generate
//
//     That local path is wired in by a **gitignored `go.work`**, created
//     by this repo's `make go.work` target — NOT by a `replace` directive
//     in go.mod. go.mod carried one until commit e6f4d2d, which deleted
//     it because `go mod download` in CI and in the Docker build fails on
//     a `replace` pointing at a sibling checkout that does not exist in
//     the build context. go.mod is deliberately patala-free now, so the
//     default build never fetches or touches patala.
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
// # This path has never been built by CI, and cannot be built from a clean clone
//
// Stated plainly because every line above reads like a supported build and
// it is not one. `grep -rn patala .github/workflows/` returns nothing:
// no workflow passes `-tags patala`, so no commit in this repo's history
// has ever been proved to compile this file. Its three prerequisites are
// all outside the repo and none is fetchable — the sibling checkout is
// wired by a gitignored `go.work`, the Go bindings under
// `../patala/patala-go/bindings/patala/` are generated build output that
// patala itself does not commit, and `build-patala` links `-lpatala_py`
// against a cdylib built locally by `cargo`. A fresh `git clone` of this
// repo therefore cannot build `-tags patala` by any sequence of commands
// that does not also involve cloning and building patala by hand.
//
// What IS covered by the default, untagged suite: the mapping tests that
// do not need the binding — see `patala_webhook_status_test.go` and
// `PatalaConfigFromEnv`. Everything in THIS file is unverified by any
// automated run. Treat it as an unbuilt integration, and do not take real
// money through it on the strength of this comment.
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
// and the only thing PatalaRailNewFiat hands back) has four methods
// reachable here: quote, charge, verify, and verify_webhook. Settlement can
// therefore be confirmed BOTH ways — by polling Verify, and by
// authenticating a processor's pushed webhook
// ((*PatalaFiatProvider).Webhook). Webhook verification used to live only
// in provider-specific free Rust functions that no binding could reach,
// which is why this adapter's Webhook returned ErrPatalaNoWebhook
// unconditionally until patala exported verify_webhook on the trait.
//
// Two things about that surface still shape the code below:
//
//   - patala-fiat's own `manual` rail (and patala_core's mock) leave
//     verify_webhook at its trait default, Err(Unsupported) — there is no
//     processor behind them to push anything. That surfaces here as
//     ErrPatalaNoWebhook, which is now a per-rail answer rather than a
//     blanket one.
//   - RailCapabilities carries no webhook flag, so Capabilities() cannot
//     ask the rail whether push delivery works; and it must not find out by
//     trying, because several rails' verify_webhook legitimately makes a
//     network call (Mollie and PayPal both re-fetch the named object from
//     the processor) while Capabilities() is contractually forbidden from
//     dialling anything. See Capabilities' own doc comment for what it
//     reports instead.
//
// verify() also returns only a bool, never the amount/currency it actually
// observed at the provider — see (*PatalaFiatProvider).Verify's own doc
// comment for how this adapter still gets Cackle's full
// pay-10-claim-1000 anti-fraud guarantee out of that bool. The webhook path
// does not have that problem: WebhookEvent carries the rail's own reported
// amount/currency, which Webhook passes through untouched so
// payments.Reconcile can do the comparison it exists to do.
package payments

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	patala "github.com/vul-os/patala/patala-go/bindings/patala"
)

// ErrPatalaNoWebhook is returned by PatalaFiatProvider.Webhook when THIS
// rail has no push-delivery surface — patala answers verify_webhook with
// Error::Unsupported. That is patala-fiat's `manual` rail (and
// patala_core's mock): there is no processor behind either to push
// anything. Every real processor patala-fiat ships implements
// verify_webhook, so this is a per-rail answer, not the blanket one it was
// before patala exported webhook verification on PaymentRail. See this
// file's module doc comment ("What this adapter can and cannot do").
var ErrPatalaNoWebhook = errors.New("payments: this patala rail has no webhook surface (patala answered Unsupported) -- poll Verify instead")

// patalaManualRailName is patala-fiat's always-available, no-processor
// rail. Cackle never registers it (cmd/cackle/patala_register.go skips it
// in favour of native manual.go); it is named here only because it is the
// one rail whose webhook support differs from every other — see
// (*PatalaFiatProvider).Capabilities.
const patalaManualRailName = "manual"

// patalaMaxWebhookBodyBytes caps the inbound webhook body Webhook will read
// before handing it to patala for signature verification, so a misbehaving
// or hostile caller can never force an unbounded read. Same 1 MiB bound
// stablecoin.go uses via the shared boundedRead helper (httpshared.go).
const patalaMaxWebhookBodyBytes int64 = 1 << 20

// Compile-time proof that patala_webhook_status.go's untagged copy of
// patala's WebhookStatus discriminants still matches the generated
// binding's own constants. Each line is an array index that is only in
// range when the two agree, so a regeneration that renumbers the enum
// breaks THIS BUILD instead of silently re-pointing the mapping that
// decides whether an order is marked paid. (uint subtraction wraps, so a
// binding value smaller than ours fails just as loudly as a larger one.)
var (
	_ = [1]struct{}{}[uint(patala.WebhookStatusSettled)-uint(patalaWebhookSettled)]
	_ = [1]struct{}{}[uint(patala.WebhookStatusNotSettled)-uint(patalaWebhookNotSettled)]
	_ = [1]struct{}{}[uint(patala.WebhookStatusUnconfirmed)-uint(patalaWebhookUnconfirmed)]
)

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
//   - Webhooks: true for every rail except patala-fiat's `manual` — see
//     webhooksSupported below for why that is a name check and not a
//     question asked of the rail.
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
		Webhooks:      p.webhooksSupported(),
		ZeroDecimalOK: true,
	}
}

// webhooksSupported reports what Capabilities().Webhooks promises: that
// Webhook can actually do something for this rail.
//
// It is a name comparison because patala offers no better answer. Two
// alternatives were rejected:
//
//   - Ask the rail. RailCapabilities (the only thing PatalaRail exposes
//     without performing an operation) has no webhook field at all.
//   - Probe it: call verify_webhook once with an empty delivery and read
//     Error::Unsupported. Several rails' verify_webhook makes a real
//     network call before it can reject anything (Mollie's and PayPal's
//     both re-fetch the named object from the processor), and
//     Capabilities() is contractually forbidden from dialling anything —
//     so probing would turn a capability query into an outbound request.
//
// What makes the name check honest rather than a guess: in patala-fiat,
// all 20 processor rails implement verify_webhook, and `manual` is the one
// rail that leaves it at PaymentRail's Err(Unsupported) trait default —
// deliberately, because it has no processor behind it to push anything.
// If that ever stops being true, the failure mode is bounded and loud
// rather than silent: Webhook still asks the real rail every time and
// still returns ErrPatalaNoWebhook whenever patala answers Unsupported, so
// a wrong answer here can only ever mis-advertise, never mis-settle.
func (p *PatalaFiatProvider) webhooksSupported() bool {
	return p.name != patalaManualRailName
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
		Provider:  p.name,
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
		Instructions: fmt.Sprintf("Payment started via patala (%s). Waiting for confirmation — either the processor's webhook or a Verify poll (see docs/PAYMENTS.md, \"The patala path\").", p.name),
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

// Webhook authenticates an inbound processor delivery through
// patala_core::PaymentRail::verify_webhook and reports the settlement it
// establishes. It fails closed at every step, exactly as the Provider
// interface requires: patala raises on a missing, malformed, stale or
// mismatched signature, and every ambiguity on this side is an error rather
// than a guessed Result.
//
// The body is read here, once, as the LITERAL bytes the processor signed
// (bounded — see patalaMaxWebhookBodyBytes); a JSON round-trip on the way in
// would invalidate every signature scheme patala-fiat implements. Headers
// and query parameters are forwarded as-is: patala matches header names
// case-insensitively, and the query map is read only by the schemes that
// put their secret in the URL rather than a header (LNbits).
//
// What comes back, and how each case is treated:
//
//   - Error::Unsupported — this rail has no push surface at all
//     (patala-fiat's `manual`). ErrPatalaNoWebhook, so callers can tell
//     "poll Verify instead" apart from "this delivery was rejected".
//   - Any other error — the delivery did not authenticate. Rejected.
//   - WebhookStatus::Settled — the only case that produces a Result, and
//     only if the delivery also names an order reference, an event id, and
//     a charge this deployment actually began.
//   - WebhookStatus::NotSettled / ::Unconfirmed — authentic, but not a
//     settlement (patala_core is explicit that Unconfirmed asserts nothing
//     whatsoever about money). Reported as ErrUnhandledEvent so the HTTP
//     route acknowledges the delivery with 200 instead of making the
//     processor retry a message there is nothing to do with, while Verify
//     stays the thing that governs the order. The Settled-vs-not decision
//     runs through patalaWebhookStatusToCackle, whose mapping is pinned
//     variant-by-variant by patala_webhook_status_test.go in the DEFAULT
//     (untagged) test suite.
//
// AmountMinor/Currency are what the RAIL reported, passed through
// untouched. They are deliberately NOT replaced with the stored order's own
// figures: payments.Reconcile compares the two, and substituting Cackle's
// own expectation here would make that comparison trivially true and throw
// away the pay-10-claim-1000 guarantee it exists for.
func (p *PatalaFiatProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r == nil || r.Body == nil {
		return Result{}, fmt.Errorf("payments: patala %s: webhook request has no body", p.name)
	}
	body, err := boundedRead(r.Body, patalaMaxWebhookBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, fmt.Errorf("payments: patala %s: webhook body exceeds %d bytes", p.name, patalaMaxWebhookBodyBytes)
		}
		return Result{}, fmt.Errorf("payments: patala %s: reading webhook body: %w", p.name, err)
	}

	headers := make(map[string]string, len(r.Header))
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	query := make(map[string]string)
	if r.URL != nil {
		for name, values := range r.URL.Query() {
			if len(values) > 0 {
				query[name] = values[0]
			}
		}
	}

	event, err := p.rail.VerifyWebhook(patala.WebhookDelivery{
		RawBody: body,
		Headers: headers,
		Query:   &query,
		// Explicit, not a clock read inside patala: this is what replay
		// windows are checked against, and passing it makes a delivery
		// exactly reproducible.
		NowUnix: uint64(time.Now().Unix()),
	})
	if err != nil {
		if errors.Is(err, patala.ErrPatalaErrorUnsupported) {
			return Result{}, fmt.Errorf("%w (rail %s: %v)", ErrPatalaNoWebhook, p.name, err)
		}
		return Result{}, fmt.Errorf("payments: patala %s: webhook rejected: %w", p.name, err)
	}

	status, err := patalaWebhookStatusToCackle(patalaWebhookStatus(event.Status))
	if err != nil {
		return Result{}, fmt.Errorf("payments: patala %s: %w", p.name, err)
	}
	if status != StatusPaid {
		return Result{}, fmt.Errorf("%w: patala %s delivery %q is authentic but reports %s (object %q) -- not a settlement; Verify still governs this order",
			ErrUnhandledEvent, p.name, event.EventId, patalaWebhookStatusName(patalaWebhookStatus(event.Status)), event.ObjectId)
	}

	reference := strings.TrimSpace(event.Reference)
	if reference == "" {
		// A settled delivery that names only a processor-side object.
		// Cackle keys orders by reference and has no object-id index, so
		// there is nothing to reconcile this against — refuse rather than
		// return a Result whose Reference cannot match any order.
		return Result{}, fmt.Errorf("payments: patala %s: settled webhook names no order reference (object %q); cannot reconcile it against an order", p.name, event.ObjectId)
	}
	if strings.TrimSpace(event.EventId) == "" {
		// Replay protection is keyed on this; a paid result that cannot be
		// deduped is worse than no result at all.
		return Result{}, fmt.Errorf("payments: patala %s: settled webhook for %q carries no event id (cannot dedupe)", p.name, reference)
	}
	if event.AmountMinor > uint64(math.MaxInt64) {
		return Result{}, fmt.Errorf("payments: patala %s: settled webhook for %q reports amount %d, which does not fit Cackle's int64 minor units", p.name, reference, event.AmountMinor)
	}

	rec, ok, err := p.store.GetPaymentRecord(ctx, p.name, reference)
	if err != nil {
		return Result{}, fmt.Errorf("payments: patala %s: loading record: %w", p.name, err)
	}
	if !ok {
		// Every patala charge persists a record at Begin, so a settled
		// delivery for a reference this deployment never began is
		// anomalous. Fail closed, exactly as Verify does.
		return Result{}, fmt.Errorf("payments: patala %s: settled webhook for unknown reference %q", p.name, reference)
	}

	now := time.Now()
	if rec.Status != StatusPaid {
		rec.Status = StatusPaid
		rec.UpdatedAt = now
		if err := p.store.PutPaymentRecord(ctx, rec); err != nil {
			return Result{}, fmt.Errorf("payments: patala %s: persisting settlement: %w", p.name, err)
		}
	}

	result := Result{
		Provider:    p.name,
		Reference:   reference,
		EventID:     event.EventId,
		Status:      StatusPaid,
		AmountMinor: int64(event.AmountMinor),
		Currency:    event.Currency,
		PaidAt:      now,
	}
	// Raw is audit data typed as JSON. Several patala-fiat rails sign a
	// form-encoded body (OpenNode), so only attach it when it really is
	// JSON rather than storing something no reader can parse.
	if json.Valid(body) {
		result.Raw = json.RawMessage(body)
	}
	return result, nil
}
