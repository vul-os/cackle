# Ticket Format — the capability, and why offline verification works

> **In plain English, and who needs to read the rest of this page:** a
> Cackle ticket is a short piece of text (it's what's inside the QR code) —
> a bit of information about the ticket plus a digital signature, the way a
> wax seal proves a letter really came from whoever's ring made it. A
> scanner can check that signature itself, with nothing else — no
> database, no internet — which is the entire reason a gate keeps working
> offline. **If you're setting up an event, you don't need to read past
> this box** — see [GETTING-STARTED.md](GETTING-STARTED.md) instead. The
> rest of this page is a byte-exact specification, for someone
> implementing a verifier in another language or auditing the crypto.

Read this before touching `internal/tickets`. This is the document that
explains the one design decision that makes Cackle different from every
other ticketing platform: **a gate can prove a ticket is real without ever
talking to a server.**

It is also a **wire-format specification**. Everything below is stated
precisely enough to write a verifier in another language from this document
alone, and
[`ticket-format-vectors.json`](ticket-format-vectors.json) is the frozen
conformance corpus you check that verifier against. Two implementations
already ship in this repo — `internal/tickets` (Go, server + any Go gate) and
`web/src/lib/capability.js` (JavaScript, the browser scanner that actually
runs at a door) — and both are held to that corpus in CI.

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

### Encoding rules (exact)

| Element | Rule |
|---|---|
| Separator | ASCII `.` (U+002E). Exactly two of them, so exactly three segments. A token with any other count is malformed. |
| Segment 1 | The 6 ASCII bytes `cackle`. Compared byte-for-byte — **case-sensitive**, so `CACKLE` is malformed. |
| Segments 2 and 3 | RFC 4648 §5 base64url — alphabet `A–Z a–z 0–9 - _` — with **padding omitted**. A `=` anywhere, or a `+`/`/` from the standard alphabet, is malformed and must never be re-mapped. (Go: `base64.RawURLEncoding`.) |
| Signature | Exactly 64 bytes after decoding. Any other length is malformed, checked before the signature is verified. |
| Public key | Exactly 32 raw bytes. Checked first, before the token is even parsed. |
| Signed bytes | The **raw decoded bytes of segment 2** — not the base64 text, not a re-serialisation of the parsed payload. |
| Character set | The whole token is ASCII. |

Nothing in the token is encrypted. A capability is a bearer token: anyone
holding the string can present it. Its value is that it cannot be *forged* or
*altered*, not that it is secret.

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
encode at a higher error-correction cost.

| Field | JSON type | Meaning |
|---|---|---|
| `v` | integer | Payload version. A scanner that doesn't understand a version rejects the ticket rather than guessing. |
| `tid` | string | Ticket ID (ULID) — what `admissions` dedupes on. |
| `eid` | string | Event ID — a scanner rejects a ticket presented at the wrong event's gate. |
| `tt` | string | Ticket type ID — lets a gate show "GA" vs "VIP" without a lookup. |
| `kid` | string | Key ID — which of the event's `event_keys` rows signed this ticket. Supports key rotation without invalidating already-issued tickets. |
| `sub` | string | Holder's user ID. |
| `nm` | string | Holder's display name, embedded so a gate can show it without a lookup. |
| `iat` | integer | Issued-at, Unix seconds. |
| `nbf` | integer | Not-before, Unix seconds. Omitted or `0` means no lower bound. |
| `exp` | integer | Expiry, Unix seconds. Omitted or `0` means no upper bound. |
| `seat` | string | Optional seat/section label. |

**Field order is canonical and fixed:** `v, tid, eid, tt, kid, sub, nm, iat,
nbf, exp, seat`. `nbf`, `exp` and `seat` are omitted entirely when
zero/empty — they are never emitted as `0` or `""`.

Order and omission matter **only to an issuer** that wants to produce
byte-identical tokens (and therefore to reproduce the vectors). A *verifier*
never re-serialises: it checks the signature over the bytes exactly as they
arrived, so it is indifferent to how the issuer laid them out. What a
verifier does care about is the strictness below.

### Payload parsing is strict

A verifier must reject, as **malformed**:

- a payload that is not a JSON **object** (an array, a bare number, `null`);
- any **field name not in the table above** — unknown fields are never
  ignored, so nothing can be smuggled through the signature into code that
  might later trust it;
- any field whose **JSON type** differs from the table (`"iat": "1750000000"`
  is malformed; so is a non-integer number);
- any **trailing bytes** after the closing `}`.

`null` for any field is treated as absent — that is what Go's
`encoding/json` does when unmarshalling `null` into a string or integer, and
the JavaScript verifier matches it deliberately.

