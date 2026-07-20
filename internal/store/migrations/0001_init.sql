-- 0001_init.sql — Cackle core schema.
-- Money is always integer cents. IDs are ULIDs (TEXT). Timestamps are
-- RFC3339 TEXT. This migration is applied inside a transaction by the
-- migration runner in internal/store/store.go.

CREATE TABLE users (
    id                TEXT PRIMARY KEY,
    email             TEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    name              TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    email_verified_at TEXT
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE oauth_identities (
    provider   TEXT NOT NULL,
    subject    TEXT NOT NULL,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY (provider, subject)
);
CREATE INDEX idx_oauth_identities_user_id ON oauth_identities(user_id);

CREATE TABLE orgs (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE org_members (
    org_id     TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'scanner')),
    created_at TEXT NOT NULL,
    PRIMARY KEY (org_id, user_id)
);
CREATE INDEX idx_org_members_user_id ON org_members(user_id);

CREATE TABLE events (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    slug        TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL,
    summary     TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    venue_name  TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    lat         REAL,
    lng         REAL,
    starts_at   TEXT NOT NULL,
    ends_at     TEXT NOT NULL,
    timezone    TEXT NOT NULL DEFAULT 'UTC',
    cover_image TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'cancelled')),
    currency    TEXT NOT NULL DEFAULT 'ZAR',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX idx_events_org_id ON events(org_id);
CREATE INDEX idx_events_status ON events(status);

CREATE TABLE event_keys (
    id          TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    public_key  BLOB NOT NULL,
    private_key BLOB NOT NULL,
    created_at  TEXT NOT NULL,
    revoked_at  TEXT
);
CREATE INDEX idx_event_keys_event_id ON event_keys(event_id);

CREATE TABLE ticket_types (
    id             TEXT PRIMARY KEY,
    event_id       TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    price_cents    INTEGER NOT NULL DEFAULT 0,
    quantity_total INTEGER NOT NULL DEFAULT 0,
    quantity_sold  INTEGER NOT NULL DEFAULT 0,
    sales_start    TEXT,
    sales_end      TEXT,
    max_per_order  INTEGER NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'draft',
    sort_order     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_ticket_types_event_id ON ticket_types(event_id);

CREATE TABLE orders (
    id             TEXT PRIMARY KEY,
    event_id       TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id        TEXT REFERENCES users(id) ON DELETE SET NULL,
    buyer_email    TEXT NOT NULL,
    buyer_name     TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'failed', 'refunded', 'cancelled')),
    subtotal_cents INTEGER NOT NULL DEFAULT 0,
    fee_cents      INTEGER NOT NULL DEFAULT 0,
    total_cents    INTEGER NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'ZAR',
    provider       TEXT NOT NULL DEFAULT '',
    provider_ref   TEXT,
    created_at     TEXT NOT NULL,
    paid_at        TEXT
);
CREATE INDEX idx_orders_event_id ON orders(event_id);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE UNIQUE INDEX idx_orders_provider_ref ON orders(provider, provider_ref) WHERE provider_ref IS NOT NULL;

CREATE TABLE order_items (
    id               TEXT PRIMARY KEY,
    order_id         TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    ticket_type_id   TEXT NOT NULL REFERENCES ticket_types(id),
    quantity         INTEGER NOT NULL,
    unit_price_cents INTEGER NOT NULL
);
CREATE INDEX idx_order_items_order_id ON order_items(order_id);

CREATE TABLE tickets (
    id             TEXT PRIMARY KEY,
    order_id       TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    event_id       TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    ticket_type_id TEXT NOT NULL REFERENCES ticket_types(id),
    holder_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    holder_name    TEXT NOT NULL DEFAULT '',
    serial         TEXT NOT NULL UNIQUE,
    capability     TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'valid' CHECK (status IN ('valid', 'void', 'refunded')),
    issued_at      TEXT NOT NULL,
    voided_at      TEXT
);
CREATE INDEX idx_tickets_order_id ON tickets(order_id);
CREATE INDEX idx_tickets_event_id ON tickets(event_id);
CREATE INDEX idx_tickets_holder_user_id ON tickets(holder_user_id);

-- Admission dedupe is LOCAL and append-only: every scan attempt gets its own
-- row (result may be admitted/duplicate/invalid/wrong_event). The partial
-- unique index below is the actual dedupe guarantee — it allows at most one
-- 'admitted' row per ticket ("first scan wins") while letting duplicate scan
-- attempts insert freely as their own 'duplicate' rows, never overwriting.
CREATE TABLE admissions (
    id         TEXT PRIMARY KEY,
    ticket_id  TEXT NOT NULL REFERENCES tickets(id),
    event_id   TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    gate_id    TEXT NOT NULL DEFAULT '',
    scanned_by TEXT,
    device_id  TEXT NOT NULL DEFAULT '',
    scanned_at TEXT NOT NULL,
    result     TEXT NOT NULL CHECK (result IN ('admitted', 'duplicate', 'invalid', 'wrong_event')),
    note       TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX idx_admissions_admitted_once ON admissions(ticket_id) WHERE result = 'admitted';
CREATE INDEX idx_admissions_event_id ON admissions(event_id);
CREATE INDEX idx_admissions_ticket_id ON admissions(ticket_id);
-- Backs the /api/scan/sync idempotency contract: idempotent by
-- (ticket_id, device_id, scanned_at).
CREATE INDEX idx_admissions_sync_key ON admissions(ticket_id, device_id, scanned_at);

CREATE TABLE allocations (
    id             TEXT PRIMARY KEY,
    event_id       TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    device_id      TEXT NOT NULL,
    ticket_type_id TEXT REFERENCES ticket_types(id),
    count          INTEGER NOT NULL DEFAULT 0,
    issued_at      TEXT NOT NULL,
    expires_at     TEXT,
    signature      TEXT NOT NULL
);
CREATE INDEX idx_allocations_event_id ON allocations(event_id);
CREATE INDEX idx_allocations_device_id ON allocations(device_id);

CREATE TABLE payouts (
    id           TEXT PRIMARY KEY,
    event_id     TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    org_id       TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    amount_cents INTEGER NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    provider_ref TEXT,
    created_at   TEXT NOT NULL,
    paid_at      TEXT
);
CREATE INDEX idx_payouts_event_id ON payouts(event_id);
CREATE INDEX idx_payouts_org_id ON payouts(org_id);
