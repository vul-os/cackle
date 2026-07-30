# Federation — Boxes That Know Each Other

> **In plain English:** Cackle is software you run yourself, on your own
> machine. Federation is about what happens when two people who both run
> Cackle want their boxes to know about each other — so a venue can show
> the promoter's gigs beside its own, and the promoter can show the
> venue's. Both sides have to say yes, separately, and either can stop at
> any time. **Nobody's box goes looking for strangers.** There is no
> directory, no "organisers near you", no central list of events. If you
> want another organiser's programme on your page, you get their address
> from them — the way you'd get a phone number — and type it in. And when
> somebody buys a ticket for their event, they buy it from **their** box,
> never yours. Most of this chapter describes a design. The section
> headings say what is built and what is not.

Cackle's whole claim is that the thing at the door keeps working when the
network does not, and that the thing running the event belongs to the
person running the event. Federation is what stops that claim quietly
eroding the moment two organisers want to cooperate. The easy version of
cooperation is a central service everyone registers with. That version
would work, and it would make the product's core sentence false. This
chapter is the argument for the harder version, plus an honest ledger of
how much of it exists.

Read [CLUSTERING.md](CLUSTERING.md) first if you have not. Federation
rides the transport that chapter documents, and this chapter does not
repeat it.

---

## 1. What "federation" means here

Four rules. Everything below is a consequence of one of them.

**1. Each host is sovereign.** A Cackle box is owned and operated by one
person or organisation. Nothing above it can add an event to it, remove
one, change a price, or issue a ticket in its name. There is no tier of
the system that is allowed to do those things, so there is nothing to
capture, subpoena, or acquire.

**2. Peering is opt-in in both directions, separately.** "I'll show your
events" and "you may show mine" are two different decisions, and neither
implies the other. Enrolling a peer to reconcile door scans
(`internal/store/migrations/0003_sync_peers_and_op_log.sql`) implies
neither of them either. A default of off is the only honest default for a
switch that publishes your programme to someone else's website.

**3. Tickets are always bought from the publisher's own box.** This is
the rule that keeps the model from collapsing. A borrowed listing carries
a title, a time, a place, whose event it is, and a link. It carries no
price, no ticket type, and no checkout. A ticket is signed by the issuing
organisation's key and verified at that organisation's gate against a
pinned public key ([TICKET-FORMAT.md](TICKET-FORMAT.md)) — there is no
key any other box could sign with, and inventing a way to relay a
purchase would mean inventing custody of somebody else's money and
somebody else's admission. Cackle will not have that. Federation moves
**listings**, not commerce.

**4. Anything one box learns from another is verified, not trusted.**
Arriving over an authenticated connection buys a fact nothing; the same
rule the replication transport already applies to every op
(`internal/scan/substrate/peerauth.go`) applies to anything else that
crosses. A signature you can check against a key a human pinned is the
only reason to believe a peer.

### What federation is not

It is not clustering. Clustering is *your* boxes agreeing about *your*
door. Federation is *your* box and *somebody else's* box agreeing about
what they will show each other. Same transport, different question, and
the second one is a trust decision where the first is a deployment
detail.

---

## 2. What exists today — BUILT

Everything in this section is on `main` and runs.

### The peer channel

Two Cackle nodes can talk to each other, over the routes registered in
`internal/httpapi/deps.go`'s `r.Route("/sync", …)` block (line numbers are
deliberately not cited — this file changes often):

| route | what it is |
| --- | --- |
| `GET /api/sync/ops` | pull a page of the peer's signed admission ops |
| `POST /api/sync/ops` | push this node's ops to the peer |
| `POST /api/sync/peers` | enrol a peer: a name, a pinned public key, an optional URL |
| `DELETE /api/sync/peers/{id}` | un-enrol it |
| `POST /api/sync/peers/{id}/sync` | run one bounded round |

The credential is a **pinned node key signing every request** — no
session, no cookie, no bearer token, no pairing secret, no
trust-on-first-use. Both directions of every exchange are signed, replay
is refused twice (clock skew and a nonce window), and a key that stops
matching its pin is a refusal rather than a re-pin
(`internal/scan/substrate/peerauth.go`). Enrolment is a human typing
another node's key. The durable side is three tables from migration
`0003_sync_peers_and_op_log.sql`.

