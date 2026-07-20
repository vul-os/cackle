# Payments

**Cackle never holds funds.** It creates orders, hands off to a payment
provider (or the organiser, for `manual`) to actually move money between the
buyer and the *organiser's own account*, and records what happened. There is
no escrow, no platform wallet, no "hold and release" step anywhere in this
codebase.

> [!WARNING]
> **None of the adapters below have been run against a real merchant
> account or sandbox.** Every one was built by reading a provider's public
> API documentation and is covered by `httptest`-based unit tests — a fake
> HTTP server standing in for the real API. That proves the code handles
> the *documented* shape of a request/response and fails closed on a bad
> signature, a mismatched amount, a timeout, or a malformed payload. It does
> **not** prove the adapter actually talks correctly to the live provider.
> **Do not take real money through any adapter here until you have run it
> against that provider's own sandbox/test-mode credentials and watched a
> real test transaction go through Begin → provider → Verify/Webhook.**
> Most providers in this list offer one for free (Stripe test mode, Paystack
> test keys, PayPal Sandbox, a personal BTCPay/LNbits regtest instance,
> etc.) — point `CACKLE_<PROVIDER>_*` at the sandbox credentials, run a
> checkout end to end, and only then switch to live keys. `manual` needs no
> such verification: it makes no network call at all.

## Cackle is country and currency agnostic

Cackle was originally built as a South African ticketing platform, and
earlier versions of this document (and the code) read that way — "ZAR
default", Paystack presented as *the* provider. **That framing was wrong and
has been removed.** There is no privileged country, currency, or processor:

- **Currency is per-event**, defaulting from an org-level default
  (`orgs.default_currency` → `events.currency`). An organiser running events
  in EUR, INR, and JPY concurrently sets each event's currency independently.
