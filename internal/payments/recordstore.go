package payments

import (
	"context"
	"time"
)

// PaymentRecord is one provider's durable, restart-proof state for a
// single reference. It generalizes what manual.go's ManualRecord and
// lnbits.go's lnbitsInvoiceRecord each used to keep ONLY in memory into
// one storable shape shared by every provider that needs it.
//
// MarkedBy/MarkedAt are manual's own auditable "who confirmed this order
// paid, and when" fields (see manual.go) — other providers that don't
// have a human-in-the-loop confirmation step simply leave them zero.
type PaymentRecord struct {
	Provider     string
	Reference    string
	AmountMinor  int64
	Currency     string
	Status       Status
	Instructions string
	MarkedBy     string
	MarkedAt     time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RecordStore is the durability seam a Provider MAY use so its state
// survives a process restart, mirroring how SeenStore/OrderLookup let this
// package depend on storage behaviour without importing internal/store
// directly. internal/store implements this against a payment_records
// table; passing nil to a provider constructor keeps that provider's
// previous in-memory-only behaviour (still exactly what every existing
// test in this package exercises) — nil is always a valid, supported
// configuration, just one that does not survive a restart.
type RecordStore interface {
	// GetPaymentRecord looks up one record by (provider, reference). ok is
	// false if no such record exists (never an error in that case).
	GetPaymentRecord(ctx context.Context, provider, reference string) (rec PaymentRecord, ok bool, err error)
	// PutPaymentRecord inserts or wholesale-replaces the record for
	// (rec.Provider, rec.Reference).
	PutPaymentRecord(ctx context.Context, rec PaymentRecord) error
	// ListPaymentRecords returns every record for provider, in no
	// particular order — used to warm-start a provider's in-memory cache
	// after a restart.
	ListPaymentRecords(ctx context.Context, provider string) ([]PaymentRecord, error)
}
