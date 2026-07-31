# HTTP API

> **In plain English, and who needs to read the rest of this page:** this is
> the reference for everything Cackle's own web interface asks the server
> to do — create an event, sell a ticket, scan someone in — laid out as raw
> requests and responses. You'd read this if you're building your own
> website that sells tickets against Cackle directly (see
> [HOST-PAGES.md](HOST-PAGES.md#worked-example-selling-a-ticket-from-your-own-site)
> for a worked example of exactly that), writing a script, or auditing what
> the server actually accepts. **If you're just running an event through
> the built-in web pages, you don't need this** — see
> [GETTING-STARTED.md](GETTING-STARTED.md).

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

**Password reset needs an operator, not a mailbox.** `POST
/api/auth/password-reset` mints a single-use, one-hour token and answers
`{"ok":true}` whether or not the address exists — it is deliberately
unable to tell a caller which, so it cannot be used to enumerate
accounts. It also **never returns the token**, and **nothing delivers
it**: Cackle contains no SMTP client, no provider SDK and no sender of
any kind, so a token minted through this route is unreachable by anybody
unless you add delivery yourself. The built-in console therefore does not
call it.

The path that works on a self-hosted box is the operator running, on the
machine itself:

```
cackle reset-password -email someone@example.com
```

which prints `<base-url>/update-password?token=…` to hand over. Same
token machinery, same one-hour single-use expiry, and using it signs that
account out everywhere. An unknown address is an error there rather than
a silent success — an operator with the database in front of them has
nothing to enumerate and needs to be told they mistyped. Add `-db` /
`-base-url` (or `CACKLE_DB` / `CACKLE_BASE_URL`) when the defaults are
wrong; the link must use an address the recipient can actually reach.

### Google sign-in — off unless an operator turns it on

```
GET    /api/auth/providers                  public → {providers:[{id,label,start_path}]}
GET    /api/auth/oauth/{provider}/start     redirects to the provider              only mounted if configured
GET    /api/auth/oauth/{provider}/callback  redirects to /login?sso=<reason>       only mounted if configured
```

`GET /api/auth/providers` always exists and is always public — the
frontend has to be able to ask "what sign-in methods work here" whether or
not anything is configured. On a stock box it answers `{"providers":[]}`,
which is what stops a sign-in button rendering that could never work.

The `/oauth/{provider}/*` routes exist **only on a box whose operator set
both `CACKLE_GOOGLE_CLIENT_ID` and `CACKLE_GOOGLE_CLIENT_SECRET`**
(`google` is the only provider today). Not mounted-and-refusing — not
mounted at all: an unconfigured box 404s `/api/auth/oauth/google/start`
exactly like any other route that doesn't exist. See
[SELF-HOSTING.md](SELF-HOSTING.md#signing-in-with-google-optional-and-off-unless-you-turn-it-on)
for what to paste into the Google Cloud Console and the two environment
variables.

This is organiser sign-in only — nothing on the admission/scanning path
can reach it, and password sign-in keeps working exactly as before whether
or not a provider is configured. `GET .../start` mints a state value and a
nonce and redirects to the provider; `GET .../callback` exchanges the
code, requires the provider to report the email address verified, and
either signs the caller into a matching existing account or creates one —
ending at `/login?sso=ok`, or `denied`/`state`/`unverified`/`failed` on the
paths that don't complete. There is never a token in that URL: the
callback issues the same server-side session, httpOnly cookie, that
password login does.

**Unit-tested, not sandbox-verified.** This code has never been run
against a real Google Cloud OAuth application — see
[SELF-HOSTING.md](SELF-HOSTING.md#what-is-and-is-not-verified) for exactly
what the test suite does and does not prove before you point it at a real
Google project.

## Events

```
GET    /api/events                 ?q=&category=&from=&to=&limit=&host=   public, published only
                                   → {events:[...], host:{scope,name,organisations[],multi_org,peers_included,org?}}
GET    /api/events/{slug}          public → event + ticket_types + issuer pubkey + gallery
POST   /api/events                 org auth
PATCH  /api/events/{id}
DELETE /api/events/{id}            admin+ auth
POST   /api/events/{id}/publish
GET    /api/events/{id}/stats      → sold, revenue_minor, admitted, by_type[]
GET    /api/events/{id}/attendees  ?q=&status=&limit=&offset=   scanner+ auth
                                   → {attendees:[...], total, limit, offset}
GET    /api/orgs/{id}/events       scanner+ auth → {events:[...]}  every event, ANY status
```

### Whose events these are — host scoping

Cackle is self-hosted by one organiser, not a marketplace, so `GET
/api/events` always answers with a `host` envelope alongside `events`,
saying whose box this is:

```json
{ "events": [...], "host": {
    "scope": "own", "name": "", "organisations": [{"id":"…","name":"…","slug":"…"}],
    "multi_org": false, "peers_included": false, "org": null } }
```

`scope`/`name`/`organisations`/`multi_org` mirror `CACKLE_HOST_SCOPE` /
`CACKLE_HOST_ORG` / `CACKLE_HOST_NAME` — see
[CONFIGURATION.md](CONFIGURATION.md#what-your-front-page-shows) for what
each value means to an operator setting them. `?host=<org-slug-or-id>`
narrows the listing to one organisation already named in `organisations`
and echoes it back as `org`.

**An out-of-scope or nonexistent `host` value answers `404` identically —
on purpose, not a bug.** Resolving `?host=` against every organisation on
the box, rather than only the ones already in scope, would turn a display
filter into a way to probe which organisations exist on somebody's server;
read it that way rather than as an oversight. Pinned by
`TestHostScope_HostParamNarrowsToOneOrganisation` and
`TestHostScope_HostParamOutOfScopeIs404` in `internal/httpapi`.

**`peers_included` is `true` under `CACKLE_HOST_SCOPE=peers` and `false`
under `own` and `single`.** It is the operator's answer to "may this host
display borrowed listings publicly" —
`config.HostScope.IncludesPeerEvents()` is `scope == "peers"` and nothing
else.

**It does not mean the `events` array contains any.** That array is this
box's own published events under every scope. A borrowed listing has no
price and cannot be bought here, so it is never merged into a list of things
that are; borrowed listings are read separately from
[`GET /api/peer-events`](#peer-feeds), and `peers_included` is a client's
permission to render what that route returns. `false` means do not display
them, whatever the peer-events route hands back — and a client that cannot
read the envelope at all must degrade to not displaying them, never to
displaying them.

Note the two independent switches: `feed_subscribe`, per peer, decides
whether this box *fetches* a publisher's programme; this scope decides
whether what was fetched is *shown to the public*. Both default to off, so an
operator who has enrolled a publisher and pulled its feed still displays
nothing until they set the scope. Pinned by
`TestHostScope_PeersDisplaysBorrowedListingsAndSaysSo` and
`TestHostScope_SingleDoesNotDisplayBorrowedListings`. See
[Peer feeds](#peer-feeds) below for the routes themselves, and
[FEDERATION.md](FEDERATION.md#3-host-display-scoping--built--and-peer-event-feeds--check-before-you-cite)
for the fuller design this scope sits inside.

**None of this is discovery.** Nothing here finds a peer. There is no
directory, no index, no rendezvous and no proximity lookup anywhere in the
stack; every publisher was enrolled by hand, by key, by an operator.

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

`GET /api/events` is deliberately published-only, even for a caller who is
an admin/owner of the org that drew the draft: it is the public storefront
browse endpoint and must never leak a draft to it. An organiser's own
in-progress events (drafts, and cancelled events, which also never appear
in the public listing) instead show up via `GET /api/orgs/{id}/events` —
every event belonging to the org, regardless of status, most recently
created first. It requires scanner-or-above membership on the org (the
same bar as `stats`/`attendees`/`scan-bundle` — any member has a reason to
see what events exist, not just admins/owners); a member of a *different*
org gets `forbidden`, never a filtered/empty result that could be mistaken
for "this org has no events."

`DELETE /api/events/{id}` requires admin+ on the event's org and hard-deletes
the event, its ticket types, its issuer key(s), and (via cascade) any
orders/order_items that were still `pending` (i.e. abandoned carts that
never actually paid). It is refused with `conflict` if the event has ever
had a ticket issued against it (a ticket only exists once a real order
settled — see [PAYMENTS.md](PAYMENTS.md)), regardless of that ticket's
current status (valid, void, or refunded all count): deleting would either
orphan a real buyer's purchase/admission history or silently erase it, so
Cackle refuses outright. Cancel the event instead
(`PATCH /api/events/{id} {"status":"cancelled"}`) once real tickets exist —
buyers keep their order history, and the event simply stops appearing as
purchasable. An event nobody has ever bought a ticket for (any draft, or a
published event with zero sales) can be deleted outright.

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

## Host pages

```
GET    /h/{slugOrID}                public HTML page for ONE EVENT
GET    /o/{slugOrID}                public HTML page for ONE ORGANISATION,
                                    its published events → 200 text/html
                                    no such organisation on this host → 404 text/html
GET    /api/events/{id}/page        public → {page:{document,is_default,url,updated_at}}
PUT    /api/events/{id}/page        admin+ auth — the request body IS the document
DELETE /api/events/{id}/page        admin+ auth — reverts to the default page
```

Every event has a page from the moment it is created, built from its own
record with zero configuration. A host can replace it with a **document**:
structured, typed blocks carrying plain text plus a constrained theme —
never HTML, never CSS, never a template. `PUT` validates against a closed
vocabulary and answers `400 invalid_request` with a message naming the
exact JSON path at fault; the rendered page is served with its own,
far stricter CSP (`script-src 'none'`, `form-action 'none'`,
`connect-src 'none'`, plus a sandbox) than the rest of the app.

A host who would rather build the page themselves does not need any of
this: `GET /api/events/{slug}` plus the order routes below are enough to
sell a ticket entirely from their own site.

[HOST-PAGES.md](HOST-PAGES.md) has the format, the reasoning for refusing
host-authored HTML, the multi-tenancy rules, and the ticket-purchase flow
as a worked example. [`host-page-vectors.json`](host-page-vectors.json) is
the frozen conformance corpus.

### `GET /o/{slugOrID}` — the organisation page

`/h/` is one event; `/o/` is one organisation, listing that organisation's
**published** events, each linking to its own `/h/` page.
`{slugOrID}` is an organisation slug or ULID, matched against the
organisations already in this host's display scope — the same rule
`GET /api/events?host=` uses. So the `host` query parameter narrows the
**JSON listing**, and `/o/{ref}` is the **page** for the same
organisation, under the same scope rule.

- **It is not a JSON route.** There is no `Accept` negotiation and no
  `/api/orgs/{id}/page`. Do not build a client against one.
- **No authentication, and no session is read at all.** There is no
  draft-preview counterpart to `/h/`'s: an organisation page is a list of
  what is on sale, so there is nothing to build and nothing to preview. A
  draft-only organisation therefore has no page, not even for its own
  owner.
- **The 404 is HTML, not the JSON error envelope**, for the same reason
  `/h/`'s is — a browser following a link is owed a page.
- **The 404 is identical for "no such organisation" and "not shown on this
  host"**: same status, same body bytes, same headers. This is deliberate
  and tested (`TestOrgPage_OutOfScopeOrgIsIndistinguishableFromNonexistent`).
  "Improving" the message to say `no such organisation on this host` would
  reopen an enumeration oracle — the same reasoning as `?host=` above.
- Responses carry the host-page header set (`setHostPageHeaders`): a
  strict per-response-nonce CSP with `script-src 'none'`,
  `form-action 'none'` and the `sandbox` directive, plus
  `X-Robots-Tag: noindex`.
- Under `CACKLE_HOST_SCOPE=single` the configured organisation has a page
  even with nothing published, showing an empty state rather than a 404 —
  see [CONFIGURATION.md](CONFIGURATION.md#what-your-front-page-shows).

Nothing lists or enumerates these pages: it is a link-back destination,
not a directory. See
[HOST-PAGES.md](HOST-PAGES.md#the-organisation-page).

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
POST   /api/orgs                {name,slug?,default_currency?}  → {org:{id,name,slug,default_currency,role}}   any authenticated user
GET    /api/orgs/{id}/members                       → {members:[{user_id,name,email,role}]}   admin+ auth
PATCH  /api/orgs/{id}/members/{user_id}  {role}     → {member:{user_id,name,email,role}}       owner auth
POST   /api/orgs/{id}/invites   {email,role}        → {invite_id,token,expires_at}             admin+ auth
GET    /api/orgs/{id}/invites                       → {invites:[{id,email,role,expires_at,created_at}]}   admin+ auth
DELETE /api/invites/{id}                                                                        admin+ auth
POST   /api/invites/accept      {token}             → {org_id,role}                            any authenticated user

GET    /api/orgs/{id}/bank-account                  → {bank_account:{bank_code,bank_name,account_name,account_number_last4,updated_at}}   owner auth
PUT    /api/orgs/{id}/bank-account  {bank_code,account_number,account_name}   → same shape as GET   owner auth
GET    /api/banks                                   → {banks:[{name,slug,code,currency,active}]}   any authenticated user

GET    /api/events/{id}/payouts                     → {payouts:{gross_minor,fees_minor,net_minor,currency,status,rows:[{id,amount_minor,currency,status,provider_ref,created_at,paid_at}]}}   admin+ auth
```

**Creating an org** is the one route in this section that is not gated on
an existing membership, because there is no org yet to hold a role in.
Any authenticated user may call it and becomes the new org's `owner`; the
membership row it writes names only the org it just created, so creating
an org grants exactly zero authority over any org that already exists
(`internal/httpapi.handleCreateOrg`, regression-tested by
`TestCreateOrg_OwnerOfNewOrgIsStillNonMemberElsewhere`). `name` is
required. `slug` is optional and **normalised, not merely validated** —
`"Neon Nights!"` becomes `neon-nights` — and falls back to a slug derived
from `name` when omitted; a name that reduces to fewer than two usable
characters is `invalid_request` rather than silently given a generated
placeholder. A slug already in use is `conflict`. `default_currency` is
optional and validated against the ISO 4217 table.

**Member role changes** are owner-only — one bar higher than every other
member/invite route in this table (admin+), since a role change can itself
grant/revoke owner-level authority and an admin gate can't be trusted to
police its own ceiling. `role` is one of `owner`, `admin`, `scanner`, same
as invites. It is refused with `conflict` if it would leave the org with
zero owners (demoting/reassigning its one and only remaining owner) —
that would permanently lock everyone out of managing the org (billing,
re-promoting anyone, anything owner-gated) with no way back in, so Cackle
refuses outright rather than allowing it and hoping nobody needed owner
access again. Promote a second member to owner first if the intent is to
step the original owner down.

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

### Delivering an invite — there is no email

**Cackle does not send email.** Not "mail is unconfigured": there is no
SMTP client, no provider SDK and no sender of any kind in this
repository. So `POST /api/orgs/{id}/invites` returning the plaintext
token is not a convenience — **it is the only delivery mechanism there
is**, and a client that discards that response has minted an invite
nobody can ever redeem. The built-in console did exactly that for a long
time while telling the user an invite had been sent, which meant no
organisation could add a `scanner` and therefore nobody could staff a
door.

The flow a client must implement:

```
POST /api/orgs/{id}/invites  {email,role}
  → {invite_id, token, expires_at}

link = <the origin the invitee can reach this server on> + "/accept-invite?token=" + token
```

Show that link to the inviter and let them send it themselves — the same
principle as the `manual` payment provider (see [PAYMENTS.md](PAYMENTS.md)):
no API key, no third party, no compliance surface, works in every country
and on a venue LAN with no internet. The built-in console does this on
the Team page (`web/src/pages/organizers/team/invite-link.js`), including
the copy-to-clipboard fallback for the plain-`http://` case where the
browser Clipboard API is unavailable.

Notes that bind on any client:

- **Show it once.** Only the sha256 hash is persisted, so no route can
  ever return the token again — re-display is not a feature that was
  skipped, it is one that cannot exist without keeping a bearer
  credential in plaintext at rest. `GET .../invites` lists pending
  invites and deliberately carries no token. If a link is lost, `DELETE
  /api/invites/{id}` and issue a new one.
- **State the expiry.** `expires_at` is 7 days out
  (`orgs.DefaultInviteTTL`).
- **Don't persist it.** It is a bearer credential; the console holds it
  in component state and never in browser storage.
- **The link carries the token in a query string**, and the server's
  request logger records `r.URL.Path` and never `RawQuery`, so it does
  not reach the log. `TestInvite_TokenIsReturnedOnceAndNeverLogged` pins
  that.
- The invitee must be signed in as the address the invite names, so a
  forwarded link is useless to anyone else. Tell them that up front
  rather than letting it surface as a `forbidden` after they have already
  made an account.

`internal/httpapi/invite_staffing_test.go` drives the whole path over
HTTP — invite at `scanner`, accept, download a scan bundle, admit a real
capability — and then asserts that same scanner is refused on ten
admin/owner-gated routes.

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
GET    /api/events/{id}/orders     admin+ auth → {orders:[{...order, marked_by?, marked_at?}]}
POST   /api/orders/{id}/mark-paid    admin+ auth → {order, tickets[]}
POST   /api/orders/{id}/mark-failed  admin+ auth → {order}
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

### The organiser side of `manual` — listing and marking orders

`GET /api/events/{id}/orders` is the data source behind the organiser's
Orders screen: every order ever placed against the event, most recent
first. Admin+ on the event's org — one bar higher than the scanner-readable
attendee roster, because this list carries the buyer's email and the
amounts paid, the same bar as event payouts and the org bank account.
Orders settled through the `manual` provider additionally carry
`marked_by`/`marked_at`, read from `manual`'s own audit trail.

`POST /api/orders/{id}/mark-paid` and `POST /api/orders/{id}/mark-failed`
are what make `manual` — Cackle's zero-config, no-API-key, no-network-call
default provider (see [PAYMENTS.md](PAYMENTS.md)) — actually usable end to
end: an organiser who received a bank transfer, cash at the door, or a paid
invoice records that here. Marking paid runs the exact same
settle-and-issue-tickets path a real provider's webhook or verify poll
would, and is idempotent — calling it twice returns the tickets already
issued rather than issuing a second set. Marking failed releases the
order's reserved inventory back to sale. Both are refused on any order that
was not created with `manual`; every other provider settles exclusively
through its own `verify`/`webhook` above.

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
                                     ticket_index_present, admitted_index[], allocation,
                                     issued_at} — everything a gate needs to run the whole
                                     event offline. `allocation` is always null
                                     (unbuilt — see OFFLINE-GATES.md)
POST   /api/scan                     {event_id, capability, device_id, gate_id, scanned_at}
                                     → {result, ticket, holder}
POST   /api/scan/sync                {admissions:[...]} batch upload of offline scans;
                                     idempotent by (ticket_id, device_id, scanned_at)
GET    /api/events/{id}/admission-conflicts
                                     scanner auth → tickets that MORE THAN ONE DEVICE
                                     claimed to admit, i.e. what got through two doors
                                     while the gates were partitioned
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
signature verifies but whose `tid` is absent from an **authoritative**
index is reported `result: "invalid"` — this is what stops a refunded
ticket from being admitted purely on the strength of its signature.
`ticket_index_present` says whether the index is authoritative: the server
always sets it `true` (it queried the current valid set to build the
bundle), so an **empty** authoritative index means *admit nothing* — every
ticket voided/refunded, or none issued — **not** "no data". Only a legacy
bundle carrying `ticket_index_present: false` falls back to signature-only
checking. Distinguishing "present but empty" from "absent" is deliberate:
inferring it from length alone would silently re-admit every physically-held
ticket for a fully-cancelled event. Even a fresh `ticket_index` is only a
snapshot as of `issued_at`: a ticket refunded after a gate downloaded its
bundle stays admittable at that gate until it re-syncs. See
[OFFLINE-GATES.md](OFFLINE-GATES.md) for the full reasoning.

`admitted_index` is the set of ticket IDs that already have a recorded
admission as of `issued_at` — the server's reconciled "these people are
already inside". A capability that verifies, is in `ticket_index`, but appears
here is reported `result: "duplicate"` with reason "ticket already admitted at
another gate". It is the **only** channel by which one gate learns about an
admission at a *different* gate, and it narrows the double-scan window on every
re-pull without ever closing it: two gates that cannot see each other cannot be
prevented from admitting the same ticket. Unlike `ticket_index` it has no
`_present` flag, because empty and absent mean the same unambiguous thing
("nobody known to be inside") and both correctly defer to the device's local
log.

`POST /api/scan/sync`'s per-item `result` is the **device's own** verdict and
is stored unrewritten in `admissions.reported_result`, even when the server
downgrades the row's `result` to `duplicate` because another gate's admission
was already recorded. Send what your gate actually did at the door; sending
`duplicate` for a scan you admitted erases the only evidence a double
admission happened.

`GET /api/events/{id}/admission-conflicts` reports each contested ticket with
`devices`, `extra_admissions`, and every gate's claim (`result` = what the
device did, `server_result` = what the server concluded, present only when they
differ). It is an after-the-fact record and never a guard, and it is only as
complete as the sync — every response carries a `caveat` string and a
`complete` flag saying so, and an empty `conflicts` list means "no conflict
among the claims that reached this server", never "no double admissions
happened". The merge runs on the shared DMTAP Sync algebra
(`internal/scan/substrate`), and the response names the `algebra` and `engine`
it merged under. See [OFFLINE-GATES.md](OFFLINE-GATES.md).

## Peer feeds

> **In plain English:** two organisers who already know each other and both
> run Cackle can let each other's box show their published events — a venue
> showing a promoter's upcoming gigs, a promoter showing a venue's. Each
> direction is its own yes/no switch, **off by default**, and either side
> can turn theirs off again at any time. Cackle never goes looking for a
> box on its own: you get the other one's address from the person who runs
> it, the way you'd get a phone number, and enrol it by hand — see
> [CLUSTERING.md](CLUSTERING.md). A borrowed listing is a signpost, never a
> ticket: buying always still happens on the publisher's own box.

```
GET    /api/sync/feed                 peer-authenticated (signed envelope, same
                                      credential every /api/sync route uses)
                                      → {node, events:[...], complete, caveat}
PUT    /api/sync/peers/{id}/feed      owner auth  {publish,subscribe} → sync peer view
POST   /api/sync/peers/{id}/feed      owner auth → {peer_id,peer,fetched,stored,
                                      refused[],complete,error,caveat}
GET    /api/sync/peers/{id}/feed      owner auth → {events:[...], caveat}
GET    /api/peer-events?org=<org_id>  public → {events:[...], caveat}
```

This rides the peer channel [CLUSTERING.md](CLUSTERING.md) documents in
full — enrolment (`POST /api/sync/peers`), the pinned node key, the signed
envelope — and adds nothing to that trust model. `/api/sync/ops`,
`/api/sync/status`, `/api/sync/peers` and `/api/sync/peers/{id}/sync`
(enrolling a peer and running an admission-replication round) are
documented [there](CLUSTERING.md#setting-it-up) rather than repeated here.

**Off by default, and that matters on an upgrade.** Enrolling a peer to
reconcile door scans grants neither feed direction: a `sync_peer` row
carries two **separate** switches, `feed_publish` and `feed_subscribe`
(`internal/store/migrations/0007_peer_event_feeds.sql`), and **both
default to `0`.** An operator who upgrades an already-clustered deployment
publishes nothing and is shown nothing from any peer until they turn a
switch on for each one, on purpose.

`GET /api/sync/feed` is the publish side, and it is a **peer route, not an
operator route**: the caller authenticates with the same signed envelope
every other `/api/sync` route uses, and is additionally refused
(`forbidden`) unless at least one of its enrolments has `feed_publish` set
— enrolment alone is not consent to be listed elsewhere. It answers this
node's own **published** events, **capped at 200 with no pagination**: a
publisher with more than 200 published events serves the soonest 200 and
reports `"complete": false` — there is no further page to ask for, ever,
on this route. Each event carries `event_id`, `slug`, `publisher` (this
node's org name, untrusted display text), `title`, `summary`, `venue_name`,
`address`, `starts_at`, `ends_at`, `timezone`, `category` — deliberately no
price, no ticket type, no capacity, so this route cannot become a
cross-node checkout even by accident.

The other three routes are ordinary owner-session routes, one per enrolled
peer:

- `PUT /api/sync/peers/{id}/feed` sets both switches in one call — the
  body must state both (`{"publish":true,"subscribe":false}`); a request
  that omits either is refused rather than treated as "leave it alone", so
  a stale form can never hold a switch open by silence. Turning
  `subscribe` off **deletes that peer's cached listings in the same
  call** — withdrawing consent removes what was already shown, not just
  future refreshes.
- `POST /api/sync/peers/{id}/feed` triggers one fetch of that peer's feed
  right now and replaces the cache wholesale. **Nothing polls** — the same
  discipline [CLUSTERING.md](CLUSTERING.md) holds replication to: a
  deployment that never calls this never opens a socket for a feed. Every
  refusal (no address, the peer disabled, `feed_subscribe` off) happens
  *before* any network call, so a peer nobody consented to is never
  contacted even once.
- `GET /api/sync/peers/{id}/feed` reads back what the last pull actually
  cached, for the operator screen that shows it.

`GET /api/peer-events?org=<org_id>` is the public read: every listing
currently borrowed into that organisation, from every peer it subscribes
to. Public, and it makes no network call of its own — it only reads this
node's cache, which is filled solely by an operator-triggered pull —
because every row in it is an event its own publisher already made public
on their own site; hiding a copy of already-public data behind auth would
not make it any less public. Each row carries `external: true`,
`publisher`/`publisher_key` (the pinned key it came from — the actual
identity; `publisher` itself is untrusted display text), `url` (composed
**on this node** from the peer's enrolled origin plus a validated slug,
never taken verbatim from the peer's own answer), and a `notice` string
every renderer must show beside the listing: *"Hosted by another
organiser. Tickets are sold on their own site, not here."* There is no
price and no ticket type anywhere in this shape — see
[FEDERATION.md](FEDERATION.md#1-what-federation-means-here) rule 3.

**Rows from this route are not permission to display them.** That is the
second switch, and it lives on the host envelope: show borrowed listings on
a public page only when `GET /api/events` reports
`"peers_included": true` ([above](#whose-events-these-are--host-scoping)).
An operator can subscribe to a peer, pull it, and still be running the
default `own` scope, in which case this route answers rows and the front
page must show none of them. Cackle's own browse page holds that line in
`web/src/pages/visitor/events/peer-scope.js`, gated on the single envelope
reader in `web/src/lib/host.js`; a client that skips it is publishing
another organiser's programme on an operator's page without being asked to.

Every answer on this surface — publish, pull, and the read-back — carries
a `caveat` string repeating that these are somebody else's events, hosted
on their own box, and that Cackle finds no peers on its own. It is not
boilerplate: it is the sentence this feature depends on a renderer never
dropping, so treat it as load-bearing text if you're building a client
against this.

## Error codes

Errors carry a `code` for programmatic handling and a `message` meant for a
human. Expect at minimum: `invalid_request`, `unauthorized`, `forbidden`,
`not_found`, `conflict` (e.g. sold-out ticket type), and `rate_limited`.
Payment and scan endpoints add domain-specific codes documented alongside
their handlers — treat any code you haven't seen as a generic failure rather
than special-casing on an incomplete list.
