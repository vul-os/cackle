package payments

import (
	"context"
	"time"
)

// CompensatingPaymentAudit is the durable record of ONE destination-
// validation decision made on the compensating-payment (refund) path — see
// compensating.go (build-tagged `patala`, since forming the decision itself
// needs patala's real ValidateDestination through the Go binding) and the
// sibling patala repo's docs/compensating-payments.md for the full design.
//
// This type deliberately has NO build tag and imports nothing from
// patala-go: Status is a plain string (see compensating.go's
// destinationStatusName, the one place that name is derived from patala's
// real enum), not patala.DestinationStatus, so that internal/store — which
// never imports patala and must keep working in Cackle's default,
// CGO_ENABLED=0 build — can implement CompensatingPaymentAuditStore without
// pulling in cgo. This mirrors why RecordStore/SeenStore/OrderLookup in
// provider.go and recordstore.go are themselves storage-agnostic seams: the
// package that decides never queries storage itself.
//
// A row here is written for EVERY decision this package reaches, refused or
// not — "customer supplied address X, it was refused because Y" is exactly
// as much an audit fact as "address X was approved by Alice and paid out" —
// so ApprovedBy/ApprovedAt/Executed being zero on a row is not an omission,
// it is the record of a refusal (or of a check that was never acted on).
type CompensatingPaymentAudit struct {
	// ID is assigned by the store implementation if left empty — see
	// internal/store.CreatePayout's identical convention.
	ID string
	// OriginalReference is the reference of the payment being compensated
	// for. Carried through for audit ONLY; this package never looks it up.
	OriginalReference string
	// PayoutReference is the compensating payment's OWN reference — empty
	// unless Executed is true. It must never equal OriginalReference (see
	// compensating.go): a payout is a second, independent payment, not a
	// reversal, and needs its own idempotency key.
	PayoutReference string
	// RailID is the patala rail this decision was formed against (matches
	// patala.PatalaRail.Id()).
	RailID string
	// Destination is the exact, customer-supplied address string this
	// decision is about. NEVER inferred from anything else — see this
	// package's own module doc comment and compensating.go.
	Destination string
	// SenderAddressKnown is false when the caller could not determine the
	// ORIGINAL payment's sender address at all. This is not the same as "it
	// differs from Destination" — an unknown sender means the sender-address
	// guard could not run, not that it ran and passed.
	SenderAddressKnown bool
	// SenderAddress is the address the original payment is known to have
	// come FROM, if SenderAddressKnown. Persisted so a later audit can see
	// exactly what Destination was checked against, not just the boolean
	// outcome.
	SenderAddress string
	// Status is patala's DestinationStatus rendered as one of exactly five
	// strings ("Malformed", "WrongNetwork", "NotAWallet",
	// "StructurallyValid", "Unknown") by compensating.go's
	// destinationStatusName. Never re-derived or guessed here.
	Status string
	// Reason is patala's own one-sentence explanation, verbatim — the same
	// text a human confirming the payout was shown.
	Reason string
	// IsRefusal mirrors patala.DestinationVerdict.IsRefusal exactly — never
	// re-derived from Status (see destination.rs and this package's own
	// compensating.go doc comment for why a status-based switch is the
	// wrong answer here).
	IsRefusal bool
	// SameAsSenderAddress is true when Destination matched SenderAddress —
	// cackle's OWN guard, additive to IsRefusal and never folded into it,
	// because patala has no notion of "the address that paid us" at all.
	SameAsSenderAddress bool
	// Refused is IsRefusal || SameAsSenderAddress, persisted rather than
	// re-derived by a reader — the same reasoning IsRefusal itself is
	// persisted as data rather than recomputed from Status.
	Refused bool
	// ApprovedBy is the human identity that confirmed this payout — empty
	// whenever Refused is true (a refusal cannot be approved) and empty on
	// any row where no approval was ever supplied.
	ApprovedBy string
	// ApprovedAt is when ApprovedBy confirmed. Zero unless ApprovedBy is
	// set.
	ApprovedAt time.Time
	// Executed is true only once the compensating Charge actually ran and
	// succeeded — see compensating.go's ExecuteCompensatingPayment. A row
	// with ApprovedBy set but Executed false records an approval that was
	// given but never (yet) acted on.
	Executed bool
	// AmountMinor/Currency are the compensating payment's own amount — see
	// CompensatingPaymentRequest.
	AmountMinor int64
	Currency    string
	CreatedAt   time.Time
}

// CompensatingPaymentAuditStore is the storage-agnostic seam
// CompensatingPaymentAudit rows go through — the same shape as
// RecordStore/SeenStore/OrderLookup above: this package never touches a
// database itself, and every caller of compensating.go's
// ExecuteCompensatingPayment may pass nil here (exactly like RecordStore's
// own nil convention) to keep prior in-memory-only behaviour, at the cost of
// the audit trail not surviving a restart.
type CompensatingPaymentAuditStore interface {
	// PutCompensatingPaymentAudit inserts one row. Implementations assign
	// a.ID when it is empty (see CreatePayout's identical convention in
	// internal/store) rather than requiring every caller to generate one.
	PutCompensatingPaymentAudit(ctx context.Context, a CompensatingPaymentAudit) error
	// ListCompensatingPaymentAudits returns every decision recorded against
	// originalReference, in no particular order — the full history of every
	// address a customer supplied for this order's refund, refused or not.
	ListCompensatingPaymentAudits(ctx context.Context, originalReference string) ([]CompensatingPaymentAudit, error)
}
