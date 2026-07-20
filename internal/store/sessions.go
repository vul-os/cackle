package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Session is a server-side login session. TokenHash is the sha256 hex digest
// of the bearer/cookie token — the plaintext token is never persisted.
type Session struct {
	TokenHash string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// CreateSession inserts a new session row. If CreatedAt is zero it is set
// to time.Now.
func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)`,
		sess.TokenHash, sess.UserID, timeToText(sess.ExpiresAt), timeToText(sess.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// GetSessionByTokenHash looks up a session by its hashed token. Returns
// ErrNotFound if absent. Callers are responsible for checking ExpiresAt —
// this is a pure lookup with no clock dependency, mirroring the offline
// verification discipline used for ticket capabilities.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	var sess Session
	var expiresAt, createdAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT token_hash, user_id, expires_at, created_at
		FROM sessions WHERE token_hash = ?`, tokenHash,
	).Scan(&sess.TokenHash, &sess.UserID, &expiresAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan session: %w", err)
	}

	if sess.ExpiresAt, err = textToTime(expiresAt); err != nil {
		return nil, fmt.Errorf("store: parse session expires_at: %w", err)
	}
	if sess.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse session created_at: %w", err)
	}
	return &sess, nil
}

// DeleteSession revokes a single session (logout). Deleting an unknown
// token is not an error — logout is idempotent.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// DeleteSessionsForUser revokes every session belonging to a user (e.g. on
// password change).
func (s *Store) DeleteSessionsForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("store: delete sessions for user: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes every session whose expiry is at or before
// now, returning the number removed. Intended for a periodic sweep.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, timeToText(now))
	if err != nil {
		return 0, fmt.Errorf("store: delete expired sessions: %w", err)
	}
	return res.RowsAffected()
}
