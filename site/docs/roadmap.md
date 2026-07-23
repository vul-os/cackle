# Cackle Roadmap

The destination: events + ticketing where the gate is never the thing that
fails. v1 is deliberately narrow — online sales, offline admission — and
everything below it is additive. Nothing on this roadmap is required for
Cackle to be useful; v1 stands on its own.

## v1 — ships now

The core loop: create an event, sell tickets, scan them at the door, keep
scanning if the network dies. This is the whole scope of the 0.1.0 line —
see [CHANGELOG.md](CHANGELOG.md) for what has actually landed at any given
point.

- Single Go binary, embedded SQLite (`modernc.org/sqlite`, pure Go), embedded React frontend
- Orgs, org membership, roles (`owner` / `admin` / `scanner`)
- Events: draft → published → cancelled, ticket types with pricing, quantity caps, sales windows
- Orders and checkout against live ticket-type availability, money as integer minor units in the event's currency
- Ed25519-signed ticket capabilities (`internal/tickets`) — the offline-verifiable format, see [docs/TICKET-FORMAT.md](docs/TICKET-FORMAT.md)
- **Offline gate scan** — `scan-bundle` endpoint, pure offline `Verify()`, local append-only admission dedupe, batch sync back once online (`internal/scan`), see [docs/OFFLINE-GATES.md](docs/OFFLINE-GATES.md)
- Payment provider seam (`internal/payments`), `manual` as the always-on native default, `paystack` native (its banking/payout methods have no patala equivalent — see docs/PAYMENTS.md), `stablecoin` native (not ported into patala), a `stub` provider for `--demo` and tests, and every other real processor (Stripe, Adyen, BTCPay, lnbits, and 16 more) reached through the **patala** substrate's Go binding on an opt-in `-tags patala` cgo build — Cackle is country/currency agnostic and never holds funds, see [docs/PAYMENTS.md](docs/PAYMENTS.md). **This migration is complete** — see the next section for how it was staged.
- Per-event sales/admission stats
- `--demo` mode: fully seeded, zero setup

Everything below this line is **not yet built** — each is marked with what it
would take and why it isn't v1.

## Done — payments migrated to the `patala` substrate (Cackle is just ticketing again)

Payment processing is not ticketing, and it had grown to nearly half of
Cackle's code. Those 20+ adapters are a general-purpose asset other VulOS
projects want too, so they moved out into **[patala](https://github.com/vul-os/patala)** —
a sovereign, centerless payment-rail substrate (non-custodial, no token, one
honest interface over fiat and crypto rails). Cackle now consumes patala
through its Go binding and keeps only what it owns: events, tickets, and the
offline gate, plus the three native exceptions below.

The migration, as it actually landed:

1. **Ported the adapters into patala** (Rust core, faithful ports of
   Cackle's tested Go logic — same minor-unit/currency rules, same
   fail-closed verify, same "never holds funds" invariant) — 20 processors
   now live in `patala-fiat`, documented per-adapter in that repo's
   `PORTING.md`.
2. **Cackle consumes patala's Go binding** (`internal/payments/patala.go`,
   `//go:build patala`) in place of the removed native adapters, via ONE
   generic `PatalaFiatProvider` type driven by `patala.PatalaRailNewFiat`
   (provider name + a `map[string]string` config built from this
   deployment's existing `CACKLE_<PROVIDER>_*` env vars — see
   docs/PAYMENTS.md's "The patala path").
3. **`manual` stays native**, deliberately, on both counts the plan called
   for: no network/config/cgo needed, AND patala's generic FFI surface
   can't drive `ManualRail`'s `mark_paid`/`mark_failed` operator actions in
   the first place (those are Rust-only inherent methods, not part of the
   exported `PaymentRail` trait). **Two more adapters stay native for
   reasons not anticipated in the original plan**: `paystack.go` (its
   `orgs.BankingProvider` payout/banking methods have no patala
   equivalent — payouts are explicitly out of scope for patala-fiat's
   port) and `stablecoin.go` (patala-fiat's crypto strategy is
   Ed25519-native rails, not EVM stablecoin-watching, so it was never
   ported). The other **19** provider files (and their tests) were
   removed from `internal/payments`.
4. **The default build is unaffected.** `make build`/`make test` (no
   `-tags patala`) never import patala-go, need no cgo, and produce the
   same pure-Go `CGO_ENABLED=0` static binary as before — verified: the
   offline gate/scanner and every other test package still pass with zero
   tags, and `otool -L` on a default-built binary shows no
   `libpatala_py` link. `make build-patala`/`make test-patala`/
   `make run-patala` opt into the cgo path.

**A real, honest gap, not glossed over**: patala's generic FFI surface
(`PatalaRailNewFiat` → `quote`/`charge`/`verify`) has **no webhook
method** — provider webhook verification is Rust-only, not exported over
UniFFI yet. A patala-backed processor today can only be confirmed by
polling `Verify`, never by a provider's push webhook. See
docs/PAYMENTS.md's "What's different from the native adapters this
replaces" for this and the other disclosed deltas (best-effort redirect
URL, `Flow` always reported as redirect, replay-protection keyed on the
order reference instead of a provider event id).

The honest sandbox-verification gap moves with the adapters: they were
written against published API docs and unit-tested (`httptest` /
`wiremock`), but **not one has been run against a real merchant
sandbox**. That end-to-end verification (a live `Begin` → provider →
`Verify` round trip, subunit conversion confirmed for zero- and
three-decimal currencies against a live account) is patala's top payment
priority, per adapter, and still needs real merchant sandbox access this
project can't self-provide. `manual`/`stub` need no sandbox and stay
proven end-to-end.

