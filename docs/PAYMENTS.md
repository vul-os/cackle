# Payments

**Cackle never holds funds.** It creates orders, hands off to a payment
provider to actually move money, and records what the provider tells it
happened. This document covers the provider seam, the Paystack adapter, and
why the webhook path is written to fail closed.

## The seam

Every payment flow in Cackle goes through one interface,
`internal/payments.Provider`:

```go
type Provider interface {
    Name() string
    // Begin returns a redirect URL (or inline instructions) for an order.
    Begin(ctx context.Context, o Order) (Charge, error)
    // Verify confirms a reference out-of-band. Fail CLOSED on error.
    Verify(ctx context.Context, reference string) (Result, error)
    // Webhook validates provider HMAC and returns the settled result.
    Webhook(ctx context.Context, r *http.Request) (Result, error)
}
```

Nothing in `internal/orders` or `internal/httpapi` hardcodes a provider —
they call `Provider.Begin`, `Provider.Verify`, or handle a webhook through
`Provider.Webhook` and act on the `Result`. Adding a second payment
processor is a matter of implementing this interface, not restructuring
anything upstream of it.

## Shipped providers

- **`paystack.go`** — Paystack's hosted checkout. `Begin` calls
  `POST /transaction/initialize` (amount in kobo/cents, the order ID as the
  `reference`, a `callback_url` built from `CACKLE_BASE_URL`) and returns the
  `authorization_url` Paystack hands back. `Verify` calls
  `GET /transaction/verify/{reference}` for out-of-band confirmation (used
  when a buyer is redirected back before any webhook arrives). `Webhook`
  validates the `x-paystack-signature` HMAC against
  `CACKLE_PAYSTACK_SECRET_KEY` before trusting anything in the payload.
- **`stub.go`** — auto-settles every order immediately, no external call.
  Used by `--demo` and by the test suite so neither depends on a real
  Paystack account or network access.

## Why ZAR-first, but not ZAR-only

Cackle's origin is a South African ticketing platform built around
Paystack, PayShap, and EFT — so defaults (`ZAR` currency, Paystack as the
first real adapter) reflect that. Nothing about the `Provider` interface is
ZAR-specific or Paystack-specific: `Order` carries its own `currency`, and a
second adapter for another processor or region is additive, not a rewrite.

## Configuring Paystack

```bash
export CACKLE_PAYSTACK_SECRET_KEY=sk_live_xxxxx   # or sk_test_xxxxx while testing
export CACKLE_BASE_URL=https://tickets.example.com
```

`CACKLE_PAYSTACK_SECRET_KEY` has no default and must never be committed —
see the config reference in [CONFIGURATION.md](CONFIGURATION.md). Register
`{CACKLE_BASE_URL}/api/payments/webhook/paystack` as the webhook URL in your
Paystack dashboard so settlement isn't solely dependent on the buyer's
browser making it back to the callback URL.

## The order lifecycle

```
pending → (Begin) → buyer pays at provider →
  (Verify, out-of-band poll) or (Webhook, provider push) →
  paid → tickets issued
```

An order can also land in `failed`, `refunded`, or `cancelled`. Ticket
issuance (`internal/tickets.Issue`, signing each ticket with the event's
current key) happens exactly once, on the transition into `paid` — whichever
path (`Verify` or `Webhook`) gets there first.

## Fail closed, always

`Verify` and `Webhook` are both specified to **fail closed**: any error —
a network failure calling the provider, an HMAC mismatch, a malformed
payload, an ambiguous status — must never be treated as "assume it's fine
and mark the order paid." The alternative (fail open) turns every transient
provider hiccup into a way to get a free ticket. If a check can't be
completed, the order stays exactly where it was.

The webhook handler in particular verifies the provider's HMAC signature
**before** doing anything else with the request body — an unsigned or
badly-signed webhook is rejected outright, not parsed-then-distrusted.

## Testing

The `stub` provider exists specifically so `internal/payments`,
`internal/orders`, and the full checkout-to-ticket-issuance path can be
exercised in tests and in `--demo` with no real Paystack account, no
network access, and no risk of moving real money. It settles instantly and
deterministically.
