# Host Pages — giving an organiser their own page, safely

Read this before touching `internal/pages` or the page routes in
`internal/httpapi`. It specifies a wire format and an API precisely enough to
build a client from this document alone, and
[`host-page-vectors.json`](host-page-vectors.json) is the frozen conformance
corpus to check that client against — the same standard
[TICKET-FORMAT.md](TICKET-FORMAT.md) is held to.

An organiser used to get whatever Cackle rendered. Now there are three ways to
have their own page, and they are meant to be used together:

| | What it is | Who it is for |
|---|---|---|
| **1. The default page** | A real page, built from the event record, with zero configuration | Every event, from the moment it is created |
| **2. A host page document** | Structured content + theme, submitted over the API, rendered by Cackle at `/h/{slug}` | A host who wants their own page but not their own website |
| **3. The public API** | JSON, on any origin, that you render yourself | A host who has a website and wants the event on it |

Option 3 is what makes this generic — no format Cackle invents will ever be as
expressive as "your own site" — and options 1 and 2 exist so that having a
website is not a prerequisite for having a decent page.

**Option 4, host-authored HTML, is deliberately not offered.** That is the
substantial decision in this document, so it comes first.

## Why a host cannot upload HTML

The obvious design is: let the host upload HTML, sanitise it, serve it. It is
what most platforms do. Here is why it is refused.

**Cackle is one origin.** The organiser SPA, the JSON API, the session cookie
and (by default) the host page all share it. Two consequences:

- The `cackle_session` cookie is `httpOnly`, which stops a script *reading* it
  but does nothing to stop a script *using* it: the browser attaches it to
  every same-origin request automatically.
- The `cackle_csrf` cookie is deliberately **not** `httpOnly` — the SPA has to
  read it to echo it back in `X-CSRF-Token`. Same-origin script can read it
  too.

So script execution in host content is not "an XSS on a marketing page". It is:
read the CSRF token, issue authenticated same-origin writes, and take over the
account of any organiser who opens the page. In a product where one host's
content is displayed to other hosts' staff, that is a tenancy failure, not a
content bug.

**Sanitising is a bet on a moving target.** An HTML sanitiser has to be correct
against every browser parser quirk that exists now and every one shipped later:
mutation XSS from re-serialisation, `<noscript>`/`<template>`/`<math>`/`<svg>`
foreign-content switching, namespace confusion, encoding tricks. The failure
mode is total, the bypasses are found continuously, and the thing standing
behind the sanitiser is the account of everyone who ever loads the page. Taking
that bet buys inline `<b>` tags.

**"Strict CSP makes it safe" is not enough on its own.** The CSP on host pages
below is strict, and it would in fact stop most of this. But it would be the
*only* thing stopping it, and it is one `'unsafe-inline'` away from stopping
none of it — added by someone shipping a chart library, or a Tailwind change,
or a browser bug in policy enforcement. Note that the app-wide policy in
`internal/httpapi/middleware.go` *already* carries `'unsafe-inline'` for
`style-src`, because the SPA's CSS-in-JS needs it. Policies drift toward
permissive. A design whose safety depends on one never drifting is a design
with a countdown on it.

### What is offered instead

A **document**: a closed vocabulary of typed blocks carrying plain text, and a
theme whose every field is either a six-digit hex colour or a value from a
fixed enum. There is no markup to sanitise because there is no markup — and
therefore no parser to have a bypass in.

This is a real trade. A host gives up inline emphasis, custom layout and
embedded video. What they get back is option 3: the same JSON the default page
is built from, on their own origin, where their own HTML is their own problem
and cannot reach anyone else's session. If a host needs `<b>`, they need their
own page, and Cackle's job is to make that easy rather than to pretend it can
host arbitrary markup safely.

### The two layers, in and out

Every host value crosses two independent gates. Neither is trusted to be the
only one.

**On the way in** — `pages.Validate` refuses anything outside the vocabulary:
unknown block types, fields that do not belong to the block carrying them,
control and bidi characters, colours that are not six hex digits, URL schemes
outside `https`/`http`/`mailto`, and every length cap in the table below.

**On the way out** — the renderer is a single `html/template`, and there is no
`template.HTML`, `template.CSS`, `template.URL` or `template.JS` conversion
anywhere in `internal/pages`. Contextual escaping therefore applies to every
host value in its real context, with no exceptions available:

| Context | What the escaper does to a hostile value |
|---|---|
| Text | `<script>` becomes `&lt;script&gt;` |
| CSS (`<style>`) | A value that is not a valid CSS token becomes `ZgotmplZ` |
| URL (`href`) | An unrecognised scheme becomes `#ZgotmplZ` |
| Attribute | Quotes and angle brackets escaped; the value cannot break out |

