package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// OrgBankAccount is an org's payout destination. AccountNumber is stored in
// full here (persistence only) — internal/orgs is responsible for never
// returning it in full over the API (last 4 digits only) and never logging
// it; this type exists purely to carry the row, not to enforce that policy
// itself.
type OrgBankAccount struct {
	OrgID         string
	BankCode      string
	BankName      string
	AccountNumber string
	AccountName   string
	RecipientCode string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UpsertOrgBankAccount replaces the org's bank account wholesale (an org
// has exactly one on file at a time). CreatedAt is preserved across an
// update via SQLite's upsert-excluded trick.
func (s *Store) UpsertOrgBankAccount(ctx context.Context, a *OrgBankAccount) error {
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO org_bank_accounts (org_id, bank_code, bank_name, account_number, account_name, recipient_code, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(org_id) DO UPDATE SET
			bank_code = excluded.bank_code,
			bank_name = excluded.bank_name,
			account_number = excluded.account_number,
			account_name = excluded.account_name,
			recipient_code = excluded.recipient_code,
			updated_at = excluded.updated_at`,
		a.OrgID, a.BankCode, a.BankName, a.AccountNumber, a.AccountName, a.RecipientCode,
		timeToText(a.CreatedAt), timeToText(a.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: upsert org bank account: %w", err)
	}
	return nil
}

// GetOrgBankAccount looks up an org's bank account. Returns ErrNotFound if
// none is on file.
func (s *Store) GetOrgBankAccount(ctx context.Context, orgID string) (*OrgBankAccount, error) {
	var a OrgBankAccount
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT org_id, bank_code, bank_name, account_number, account_name, recipient_code, created_at, updated_at
		FROM org_bank_accounts WHERE org_id = ?`, orgID,
	).Scan(&a.OrgID, &a.BankCode, &a.BankName, &a.AccountNumber, &a.AccountName, &a.RecipientCode, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get org bank account: %w", err)
	}
	if a.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse org bank account created_at: %w", err)
	}
	if a.UpdatedAt, err = textToTime(updatedAt); err != nil {
		return nil, fmt.Errorf("store: parse org bank account updated_at: %w", err)
	}
	return &a, nil
}