**What it carries today is admission claims and nothing else.** Not
events, not tickets, not orders, not users, not keys, not images, not
payments. [CLUSTERING.md](CLUSTERING.md) is the operator's guide to it,
including the part where replication makes a cross-gate double admission
**visible sooner and never prevents one**.

### The public page

`GET /h/{ref}` — registered on the root router in
`internal/httpapi/deps.go`, handled in `page_handlers.go` — is the public,
server-rendered, published-only page for one organisation's event. No
script, its own stricter Content-Security-Policy, unauthenticated,
drafts invisible. It is what a stranger sees, and it is already the shape
a federated listing points at: **a link to the publisher's own box.** See
[HOST-PAGES.md](HOST-PAGES.md).

### The shared merge engine

Cackle depends on `github.com/vul-os/kotva/bindings/go v0.2.2` (`go.mod`,
a direct require), imported as `kotvasync` by
`internal/scan/substrate/substrate.go`, `internal/scan/substrate/peerauth.go`,
`internal/httpapi/sync_handlers.go` and
`internal/httpapi/sync_replication.go`. It is worth knowing exactly
what that dependency is, because the shape of it decides what Phase 2
below can and cannot be:

- It is **not cgo.** The binding runs the compiled Rust engine as
  WebAssembly under `wazero`, a pure-Go runtime. Its own `runtime.go`
  says it outright: there is no C toolchain and no shared library. A
  repo-wide search of the module for `import "C"` finds nothing.
- The `.wasm` module is **embedded in the Go module** (`//go:embed
  kotva_sync_abi.wasm`), so there is no download, no sidecar, and no
  version skew between a binary and its engine.
- `CGO_ENABLED=0` and cross-compilation both survive, which is the only
  reason it is allowed near this product at all — see
  [OFFLINE-GATES.md](OFFLINE-GATES.md) for why the gate must stay a
  single static file.

**The engine exposes 72 entry points, and every one of them is sync.**
Enumerated by calling `EntryPoints()` on a live instance: versioning, op
encode/decode, HLC, signing input and envelope assembly, engine ingest
and merge, state root, version vector, the CRDT accessors (LWW cell, set,
counter, sequence, tree), snapshot, fastjoin, fingerprint/summarize/
reconcile, and admission checks. **There is no pub, feed, identity or
key-name entry point.** Not "not wired up yet" — not present in the
module.

> ### ⚠️ The trap: `Instance.ScopeToSubscription`
>
> The Go binding has a method called `ScopeToSubscription`. It looks
> exactly like feed subscription and it is not. It is **sync-namespace
> scoping**: KOTVA `SYNC.md` §7's responder-side sparse-sync filter,
> which takes a list of namespace strings the *caller* is syncing and
> drops ops outside them. Its own doc comment says so. The underlying
> ABI export is `scope_to_subscription`, in the same family as
> `check_ns_ref` and `check_admitted`.
>
> **Do not cite it as evidence that feeds are wired.** It is the single
> most citable-looking wrong thing in this dependency, and a design doc
> that leaned on it would be built on a false claim.

---

## 3. Host display scoping and peer event feeds — check before you cite

Two pieces of work sit between "what exists today" and Phase 2:

- **Host display scoping** — making the public listing say *whose* box
  this is, so a root page cannot read as a global marketplace when it is
  one organiser's server.
- **Opt-in peer event feeds** — letting an enrolled peer, and only an
  enrolled peer whose operator has flipped a per-direction switch, list
  this node's published events and be listed in return, over the peer
  channel in §2.

Both were in flight in the same cycle as this chapter, which is exactly
the situation in which a design document starts telling comfortable
lies. So this chapter makes **no claim about their state**. Check
[CHANGELOG.md](../CHANGELOG.md) for what landed and
[API.md](API.md) for the routes that actually exist; if a route is not in
API.md, assume it is not there. What this chapter *does* fix is the
shape they must hold to: rules 2 and 3 of §1 — two independent switches,
both defaulting to off, and a borrowed listing that links out rather than
sells.

---

## 4. Phase 2 — real KOTVA public objects — DESIGNED ELSEWHERE, NOT BUILT

