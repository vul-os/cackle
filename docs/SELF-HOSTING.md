# Self-Hosting

Running Cackle for a real event, not just kicking the tyres with `--demo`.

## Docker

```bash
docker build -t vulos/cackle .
docker run -d \
  --name cackle \
  -p 8080:8080 \
  -e CACKLE_BASE_URL=https://tickets.example.com \
  -v cackle-data:/srv/data \
  --restart unless-stopped \
  vulos/cackle
```

This runs with the default `manual` provider only (no payment secrets
needed). To take payments through a real processor as well, add
`CACKLE_PAYMENT_PROVIDERS` and that provider's own secrets — e.g.
`-e CACKLE_PAYMENT_PROVIDERS=manual,stripe -e CACKLE_STRIPE_SECRET_KEY=sk_live_xxxxx
-e CACKLE_STRIPE_WEBHOOK_SECRET=whsec_xxxxx`. See
[PAYMENTS.md](PAYMENTS.md) for every built-in adapter and its required
env vars, and the warning at the top of that document about sandbox-testing
one before it ever touches real money.

The image already sets `CACKLE_ADDR=:8080` and `CACKLE_DB=/srv/data/cackle.db`
(its `WORKDIR` and declared `VOLUME` are both `/srv/data`) — you only need to
override `CACKLE_DB` if you want the database somewhere else inside the
container. The volume is not optional either way: `CACKLE_DB` is the entire
state of the instance — every org, event, ticket, event key, and order.
Without a mounted volume at that path, `docker rm` deletes your event.

## Bare binary

Build the real single binary with the frontend embedded (`make build` — see
[GETTING-STARTED.md](GETTING-STARTED.md) for what that does under the hood),
then run it directly:

```bash
make build
CACKLE_ADDR=:8080 \
CACKLE_DB=/var/lib/cackle/cackle.db \
CACKLE_BASE_URL=https://tickets.example.com \
./cackle
```

Run it under a process supervisor (systemd, supervisord) so it restarts on
crash and starts on boot. A minimal systemd unit:

```ini
[Unit]
Description=Cackle
After=network.target

[Service]
ExecStart=/usr/local/bin/cackle
Environment=CACKLE_ADDR=:8080
Environment=CACKLE_DB=/var/lib/cackle/cackle.db
Environment=CACKLE_BASE_URL=https://tickets.example.com
EnvironmentFile=/etc/cackle/payments.env
Restart=on-failure
User=cackle

[Install]
WantedBy=multi-user.target
```

If you're using `manual` only, `payments.env` can be empty or omitted
entirely — there's nothing to configure. If you've enabled a real
provider, keep `CACKLE_PAYMENT_PROVIDERS` and that provider's
`CACKLE_<PROVIDER>_*` secrets in the `EnvironmentFile`, not the unit file
itself, and make sure that file isn't world-readable — it's a production
secret.

## TLS / reverse proxy

Cackle speaks plain HTTP; put a reverse proxy in front for TLS. Any of
Caddy, nginx, or Traefik work — Caddy is the least config for a single
domain:

```
tickets.example.com {
    reverse_proxy localhost:8080
}
```

Whatever you use, terminate TLS at the proxy and set `CACKLE_BASE_URL` to
the public HTTPS URL — payment provider callbacks and every link Cackle
generates are built from it.

## Backups

`CACKLE_DB` is a single SQLite file — back it up like any other database
you'd be upset to lose:

- **Simplest:** stop the container/process, copy the file, restart. Fine
  for low-traffic instances and scheduled maintenance windows.
- **Live backup:** SQLite supports the online backup API and the
  `.backup` CLI command against a running database without an exclusive
  lock; wrap either in a cron job.