A colour that somehow got past validation cannot close the `<style>` element; a
`javascript:` href that got past validation cannot become a working link. Both
are tested by pinning a payload that Validate *does* reject and driving the
template with it directly (`TestRenderHostileThemeNeverEscapesCSS`), so the
second layer is proven independently of the first.

## The rendered page's own policy

`GET /h/{ref}` replaces the app-wide security headers with a much stricter set.

```
Content-Security-Policy: default-src 'none'; script-src 'none';
  style-src 'nonce-<random>'; img-src 'self'; connect-src 'none';
  font-src 'none'; object-src 'none'; media-src 'none'; frame-src 'none';
  form-action 'none'; base-uri 'none'; frame-ancestors 'none';
  sandbox allow-top-navigation-by-user-activation allow-popups
```

Read as a list of things a host page structurally cannot do:

| Directive | What it forecloses |
|---|---|
| `script-src 'none'` | Any script at all. The renderer cannot emit one; this makes that a browser-enforced fact rather than a property of our template. |
| `style-src 'nonce-…'` | Only the page's single inline stylesheet. **Not** `'unsafe-inline'` — the page carries no `style=""` attribute anywhere, so the nonce alone suffices, and a style attribute introduced later fails visibly instead of quietly widening the policy. |
| `form-action 'none'` | A working credential form on Cackle's own origin. This is the anti-phishing directive, and it is why a host page is not merely "safe because there is no script". |
| `img-src 'self'` | Remote images, and the beacons/pixels they double as. |
| `font-src 'none'`, `connect-src 'none'` | Every remaining way to make the browser talk to a third party. Requirement: **no third-party fetch**, enforced by the browser, not just by review. |
| `base-uri 'none'` | Rewriting what relative URLs resolve against. |
| `sandbox …` | See below. |

The nonce is 16 bytes of `crypto/rand` per response, URL-safe base64. (Not
standard base64: `html/template` escapes `+` to `&#43;` inside an attribute, so
a standard-alphabet nonce silently stops matching its own header about three
times in four. There is a test comparing the two values.)

### Origin isolation without a second hostname

The right answer to "one host must not be able to reach another host's
organiser session" is a separate origin. But requiring a second hostname means
a second DNS record and a second TLS certificate before a self-hoster can
publish their first event page, which contradicts working with zero
configuration.

CSP's `sandbox` directive buys most of the same isolation for free. The browser
places the document in an **opaque origin**: no `document.cookie`, no
`localStorage`/`IndexedDB` for the real origin, and no same-origin relationship
with the organiser app — from the same hostname. Two capabilities are granted
back, the minimum a page of links needs:

- `allow-top-navigation-by-user-activation` — a link the visitor *clicks*
  works. A page navigating on its own does not, and cannot: there is no script.
- `allow-popups` — `target=_blank` links open.

Not granted, deliberately: `allow-scripts`, `allow-forms`, `allow-same-origin`,
`allow-modals`, `allow-downloads`, `allow-popups-to-escape-sandbox`.

A browser that ignores the sandbox directive is still left with
`script-src 'none'` and `form-action 'none'`. This hardens the design; it is not
load-bearing for it.

**If you do want a genuinely separate origin**, nothing here prevents it: proxy
`/h/` onto its own hostname and leave everything else on the app's. That is a
deployment choice, it needs no code change, and Cackle assumes neither way. See
[SELF-HOSTING.md](SELF-HOSTING.md) for how the reverse proxy is wired
generally.

## Multi-tenancy

Three properties, each enforced somewhere specific:

1. **A page has no id of its own.** `event_pages` is keyed by `event_id`
   (migration `0005_event_pages.sql`). A page cannot be addressed except through
   an event, an event belongs to exactly one org, and so the RBAC check the
   route already runs — `auth.CanManageEvent`, which resolves the event's org
   itself — is automatically the right check for the page. There is no
   `/api/pages/{id}` route that could forget to re-derive the org, because there
   is no page id to put in one.
2. **A page may only reference its own event's images.** Checked against the
   database before storing, and checked *again* at render time against the
   gallery the renderer is handed. The second check is not redundant: it also
   covers an image deleted after the page was saved, and it means a foreign id
   cannot become a `/media/` URL even if it somehow reached the column.
3. **A draft event has no public page.** For a published event, `GET /h/{ref}`
   and `GET /api/events/{ref}/page` are public. For a draft, both are visible
   **only to an admin/owner of its own org** — a preview, served
   `Cache-Control: private, no-store` — and reported as `404` to everyone else,
   anonymous or not. Never `403`: "this draft exists but you may not see it" is
   itself the leak. The preview is what makes building a page before publishing
   possible; without it the only way to see your own page would be to publish
   the event first.

