# Changelog

All notable changes to Cackle are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- **Cross-gate reconciliation, on the shared DMTAP Sync engine.**
  `GET /api/events/{id}/admission-conflicts` reports every ticket that more
  than one device claimed to admit — the double-scans that get through while
  two gates are partitioned from each other — with each gate's own account of
  what it did at the door. The merge is the suite's §4.3 add-only set
  (`internal/scan/substrate`, `github.com/vul-os/kotva/bindings/go`, compiled
  Rust under wazero) rather than merge logic hand-written here; the element is
  stamped with the scanning device and scan time so two gates' claims about the
  *same ticket* cannot collapse into one and hide the duplicate. Costs
  +3.59 MiB on the binary (~89% of that wazero, not the engine), pays module
  compilation lazily on first report, and touches nothing on the admission
  path — `internal/scan` does not import it and `CGO_ENABLED=0`
  cross-compilation still works for linux/amd64, linux/arm64, darwin/arm64,
  windows/amd64 and freebsd/amd64.
  **This detects; it does not prevent.** Two gates that cannot see each other
  cannot be stopped from admitting the same ticket, and no merge rule changes
  that. See [docs/OFFLINE-GATES.md](docs/OFFLINE-GATES.md).
- **`admitted_index` in the scan bundle** — the reconciled set of tickets
  already admitted anywhere, carried back down to gates so a re-pull refuses a
  ticket admitted at another gate (`scan.DecideWithBundle`, and the browser
  gate's `use-scan-engine.js`). This is the convergence channel
  docs/OFFLINE-GATES.md previously claimed existed and did not: "a synced
  ticket comes back as a duplicate everywhere after the next bundle refresh"
  was false, because the bundle only carried the *valid*-ticket index, which
  admission does not change. It is now true. It narrows the double-scan window
  on every re-pull without closing it.
- Running Cackle as an internet-reachable node venue gates sync to, walked end
  to end in [docs/SELF-HOSTING.md](docs/SELF-HOSTING.md) — bind address, TLS,
  what a scanner session actually authenticates, durability, backups, and a
  blunt statement that **event issuer private keys are stored as plaintext
  BLOBs in the SQLite file**, so whoever holds that file can mint valid tickets
  for every event in it.
- Initial rebuild as a standalone Go + SQLite + embedded-React product,
  replacing the original React/Supabase implementation. Single binary,
  `docker run -p 8080:8080 vulos/cackle`, `./cackle --demo` for a fully
  seeded zero-setup boot.
- Ed25519-signed, offline-verifiable ticket capabilities (`internal/tickets`)
  — the format is the product's core differentiator. See
  [docs/TICKET-FORMAT.md](docs/TICKET-FORMAT.md).
- Offline gate scanning (`internal/scan`): a `scan-bundle` endpoint hands a
  scanner everything it needs to run an entire event with no network, local
  append-only admission dedupe, and a batch sync endpoint for reconciling
  once back online. See [docs/OFFLINE-GATES.md](docs/OFFLINE-GATES.md).
- Events, ticket types, orgs and org roles (`owner` / `admin` / `scanner`),
  orders and checkout, integer-cents accounting throughout.
- Pluggable payment provider seam (`internal/payments`): a Paystack adapter
  and a `stub` provider used by `--demo` and tests. Cackle never holds funds.
  See [docs/PAYMENTS.md](docs/PAYMENTS.md).
- Full documentation set (`docs/`), roadmap, security policy, contributing
  guide, and this changelog.
- **Frozen conformance vectors for the ticket wire format**
  (`docs/ticket-format-vectors.json`): fixed keys, tokens pinned as bytes,
  and the exact accept/reject outcome for each. Run against BOTH shipped
  verifiers — `internal/tickets` (Go) and `web/src/lib/capability.js` (the
  browser scanner) — by `internal/tickets/conformance_test.go` and
  `web/src/lib/capability.conformance.test.js`. Both assert minimum vector
  counts and that every documented error code is exercised, so a truncated
  corpus fails loudly instead of passing by running nothing.
- Frontend test suite (`npm test` in `web/`, Node's built-in `node:test` —
  no new dependency) and a CI job that runs it. `CONTRIBUTING.md` had
  documented this command for some time; until now it did not exist.
- `gofmt` gate in CI and `make lint` (five files were not gofmt-clean).
- `scripts/check-doc-links.mjs` plus a CI job: every relative documentation
  link must resolve, both as GitHub renders the repo and as
  `site/docs.html` serves the flat mirror. It found five dead links on its
  first run.
- Relative-link rewriting in the published docs viewer (`site/docs.html`).
  Cross-chapter links in the mirror previously resolved against
  `/docs.html` and 404'd, every one of them.
- Joined VulOS as a product: standalone-first, hostable as an app by the
  Vulos OS, with no dependency on any Vulos service — Vulos is free and
  open-source, self-hosted — see [vulos.org](https://vulos.org).

### Changed

- `POST /api/scan/sync` now records the scanning device's own verdict in
  `admissions.reported_result` (migration `0002`) alongside the server's
  conclusion in `result`. The server still downgrades a second gate's
  `admitted` claim to `duplicate` — one ticket, one admitted row — but it no
  longer *erases* the fact that the second gate let somebody through. Before
  this, a gate that admitted during a partition was indistinguishable in the
  database from a gate that correctly refused a ticket its own log already had,
  which made the conflict unrecoverable after the fact.
- `POST /api/scan/sync` is now rate-limited per IP on the same bucket as
  `/api/scan`. It is an authenticated write reached from the public internet
  and was previously unbounded while the endpoint beside it was not.
- `scripts/gen-notices.sh` now **fails closed** (exit 4) when a Go module's
  licence cannot be determined, instead of writing a notices file with the
  entire Go section silently omitted. It previously degraded that case the same
  way it degrades a missing network, which produced a shorter, valid-looking
  file missing attribution the binary is obliged to reproduce.
  **Known blocker:** `github.com/vul-os/kotva/bindings/go@v0.2.1` ships no
  licence file, so `npm run notices` currently exits 4 and
  THIRD-PARTY-NOTICES.txt cannot be regenerated. No licence has been assumed on
  the upstream module's behalf; the fix is a LICENSE file in the kotva
  repository. See the note in `scripts/gen-notices.sh`.
- Payments story is ZAR-first (the platform's South African origin) but no
  longer hardcoded to Paystack — the provider sits behind a seam.
- **Genuinely country- and currency-agnostic**: removed every remaining ZAR/
  "cents" assumption. Every `*_cents` column and JSON field is renamed
  `*_minor` (since folded into the `0001_init.sql` baseline) and
  goes through `internal/money`'s ISO-4217 exponent table instead of a
  hardcoded 100 — JPY/KRW/VND/CLP/ISK (0 decimal places) and KWD/BHD/JOD/
  OMR/TND (3 decimal places) now render correctly everywhere, frontend
  included (new shared `web/src/lib/money.js`, `currencyDisplay:
  'narrowSymbol'` so a mismatched browser locale doesn't render "ZAR 450.00"
  instead of "R 450.00"). Currency is per-event, defaulting from a new
  `orgs.default_currency`; a `GET /api/currencies` endpoint replaces every
  hardcoded currency-picker shortlist. The `manual` payment provider (bank
  transfer/cash/invoice — zero API keys, zero network calls) is now always
  registered as Cackle's default, and both `manual` and `lnbits` persist
  their state (including manual's audit trail) to a new `payment_records`
  table instead of an in-memory map, so a restart no longer loses in-flight
  payment state.
- **Payments migrated onto the [patala](https://github.com/vul-os/patala)
  substrate.** 19 provider adapters (Stripe, Adyen, Checkout.com, PayPal,
  Square, Mollie, Flutterwave, Xendit, Midtrans, Mercado Pago, Razorpay,
  PayU, iyzico, PayFast, Yoco, BTCPay Server, lnbits, OpenNode, Coinbase
  Commerce) were removed from `internal/payments` and are now reached
  through patala's Go binding on an opt-in `-tags patala` cgo build
  (`internal/payments/patala.go`); the default, pure-Go `make build`/
  `make test` are unaffected. `manual` stays native (no network/cgo
  needed, and patala's generic surface can't drive its `MarkPaid`
  operator action anyway); `paystack.go` and `stablecoin.go` also stay
  native — see [docs/PAYMENTS.md](docs/PAYMENTS.md) for why. See
  [ROADMAP.md](ROADMAP.md) for the full migration writeup and the
  disclosed deltas.
- **Webhooks on the patala path.** `patala_core::PaymentRail` gained a
  `verify_webhook` export, so `PatalaFiatProvider.Webhook` now
  authenticates a real processor push through patala's own Rust
  verification instead of returning `ErrPatalaNoWebhook` unconditionally;
  that error now means only "this rail has no push surface" (patala-fiat's
  `manual`). Only `WebhookStatus::Settled` is ever treated as payment —
  `Unconfirmed` (authentic, but asserting nothing about money) maps to
  pending, pinned variant-by-variant by a test in the default,
  non-cgo test suite.
- Homepage (`/`) now shows the full demo events listing (Featured +
  Upcoming, sourced live from `GET /api/events`) in the same shot as the
  hero — the flagship screenshot (`docs/screenshots/hero.png`) captures
  the whole scrollable page, not just the marketing hero above the fold.
- **One ISO-4217 exponent table, not two.** `internal/payments` carried its
  own private copy of the zero-/three-decimal currency lists alongside
  `internal/money`'s. The copy is deleted; `internal/payments` now calls
  `money.Exponent` / `money.Amount.Major` directly. Its dead
  `majorStringToMinor` helper (no caller outside its own test) went with it.
  The stablecoin adapter now **fails closed** on a currency `internal/money`
  cannot resolve — at construction for `CACKLE_STABLECOIN_QUOTE_CURRENCY`,
  and at settlement for an allocation's currency — instead of assuming two
  decimals, which for a zero-decimal currency would have mis-scaled the
  settled amount by 100x.
- **The browser ticket verifier was laxer than the Go one, and now isn't.**
  Building the conformance corpus surfaced five cases the JavaScript
  verifier accepted that Go rejects: unknown payload fields, a non-object
  payload, wrong JSON field types, padded base64url, and standard-alphabet
  base64. All are now rejected, matching `internal/tickets` exactly.
- `docs/TICKET-FORMAT.md` rewritten as an implementable wire-format
  specification: exact encodings, canonical field order and omission rules,
  strict-parsing rules, `kid` derivation, key-ring wire shape, the full
  check order, and the error-code taxonomy.

### Fixed

- Documentation claims that ran ahead of the code, corrected in the docs
  rather than papered over:
  - `scan-bundle`'s `allocation` field was described as "a signed claim
    bounding how many admissions a device may grant". Nothing populates it:
    the server always sends `null`, no gate reads it, and the underlying
    helpers are for delegated *issuance*, not admission. README,
    `docs/OFFLINE-GATES.md`, `docs/ARCHITECTURE.md`, `docs/API.md` and
    `internal/tickets/README.md` now say so plainly.
  - Offline double-scan protection is now stated precisely: **prevented**
    on one device, **detected at sync** (downgraded to `duplicate`) across
    two offline devices — not stopped at the door. See
    [docs/OFFLINE-GATES.md](docs/OFFLINE-GATES.md).
  - `docs/PAYMENTS.md` referenced `stripe.go`, `checkoutcom.go` and
    `adyen.go` as "adapters in this package"; they moved to patala.
  - `CONTRIBUTING.md` still said Cackle was MIT-only and that money is
    "integer cents"; both were stale.
  - `CHANGELOG.md` referenced `0006_currency_minor_units.sql`, folded into
    the `0001_init.sql` baseline.
- The browser gate now **fails closed** when its local dedupe store errors,
  recording the scan `invalid` with the reason instead of throwing out of the
  decode handler and leaving the operator staring at an unchanged screen
  (which looks exactly like "the scanner didn't see the code" and invites a
  retry that admits). This matches `scan.admitOrDuplicate` on the Go side,
  which already refused in that case.

## What came before

Cackle originated as a React + Supabase ticketing application built around
Paystack, PayShap, and EFT payments. That implementation is not part of this
repository's history — the rebuild starts fresh as a single Go binary. The
old app's Deno edge functions (payment verification, order creation,
Paystack recipient/bank-list lookups) informed the design of
`internal/payments`, but no code was ported directly.

[Unreleased]: https://github.com/vul-os/cackle/compare/main...HEAD
