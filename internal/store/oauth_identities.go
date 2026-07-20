package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// OAuthIdentity links a third-party OAuth (provider, subject) pair to a
// Cackle user. internal/auth owns the OAuth provider seam; this is just the
// storage side of it.
type OAuthIdentity struct {
	Provider  string
	Subject   string
	UserID    string
	CreatedAt time.Time
}

// CreateOAuthIdentity links a provider identity to a user.
func (s *Store) CreateOAuthIdentity(ctx context.Context, id *OAuthIdentity) error {
	if id.CreatedAt.IsZero() {
		id.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_identities (provider, subject, user_id, created_at) VALUES (?, ?, ?, ?)`,
		id.Provider, id.Subject, id.UserID, timeToText(id.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create oauth identity: %w", err)
	}
	return nil
}

// GetOAuthIdentity looks up a linked identity by (provider, subject).
// Returns ErrNotFound if absent.
func (s *Store) GetOAuthIdentity(ctx context.Context, provider, subject string) (*OAuthIdentity, error) {
	var id OAuthIdentity
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT provider, subject, user_id, created_at FROM oauth_identities WHERE provider = ? AND subject = ?`,
		provider, subject,
	).Scan(&id.Provider, &id.Subject, &id.UserID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan oauth identity: %w", err)
	}
	if id.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse oauth identity created_at: %w", err)
	}
	return &id, nil
}