## The document format

```json
{
  "version": 1,
  "lang": "pt-BR",
  "theme": { "background": "#101014", "accent": "#f2c14e", "direction": "ltr" },
  "labels": { "tickets": "Ingressos", "free": "Gratuito" },
  "blocks": [
    { "type": "heading", "text": "Sobre", "level": 2 },
    { "type": "text", "paragraphs": ["Uma noite só."] },
    { "type": "details" },
    { "type": "tickets" }
  ]
}
```

`version` must be `1`. A document declaring any other version is rejected
rather than coerced — reading a future version as this one would render a page
its author never wrote.

`lang` is an optional BCP-47 tag for `<html lang>`. Omitted means unspecified,
and the attribute is left off rather than guessed.

### `theme`

Every field is optional; omitted means "use the built-in default".

| Field | Grammar |
|---|---|
| `background`, `surface`, `text`, `muted`, `accent`, `accent_text` | `#rrggbb` — exactly six hex digits. No named colours, no `rgb()`/`hsl()`, no 3-digit shorthand, no alpha. |
| `font` | `system` \| `serif` \| `mono` — three locally-resolved stacks. There is no way to name a font file or a font service. |
| `direction` | `ltr` \| `rtl` \| `auto` (default `auto`) |
| `corners` | `sharp` \| `soft` \| `round` (default `soft`) |

Enum matching is exact and case-sensitive: `"SERIF"` is rejected, not folded.

### `labels`

The renderer's own chrome strings, all overridable plain text: `tickets`,
`get_tickets`, `free`, `sold_out`, `when`, `where`, `cancelled`.

Cackle's defaults are English because something has to be. A host running an
event in another language replaces them here rather than being stuck with a
page that is half-translated. Together with `direction` and `lang`, this is how
the page avoids assuming a language — the same way `*_minor` amounts avoid
assuming a currency.

Dates are rendered as `YYYY-MM-DD HH:MM` plus a numeric UTC offset, with the
machine-readable value in `<time datetime>`. Not "Friday 14 August 2026, 7:30
PM", which would hardcode English into every host's page regardless of the
event's own language.

### `blocks`

At most 64, rendered in order.

| `type` | Fields | Renders |
|---|---|---|
| `heading` | `text`, `level` (2 or 3, default 2) | A section heading. Level 1 is the event title and belongs to the renderer. |
| `text` | `paragraphs` (1–24 strings) | One `<p>` per entry. |
| `image` | `image_id`, `alt` (both required) | An image from **this event's own gallery**. `alt` is required — there is no decorative-image escape hatch, because an event page has no decorative images. |
| `links` | `links` (1–24 × `{label, href}`) | A list of links, each `rel="nofollow noopener noreferrer ugc"`. |
| `faq` | `items` (1–32 × `{question, answer}`) | A definition list. |
| `tickets` | `cta_href` (optional) | The event's live ticket types and a call to action. Without `cta_href` the CTA points at Cackle's own checkout for the event. |
| `details` | — | When and where, from the event's own record. |
| `divider` | — | A rule. |

A block carrying a field belonging to a *different* type is rejected —
`{"type":"divider","paragraphs":["…"]}` is an error, not a no-op. Without that
rule a document can hold content nothing displays today, and the day a renderer
starts reading that field it renders text nobody re-reviewed. "What is stored"
and "what is shown" stay the same set.

Unknown JSON fields are an error too, at every level. A host who misspells
`paragraphs` is told, not served a blank section and left guessing.

### Text rules (exact)

Every human-readable string is checked with the same gate.

| Rule | Why |
|---|---|
| Valid UTF-8 | — |
| No control characters (`< U+0020`, `U+007F`) | — |
| No tabs or line breaks, including inside a paragraph | Line structure is expressed by separate paragraphs. Allowing them puts whitespace — and therefore layout — back inside the host's string, which is what this format exists to avoid. |
| No bidi controls: `U+061C`, `U+200E`, `U+200F`, `U+202A`–`U+202E`, `U+2066`–`U+2069` | Trojan Source. The page supports right-to-left properly via `direction`, so there is no legitimate reason to reorder text with invisible characters — and every illegitimate one (a link label that reads as a different URL, a price that reads backwards) is a spoofing primitive on a page other people's buyers trust. |
| No `U+FFFD` | It means something was already lost in transit; storing it freezes a mojibake page. |
| Length caps below | — |

