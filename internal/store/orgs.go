package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vul-os/cackle/internal/money"
)

// Org is a ticket-selling organisation (the tenant boundary for events).
type Org struct {
	ID        string
	Name      string
	Slug      string
	// DefaultCurrency is the ISO-4217 alpha-3 code new events under this
	// org default to when the event itself doesn't specify one — see
	// internal/events.Service.Create. Cackle has no privileged global
	// default currency; this is the org's own choice, always a
	// money.Normalize-validated code (never empty).
	DefaultCurrency string
	CreatedAt       time.Time
}

// OrgMember is a user's role within an org. Role is one of
// "owner", "admin", "scanner" (enforced by a CHECK constraint).
type OrgMember struct {
	OrgID     string
	UserID    string
	Role      string
	CreatedAt time.Time
}

// OrgWithRole is an org joined with the calling user's role in it — the
// shape GET /api/auth/me needs for its orgs:[{id,name,role}] list.
type OrgWithRole struct {
	Org
	Role string
}

// CreateOrg inserts a new org. If ID or CreatedAt are zero they are
// populated. DefaultCurrency defaults to "USD" if left empty — a plain,
// pragmatic fallback for the NOT NULL column, not a privileged currency:
// every caller that lets an organiser actually choose (once that surface
// exists) should pass an explicit code instead. Either way it is always
// validated/normalized via internal/money before being persisted, so a
// malformed or unknown code is rejected here rather than silently stored.
func (s *Store) CreateOrg(ctx context.Context, o *Org) error {
	if o.ID == "" {
		o.ID = NewID()
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now()
	}
	if o.DefaultCurrency == "" {
		o.DefaultCurrency = "USD"
	}
	norm, err := money.Normalize(o.DefaultCurrency)
	if err != nil {
		return fmt.Errorf("store: create org: default_currency: %w", err)
	}
	o.DefaultCurrency = norm

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO orgs (id, name, slug, default_currency, created_at) VALUES (?, ?, ?, ?, ?)`,
		o.ID, o.Name, o.Slug, o.DefaultCurrency, timeToText(o.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create org: %w", err)
	}
	return nil
}

const orgSelectColumns = `SELECT id, name, slug, default_currency, created_at`

// GetOrgByID looks up an org by primary key. Returns ErrNotFound if absent.
func (s *Store) GetOrgByID(ctx context.Context, id string) (*Org, error) {
	return s.scanOrg(s.db.QueryRowContext(ctx, orgSelectColumns+` FROM orgs WHERE id = ?`, id))
}

// GetOrgBySlug looks up an org by its unique slug. Returns ErrNotFound if
// absent.
func (s *Store) GetOrgBySlug(ctx context.Context, slug string) (*Org, error) {
	return s.scanOrg(s.db.QueryRowContext(ctx, orgSelectColumns+` FROM orgs WHERE slug = ?`, slug))
}

func (s *Store) scanOrg(row *sql.Row) (*Org, error) {
	var o Org
	var createdAt string
	err := row.Scan(&o.ID, &o.Name, &o.Slug, &o.DefaultCurrency, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan org: %w", err)
	}
	if o.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse org created_at: %w", err)
	}
	return &o, nil
}

// AddOrgMember inserts a membership row (org_id, user_id) -> role. Fails if
// the membership already exists (use UpdateOrgMemberRole to change role).
func (s *Store) AddOrgMember(ctx context.Context, m *OrgMember) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		m.OrgID, m.UserID, m.Role, timeToText(m.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: add org member: %w", err)
	}
	return nil
}

// GetOrgMember looks up a single membership. Returns ErrNotFound if the
// user is not a member of the org — this is the primary RBAC lookup.
func (s *Store) GetOrgMember(ctx context.Context, orgID, userID string) (*OrgMember, error) {
	var m OrgMember
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT org_id, user_id, role, created_at FROM org_members WHERE org_id = ? AND user_id = ?`,
		orgID, userID,
	).Scan(&m.OrgID, &m.UserID, &m.Role, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan org member: %w", err)
	}
	if m.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse org member created_at: %w", err)
	}
	return &m, nil
}