### Reproducing byte-identical tokens (issuers only)

The reference issuer is Go's `encoding/json` over the `Payload` struct, which
means:

- No whitespace anywhere.
- `&`, `<` and `>` inside string values are escaped as the six-character
  sequences `\u0026`, `\u003c` and `\u003e` (Go escapes
  HTML-significant characters by default).
- All other non-ASCII is emitted as raw UTF-8, not `\u`-escaped.

The third issue vector in
[`ticket-format-vectors.json`](ticket-format-vectors.json) pins exactly this,
with a holder name containing `&`, `<`, `>`, an em dash, a diaeresis and CJK
characters. An issuer that escapes differently still produces *valid* tokens
— the signature covers whatever bytes it emitted — it just won't reproduce
the vectors byte for byte.

### Signing

Every event has its own Ed25519 keypair, stored in `event_keys` (public key,
**encrypted** private key, `created_at`, `revoked_at`). **There is no global
signing key, and there never will be** — see the frozen invariant in
[CONTRIBUTING.md](../CONTRIBUTING.md). Per-event authority is the whole
design: a compromised or leaked key compromises exactly one event, and rotating
it doesn't touch any other event on the platform.

The private half is encrypted at rest under key material the operator supplies
at startup, so an issuing server that has not been given that material cannot
sign anything. This changes nothing for a gate: a gate holds **public keys
only**, and verification is unaffected — see
[SELF-HOSTING.md](SELF-HOSTING.md#the-ticket-keys-are-the-crown-jewels-and-here-is-exactly-how-they-sit).

Signatures are plain Ed25519 (RFC 8032, PureEdDSA over Curve25519) — no
pre-hash, no context string, no domain separator. The message is the payload
bytes and nothing else.

### Key rotation, and what it costs

Multiple valid keys per event are supported, and this is not a plan — it is what
the code does today:

- `event_keys` holds **many rows per event**. Nothing in the schema or the
  queries assumes one.
- A key ring is built from **every non-revoked key** for the event, not just the
  newest one, so a ring routinely carries several public keys.
- Every token names the key that signed it in its `kid`, and ring dispatch looks
  that `kid` up. A ticket does not care which key is "current".
- New tickets are signed with the **newest non-revoked** key.

So rotation splits into two operations with very different consequences, and
conflating them is how a rotation story quietly voids sold tickets:

| Operation | Effect on tickets already in attendees' inboxes |
| --- | --- |
| **Add a key** (new `event_keys` row) | **None.** The old key stays in the ring, old tickets keep verifying, new tickets get the new key. This is the safe half, and it is the whole of an ordinary rotation. |
| **Revoke a key** (`revoked_at`) | **Every ticket signed with that key stops working at the door**, as soon as each gate re-pulls its bundle. The gate answers `ErrUnknownKID`: it no longer holds the key, so it cannot check the signature. |

Revocation is a *ring policy*, not a cryptographic event — the signature on
those tickets remains mathematically valid forever, and a gate that has **not**
re-pulled its bundle still admits them. That is why a revocation is only fully
in effect once every gate has refreshed, and why a gate that stays offline
through the event keeps honouring a key you revoked mid-show.

Practical consequence: rotate by **adding**, and only revoke the old key when
either its tickets are out of circulation, or you have decided that stopping a
forger is worth turning away legitimate holders. Cackle has no re-issue pass
that would re-sign outstanding tickets under a new key, so there is no way to
revoke a key *and* keep its tickets working.

Neither operation is exposed over the HTTP API or a CLI flag today: they are
store-level operations (`CreateEventKey`, `RevokeEventKey`), so rotating means
running code against the database, not clicking something. Rotating the
operator's *passphrase* is separate and much cheaper — it re-wraps one row and
cannot affect a ticket at all (see
[SELF-HOSTING.md](SELF-HOSTING.md#how-it-is-protected)).

### Key IDs and the key ring

A `kid` is derived deterministically from the public key:

```
kid = "k_" + base64url_unpadded( SHA-256(public_key)[0:16] )
```

so two implementations independently given the same key agree on its id.

A gate pins a **key ring**: a map of `kid` to public key, scoped to one
event, delivered inside the scan bundle
(see [OFFLINE-GATES.md](OFFLINE-GATES.md)). Its wire shape is:

```json
{
  "event_id": "<event_ulid>",
  "keys": { "k_If4x36FUomFia_hUBG_SJw": "<base64url(32-byte public key)>" }
}
```

Ring dispatch reads the token's `kid` **without verifying anything** — the
same move as reading an unverified `kid` header on a JWT — purely to choose
which pinned key to check against. It is never trusted for more than that
map lookup; the actual trust decision is still the signature check. A `kid`
that isn't in the ring is rejected as `unknown_kid` *before* any signature
work happens. A `kid` that is in the ring but bound to the wrong key fails on
the signature, as it must.

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

### Check order, and the error each check produces

The order is part of the contract: it decides which error a token that is
wrong in several ways reports, and the vectors pin it.

| # | Check | Error on failure |
|---|---|---|
| 1 | Public key is exactly 32 bytes | `malformed` |
| 2 | Exactly three `.`-separated segments, first is `cackle` | `malformed` |
| 3 | Segments 2 and 3 decode as unpadded base64url | `malformed` |
| 4 | Decoded signature is exactly 64 bytes | `malformed` |
| 5 | **Ed25519 signature over the raw decoded payload bytes** | `bad_signature` |
| 6 | Payload parses as strict JSON per the rules above | `malformed` |
| 7 | `v` equals the version this build supports | `unsupported_version` |
| 8 | `nbf == 0` or `now >= nbf` | `not_yet_valid` |
| 9 | `exp == 0` or `now < exp` | `expired` |

Ring dispatch (`VerifyWithRing`) inserts one step before all of these: peek
`kid`, look it up, `unknown_kid` if absent.

Three things about that ordering that an implementer must not "improve":

- **The signature is checked before the JSON is parsed.** A parser is never
  run over bytes that haven't been authenticated. Moving the parse earlier
  hands an attacker your JSON parser as an attack surface.
- **Tampering and wrong-key are the same error.** `bad_signature` covers
  both, deliberately: distinguishing them would hand an attacker an oracle.
- **`nbf` is inclusive, `exp` is exclusive.** A ticket is valid at exactly
  `nbf` and expired at exactly `exp`. Both boundary seconds are vectors.

### Error codes

The vectors and the JavaScript verifier use these names; Go uses the
matching sentinel, matched with `errors.Is`.

| Code | Go sentinel |
|---|---|
| `malformed` | `ErrMalformed` |
| `unsupported_version` | `ErrUnsupportedVersion` |
| `bad_signature` | `ErrBadSignature` |
| `not_yet_valid` | `ErrNotYetValid` |
| `expired` | `ErrExpired` |
| `unknown_kid` | `ErrUnknownKID` |

On any error a verifier returns **no payload at all** — never a partially
populated one. Callers must not be able to accidentally read fields off a
token that failed.

## Conformance vectors

[`ticket-format-vectors.json`](ticket-format-vectors.json) is the executable
half of this spec: fixed keys, tokens frozen as bytes, and the exact outcome
each one must produce.

```
keys.issuer          seed (hex), public key (base64url), derived kid
keys.other           a second key, for wrong-key cases
keys.truncated       a deliberately 16-byte "key"
issue[]              payload -> exact payload_json -> exact token
verify[]             token + key + now_unix -> "ok" (with tid) or "error" (with code)
verify_with_ring[]   token + ring + now_unix -> "ok" or "error" (with code)
```

The keys are published, fixed and non-secret; they exist only to make the
corpus reproducible and sign nothing real.

To validate a new implementation: reconstruct `keys.issuer` from its seed,
confirm it derives the published public key and `kid`, then run every
`issue` vector (byte-identical token required) and every `verify` /
`verify_with_ring` vector (exact error code required).

In this repo the corpus is run by:

- `internal/tickets/conformance_test.go` — the Go verifier, under `go test`.
- `web/src/lib/capability.conformance.test.js` — the browser verifier, under
  `npm test` in `web/` (Node's built-in `node:test`, no extra dependency).

Both assert minimum vector counts and that every documented error code is
exercised by at least one vector, so a truncated or half-parsed corpus fails
loudly rather than passing by running nothing.

**These vectors are frozen.** There is no `-update` flag, on purpose. If a
change to the format is genuinely wanted, that is a `v` bump plus a
deliberate regeneration plus a CHANGELOG entry — not something a test run
gets to rubber-stamp.

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
  network the same way `Verify` does. That dedupe is per device: what it
  does and does not stop is spelled out in
  [OFFLINE-GATES.md](OFFLINE-GATES.md#what-offline-double-scan-protection-actually-gives-you).
- Nor does it answer **"was this ticket refunded after it was issued?"** — a
  signature can't, since it was made before the refund existed. The scan
  bundle's `ticket_index` is what closes that gap, again in
  [OFFLINE-GATES.md](OFFLINE-GATES.md).

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