- **Off-box:** copy the backup somewhere that isn't the same disk — this is
  exactly the kind of thing Vulos's backup-storage service (buckets) is
  for, if you're running Cackle as part of a broader Vulos deployment (see
  [vulos.org](https://vulos.org)). It is
  equally fine to sync backups to your own storage of choice — Cackle has
  no opinion here.

Back up **before** any upgrade that touches `internal/store/migrations` —
migrations are forward-only.

**The database is no longer the whole story.** Event signing keys are encrypted
at rest, so a restore needs the database *and* the key material that unlocks it
— and that material must be backed up **somewhere else**, or the backup archive
becomes a single object that contains both the lock and the key. Losing the
material is unrecoverable: already-issued tickets keep verifying and gates keep
working, but that database can never issue another ticket. See
[the key material section](#configuring-the-key-material).

## Running it as a node venue gates sync to

Cackle is a single static binary with a Docker image, so putting it on a VPS
and having venue gates sync to it over the internet works today. What follows
is that path walked end to end, including the parts that are **not** handled
for you. Read it before you point a real event at a public host.

**Bind address.** `CACKLE_ADDR` defaults to `:8080`, which binds **every**
interface. On a VPS that means the API is reachable from the internet the
moment the process starts, before you have configured a proxy or a firewall.
Either bind loopback and let the proxy reach it —
`CACKLE_ADDR=127.0.0.1:8080` — or firewall 8080 and expose only 443. Binding
loopback is the better default of the two: a misconfigured firewall then fails
closed instead of open.

**TLS.** There is none in the binary. Cackle speaks plain HTTP by design and
expects a terminating proxy (see the section above). Over the public internet
this is not optional: gate sync carries session cookies and CSRF tokens, and a
scanner session is enough to write admissions. Set `CACKLE_BASE_URL` to the
`https://` URL — secure-cookie and callback behaviour is derived from it.

**Auth.** Gates authenticate as ordinary users with the `scanner` role on the
event's org; there is no device-enrolment flow, no per-gate credential, and no
mutual TLS. Consequences to plan around:

- A scanner session is a **bearer** credential. Anyone holding it can sync
  admissions for that event from anywhere. Use one account per event or per
  crew, not one shared account across events, so revocation is scoped.
- `device_id` is a UUID the browser generates and stores in `localStorage`. It
  is **reported data, not an authenticated identity** — a caller with a valid
  scanner session can submit claims under any `device_id` it likes. Everything
  attributing admissions to a physical scanner (including
  `admission-conflicts`) rests on that RBAC check and on operational control
  of the devices, not on cryptography.
- `/api/auth/*`, `/api/scan`, `/api/scan/sync` and
  `/api/events/{id}/admission-conflicts` are rate-limited per IP. Nothing else
  is. Every venue gate behind one NAT shares one bucket; the scan bucket
  (10/sec sustained, burst 30) has ample headroom for batch sync, but size
  your proxy limits with that sharing in mind.

**Durability.** One SQLite file, WAL journaling, `busy_timeout` 5s, a pool of
8 connections. That is genuinely enough for the sales traffic and gate sync a
single-venue operator generates, and it is also the whole story: there is no
replication, no failover, and **no server-to-server sync**. Two Cackle
instances do not exchange anything. If you want a venue-local server *and* a
cloud node, today they are two independent deployments with two databases, and
nothing merges them — `internal/scan/substrate` defines the algebra such a
mesh would merge under and the report path exercises it, but there is no
transport, no peer handshake and no node identity, and none of that should be
read as "nearly there".

**Backups.** As the section above, with one addition that matters more on an
exposed host: back up **off-box**, because on a single VPS your database and
your only copy of it share a fate. Back up before every upgrade; migrations
are forward-only.

**Cache directory.** A `wasm-cache/` directory appears beside `CACKLE_DB` the
first time a reconciliation report is read. It holds only wazero's compiled
machine code for an engine embedded in the binary — nothing secret, nothing
durable. Deleting it costs a few hundred milliseconds on the next report.
Do not back it up and do not serve it.

### The ticket keys are the crown jewels, and here is exactly how they sit

Every event has its own Ed25519 issuer keypair. The private half is **encrypted
at rest** and cannot be read — by Cackle or by anyone else — without key
material you supply at startup. There is no plaintext mode, no auto-generated
fallback key, and no flag that turns this off.

This was not always true. Before this release `event_keys.private_key` was a
plaintext BLOB in the SQLite file, which meant anyone who obtained a copy of
that file could mint valid tickets for every event in it, forever, and no gate
could tell — the forgery carries a real signature from the real key. If you are
upgrading, read [Upgrading an existing database](#upgrading-an-existing-database)
below, and treat every backup you took before the upgrade as key material.

#### Configuring the key material

Set exactly **one** of these. Cackle refuses to start without one (except
`--demo`, see below), and refuses to start if more than one is set — resolving
that ambiguity by precedence would mean your database is unlocked by material
you think is unused.

| Variable | What it is |
| --- | --- |
| `CACKLE_KEY_PASSPHRASE` | An operator passphrase, minimum 12 characters. Stretched with Argon2id (64 MiB, t=3, 4 lanes) using a per-database salt. |
| `CACKLE_KEY_PASSPHRASE_FILE` | Path to a file containing that passphrase. Preferred on Docker/systemd, where an env var is visible to anything that can run `docker inspect` or read `/proc/<pid>/environ`. A single trailing newline is ignored. |
| `CACKLE_KEY_FILE` | Path to a file of at least 32 random bytes: `head -c 32 /dev/urandom > /etc/cackle/keyfile && chmod 600 /etc/cackle/keyfile`. Derived with HKDF-SHA256 — no stretching, because there is nothing to stretch: it is already full-entropy key material. |

An empty value (`CACKLE_KEY_PASSPHRASE=`) is an **error**, not a synonym for
unset. An operator who wrote that believes they configured a passphrase, and
quietly reading it as "no encryption" is how the plaintext path survived as long
as it did.

**Back the material up separately from the database, and keep it out of the
database's backup.** This is the entire point. See
[what you actually gain](#what-an-attacker-with-your-database-file-gets), below.

#### How it is protected

Envelope encryption, two layers, mirroring the credential vault in the sibling
SlipScan project:

```
you        ──supply──  passphrase or keyfile      (never stored in the database)
                        │ Argon2id / HKDF-SHA256
                       KEK                        (derived at boot, never written down)
                        │ wraps (XChaCha20-Poly1305)
key_vault  ──holds──   DEK ciphertext             (one row)
                        │ seals (XChaCha20-Poly1305, per-key nonce + AAD)
event_keys ──holds──   sealed_private_key         (one per event key)
```

Consequences worth knowing:

- **Rotating your passphrase does not re-encrypt anything.** It re-wraps one
  row — the DEK — so it cannot disturb a single issued ticket, and it never
  decrypts an event key at all.
- **A sealed key is bound to its row.** The event id and key id are
  authenticated as associated data, so a ciphertext lifted from one event's row
  and pasted into another's fails to decrypt rather than silently signing as the
  wrong event.
- **Nothing can display a private key.** There is no API, no CLI flag and no log
  line that emits one. Keys can be created, migrated, rotated, revoked and
  *used* — the only way private material leaves the store is as a signature over
  a ticket. `KeyVaultStatus` (what the boot log prints) carries metadata only.
- **Wrong material fails closed.** A passphrase that does not match the database
  is a startup failure, not a silent downgrade.

#### What an attacker with your database file gets

Be precise about this, because the honest answer depends on whether they also
got the key material.

| They have | Before this release | Now |
| --- | --- | --- |
| The database file | **Every event signing key.** Mint valid tickets for any event, forever; every gate admits them, online or offline. Undetectable. | Public keys, plus all the business data (events, orders, buyer emails and names, ticket ids, admissions). **No signing key**, so no forgery. Sealed keys are XChaCha20-Poly1305 ciphertext; recovering one means guessing your passphrase through Argon2id, or your 32-byte keyfile. |
| The file **and** the key material | Same as above. | Same as before this release: every signing key. |
| Root on the running host | Everything, including live process memory. | Everything, including live process memory — the DEK is unwrapped and resident while Cackle runs. |

Read the middle row twice. **If your passphrase sits in the same directory, the
same compose file, or the same backup archive as `CACKLE_DB`, you have gained
nothing.** Encryption at rest buys exactly one thing: it decouples a copy of
the *file* from a compromise of the *keys*. That covers the realistic incidents
— an over-broad backup, a snapshot of a decommissioned volume, a rsync to the
wrong bucket, a stolen laptop with a copy of prod, a misconfigured static-file
route — and it covers nothing else.

It does **not** protect against a live-host compromise. A running Cackle holds
the DEK in memory by necessity; anything that can read that process can read
the keys. Go also offers no way to guarantee a secret is scrubbed from memory
(the runtime may have copied it), so the wiping this code does after use
narrows the window rather than closing it.

#### What is still entirely on you

- **Back up the key material, separately.** The database alone is no longer
  sufficient to restore a working instance. If you lose the material there is
  **no recovery path in the binary**: the sealed keys stay sealed, and while
  every already-issued ticket keeps verifying and every gate keeps working
  (both need public keys only), that database can never issue another ticket
  for any event again. Store the material the way you would store the only copy
  of a signing key, because that is what it is.
- **Full-disk encryption on the VPS.** Still worth it. It covers the same
  snapshot and decommissioned-volume cases at a different layer, and it also
  covers the passphrase file if you keep one on that host.
- **Encrypt the backups anyway.** They contain buyer names and email addresses,
  which are worth protecting whether or not a signing key is in there.
- **File permissions and a dedicated user.** Run as `User=cackle` (the systemd
  unit above does), keep `CACKLE_DB`'s directory `0700` owned by that user, and
  keep it off any path a web server or the media directory can reach. Cackle
  never serves the database, but a misconfigured proxy that serves the data
  directory would hand over every buyer's details at once.
- **Rotate after any suspected exposure**, and read
  [TICKET-FORMAT.md](TICKET-FORMAT.md#key-rotation-and-what-it-costs) first:
  adding a key is free and voids nothing, but *revoking* the old key is what
  makes tickets signed with it stop working at the door. The two are separate
  decisions.
- **Prefer keeping issuance off the exposed host entirely** if your threat model
  warrants it. Nothing in Cackle supports that split today (issuance and serving
  are the same process), so treat it as a reason to keep the public node small
  and well-patched rather than as a configuration you can select.

If that residual risk is unacceptable for your event, the honest answer is to
run Cackle on a host you control on-premises and expose only the reverse proxy
— not to run it on a VPS and hope.

#### `--demo` is sealed with a public key, on purpose

`cackle --demo` needs no key material: it seals its throwaway event keys with a
constant that is published in the Cackle source, and logs a warning saying so.
That protects nothing, which is fine — demo mode already prints its own login
password to your terminal.

The safety property is that a demo database **announces itself**. Its vault row
records that it was sealed with the demo key, so a non-demo boot against it
refuses to start rather than quietly serving a real event from keys anyone can
unseal. The reverse is refused too: `--demo` cannot open a database sealed with
a real passphrase. If key material *is* configured, it always wins, even under
`--demo`.

#### Upgrading an existing database

The upgrade is a normal migration plus a data pass, and it is not optional.

1. **Back up `CACKLE_DB`.** That backup contains plaintext signing keys.
2. Configure key material (above).
3. Start the new binary. On boot it applies the schema migration, then encrypts
   every existing key and clears the plaintext in one transaction, then rebuilds
   the database file so the freed pages holding plaintext are gone. It logs how
   many keys it sealed.
4. **Destroy — or treat as key material — every backup and snapshot taken
   before step 3.** The migration rewrites your database. It cannot reach copies
   that already left the machine, and every one of them still contains keys that
   can mint tickets for your events. Nothing about this upgrade makes an old
   backup safe. If any of them may already have leaked, rotate.

If you start the new binary **without** key material against a database that
still holds plaintext keys, it refuses to start and tells you what to set. It
does not half-migrate: either every key is sealed or none is. The same applies
mid-migration — a crash leaves the whole batch sealed or the whole batch as it
was, never a mixture.

An already-migrated database is unaffected by later restarts: the data pass only
looks at rows still holding plaintext, so it is a no-op from the second boot on.


## Scaling the gate, not the server

The whole point of the offline design ([OFFLINE-GATES.md](OFFLINE-GATES.md))
is that gate throughput does not depend on server capacity. If you're
running a large event:

- Size the server for **sales traffic** (checkout, order creation, payment
  webhooks) — that's the load that's actually proportional to concurrent
  online users.
- Size gate device count for **queue throughput** at the door — that's a
  function of scan time per attendee and how many entrances you're staffing,
  entirely decoupled from server load, because each device fetched its scan
  bundle once and doesn't call the server again until it's syncing.
- If you expect a spike at doors-open (everyone arrives in the first 20
  minutes), that spike hits your gate devices, not your server — which is
  the property that makes this design worth the extra setup step.

## Upgrading

Migrations in `internal/store/migrations/*.sql` are numbered and forward
only — Cackle applies any migration newer than the database's current
version on boot. Back up `CACKLE_DB` first, deploy the new binary, and let
it migrate on startup. There is no supported downgrade path; restore from
backup if you need to go back.
