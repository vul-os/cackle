# Changelog

All notable changes to Cackle are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

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
- Joined VulOS as a product: standalone-first, hostable as an app by the
  Vulos OS, no dependency on Vulos billing beyond the two-service model
  (Relay, backup storage) described in [README.md](README.md#part-of-vulos).

### Changed

- Payments story is ZAR-first (the platform's South African origin) but no
  longer hardcoded to Paystack — the provider sits behind a seam.

## What came before

Cackle originated as a React + Supabase ticketing application built around
Paystack, PayShap, and EFT payments. That implementation is not part of this
repository's history — the rebuild starts fresh as a single Go binary. The
old app's Deno edge functions (payment verification, order creation,
Paystack recipient/bank-list lookups) informed the design of
`internal/payments`, but no code was ported directly.

[Unreleased]: https://github.com/vul-os/cackle/compare/main...HEAD
