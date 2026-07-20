package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/orgs"
	"github.com/vul-os/cackle/internal/store"
)

// --- members -----------------------------------------------------------

func TestListOrgMembers(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "members")

	rec := h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/members", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list members: status %d body %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[struct {
		Members []struct {
			UserID string `json:"user_id"`
			Name   string `json:"name"`
			Email  string `json:"email"`
			Role   string `json:"role"`
		} `json:"members"`
	}](t, rec)
	if len(resp.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(resp.Members))
	}
	if resp.Members[0].UserID != fx.ownerID || resp.Members[0].Role != "owner" {
		t.Fatalf("unexpected member row: %+v", resp.Members[0])
	}
	if resp.Members[0].Email == "" || resp.Members[0].Name == "" {
		t.Fatalf("expected name+email populated, got %+v", resp.Members[0])
	}
}

// TestUpdateOrgMemberRole_OwnerCanChangeAdminOrScanner covers the happy
// path for PATCH /api/orgs/{id}/members/{user_id}: an owner can move a
// non-owner member between admin and scanner freely.
func TestUpdateOrgMemberRole_OwnerCanChangeAdminOrScanner(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "role-change")

	memberToken, memberID := h.signupUser("member-role-change@example.com", "member-password-123", "Member")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: memberID, Role: "scanner"}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	rec := h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+memberID, fx.ownerToken, map[string]any{"role": "admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("promote to admin: status %d body %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[struct {
		Member struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"member"`
	}](t, rec)
	if resp.Member.Role != "admin" || resp.Member.UserID != memberID {
		t.Fatalf("unexpected member after promotion: %+v", resp.Member)
	}

	// The promoted admin can now do admin-only things (e.g. list members).
	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/members", memberToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("promoted admin list members: status %d body %s", rec.Code, rec.Body.String())
	}

	// Demote back to scanner.
	rec = h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+memberID, fx.ownerToken, map[string]any{"role": "scanner"})
	if rec.Code != http.StatusOK {
		t.Fatalf("demote to scanner: status %d body %s", rec.Code, rec.Body.String())
	}

	// An unrecognised role is a clean 400, not a 500 or silent no-op.
	rec = h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+memberID, fx.ownerToken, map[string]any{"role": "superadmin"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus role: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}

	// A nonexistent target user is a clean 404.
	rec = h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/does-not-exist", fx.ownerToken, map[string]any{"role": "admin"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nonexistent member: expected 404, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestUpdateOrgMemberRole_AdminCannotChangeRoles proves the route sits at
// the OWNER bar, not admin+: an admin attempting to change anyone's role
// (including their own) is forbidden, same as a plain member would be.
func TestUpdateOrgMemberRole_AdminCannotChangeRoles(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "role-change-admin")

	adminToken, adminID := h.signupUser("admin-role-change@example.com", "admin-password-123", "Admin")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: adminID, Role: "admin"}); err != nil {
		t.Fatalf("add admin: %v", err)
	}

	rec := h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+adminID, adminToken, map[string]any{"role": "owner"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin self-promoting to owner: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)
}

// TestUpdateOrgMemberRole_RefusesDemotingLastOwner is the mandatory
// regression test called out in the task: demoting an org's only owner
// must be refused outright, since it would permanently lock everyone out
// of managing the org (no owner left to reverse the mistake).
func TestUpdateOrgMemberRole_RefusesDemotingLastOwner(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "last-owner")

	// fx's owner is the org's ONLY owner. Demoting them must fail.
	rec := h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+fx.ownerID, fx.ownerToken, map[string]any{"role": "admin"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("demote sole owner: expected 409, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)

	// Role must be unchanged — the owner still has full access afterward.
	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/members", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner should retain access after refused demotion: status %d", rec.Code)
	}
	membersResp := decodeBody[struct {
		Members []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"members"`
	}](t, rec)
	found := false
	for _, m := range membersResp.Members {
		if m.UserID == fx.ownerID {
			found = true
			if m.Role != "owner" {
				t.Fatalf("sole owner's role changed to %q despite refusal", m.Role)
			}
		}
	}
	if !found {
		t.Fatal("owner missing from members list")
	}

	// Promote a second member to owner: NOW demoting the first owner must
	// succeed, since at least one owner always remains.
	_, secondID := h.signupUser("second-owner-last-owner@example.com", "second-password-123", "Second")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: secondID, Role: "scanner"}); err != nil {
		t.Fatalf("add second member: %v", err)
	}
	rec = h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+secondID, fx.ownerToken, map[string]any{"role": "owner"})
	if rec.Code != http.StatusOK {
		t.Fatalf("promote second owner: status %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+fx.ownerID, fx.ownerToken, map[string]any{"role": "admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("demote original owner once a co-owner exists: status %d body %s", rec.Code, rec.Body.String())
	}
}