Peer feeds over the existing channel are a listing exchange between two
nodes that already know each other. The properly-specified version of
"an identity publishes a signed, append-only, anyone-may-serve feed" is
**KOTVA DMTAP-PUB (§22)**, with **DMTAP-PUBSUB (§25)** adding a
subscription object and push hints on top.

### The state of §22/§25 in KOTVA

Honest, and better than you might expect:

- Both are **written as normative spec** (`22-public-objects.md`,
  `25-pubsub.md`) and both are **implemented in Rust** in `kotva-core`:
  `src/pubobj.rs` (~1,700 lines, 27 tests) and `src/pubsub.rs` (~930
  lines, 22 tests).
- §22 has **15 frozen conformance vectors** (`pub_vectors.json`) and a
  genuine two-implementation cross-check: KOTVA's `substrate/ADOPTION.md`
  records both envoir's Rust `pubobj` and kerf-pub's independent Python
  implementation as to-spec and vector-verified against that same file.
  That is a real second pen holding the spec accountable.
- §25 is much earlier: **2 frozen vectors**, against §22's 15.

### Why Cackle cannot adopt it today

**The Go binding does not expose any of it.** §2 above: 72 entry points,
all sync. `pubobj` and `pubsub` are not referenced anywhere in
`kotva-sync-wasm`, the crate the embedded `.wasm` is built from — so
there is no argument to be marshalled, no export to call, and no amount
of Go-side work in this repo that reaches them.

Adopting §22 would require, **in the KOTVA repository and on KOTVA's
release cadence**:

1. Exposing `pubobj` (and, for §25, `pubsub`) through the WASM ABI.
2. Running the frozen `pub_vectors.json` through that binding path — the
   discipline `substrate/BINDINGS.md` §4 already requires of every
   surface, and the only thing that makes "the Go product computes what
   the Rust product computes" a checkable claim instead of an editorial
   one.
3. Cutting a new `bindings/go` release for Cackle to depend on.

**None of that is Cackle work, and none of it should be done here.**
Vendoring a copy, re-implementing §22 in Go, or shelling out to something
else would each produce a second implementation of an algebra whose whole
value is that there is only one.

### The risk, stated rather than buried

Two things that belong in the decision, not in a footnote:

- **A Go adopter would be the first to run §22 through a Go binding.**
  The existing cross-check is Rust ↔ Python, over an HTTP/CBOR wire.
  Neither pen is the marshalling layer a Go binding would add. KOTVA's
  own `ADOPTION.md` documents a follow-up audit that found the same
  ordered-domain defect **four times in four languages**, each invisible
  to that repo's own tests because the local engine agreed with itself.
  That is the exact failure mode a new binding surface invites, and the
  only defence is running the frozen vectors through the new path before
  trusting it — item 2 above, which is why it is not optional.
- **§25 is not ready to be depended on.** Two vectors is a start, not a
  conformance surface.

### `kerf-pub`: read it, never depend on it

`/Users/pc/code/vulos/kerf/packages/kerf-pub` is a working HTTP
implementation of §22 — the five `/.well-known/dmtap-pub/*` endpoints,
deterministic CBOR matching the CDDL key numbers, BLAKE3-256 addressing,
15 of 15 shared vectors passing. As a **blueprint for what the routes and
the wire look like, it is worth reading before designing anything in this
area.**

It must never become a dependency of Cackle:

- It is **Python** (`fastapi`, `asyncpg`), and Cackle ships one static
  Go binary.
- It lives in **another product's repository**, and its own dependency
  list requires `kerf-core` — a sibling package in that same repo. You
  cannot take one without taking the tree.
- It is not published anywhere Cackle could pin a version from.

Depending on it would violate the suite's standalone rule, and it would
also destroy the thing that makes kerf-pub valuable: it is a *second,
independently written* implementation. A dependency is not a second
opinion.

---

## 5. What we are NOT building, and why

This section is the point of the chapter. Everything above is sequencing;
this is a boundary.

### No peer discovery

**Cackle will not ship a way to find a box you were not told about.** No
directory, no index, no DHT, no rendezvous service, no bootstrap list, no
LAN broadcast, no crawler. A node contacts an address a human typed and
no other. [CLUSTERING.md](CLUSTERING.md) already states this for the
replication transport; it is not a property of that transport, it is the
model.

This is not a gap waiting to be filled — **the substrate underneath has
nothing to grow into**:

