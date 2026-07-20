package orgs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/vul-os/cackle/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "cackle.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mustUser(t *testing.T, st *store.Store, email string) string {
	t.Helper()
	u := &store.User{Email: email, PasswordHash: "x", Name: email}
	if err := st.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u.ID
}

func mustOrgWithOwner(t *testing.T, st *store.Store, ownerID string) string {
	t.Helper()
	org := &store.Org{Name: "Test Org", Slug: "test-org-" + store.NewID()}
	if err := st.CreateOrg(context.Background(), org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := st.AddOrgMember(context.Background(), &store.OrgMember{OrgID: org.ID, UserID: ownerID, Role: "owner"}); err != nil {
		t.Fatalf("add owner: %v", err)
	}
	return org.ID
}

func TestUpdateMemberRole_ChangesRole(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	svc := New(st, nil)

	ownerID := mustUser(t, st, "owner@example.com")
	orgID := mustOrgWithOwner(t, st, ownerID)
	memberID := mustUser(t, st, "member@example.com")
	if err := st.AddOrgMember(ctx, &store.OrgMember{OrgID: orgID, UserID: memberID, Role: "scanner"}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	m, err := svc.UpdateMemberRole(ctx, orgID, memberID, "admin")
	if err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	if m.Role != "admin" {
		t.Fatalf("role = %q, want admin", m.Role)
	}
	if m.Email != "member@example.com" {
		t.Fatalf("email = %q, want member@example.com", m.Email)
	}

	reloaded, err := st.GetOrgMember(ctx, orgID, memberID)
	if err != nil {
		t.Fatalf("reload member: %v", err)
	}
	if reloaded.Role != "admin" {
		t.Fatalf("persisted role = %q, want admin", reloaded.Role)
	}
}

func TestUpdateMemberRole_RefusesInvalidRole(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	svc := New(st, nil)

	ownerID := mustUser(t, st, "owner2@example.com")
	orgID := mustOrgWithOwner(t, st, ownerID)
	memberID := mustUser(t, st, "member2@example.com")
	if err := st.AddOrgMember(ctx, &store.OrgMember{OrgID: orgID, UserID: memberID, Role: "scanner"}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	if _, err := svc.UpdateMemberRole(ctx, orgID, memberID, "superadmin"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bogus role: got %v, want ErrInvalidInput", err)
	}
}

func TestUpdateMemberRole_NotFoundForNonMember(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	svc := New(st, nil)

	ownerID := mustUser(t, st, "owner3@example.com")
	orgID := mustOrgWithOwner(t, st, ownerID)
	strangerID := mustUser(t, st, "stranger3@example.com")

	if _, err := svc.UpdateMemberRole(ctx, orgID, strangerID, "admin"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("non-member target: got %v, want store.ErrNotFound", err)
	}
}

// TestUpdateMemberRole_RefusesDemotingLastOwner is the specific regression
// this whole feature exists to prevent: demoting (or otherwise moving off
// "owner") an org's ONLY owner would leave nobody able to manage the org —
// no one left to promote another owner, change billing, or reverse the
// mistake. It must be refused outright.
func TestUpdateMemberRole_RefusesDemotingLastOwner(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	svc := New(st, nil)

	ownerID := mustUser(t, st, "sole-owner@example.com")
	orgID := mustOrgWithOwner(t, st, ownerID)

	if _, err := svc.UpdateMemberRole(ctx, orgID, ownerID, "admin"); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("demote sole owner: got %v, want ErrLastOwner", err)
	}
	if _, err := svc.UpdateMemberRole(ctx, orgID, ownerID, "scanner"); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("demote sole owner to scanner: got %v, want ErrLastOwner", err)
	}

	// Role must be untouched.
	m, err := st.GetOrgMember(ctx, orgID, ownerID)
	if err != nil {
		t.Fatalf("reload owner: %v", err)
	}
	if m.Role != "owner" {
		t.Fatalf("sole owner's role changed to %q despite refusal", m.Role)
	}

	// Once a SECOND owner exists, demoting the first is fine — there's
	// always at least one owner left.
	secondOwnerID := mustUser(t, st, "second-owner@example.com")
	if err := st.AddOrgMember(ctx, &store.OrgMember{OrgID: orgID, UserID: secondOwnerID, Role: "owner"}); err != nil {
		t.Fatalf("add second owner: %v", err)
	}
	if _, err := svc.UpdateMemberRole(ctx, orgID, ownerID, "admin"); err != nil {
		t.Fatalf("demote with a co-owner present: %v", err)
	}
}
