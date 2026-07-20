# internal/tickets

Ed25519-signed ticket capabilities. This is Cackle's core differentiator:
**a gate scanner verifies a ticket with the network unplugged**, against a
per-event public key it pinned ahead of time. The server is not in the
critical path of admission.

## Format

```
cackle.<base64url(payload_json)>.<base64url(ed25519_signature)>
```

Three dot-separated segments:

1. the literal string `cackle`
2. the compact JSON payload, base64url-encoded (no padding, `RawURLEncoding`)
3. the Ed25519 signature over the *raw decoded payload bytes* (not over the
   base64 text), also base64url-encoded

The whole thing is plain ASCII and short enough to fit comfortably in a QR
code.

### Payload shape

```json
{"v":1,"tid":"01J8ZK8T0M8N0P0Q0R0S0T0U0V","eid":"01J8ZK8T0M8N0P0Q0R0S0T0U0E",
 "tt":"01J8ZK8T0M8N0P0Q0R0S0T0U0T","kid":"k_9f3a...","sub":"01J8ZK8T0M8N0P0Q0R0S0T0U0S",
 "nm":"Ada Lovelace","iat":1750000000,"nbf":1750000000,"exp":1750086400,"seat":"A14"}
```

| field | meaning |
|---|---|
| `v` | payload version, must equal `CurrentVersion` (1) |
| `tid` | ticket ULID |
| `eid` | event ULID |
| `tt` | ticket_type ULID |
| `kid` | id of the event issuer key this token is signed with |
| `sub` | holder user ULID |
| `nm` | holder display name |
| `iat` | issued-at, unix seconds |
| `nbf` | not-before, unix seconds (omitted/0 = no lower bound) |
| `exp` | expiry, unix seconds (omitted/0 = no upper bound) |
| `seat` | optional seat/section label |

Every event has its own Ed25519 keypair (`event_keys` in the schema,
represented here as `IssuerKey`). There is **no global signing key** —
per-event authority is the whole design. A key's public half is identified
by a `kid`, a stable id derived from `SHA-256(pubkey)`, and pinned into a
per-event `KeyRing` (`kid -> pubkey`) that scanners fetch once, while
online, and cache to disk.

## Why offline verification is possible

`Verify(token, pub, now)` is **pure**: no database, no network call, and no
internal clock — `now` is always supplied by the caller. That's the whole
trick. A gate device:

1. Downloads a `Bundle` (see `internal/scan`) once, while it still has
   signal: event metadata, the event's `KeyRing`, and an allocation.
2. Goes fully offline for the rest of the event.
3. For every scanned QR code, calls `tickets.VerifyWithRing(token, ring,
   time.Now())` — a pure function of bytes already resident on the device.
   Nothing here can block on a network call, because nothing here makes
   one. If it did, the offline guarantee would be a lie.

Local admission bookkeeping (first-scan-wins dedupe, `admitted` vs
`duplicate`) is handled by `internal/scan`, which layers a `SeenSet` on top
of this package's pure verification. `internal/tickets` itself has no
concept of "already scanned" — that would require state, and state would
require the purity guarantee above to be broken.

## Verification order (why the errors look the way they do)

`Verify` checks things in a specific, deliberate order:

1. Structural sanity — three dot-separated segments, correct `cackle.`
   prefix, valid base64 in both the payload and signature segments, and a
   signature of the exact expected byte length. Any failure here is
   `ErrMalformed`.
2. **The Ed25519 signature, over the raw undecoded-JSON payload bytes** —
   before any JSON parsing happens. This is intentional: a parser is never
   run over bytes that haven't been authenticated yet. A signature mismatch
   (tampered payload *or* wrong key — these are indistinguishable by
   design, to avoid handing an attacker an oracle) is `ErrBadSignature`.
3. Strict JSON decoding of the now-authenticated bytes into `Payload`
   (`DisallowUnknownFields`, no trailing data after the object). Failure is
   `ErrMalformed`.
4. Payload version check against `CurrentVersion`. Failure is
   `ErrUnsupportedVersion`.
5. `nbf`/`exp` window check against the caller-supplied `now`. `now < nbf`
   is `ErrNotYetValid`; `now >= exp` is `ErrExpired` (i.e. `exp` is an
   exclusive upper bound — a ticket expires exactly at its `exp` second,
   not one second after).

`VerifyWithRing` adds one more step *before* all of the above: it peeks the
token's `kid` (without trusting it — this is exactly analogous to reading
an unverified `kid` header in a JWT) and looks it up in the supplied
`KeyRing`. An unrecognised `kid` is `ErrUnknownKID`, returned before any
signature check is even attempted.

## Worked example

```go
issuerKey, _ := tickets.GenerateIssuerKey("event-123")

token, _ := tickets.Issue(tickets.Payload{
    TID:  "01J8ZK8T0M8N0P0Q0R0S0T0U0V",
    EID:  "event-123",
    TT:   "01J8ZK8T0M8N0P0Q0R0S0T0U0T",
    KID:  issuerKey.KID,
    Sub:  "01J8ZK8T0M8N0P0Q0R0S0T0U0S",
    Name: "Ada Lovelace",
    IAT:  1750000000,
    NBF:  1750000000,
    EXP:  1750086400,
    Seat: "A14",
}, issuerKey.PrivateKey)

// token now looks like:
// cackle.eyJ2IjoxLCJ0aWQiOiIwMUo4WksuLi59.MEUCIQD3f9k...

ring := tickets.NewKeyRing("event-123")
ring.AddKey(issuerKey)
_ = ring.SaveToFile("/var/lib/cackle-gate/event-123-keys.json") // once, while online

// ... later, fully offline, at the gate:
scannerRing, _ := tickets.LoadKeyRingFromFile("/var/lib/cackle-gate/event-123-keys.json")
payload, err := tickets.VerifyWithRing(token, scannerRing, time.Now())
if err != nil {
    // ErrMalformed / ErrBadSignature / ErrUnknownKID / ErrNotYetValid / ErrExpired
}
// payload.TID, payload.Sub, payload.Seat, ... now trustworthy — hand off to
// internal/scan for the admit/duplicate decision.
```

## Rejection cases covered by tests

`capability_test.go` table-drives every rejection path: tampered payload
(multiple fields), wrong key, expired (`exp`, including the exact boundary
second), not-yet-valid (`nbf`, including the exact boundary second),
truncated tokens (missing segment, chopped mid-base64), empty string, wrong
prefix, too many segments, bad base64 in either segment, wrong version,
non-object JSON payload, unknown JSON fields, trailing data after the JSON
object, invalid public key size, and unknown `kid` via `VerifyWithRing`.
`FuzzVerify` throws arbitrary bytes at `Verify` and asserts it never panics
and never returns both a payload and an error.

## What this package deliberately does NOT do

- No database, no HTTP, no file I/O in `Verify`/`Issue`/`VerifyWithRing`
  (file I/O only appears in the opt-in `KeyRing.SaveToFile`/
  `LoadKeyRingFromFile` convenience helpers, which callers are free to
  ignore).
- No revocation checking inside `Verify`. `IssuerKey.Revoke` exists so a
  server can stop *including* a revoked key in future `KeyRing`/`Bundle`
  exports, but a scanner that already cached the old ring offline has no
  way to learn of a revocation until it's next online — that's an inherent
  trade-off of the offline model, not a bug.
- No ULID generation. `tid`/`eid`/`tt`/`sub` are opaque strings as far as
  this package is concerned; something else (owned by another package)
  mints them.
