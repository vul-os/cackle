# Configuration

Cackle is configured env-first. Every key is prefixed `CACKLE_`. Flags
mirror the env vars for the same setting. No config file is required or
supported — this is intentional, to keep the single-binary story simple.

## Reference

| Variable | Flag | Default | Description |
|---|---|---|---|
| `CACKLE_ADDR` | `--addr` | `:8080` | HTTP listen address. |
| `CACKLE_DB` | `--db` | `./cackle.db` | Path to the SQLite database file. Created (and migrated) on first boot if it doesn't exist. |
| `CACKLE_BASE_URL` | `--base-url` | — | The public URL Cackle is reachable at. Used to build links in emails and payment provider callback URLs. Set this to your real domain before taking payments — a wrong or missing base URL breaks the payment callback round-trip. |
| `CACKLE_SESSION_SECRET` | `--session-secret` | auto-generated, persisted | Secret used to sign session tokens. If unset, Cackle generates one on first boot and persists it (in the database) so subsequent restarts don't invalidate every session. Set it explicitly in any multi-instance deployment so all instances share one secret. |
| `CACKLE_PAYMENT_PROVIDERS` | — | unset (every registered provider enabled) | Comma-separated allowlist of optional payment providers for this deployment, e.g. `manual,stripe,paystack`. `manual` cannot be disabled and is always enabled regardless of this variable. See [PAYMENTS.md](PAYMENTS.md). |
| `CACKLE_DEMO` | `--demo` | `false` | Boot with a fully seeded demo organisation, event, ticket types, and the `stub` payment provider. Zero setup, meant for evaluation, screenshots, and local development — not for anything real. |

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

## Notes

- **Restart required for all of the above.** Cackle reads configuration once
  at startup; there is no live-reload.
- **`CACKLE_DB` is the entire state of a Cackle instance.** Back it up like
  you mean it — see [SELF-HOSTING.md](SELF-HOSTING.md#backups).
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
