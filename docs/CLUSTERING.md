# Running More Than One Node

> **In plain English:** this is for the case where you're running **two**
> Cackle servers for the same event — say, a laptop at the venue and a
> server in the cloud — and want them to compare notes on who's already
> been let in. It's optional, off by default, and it only makes duplicate
> admissions **visible sooner**, across more places; it does not make a
> duplicate at two offline doors impossible — nothing can, and this page
> says exactly why. Running a single Cackle server, which is what
> [GETTING-STARTED.md](GETTING-STARTED.md) walks through, needs none of
> this.

Cackle can replicate its **admission ledger** between servers: a venue node and a
cloud node, two venue nodes, a box-office laptop and the machine in the rack.
Each one keeps admitting people whether or not the others are reachable, and when
they can talk, they agree on who came through which door.

This is the newest thing in the product and the shortest chapter, because there
is not much to it. Read the first section before you plan around it.

## What this buys you, stated exactly

**It makes a cross-gate double admission visible sooner, and on more nodes. It
does not prevent one.**

That sentence is the whole feature. [OFFLINE-GATES.md](OFFLINE-GATES.md#what-offline-double-scan-protection-actually-gives-you)
explains why prevention is impossible: two gates that cannot see each other at
the moment of a scan will both open the door, and nothing that happens afterwards
reaches back to that moment. Replication is "afterwards". It is a faster, wider
`admission-conflicts` report, not a guard.

What changes concretely:

- A claim recorded at the venue node reaches the cloud node, so the conflict
  report is the same wherever you read it.
- A ticket admitted at one node comes back down to the *other* node's gates in
  their next `scan-bundle` (`admitted_index`), so the window in which two gates
  can disagree gets narrower — the same mechanism, and the same limits, as a
  gate re-pulling its bundle.
- A node's database is no longer the only copy of what its gates reported.

What does not change: the number of people already inside on one ticket.

## What replicates, and what does not

Only admission claims. There is no replication of events, tickets, orders,
users, keys, images or payments, and this is not a backup.

The practical consequence is worth being blunt about: **two nodes only converge
on an event they both already hold.** A claim about a ticket a node does not have
is stored and reported but cannot be counted (it shows up as `unapplied` in the
status view); a claim about an *event* a node does not have is refused outright,
with a message saying so.

So the deployment that works is one where the nodes start from the same data —
typically because the second node was brought up from a copy of the first, or
because both were seeded from the same source. If you copy a database, **delete
the `sync_node_identity` row on the copy** (see "Identity", below) or you will
have two nodes signing as the same node.

### Capacity allocations are deliberately not part of this

`internal/scan/allocation.go` is a signed grant that lets a disconnected
sub-issuer *mint* up to N tickets offline. It is implemented and tested, and it
is **not wired to anything** — no route issues one, no bundle carries one
(`scan.Bundle.Allocation` is always `nil`), and nothing writes the `allocations`
table. Delegated issuance is roadmap work; see
[ROADMAP.md](../ROADMAP.md).

It stays an empty seam, and replication is not the thing that should fill it:

- There is nothing to replicate. No node produces an allocation, so putting one
  on this transport would mean shipping an empty set and then inventing a
  producer to justify the shipping.
- A capacity cap is a **coordination** property, not a merge rule. "The total
  minted across every sub-issuer stays under N" cannot be enforced by any
  after-the-fact union, for exactly the reason two partitioned gates cannot be
  stopped from admitting one ticket twice. Replicating grants would create the
  appearance of an enforced cap and enforce nothing — the one illusion this
  codebase is most careful not to sell.
- When delegated issuance is built, the honest mapping is the one the admission
  ledger already uses: replicate the **mint facts** ("sub-issuer S minted ticket
  T") as add-only-set elements, so over-issuance becomes *detectable* after the
  fact in the same way a double admission is. That needs no change to this
  transport — an op is an op — which is a reason to leave the seam alone rather
  than a reason to fill it early.

## Setting it up

Two nodes, `venue` and `cloud`. Everything below is done by a human, once. There
is no discovery, no directory, no default endpoint and no LAN broadcast: a node
talks to exactly the nodes an operator typed in.

### 1. Read each node's key

On each node, as an **owner** of the organisation you want to replicate:

```
GET /api/sync/status?org=<org_id>
```

```json
{
  "node": "3f9c…64 hex characters…",
  "algebra": "dmtap-sync-v0",
  "engine": "…",
  "peers": [],
  "op_log": { "ops": 0, "unapplied": 0, "highest_seq": 0, "pending": 12 },
  "standalone": true,
  "caveat": "Replication makes a cross-gate double admission visible sooner…"
}
```

`node` is this node's replication identity — an Ed25519 public key. It is
created the first time you ask for it and never before: a node with no peers
holds no replication key material at all.

### 2. Enrol each node on the other

```
POST /api/sync/peers
{ "org_id": "…", "name": "cloud", "url": "https://cackle.example.org",
  "public_key": "<the other node's key>" }
```

Owner role required, and the enrolment is **scoped to one organisation**: that
peer will see the admission ledger of that organisation's events and nothing
else. Enrol a peer once per organisation you want it to see.

`url` is optional, and leaving it out is a real configuration rather than an
omission:

| `url` set | meaning |
| --- | --- |
| yes | this node dials that peer — it pushes its ops there and pulls the peer's back |
| no | **inbound only**: this node can never reach that peer and will never try; the peer dials in |

That is how a venue node behind NAT works. The venue enrols the cloud node
**with** its URL; the cloud node enrols the venue node **by key alone**. The
venue drives both directions and nothing ever tries to dial an address that
cannot be reached.

The URL must be an origin with no path (`https://host:port`). Cackle serves
`/api` at the root, and the signature a peer verifies covers the request path —
so a prefix-stripping reverse proxy would make every request fail as a signature
error. Enrolment refuses the URL instead, which is a clearer sentence at a
better time.

### 3. Run a round

```
POST /api/sync/peers/{peer_id}/sync
```

One bounded round: mint this node's new claims into signed ops, push them, pull
the peer's, verify each one, store it, apply it.

```json
{ "peer_id": "…", "peer": "…", "minted": 2, "pushed": 2, "push_held": 0,
  "push_refused": 0, "pulled": 3, "stored": 1, "pull_refused": 0,
  "complete": true, "pull_cursor": 3, "push_cursor": 2, "caveat": "…" }
```

- `pushed` is what the peer accepted as new; `push_held` what it already had
  (a success — the transport is idempotent); `push_refused` what it refused, per
  op and by its own rules.
- `pulled` counts everything that arrived, including this node's own ops coming
  back; `stored` is how many were new here.
- `complete: false` means the round hit its page bound with work left. Trigger
  another; cursors are durable and it resumes.
- A `502` with an `error` field means the peer was not acceptable — unreachable,
  wrong key, different algebra. Nothing was stored and no cursor moved.

**Nothing polls.** Cackle opens no socket on a schedule; a round happens when
something calls that route. For continuous replication, call it on a timer you
control:

```
*/5 * * * * curl -fsS -X POST -H "Authorization: Bearer $CACKLE_TOKEN" \
  https://venue.example.org/api/sync/peers/$PEER_ID/sync >/dev/null
```

Every five minutes is generous for a ledger that is read after an event. During
one, every round narrows the disagreement window a little.

### 4. Check it

`GET /api/sync/status?org=…` again, on both nodes. `op_log.ops` should agree,
`pending` should be `0` shortly after a round, and each peer carries its
`last_sync_at` and `last_status` — including the failures, which is what you
want to find there.

## Authentication, and what a changed key does

A cloud node is on the open internet, so the peer routes are built for that:

- **Every request is signed** with the caller's node key, over a canonical
  envelope covering the method, path, **query string**, body hash, a timestamp
  and a nonce. The query string is in there because the replication cursor
  lives in it; a signature that skipped it would let anything on the path
  rewrite `after=0` and quietly starve a node of history it thinks it received.
- **Every response is signed too**, bound to the request's nonce. Both
  directions are authenticated per message, and a recorded answer cannot be
  replayed as the answer to a later question.
- **Replay is refused** twice: a timestamp more than ±5 minutes from the
  receiver's clock, and a nonce already seen inside that window.
- **Every op is verified on its own** — signature, structure, namespace, stamp
  agreement, and a bound on claims stamped in the future — before it is stored.
  Arriving over an authenticated connection buys an op nothing.
- **An op's author must also be an enrolled key.** A claim relayed by an
  enrolled peer but signed by a key you never pinned is refused. A mesh of more
  than two nodes therefore converges only when every node has enrolled every
  other node, which is a deliberate human decision per pair rather than trust
  that spreads by itself.

There is no pairing secret, no bootstrap token, no trust-on-first-use, and no
"accept it unsigned if a shared secret is presented" fallback. Enrolment is a
human typing a key, so there is no first-contact problem to solve and no reason
to keep a weaker path open beside the strong one.

**A key that stops matching its pin is a refusal, never a re-pin.** If the
address answers with a different key than you enrolled, the round fails, says
so, names both keys, and changes nothing — not the pin, not the cursors. That is
correct even when the cause is innocent: a node whose key changed is
indistinguishable from an impostor at that address, and only a human can tell
the two apart.

To rotate a peer's key deliberately: **delete the peer, then enrol the new key.**
There is no "edit the key" and there will not be one. Deleting also resets that
peer's cursors, so a fresh enrolment replays its ledger from the beginning —
which is the recovery path if a peer refused ops earlier (a refused op is not
retried automatically; the cursor moves past it so one permanently-unacceptable
op cannot wedge replication for every other event).

## Identity

A node's identity is one row in its own database (`sync_node_identity`), created
lazily. Two things follow, and both matter more than they look:

- **Copying the database clones the identity.** A restored backup brought up
  beside the original is a second node signing as the first. Delete the row on
  the copy; the next enrolment mints a fresh key, and every peer must be told
  the new one.
- **Rotating this node's key is deleting that row.** The node comes back with a
  key no peer has pinned, so every peer refuses it until a human re-enrols it at
  the other end. That refusal is the correct direction.

The private half lives in the database file. Protect that file the way you
already protect the ticket issuer keys in it — see
[SELF-HOSTING.md](SELF-HOSTING.md).

## Bounds

`POST /api/scan/sync` was an unbounded authenticated write until it was rate
limited. These routes are bounded from their first commit:

| bound | value |
| --- | --- |
| request body | 1 MiB, refused (413) before it is read |
| ops per push | 256 |
| ops per pull page | 128 default, 256 maximum |
| pages per round | 8 in each direction |
| rate limit | 5 requests/second per IP, burst 20, shared by both peer routes |
| peer request timeout | 30 seconds |

A peer that follows a redirect is refused too: a redirect moves the request to a
path or host the signature does not cover, and following one to another host
would hand this node's signed envelope to a stranger.

## Running alone

**One node with no peers is the default and is not a degraded mode.** No
identity key is generated, no op is minted, no row is written, no socket is
opened, and nothing in this chapter applies. Everything Cackle does — including
offline gates and the conflict report — works exactly as it did before this
existed. Clustering is something you turn on.

## Under the hood

The merge is the shared DMTAP Sync algebra (KOTVA `substrate/SYNC.md` §4.3
add-only set), the same one the single-node conflict report already used — see
`internal/scan/substrate`. Replication gave that algebra a transport; it did not
write a second one.

An op is a COSE_Sign1 envelope over one admission claim, and the element inside
it carries the scanning device and the scan time, so two gates' claims about the
same ticket stay two distinct set members instead of collapsing into one and
hiding the duplicate. Two nodes that have converged hold the same members, which
is why the same claim minted independently on two nodes is stored once rather
than twice.

None of it touches a gate. `internal/scan` does not import the sync engine and
cannot; a device at a door runs local Ed25519 verification and a local dedupe
claim, with no network, no WebAssembly and no knowledge that any of this exists.

## Verifying it yourself

Two binaries, one host, a non-loopback address. Seed one node, copy its database
to the other, and let them find each other:

```sh
IP=$(ipconfig getifaddr en0)          # Linux: hostname -I | awk '{print $1}'

# Seed node A, then stop it so its database can be copied.
CACKLE_DB=n1/cackle.db ./cackle --demo --addr "$IP:8181" --base-url "http://$IP:8181"
cp n1/cackle.db n2/cackle.db          # no sync_node_identity row exists yet,
                                      # so there is nothing to delete

CACKLE_DB=n1/cackle.db ./cackle --demo --addr "$IP:8181" --base-url "http://$IP:8181" &
CACKLE_DB=n2/cackle.db ./cackle --demo --addr "$IP:8282" --base-url "http://$IP:8282" &
```

`--demo` is used on both because a demo-seeded database's event signing keys are
sealed with the demo key (see [SELF-HOSTING.md](SELF-HOSTING.md)); the seed
itself is idempotent, so the second boot of node A adds nothing. For a real
pair, supply your own `CACKLE_KEY_PASSPHRASE` instead and create the event
yourself.

Then, on each node: log in, read both keys from `GET /api/sync/status?org=…`,
enrol each node on the other, admit the *same* ticket at each node with
`POST /api/scan/sync` under two different `device_id`s, and run
`POST /api/sync/peers/{id}/sync` on the node that has the other's URL. Both
nodes' `GET /api/events/{id}/admission-conflicts` then report the same double
admission, with both gates' own verdicts intact and only one of them holding the
stored `admitted` row.

The same scenario runs in the test suite over this host's real interface
address: `TestTwoNodesConvergeAndRefuseAChangedKey` in `internal/httpapi`, which
also asserts that a peer answering with a changed key is refused and that the
pin survives unchanged.

## Related

- [OFFLINE-GATES.md](OFFLINE-GATES.md) — the gate side, and why cross-gate
  double admission is detected rather than prevented.
- [API.md](API.md) — every other route, including `/api/scan` and
  `/api/scan/sync`. The `/api/sync` routes in this chapter are documented here
  rather than there, because the shapes above and the reasons for them are the
  same text.
- [SELF-HOSTING.md](SELF-HOSTING.md) — running an internet-reachable node.
- [ARCHITECTURE.md](ARCHITECTURE.md) — where `internal/scan/substrate` sits.
