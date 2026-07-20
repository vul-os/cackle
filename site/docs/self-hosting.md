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
  for, if you're running Cackle as part of a broader Vulos deployment; see
  the [README's "Part of VulOS" section](../README.md#part-of-vulos). It is
  equally fine to sync backups to your own storage of choice — Cackle has
  no opinion here.

Back up **before** any upgrade that touches `internal/store/migrations` —
migrations are forward-only.

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
