package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/vul-os/cackle/internal/store"
)

// Role is an org membership role. The hierarchy is owner > admin > scanner:
// owners and admins can do anything a scanner can, admins can do anything
// scanner-level plus manage the event, owners additionally manage the org
// itself (members, payouts).
type Role string

const (
	RoleOwner   Role = "owner"
	RoleAdmin   Role = "admin"
	RoleScanner Role = "scanner"
)

var roleRank = map[Role]int{
	RoleScanner: 1,
	RoleAdmin:   2,
	RoleOwner:   3,
}

// RoleMeets reports whether actual meets or exceeds min in the role
// hierarchy. An unrecognised role (typo, tampered claim) never meets
// anything — this fails closed.
func RoleMeets(actual, min Role) bool {
	a, aok := roleRank[actual]
	m, mok := roleRank[min]
	if !aok || !mok {
		return false
	}
	return a >= m
}

// CanManageEvent reports whether userID holds at least minRole within the
// org that owns eventID. This is the primary RBAC gate for org/event
// routes: call it on EVERY org/event route handler using the DB-backed
// membership, never a client-supplied role claim (the old app's unprotected
// /admin/events/:id/payouts is exactly the mistake this guards against).
//
// A nonexistent event or a user with no membership both return (false,
// nil) — RBAC denial, not an error. Only a genuine storage failure returns
// a non-nil error.
func (s *Service) CanManageEvent(ctx context.Context, userID, eventID string, minRole Role) (bool, error) {
	orgID, err := s.store.EventOrgID(ctx, eventID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("auth: can manage event: %w", err)
	}
	return s.CanManageOrg(ctx, userID, orgID, minRole)
}

// CanManageOrg reports whether userID holds at least minRole within orgID.
func (s *Service) CanManageOrg(ctx context.Context, userID, orgID string, minRole Role) (bool, error) {
	m, err := s.store.GetOrgMember(ctx, orgID, userID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("auth: can manage org: %w", err)
	}
	return RoleMeets(Role(m.Role), minRole), nil
}
