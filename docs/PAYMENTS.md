# Payments

**Cackle never holds funds.** It creates orders, hands off to a payment
provider (or the organiser, for `manual`) to actually move money between the
buyer and the *organiser's own account*, and records what happened. There is
no escrow, no platform wallet, no "hold and release" step anywhere in this
codebase.

> [!NOTE]
> **This migration is complete.** The ~20 processor adapters that used to
> live in `internal/payments/<provider>.go` (Stripe, Adyen, BTCPay, lnbits,
> and the rest) have moved out to **[patala](https://github.com/vul-os/patala)**,
> a sovereign, centerless payment-rail substrate other VulOS projects share.
> Cackle now consumes them through patala's Go binding — see
> ["The patala path"](#the-patala-path-real-processors) below for what that
> means for you as an operator, and [ROADMAP.md](../ROADMAP.md) for how this
> migration was staged. `manual` (see below) is untouched and still needs no
> patala, no cgo, and no network call at all.
>
> **None of the processors reachable through patala have been run against a
> real merchant account or sandbox from this repo.** Every adapter was
> ported from Cackle's own (previously unverified) Go implementation and is
> covered by `wiremock`-based unit tests on the patala side — see that
> repo's `patala-fiat/PORTING.md`. That proves the code handles the
> *documented* shape of a request/response and fails closed on a bad
> signature, a mismatched amount, a timeout, or a malformed payload. It does
> **not** prove the adapter actually talks correctly to the live provider.
> **Do not take real money through any processor here until you have run it
> against that provider's own sandbox/test-mode credentials and watched a
> real test transaction go through Begin → provider → Verify.** `manual`
> needs no such verification: it makes no network call at all.

## Cackle is country and currency agnostic

Cackle was originally built as a South African ticketing platform, and
earlier versions of this document (and the code) read that way — "ZAR
default", Paystack presented as *the* provider. **That framing was wrong and
has been removed.** There is no privileged country, currency, or processor:

- **Currency is per-event**, defaulting from an org-level default
  (`orgs.default_currency` → `events.currency`). An organiser running events
  in EUR, INR, and JPY concurrently sets each event's currency independently.
- **`manual` is the default provider for every deployment**, and cannot be
  disabled. Every real processor — Stripe, Adyen, BTCPay, and the rest —
  is optional, off by default, and (as of this migration) reached through
  patala rather than a native Cackle adapter — see
  [The patala path](#the-patala-path-real-processors). See
  [`CACKLE_PAYMENT_PROVIDERS`](#enabling-providers) below for enablement.
- **Cackle never holds funds**, regardless of which provider is in use.

## Money: minor units, not "cents"

Cackle stores every amount as `AmountMinor int64` plus an ISO-4217 alpha-3
currency code — never a float, never an assumed `/100`. "Cents" is not a
universal concept:

| Exponent | Currencies | 1 unit of `AmountMinor` |
|---|---|---|
| 0 (no minor unit) | JPY, KRW, VND, CLP, ISK, and a dozen others (BIF, DJF, GNF, KMF, PYG, RWF, UGX, VUV, XAF, XOF, XPF) | one whole major unit — ¥1000 is `AmountMinor: 1000`, not `100000` |
| 2 (the common case) | USD, EUR, GBP, ZAR, INR, and most of ISO-4217 | 1/100 of a major unit, same as "cents" |
| 3 | KWD, BHD, JOD, OMR, TND, IQD, LYD | 1/1000 of a major unit — 1 KWD is `AmountMinor: 1000` |

The authoritative table (exponent + display name per code) lives in
`internal/money`, not in this package. `internal/payments` never assumes 2 —
every adapter that needs to convert `AmountMinor` to/from a provider's own
wire format routes through `internal/payments/currency.go`'s
`minorUnitExponent` / `minorToMajorString` / `majorStringToMinor`, which
mirror `internal/money`'s table. Getting this exponent wrong in either
direction is a 100x bug: it either overcharges a buyer 100x or lets an event
sell out for 1% of its actual price.

### Providers disagree with ISO-4217, and with each other

This is the single most important thing to know before writing a new
adapter: **the "how many decimals does this currency have on the wire"
question is a property of *(currency, provider)*, not of the currency
alone.** Three real, confirmed examples from the adapters in this package:

- **Stripe** treats ISK and UGX as **forced two-decimal** even though ISO-4217
  (and every other currency in Stripe's own zero-decimal list) says they have
  no minor unit — see `stripe.go`'s `stripeZeroDecimalCurrencies` /
  `stripeForcedTwoDecimalCurrencies` and
  [docs.stripe.com/currencies](https://docs.stripe.com/currencies): "ISK
  transitioned to a zero-decimal currency... to charge 5 ISK, provide an
  amount value of 500."
- **Checkout.com** makes the *same kind of exception*, but for a *different*
  currency: it forces **CLP** to a ×100 representation ("the last two digits
  must be 00") while treating ISK as genuinely zero-decimal — the opposite
  of Stripe's ISK treatment. See `checkoutcom.go`'s
  `checkoutComZeroDecimalCurrencies` / `checkoutComForcedTwoDecimalCurrencies`.
- **Adyen** keeps its own **non-ISO-standard bucket** (CLP, CVE, IDR, ISK) with
  a currency-specific multiplier Adyen documents separately from the ordinary
  ISO-4217 exponent table — see `adyen.go`'s `adyenNonISOStandardCurrencies`.
  This adapter could not confirm the exact multiplier for that bucket from
  documentation alone and **refuses those four currencies outright**
  (`ErrAdyenUnsupportedCurrency`) rather than guess.

Anyone writing a new adapter: check that specific provider's own currency
documentation for zero/three-decimal exceptions. Do not assume ISO-4217's
exponent table applies unmodified, and do not assume two providers that both
"support CLP" encode it the same way on the wire.

## The seam

Every payment flow in Cackle goes through one interface,
`internal/payments.Provider` (`internal/payments/provider.go`):

```go
type Provider interface {
    Name() string                  // stable id, e.g. "manual", "stripe", "paystack"
    Capabilities() Capabilities
    Begin(ctx context.Context, o Order) (Charge, error)
    Verify(ctx context.Context, reference string) (Result, error)
    Webhook(ctx context.Context, r *http.Request) (Result, error)
}

type Capabilities struct {
    Currencies    []string // ISO-4217; empty = broad support
    Countries     []string // ISO-3166-1 alpha-2 of merchant accounts; empty = broad
    Flow          Flow     // FlowRedirect | FlowInline | FlowManual | FlowInvoice
    Refunds       bool
    Payouts       bool
    Webhooks      bool
    ZeroDecimalOK bool     // handles 0/3-decimal currencies correctly
}
```

`Order` carries `AmountMinor int64`, `Currency string`, `Reference string`,
buyer contact, and the event/org identifiers — never a client-supplied
amount that gets trusted downstream. `Result` carries the settled
`AmountMinor`, `Currency`, `Reference`, `Status`, and the provider's own raw
event id. `Reconcile` (in `provider.go`) is the anti-fraud check every
webhook/verify path runs before issuing a ticket: the provider's reported
amount and currency must match the stored order **exactly**, or the result
is rejected. Nothing in `internal/orders` or `internal/httpapi` hardcodes a
provider name — everything is looked up from a `Registry` by `Name()`.

`Flow` tells a checkout UI how to present a provider without hardcoding
per-provider knowledge:

- **`FlowRedirect`** — send the buyer's browser to a hosted payment page,
  then back via `Order.CallbackURL`.
- **`FlowInline`** — settles without leaving the checkout page (an embedded
  widget, a client-side SDK call).
- **`FlowManual`** — no payment page at all: the buyer sees instructions
  (bank details, pay-at-the-door, an invoice reference) and an organiser
  later marks the order paid. This is `manual`'s flow, and Cackle's default.
- **`FlowInvoice`** — the provider generates a payable invoice/request
  (crypto adapters, some invoice flows) rather than a page or a widget.

## Enabling providers

`CACKLE_PAYMENT_PROVIDERS` is a comma-separated list of provider names this
deployment allows (e.g. `manual,stripe,paystack`). `manual` is always
enabled regardless of this variable — it cannot be turned off. Leave the
variable unset and every *registered* provider is enabled (the permissive
default, meant for a self-hoster who's only wired up one real provider and
doesn't want to also enumerate it); once you set it, only the listed names
(plus `manual`) are usable — anything else stays registered (reachable via
admin/debug tooling) but won't show up in provider lists or be selectable
for a new order.

An event's currency is checked against the chosen provider's
`Capabilities().Currencies` at order-creation time (`Registry.Select`), so
picking a provider that can't handle an event's currency fails immediately
with `ErrCurrencyNotSupported` — never a confusing failure deep inside
`Begin`, and never a silently wrong charge.

## Native providers (still in `internal/payments/`)

Only four providers are still native Go code in this package — everything
else moved to patala (see below).

| Provider | `Name()` | Why it stays native |
|---|---|---|
| **Manual** | `manual` | Cackle's always-on default. No network call, no config, no cgo — see [The patala path](#the-patala-path-real-processors) for why this is *never* routed through patala even though patala also has its own `ManualRail`. State (which orders are marked paid, by whom, when) is durable via `RecordStore`/`internal/store` — see [Known limitations](#known-limitations) for the one remaining in-memory-only edge case. |
| **Paystack** | `paystack` | Kept native ONLY because `*PaystackProvider` also implements `orgs.BankingProvider` (`ListBanks`/`CreateRecipient` — listing supported banks and registering an organiser's payout recipient). Payout/banking management is explicitly **out of scope** for patala-fiat's port (`patala-fiat/PORTING.md`: *"Payouts... is out of scope — `PaymentRail` is about moving money INTO a merchant account, not paying organisers out of one"*), so there is no patala equivalent to switch to for this responsibility. Splitting `paystack.go` into "charge/verify via patala" + "banking natively" is a legitimate follow-up, not done here — see `paystack.go`'s own doc comment. Its `Begin`/`Verify`/`Webhook` (the actual charge path) are unchanged native Go, exactly as before this migration. |
| **Stablecoin (watch-only)** | `stablecoin` | Kept native because patala-fiat's 20-provider port does **not** include it — patala's own crypto strategy (`PATALA.md` §4/§6) is Ed25519-native rails (Solana/Stellar) rather than EVM stablecoin-watching, so this adapter had nowhere to move to. Unchanged by this migration. |
| **Demo stub** | `stub` | Unchanged — `--demo`/test-only, auto-settles instantly, refuses to register outside those contexts. |

## The patala path (real processors)

Every other processor Cackle used to implement natively — Stripe, Adyen,
Checkout.com, PayPal, Square, Mollie, Flutterwave, Xendit, Midtrans, Mercado
Pago, Razorpay, PayU, iyzico, PayFast, Yoco, BTCPay Server, lnbits,
OpenNode, and Coinbase Commerce (19 processors; Paystack's *charge* path
also ported, even though the Go file stays for banking — see above) — is
now reached through **[patala](https://github.com/vul-os/patala)**'s Go
binding (`patala-go`), via `internal/payments/patala.go`.

### Building it in

This is opt-in and requires more than `go build`:

1. Clone patala next to this repo, so `../patala` exists. The gitignored
   `go.work` (created by `make go.work`) wires in `../patala/patala-go` for
   the `-tags patala` build only; `go.mod` deliberately stays patala-free, so
   the default `go build`/CI never fetch or touch it.
2. Install `uniffi-bindgen-go`, pinned to the version this workspace's
   `uniffi` crate needs — see `../patala/patala-go/README.md`'s exact
   `cargo install` command.
3. A Rust toolchain (`cargo`/`rustc`) and a C toolchain (`cc`/`clang`/`gcc`
   — cgo needs one).
4. `make build-patala` / `make test-patala` / `make run-patala` (see the
   Makefile) run the whole `patala-go`'s bindings-generation →
   `CGO_ENABLED=1` build/test/run pipeline as one command. By hand: `go
   build -tags patala ./cmd/... ./internal/...` with `CGO_LDFLAGS`/
   `DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH` pointed at
   `../patala/patala-go/bindings/patala/` — see `internal/payments/patala.go`'s
   own build comment for the full recipe.

**The DEFAULT `make build`/`make test` (no `-tags patala`) are completely
unaffected** — they never import patala-go, never need cgo, and produce
the exact same pure-Go, `CGO_ENABLED=0` static binary as before this
migration. This is deliberate: `internal/tickets`' offline gate and
`internal/scan`'s scanner (Cackle's whole thesis) must never depend on cgo
— see [docs/OFFLINE-GATES.md](OFFLINE-GATES.md). If you don't want cgo at
all but still want real processors, **[`patala-sidecar`](https://github.com/vul-os/patala/tree/main/patala-sidecar)**
(a loopback-only HTTP server over the same patala-core surface) is the
documented alternative — Cackle does not integrate it today; only the cgo
binding path (`patala-go`) is wired up.

### Enabling a processor

Once built with `-tags patala`, every processor patala-fiat ships is
reachable through the exact same `CACKLE_<PROVIDER>_*` environment
variables the old native adapters used — `cmd/cackle/patala_register.go`
registers a processor iff at least one such variable is set, and fails
startup loudly (not silently) if what's set is incomplete or malformed.
`internal/payments.PatalaConfigFromEnv` does the translation from Cackle's
own variable names to patala-fiat's config map keys; almost every key is a
literal lower-case of Cackle's own suffix (`CACKLE_STRIPE_SECRET_KEY` →
`secret_key`), with two documented exceptions:

| Cackle env var | patala-fiat config key |
|---|---|
| `CACKLE_ADYEN_HMAC_KEY` | `hmac_key_hex` |
| `CACKLE_LNBITS_QUOTE_TTL_SECONDS` | `quote_ttl_secs` |

Every other provider's `CACKLE_<PROVIDER>_*` variables are unchanged from
this document's previous adapter table (Stripe's `SECRET_KEY`/
`WEBHOOK_SECRET`, Paystack's `SECRET_KEY`, BTCPay's `BASE_URL`/`API_KEY`/
`STORE_ID`/`WEBHOOK_SECRET`, and so on) — see patala's own
`patala-py/src/fiat.rs` (`build_<provider>` functions) for the
authoritative key list per provider, and `patala-fiat/PORTING.md` for how
each one was ported from this repo's original Go adapter.

### What's different from the native adapters this replaces

Read this before relying on the patala path for anything beyond `manual`:

- **No webhook support yet.** `patala_core::PaymentRail` (the trait
  `PatalaRailNewFiat` hands back) has exactly `quote`/`charge`/`verify` —
  no webhook concept at all; provider-specific webhook verification exists
  as free Rust functions in patala-fiat that are **not** part of the
  UniFFI-exported surface. `PatalaFiatProvider.Webhook` always returns
  `ErrPatalaNoWebhook`. Settlement can only be confirmed by **polling
  `Verify`** — an operator or a periodic job, not an instant provider push.
  This is a real, current limitation of the binding, not a Cackle
  shortcut; it will improve if/when patala exposes a webhook surface.
- **Anti-fraud reconciliation happens INSIDE `Verify`, not after it.**
  `patala_core::verify` returns only a `bool`, never the amount/currency it
  observed — so `PatalaFiatProvider.Verify` reconstructs the `Receipt` it
  asks patala to check using the ORDER's real expected total (persisted at
  `Begin` time), which makes patala's own `>=`-amount / exact-currency
  check into exactly Cackle's `Reconcile` guarantee. See that method's own
  doc comment for the full reasoning.
- **Redirect URLs are best-effort.** `patala_core::Receipt` has no
  redirect-URL field; hosted-checkout rails carry it inside the opaque
  `proof` bytes by convention (`{"redirect_url": "..."}`, per
  `patala-fiat/PORTING.md` §5). `PatalaFiatProvider.Begin` does a
  best-effort JSON read of that convention; a provider whose proof doesn't
  follow it leaves `RedirectURL` empty (buyers still get `Instructions`).
- **`Flow` is always reported as `FlowRedirect`.** `RailCapabilities` has
  no `Flow` field, so this generic adapter can't distinguish Razorpay's
  old `FlowInline` (Checkout.js widget) from everyone else's hosted
  redirect page — a disclosed approximation, not a silent one.
- **`Countries`, `Refunds`, `Payouts` are always empty/`false`.**
  `RailCapabilities` has no equivalent fields (see
  `patala-fiat/PORTING.md` §4's own gap list); Cackle's `Capabilities`
  struct reports the honest default rather than fabricating a value.
- **Replay protection keys on the order reference**, not a provider
  transaction id — there is no per-event id available through this
  generic `bool`-only surface. Reusing the reference is still correct
  (don't re-process an already-settled order twice), just coarser than
  the true per-event id the old native webhooks used.

### Deliberately not built (patala side)

Four providers named in Cackle's original build plan were never
implemented natively, and remain unported into patala, on purpose: their
APIs could not be verified confidently enough from available documentation
to write an honest adapter.

- **Braintree**
- **dLocal**
- **Worldline**
- **Przelewy24**

A missing adapter beats a wrong one. If you need one of these, write it
against that provider's current documentation in patala-fiat (see its own
`PORTING.md`), cite the doc source the same way every other adapter there
does, and be explicit
about which parts you're confident in and which you're not.

## Crypto adapters: underpayment, overpayment, confirmations, quote expiry

BTCPay Server, lnbits, OpenNode, and Coinbase Commerce — Cackle's former
native crypto adapters — moved to patala along with everything else (see
[The patala path](#the-patala-path-real-processors)); their
underpayment/overpayment/confirmation/quote-expiry handling now lives in
`patala-fiat`'s own source (`patala-fiat/src/btcpay/`,
`patala-fiat/src/lnbits/`, etc.) and `patala-fiat/PORTING.md`, not here.

**Stablecoin (watch-only)** — `stablecoin` — is the one crypto adapter
still native to this package (patala-fiat did not port it; see
[Native providers](#native-providers-still-in-internalpayments) above).
Its own behaviour is unchanged by this migration:

- **Underpayment** never settles an order: it sums qualifying on-chain
  transfers and compares the total **exactly** against the order's fiat
  total.
- **Confirmations**: `CACKLE_STABLECOIN_MIN_CONFIRMATIONS` is required
  with no default, since there is no provider-side settlement policy to
  lean on the way a hosted processor has.
- **Quote expiry / FX drift**: prices 1:1 against its configured quote
  currency; there is no separate FX rate to go stale.

### Why BTCPay/lnbits were preferred over hosted custodial crypto services

This framing (recorded here for context, not as current Cackle-native
behaviour) still holds on the patala side: BTCPay Server and lnbits are
**self-hosted and non-custodial** — the organiser runs their own instance
against their own on-chain wallet or Lightning node, and funds move
directly from buyer to organiser wallet with no third party ever touching
them, the same never-hold-funds property extended one layer further down.
OpenNode and Coinbase Commerce are **hosted and custodial** — a real
convenience (no server to run), but the provider briefly holds funds
before paying the organiser out, same as any card processor.

## Known limitations

- **`manual` state persistence**: `manual`'s settlement records (which
  orders are marked paid, by whom, when) are durable via `RecordStore`
  when `cmd/cackle` wires one up (see `NewManualWithStore` in
  `cmd/cackle/main.go`) — the one gap is a nil-store construction (used in
  tests / narrower embeddings), which is in-memory-only by design there.
- No processor reached through patala is sandbox-verified — see the note
  at the top of this document, and
  [What's different from the native adapters this replaces](#whats-different-from-the-native-adapters-this-replaces)
  for the webhook/reconciliation/redirect-URL gaps specific to the patala
  path.

## Security rules every adapter follows

These match the original Paystack adapter's behaviour and are the bar every
other adapter in this package is held to:

1. **Webhooks fail closed.** A missing or invalid signature is rejected.
   Signature comparisons use `hmac.Equal` (or the provider's own
   constant-time verifier), never `==`. The raw body is read and verified
   *before* any JSON decoding happens.
2. **A client-supplied amount or currency is never trusted.** Every webhook
   and verify path reconciles the provider's settled amount and currency
   against the stored order (`Reconcile` in `provider.go`) and rejects on
   any mismatch — this is exactly how ticketing platforms get robbed (pay
   10, claim 1000).
3. **Replay protection** keys off the provider's own event/reference id
   (`SeenStore.MarkSeen`), not anything client-supplied.
4. **Fail closed on any transport or parse error.** A timeout, a 500, or
   malformed JSON from the provider is an error, never a guessed "paid."
5. **Secrets only ever come from `CACKLE_<PROVIDER>_*` environment
   variables.** No defaults, never logged, never persisted anywhere secrets
   shouldn't be.
6. Every outbound call has a context timeout, and every response body read
   (provider API responses and incoming webhook bodies alike) is bounded to
   1 MiB.

## The order lifecycle

```
pending → (Begin) → buyer pays at provider (or organiser marks it paid) →
  (Verify, out-of-band poll) or (Webhook, provider push) →
  paid → tickets issued
```

An order can also land in `failed`, `refunded`, or `cancelled`. Ticket
issuance (`internal/tickets.Issue`, signing each ticket with the event's
current key) happens exactly once, on the transition into `paid` — whichever
path (`Verify` or `Webhook`) gets there first.

## Testing

`manual.go`, `paystack.go`, and `stablecoin.go` each still have their own
`_test.go` file exercising them against an `httptest` fake server (where
applicable): success, tampered signature, missing signature, amount
mismatch, currency mismatch, replay, timeout, a 500, and malformed JSON,
asserting fail-closed behaviour on every one of those. The `stub` provider
(`internal/payments/stub.go`) exists separately, for `--demo` and for
exercising the full checkout-to-ticket-issuance path in tests without any
real payment processor involved. It auto-settles instantly and
deterministically, and refuses to register at all if a real Paystack
secret is configured in the environment, so it can never end up live by
accident.

```bash
go test ./internal/payments/...
```

`internal/payments/patala.go` and its own `_test.go` only compile with
`-tags patala` (see [The patala path](#the-patala-path-real-processors)):

```bash
make test-patala
```

Every processor patala-fiat ports from Cackle's original adapters keeps
its own `httptest`/`wiremock`-based coverage — now in the patala repo
(`patala-fiat/src/<provider>/`), asserting the identical fail-closed
contract (tampered/missing signature, amount/currency mismatch, replay,
timeout, malformed JSON) this document's [security rules](#security-rules-every-adapter-follows)
require.
