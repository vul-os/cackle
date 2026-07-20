// Package orgs implements Cackle's org-management surfaces added in wave 3:
// team membership listing, single-use email invites, an org's payout bank
// account, and a read-only per-event payout summary.
//
// RBAC is NOT this package's job: internal/httpapi checks
// auth.CanManageOrg/CanManageEvent before calling anything here, and every
// exported method on Service trusts its caller already did that — exactly
// the same division of responsibility internal/events uses.
package orgs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// Sentinel errors. Callers should match with errors.Is. store.ErrNotFound
// is returned as-is (unwrapped) from lookups so callers can match it
// directly, mirroring internal/events.
var (
	ErrInvalidInput  = errors.New("orgs: invalid input")
	ErrInviteInvalid = errors.New("orgs: invite is invalid, expired, or already used")
	// ErrEmailMismatch is returned by AcceptInvite when the authenticated
	// caller's own account email does not match the address the invite was
	// issued to — a token is only redeemable by the account it was sent
	// to, not by whoever happens to hold the link.
	ErrEmailMismatch = errors.New("orgs: this invite was issued to a different email address")
)

// DefaultInviteTTL is how long a freshly created invite is valid.
const DefaultInviteTTL = 7 * 24 * time.Hour

// validRoles mirrors the CHECK constraint on org_members.role /
// org_invites.role.
var validRoles = map[string]bool{"owner": true, "admin": true, "scanner": true}

