# Contributing to Cackle

Thanks for helping build ticketing where the gate doesn't fall over when the
network does. All contributions are under the [MIT license](LICENSE).

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
make check   # go vet + go test (scoped to cmd/ and internal/) + full build
cd web && npm run lint && npm test
```

`internal/tickets` is the highest-scrutiny package in the codebase — it is
the whole reason offline gate scanning works. Changes there need tests
covering tamper, wrong key, expired, not-yet-valid, truncated, wrong
version, and replay, per the contract in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

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
- **No hard runtime dependency** on Supabase, Firebase, Vulos Relay, Vulos
  CP, or DMTAP. Cackle must build and run fully standalone.
- **No float money.** Amounts are integer cents, always.
- Making `internal/tickets.Verify` impure (adding a DB call, a network call,
  or an implicit clock read) — it must stay a pure function of
  `(token, pubkey, now)`. That purity is what makes offline scanning
  possible.
- New runtime dependencies without prior issue discussion.

## Licensing

Cackle is MIT-licensed. Contributions inherit MIT. No CLA required.
