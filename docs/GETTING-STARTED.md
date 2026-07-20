# Getting Started

Clone to first ticket scanned.

## Fastest path: Docker

```bash
git clone https://github.com/vul-os/cackle.git
cd cackle
docker build -t vulos/cackle .
docker run -d --name cackle -p 8080:8080 -v cackle-data:/srv/data vulos/cackle
```

Open `http://localhost:8080`. The image's multi-stage build compiles the
frontend and embeds it into the Go binary, so this gets you the real UI, not
a placeholder — but with no demo data and no payment provider configured, so
there isn't much to click on yet. Jump to
[Running for real](#running-for-real) below, or add `-e CACKLE_DEMO=true` to
the `docker run` line to boot it seeded instead.

## Fastest path to see it working: `--demo`

```bash
git clone https://github.com/vul-os/cackle.git
cd cackle
make build
./cackle --demo
```

Open `http://localhost:8080`. `make build` builds the frontend (`web/`),
embeds it into the Go binary (`go build -tags embed_frontend`), and leaves
you a single `./cackle` file at the repo root — see the "EMBED CONTRACT"
comment at the top of the [`Dockerfile`](../Dockerfile) for exactly what
that tag does and why `//go:embed` needs it.

`--demo` seeds an organisation, a published event with a few ticket types,
and wires up the `stub` payment provider so "buying" a ticket auto-settles
with no real payment processor involved. Log in, browse the seeded event,
complete a checkout, and look at the ticket you get back — that's the whole
loop.

To scan it: sign in with a `scanner` role on the demo org (the seed data
includes one), open the scanner view, and present the ticket's QR. It works
exactly the same with your Wi-Fi off, once the scanner view has loaded —
that's the point.

### Faster backend-only iteration

```bash
go build -o cackle ./cmd/cackle
./cackle --demo
```

This compiles fine and boots the same backend, but **without** the
`embed_frontend` build tag it serves a bare dev fallback instead of the
built React app — useful when you're only touching Go and don't want to
wait on a frontend build, not useful for looking at the UI. `make
build-backend` is the named target for the same thing.

## Building the frontend from source

If you're changing `web/` and want to see it embedded in a real binary,
build the frontend first so the embedded copy is current, then run the full
build — this is exactly what `make build` automates:

```bash
cd web && npm install && npm run build && cd ..
rm -rf cmd/cackle/dist
cp -r web/dist cmd/cackle/dist
CGO_ENABLED=0 go build -tags embed_frontend -o cackle ./cmd/cackle
rm -rf cmd/cackle/dist
```

Prefer `make build` — it's the same steps, already scripted.

For frontend development with hot reload, run the Vite dev server against a
running Go backend instead of rebuilding on every change:

```bash
# terminal 1
go run ./cmd/cackle --demo

# terminal 2
cd web && npm install && npm run dev
```

The Vite dev server proxies API requests to the Go backend.

## Running for real

Zero-setup demo mode is for evaluation only. To run a real event:

1. Set `CACKLE_BASE_URL` to your real, publicly reachable domain — payment
   provider callbacks need it. See [CONFIGURATION.md](CONFIGURATION.md).
2. Decide how you'll take payment. The default, `manual`, needs no setup at
   all — the organiser records that money arrived (bank transfer, cash at
   the door, an invoice) and marks the order paid. To take payments through
   a real processor instead (or alongside it), set
   `CACKLE_PAYMENT_PROVIDERS` and that provider's `CACKLE_<PROVIDER>_*`
   secrets — see [PAYMENTS.md](PAYMENTS.md) for the full list of built-in
   adapters and each one's verification status, or write your own
   `payments.Provider`.
3. Point `CACKLE_DB` at a path on a volume you actually back up (the Docker
   image already defaults this to `/srv/data/cackle.db` under the `/srv/data`
   volume — mount it).
4. Create your organisation and event through the UI (or the API — see
   [API.md](API.md)) — real data, not `--demo`.
5. Before doors, have every gate device fetch the event's scan bundle while
   it still has a connection. See [OFFLINE-GATES.md](OFFLINE-GATES.md) —
   this is the step that makes the rest of the event resilient to bad
   network at the venue.

See [SELF-HOSTING.md](SELF-HOSTING.md) for backups, TLS, and running behind
a reverse proxy.

## Next

- [ARCHITECTURE.md](ARCHITECTURE.md) — how the pieces fit together
- [TICKET-FORMAT.md](TICKET-FORMAT.md) — the ticket capability format, and
  why offline verification works (read this before touching `internal/tickets`)
- [OFFLINE-GATES.md](OFFLINE-GATES.md) — running a gate with no network
- [CONFIGURATION.md](CONFIGURATION.md) — every env var and flag