Caps count **characters, not bytes**. A byte cap would silently give a
Latin-script host three or four times the room of a host writing in an
ideographic script — the same class of built-in assumption as a hardcoded
currency symbol.

| Limit | Value |
|---|---|
| Whole document | 65536 bytes |
| Blocks | 64 |
| Heading | 200 characters |
| Paragraphs per `text` block | 24 |
| Paragraph | 2000 characters |
| Links per block | 24 |
| Link/label text | 120 characters |
| `href` | 2048 bytes |
| FAQ items | 32 |
| Question / answer | 300 / 2000 characters |
| `alt` | 300 characters |
| `lang` | 35 bytes |

### `href` rules (exact)

Absolute `https`, `http` or `mailto`, and nothing else.

| Rejected | Because |
|---|---|
| `javascript:`, `data:`, `file:`, any other scheme | — |
| `/admin/orgs` and other relative paths | A link on a public page should say where it goes. |
| `//evil.example/x` | Protocol-relative: an absolute link to somewhere else, wearing a relative link's clothes. |
| `https://cackle.example.org@evil.example/` | Reads as a Cackle URL to a human; resolves to `evil.example`. |
| `https:///path` | No host. |

`html/template` would neutralise a `javascript:` href on the way out anyway.
Validation rejects it on the way *in* so that a host gets told, at submission
time, rather than shipping a page with a dead link.

## The HTTP API

```
GET    /h/{slugOrID}                  HTML page — public once published, organiser-only preview while draft
GET    /api/events/{slugOrID}/page    JSON — same visibility rule as above
PUT    /api/events/{slugOrID}/page    admin+ on the event's org — body IS the document
DELETE /api/events/{slugOrID}/page    admin+ — reverts to the default page
```

`GET` returns:

```json
{ "page": {
    "document":   { "version": 1, "...": "..." },
    "is_default": true,
    "url":        "/h/midnight-set",
    "updated_at": "2026-07-30T09:12:44.221Z"
} }
```

`is_default` reports that `document` is Cackle's zero-configuration default
rather than something a host wrote — a page editor uses it to choose between
"edit" and "start from the default". The default is itself an ordinary,
submittable document: there is no privileged block type only Cackle may use.
`updated_at` is absent for a default page.

`PUT` takes the document as the whole body (a `PUT` of a resource is the
resource) and replies with the same shape `GET` does. Errors use the standard
envelope from [API.md](API.md); a validation failure is `400 invalid_request`
with a message naming the exact JSON path at fault, written to be shown to a
host verbatim:

```json
{ "error": { "code": "invalid_request",
             "message": "pages: invalid document: blocks[2].links[0].href scheme \"javascript\" is not allowed; use https, http or mailto" } }
```

`DELETE` returns `204` and is idempotent. It never leaves an event without a
page — it reverts to the default.

Both management routes accept a slug or an event ULID, and both run RBAC
against the resolved event's org. Authentication is the same as everywhere else
(see [API.md](API.md)): a bearer token, or the session cookie plus
`X-CSRF-Token`.

## Worked example: selling a ticket from your own site

