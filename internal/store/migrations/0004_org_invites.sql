-- 0004_org_invites.sql — single-use, hashed org team invites (wave 3
-- backend contract). token_hash mirrors sessions/password_reset_tokens:
-- the plaintext invite token is generated in internal/auth
-- (NewOpaqueToken), handed to the caller exactly once in the API response,
-- and never persisted anywhere — only its sha256 hex digest is stored
-- here, and only that hash is ever looked up against.

CREATE TABLE org_invites (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'scanner')),
    token_hash  TEXT NOT NULL UNIQUE,
    invited_by  TEXT REFERENCES users(id) ON DELETE SET NULL,
    expires_at  TEXT NOT NULL,
    accepted_at TEXT,
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_org_invites_org_id ON org_invites(org_id);
CREATE INDEX idx_org_invites_email ON org_invites(email);
