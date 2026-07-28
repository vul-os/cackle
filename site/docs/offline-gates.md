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
  "ticket_index_present": true,
  "allocation": null,
  "issued_at": "2026-07-20T18:00:00Z"
}
```

- **`issuer_keys`** — every current (non-revoked) public key for the event.
  A device pins these; `internal/tickets.Verify` is called with the
  matching `kid` for each presented ticket. This is the only thing that
  makes signature verification possible offline — without it, the device
  has a ticket but nothing to check it against.
- **`ticket_index`** — the set of ticket IDs currently **valid** for this
  event as of `issued_at` below: issued, and neither voided nor refunded. A
  signature only proves a ticket was validly *issued* at some point — it
  says nothing about whether it was later voided/refunded, and an offline
  gate has no other way to find out. `ticket_index` closes that gap: a
  scan whose signature verifies cleanly but whose `tid` is **absent** from
  an authoritative `ticket_index` is rejected (`result=invalid`, reason
  "ticket revoked or not issued for this event") — see
  `internal/scan.DecideWithBundle`.

  Two things worth stating plainly rather than glossing over:
  - **`ticket_index_present` decides whether the index is authoritative,
    and an empty authoritative index means "admit nothing".** The server
    always sets `ticket_index_present: true` — it queried the current valid
    set to build the bundle, even when that set is empty. So an event whose
    every ticket has been refunded (or a cancelled event) ships an empty but
    *present* index, and a gate admits no one, exactly as it should. Only a
    legacy bundle that predates the field carries `ticket_index_present:
    false`, and *only then* does a gate fall back to signature-only
    checking. This distinction matters: inferring "no data" from an empty
    index alone would silently re-admit every physically-held ticket for a
    fully-cancelled event — the one failure this design must not have.
  - **Point-in-time snapshot, not a live feed.** `ticket_index` reflects
    ticket status *at the moment the bundle was generated*. A ticket
    refunded five minutes after a gate downloaded its bundle is still
    admittable at that gate until it re-pulls a fresh one — this is
    inherent to running fully offline, not a bug. The mitigation is
    operational: re-download the bundle periodically (e.g. at shift
    changes, or opportunistically whenever a device briefly has signal),
    the same way you'd periodically re-sync admissions in the other
    direction.
- **`allocation`** — **always `null` today.** The field, the `allocations`
  table and `internal/scan`'s sign/verify/count helpers exist as the seam for
  capacity delegation to sub-issuers (a signed grant letting a disconnected
  device *mint* up to N tickets of a type), but nothing populates it: the
  server sets it to `null` unconditionally, and no gate consults it. It
  never bounded *admissions* and does not gate anything you can run today.
  See [ROADMAP.md](../ROADMAP.md) for where it is headed. Plan your event as
  though this field did not exist, because operationally it doesn't.
- **`issued_at`** — when the bundle was generated, so a device (and a human
  looking at it) can tell how stale its offline copy is.

Fetch this again if you re-open the event, add a new ticket type after
generating the first bundle, or rotate an event key — anything the bundle
doesn't already know about won't be recognised until refreshed.

## Step 2 — scanning, with no network

Every scan against a locally-held bundle does up to three things, all
local (`internal/scan.DecideWithBundle`):

1. **Verify the capability** — `internal/tickets.Verify(token, pinned_pubkey,
   now)`. `now` is the device's own clock; there is no server round-trip to
   get a canonical time, so keep gate devices' clocks correct (NTP-synced
   before doors, same as you'd want for any other reason).
2. **Check `ticket_index`**, if the bundle has one. A ticket whose id isn't
   in it is rejected `result='invalid'` even though its signature is
   perfectly valid — see the `ticket_index` bullet above for exactly what
   this does and does not catch.
3. **Check the local `admissions` table** for this `ticket_id`. First scan
   wins and is recorded `result='admitted'`. Every scan after that is
   recorded as its own row, `result='duplicate'` — **never** overwriting the
   original. A scan that fails verification (step 1) or fails the index
   check (step 2) is recorded `result='invalid'`; a scan for the wrong
   event's ticket is recorded `result='wrong_event'`.

Nothing in this loop touches the network. A gate can run for the length of
an entire festival on a device that's been in airplane mode the whole time,
as long as its battery lasts and its clock is right.

## What offline double-scan protection actually gives you

"Deduped locally" is doing real work in that sentence, but it is not the
same guarantee online scanning gives you, and the difference is worth being
blunt about before you staff a door with it.

**On one device: genuinely prevented.** The second presentation of the same
ticket to the same scanner is refused at the gate, offline, immediately —
`result='duplicate'`, and the operator sees a duplicate, not an admit. The
dedupe is a single atomic claim (`INSERT OR IGNORE` on the Go side,
`SQLiteSeenSet`; an IndexedDB check-then-append on the browser side, with
scans serialised so only one is ever in flight). If the dedupe store itself
errors, the scan **fails closed** — refused, not admitted. Device restarts
don't lose it: the log is on disk, not in memory.

**Across two devices: detected, not prevented.** Two scanners at two
entrances, both offline, keep separate local logs and cannot see each
other's. The same ticket presented at both gets admitted at both — one
person, two entrances, two admits. Nothing catches that until the devices
sync. When they do, the server's `admissions` table has a partial unique
index on `ticket_id WHERE result='admitted'`, so exactly one row wins and
every later claim is downgraded to `duplicate` no matter which device
believed it was first (`POST /api/scan/sync`). You end up with an accurate,
auditable record of what happened — and a person who is already inside.

So: **on-device double-scan is prevented; cross-device double-scan while
offline is detected after the fact, not stopped at the door.** If your threat
model is one attendee lending a ticket to a friend at a second entrance
while the venue is offline, this design does not stop it today.

The fix — a venue mesh sync between scanners, CRDT-merged over local
Wi-Fi/Bluetooth with no server involved — is **not built**; see
[ROADMAP.md](../ROADMAP.md#later--venue-mesh-sync-between-scanners). Until it
is, the mitigations are operational, and they are the same ones paper tickets
always needed:

- Prefer one gate per event where you can. A single device (or several
  devices that stay online and share the server's table) has no gap.
- Sync opportunistically. Every sync narrows the window in which two gates
  can disagree, because a synced ticket comes back as a duplicate everywhere
  after the next bundle refresh.
- Don't run more independent offline entrances than you can staff with tight
  communication.

**Revocation while offline works the same way, for the same reason.** A
gate rejects a refunded or voided ticket only if it was already refunded
when that gate last pulled its bundle — `ticket_index` is a snapshot, not a
feed (see the bullet above). A refund issued mid-event does not reach an
offline gate until it re-pulls. There is no mechanism by which it could:
that is what "no network" means. Re-pull bundles at shift changes and
whenever a device has signal.

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
