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

Every event has its own Ed25519 issuer keypair. The **private half is stored
in the SQLite database as a plaintext BLOB** (`event_keys.private_key`). It is
not encrypted at rest, not wrapped by a KMS or an HSM, and not derived from
anything you supply — there is no passphrase, no envelope key, and no
`CACKLE_*` variable that changes this. Saying it plainly:

> Anyone who obtains the database file can mint valid tickets for every event
> in it, indefinitely, and every offline gate will admit them. The capability
> format's whole security property is that a signature proves issuance by the
> event key; a stolen event key defeats it completely, and no gate can tell
> the difference.

`ticket_index` limits the blast radius only if the forged ticket id is not in
it — a forger who also reads the `tickets` table can sign a real, valid ticket
id and be indistinguishable from the legitimate issuer.

What actually protects them on an exposed host is therefore entirely
operational, and it is worth being deliberate about:

- **File permissions and a dedicated user.** Run as `User=cackle` (the
  systemd unit above does), keep `CACKLE_DB`'s directory `0700` owned by that
  user, and keep it off any path a web server or the media directory can
  reach. Cackle never serves the database, but a misconfigured proxy that
  serves the data directory would hand over every event key at once.
- **Full-disk encryption on the VPS**, so a snapshot, a decommissioned volume
  or a provider-side disk image is not a key disclosure. This is the single
  highest-value mitigation available today.
- **Encrypt the backups.** An unencrypted off-box backup is the same
  disclosure as an unencrypted disk, just somewhere you are watching less.
- **Rotate after any suspected exposure.** `event_keys` supports multiple keys
  per event and `IssuerKey.Revoke` marks one revoked, so a rotated key drops
  out of newly-built key rings — but a gate holding an old bundle still trusts
  the revoked key until it re-pulls, so rotation is only complete once every
  gate has refreshed.
- **Prefer keeping issuance off the exposed host entirely** if your threat
  model warrants it. Nothing in Cackle supports that split today (issuance and
  serving are the same process), so treat it as a reason to keep the public
  node small and well-patched rather than as a configuration you can select.

If that residual risk is unacceptable for your event, the honest answer is to
run Cackle on a host you control on-premises and expose only the reverse proxy
— not to run it on a VPS and hope.

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
