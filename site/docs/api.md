# HTTP API

The contract the frontend (and any other client) codes against. All request
and response bodies are JSON. Errors use a consistent shape and the correct
HTTP status:

```json
{ "error": { "code": "invalid_request", "message": "human-readable detail" } }
```

Authentication is `Authorization: Bearer <session>` **or** the httpOnly
cookie `cackle_session` — pick whichever suits your client. Cookie-authed
mutations require CSRF protection per [ARCHITECTURE.md](ARCHITECTURE.md#security-bar).

## Auth

```
POST   /api/auth/signup            {email,password,name} → {user,token}
POST   /api/auth/login             {email,password} → {user,token}
POST   /api/auth/logout
GET    /api/auth/me                → {user, orgs:[{id,name,role}]}
POST   /api/auth/password-reset    {email}
POST   /api/auth/password-update   {token,password}
```

## Events

```
GET    /api/events                 ?q=&from=&to=&limit=   public, published only
GET    /api/events/{slug}          public → event + ticket_types + issuer pubkey
POST   /api/events                 org auth
PATCH  /api/events/{id}
POST   /api/events/{id}/publish
GET    /api/events/{id}/stats      → sold, revenue_cents, admitted, by_type[]
```

`GET /api/events` and `GET /api/events/{slug}` are the only two endpoints in
this table that don't require auth — an event browsing page and a public
event page need to work for an anonymous visitor. Every other event route
requires an authenticated session with a role on the event's org, checked
server-side, every time — see the RBAC rule in
[ARCHITECTURE.md](ARCHITECTURE.md#security-bar).

## Ticket types

```
GET    /api/events/{id}/ticket-types
POST   /api/events/{id}/ticket-types
PATCH  /api/ticket-types/{id}
DELETE /api/ticket-types/{id}
```

## Orders & payments

```
POST   /api/orders                 {event_id, items:[{ticket_type_id,quantity}], buyer}
                                   → {order, payment:{provider,redirect_url,reference}}
GET    /api/orders                 mine
GET    /api/orders/{id}
POST   /api/payments/verify        {reference} → {order, tickets[]}
POST   /api/payments/webhook/{provider}   HMAC-verified, fail-closed
```

`POST /api/orders` creates a pending order and asks the configured
`payments.Provider` to `Begin` a charge — the response carries whatever the
provider needs (a redirect URL for a hosted checkout, or inline
instructions). Once a provider confirms — via the buyer polling
`/api/payments/verify` with the reference it was given, or via the
provider's own webhook hitting `/api/payments/webhook/{provider}` — Cackle
marks the order paid and issues tickets. See [PAYMENTS.md](PAYMENTS.md) for
the full flow and why the webhook route fails closed rather than open on any
verification error.

## Tickets

```
GET    /api/tickets                mine → [{...,capability}]
GET    /api/tickets/{id}
GET    /api/tickets/{id}/pdf
```

`capability` is the signed ticket string described in
[TICKET-FORMAT.md](TICKET-FORMAT.md) — this is what gets rendered as the QR
code an attendee presents at the gate.

## Offline gate

```
GET    /api/events/{id}/scan-bundle  scanner auth → {event, issuer_keys[], ticket_index[],
                                     allocation, issued_at} — everything a gate needs to
                                     run for the whole event with the network unplugged
POST   /api/scan                     {capability, device_id, gate_id, scanned_at}
                                     → {result, ticket, holder}
POST   /api/scan/sync                {admissions:[...]} batch upload of offline scans;
                                     idempotent by (ticket_id, device_id, scanned_at)
```

`POST /api/scan` is the **online** scan path — useful for a gate that does
have connectivity and wants server-side admission recorded immediately
rather than batched. It runs the exact same `internal/tickets.Verify` +
`internal/scan` dedupe logic a fully offline gate runs locally; the only
difference is where the admissions table lives. `scan-bundle` and
`scan/sync` are the offline path, and are the reason this product exists —
see [OFFLINE-GATES.md](OFFLINE-GATES.md) for the full operational guide.

## Error codes

Errors carry a `code` for programmatic handling and a `message` meant for a
human. Expect at minimum: `invalid_request`, `unauthorized`, `forbidden`,
`not_found`, `conflict` (e.g. sold-out ticket type), and `rate_limited`.
Payment and scan endpoints add domain-specific codes documented alongside
their handlers — treat any code you haven't seen as a generic failure rather
than special-casing on an incomplete list.
