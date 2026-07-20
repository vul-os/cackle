package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// OrgInvite is a single-use, hashed, expiring invitation for someone to
// join an org at a given role. TokenHash is the sha256 hex digest of the
// plaintext token minted by internal/auth.NewOpaqueToken — the plaintext
// itself is never persisted, mirroring session and password-reset tokens.
type OrgInvite struct {
	ID         string
	OrgID      string
	Email      string
	Role       string
	TokenHash  string
	InvitedBy  *string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	AcceptedAt *time.Time
}

// CreateOrgInvite inserts a new invite row. If ID or CreatedAt are zero
// they are populated.
func (s *Store) CreateOrgInvite(ctx context.Context, inv *OrgInvite) error {
	if inv.ID == "" {
		inv.ID = NewID()
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO org_invites (id, org_id, email, role, token_hash, invited_by, expires_at, created_at, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.OrgID, inv.Email, inv.Role, inv.TokenHash, nullString(inv.InvitedBy),
		timeToText(inv.ExpiresAt), timeToText(inv.CreatedAt), nullTimeToText(inv.AcceptedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create org invite: %w", err)
	}
	return nil
}

// GetOrgInviteByTokenHash looks up an invite by its token hash. Returns
// ErrNotFound if absent. Callers must check ExpiresAt/AcceptedAt
// themselves — this is a raw lookup, mirroring GetPasswordResetToken.
func (s *Store) GetOrgInviteByTokenHash(ctx context.Context, tokenHash string) (*OrgInvite, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, org_id, email, role, token_hash, invited_by, expires_at, created_at, accepted_at
		FROM org_invites WHERE token_hash = ?`, tokenHash)
	return scanOrgInvite(row)
}

// InviteOrgID resolves the org_id an invite belongs to, so a handler can
// run RBAC before allowing DELETE /api/invites/{id}. Returns ErrNotFound if
// absent.
func (s *Store) InviteOrgID(ctx context.Context, inviteID string) (string, error) {
	var orgID string
	err := s.db.QueryRowContext(ctx, `SELECT org_id FROM org_invites WHERE id = ?`, inviteID).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: invite org id: %w", err)
	}
	return orgID, nil
}

// ListPendingOrgInvites returns every not-yet-accepted invite for an org
// (regardless of expiry — a lapsed invite still shows up so an organiser
// can see and delete/re-send it), most recently created first.
func (s *Store) ListPendingOrgInvites(ctx context.Context, orgID string) ([]OrgInvite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, org_id, email, role, token_hash, invited_by, expires_at, created_at, accepted_at
		FROM org_invites WHERE org_id = ? AND accepted_at IS NULL ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: list pending org invites: %w", err)
	}
	defer rows.Close()

	var out []OrgInvite
	for rows.Next() {
		inv, err := scanOrgInviteRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

// DeleteOrgInvite removes an invite row (revoking it, whether pending or
// already accepted). Returns ErrNotFound if absent.
func (s *Store) DeleteOrgInvite(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM org_invites WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete org invite: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// MarkOrgInviteAccepted marks an invite as consumed, so it cannot be
// replayed. Returns ErrNotFound if absent.
func (s *Store) MarkOrgInviteAccepted(ctx context.Context, id string, acceptedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE org_invites SET accepted_at = ? WHERE id = ?`, timeToText(acceptedAt), id)
	if err != nil {
		return fmt.Errorf("store: mark org invite accepted: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

func scanOrgInvite(row *sql.Row) (*OrgInvite, error) {
	var inv OrgInvite
	var invitedBy sql.NullString
	var expiresAt, createdAt string
	var acceptedAt sql.NullString
	err := row.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &invitedBy, &expiresAt, &createdAt, &acceptedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan org invite: %w", err)
	}
	return finishScanOrgInvite(&inv, invitedBy, expiresAt, createdAt, acceptedAt)
}

func scanOrgInviteRow(rows *sql.Rows) (*OrgInvite, error) {
	var inv OrgInvite
	var invitedBy sql.NullString
	var expiresAt, createdAt string
	var acceptedAt sql.NullString
	if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &invitedBy, &expiresAt, &createdAt, &acceptedAt); err != nil {
		return nil, fmt.Errorf("store: scan org invite row: %w", err)
	}
	return finishScanOrgInvite(&inv, invitedBy, expiresAt, createdAt, acceptedAt)
}

func finishScanOrgInvite(inv *OrgInvite, invitedBy sql.NullString, expiresAt, createdAt string, acceptedAt sql.NullString) (*OrgInvite, error) {
	if invitedBy.Valid {
		inv.InvitedBy = &invitedBy.String
	}
	var err error
	if inv.ExpiresAt, err = textToTime(expiresAt); err != nil {
		return nil, fmt.Errorf("store: parse org invite expires_at: %w", err)
	}
	if inv.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse org invite created_at: %w", err)
	}
	if inv.AcceptedAt, err = textToNullTime(acceptedAt); err != nil {
		return nil, fmt.Errorf("store: parse org invite accepted_at: %w", err)
	}
	return inv, nil
}
