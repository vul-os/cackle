package main

import (
	"context"

	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// compensatingPaymentAuditStoreAdapter satisfies
// payments.CompensatingPaymentAuditStore against *store.Store, converting
// between store.CompensatingPaymentAudit (a plain, storage-native row shape)
// and payments.CompensatingPaymentAudit (the shape
// internal/payments/compensating.go speaks) — the same adapter pattern
// paymentRecordStoreAdapter already uses in this package for
// payments.RecordStore, kept here for the same reason: this is where
// providers (and, on the refund path, an operator tool) are constructed.
type compensatingPaymentAuditStoreAdapter struct{ store *store.Store }

func (a compensatingPaymentAuditStoreAdapter) PutCompensatingPaymentAudit(ctx context.Context, rec payments.CompensatingPaymentAudit) error {
	return a.store.PutCompensatingPaymentAudit(ctx, toStoreCompensatingPaymentAudit(rec))
}

func (a compensatingPaymentAuditStoreAdapter) ListCompensatingPaymentAudits(ctx context.Context, originalReference string) ([]payments.CompensatingPaymentAudit, error) {
	rows, err := a.store.ListCompensatingPaymentAudits(ctx, originalReference)
	if err != nil {
		return nil, err
	}
	out := make([]payments.CompensatingPaymentAudit, len(rows))
	for i := range rows {
		out[i] = fromStoreCompensatingPaymentAudit(&rows[i])
	}
	return out, nil
}

func fromStoreCompensatingPaymentAudit(r *store.CompensatingPaymentAudit) payments.CompensatingPaymentAudit {
	out := payments.CompensatingPaymentAudit{
		ID:                  r.ID,
		OriginalReference:   r.OriginalReference,
		PayoutReference:     r.PayoutReference,
		RailID:              r.RailID,
		Destination:         r.Destination,
		SenderAddressKnown:  r.SenderAddressKnown,
		SenderAddress:       r.SenderAddress,
		Status:              r.Status,
		Reason:              r.Reason,
		IsRefusal:           r.IsRefusal,
		SameAsSenderAddress: r.SameAsSenderAddress,
		Refused:             r.Refused,
		ApprovedBy:          r.ApprovedBy,
		Executed:            r.Executed,
		AmountMinor:         r.AmountMinor,
		Currency:            r.Currency,
		CreatedAt:           r.CreatedAt,
	}
	if r.ApprovedAt != nil {
		out.ApprovedAt = *r.ApprovedAt
	}
	return out
}

func toStoreCompensatingPaymentAudit(rec payments.CompensatingPaymentAudit) *store.CompensatingPaymentAudit {
	out := &store.CompensatingPaymentAudit{
		ID:                  rec.ID,
		OriginalReference:   rec.OriginalReference,
		PayoutReference:     rec.PayoutReference,
		RailID:              rec.RailID,
		Destination:         rec.Destination,
		SenderAddressKnown:  rec.SenderAddressKnown,
		SenderAddress:       rec.SenderAddress,
		Status:              rec.Status,
		Reason:              rec.Reason,
		IsRefusal:           rec.IsRefusal,
		SameAsSenderAddress: rec.SameAsSenderAddress,
		Refused:             rec.Refused,
		ApprovedBy:          rec.ApprovedBy,
		Executed:            rec.Executed,
		AmountMinor:         rec.AmountMinor,
		Currency:            rec.Currency,
		CreatedAt:           rec.CreatedAt,
	}
	if !rec.ApprovedAt.IsZero() {
		approvedAt := rec.ApprovedAt
		out.ApprovedAt = &approvedAt
	}
	return out
}