// TestOrgInvite_AdminCannotSelfEscalateToOwner is the regression test for a
// verified privilege-escalation finding: an admin (admin+-gated on
// POST /api/orgs/{id}/invites) could previously invite ITSELF (or anyone
// else) at role "owner", accept the invite, and walk away with full
// owner-level authority — including reading the org's bank account — even
// though PATCH /api/orgs/{id}/members/{user_id} (owner-only, with its own
// last-owner guard) correctly refused the equivalent role change through
// the front door. The invite route was a backdoor around that gate.
// CreateInvite must refuse to mint an invite for a role higher than the
// inviter's own (orgs.ErrRoleEscalation, surfaced here as 403).
func TestOrgInvite_AdminCannotSelfEscalateToOwner(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-escalation")

	adminToken, adminID := h.signupUser("admin-escalation@example.com", "admin-password-123", "Admin")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: adminID, Role: "admin"}); err != nil {
		t.Fatalf("add admin: %v", err)
	}

	// The exact reproduction: an admin invites the email address of an
	// account they ALREADY control (their own) at role "owner".
	rec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", adminToken, map[string]any{
		"email": "admin-escalation@example.com", "role": "owner",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin self-inviting as owner: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
	assertErrorShape(t, rec)

	// Also refused when inviting a THIRD PARTY at owner — not just self.
	rec = h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", adminToken, map[string]any{
		"email": "accomplice-escalation@example.com", "role": "owner",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin inviting a third party as owner: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}

	// The admin's own role must remain admin — no invite was created, no
	// membership was touched.
	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/members", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list members: status %d", rec.Code)
	}
	members := decodeBody[struct {
		Members []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"members"`
	}](t, rec)
	for _, m := range members.Members {
		if m.UserID == adminID && m.Role != "admin" {
			t.Fatalf("admin's role changed to %q despite refused invite", m.Role)
		}
	}

	// An admin inviting at admin or scanner (at or below their own rank)
	// still works fine — this is a ceiling, not a total lockout.
	rec = h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", adminToken, map[string]any{
		"email": "legit-invitee-escalation@example.com", "role": "admin",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin inviting at admin (own rank): expected 201, got %d body %s", rec.Code, rec.Body.String())
	}

	// The owner, by contrast, CAN mint an owner-role invite.
	rec = h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, map[string]any{
		"email": "new-owner-escalation@example.com", "role": "owner",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("owner inviting at owner: expected 201, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestOrgInvite_AcceptRefusesStaleInviteAboveInviterCurrentRole covers the
// defence-in-depth half of the same fix: AcceptInvite re-checks, at accept
// time, that the invite's role does not exceed what the inviter (recorded
// on the invite row at creation time) can CURRENTLY grant. A stale invite
// minted before this fix existed (or by any future bug upstream of
// CreateInvite's own guard) must not still be redeemable for a role above
// the inviter's current rank — here simulated by demoting the inviter
// after the invite was minted but before it's accepted.
func TestOrgInvite_AcceptRefusesStaleInviteAboveInviterCurrentRole(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-stale-escalation")

	// A second owner mints a legitimate owner-role invite...
	secondOwnerToken, secondOwnerID := h.signupUser("second-owner-stale@example.com", "second-password-123", "Second Owner")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: secondOwnerID, Role: "owner"}); err != nil {
		t.Fatalf("add second owner: %v", err)
	}
	rec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", secondOwnerToken, map[string]any{
		"email": "invitee-stale-escalation@example.com", "role": "owner",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint owner invite: status %d body %s", rec.Code, rec.Body.String())
	}
	inv := decodeBody[struct {
		Token string `json:"token"`
	}](t, rec)

	// ...but is then demoted (still leaving the org with fx's original
	// owner in place, so this isn't blocked by the last-owner guard).
	rec = h.do(http.MethodPatch, "/api/orgs/"+fx.orgID+"/members/"+secondOwnerID, fx.ownerToken, map[string]any{"role": "admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("demote second owner: status %d body %s", rec.Code, rec.Body.String())
	}

	// The now-stale owner-role invite must be refused at accept time, even
	// though it was validly minted at the time.
	inviteeToken, _ := h.signupUser("invitee-stale-escalation@example.com", "invitee-password-123", "Invitee")
	rec = h.do(http.MethodPost, "/api/invites/accept", inviteeToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("accept stale owner invite: expected 400 (invalid), got %d body %s", rec.Code, rec.Body.String())
	}
}

// --- invites -------------------------------------------------------------

func TestOrgInvite_AcceptAddsMemberAtInvitedRole(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-accept")

	inviteeEmail := "invitee-accept@example.com"
	rec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, map[string]any{
		"email": inviteeEmail, "role": "scanner",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite: status %d body %s", rec.Code, rec.Body.String())
	}
	inv := decodeBody[struct {
		InviteID  string `json:"invite_id"`
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}](t, rec)
	if inv.Token == "" || inv.InviteID == "" || inv.ExpiresAt == "" {
		t.Fatalf("expected populated invite_id/token/expires_at, got %+v", inv)
	}

	// Pending invite shows up for the org.
	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list invites: status %d body %s", rec.Code, rec.Body.String())
	}
	list := decodeBody[struct {
		Invites []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"invites"`
	}](t, rec)
	if len(list.Invites) != 1 || list.Invites[0].Email != inviteeEmail {
		t.Fatalf("expected 1 pending invite for %s, got %+v", inviteeEmail, list.Invites)
	}

	// The invited person signs up (with the exact email the invite names)
	// and accepts.
	inviteeToken, inviteeID := h.signupUser(inviteeEmail, "invitee-password-123", "Invitee")
	rec = h.do(http.MethodPost, "/api/invites/accept", inviteeToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept invite: status %d body %s", rec.Code, rec.Body.String())
	}
	accepted := decodeBody[struct {
		OrgID string `json:"org_id"`
		Role  string `json:"role"`
	}](t, rec)
	if accepted.OrgID != fx.orgID || accepted.Role != "scanner" {
		t.Fatalf("unexpected accept response: %+v", accepted)
	}

	m, err := h.store.GetOrgMember(h.t.Context(), fx.orgID, inviteeID)
	if err != nil {
		t.Fatalf("get org member after accept: %v", err)
	}
	if m.Role != "scanner" {
		t.Fatalf("member role = %q, want scanner", m.Role)
	}

	// The invite is no longer pending.
	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, nil)
	list = decodeBody[struct {
		Invites []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"invites"`
	}](t, rec)
	if len(list.Invites) != 0 {
		t.Fatalf("expected 0 pending invites after accept, got %d", len(list.Invites))
	}
}

func TestOrgInvite_SingleUseRejectsReplay(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-replay")

	inviteeEmail := "invitee-replay@example.com"
	rec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, map[string]any{
		"email": inviteeEmail, "role": "scanner",
	})
	inv := decodeBody[struct {
		Token string `json:"token"`
	}](t, rec)

	inviteeToken, _ := h.signupUser(inviteeEmail, "invitee-password-123", "Invitee")

	rec = h.do(http.MethodPost, "/api/invites/accept", inviteeToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusOK {
		t.Fatalf("first accept: status %d body %s", rec.Code, rec.Body.String())
	}

	// Replay: same token, same (now-member) user.
	rec = h.do(http.MethodPost, "/api/invites/accept", inviteeToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("replayed accept: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}

	// Replay by a totally different account is equally rejected.
	otherToken, _ := h.signupUser("someone-else-replay@example.com", "other-password-123", "Someone Else")
	rec = h.do(http.MethodPost, "/api/invites/accept", otherToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("replayed accept by different account: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestOrgInvite_ExpiredTokenRejected(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-expiry")

	// Build an already-expired invite directly in the store (mirrors how
	// internal/orgs.CreateInvite persists one, but with ExpiresAt in the
	// past) — token minted the same way the real flow does.
	plaintext, hash, err := auth.NewOpaqueToken()
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	inv := &store.OrgInvite{
		OrgID:     fx.orgID,
		Email:     "invitee-expired@example.com",
		Role:      "scanner",
		TokenHash: hash,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if err := h.store.CreateOrgInvite(h.t.Context(), inv); err != nil {
		t.Fatalf("create expired invite: %v", err)
	}

	inviteeToken, _ := h.signupUser("invitee-expired@example.com", "invitee-password-123", "Invitee")
	rec := h.do(http.MethodPost, "/api/invites/accept", inviteeToken, map[string]any{"token": plaintext})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expired invite accept: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestOrgInvite_EmailMismatchRejected(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-mismatch")

	rec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, map[string]any{
		"email": "intended-recipient@example.com", "role": "admin",
	})
	inv := decodeBody[struct {
		Token string `json:"token"`
	}](t, rec)

	// A DIFFERENT account (not the invited email) tries to redeem it.
	wrongToken, _ := h.signupUser("not-the-invitee@example.com", "wrong-password-123", "Wrong Person")
	rec = h.do(http.MethodPost, "/api/invites/accept", wrongToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("email-mismatch accept: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestOrgInvite_DeleteRevokesPendingInvite(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-delete")

	rec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, map[string]any{
		"email": "invitee-delete@example.com", "role": "scanner",
	})
	inv := decodeBody[struct {
		InviteID string `json:"invite_id"`
		Token    string `json:"token"`
	}](t, rec)

	rec = h.do(http.MethodDelete, "/api/invites/"+inv.InviteID, fx.ownerToken, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete invite: status %d body %s", rec.Code, rec.Body.String())
	}

	inviteeToken, _ := h.signupUser("invitee-delete@example.com", "invitee-password-123", "Invitee")
	rec = h.do(http.MethodPost, "/api/invites/accept", inviteeToken, map[string]any{"token": inv.Token})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("accept after delete: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestOrgInvite_InvalidRoleRejected(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "invite-badrole")

	rec := h.do(http.MethodPost, "/api/orgs/"+fx.orgID+"/invites", fx.ownerToken, map[string]any{
		"email": "someone@example.com", "role": "superadmin",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid role invite: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

// --- bank account ----------------------------------------------------------

func TestBankAccount_SetAndGetIsMasked(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "bank")

	rec := h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/bank-account", fx.ownerToken, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get bank account before set: expected 404, got %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodPut, "/api/orgs/"+fx.orgID+"/bank-account", fx.ownerToken, map[string]any{
		"bank_code": "051001", "account_number": "1234567890", "account_name": "Test Org Events",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("set bank account: status %d body %s", rec.Code, rec.Body.String())
	}

	type bankAccountView struct {
		BankCode           string `json:"bank_code"`
		AccountName        string `json:"account_name"`
		AccountNumberLast4 string `json:"account_number_last4"`
	}
	resp := decodeBody[struct {
		BankAccount bankAccountView `json:"bank_account"`
	}](t, rec)
	if resp.BankAccount.AccountNumberLast4 != "7890" {
		t.Fatalf("account_number_last4 = %q, want 7890", resp.BankAccount.AccountNumberLast4)
	}
	if resp.BankAccount.BankCode != "051001" {
		t.Fatalf("bank_code = %q, want 051001", resp.BankAccount.BankCode)
	}

	// The full account number must never appear anywhere in the response
	// body — only the last 4 digits.
	if containsFullAccountNumber(rec.Body.String(), "1234567890") {
		t.Fatal("response body contains the full account number, want masked")
	}

	rec = h.do(http.MethodGet, "/api/orgs/"+fx.orgID+"/bank-account", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get bank account after set: status %d body %s", rec.Code, rec.Body.String())
	}
	resp = decodeBody[struct {
		BankAccount bankAccountView `json:"bank_account"`
	}](t, rec)
	if resp.BankAccount.AccountNumberLast4 != "7890" {
		t.Fatalf("get after set: account_number_last4 = %q, want 7890", resp.BankAccount.AccountNumberLast4)
	}
}

func containsFullAccountNumber(body, fullNumber string) bool {
	for i := 0; i+len(fullNumber) <= len(body); i++ {
		if body[i:i+len(fullNumber)] == fullNumber {
			return true
		}
	}
	return false
}

func TestBankAccount_AdminCannotSetOnlyOwnerCan(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "bank-admin")

	adminToken, adminID := h.signupUser("admin-bank@example.com", "admin-password-123", "Admin")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: adminID, Role: "admin"}); err != nil {
		t.Fatalf("add admin member: %v", err)
	}

	rec := h.do(http.MethodPut, "/api/orgs/"+fx.orgID+"/bank-account", adminToken, map[string]any{
		"bank_code": "051001", "account_number": "1234567890", "account_name": "Test Org",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin set bank account: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestListBanks_FallsBackWithoutLiveProvider(t *testing.T) {
	h := newTestHarness(t)
	token, _ := h.signupUser("bankslist@example.com", "password-123456", "Someone")

	rec := h.do(http.MethodGet, "/api/banks", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list banks: status %d body %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[struct {
		Banks []struct {
			Name string `json:"name"`
			Code string `json:"code"`
		} `json:"banks"`
	}](t, rec)
	if len(resp.Banks) == 0 {
		t.Fatal("expected a non-empty fallback bank list")
	}
}

// --- payouts + categories -------------------------------------------------

func TestEventPayouts_ComputesGrossFromPaidOrders(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "payouts")

	h.placeAndSettleOrder(t, fx.ownerToken, fx.eventID, fx.ticketTypeID, "buyer-payouts@example.com", "Buyer", 2)

	rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/payouts", fx.ownerToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("event payouts: status %d body %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[struct {
		Payouts orgs.PayoutSummary `json:"payouts"`
	}](t, rec)
	if resp.Payouts.GrossMinor != 30000 { // 2 * 15000 (see newPublishedEvent's ticket type price)
		t.Fatalf("gross_minor = %d, want 30000", resp.Payouts.GrossMinor)
	}
	if resp.Payouts.NetMinor != resp.Payouts.GrossMinor-resp.Payouts.FeesMinor {
		t.Fatalf("net_minor inconsistent: gross=%d fees=%d net=%d", resp.Payouts.GrossMinor, resp.Payouts.FeesMinor, resp.Payouts.NetMinor)
	}
	if resp.Payouts.Status != "unpaid" {
		t.Fatalf("status = %q, want unpaid (revenue exists, no payout rows yet)", resp.Payouts.Status)
	}
}

func TestEventPayouts_ScannerForbidden(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "payouts-scanner")

	scannerToken, scannerID := h.signupUser("scanner-payouts@example.com", "scanner-password-123", "Scanner")
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: fx.orgID, UserID: scannerID, Role: "scanner"}); err != nil {
		t.Fatalf("add scanner member: %v", err)
	}

	rec := h.do(http.MethodGet, "/api/events/"+fx.eventID+"/payouts", scannerToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("scanner event payouts: expected 403, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestCategories_FilterAndDerivedList(t *testing.T) {
	h := newTestHarness(t)
	fx1 := h.newPublishedEvent(t, "cat-music")
	fx2 := h.newPublishedEvent(t, "cat-sports")

	rec := h.do(http.MethodPatch, "/api/events/"+fx1.eventID, fx1.ownerToken, map[string]any{"category": "Live Music!"})
	if rec.Code != http.StatusOK {
		t.Fatalf("set category 1: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = h.do(http.MethodPatch, "/api/events/"+fx2.eventID, fx2.ownerToken, map[string]any{"category": "  Sports  "})
	if rec.Code != http.StatusOK {
		t.Fatalf("set category 2: status %d body %s", rec.Code, rec.Body.String())
	}

	rec = h.do(http.MethodGet, "/api/categories", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list categories: status %d body %s", rec.Code, rec.Body.String())
	}
	cats := decodeBody[struct {
		Categories []struct {
			Slug  string `json:"slug"`
			Label string `json:"label"`
			Count int    `json:"count"`
		} `json:"categories"`
	}](t, rec)
	found := map[string]string{}
	for _, c := range cats.Categories {
		found[c.Slug] = c.Label
	}
	if found["live-music"] != "Live Music" {
		t.Fatalf("expected slug live-music -> label %q, got categories %+v", "Live Music", cats.Categories)
	}
	if found["sports"] != "Sports" {
		t.Fatalf("expected slug sports -> label Sports, got categories %+v", cats.Categories)
	}

	// Filtering the public list by category returns only the matching event.
	rec = h.do(http.MethodGet, "/api/events?category=live-music", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter by category: status %d body %s", rec.Code, rec.Body.String())
	}
	listResp := decodeBody[struct {
		Events []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
		} `json:"events"`
	}](t, rec)
	if len(listResp.Events) != 1 || listResp.Events[0].ID != fx1.eventID {
		t.Fatalf("expected exactly fx1's event for category=live-music, got %+v", listResp.Events)
	}
	if listResp.Events[0].Category != "live-music" {
		t.Fatalf("category = %q, want live-music", listResp.Events[0].Category)
	}
}

// TestListCurrencies_ReturnsFullISO4217Table proves the currency picker
// endpoint isn't a hardcoded handful of currencies — it's the same
// authoritative internal/money table used everywhere else, including
// zero- and three-decimal currencies.
func TestListCurrencies_ReturnsFullISO4217Table(t *testing.T) {
	h := newTestHarness(t)

	rec := h.do(http.MethodGet, "/api/currencies", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list currencies: status %d body %s", rec.Code, rec.Body.String())
	}
	resp := decodeBody[struct {
		Currencies []struct {
			Code     string `json:"code"`
			Name     string `json:"name"`
			Exponent int    `json:"exponent"`
		} `json:"currencies"`
	}](t, rec)

	if len(resp.Currencies) < 100 {
		t.Fatalf("expected the full ISO-4217 table (100+ currencies), got %d", len(resp.Currencies))
	}

	byCode := map[string]int{}
	for _, c := range resp.Currencies {
		byCode[c.Code] = c.Exponent
	}
	if exp, ok := byCode["JPY"]; !ok || exp != 0 {
		t.Fatalf("JPY exponent = %d (ok=%v), want 0", exp, ok)
	}
	if exp, ok := byCode["KWD"]; !ok || exp != 3 {
		t.Fatalf("KWD exponent = %d (ok=%v), want 3", exp, ok)
	}
	if exp, ok := byCode["ZAR"]; !ok || exp != 2 {
		t.Fatalf("ZAR exponent = %d (ok=%v), want 2", exp, ok)
	}
}
