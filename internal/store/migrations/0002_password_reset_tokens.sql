-- 0002_password_reset_tokens.sql — needed by internal/auth for
-- POST /api/auth/password-reset and /api/auth/password-update. Not listed
-- in the original schema sketch but required to implement that flow, so it
-- is added here rather than bolted onto the users table.

CREATE TABLE password_reset_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    used_at    TEXT
);
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
