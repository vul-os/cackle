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
GET    /api/events                 ?q=&category=&from=&to=&limit=   public, published only
GET    /api/events/{slug}          public → event + ticket_types + issuer pubkey + gallery
POST   /api/events                 org auth
PATCH  /api/events/{id}
POST   /api/events/{id}/publish
GET    /api/events/{id}/stats      → sold, revenue_minor, admitted, by_type[]
GET    /api/events/{id}/attendees  ?q=&status=&limit=&offset=   scanner+ auth
                                   → {attendees:[...], total, limit, offset}
```

### Money

Cackle is country- and currency-agnostic: there is no privileged currency,
and **"cents" is not a universal truth** — ISO-4217 currencies do not all
have two decimal places (JPY/KRW/VND/CLP/ISK have zero, KWD/BHD/JOD/OMR/TND
have three). Every money field in this API is named `*_minor` (never
`*_cents`) and is an integer count of the currency's own minor unit —
`ticket_types[].price_minor`, `order.subtotal_minor`/`fee_minor`/
`total_minor`, `order.items[].unit_price_minor`, `stats.revenue_minor`,
`payouts.gross_minor`/`fees_minor`/`net_minor`. A money field is always
accompanied by a `currency` (ISO-4217 alpha-3) somewhere in the same
response — on the `event` object for ticket types/orders/stats, and
directly on `payouts`/each payout row — never assume a currency or an
exponent; look it up via [Currencies](#currencies) if you need to convert
a `*_minor` integer to a decimal string yourself. An event's `currency`
defaults from its owning org's `default_currency` when not set explicitly
at creation (`POST /api/events`); once set, changing it (`PATCH
/api/events/{id}`) only affects orders placed after the change.

Every `event` object carries `category` (a normalised slug — see
[Categories](#categories) — empty string if uncategorised) and
`cover_image_id` (the id of an image in the event's own gallery chosen as
its cover, omitted if none is set). `PATCH /api/events/{id}` accepts both
as ordinary partial-update fields: `category` is free text, normalised to
a slug server-side ("Live Music!" → `"live-music"`); `cover_image_id` set
to `""` clears the cover, set to an existing image id from **this
event's own gallery** sets it (any other id — another event's image, or
one that doesn't exist — is rejected `invalid_request`). `GET
/api/events/{slug}` additionally returns `gallery: [{id,url,width,height}]`
— every image uploaded to the event via [Images](#images), in upload
order; the list endpoint does not include galleries (keep the public
browse response lean).

`GET /api/events` and `GET /api/events/{slug}` are the only two endpoints in
this table that don't require auth — an event browsing page and a public
event page need to work for an anonymous visitor. Every other event route
requires an authenticated session with a role on the event's org, checked
server-side, every time — see the RBAC rule in
[ARCHITECTURE.md](ARCHITECTURE.md#security-bar).

`GET /api/events/{id}/attendees` is the organiser-facing ticket-holder
roster — every issued ticket for the event, one row per ticket, with the
holder's name, ticket type, serial, order id, issue time, and admission
status/time. It requires scanner-or-above membership on the event's org
(the same bar as `stats` and `scan-bundle`): the door team needs this list
as much as the organiser does. `q` matches holder name (substring,
case-insensitive); `status` is one of `valid`, `void`, `refunded` (ticket
status) or `admitted`, `not_admitted` (gate status) — an unrecognised
value returns zero rows rather than the unfiltered roster. `limit`
defaults to 50 and is hard-capped at 200 regardless of what's requested,
so a large event's roster can never be pulled as one unbounded response.
The response never includes the buyer's email — see
[ARCHITECTURE.md](ARCHITECTURE.md#security-bar) if that seam changes.

## Images

```
POST   /api/events/{id}/images     multipart field "file" → {id,url,width,height}   admin+ auth
DELETE /api/images/{id}                                                             admin+ auth
GET    /media/{id}                 public, serves the stored image bytes
```

Images are validated by **magic bytes and a full decode of the pixel
data** — never by the client's claimed filename or `Content-Type`, both of
which are ignored entirely. Only `png`, `jpeg`, and `webp` are accepted;
anything else (including a real image format not on that list, e.g. GIF)
is rejected `invalid_request`. Files are capped at 8MB. Every accepted
image is re-encoded (png/jpeg) or has its metadata chunks surgically
stripped (webp, which this build cannot losslessly re-encode without a new
codec dependency) so EXIF/XMP never survives an upload — see
[`internal/media`](../internal/media/media.go) for the full approach,
including the pixel-count bound that guards against a decompression bomb.

The server generates its own opaque, random storage id for every upload
(a ULID) and that id — never anything client-supplied — is what
`{id,url,width,height}` returns and what the on-disk filename is derived
from; a client can never influence the storage path. `url` is
`/media/{id}`, servable directly in an `<img src>` with no auth. `GET
/media/{id}` sets `Cache-Control: public, max-age=31536000, immutable` —
an image id is never mutated or reused in place; delete removes the row
and file outright rather than ever replacing bytes at the same id.

Deleting an event's chosen cover image (`events.cover_image_id`) via
`DELETE /api/images/{id}` clears that reference automatically at the
database level — no separate call needed.

## Categories

```
GET    /api/categories              → {categories:[{slug,label,count}]}
```

Public, no auth. Derived from currently **published** events only (a
category with zero live events isn't worth a browse-page tab) —
uncategorised events are excluded. `slug` is the normalised value stored
on `events.category` and the value `GET /api/events?category=` filters
on; `label` is a human-friendly reconstruction (`"live-music"` →
`"Live Music"`); `count` is how many published events currently carry
that category.

## Currencies

```
GET    /api/currencies              → {currencies:[{code,name,exponent}]}
```

Public, no auth. The full ISO-4217 table `internal/money` knows about
(150+ currencies) — this is what an event-creation currency picker should
source its options from, not a hardcoded shortlist. `exponent` is how many
digits follow the decimal point in that currency's major-unit display (0
for JPY, 3 for KWD, 2 for most others) — the authoritative source for
converting any `*_minor` field in this API to/from a decimal amount.

## Ticket types

```
GET    /api/events/{id}/ticket-types
POST   /api/events/{id}/ticket-types
PATCH  /api/ticket-types/{id}
DELETE /api/ticket-types/{id}
```

## Org management

```
GET    /api/orgs/{id}/members                       → {members:[{user_id,name,email,role}]}   admin+ auth
POST   /api/orgs/{id}/invites   {email,role}        → {invite_id,token,expires_at}             admin+ auth
GET    /api/orgs/{id}/invites                       → {invites:[{id,email,role,expires_at,created_at}]}   admin+ auth
DELETE /api/invites/{id}                                                                        admin+ auth
POST   /api/invites/accept      {token}             → {org_id,role}                            any authenticated user

