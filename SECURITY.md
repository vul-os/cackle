# Security Policy — Cackle

Cackle handles payment references, PII (attendee names/emails), and the
private keys that back every ticket for an event. Security reports are taken
seriously and handled with priority.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

- Preferred: [GitHub private vulnerability reporting](https://github.com/vul-os/cackle/security/advisories/new) on `vul-os/cackle`.
- Alternatively, email **security@vulos.org** with `[cackle security]` in the subject.

Include what you can: affected component (ticket signing, scan/admission,
payments, auth, storage), reproduction steps, and impact as you understand
it. You'll get an acknowledgement within **72 hours** and a status update at
least every **14 days** until resolution. Please give us a reasonable window
to ship a fix before public disclosure — confirmed reporters are credited in
the release notes unless they'd rather stay anonymous.

## Scope

### In scope

- **Ticket capability signing and verification** (`internal/tickets`) — any
  forgery, signature-bypass, or key-confusion path (wrong event's key
  accepted, expired/not-yet-valid tickets admitted, truncated or malformed
  tokens parsed as valid). This is the highest-value target in the codebase:
  see [docs/TICKET-FORMAT.md](docs/TICKET-FORMAT.md).
- **Gate admission and offline dedupe** (`internal/scan`) — replay across
  devices, duplicate admission, or a way to make the sync endpoint
  double-admit a ticket that was already scanned offline.
- **Event key custody** (`event_keys` table) — exposure of a private signing
  key, or a way for one event's key material to leak into another event's
  scope.
- **Payments** (`internal/payments`) — webhook signature bypass, replay,
  amount/currency confusion, or anything that could mark an order paid
  without a verified provider confirmation.
- **Auth & sessions** (`internal/auth`) — password handling, session token
  generation/storage, CSRF on cookie-auth mutations.
- **Authorization** (`internal/httpapi`) — any org/event route missing a
  server-side role check (the old app shipped at least one unprotected
  `/admin/events/:id/payouts`-style route — that class of bug is exactly
  what this policy exists to catch).

### Out of scope

- Third-party Go and npm dependencies — report to upstream maintainers.
- Social engineering or phishing.
- Denial-of-service via request flooding (operational concern, not a code
  vulnerability).
- Vulnerabilities in the underlying OS, browser, or payment provider (Paystack)
  outside our control.

## Supported versions

Pre-1.0: only the latest release (and `main`) receives fixes.

## Safe Harbor

We commit to not pursuing legal action against researchers who:

- Act in good faith to identify and report vulnerabilities.
- Do not exploit beyond demonstrating the issue.
- Do not access, modify, or exfiltrate real attendee data or payment
  references.
- Do not disrupt a live event's ticket sales or gate scanning.
- Disclose to us before public disclosure.

## Bug Bounty

No paid bug-bounty program at this time. Confirmed reporters are credited in
the release that ships the fix.