- The compiled engine Cackle embeds has no transport at all. The crate it
  is built from says it in its own module docs: *no sockets, no HTTP, no
  peer discovery* — the wire protocol is the host's job.
- KOTVA §25's subscription object carries the publisher's identity key as
  a field: you subscribe *to a key you already hold*. It presupposes
  discovery; it does not provide it.
- KOTVA **key-names** are sometimes mistaken for a lookup mechanism. They
  are the opposite: an 8-word encoding **derived from an identity key you
  already have** (`03-naming.md` §3.9.6), offered as the zero-authority
  floor precisely because it needs no resolver and no registry. A
  key-name answers "how do I write this key down", not "who is out
  there". The naming chapter is explicit that a protocol-specific handle
  registry would be a *new authority*, which is the thing the design
  exists to avoid.

### No "organisers near you"

A geographic index of who is running what is the single most requested
shape of this feature and the single most damaging. To answer "events
near me" somebody must hold a list of every organiser, where they are,
and what they are running. Whoever holds that list can rank it, charge
for position on it, and be compelled to hand it over. It is the
marketplace, rebuilt, with the boxes as inventory.

The honest version of "find events near me" is the one that already
exists off the internet: you hear about a venue, you get its address,
you look. Cackle's job is to make that box **work**, not to become the
thing you look in.

### No global or central feed

**A central index is centralization by another name.** It does not matter
that the boxes still hold the data, or that the index is only a cache, or
that it is open source. The moment there is one place every organiser is
expected to appear for anyone to find them, that place is the product,
the boxes are its backend, and the sentence at the top of every page
Cackle publishes — that this is yours and nothing above it can take it —
is false.

So: **if such a service ever exists, it belongs in a separate,
clearly-labelled, opt-in service, and never in this binary.** Separate
repository, separate deployment, separate decision by the operator, and
plainly described as what it is. A Cackle box that its operator never
opted in must be indistinguishable, from the outside, from one that could
not have opted in. That is the test.

### Not TRACT

For completeness, since it is the obvious-looking answer and it is not
one. **TRACT** is KOTVA's commerce profile. As of this writing it is
**~5,810 lines of prose across 23 numbered sections and zero lines of
implementation** — not a workspace member of the KOTVA Cargo workspace,
no crate, no Rust. Its conformance vectors use ASCII placeholders where
real hashes would go (`686169726375742d736572766963652d61646472…` is
`haircut-service-addr` zero-padded), and the word **"ticket" appears zero
times in the entire profile**.

It is a serious document and it may one day be the right substrate for
selling things. It is not a thing Cackle can build on, and it has not
been written with ticketing in mind.

---

## 6. The shape of the whole thing, in one table

| | who decides | what crosses | status |
| --- | --- | --- | --- |
| Gate ↔ its own server | the operator | admission claims | **built** — [OFFLINE-GATES.md](OFFLINE-GATES.md) |
| Node ↔ node, same operator | the operator | admission claims | **built** — [CLUSTERING.md](CLUSTERING.md) |
| Node ↔ node, two operators | both, separately | published listings | see §3 — check [CHANGELOG.md](../CHANGELOG.md) |
| Signed public feeds (§22) | the publisher | signed, servable feed objects | **not built**, and blocked in KOTVA — §4 |
| Subscriptions & push (§25) | subscriber + publisher | subscription objects, hints | **not built**, spec early — §4 |
| Discovery of any kind | — | — | **will not be built** — §5 |
| Selling somebody else's ticket | — | — | **will not be built** — §1 rule 3 |

---

## 7. Related

- [CLUSTERING.md](CLUSTERING.md) — the peer transport this rides:
  enrolment, signing, bounds, and what replication does and does not buy.
- [OFFLINE-GATES.md](OFFLINE-GATES.md) — why the gate is a static binary
  with no network, and why nothing here may change that.
- [HOST-PAGES.md](HOST-PAGES.md) — `GET /h/{ref}`, the published-only
  public page a federated listing links to.
- [TICKET-FORMAT.md](TICKET-FORMAT.md) — why a ticket can only be issued
  by the organisation whose key signs it.
- [ARCHITECTURE.md](ARCHITECTURE.md) — where `internal/scan/substrate`
  sits.
- [ROADMAP.md](../ROADMAP.md) — what is actually queued.
