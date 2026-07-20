package main

import (
	"context"

	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// paymentRecordStoreAdapter satisfies payments.RecordStore against
// *store.Store, converting between store.PaymentRecord (a plain,
// storage-native row shape) and payments.PaymentRecord (the shape
// internal/payments' providers speak) — the same adapter pattern
// internal/httpapi already uses for payments.SeenStore/OrderLookup, kept
// here instead because this is where providers are constructed.
type paymentRecordStoreAdapter struct{ store *store.Store }

func (a paymentRecordStoreAdapter) GetPaymentRecord(ctx context.Context, provider, reference string) (payments.PaymentRecord, bool, error) {
	rec, err := a.store.GetPaymentRecord(ctx, provider, reference)
	if err != nil {
		if err == store.ErrNotFound {
			return payments.PaymentRecord{}, false, nil
		}
		return payments.PaymentRecord{}, false, err
	}
	return fromStoreRecord(rec), true, nil
}

func (a paymentRecordStoreAdapter) PutPaymentRecord(ctx context.Context, rec payments.PaymentRecord) error {
	return a.store.PutPaymentRecord(ctx, toStoreRecord(rec))
}

func (a paymentRecordStoreAdapter) ListPaymentRecords(ctx context.Context, provider string) ([]payments.PaymentRecord, error) {
	rows, err := a.store.ListPaymentRecords(ctx, provider)
	if err != nil {
		return nil, err
	}
	out := make([]payments.PaymentRecord, len(rows))
	for i := range rows {
		out[i] = fromStoreRecord(&rows[i])
	}
	return out, nil
}

func fromStoreRecord(r *store.PaymentRecord) payments.PaymentRecord {
	out := payments.PaymentRecord{
		Provider:     r.Provider,
		Reference:    r.Reference,
		AmountMinor:  r.AmountMinor,
		Currency:     r.Currency,
		Status:       payments.Status(r.Status),
		Instructions: r.Instructions,
		MarkedBy:     r.MarkedBy,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
	if r.MarkedAt != nil {
		out.MarkedAt = *r.MarkedAt
	}
	return out
}

func toStoreRecord(rec payments.PaymentRecord) *store.PaymentRecord {
	out := &store.PaymentRecord{
		Provider:     rec.Provider,
		Reference:    rec.Reference,
		AmountMinor:  rec.AmountMinor,
		Currency:     rec.Currency,
		Status:       string(rec.Status),
		Instructions: rec.Instructions,
		MarkedBy:     rec.MarkedBy,
		CreatedAt:    rec.CreatedAt,
		UpdatedAt:    rec.UpdatedAt,
	}
	if !rec.MarkedAt.IsZero() {
		markedAt := rec.MarkedAt
		out.MarkedAt = &markedAt
	}
	return out
}
