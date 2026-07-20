package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vul-os/cackle/internal/money"
)

// Payout is a single payout record against an event's org (the `payouts`
// table has existed since the original schema, but nothing wrote to it
// until GET /api/events/{id}/payouts needed real rows to show — see
// internal/orgs.EventPayoutSummary). Cackle never holds funds itself; a
// Payout row is a record of a transfer initiated through the org's own
// bank account/Paystack recipient, not an escrowed balance.
type Payout struct {
	ID          string
	EventID     string
	OrgID       string
	AmountMinor int64
	// Currency is the ISO-4217 code this payout is denominated in —
	// always the owning event's own currency (a payout moves exactly the
	// money that event collected; Cackle never converts currencies).
	Currency    string
	Status      string
	ProviderRef *string
	CreatedAt   time.Time
	PaidAt      *time.Time
}

// CreatePayout inserts a new payout row. If ID or CreatedAt are zero they
// are populated. Currency must be a validated ISO-4217 code (normalized
// via internal/money) — callers should pass the owning event's own
// currency.
func (s *Store) CreatePayout(ctx context.Context, p *Payout) error {
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	norm, err := money.Normalize(p.Currency)
	if err != nil {
		return fmt.Errorf("store: create payout: currency: %w", err)
	}
	p.Currency = norm
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO payouts (id, event_id, org_id, amount_minor, currency, status, provider_ref, created_at, paid_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.EventID, p.OrgID, p.AmountMinor, p.Currency, p.Status, nullString(p.ProviderRef),
		timeToText(p.CreatedAt), nullTimeToText(p.PaidAt),
	)
	if err != nil {
		return fmt.Errorf("store: create payout: %w", err)
	}
	return nil
}

// ListPayoutsForEvent returns every payout record for an event, most
// recently created first.
func (s *Store) ListPayoutsForEvent(ctx context.Context, eventID string) ([]Payout, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, event_id, org_id, amount_minor, currency, status, provider_ref, created_at, paid_at
		FROM payouts WHERE event_id = ? ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list payouts for event: %w", err)
	}
	defer rows.Close()

	var out []Payout
	for rows.Next() {
		var p Payout
		var providerRef sql.NullString
		var createdAt string
		var paidAt sql.NullString
		if err := rows.Scan(&p.ID, &p.EventID, &p.OrgID, &p.AmountMinor, &p.Currency, &p.Status, &providerRef, &createdAt, &paidAt); err != nil {
			return nil, fmt.Errorf("store: scan payout: %w", err)
		}
		if providerRef.Valid {
			p.ProviderRef = &providerRef.String
		}
		if p.CreatedAt, err = textToTime(createdAt); err != nil {
			return nil, fmt.Errorf("store: parse payout created_at: %w", err)
		}
		if p.PaidAt, err = textToNullTime(paidAt); err != nil {
			return nil, fmt.Errorf("store: parse payout paid_at: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EventRevenue is an event's gross/fee totals derived from PAID orders
// only — the same "paid orders, never the reservation counter" discipline
// as TicketTypeStatsForEvent.
type EventRevenue struct {
	GrossMinor int64
	FeesMinor  int64
}

// OrderRevenueForEvent sums subtotal_minor and fee_minor across every PAID
// order for an event — the gross/fees figures GET
// /api/events/{id}/payouts reports. Gross is the buyer-facing ticket
// subtotal (excluding any Cackle/processor fee layered on top), matching
// how internal/events.Stats already treats "revenue".
func (s *Store) OrderRevenueForEvent(ctx context.Context, eventID string) (EventRevenue, error) {
	var rev EventRevenue
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(subtotal_minor), 0), COALESCE(SUM(fee_minor), 0)
		FROM orders WHERE event_id = ? AND status = 'paid'`, eventID,
	).Scan(&rev.GrossMinor, &rev.FeesMinor)
	if errors.Is(err, sql.ErrNoRows) {
		return rev, nil
	}
	if err != nil {
		return EventRevenue{}, fmt.Errorf("store: order revenue for event: %w", err)
	}
	return rev, nil
}