- **`manual` is the default provider for every deployment**, and cannot be
  disabled. Every other provider — Stripe, Paystack, BTCPay, all of them —
  is optional and off by default. See [`CACKLE_PAYMENT_PROVIDERS`](#enabling-providers)
  below.
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

## Adapter reference

Every adapter lives in its own file (`internal/payments/<name>.go`) with a
doc comment citing where it was built from and how confident that adapter's
author was in each part of it — **read the file before trusting a row
below**; this table summarises, it doesn't replace, that comment.

**Verification status** for every row below is `unit-tested (httptest) — NOT
sandbox-verified` unless stated otherwise. That is not a hedge; as of this
writing, not one adapter has been run against a real or sandbox merchant
account.

| Provider | `Name()` | Regions / currencies | Flow | Required env vars | Verification status & notes |
|---|---|---|---|---|---|
| **Manual** | `manual` | Every currency, every country — no merchant account | Manual (buyer sees instructions; organiser calls `MarkPaid`) | *(none)* | No network calls, nothing to verify. **Default provider, always enabled.** State (which orders are marked paid, by whom, when) is **in-memory only** — see [Known limitations](#known-limitations). |
| Stripe | `stripe` | Broad (Checkout Sessions, any Stripe-supported currency) | Redirect | `CACKLE_STRIPE_SECRET_KEY`, `CACKLE_STRIPE_WEBHOOK_SECRET` | unit-tested — NOT sandbox-verified. `Verify`'s reference identity (Stripe session id vs. Cackle's own order reference) is flagged in the file as a one-line reconciliation to double check once wired to real `Order`/`Charge` persistence. |
| Adyen | `adyen` | Broad (Pay by Link) | Redirect | `CACKLE_ADYEN_API_KEY`, `CACKLE_ADYEN_MERCHANT_ACCOUNT`, `CACKLE_ADYEN_HMAC_KEY`, `CACKLE_ADYEN_API_BASE_URL` | unit-tested — NOT sandbox-verified. **HMAC key encoding** (hex-decode vs. raw UTF-8 bytes) was not re-confirmed against a live doc page — if wrong, it only makes genuine webhooks fail closed, never lets a forged one through. **Refuses CLP/CVE/IDR/ISK** outright (Adyen's non-ISO minor-unit bucket — see above) rather than guess a multiplier. |
| Checkout.com | `checkoutcom` | Broad (Hosted Payments Page) | Redirect | `CACKLE_CHECKOUTCOM_SECRET_KEY`, `CACKLE_CHECKOUTCOM_WEBHOOK_SECRET`, `CACKLE_CHECKOUTCOM_API_BASE_URL` | unit-tested — NOT sandbox-verified. The Hosted Payments Page **request** field list (`Begin`) was corroborated from Checkout.com's general Payments API conventions, not the JS-rendered Hosted Payments Page schema page itself — `Verify`/`Webhook` were confirmed directly against documented examples and are higher confidence than `Begin`. |
| PayPal | `paypal` | Broad (Orders v2) | Redirect | `CACKLE_PAYPAL_CLIENT_ID`, `CACKLE_PAYPAL_CLIENT_SECRET`, `CACKLE_PAYPAL_WEBHOOK_ID`, `CACKLE_PAYPAL_ENV` (`live` or `sandbox`, required explicitly) | unit-tested — NOT sandbox-verified. The full Get-Order status enum and the exact webhook event envelope were corroborated via secondary sources rather than a freshly fetched schema — an incomplete enum can only make this adapter too conservative, never wrongly permissive. **Refuses three-decimal currencies** (KWD, BHD, JOD, OMR, TND) — not documented in PayPal's fetched currency reference. |
| Square | `square` | Broad (Payment Links) | Redirect | `CACKLE_SQUARE_ACCESS_TOKEN`, `CACKLE_SQUARE_WEBHOOK_SIGNATURE_KEY`, `CACKLE_SQUARE_LOCATION_ID`, `CACKLE_SQUARE_NOTIFICATION_URL`, `CACKLE_SQUARE_API_BASE_URL` | unit-tested — NOT sandbox-verified. Webhook HMAC is confirmed to cover "signature key + notification URL + raw body", but the exact concatenation/encoding was corroborated via known SDK behaviour, not quoted verbatim from Square's prose — wrong would fail closed. **Refuses three-decimal currencies.** |
| Mollie | `mollie` | Broad, 2-decimal currencies only | Redirect | `CACKLE_MOLLIE_API_KEY`, `CACKLE_MOLLIE_WEBHOOK_URL` | unit-tested — NOT sandbox-verified. Webhook has **no signature at all** by Mollie's own design (the callback carries only an `id`; verification *is* the authenticated re-fetch) — that's Mollie's documented model, not a gap in this adapter. **Refuses zero- and three-decimal currencies** — could not confirm word-for-word whether Mollie wants `"1000"` or `"1000.00"` for JPY. |
| Paystack | `paystack` | Africa-oriented, see `paystackCurrencies` | Redirect | `CACKLE_PAYSTACK_SECRET_KEY` | unit-tested — NOT sandbox-verified, but this is the **oldest and most heavily tested adapter in the package** (60+ tests; predates the v2 `Provider` interface and was refactored onto it without changing any security-relevant logic). No longer the default — see above. |
| Flutterwave | `flutterwave` | NG/GH/KE/UG/TZ/ZA/RW/CI/CM; NGN/GHS/KES/UGX/TZS/ZAR/USD/XOF/XAF/RWF | Redirect | `CACKLE_FLUTTERWAVE_SECRET_KEY`, `CACKLE_FLUTTERWAVE_WEBHOOK_HASH` | unit-tested — NOT sandbox-verified. Confidence MEDIUM. Webhook verification is a **static shared secret**, not an HMAC (Flutterwave's own design) — still compared in constant time to avoid a timing side-channel. |
| Xendit | `xendit` | ID/PH/VN/TH/MY; IDR/PHP/VND/THB/MYR | Redirect | `CACKLE_XENDIT_SECRET_KEY`, `CACKLE_XENDIT_WEBHOOK_TOKEN` | unit-tested — NOT sandbox-verified. Confidence MEDIUM. Webhook check is a static per-account **callback token**, not an HMAC. IDR/VND handling (Xendit's primary markets) is implemented with confidence; PHP/THB/MYR are not independently verified. |
| Midtrans | `midtrans` | ID; IDR only | Redirect | `CACKLE_MIDTRANS_SERVER_KEY` | unit-tested — NOT sandbox-verified. Confidence MEDIUM-HIGH. Rejects any non-IDR currency outright. |
| Mercado Pago | `mercadopago` | LatAm, see `mercadoPagoCurrencies` | Redirect | `CACKLE_MERCADOPAGO_ACCESS_TOKEN`, `CACKLE_MERCADOPAGO_WEBHOOK_SECRET` | unit-tested — NOT sandbox-verified. Webhook signature manifest (MEDIUM-HIGH confidence) is a specific three-field semicolon-terminated template confirmed from Mercado Pago's own docs; the Preferences API request/response shape is MEDIUM confidence. The webhook push carries no amount — `Webhook` always re-fetches the payment server-to-server before returning a `Result`. |
| Razorpay | `razorpay` | IN; INR only | Inline (Checkout.js widget) | `CACKLE_RAZORPAY_KEY_ID`, `CACKLE_RAZORPAY_KEY_SECRET`, `CACKLE_RAZORPAY_WEBHOOK_SECRET` | unit-tested — NOT sandbox-verified. Confidence HIGH on order creation + webhook signature (Razorpay's most heavily used, most consistently documented endpoints). |
| PayU (India) | `payu` | IN; INR only — this is **PayU India / "PayU Biz" specifically**, not PayU LatAm or PayU Global, which are different products this adapter does not cover | Redirect (HTML form auto-POST, not a bare link) | `CACKLE_PAYU_MERCHANT_KEY`, `CACKLE_PAYU_SALT` | unit-tested — NOT sandbox-verified. Confidence MEDIUM. The SHA-512 request/response hash sequence is corroborated across PayU's own docs and every third-party integration guide (real confidence); the Verify Payment API's exact field names are less certain. |
| iyzico | `iyzico` | TR; TRY/USD/EUR/GBP | Redirect | `CACKLE_IYZICO_API_KEY`, `CACKLE_IYZICO_SECRET_KEY`, `CACKLE_IYZICO_BASE_URL` | unit-tested — NOT sandbox-verified. **Confidence is explicitly split** — see the file header. MEDIUM-HIGH on the security-critical shape (the checkout callback carries no signature; the adapter always re-verifies server-to-server). **LOW-MEDIUM, explicitly flagged**, on the outbound IYZWS request-signing byte sequence — iyzico has since introduced a newer HMAC-SHA256 scheme ("IYZWSv2") for some merchants and this file has not been checked against either scheme with a real account. Also does not populate iyzico's extensive mandatory buyer/address/basket fields (Cackle's `Order` doesn't carry most of them) — a functional gap, not a security one. |
| PayFast | `payfast` | ZA; ZAR only | Redirect (HTML form auto-POST) | `CACKLE_PAYFAST_MERCHANT_ID`, `CACKLE_PAYFAST_MERCHANT_KEY`, `CACKLE_PAYFAST_PASSPHRASE` (optional but recommended) | unit-tested — NOT sandbox-verified. Confidence HIGH on ITN (webhook) verification — signature check + the mandatory validate-callback round-trip. Does **not** implement PayFast's third recommended check (source-IP allowlisting) — add that at your ingress/load balancer. |
| Yoco | `yoco` | ZA; ZAR only | Redirect | `CACKLE_YOCO_SECRET_KEY`, `CACKLE_YOCO_WEBHOOK_SECRET` (`whsec_...`) | unit-tested — NOT sandbox-verified. Confidence MEDIUM-HIGH. Implements the Svix webhook standard (Yoco's documented choice) faithfully, including the timestamp-tolerance replay guard. |
| BTCPay Server | `btcpay` | Self-hosted, non-custodial; any fiat currency your BTCPay instance prices in | Invoice | `CACKLE_BTCPAY_BASE_URL`, `CACKLE_BTCPAY_API_KEY`, `CACKLE_BTCPAY_STORE_ID`, `CACKLE_BTCPAY_WEBHOOK_SECRET` | unit-tested — NOT sandbox-verified (no BTCPay instance was available to test against). **The flagship crypto adapter** — see [Crypto adapters](#crypto-adapters-underpayment-overpayment-confirmations-quote-expiry) below. HIGH confidence on invoice shape + webhook HMAC scheme; MODERATE on the exact `additionalStatus` enum (Marked/Invalid/PaidPartial/PaidOver) used to detect under/overpayment — verify against your BTCPay version. |
| LNbits (Lightning) | `lnbits` | Self-hosted, non-custodial; whatever fiat your LNbits instance's rate source supports | Invoice | `CACKLE_LNBITS_BASE_URL`, `CACKLE_LNBITS_API_KEY` (an invoice/read key, never an admin key), `CACKLE_LNBITS_WEBHOOK_SECRET`, `CACKLE_LNBITS_WEBHOOK_URL` (optional), `CACKLE_LNBITS_QUOTE_TTL_SECONDS` (optional, default 900) | unit-tested — NOT sandbox-verified. HIGH confidence on invoice creation/polling (LNbits' oldest, most stable API). MODERATE on the fiat-denominated amount fields, which have shifted across LNbits versions. LNbits' native webhook has **no signature at all** — this adapter requires its own shared secret on the registered webhook URL and never trusts the push body for settlement data, only as a hint to re-check. **In-memory state — see [Known limitations](#known-limitations).** |
| OpenNode | `opennode` | Hosted, custodial Bitcoin/Lightning checkout | Invoice | `CACKLE_OPENNODE_API_KEY`, `CACKLE_OPENNODE_BASE_URL` (optional) | unit-tested — NOT sandbox-verified. HIGH confidence on charge create/fetch and the documented status enum. MODERATE confidence on the exact webhook signing scheme (`hashed_order`) — compensated for by always re-fetching the charge from OpenNode's authenticated API before trusting a webhook, so a subtly-wrong signature construction can't fabricate a settlement. |
| Coinbase Commerce | `coinbasecommerce` | Hosted, custodial crypto checkout | Invoice | `CACKLE_COINBASECOMMERCE_API_KEY`, `CACKLE_COINBASECOMMERCE_WEBHOOK_SECRET`, `CACKLE_COINBASECOMMERCE_BASE_URL` (optional) | unit-tested — NOT sandbox-verified. HIGH confidence on auth, create/fetch charge shape, and webhook HMAC scheme. MODERATE confidence on the status enum beyond NEW/PENDING/COMPLETED/EXPIRED — **this is exactly where under/overpayment surfaces** (see below). Confirm before depending on it that Coinbase Commerce is still onboarding new merchants — it has stopped at points in its history. |
| Stablecoin (watch-only) | `stablecoin` | Any EVM chain with an Etherscan-family explorer API; one configured quote currency, no FX | Invoice | `CACKLE_STABLECOIN_INDEXER_BASE_URL`, `CACKLE_STABLECOIN_INDEXER_API_KEY`, `CACKLE_STABLECOIN_TOKEN_CONTRACT`, `CACKLE_STABLECOIN_TOKEN_DECIMALS`, `CACKLE_STABLECOIN_QUOTE_CURRENCY` (optional, default USD), `CACKLE_STABLECOIN_MIN_CONFIRMATIONS` (required, no default) | unit-tested — NOT sandbox-verified (tested only against `httptest` fakes of the documented Etherscan-family shape, never a real indexer account). **Never holds a private key** — hands out a pre-generated address from an operator-supplied pool and only watches the chain. **Address-reuse hazard**: reconciliation assumes each pool address is used for exactly one order; reissuing an address to a second order while the first is open will conflate their transfers. This is a correctness requirement on whoever implements the address allocator, not something this file can detect. |
| Demo stub | `stub` | Any (accepts everything) | Inline | *(none — refuses to run unless explicitly opted in via `--demo`/`CACKLE_DEMO`, and refuses to register at all if a real Paystack secret is configured)* | Not a real provider. Auto-settles every order instantly for `--demo` and the test suite. Cannot be reached in a real deployment by construction. |

### Deliberately not built

Four providers named in the original build plan were **not** implemented,
on purpose, because their APIs could not be verified confidently enough from
available documentation to write an honest adapter:

- **Braintree**
- **dLocal**
- **Worldline**
- **Przelewy24**

A missing adapter beats a wrong one. If you need one of these, write it
against that provider's current documentation, cite the doc source in the
file header the same way every other adapter here does, and be explicit
about which parts you're confident in and which you're not.

## Crypto adapters: underpayment, overpayment, confirmations, quote expiry

The crypto adapters do not share a single mechanism for these — each
provider's own model differs, and every adapter is honest in its file header
about exactly how it detects each case:

- **Underpayment** never settles an order through any crypto adapter here.
  BTCPay reports `additionalStatus` values (`PaidPartial`) that this package
  maps to "not yet paid," never paid; OpenNode has an explicit `underpaid`
  status mapped to `StatusFailed`; the stablecoin adapter sums qualifying
  on-chain transfers and compares the total **exactly** against the order's
  fiat total.
- **Overpayment** is treated as a condition requiring a human, not something
  to silently accept or silently discard. BTCPay's `PaidOver` and Coinbase
  Commerce's `UNRESOLVED` state both return a distinct sentinel error
  (`ErrBTCPayOverpaid`, `ErrCoinbaseCommerceRequiresManualReview`) instead of
  a `Result` — flagged for the organiser to check the provider's own
  dashboard and refund/credit the difference. OpenNode has no documented
  `overpaid` status; if OpenNode itself settles a modest overpayment
  silently, the generic `Reconcile` check in `provider.go` still catches it,
  since the settled amount won't exactly equal the stored order total.
- **Confirmations** are not independently configurable by Cackle for
  BTCPay, LNbits, OpenNode, or Coinbase Commerce — each trusts that
  provider's own settlement status (BTCPay's per-store `SpeedPolicy`,
  Lightning's instant HTLC settlement with no block wait at all) rather than
  re-deriving a confirmation count Cackle has no reliable way to see. The
  stablecoin adapter is the exception: `CACKLE_STABLECOIN_MIN_CONFIRMATIONS`
  is required with no default, since there is no provider-side policy to
  lean on there.
- **Quote expiry / FX drift**: every crypto adapter prices in the event's own
  fiat currency and lets the provider (or, for stablecoin, the 1:1 peg
  assumption) lock that rate at charge-creation time. Coinbase Commerce
  expires a charge that isn't paid in time rather than settling it late at a
  stale rate (`EXPIRED` → `StatusFailed`). LNbits invoices are always
  fixed-amount BOLT11 with a configurable TTL
  (`CACKLE_LNBITS_QUOTE_TTL_SECONDS`) — a Lightning HTLC settles for exactly
  the invoiced amount or not at all, so there is no partial-settlement case
  to handle there at all.

### Why BTCPay/LNbits are preferred over hosted custodial crypto services

BTCPay Server and LNbits are **self-hosted and non-custodial**: the
organiser runs their own instance against their own on-chain wallet or
Lightning node, and Cackle only ever talks to that instance's API — funds
move directly from buyer to organiser wallet with no third party ever
touching them. That is the same never-hold-funds property Cackle has for
itself, applied one layer further down, which is why BTCPay is called out in
its own file header as "the flagship crypto provider." OpenNode and
Coinbase Commerce are **hosted and custodial** — a real convenience (no
server to run), but the provider briefly holds the funds before paying the
organiser out, same as any card processor. Prefer BTCPay/LNbits where an
organiser is willing to run the self-hosted piece; treat OpenNode/Coinbase
Commerce as the easier on-ramp for organisers who aren't.

## Known limitations

- **`manual` and `lnbits` hold their settlement state in memory only.** A
  process restart between an order being created and being marked/settled
  loses that state — `manual`'s marked-paid records and `lnbits`'s
  `payment_hash` → invoice map both live in an in-memory map guarded by a
  mutex, not a database row. This is called out explicitly in both files'
  doc comments. **A fix is pending**: the intent is for callers to persist
  the audit trail (`internal/store`) alongside these in-memory records, not
  to replace the in-memory fast path outright. Don't run either provider
  across a planned restart boundary until that lands, and don't run
  `lnbits` behind multiple replicas of the Cackle process today.
- No adapter here is sandbox-verified — see the warning at the top of this
  document.

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

Every adapter above has its own `_test.go` file exercising it against an
`httptest` fake server: success, tampered signature, missing signature,
amount mismatch, currency mismatch, replay, timeout, a 500, and malformed
JSON, asserting fail-closed behaviour on every one of those. The `stub`
provider (`internal/payments/stub.go`) exists separately, for `--demo` and
for exercising the full checkout-to-ticket-issuance path in tests without
any of the above adapters or a real payment processor involved. It
auto-settles instantly and deterministically, and refuses to register at
all if a real Paystack secret is configured in the environment, so it can
never end up live by accident.

```bash
go test ./internal/payments/...
```
