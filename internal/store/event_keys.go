package store

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// EventKey is one event's Ed25519 issuer keypair, as persisted in
// event_keys. Every event gets its OWN key, generated at creation time —
// see CreateEventWithKey in events.go. There is never a global signing key.
type EventKey struct {
	ID         string
	EventID    string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// CreateEventKey inserts an additional issuer key for an event (key
// rotation). Event creation itself always goes through
// CreateEventWithKey, so this is only needed for rotating/adding keys to
// an existing event.
func (s *Store) CreateEventKey(ctx context.Context, k *EventKey) error {
	if k.ID == "" {
		k.ID = NewID()
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO event_keys (id, event_id, public_key, private_key, created_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		k.ID, k.EventID, []byte(k.PublicKey), []byte(k.PrivateKey), timeToText(k.CreatedAt), nullTimeToText(k.RevokedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create event key: %w", err)
	}
	return nil
}

// ListEventKeys returns every issuer key ever generated for an event,
// including revoked ones, oldest first.
func (s *Store) ListEventKeys(ctx context.Context, eventID string) ([]EventKey, error) {
	return s.queryEventKeys(ctx, `
		SELECT id, event_id, public_key, private_key, created_at, revoked_at
		FROM event_keys WHERE event_id = ? ORDER BY created_at ASC`, eventID)
}

// ActiveEventKeys returns every non-revoked issuer key for an event, oldest
// first. In the common case (no rotation) this is exactly one key — this
// is what a scanner's pinned KeyRing should be built from.
func (s *Store) ActiveEventKeys(ctx context.Context, eventID string) ([]EventKey, error) {
	return s.queryEventKeys(ctx, `
		SELECT id, event_id, public_key, private_key, created_at, revoked_at
		FROM event_keys WHERE event_id = ? AND revoked_at IS NULL ORDER BY created_at ASC`, eventID)
}

// LatestActiveEventKey returns the most recently created non-revoked key
// for an event — the key new tickets should be signed with. Returns
// ErrNotFound if the event has no active key (should not happen for any
// event created via CreateEventWithKey and never revoked into oblivion).
func (s *Store) LatestActiveEventKey(ctx context.Context, eventID string) (*EventKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, event_id, public_key, private_key, created_at, revoked_at
		FROM event_keys WHERE event_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC LIMIT 1`, eventID)
	return s.scanEventKey(row)
}

// RevokeEventKey marks a key revoked at t. It does not delete the row —
// old tickets signed with a revoked key remain cryptographically
// verifiable; revocation only affects whether future KeyRings include it.
func (s *Store) RevokeEventKey(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE event_keys SET revoked_at = ? WHERE id = ?`, timeToText(at), id)
	if err != nil {
		return fmt.Errorf("store: revoke event key: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

func (s *Store) scanEventKey(row *sql.Row) (*EventKey, error) {
	var k EventKey
	var pub, priv []byte
	var createdAt string
	var revokedAt sql.NullString

	err := row.Scan(&k.ID, &k.EventID, &pub, &priv, &createdAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan event key: %w", err)
	}
	k.PublicKey = ed25519.PublicKey(pub)
	k.PrivateKey = ed25519.PrivateKey(priv)
	if k.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse event key created_at: %w", err)
	}
	if k.RevokedAt, err = textToNullTime(revokedAt); err != nil {
		return nil, fmt.Errorf("store: parse event key revoked_at: %w", err)
	}
	return &k, nil
}

func (s *Store) queryEventKeys(ctx context.Context, query string, args ...any) ([]EventKey, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list event keys: %w", err)
	}
	defer rows.Close()

	var out []EventKey
	for rows.Next() {
		var k EventKey
		var pub, priv []byte
		var createdAt string
		var revokedAt sql.NullString
		if err := rows.Scan(&k.ID, &k.EventID, &pub, &priv, &createdAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("store: scan event key row: %w", err)
		}
		k.PublicKey = ed25519.PublicKey(pub)
		k.PrivateKey = ed25519.PrivateKey(priv)
		if k.CreatedAt, err = textToTime(createdAt); err != nil {
			return nil, fmt.Errorf("store: parse event key created_at: %w", err)
		}
		if k.RevokedAt, err = textToNullTime(revokedAt); err != nil {
			return nil, fmt.Errorf("store: parse event key revoked_at: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
