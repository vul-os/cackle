# Configuration

> **In plain English:** everything you can configure is a single named
> setting (an "environment variable"), not a config file to write by hand.
> If you're just trying Cackle out, you don't need any of this —
> [GETTING-STARTED.md](GETTING-STARTED.md) covers the two settings that
> matter for a real event (`CACKLE_BASE_URL` and `CACKLE_KEY_PASSPHRASE`).
> Come back here when you need the full list, or when someone technical is
> setting this up for you and wants the reference table.

Cackle is configured env-first. Every key is prefixed `CACKLE_`. Flags
mirror the env vars for the same setting. No config file is required or
supported — this is intentional, to keep the single-binary story simple.

## Reference

| Variable | Flag | Default | Description |
|---|---|---|---|
| `CACKLE_ADDR` | `--addr` | `:8080` | HTTP listen address. |
| `CACKLE_DB` | `--db` | `./cackle.db` | Path to the SQLite database file. Created (and migrated) on first boot if it doesn't exist. |
| `CACKLE_MEDIA_DIR` | `--media-dir` | `<db dir>/media` | Directory uploaded event images are stored under. Defaults to a `media` folder beside the database file. Must be writable and should be included in backups alongside the database. |
| `CACKLE_BASE_URL` | `--base-url` | derived from `--addr` (e.g. `http://localhost:8080`) | The public URL Cackle is reachable at. Used for payment provider callback URLs and for the reset link `cackle reset-password` prints (see [Accounts without email](#accounts-without-email) below). Never used for email — Cackle sends none. Defaults to a localhost URL derived from the listen address, which is fine for local development — but set this to your real domain before taking payments, or the payment callback round-trip points at localhost and breaks. |
| `CACKLE_SESSION_SECRET` | — | auto-generated, persisted | Secret used to sign session tokens. If unset, Cackle generates one on first boot and persists it to a `.cackle_session_secret` file (mode `0600`) beside the database file, so subsequent restarts don't invalidate every session. Set it explicitly in any multi-instance deployment so all instances share one secret. |
| `CACKLE_KEY_PASSPHRASE` | — | **none — required** | Operator passphrase (minimum 12 characters) that unlocks the event signing keys, which are encrypted at rest. Cackle **refuses to start** without key material: there is no plaintext mode and no generated fallback. Set exactly one of this, `CACKLE_KEY_PASSPHRASE_FILE` or `CACKLE_KEY_FILE` — setting two is an error, and setting one to an empty value is an error rather than "unset". Back it up separately from `CACKLE_DB`; losing it means that database can never issue another ticket (already-issued tickets and every gate keep working). See [SELF-HOSTING.md](SELF-HOSTING.md#configuring-the-key-material). |
| `CACKLE_KEY_PASSPHRASE_FILE` | — | — | Path to a file holding the passphrase instead of putting it in the environment, where `docker inspect` and `/proc/<pid>/environ` can read it. A single trailing newline is ignored. |
| `CACKLE_KEY_FILE` | — | — | Path to a file of 32+ random bytes used as key material instead of a passphrase: `head -c 32 /dev/urandom > /etc/cackle/keyfile && chmod 600 /etc/cackle/keyfile`. |
| `CACKLE_PAYMENT_PROVIDERS` | — | unset (every registered provider enabled) | Comma-separated allowlist of optional payment providers for this deployment, e.g. `manual,stripe,paystack`. `manual` cannot be disabled and is always enabled regardless of this variable. See [PAYMENTS.md](PAYMENTS.md). |
| `CACKLE_HOST_SCOPE` | — | `own` | Whose events your front page shows: `own`, `single` or `peers`. See [What your front page shows](#what-your-front-page-shows) below. |
| `CACKLE_HOST_ORG` | — | — | The URL name (or id) of the one organisation this Cackle presents as. **Required** when `CACKLE_HOST_SCOPE=single`, and refused with any other scope — a setting that names a boundary must not be quietly ignored. |
| `CACKLE_HOST_NAME` | — | — | What to call this Cackle on the front page, e.g. `The Bijou`. Unset means unnamed: with `single` the organisation's own name is used, and otherwise the heading simply carries no name. Cackle never invents one. |
| `CACKLE_DEMO` | `--demo` | `false` | Boot with a fully seeded demo organisation, event, ticket types, and the `stub` payment provider. Zero setup, meant for evaluation, screenshots, and local development — not for anything real. |

## What your front page shows

> **In plain English:** this is your Cackle, on your machine. The front page
> lists *your* events — it is not a listing site, and there is no directory
> of other people's events anywhere in this software. This setting is only
> about how your page presents itself when you run more than one
> organisation on the same box.

`CACKLE_HOST_SCOPE` takes one of three values.

**`own` — the default.** The front page lists the published events of every
organisation on this machine. If you run one organisation, that is simply
your events, and the page shows no organisation labels at all. If you run
several — a venue and a festival, say, or a hall that hosts three promoters
— each event is labelled with the organisation it belongs to, and each label
links to just that organisation's events.

**`single` — present as one organisation.** The front page *is* one
organisation's page. Name it with `CACKLE_HOST_ORG` (its URL name, the one
in its web address):

```bash
CACKLE_HOST_SCOPE=single
CACKLE_HOST_ORG=the-bijou
CACKLE_HOST_NAME="The Bijou"
```

Any other organisation on the same machine keeps working normally — its
events sell, its gate scans, its organisers sign in — but its events do not
appear on this front page. This is the right setting for a single venue that
happens to share a box with someone else, and it is a one-line change.

If `CACKLE_HOST_ORG` names no organisation, the listing returns an error
rather than falling back to showing everybody. A typo must not publish
events you meant to keep off the page.

**`peers` — reserved, and does nothing yet.** This value is accepted so that
a setting written today keeps its meaning later, but **there is no peer event
source in this binary**: with `peers`, Cackle behaves *exactly* as `own` —
your own organisations' events, and nothing from anywhere else. Nothing in
Cackle discovers other installations, and there is no global feed, no
"organisers near you", and no directory. The listing API states this
outright in its response (`"peers_included": false`), so nothing built on
top of it can quietly assume otherwise.

### What the listing API returns

`GET /api/events` answers with the events **and** a `host` object saying
whose they are:

```json
{
  "events": [ … ],
  "host": {
    "scope": "own",
    "name": "The Bijou",
    "organisations": [{ "id": "…", "name": "The Bijou", "slug": "the-bijou" }],
    "multi_org": false,
    "peers_included": false
  }
}
```

`organisations` lists only organisations that currently have at least one
published event (so an organisation still working on a draft is not named to
the public), except under `single`, where the organisation you configured is
always named even when it has nothing on. `multi_org` is what a page uses to
decide whether to show organisation labels at all. `GET /api/events?host=the-bijou`
narrows the listing to one organisation; a name that is not on this host
answers `404`, the same as one that does not exist.

## Payment provider secrets

Cackle is country and currency agnostic: there is no default paid provider.
`manual` needs no configuration at all — it's always on. Every other
provider is opt-in, enabled via `CACKLE_PAYMENT_PROVIDERS` above, and reads
its own credentials from `CACKLE_<PROVIDER>_*` environment variables — never
a default, never committed, never logged. A few examples (see
[PAYMENTS.md](PAYMENTS.md) for the complete, per-adapter list):

| Provider | Env vars |
|---|---|
| Stripe | `CACKLE_STRIPE_SECRET_KEY`, `CACKLE_STRIPE_WEBHOOK_SECRET` |
| Paystack | `CACKLE_PAYSTACK_SECRET_KEY` |
| PayPal | `CACKLE_PAYPAL_CLIENT_ID`, `CACKLE_PAYPAL_CLIENT_SECRET`, `CACKLE_PAYPAL_WEBHOOK_ID`, `CACKLE_PAYPAL_ENV` |
| BTCPay Server | `CACKLE_BTCPAY_BASE_URL`, `CACKLE_BTCPAY_API_KEY`, `CACKLE_BTCPAY_STORE_ID`, `CACKLE_BTCPAY_WEBHOOK_SECRET` |
| LNbits | `CACKLE_LNBITS_BASE_URL`, `CACKLE_LNBITS_API_KEY`, `CACKLE_LNBITS_WEBHOOK_SECRET` |

## Accounts without email

**Cackle sends no email.** There is no SMTP client, no provider SDK and no
sender of any kind in the binary, and nothing to configure to change that.
That is a deliberate consequence of the same choice that makes `manual` the
default payment provider: a gate that has to reach a third party before it
can work is a gate that stops working when the internet does.

Two things a hosted product would normally do by email are done by handing
somebody a link instead.

**Adding staff to your organisation.** On the Team page, "Create invite
link" produces a link. Send it to them yourself — WhatsApp, SMS, read it
out. It is shown once (the server keeps only a hash of it), works for seven
days, and only works for the email address you invited. If you lose it,
revoke the invite and make another. This is how you add a `scanner`, which
is how you staff a door.

**Resetting a password.** There is no self-service reset, because there is
nowhere to send one. You, the operator, run this on the machine Cackle is
installed on:

```bash
cackle reset-password -email someone@example.com
```

It prints a link, valid once and for one hour. Give it to them; they choose
their own password, and every existing session for that account is signed
out. Add `-db` if your database is not at `./cackle.db`, and `-base-url` (or
set `CACKLE_BASE_URL`) if the link must point at something other than the
default — the address in the link has to be one the recipient can actually
reach, which on a venue LAN is usually not `localhost`.

The `POST /api/auth/password-reset` HTTP route still exists and still mints
a token, but nothing delivers it and the response never contains it, so it
is unusable unless you build delivery yourself. See
[API.md](API.md#auth).

## Notes

- **Restart required for all of the above.** Cackle reads configuration once
  at startup; there is no live-reload.
- **`CACKLE_DB` is *almost* the entire state of a Cackle instance.** It is no
  longer sufficient on its own: event signing keys inside it are encrypted, so a
  restore needs the database *and* the key material above — backed up
  **separately**, or the archive holds both the lock and the key. Back both up
  like you mean it — see [SELF-HOSTING.md](SELF-HOSTING.md#backups).
- **`--demo` and real provider secrets don't mix well on purpose.** Demo
  mode uses the `stub` provider regardless of what else is configured, so
  you can't accidentally run a real event against seeded demo data.
- **Secrets never appear in logs.** If you ever see one, that's a bug — file
  it per [SECURITY.md](../SECURITY.md).

## Docker

The Docker image reads the same env vars, and already sets
`CACKLE_ADDR=:8080` and `CACKLE_DB=/srv/data/cackle.db` (the image's `WORKDIR`
and declared `VOLUME` are both `/srv/data`). A minimal real deployment —
here using just `manual`, which needs no secrets at all:

```bash
docker build -t vulos/cackle .
docker run -d --name cackle \
  -p 8080:8080 \
  -e CACKLE_BASE_URL=https://tickets.example.com \
  -v cackle-data:/srv/data \
  vulos/cackle
```

To enable an optional provider instead (or in addition), set
`CACKLE_PAYMENT_PROVIDERS` and that provider's own secrets, e.g. for
Stripe:

```bash
docker run -d --name cackle \
  -p 8080:8080 \
  -e CACKLE_BASE_URL=https://tickets.example.com \
  -e CACKLE_PAYMENT_PROVIDERS=manual,stripe \
  -e CACKLE_STRIPE_SECRET_KEY=sk_live_xxxxx \
  -e CACKLE_STRIPE_WEBHOOK_SECRET=whsec_xxxxx \
  -v cackle-data:/srv/data \
  vulos/cackle
```

Mount a volume at `/srv/data` (or wherever you override `CACKLE_DB` to
point), or the database — and every event key, order, and ticket in it —
disappears when the container is removed.
