# Ticket Format — the capability, and why offline verification works

Read this before touching `internal/tickets`. This is the document that
explains the one design decision that makes Cackle different from every
other ticketing platform: **a gate can prove a ticket is real without ever
talking to a server.**

## The format

A ticket is a single string that fits comfortably in a QR code:

```
cackle.<base64url(payload_json)>.<base64url(ed25519_sig)>
```

Three dot-separated parts:

1. A literal `cackle` prefix — cheap sanity check, lets a scanner reject
   garbage before doing any crypto.
2. A base64url-encoded, compact-JSON payload.
3. A base64url-encoded Ed25519 signature over that payload.

### Payload

```json
{
  "v": 1,
  "tid": "<ulid>",
  "eid": "<ulid>",
  "tt": "<ticket_type_ulid>",
  "kid": "<key_id>",
  "sub": "<holder_user_ulid>",
  "nm": "<holder name>",
  "iat": 1750000000,
  "nbf": 1750000000,
  "exp": 1750099999,
  "seat": "A14"
}
```

Compact keys, no whitespace — every byte here is a byte a QR code has to
encode at a higher error-correction cost. Field meanings:

| Field | Meaning |
|---|---|
| `v` | Payload version. A scanner that doesn't understand a version rejects the ticket rather than guessing. |
| `tid` | Ticket ID (ULID) — what `admissions` dedupes on. |
| `eid` | Event ID — a scanner rejects a ticket presented at the wrong event's gate. |
| `tt` | Ticket type ID — lets a gate show "GA" vs "VIP" without a lookup. |
| `kid` | Key ID — which of the event's `event_keys` rows signed this ticket. Supports key rotation without invalidating already-issued tickets. |
| `sub` | Holder's user ID. |
| `nm` | Holder's display name, embedded so a gate can show it without a lookup. |
| `iat` | Issued-at timestamp. |
| `nbf` | Not-before — a ticket isn't valid before doors open, even if presented. |
| `exp` | Expiry — a ticket stops being admittable after the event's admission window closes. |
| `seat` | Optional seat/section label. |

### Signing

Every event has its own Ed25519 keypair, stored in `event_keys` (public key,
private key, `created_at`, `revoked_at`). **There is no global signing key,
and there never will be** — see the frozen invariant in
[CONTRIBUTING.md](../CONTRIBUTING.md). Per-event authority is the whole
design: a compromised or leaked key compromises exactly one event, and
rotating it (issue a new `event_keys` row, keep the old one valid via `kid`
until its tickets expire, then `revoked_at` it) doesn't touch any other
event on the platform.

## Verification is a pure function

```go
func Verify(token string, pubkey ed25519.PublicKey, now time.Time) (*Payload, error)
```

`Verify` takes the token, the pinned public key for the event, and a clock
value the caller supplies. That's it. **No database handle. No network
client. No implicit `time.Now()`.** This is not a style preference — it is
the mechanism that makes offline scanning possible. If `Verify` ever needs
anything else, offline scanning is broken, because a gate with no network
has nothing else to give it.

The checks `Verify` performs, in order, and the failure each one exists to
catch:

1. **Prefix and shape** — malformed or truncated tokens (a QR scanned
   halfway, a copy-paste error) fail immediately, cheaply.
2. **Base64url decode** of both segments — corrupt encoding fails here.
3. **Signature verification** against the supplied `pubkey` — a tampered
   payload (change the seat, change the name, change the `exp`) fails here,
   because the signature covers the exact payload bytes. A ticket signed
   with the **wrong event's key** also fails here — this is what stops a
   ticket for one event being presented at another event's gate, as long as
   the gate pins the right key for the event it's guarding.
4. **Version check (`v`)** — an unrecognised version is rejected rather than
   partially parsed.
5. **Time window (`nbf` / `exp`)** against the caller-supplied `now` — a
   ticket presented before doors open, or after the admission window closes,
   fails here.

None of this touches storage or the network. `internal/tickets` ships
thorough unit tests for exactly the failure modes above: tamper, wrong key,
expired, not-yet-valid, truncated, wrong version, and replayed (a
structurally valid, unexpired ticket presented a second time — which
`Verify` itself does **not** catch, by design; see below).

## What `Verify` deliberately does not do

`Verify` answers **"is this a real, current ticket for this event?"** It
does not answer **"has this ticket already been used?"** That's a stateful
question — it needs a record of what's already been scanned — and pushing
it into `Verify` would break the purity that makes offline scanning
possible. Instead:

- **Admission dedupe is local**, in the `admissions` table: unique on
  `ticket_id`, first scan wins. Every scan after the first is recorded as
  its own row with `result='duplicate'` — never overwritten, never
  discarded. This is what makes the design auditable: you can always
  reconstruct exactly what every gate did, in order, even offline.
- A gate keeps this table **on the device**, so dedupe works without a
  network the same way `Verify` does.
- See [OFFLINE-GATES.md](OFFLINE-GATES.md) for how a gate acquires the keys
  it needs and reconciles admissions once it's back online.

## Why this beats a server round-trip

The incumbent design — "scan a QR, the app calls the server, the server
says yes or no" — has one failure mode that matters more than any feature
comparison: **when the server or the network is unavailable, nobody gets
in.** For a festival in a field, a remote venue on a bad rural connection,
or a server that simply falls over under load at doors-open, that failure
mode is the whole ballgame. A signed capability with a locally-pinned key
turns "can the server answer right now" into "did I fetch the key once,
earlier" — a question you can answer hours or days in advance, and never
have to ask again during the event.

## Roadmap notes

Everything that would extend this format — signed transfers (a hash chain
of ownership), capacity delegation to sub-issuers, post-quantum signature
agility via the existing `kid` field — is deliberately **not v1** and is
tracked in [ROADMAP.md](../ROADMAP.md), each marked not yet built. The
format above is designed so those extensions are additive, not breaking:
`kid` already gives you multiple valid keys per event, and `v` already
gives you a forward-compatible version gate.