GET    /api/orgs/{id}/bank-account                  → {bank_account:{bank_code,bank_name,account_name,account_number_last4,updated_at}}   owner auth
PUT    /api/orgs/{id}/bank-account  {bank_code,account_number,account_name}   → same shape as GET   owner auth
GET    /api/banks                                   → {banks:[{name,slug,code,currency,active}]}   any authenticated user

GET    /api/events/{id}/payouts                     → {payouts:{gross_minor,fees_minor,net_minor,currency,status,rows:[{id,amount_minor,currency,status,provider_ref,created_at,paid_at}]}}   admin+ auth
```

**Invites** are single-use and expiring (7 days): the token is 32 random
bytes, and only its sha256 hash is ever persisted — the plaintext value in
`POST .../invites`'s response is the only time it is ever available,
mirroring how session and password-reset tokens already work in
`internal/auth` (which is where the token-minting primitive itself lives,
`auth.NewOpaqueToken`/`HashOpaqueToken`, shared rather than
reimplemented). `POST /api/invites/accept` additionally requires that the
**calling account's own email matches the address the invite was issued
to** — token possession alone is not sufficient, so a forwarded link
cannot be redeemed by the wrong account; a mismatch is `forbidden`, not
`invalid_request`. Accepting adds (or updates) the caller's membership at
the invite's role; accepting twice, or after expiry, or after the invite
was deleted, is `invalid_request`.

**Bank account** details are masked on read — `account_number_last4` only,
never the full number — and the full number is never written to a log
line anywhere in this path. If a live Paystack secret is configured (see
[PAYMENTS.md](PAYMENTS.md)), `PUT .../bank-account` registers a transfer
recipient with Paystack first and only persists locally once that
succeeds (a bad `bank_code`/`account_number` is rejected with the
provider's own error, not silently stored); `GET /api/banks` returns
Paystack's live South African bank list. Without a live secret configured
(self-host or `--demo` with no Paystack account) both endpoints still
work: `GET /api/banks` returns a small built-in fallback list of major
South African banks (using Paystack's own published codes, so a later live
PUT against the same code succeeds unmodified once a real provider is
configured), and the bank account is stored locally with no live recipient
reference — this is a supported configuration, not a degraded error state.

**Payouts** is a read-only projection, not a "trigger a transfer" endpoint
— there is no POST here. `gross_minor`/`fees_minor` are summed from the
event's own **paid** orders only (`subtotal_minor`/`fee_minor`), the same
"paid orders, never the reservation counter" discipline `GET
/api/events/{id}/stats` already follows; `net_minor` is gross minus fees.
`currency` (both at the top level and on each row) is always the owning
event's own currency — a payout moves exactly the money that event
collected; Cackle never converts currencies. `rows` lists every payout
record ever created against the event (empty
until a real payout pipeline writes one); `status` is the most recent
row's status if any exist, otherwise `"unpaid"` once there is revenue to
pay out or `"no_sales"` if there is none yet. This route is exactly the
one the original app shipped with **no protection at all**
(`/admin/events/:id/payouts`) — it is admin+-gated here and covered by
`internal/httpapi/rbac_test.go`'s table so that mistake can't repeat
silently.

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
`internal/scan` dedupe logic a fully offline gate runs locally (including
the `ticket_index` revocation check below); the only difference is where
the admissions table lives. `scan-bundle` and `scan/sync` are the offline
path, and are the reason this product exists — see
[OFFLINE-GATES.md](OFFLINE-GATES.md) for the full operational guide.

`ticket_index` is the set of ticket IDs currently valid (issued, not void,
not refunded) for the event, as of `issued_at`. A capability whose
signature verifies but whose `tid` is absent from a non-empty
`ticket_index` is reported `result: "invalid"` — this is what stops a
refunded ticket from being admitted purely on the strength of its
signature. An empty/absent `ticket_index` (older bundle, or an event with
no tickets issued yet) is a deliberate fallback to signature-only
checking, not "reject everyone" — and even a fresh `ticket_index` is only
a snapshot as of `issued_at`: a ticket refunded after a gate downloaded its
bundle stays admittable at that gate until it re-syncs. See
[OFFLINE-GATES.md](OFFLINE-GATES.md) for the full reasoning.

## Error codes

Errors carry a `code` for programmatic handling and a `message` meant for a
human. Expect at minimum: `invalid_request`, `unauthorized`, `forbidden`,
`not_found`, `conflict` (e.g. sold-out ticket type), and `rate_limited`.
Payment and scan endpoints add domain-specific codes documented alongside
their handlers — treat any code you haven't seen as a generic failure rather
than special-casing on an incomplete list.