// UpdateOrgMemberRole changes an existing member's role.
func (s *Store) UpdateOrgMemberRole(ctx context.Context, orgID, userID, role string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE org_members SET role = ? WHERE org_id = ? AND user_id = ?`, role, orgID, userID)
	if err != nil {
		return fmt.Errorf("store: update org member role: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// RemoveOrgMember deletes a membership row.
func (s *Store) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, userID)
	if err != nil {
		return fmt.Errorf("store: remove org member: %w", err)
	}
	return nil
}

// ListOrgsForUser returns every org a user belongs to, with their role in
// each — the shape needed for GET /api/auth/me.
func (s *Store) ListOrgsForUser(ctx context.Context, userID string) ([]OrgWithRole, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id, o.name, o.slug, o.default_currency, o.created_at, m.role
		FROM org_members m
		JOIN orgs o ON o.id = m.org_id
		WHERE m.user_id = ?
		ORDER BY o.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list orgs for user: %w", err)
	}
	defer rows.Close()

	var out []OrgWithRole
	for rows.Next() {
		var ow OrgWithRole
		var createdAt string
		if err := rows.Scan(&ow.ID, &ow.Name, &ow.Slug, &ow.DefaultCurrency, &createdAt, &ow.Role); err != nil {
			return nil, fmt.Errorf("store: scan org for user: %w", err)
		}
		if ow.CreatedAt, err = textToTime(createdAt); err != nil {
			return nil, fmt.Errorf("store: parse org created_at: %w", err)
		}
		out = append(out, ow)
	}
	return out, rows.Err()
}

// OrgMemberWithUser is a membership row joined with the member's own
// name/email — the shape GET /api/orgs/{id}/members needs, so the
// organiser team screen never has to make N follow-up user lookups.
type OrgMemberWithUser struct {
	UserID    string
	Name      string
	Email     string
	Role      string
	CreatedAt time.Time
}

// ListOrgMembersWithUser returns every member of an org joined with their
// name/email, ordered by membership creation time.
func (s *Store) ListOrgMembersWithUser(ctx context.Context, orgID string) ([]OrgMemberWithUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.user_id, u.name, u.email, m.role, m.created_at
		FROM org_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.org_id = ?
		ORDER BY m.created_at ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: list org members with user: %w", err)
	}
	defer rows.Close()

	var out []OrgMemberWithUser
	for rows.Next() {
		var m OrgMemberWithUser
		var createdAt string
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan org member with user: %w", err)
		}
		if m.CreatedAt, err = textToTime(createdAt); err != nil {
			return nil, fmt.Errorf("store: parse org member created_at: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListOrgMembers returns every member of an org.
func (s *Store) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMember, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT org_id, user_id, role, created_at FROM org_members WHERE org_id = ? ORDER BY created_at`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: list org members: %w", err)
	}
	defer rows.Close()

	var out []OrgMember
	for rows.Next() {
		var m OrgMember
		var createdAt string
		if err := rows.Scan(&m.OrgID, &m.UserID, &m.Role, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan org member: %w", err)
		}
		if m.CreatedAt, err = textToTime(createdAt); err != nil {
			return nil, fmt.Errorf("store: parse org member created_at: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetOrgMemberWithUser looks up a single membership joined with the
// member's own name/email — the shape a role-change response needs so the
// caller never has to make a separate user lookup. Returns ErrNotFound if
// the user is not a member of the org.
func (s *Store) GetOrgMemberWithUser(ctx context.Context, orgID, userID string) (*OrgMemberWithUser, error) {
	var m OrgMemberWithUser
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT m.user_id, u.name, u.email, m.role, m.created_at
		FROM org_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.org_id = ? AND m.user_id = ?`, orgID, userID,
	).Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan org member with user: %w", err)
	}
	if m.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse org member created_at: %w", err)
	}
	return &m, nil
}

// CountOrgOwners returns how many members of orgID currently hold the
// "owner" role — the check UpdateMemberRole uses to refuse demoting the
// last owner (which would lock everyone out of managing the org
// permanently).
func (s *Store) CountOrgOwners(ctx context.Context, orgID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM org_members WHERE org_id = ? AND role = 'owner'`, orgID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count org owners: %w", err)
	}
	return n, nil
}

// EventOrgID returns the org_id owning an event. This thin read is exposed
// from store (rather than internal/events) because internal/auth's RBAC
// helpers need it and must not depend on internal/events. Returns
// ErrNotFound if the event does not exist.
func (s *Store) EventOrgID(ctx context.Context, eventID string) (string, error) {
	var orgID string
	err := s.db.QueryRowContext(ctx, `SELECT org_id FROM events WHERE id = ?`, eventID).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: event org id: %w", err)
	}
	return orgID, nil
}
