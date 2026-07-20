package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PaymentRecord is the durable state one payment provider keeps per
// reference — see the payment_records table (0006_currency_minor_units.sql).
// It exists so providers that previously held their entire state in memory
// (internal/payments' manual and lnbits adapters) survive a process
// restart. This is a plain store-native type — deliberately not
// payments.PaymentRecord itself, mirroring how every other domain package
// (events, orders, orgs) has its own store.* row type distinct from its
// service-layer type; internal/payments adapts between the two (see
// internal/payments' own RecordStore-implementing adapter, wired up in
// cmd/cackle) so this package never has to import internal/payments.
type PaymentRecord struct {
	Provider     string
	Reference    string
	AmountMinor  int64
	Currency     string
	Status       string
	Instructions string
	MarkedBy     string
	MarkedAt     *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PutPaymentRecord inserts a new payment_records row, or wholesale-replaces
// the existing one for the same (provider, reference) — callers always
// pass the full, current state; there is no partial update.
func (s *Store) PutPaymentRecord(ctx context.Context, r *PaymentRecord) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = r.CreatedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payment_records
			(provider, reference, amount_minor, currency, status, instructions, marked_by, marked_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, reference) DO UPDATE SET
			amount_minor = excluded.amount_minor,
			currency     = excluded.currency,
			status       = excluded.status,
			instructions = excluded.instructions,
			marked_by    = excluded.marked_by,
			marked_at    = excluded.marked_at,
			updated_at   = excluded.updated_at`,
		r.Provider, r.Reference, r.AmountMinor, r.Currency, r.Status, r.Instructions,
		r.MarkedBy, nullTimeToText(r.MarkedAt), timeToText(r.CreatedAt), timeToText(r.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: put payment record: %w", err)
	}
	return nil
}

const paymentRecordSelectColumns = `SELECT provider, reference, amount_minor, currency, status,
	instructions, marked_by, marked_at, created_at, updated_at`

// GetPaymentRecord looks up one record by (provider, reference). Returns
// ErrNotFound if absent.
func (s *Store) GetPaymentRecord(ctx context.Context, provider, reference string) (*PaymentRecord, error) {
	row := s.db.QueryRowContext(ctx,
		paymentRecordSelectColumns+` FROM payment_records WHERE provider = ? AND reference = ?`,
		provider, reference)
	return scanPaymentRecord(row)
}

// ListPaymentRecords returns every record for provider, in no particular
// order — used to warm-start a provider's in-memory cache after a restart.
func (s *Store) ListPaymentRecords(ctx context.Context, provider string) ([]PaymentRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		paymentRecordSelectColumns+` FROM payment_records WHERE provider = ?`, provider)
	if err != nil {
		return nil, fmt.Errorf("store: list payment records: %w", err)
	}
	defer rows.Close()

	var out []PaymentRecord
	for rows.Next() {
		r, err := scanPaymentRecordRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanPaymentRecord(row *sql.Row) (*PaymentRecord, error) {
	r, err := scanPaymentRecordRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func scanPaymentRecordRow(row rowScanner) (PaymentRecord, error) {
	var r PaymentRecord
	var markedAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&r.Provider, &r.Reference, &r.AmountMinor, &r.Currency, &r.Status,
		&r.Instructions, &r.MarkedBy, &markedAt, &createdAt, &updatedAt)
	if err != nil {
		return PaymentRecord{}, fmt.Errorf("store: scan payment record: %w", err)
	}
	if r.MarkedAt, err = textToNullTime(markedAt); err != nil {
		return PaymentRecord{}, fmt.Errorf("store: parse payment record marked_at: %w", err)
	}
	if r.CreatedAt, err = textToTime(createdAt); err != nil {
		return PaymentRecord{}, fmt.Errorf("store: parse payment record created_at: %w", err)
	}
	if r.UpdatedAt, err = textToTime(updatedAt); err != nil {
		return PaymentRecord{}, fmt.Errorf("store: parse payment record updated_at: %w", err)
	}
	return r, nil
}
