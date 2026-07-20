-- 0005_org_bank_accounts.sql — an org's payout destination (wave 3 backend
-- contract). One row per org: an org has exactly one bank account on file
-- at a time (PUT replaces it wholesale). account_number is stored in full
-- so a later real transfer/reconciliation can use it, but internal/orgs
-- NEVER returns it in full over the API — only the last 4 digits — and it
-- must never appear in a log line (see internal/orgs' doc comments).
-- recipient_code is Paystack's own reference for this payout destination,
-- set when a live payments.Paystack provider is configured; it is blank
-- when Cackle is run without one (self-host/demo), which is a supported,
-- non-error configuration.

CREATE TABLE org_bank_accounts (
    org_id         TEXT PRIMARY KEY REFERENCES orgs(id) ON DELETE CASCADE,
    bank_code      TEXT NOT NULL,
    bank_name      TEXT NOT NULL DEFAULT '',
    account_number TEXT NOT NULL,
    account_name   TEXT NOT NULL,
    recipient_code TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
