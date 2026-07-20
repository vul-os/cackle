package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PasswordResetToken is a single-use password reset token. TokenHash is the
// sha256 hex digest of the token emailed to the user — the plaintext token
// is never persisted, mirroring session tokens.
type PasswordResetToken struct {
	TokenHash string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
	UsedAt    *time.Time
}

// CreatePasswordResetToken inserts a new reset token row.
func (s *Store) CreatePasswordResetToken(ctx context.Context, t *PasswordResetToken) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO password_reset_tokens (token_hash, user_id, expires_at, created_at, used_at)
		VALUES (?, ?, ?, ?, ?)`,
		t.TokenHash, t.UserID, timeToText(t.ExpiresAt), timeToText(t.CreatedAt), nullTimeToText(t.UsedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create password reset token: %w", err)
	}
	return nil
}

// GetPasswordResetToken looks up a reset token by its hash. Returns
// ErrNotFound if absent. Callers must check ExpiresAt and UsedAt themselves.
func (s *Store) GetPasswordResetToken(ctx context.Context, tokenHash string) (*PasswordResetToken, error) {
	var t PasswordResetToken
	var expiresAt, createdAt string
	var usedAt sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT token_hash, user_id, expires_at, created_at, used_at
		FROM password_reset_tokens WHERE token_hash = ?`, tokenHash,
	).Scan(&t.TokenHash, &t.UserID, &expiresAt, &createdAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan password reset token: %w", err)
	}

	if t.ExpiresAt, err = textToTime(expiresAt); err != nil {
		return nil, fmt.Errorf("store: parse password reset token expires_at: %w", err)
	}
	if t.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse password reset token created_at: %w", err)
	}
	if t.UsedAt, err = textToNullTime(usedAt); err != nil {
		return nil, fmt.Errorf("store: parse password reset token used_at: %w", err)
	}
	return &t, nil
}

// MarkPasswordResetTokenUsed marks a token as consumed, so it cannot be
// replayed.
func (s *Store) MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string, usedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE password_reset_tokens SET used_at = ? WHERE token_hash = ?`, timeToText(usedAt), tokenHash)
	if err != nil {
		return fmt.Errorf("store: mark password reset token used: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// DeleteExpiredPasswordResetTokens removes every token past its expiry,
// returning the number removed. Intended for a periodic sweep.
func (s *Store) DeleteExpiredPasswordResetTokens(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE expires_at <= ?`, timeToText(now))
	if err != nil {
		return 0, fmt.Errorf("store: delete expired password reset tokens: %w", err)
	}
	return res.RowsAffected()
}
