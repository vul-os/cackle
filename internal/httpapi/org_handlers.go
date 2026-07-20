package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/orgs"
	"github.com/vul-os/cackle/internal/store"
)

// handleListOrgMembers serves GET /api/orgs/{id}/members — admin+ on the
// org.
func (s *server) handleListOrgMembers(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	orgID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, orgID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "list org members rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this org")
		return
	}

	members, err := s.deps.Orgs.ListMembers(r.Context(), orgID)
	if err != nil {
		internalError(w, s.log(), "list org members", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

type createInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// handleCreateOrgInvite serves POST /api/orgs/{id}/invites — admin+ on the
// org. The plaintext token is returned exactly once here and never again;
// only its hash is persisted (see internal/orgs.CreateInvite).
func (s *server) handleCreateOrgInvite(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	orgID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, orgID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "create invite rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this org")
		return
	}

	var req createInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	inv, token, err := s.deps.Orgs.CreateInvite(r.Context(), orgID, req.Email, req.Role, user.ID)
	if err != nil {
		if errors.Is(err, orgs.ErrInvalidInput) {
			badRequest(w, "email and a valid role (owner, admin, scanner) are required")
			return
		}
		internalError(w, s.log(), "create invite", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"invite_id":  inv.ID,
		"token":      token,
		"expires_at": nowText(inv.ExpiresAt),
	})
}

// handleListOrgInvites serves GET /api/orgs/{id}/invites — admin+ on the
// org.
func (s *server) handleListOrgInvites(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	orgID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, orgID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "list invites rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this org")
		return
	}

	invites, err := s.deps.Orgs.ListPendingInvites(r.Context(), orgID)
	if err != nil {
		internalError(w, s.log(), "list invites", err)
		return
	}

	type inviteView struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		ExpiresAt string `json:"expires_at"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]inviteView, 0, len(invites))
	for _, inv := range invites {
		out = append(out, inviteView{ID: inv.ID, Email: inv.Email, Role: inv.Role, ExpiresAt: nowText(inv.ExpiresAt), CreatedAt: nowText(inv.CreatedAt)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

// handleDeleteOrgInvite serves DELETE /api/invites/{id} — admin+ on the
// invite's org, resolved via the invite itself (the route only identifies
// the invite), mirroring ticketTypeEventID/ImageEventID.
func (s *server) handleDeleteOrgInvite(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	inviteID := chi.URLParam(r, "id")

	orgID, err := s.deps.Store.InviteOrgID(r.Context(), inviteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "invite not found")
			return
		}
		internalError(w, s.log(), "delete invite lookup", err)
		return
	}

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, orgID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "delete invite rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this org")
		return
	}

	if err := s.deps.Orgs.DeleteInvite(r.Context(), inviteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "invite not found")
			return
		}
		internalError(w, s.log(), "delete invite", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type acceptInviteRequest struct {
	Token string `json:"token"`
}

// handleAcceptOrgInvite serves POST /api/invites/accept — any
// authenticated user. It is not org-scoped in the URL (the token itself
// carries the authority), so it sits outside the org/event RBAC table's
// 401+403-non-member shape; see TestRBAC_InvitesAcceptRequiresAuth in
// rbac_test.go for its coverage. internal/orgs additionally requires the
// caller's own account email to match the address the invite was issued
// to (ErrEmailMismatch) — token possession alone is not sufficient.
func (s *server) handleAcceptOrgInvite(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())

	var req acceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	member, err := s.deps.Orgs.AcceptInvite(r.Context(), req.Token, user)
	if err != nil {
		switch {
		case errors.Is(err, orgs.ErrInviteInvalid):
			badRequest(w, "invite is invalid, expired, or already used")
		case errors.Is(err, orgs.ErrEmailMismatch):
			forbidden(w, "this invite was issued to a different email address")
		default:
			internalError(w, s.log(), "accept invite", err)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"org_id": member.OrgID, "role": member.Role})
}

// handleGetBankAccount serves GET /api/orgs/{id}/bank-account — owner
// only.
func (s *server) handleGetBankAccount(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	orgID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, orgID, auth.RoleOwner)
	if err != nil {
		internalError(w, s.log(), "get bank account rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an owner of this org")
		return
	}

	acct, err := s.deps.Orgs.GetBankAccount(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "no bank account on file for this org")
			return
		}
		internalError(w, s.log(), "get bank account", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bank_account": acct})
}

type setBankAccountRequest struct {
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
}

// handleSetBankAccount serves PUT /api/orgs/{id}/bank-account — owner
// only. The account number is never logged: it is decoded straight into
// req and passed to internal/orgs, and neither this handler nor
// internal/orgs ever pass it to internalError/log.* — see
// internal/orgs.SetBankAccount's doc comment.
func (s *server) handleSetBankAccount(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	orgID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, orgID, auth.RoleOwner)
	if err != nil {
		internalError(w, s.log(), "set bank account rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an owner of this org")
		return
	}

	var req setBankAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	if err := s.deps.Orgs.SetBankAccount(r.Context(), orgID, req.BankCode, req.AccountNumber, req.AccountName); err != nil {
		if errors.Is(err, orgs.ErrInvalidInput) {
			badRequest(w, "bank_code, account_number, and account_name are required and must be valid")
			return
		}
		// Never echo the underlying error: it may be a provider response
		// that itself embedded request details. internalError already logs
		// server-side only.
		internalError(w, s.log(), "set bank account", err)
		return
	}

	acct, err := s.deps.Orgs.GetBankAccount(r.Context(), orgID)
	if err != nil {
		internalError(w, s.log(), "set bank account reload", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bank_account": acct})
}

// handleListBanks serves GET /api/banks — any authenticated user (it is
// reference data, not org-scoped: populating a bank-account form dropdown
// requires no particular org membership).
func (s *server) handleListBanks(w http.ResponseWriter, r *http.Request) {
	banks, err := s.deps.Orgs.ListBanks(r.Context())
	if err != nil {
		internalError(w, s.log(), "list banks", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"banks": banks})
}

// handleEventPayouts serves GET /api/events/{id}/payouts — admin+ on the
// event's org (payout figures are financial data at the same sensitivity
// bar as the bank account itself; a scanner-role gate member has no
// legitimate need to see them — see the RBAC test table's comment about
// the old app's unprotected payouts route).
func (s *server) handleEventPayouts(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "event payouts rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	summary, err := s.deps.Orgs.EventPayoutSummary(r.Context(), eventID)
	if err != nil {
		internalError(w, s.log(), "event payouts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payouts": summary})
}
