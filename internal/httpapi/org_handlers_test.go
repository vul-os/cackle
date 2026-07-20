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
