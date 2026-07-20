# Running a Gate With No Network

This is the operational guide for the thing Cackle is actually for: staffing
a door with a scanner that keeps working after the venue's Wi-Fi, the
server, or both, stop responding. The design is explained in
[TICKET-FORMAT.md](TICKET-FORMAT.md); this document is about running it.

## The shape of it

1. **Before doors, while you still have a connection:** each scanning
   device fetches the event's scan bundle once.
2. **At the gate, for the whole event:** the device scans QR codes and
   admits people using only what it downloaded in step 1. No network calls.
3. **After the event, or whenever connectivity comes back:** each device
   uploads its admission log; the server merges it, idempotently, with
   every other gate's log.

If step 3 never happens — the device's battery dies, someone drops it in a
puddle — you lose that device's admission log, not the ability to run the
gate. That's the trade the whole design makes on purpose.

## Step 1 — the scan bundle

```
GET /api/events/{id}/scan-bundle
```

Requires scanner auth (a session with at least the `scanner` role on the
event's org). Returns everything a device needs to run the entire event
unplugged:

```json
{
  "event": { "...": "event metadata" },
  "issuer_keys": [{ "kid": "...", "public_key": "..." }],
  "ticket_index": ["<ticket_id>", "..."],
  "allocation": { "...": "see below" },
  "issued_at": "2026-07-20T18:00:00Z"
}
```

- **`issuer_keys`** — every current (non-revoked) public key for the event.
  A device pins these; `internal/tickets.Verify` is called with the
  matching `kid` for each presented ticket. This is the only thing that
  makes signature verification possible offline — without it, the device
  has a ticket but nothing to check it against.
- **`ticket_index`** — the set of ticket IDs that exist for this event.
  Combined with the signature check, this lets a gate flag a
  structurally-valid-looking ticket that was never actually issued (a
  forged `tid` with an otherwise-plausible payload still fails the
  signature check on its own, but the index gives a second, independent
  signal a UI can surface).
- **`allocation`** — a signed claim, scoped to this device, bounding how
  many admissions of a given ticket type it may grant before it's expected
  to reconcile. See [ROADMAP.md](../ROADMAP.md) for where this is headed
  (capacity delegation to sub-issuers); today it exists as the seam for that
  work, in the `allocations` table.
- **`issued_at`** — when the bundle was generated, so a device (and a human
  looking at it) can tell how stale its offline copy is.

Fetch this again if you re-open the event, add a new ticket type after
generating the first bundle, or rotate an event key — anything the bundle
doesn't already know about won't be recognised until refreshed.

## Step 2 — scanning, with no network

Every scan against a locally-held bundle does two things, both local:

1. **Verify the capability** — `internal/tickets.Verify(token, pinned_pubkey,
   now)`. `now` is the device's own clock; there is no server round-trip to
   get a canonical time, so keep gate devices' clocks correct (NTP-synced
   before doors, same as you'd want for any other reason).
2. **Check the local `admissions` table** for this `ticket_id`. First scan
   wins and is recorded `result='admitted'`. Every scan after that is
   recorded as its own row, `result='duplicate'` — **never** overwriting the
   original. A scan that fails verification is recorded `result='invalid'`;
   a scan for the wrong event's ticket is recorded `result='wrong_event'`.

Nothing in this loop touches the network. A gate can run for the length of
an entire festival on a device that's been in airplane mode the whole time,
as long as its battery lasts and its clock is right.

> **Known limitation, tracked on the roadmap:** two scanners at two entrances
> of the same venue, both offline, dedupe **independently** — each has its
> own local `admissions` table, so the same ticket could in principle be
> admitted once at each entrance before the devices ever compare notes. A
> venue mesh sync between scanners (CRDT-merged over local Wi-Fi/Bluetooth,
> no server required) is the fix, and is **not yet built** — see
> [ROADMAP.md](../ROADMAP.md#later--venue-mesh-sync-between-scanners). Until
> then, the practical mitigation is the same one paper tickets always
> needed: don't run more independent entrances than you can staff with tight
> communication.

## Step 3 — syncing back

```
POST /api/scan/sync
{ "admissions": [ { "ticket_id": "...", "device_id": "...", "gate_id": "...",
                     "scanned_at": "...", "result": "admitted" }, ... ] }
```

Idempotent on `(ticket_id, device_id, scanned_at)` — syncing the same batch
twice (a flaky connection retries the upload) does not create duplicate
admission rows on the server side. Sync as often as connectivity allows;
there's no requirement to wait until the event ends. Running multiple
devices at one gate, or one device per entrance, all works the same way:
each device's log merges independently, and the server's own dedupe applies
across all of them once merged, so you get an accurate combined admission
count as soon as every device has synced at least once.

## Practical setup notes

- **Fetch the bundle with time to spare.** Doors-open is the worst possible
  moment to discover a device's connection is too slow to pull the ticket
  index for a large event. Fetch it the night before, or at latest, well
  before the queue forms.
- **Keep device clocks correct.** `nbf`/`exp` checks are only as good as the
  clock `Verify` is handed.
- **One `device_id` per physical scanner.** It's what admission logs and
  sync idempotency are keyed on — reusing an ID across devices defeats the
  dedupe; reissuing a fresh one for a replacement device is fine and
  expected.
- **Sync opportunistically.** Any moment of connectivity — a phone briefly
  finding a signal, a laptop plugged in at the merch tent with a hotspot —
  is worth a sync call. It's cheap and idempotent.
- **A dead device is a lost log, not a lost gate.** Losing one scanner's
  admission history is an operational annoyance (you'll reconcile it
  manually or accept the gap in the stats); it is never a reason a gate
  stops admitting people.

## Related

- [TICKET-FORMAT.md](TICKET-FORMAT.md) — the capability format and the
  verification contract this whole guide depends on.
- [API.md](API.md) — full request/response shapes for `scan-bundle`,
  `/api/scan`, and `/api/scan/sync`.
- [ARCHITECTURE.md](ARCHITECTURE.md) — where `internal/scan` sits in the
  wider system.