This is the whole flow, using only public routes and no Cackle frontend. Money
is always an integer count of the currency's own minor unit — see
[API.md](API.md#money); `250.00 ZAR` is `25000`, and `25000 JPY` is `25000`
because JPY has no minor unit at all.

**1. Read the event.** No auth.

```http
GET /api/events/midnight-set
```
```json
{
  "event": { "id": "01J8…", "slug": "midnight-set", "title": "Midnight Set",
             "currency": "ZAR", "status": "published",
             "starts_at": "2026-08-14T19:30:00+02:00", "timezone": "Africa/Johannesburg" },
  "ticket_types": [
    { "id": "01J9…", "name": "General", "price_minor": 25000,
      "quantity_total": 200, "quantity_sold": 41, "max_per_order": 6, "status": "active" }
  ],
  "issuer_keys": { "...": "..." },
  "gallery": [ { "id": "01JA…", "url": "/media/01JA…", "width": 1600, "height": 900 } ]
}
```

A ticket type with `price_minor: 0` is a free RSVP. Nothing in this flow
assumes otherwise: the order total is `0`, the provider settles it, and a
ticket is issued exactly as for a paid one.

**2. Optionally read your page document**, if you want to reuse the copy and
theme you set through Cackle rather than duplicating it:

```http
GET /api/events/midnight-set/page
```

**3. Create the order.** No auth required — guest checkout is the normal case.
**Price is never taken from the client**; only `ticket_type_id` and `quantity`
are read, and the server prices from its own `ticket_types` row.

```http
POST /api/orders
Content-Type: application/json

{ "event_id": "01J8…",
  "items": [ { "ticket_type_id": "01J9…", "quantity": 2 } ],
  "buyer": { "email": "buyer@example.org", "name": "A Buyer" } }
```
```json
{ "order": { "id": "01JB…", "status": "pending", "total_minor": 50000, "currency": "ZAR" },
  "payment": { "provider": "manual", "redirect_url": "", "reference": "01JB…",
               "instructions": "…" } }
```

The response carries whatever the configured provider needs: a `redirect_url`
for a hosted checkout, or inline `instructions` for the manual provider (which
is the default — Cackle requires no payment account to run). See
[PAYMENTS.md](PAYMENTS.md).

**4. Confirm payment.** Either the provider's webhook reaches
`POST /api/payments/webhook/{provider}` on its own, or you poll with the
reference you were handed:

```http
POST /api/payments/verify
{ "reference": "01JB…" }
```
```json
{ "order": { "id": "01JB…", "status": "paid", "paid_at": "2026-07-30T09:20:11Z" },
  "tickets": [ { "id": "01JC…", "capability": "cackle.eyJ2Ijox….Ah8f…" } ] }
```

This route **fails closed**: any ambiguity — transport error, unclear provider
status, amount or currency mismatch — is reported as
`402 payment_not_confirmed`, never as paid.

**5. Render the ticket.** `capability` is the signed token specified in
[TICKET-FORMAT.md](TICKET-FORMAT.md). Put it in a QR code and you are done —
that string is what a gate verifies **offline**, against a cached keyring, with
no network at all. See [OFFLINE-GATES.md](OFFLINE-GATES.md).

Nothing in steps 1–5 needs a Cackle-hosted page, so a host who builds against
this owns their entire visitor experience.

## Conformance vectors

[`host-page-vectors.json`](host-page-vectors.json) is the executable half of
this document: fixed documents and the exact outcome each must produce.

```
limits          the caps above, as numbers
block_types     the whole vocabulary
reason_classes  every category of rejection
accept[]        documents that must validate, and round-trip unchanged
reject[]        documents that must fail, with the reason class, the JSON
                path named in the error, and a substring of the message
render[]        document -> HTML, with must_contain / must_not_contain
```

Two expansion rules keep length-limit cases from needing thousands of literal
characters, and they are the *only* preprocessing there is:

1. A string exactly equal to `"@rep:<unit>:<count>"` becomes `<unit>` repeated
   `<count>` times.
2. An array whose only element is `{"@rep": <count>, "value": <v>}` becomes
   `<count>` copies of `<v>`.

To validate a new implementation: run every `accept` vector (must validate, and
its canonical form must re-validate to the same bytes), every `reject` vector
(must fail, naming that path), and — if you render — every `render` vector.

The runner also asserts that the published `limits` equal the limits the code
actually enforces, that the published `block_types` are the whole vocabulary,
and that **every** `reason_class` is exercised by at least one vector. A
validation rule cannot be quietly deleted while the corpus still reports
success.

**These vectors are frozen.** There is no `-update` flag, on purpose — the same
discipline as [TICKET-FORMAT.md](TICKET-FORMAT.md#conformance-vectors). A
change to the format is a `version` bump plus a deliberate regeneration plus a
CHANGELOG entry, not something a test run gets to rubber-stamp.

In this repo the corpus is run by `internal/pages/conformance_test.go`. There is
one runner and not two, unlike the ticket format: the ticket format has a Go
verifier and a JavaScript one that must agree byte for byte, whereas the page
document has exactly one implementation. The corpus is published anyway, because
its audience is a host writing their own client-side validator, and that
implementation will have something to check itself against on the day it exists.

## What this deliberately does not do

- **No arbitrary HTML, CSS or JavaScript.** Explained at length above. If you
  need it, use the API and host the page yourself.
- **No file upload for pages.** Images go through the existing event gallery
  (`POST /api/events/{id}/images`), which already validates by decoding rather
  than by trusting a filename or `Content-Type` — see `internal/media`. Adding a
  second, page-specific upload path would mean a second validator to keep
  correct.
- **No per-page analytics, and no third-party anything.** `connect-src 'none'`
  and `img-src 'self'` are not an oversight. An operator who wants request
  metrics has the server logs, which never record a query string or a cookie
  (`internal/httpapi/middleware.go`).
- **No custom domain per event.** That is a reverse-proxy and certificate
  concern, not an application one.
- **No separate draft state for the page itself.** A page is stored or it is
  not; there is no page-level draft/published pair to keep in sync. The
  *event's* own draft state already provides private-until-ready, and while the
  event is a draft its page is fully editable and previewable by its organiser
  (see Multi-tenancy above).
