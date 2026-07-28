# Contributing to Cackle

Thanks for helping build ticketing where the gate doesn't fall over when the
network does. All contributions are under the project's dual licence,
[MIT](LICENSE-MIT) OR [Apache-2.0](LICENSE-APACHE).

## Code of Conduct

We follow the [Contributor Covenant v2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).

## Dev environment setup

Requirements: Go 1.25+, Node 20+.

```bash
# Backend — scope to the real Go packages, never bare `./...`
# (web/node_modules can contain a stray vendored .go file).
go vet ./cmd/... ./internal/...
go test ./cmd/... ./internal/...

# Frontend
cd web && npm install && npm run dev
```

## Branch and PR conventions

- Branch off `main`. Name: `feat/description`, `fix/description`,
  `chore/description`.
- One logical change per PR. Keep diffs reviewable.
- PRs require at least one approving review.
- Squash-merge preferred.

## Commit message style

Conventional Commits welcome, not required:

```
feat(tickets): add key rotation support to event_keys
fix(scan): reject admissions with a future scanned_at
chore: bump modernc.org/sqlite
```

## Testing expectations

Before opening a PR:

```bash
make check   # gofmt + go vet + eslint + docs gates, then go test and
             # `npm test` in web/, then the full single-binary build
```

`make check` is the whole gate CI runs. If you want to run a piece of it
directly:

```bash
go test ./cmd/... ./internal/...   # backend, scoped — never bare ./...
cd web && npm test                 # browser ticket verifier vs. the
                                   # frozen conformance vectors
node scripts/check-doc-links.mjs   # every relative doc link — and every
                                   # SOMETHING.md a source comment cites —
                                   # resolves to a file that exists
```

`internal/tickets` is the highest-scrutiny package in the codebase — it is
the whole reason offline gate scanning works. Changes there need tests
covering tamper, wrong key, expired, not-yet-valid, truncated, wrong
version, and replay, per the contract in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

The ticket wire format additionally has a **frozen conformance corpus**,
[`docs/ticket-format-vectors.json`](docs/ticket-format-vectors.json), run
against both shipped verifiers (Go and the browser one in
`web/src/lib/capability.js`). Do not edit those vectors to make a change
pass — they are the contract, and changing them means bumping
`tickets.CurrentVersion` deliberately. Adding vectors is welcome; raise the
minimum-count floors in both test files when you do.

## Finding a good first issue

Look for `good first issue` or `help wanted` labels. UI polish,
accessibility, and documentation are low-friction entry points.

## Scope: what we say yes and no to

### Yes

- Bug fixes and security improvements
- Ticket-format and offline-scan correctness (this is the product)
- Additional payment provider adapters (behind the existing `Provider` seam)
- Accessibility improvements
- Tests and documentation

### No — frozen invariants

- **No global ticket-signing key.** Every event signs with its own
  `event_keys` entry. Do not introduce a shared/global key, ever.
- **No CGO.** `modernc.org/sqlite` is pure Go on purpose — it's what makes
  the single static binary possible.
- **No .tsx files.** Frontend is JSX only (`*.jsx`) — a house-wide VulOS
  invariant.
- **No hard runtime dependency** on Supabase, Firebase, Ephor, or DMTAP.
  Cackle must build and run fully standalone.
- **No float money.** Amounts are integer MINOR UNITS (`AmountMinor`) plus
  an ISO-4217 code, always — never floats, and never assumed to be
  hundredths. The one exponent table lives in `internal/money`; adding a
  second copy of it anywhere is how this invariant gets broken in practice.
- Making `internal/tickets.Verify` impure (adding a DB call, a network call,
  or an implicit clock read) — it must stay a pure function of
  `(token, pubkey, now)`. That purity is what makes offline scanning
  possible.
- New runtime dependencies without prior issue discussion.

## Licensing

Cackle is dual-licensed [MIT](LICENSE-MIT) OR [Apache-2.0](LICENSE-APACHE).
Contributions inherit that dual licence. No CLA required.
