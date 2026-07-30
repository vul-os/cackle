package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CompensatingPaymentAudit is the durable row for one destination-validation
// decision on the compensating-payment (refund) flow — see
// compensating_payment_audits table (0006_compensating_payments.sql) and
// internal/payments/compensating.go. A plain, storage-native type, distinct
// from payments.CompensatingPaymentAudit — the same split as
// store.PaymentRecord vs payments.PaymentRecord: internal/payments' seam
// stays storage-agnostic, and this package never imports internal/payments
// (see payment_records.go's identical doc comment for why).
type CompensatingPaymentAudit struct {
	ID                  string
	OriginalReference   string
	PayoutReference     string
	RailID              string
	Destination         string
	SenderAddressKnown  bool
	SenderAddress       string
	Status              string
	Reason              string
	IsRefusal           bool
	SameAsSenderAddress bool
	Refused             bool
	ApprovedBy          string
	ApprovedAt          *time.Time
	Executed            bool
	AmountMinor         int64
	Currency            string
	CreatedAt           time.Time
}

// PutCompensatingPaymentAudit inserts one row, assigning a.ID when empty —
// the same convention as CreatePayout in payouts.go. Rows here are never
// updated in place: each decision (refused or approved-and-executed) is its
// own row, so the full history of every address a customer supplied for a
// given order's refund survives, not just the latest one.
func (s *Store) PutCompensatingPaymentAudit(ctx context.Context, a *CompensatingPaymentAudit) error {
	if a.ID == "" {
		a.ID = NewID()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO compensating_payment_audits
			(id, original_reference, payout_reference, rail_id, destination,
			 sender_address_known, sender_address, status, reason, is_refusal,
			 same_as_sender_address, refused, approved_by, approved_at, executed,
			 amount_minor, currency, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.OriginalReference, a.PayoutReference, a.RailID, a.Destination,
		boolToInt(a.SenderAddressKnown), a.SenderAddress, a.Status, a.Reason, boolToInt(a.IsRefusal),
		boolToInt(a.SameAsSenderAddress), boolToInt(a.Refused), a.ApprovedBy, nullTimeToText(a.ApprovedAt), boolToInt(a.Executed),
		a.AmountMinor, a.Currency, timeToText(a.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: put compensating payment audit: %w", err)
	}
	return nil
}

const compensatingPaymentAuditSelectColumns = `SELECT id, original_reference, payout_reference, rail_id, destination,
	sender_address_known, sender_address, status, reason, is_refusal,
	same_as_sender_address, refused, approved_by, approved_at, executed,
	amount_minor, currency, created_at`

// ListCompensatingPaymentAudits returns every decision recorded against
// originalReference, most recently created first — the full history of
// every address a customer supplied for this order's refund, refused or
// not.
func (s *Store) ListCompensatingPaymentAudits(ctx context.Context, originalReference string) ([]CompensatingPaymentAudit, error) {
	rows, err := s.db.QueryContext(ctx,
		compensatingPaymentAuditSelectColumns+` FROM compensating_payment_audits WHERE original_reference = ? ORDER BY created_at DESC`,
		originalReference)
	if err != nil {
		return nil, fmt.Errorf("store: list compensating payment audits: %w", err)
	}
	defer rows.Close()

	var out []CompensatingPaymentAudit
	for rows.Next() {
		a, err := scanCompensatingPaymentAuditRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanCompensatingPaymentAuditRow(row rowScanner) (CompensatingPaymentAudit, error) {
	var a CompensatingPaymentAudit
	var senderAddressKnown, isRefusal, sameAsSenderAddress, refused, executed int
	var approvedAt sql.NullString
	var createdAt string
	err := row.Scan(&a.ID, &a.OriginalReference, &a.PayoutReference, &a.RailID, &a.Destination,
		&senderAddressKnown, &a.SenderAddress, &a.Status, &a.Reason, &isRefusal,
		&sameAsSenderAddress, &refused, &a.ApprovedBy, &approvedAt, &executed,
		&a.AmountMinor, &a.Currency, &createdAt)
	if err != nil {
		return CompensatingPaymentAudit{}, fmt.Errorf("store: scan compensating payment audit: %w", err)
	}
	a.SenderAddressKnown = senderAddressKnown != 0
	a.IsRefusal = isRefusal != 0
	a.SameAsSenderAddress = sameAsSenderAddress != 0
	a.Refused = refused != 0
	a.Executed = executed != 0
	if a.ApprovedAt, err = textToNullTime(approvedAt); err != nil {
		return CompensatingPaymentAudit{}, fmt.Errorf("store: parse compensating payment audit approved_at: %w", err)
	}
	if a.CreatedAt, err = textToTime(createdAt); err != nil {
		return CompensatingPaymentAudit{}, fmt.Errorf("store: parse compensating payment audit created_at: %w", err)
	}
	return a, nil
}

// boolToInt renders a Go bool as SQLite's own INTEGER 0/1 convention —
// matching how every other bool-shaped column in this package is stored
// (see e.g. admissions.go).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