## Later — signed transfers & resale

Ticket transfer (attendee-to-attendee) done honestly, without turning Cackle
into a scalper platform: a per-ticket **hash chain** where each transfer is a
signed link (old holder signs off, new holder is named), so a scanned ticket
proves its whole ownership history back to issuance. "Settlement at the door"
— a transferred ticket that reaches the gate is admitted on the strength of
the chain, not a phone call to the server. Needs: chain format, a transfer UI,
and a policy knob organisers set per event (allow / disallow / cap price).
**Not yet built.**

## Later — venue mesh sync between scanners

Today a gate deduplicates locally, against its own admissions table, and
reconciles with the server once it's back online — which is correct for one
scanner, but two scanners at two entrances of the same venue, both offline,
can independently admit the same ticket. A venue mesh (scanners gossiping
admissions over local Wi-Fi/Bluetooth, CRDT-merged, no server in the loop) is
the fix. This is exactly the kind of problem the VulOS **Sync** substrate
capability (CRDT op algebra + version vectors, one Rust core) exists for — if
Cackle adopts it, it adopts the spec as-is rather than hand-rolling a second
merge engine. **Not yet built; design not yet started.**

**Design note, recorded now so it isn't discovered the hard way later.**
`internal/scan.ReconcileTicket` resolves a cross-device duplicate by earliest
`scanned_at`, breaking an exact tie on `device_id`. That is deterministic and
correct on its own. But `VULOS-PRODUCT-STANDARD.md` records that two engines
can both converge and still disagree: flowstock breaks an exact tie on node
id, the substrate breaks it on author public key. Cackle's `device_id` is a
third convention, and today that is harmless — v1 gates never merge with each
other, only with the server.

It stops being harmless the moment venue mesh sync lands. The standard's
stated clean fix is to make a node's id *be* its public key, and Cackle is
unusually well placed to do that cheaply: scanner devices will need keypairs
anyway to sign their own admission records. **If `device_id` is minted as the
device's public key when device identity is built, this whole class of
divergence disappears before it can occur** — and adopting the Sync capability
becomes a drop-in rather than a migration. Do it then, not after.

## Later — on-site closed-loop payments

Cashless top-up wristbands/cards for festivals with patchy connectivity:
signed spend tokens with an **offline floor limit** (a scanner can approve a
small spend against a locally-cached balance without phoning home, above the
floor it needs a network hop), settled and reconciled centrally once online.
This is a genuinely hard problem — double-spend prevention without a network
is the same class of problem as offline payments has always been — and is
explicitly deferred until the simpler admission problem is solid in
production. **Not yet built; not yet designed.**

## Later — DMTAP ticket delivery + DMTAP-PUB event discovery

Tickets arrive in your inbox instead of an app: deliver the capability over
**DMTAP** (see the DMTAP spec) as a first-class message, so an attendee's
proof of purchase lives wherever their mail lives, not only in Cackle's
database. **DMTAP-PUB** for event discovery — publish events as a signed feed
that any conforming reader can subscribe to, no Cackle account required to
browse what's on. This is an à-la-carte substrate adoption per the VulOS
product standard: Cackle would speak the DMTAP spec as-is, never a parallel
invention. **Not yet built.**

## Later — capacity delegation to sub-issuers

For a remote or disconnected site (a satellite stage at a festival, a venue
with no reliable link to the organiser's main Cackle instance): the main
event key delegates a **capped ticket-issuance allowance** to a sub-issuer key
that can mint admission-valid tickets on its own, offline, up to that cap.
Reconciliation happens later. This generalizes the existing per-event key
design (`event_keys`) one level further — a delegated key instead of a single
issuer — without ever introducing a global signing key. **Not yet built; not
yet designed.**

## Later — optional escrow adapters

Cackle's payment seam settles directly to the organiser today (`manual` plus
20+ optional adapters, and `stub` for demos/tests). An **optional** escrow
adapter would hold funds until an event completes (or a dispute window
closes) before releasing payout — useful for marketplaces that don't fully
trust first-time organisers, irrelevant for everyone else. Strictly opt-in
behind the same `Provider`-adjacent seam; Cackle's default posture (never
hold funds) does not change. **Not yet built.**

## Later — post-quantum signature agility

`event_keys` is Ed25519 today. The capability format's `kid` field already
gives a scanner a way to look up "which key signed this," which is most of
what's needed to support a second signature algorithm side-by-side and
rotate event-by-event as PQ signature schemes mature and get fast enough for
a QR-sized payload. No urgency — this is here so the format doesn't need a
breaking change later. **Not yet designed.**

## The honest framing

Every "later" item above reads as a feature for a festival in a field with
one bar of signal. That's the real, current market, and it's reason enough on
its own. It also happens to be the same design that would work somewhere
genuinely disconnected — a colony with a multi-minute speed-of-light delay to
"the server," for instance. Mars is the honest extreme of the same
constraint, not the pitch. If it works at a farm venue with no fibre, it
works anywhere.

## Non-goals

- A global ticket-signing key. Every event signs with its own key, always.
- Making the server a required hop for admission. If a feature above can't be
  verified offline, it doesn't belong in the critical admission path.
- Holding funds. Cackle is a ticketing platform with a payment seam, not a
  payment processor.