// Member is one org membership joined with the member's own name/email —
// the shape GET /api/orgs/{id}/members needs.
type Member struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// Invite is a pending (or, briefly, just-created) org invitation. It never
// carries the plaintext token — that is returned exactly once, directly
// from CreateInvite's return value, and never again.
type Invite struct {
	ID        string    `json:"invite_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// BankAccountView is an org's payout destination as returned over the API
// — the account number is masked to its last 4 digits; the full number
// never leaves internal/store.
type BankAccountView struct {
	BankCode           string    `json:"bank_code"`
	BankName           string    `json:"bank_name"`
	AccountName        string    `json:"account_name"`
	AccountNumberLast4 string    `json:"account_number_last4"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// PayoutRow is one payout record within a PayoutSummary. Currency is
// always the owning event's own currency (a payout is a transfer of
// exactly the money that event collected — Cackle never converts
// currencies), duplicated onto each row so a client never has to
// cross-reference the event to render an amount correctly.
type PayoutRow struct {
	ID          string     `json:"id"`
	AmountMinor int64      `json:"amount_minor"`
	Currency    string     `json:"currency"`
	Status      string     `json:"status"`
	ProviderRef string     `json:"provider_ref,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
}

// PayoutSummary is GET /api/events/{id}/payouts' response body. Currency
// is the event's own ISO-4217 currency — every amount in this summary
// (Gross/Fees/Net and every Rows[i].AmountMinor) is denominated in it.
type PayoutSummary struct {
	GrossMinor int64       `json:"gross_minor"`
	FeesMinor  int64       `json:"fees_minor"`
	NetMinor   int64       `json:"net_minor"`
	Currency   string      `json:"currency"`
	Status     string      `json:"status"`
	Rows       []PayoutRow `json:"rows"`
}

// BankingProvider is the seam for listing supported banks and registering
// a payout recipient — satisfied by *payments.PaystackProvider in
// production (its ListBanks/CreateRecipient methods already match this
// interface exactly; no adapter needed). A nil BankingProvider is a valid,
// supported configuration (no live Paystack secret configured — the
// common case for --demo and self-host without a merchant account):
// ListBanks falls back to a small built-in list of major South African
// banks, and SetBankAccount stores details locally without registering a
// live recipient.
type BankingProvider interface {
	ListBanks(ctx context.Context) ([]payments.Bank, error)
	CreateRecipient(ctx context.Context, req payments.RecipientRequest) (payments.Recipient, error)
}

// Service is the entrypoint for all org-management operations added in
// wave 3.
type Service struct {
	store   *store.Store
	banking BankingProvider // nil is valid; see BankingProvider doc
	now     func() time.Time
}

// New constructs a Service backed by st. banking may be nil (see
// BankingProvider's doc comment).
func New(st *store.Store, banking BankingProvider) *Service {
	return &Service{store: st, banking: banking, now: time.Now}
}

// ListMembers returns every member of orgID, joined with their name/email.
func (s *Service) ListMembers(ctx context.Context, orgID string) ([]Member, error) {
	rows, err := s.store.ListOrgMembersWithUser(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("orgs: list members: %w", err)
	}
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		out = append(out, Member{UserID: r.UserID, Name: r.Name, Email: r.Email, Role: r.Role})
	}
	return out, nil
}

// normalizeEmail lower-cases and trims an email for consistent comparison
// — mirrors internal/auth's own normalizeEmail (unexported there, so this
// is a narrow, intentional duplication rather than a new cross-package
// dependency for one string helper).
func normalizeEmail(email string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" || !strings.Contains(e, "@") || strings.HasPrefix(e, "@") || strings.HasSuffix(e, "@") {
		return "", fmt.Errorf("%w: invalid email address", ErrInvalidInput)
	}
	return e, nil
}

// CreateInvite mints a new single-use, hashed, expiring invite for email to
// join orgID at role. Returns the created Invite plus the plaintext token —
// the ONLY time it is ever available; only its sha256 hash is persisted.
func (s *Service) CreateInvite(ctx context.Context, orgID, email, role, invitedBy string) (*Invite, string, error) {
	norm, err := normalizeEmail(email)
	if err != nil {
		return nil, "", err
	}
	if !validRoles[role] {
		return nil, "", fmt.Errorf("%w: role must be one of owner, admin, scanner", ErrInvalidInput)
	}

	plaintext, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return nil, "", fmt.Errorf("orgs: create invite: mint token: %w", err)
	}

	now := s.now()
	inv := &store.OrgInvite{
		OrgID:     orgID,
		Email:     norm,
		Role:      role,
		TokenHash: hash,
		InvitedBy: &invitedBy,
		ExpiresAt: now.Add(DefaultInviteTTL),
		CreatedAt: now,
	}
	if err := s.store.CreateOrgInvite(ctx, inv); err != nil {
		return nil, "", fmt.Errorf("orgs: create invite: %w", err)
	}

	return &Invite{ID: inv.ID, Email: inv.Email, Role: inv.Role, ExpiresAt: inv.ExpiresAt, CreatedAt: inv.CreatedAt}, plaintext, nil
}

// ListPendingInvites returns every not-yet-accepted invite for orgID.
func (s *Service) ListPendingInvites(ctx context.Context, orgID string) ([]Invite, error) {
	rows, err := s.store.ListPendingOrgInvites(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("orgs: list pending invites: %w", err)
	}
	out := make([]Invite, 0, len(rows))
	for _, r := range rows {
		out = append(out, Invite{ID: r.ID, Email: r.Email, Role: r.Role, ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// DeleteInvite revokes an invite outright (pending or already accepted).
func (s *Service) DeleteInvite(ctx context.Context, inviteID string) error {
	if err := s.store.DeleteOrgInvite(ctx, inviteID); err != nil {
		return fmt.Errorf("orgs: delete invite: %w", err)
	}
	return nil
}

// AcceptInvite validates a plaintext invite token — single-use, unexpired,
// and issued to the calling user's own email — and adds (or updates) the
// caller's membership in the invite's org at the invite's role. Returns the
// resulting membership.
func (s *Service) AcceptInvite(ctx context.Context, token string, user *store.User) (*store.OrgMember, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrInviteInvalid
	}

	inv, err := s.store.GetOrgInviteByTokenHash(ctx, auth.HashOpaqueToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("orgs: accept invite: lookup: %w", err)
	}
	if inv.AcceptedAt != nil || !inv.ExpiresAt.After(s.now()) {
		return nil, ErrInviteInvalid
	}

	callerEmail, err := normalizeEmail(user.Email)
	if err != nil {
		return nil, fmt.Errorf("orgs: accept invite: caller email: %w", err)
	}
	if callerEmail != inv.Email {
		return nil, ErrEmailMismatch
	}

	if err := s.store.MarkOrgInviteAccepted(ctx, inv.ID, s.now()); err != nil {
		return nil, fmt.Errorf("orgs: accept invite: mark accepted: %w", err)
	}

	member := &store.OrgMember{OrgID: inv.OrgID, UserID: user.ID, Role: inv.Role, CreatedAt: s.now()}
	if err := s.store.AddOrgMember(ctx, member); err != nil {
		// Already a member (e.g. re-invited at a different role): update
		// the role in place rather than fail the whole accept flow.
		if updErr := s.store.UpdateOrgMemberRole(ctx, inv.OrgID, user.ID, inv.Role); updErr != nil {
			return nil, fmt.Errorf("orgs: accept invite: add/update membership: %w", err)
		}
		m, err := s.store.GetOrgMember(ctx, inv.OrgID, user.ID)
		if err != nil {
			return nil, fmt.Errorf("orgs: accept invite: reload membership: %w", err)
		}
		return m, nil
	}
	return member, nil
}

// accountNumberLast4 masks a bank account number down to its last 4
// digits (or fewer, if the number itself is shorter — never panics, never
// echoes anything beyond that regardless of input length).
func accountNumberLast4(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return accountNumber
	}
	return accountNumber[len(accountNumber)-4:]
}

// GetBankAccount returns orgID's masked bank account details. Returns
// store.ErrNotFound if none is on file.
func (s *Service) GetBankAccount(ctx context.Context, orgID string) (*BankAccountView, error) {
	a, err := s.store.GetOrgBankAccount(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &BankAccountView{
		BankCode:           a.BankCode,
		BankName:           a.BankName,
		AccountName:        a.AccountName,
		AccountNumberLast4: accountNumberLast4(a.AccountNumber),
		UpdatedAt:          a.UpdatedAt,
	}, nil
}

// minAccountNumberLen/maxAccountNumberLen bound a plausible bank account
// number length (South African account numbers are typically 8-11 digits;
// this is deliberately a little generous either side rather than a tight
// country-specific rule).
const (
	minAccountNumberLen = 6
	maxAccountNumberLen = 20
)

// SetBankAccount validates and replaces orgID's bank account wholesale. If
// a live BankingProvider is configured, it registers a transfer recipient
// with the provider first and only persists once that succeeds — a bad
// bank_code/account_number is rejected with the provider's own error
// rather than silently stored. If no BankingProvider is configured (nil),
// details are stored locally with an empty recipient reference; nothing
// here fails just because Cackle is running without a live Paystack
// account (self-host/demo).
func (s *Service) SetBankAccount(ctx context.Context, orgID, bankCode, accountNumber, accountName string) error {
	bankCode = strings.TrimSpace(bankCode)
	accountNumber = strings.TrimSpace(accountNumber)
	accountName = strings.TrimSpace(accountName)

	if bankCode == "" {
		return fmt.Errorf("%w: bank_code is required", ErrInvalidInput)
	}
	if accountName == "" {
		return fmt.Errorf("%w: account_name is required", ErrInvalidInput)
	}
	if len(accountNumber) < minAccountNumberLen || len(accountNumber) > maxAccountNumberLen {
		return fmt.Errorf("%w: account_number must be between %d and %d characters", ErrInvalidInput, minAccountNumberLen, maxAccountNumberLen)
	}
	for _, r := range accountNumber {
		if r < '0' || r > '9' {
			return fmt.Errorf("%w: account_number must contain digits only", ErrInvalidInput)
		}
	}

	var recipientCode, bankName string
	if s.banking != nil {
		rec, err := s.banking.CreateRecipient(ctx, payments.RecipientRequest{
			Name:          accountName,
			AccountNumber: accountNumber,
			BankCode:      bankCode,
		})
		if err != nil {
			return fmt.Errorf("orgs: set bank account: register recipient: %w", err)
		}
		recipientCode = rec.RecipientCode
		bankName = rec.BankName
		if rec.AccountName != "" {
			accountName = rec.AccountName
		}
	}

	acct := &store.OrgBankAccount{
		OrgID:         orgID,
		BankCode:      bankCode,
		BankName:      bankName,
		AccountNumber: accountNumber,
		AccountName:   accountName,
		RecipientCode: recipientCode,
	}
	if err := s.store.UpsertOrgBankAccount(ctx, acct); err != nil {
		return fmt.Errorf("orgs: set bank account: %w", err)
	}
	return nil
}

// fallbackBanks is returned by ListBanks when no live BankingProvider is
// configured, so the bank-account form still has usable options in
// --demo/self-host without a Paystack account. Codes match Paystack's own
// published South African bank codes, so a later live PUT against the
// same code succeeds unmodified once a real provider is configured.
func fallbackBanks() []payments.Bank {
	return []payments.Bank{
		{Name: "Absa Bank", Slug: "absa-bank", Code: "632005", Currency: "ZAR", Active: true},
		{Name: "Capitec Bank", Slug: "capitec-bank", Code: "470010", Currency: "ZAR", Active: true},
		{Name: "First National Bank", Slug: "first-national-bank", Code: "250655", Currency: "ZAR", Active: true},
		{Name: "Nedbank", Slug: "nedbank", Code: "198765", Currency: "ZAR", Active: true},
		{Name: "Standard Bank", Slug: "standard-bank", Code: "051001", Currency: "ZAR", Active: true},
		{Name: "TymeBank", Slug: "tymebank", Code: "678910", Currency: "ZAR", Active: true},
	}
}

// ListBanks returns the bank list a bank-account form should offer: the
// live provider's list if one is configured, or a small built-in fallback
// otherwise (see BankingProvider's doc comment).
func (s *Service) ListBanks(ctx context.Context) ([]payments.Bank, error) {
	if s.banking == nil {
		return fallbackBanks(), nil
	}
	banks, err := s.banking.ListBanks(ctx)
	if err != nil {
		return nil, fmt.Errorf("orgs: list banks: %w", err)
	}
	return banks, nil
}

// EventPayoutSummary computes an event's gross/fee/net revenue (from PAID
// orders only, same discipline as internal/events.Stats) plus every payout
// record ever created for it. Status reflects the most recent payout
// attempt if one exists; otherwise "unpaid" once there is revenue to pay
// out, or "no_sales" if there is none yet.
func (s *Service) EventPayoutSummary(ctx context.Context, eventID string) (*PayoutSummary, error) {
	ev, err := s.store.GetEventByID(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("orgs: event payout summary: look up event: %w", err)
	}
	rev, err := s.store.OrderRevenueForEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("orgs: event payout summary: revenue: %w", err)
	}
	payouts, err := s.store.ListPayoutsForEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("orgs: event payout summary: payouts: %w", err)
	}

	rows := make([]PayoutRow, 0, len(payouts))
	for _, p := range payouts {
		row := PayoutRow{ID: p.ID, AmountMinor: p.AmountMinor, Currency: p.Currency, Status: p.Status, CreatedAt: p.CreatedAt, PaidAt: p.PaidAt}
		if row.Currency == "" {
			row.Currency = ev.Currency
		}
		if p.ProviderRef != nil {
			row.ProviderRef = *p.ProviderRef
		}
		rows = append(rows, row)
	}

	status := "no_sales"
	if rev.GrossMinor > 0 {
		status = "unpaid"
	}
	if len(rows) > 0 {
		// ListPayoutsForEvent orders most-recently-created first.
		status = rows[0].Status
	}

	return &PayoutSummary{
		GrossMinor: rev.GrossMinor,
		FeesMinor:  rev.FeesMinor,
		NetMinor:   rev.GrossMinor - rev.FeesMinor,
		Currency:   ev.Currency,
		Status:     status,
		Rows:       rows,
	}, nil
}
