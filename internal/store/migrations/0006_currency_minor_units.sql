-- 0006_currency_minor_units.sql — country/currency-agnostic money model.
--
-- "Cents" was never a universal truth: ISO-4217 defines currencies with
-- ZERO decimal places (JPY, KRW, VND, CLP, ISK, ...) and THREE decimal
-- places (KWD, BHD, JOD, OMR, TND) — most (but not all) currencies have
-- two. Every money column in this schema is renamed from *_cents to
-- *_minor to reflect what it actually stores: an integer count of the
-- currency's own minor unit, whatever that currency's exponent is (see
-- internal/money, the authoritative ISO-4217 table this codebase now
-- routes every amount/currency pair through). This is a pure rename —
-- SQLite's RENAME COLUMN preserves every existing value; no data is
-- rewritten or reinterpreted.
--
-- orgs gains default_currency: an event's currency now defaults from its
-- owning org (internal/events.Service.Create) rather than a hardcoded
-- literal. The 'USD' column-level default below only matters for the
-- NOT NULL backfill on existing rows — every code path that creates an
-- org going forward validates/normalizes an explicit ISO-4217 code via
-- internal/money before this column is ever written.
--
-- events.currency already existed (0001_init.sql) and is NOT renamed —
-- only the amount columns needed the *_cents -> *_minor fix. Its column
-- level `DEFAULT 'ZAR'` is intentionally left alone: it is dead weight
-- (every INSERT already supplies an explicit, validated currency; SQLite
-- cannot cheaply ALTER a column's DEFAULT without a full table rebuild,
-- and rebuilding a live table is a strictly riskier migration than
-- leaving an unreachable default in place).
--
-- payment_records is new: it backs a durable, restart-proof audit trail
-- for payment providers that used to hold ALL of their state in memory
-- (internal/payments' manual and lnbits adapters — see their doc
-- comments). One row per (provider, reference). manual additionally uses
-- marked_by/marked_at as its auditable "who confirmed this order paid,
-- and when" trail — the payments contract's non-negotiable requirement
-- for the default provider.
--
-- Idempotency: like every migration in this directory, this file is
-- applied AT MOST ONCE per database — internal/store.Migrate records each
-- applied version in schema_migrations and never re-runs it. It is not
-- separately safe to execute twice by hand outside that runner (a second
-- RENAME COLUMN would fail with "no such column", exactly like a second
-- bare ADD COLUMN would fail in 0003/0004/0005) — that guarantee has
-- always come from the migration runner, not from each file being
-- independently re-runnable, and this file follows the same contract.

ALTER TABLE orgs ADD COLUMN default_currency TEXT NOT NULL DEFAULT 'USD';

ALTER TABLE ticket_types RENAME COLUMN price_cents TO price_minor;

ALTER TABLE orders RENAME COLUMN subtotal_cents TO subtotal_minor;
ALTER TABLE orders RENAME COLUMN fee_cents TO fee_minor;
ALTER TABLE orders RENAME COLUMN total_cents TO total_minor;

ALTER TABLE order_items RENAME COLUMN unit_price_cents TO unit_price_minor;

ALTER TABLE payouts RENAME COLUMN amount_cents TO amount_minor;
-- payouts never previously carried its own currency (it only ever had an
-- implicit ZAR assumption via the event it paid out). A payout is always
-- denominated in the event's own currency; nothing currently writes this
-- table in production (CreatePayout has no caller yet — see
-- internal/store/payouts.go), so there is no existing data to backfill.
ALTER TABLE payouts ADD COLUMN currency TEXT NOT NULL DEFAULT '';

CREATE TABLE payment_records (
    provider     TEXT NOT NULL,
    reference    TEXT NOT NULL,
    amount_minor INTEGER NOT NULL,
    currency     TEXT NOT NULL,
    status       TEXT NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    marked_by    TEXT NOT NULL DEFAULT '',
    marked_at    TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (provider, reference)
);
CREATE INDEX idx_payment_records_provider ON payment_records(provider);
