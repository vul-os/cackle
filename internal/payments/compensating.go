//go:build patala

// This file is the compensating-payment (refund) flow: how Cackle pays a
// customer BACK on a rail that cannot reverse a settled payment. See the
// sibling patala repo's docs/compensating-payments.md, which this mirrors
// step-for-step (its "Go" section, §8, is close to line-for-line what
// ExecuteCompensatingPayment does), and patala-core/src/destination.rs's
// module doc comment for the underlying design.
//
// # Why this needs patala, and why it is gated behind `-tags patala`
//
// A rail-agnostic "did this address check out" question can only be
// answered by asking a rail — patala_core::PaymentRail::validate_destination
// — and that is only reachable from Go through the cgo binding this repo's
// patala.go already gates behind `-tags patala`. There is no non-cgo path to
// a real ValidateDestination call, so this file inherits that same gate
// rather than inventing a second one: the plain `go build`/`go test`
// (CGO_ENABLED=0) default stays completely unaffected, exactly as
// patala.go's own doc comment states.
//
// # Four rules from the design this file exists to enforce, not to soften
//
//  1. The customer supplies the destination. This package never infers one
//     from the original payment, a wallet pool, or anywhere else — every
//     function below takes it as an explicit string.
//  2. patala_core::DestinationVerdict has five statuses and no
//     is_valid()/is_safe() — see destination.rs. This file never collapses
//     them into a bool. RefundDestinationCheck embeds the verdict AS-IS and
//     adds only ADDITIVE fields (SameAsSenderAddress, SenderAddressUnknown);
//     nothing here re-derives IsRefusal from Status, matching why
//     DestinationVerdict crosses the FFI boundary with IsRefusal as a field
//     computed on the Rust side, never as a switch a Go caller writes.
//  3. Every verdict requires human confirmation — patala sets
//     HumanMustConfirm true unconditionally, and ExecuteCompensatingPayment
//     asserts it defensively (not merely trusts it) before ever charging.
//     There is no "trusted caller" bypass, no batch auto-approve, and no
//     code path in this file that reaches patala.PatalaRail.Charge without
//     an explicit, populated HumanApproval.
//  4. patala cannot tell whether an address belongs to an exchange, and
//     therefore cannot tell whether it is the SAME address that paid the
//     merchant in the first place (validate_destination takes one address
//     in isolation — see destination.rs's "purity contract" — with no
//     notion of "the payment this is compensating for" at all). Refusing a
//     destination that equals the original payment's known sender address
//     is entirely this file's own guard, layered on top of patala's, never
//     inside it.
package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	patala "github.com/vul-os/patala/patala-go/bindings/patala"
)

// Sentinel errors for the compensating-payment flow. Callers should match
// with errors.Is.
var (
	// ErrRefundDestinationRefused means either patala's own offline check
	// established a defect (Malformed/WrongNetwork/NotAWallet), or the
	// destination is the same address the original payment came from.
	// Neither is overridable — see RefundDestinationCheck.Refused.
	ErrRefundDestinationRefused = errors.New("payments: compensating: destination refused")
	// ErrRefundNotApproved means ExecuteCompensatingPayment was called
	// without a valid HumanApproval. There is no default, no "trusted
	// service account", and no way to skip this — see HumanApproval.
	ErrRefundNotApproved = errors.New("payments: compensating: no human approval was supplied")
	// ErrRefundRailMismatch means the RefundDestinationCheck passed to
	// ExecuteCompensatingPayment was formed against a different rail than
	// the one about to be charged — refused rather than trusted, since a
	// verdict is only ever an opinion about the network ITS rail pays on
	// (see destination.rs's DestinationVerdict.rail_id doc comment).
	ErrRefundRailMismatch = errors.New("payments: compensating: destination check was formed against a different rail")
	// ErrRefundBadRequest covers structurally invalid
	// CompensatingPaymentRequest fields (empty references, non-positive
	// amount, a payout reference that collides with the original).
	ErrRefundBadRequest = errors.New("payments: compensating: invalid request")
)

// RefundDestinationCheck is what ValidateRefundDestination reports about one
// customer-supplied destination: everything patala's own
// validate_destination established, PLUS the one guard patala cannot
// perform itself (see this file's doc comment, point 4).
//
// Every field patala's DestinationVerdict carries is embedded here
// UNCHANGED — Status stays a five-variant enum, IsRefusal stays data computed
// on the Rust side, HumanMustConfirm stays unconditionally true. This type
// adds fields; it never removes information or folds two questions into one
// answer.
type RefundDestinationCheck struct {
	patala.DestinationVerdict
	// Destination is the EXACT string this check was formed for. patala's
	// own DestinationVerdict carries no destination of its own (a verdict is
	// about a string passed in, not a stored copy — see destination.rs) so
	// this file captures it here: ExecuteCompensatingPayment refuses unless
	// the check it is given and the address it is about to charge to are
	// the SAME string, which would otherwise be a silent way to validate one
	// address and pay out to another.
	Destination string
	// SameAsSenderAddress is true when Destination case-insensitively
	// equals the original payment's known sender address. patala cannot
	// compute this — validate_destination takes one address in total
	// isolation, with no notion of "the payment this compensates for" (see
	// destination.rs's purity contract) — so it exists entirely on this
	// side, additive to (never folded into) IsRefusal.
	SameAsSenderAddress bool
	// SenderAddressUnknown is true when the caller could not supply an
	// original sender address at all. This is NOT the same as
	// SameAsSenderAddress == false: an unknown sender means the guard could
	// not run, not that it ran and passed. A UI should treat this with EXTRA
	// caution, never as reassurance.
	SenderAddressUnknown bool
}

// Refused reports whether ANY guard in this check forbids paying out to
// Destination: patala's own IsRefusal, OR cackle's own sender-address guard.
// It is the only combinator this file provides, and it is a pure OR — no
// guard here can be overridden by another one passing. This is the ONE
// question ExecuteCompensatingPayment asks before it will even look at an
// approval.
func (c RefundDestinationCheck) Refused() bool {
	return c.IsRefusal || c.SameAsSenderAddress
}

// destinationStatusName renders a patala.DestinationStatus as one of exactly
// five stable strings, kept to this one function so
// CompensatingPaymentAudit.Status can never end up with two different
// spellings of the same status from two different call sites. An
// unrecognised discriminant (a binding regression, or a status patala added
// that this file has not been updated for) renders LOUDLY rather than
// silently — see destinationStatusNameTestCoverage in the test file for the
// exhaustiveness proof.
func destinationStatusName(s patala.DestinationStatus) string {
	switch s {
	case patala.DestinationStatusMalformed:
		return "Malformed"
	case patala.DestinationStatusWrongNetwork:
		return "WrongNetwork"
	case patala.DestinationStatusNotAWallet:
		return "NotAWallet"
	case patala.DestinationStatusStructurallyValid:
		return "StructurallyValid"
	case patala.DestinationStatusUnknown:
		return "Unknown"
	default:
		return fmt.Sprintf("UnrecognisedPatalaStatus(%d)", s)
	}
}

// ValidateRefundDestination is step 3 (patala's offline check) plus this
// file's own sender-address guard, in one call. destination is the
// CUSTOMER-supplied address — never infer this from senderAddress, an
// address pool, or anywhere else. senderAddress is the ORIGINAL payment's
// known sender address if the caller can determine one (e.g. the "from"
// field of the on-chain transfer that funded the order); pass "" when it is
// not known — that is what SenderAddressUnknown surfaces, and it is never
// treated as "different from destination, therefore safe".
//
// This function is pure and offline, inheriting patala's own contract for
// validate_destination: no network, no clock, no filesystem. It can be
// called on every keystroke of an address field.
func ValidateRefundDestination(rail *patala.PatalaRail, senderAddress, destination string) (RefundDestinationCheck, error) {
	if rail == nil {
		return RefundDestinationCheck{}, errors.New("payments: compensating: a rail is required")
	}
	dest := strings.TrimSpace(destination)

	verdict := rail.ValidateDestination(dest)
	check := RefundDestinationCheck{
		DestinationVerdict: verdict,
		Destination:        dest,
	}

	sender := strings.TrimSpace(senderAddress)
	switch {
	case sender == "":
		check.SenderAddressUnknown = true
	case dest != "" && strings.EqualFold(sender, dest):
		// EqualFold, not an exact byte match: an EVM address may arrive
		// checksum-cased in one place and lowercased in another for the
		// SAME address, and folding case only ever makes this guard MORE
		// likely to catch a real match, never less — see the file's own
		// test for a case-differing pair that must still be caught.
		check.SameAsSenderAddress = true
	}
	return check, nil
}

// HumanApproval is the record of the one step in this flow that cannot be
// automated: a real person reading RefundDestinationCheck's Reason and
// ExchangeDepositCaveat (and, had the check been refused, never reaching
// this step at all) and choosing to proceed. There is no constructor that
// produces a "pre-approved" or default value that passes validation — every
// field must be explicitly populated by a caller who actually captured a
// human decision.
type HumanApproval struct {
	// ApprovedBy identifies the human who confirmed this payout — an
	// operator/admin identity, never empty, never a service account or
	// "system".
	ApprovedBy string
	// ApprovedAt is when they confirmed.
	ApprovedAt time.Time
}

// valid reports whether a is a genuine, populated approval. Both fields are
// required: an approval with no ApprovedBy is indistinguishable from no
// approval at all, and one with a zero ApprovedAt cannot be placed in the
// audit trail's timeline.
func (a HumanApproval) valid() bool {
	return strings.TrimSpace(a.ApprovedBy) != "" && !a.ApprovedAt.IsZero()
}

// CompensatingPaymentRequest is what a caller assembles to request a
// compensating payment — see docs/compensating-payments.md §2 for why this
// is a Charge and not a refund() call.
type CompensatingPaymentRequest struct {
	// OriginalReference is the reference of the payment being compensated
	// for. Required.
	OriginalReference string
	// PayoutReference is the compensating payment's OWN idempotency key. If
	// empty, defaults to OriginalReference + "-payout" (the convention
	// docs/compensating-payments.md §8 uses). It must never equal
	// OriginalReference — see ExecuteCompensatingPayment.
	PayoutReference string
	// AmountMinor/Currency are the amount being paid out, in the currency's
	// own minor units (see internal/money) — not necessarily the full
	// original order total, since a partial compensating payment is a valid
	// use case this type does not forbid.
	AmountMinor int64
	Currency    string
}

// ExecuteCompensatingPayment is the ONLY function in this package that moves
// money for a refund — "step 6" of docs/compensating-payments.md: an
// ordinary Charge to check.Destination, in the opposite direction of the
// original payment, with its own reference. It refuses unless ALL of the
// following hold, with no override for any of them:
//
//   - check was formed against THIS rail (ErrRefundRailMismatch otherwise) —
//     a verdict is only ever an opinion about the network its own rail pays
//     on.
//   - check.Refused() is false: neither patala's own refusal NOR cackle's
//     sender-address guard may be bypassed by this function or by a caller.
//   - check.HumanMustConfirm is true. This is asserted, not trusted: patala
//     sets it unconditionally today, but if a future binding regression
//     ever produced false here, this is the one call site about to move
//     money and it must not proceed on blind faith in an upstream contract.
//   - approval.valid(): a real ApprovedBy identity and a real ApprovedAt
//     time. There is no default HumanApproval, no "trusted caller" bypass,
//     and no batch/auto-approve path anywhere in this file.
//   - req is well-formed: non-empty OriginalReference, positive AmountMinor,
//     non-empty Currency, and a PayoutReference that is not simply the
//     original's own reference (a compensating payment needs its own
//     idempotency key — see docs/compensating-payments.md's reversal-vs-
//     compensating-payment table).
//
// audit may be nil (the same convention as RecordStore elsewhere in this
// package); when non-nil, ONE row is persisted unconditionally — whether
// this call refuses or succeeds — so "customer supplied address X, refused
// because Y" is exactly as much a durable audit fact as an approved payout.
func ExecuteCompensatingPayment(
	ctx context.Context,
	rail *patala.PatalaRail,
	req CompensatingPaymentRequest,
	check RefundDestinationCheck,
	approval HumanApproval,
	audit CompensatingPaymentAuditStore,
) (patala.Receipt, error) {
	if rail == nil {
		return patala.Receipt{}, errors.New("payments: compensating: a rail is required")
	}

	record := func(executed bool) {
		if audit == nil {
			return
		}
		a := CompensatingPaymentAudit{
			OriginalReference:   req.OriginalReference,
			RailID:              rail.Id(),
			Destination:         check.Destination,
			SenderAddressKnown:  !check.SenderAddressUnknown,
			Status:              destinationStatusName(check.Status),
			Reason:              check.Reason,
			IsRefusal:           check.IsRefusal,
			SameAsSenderAddress: check.SameAsSenderAddress,
			Refused:             check.Refused(),
			AmountMinor:         req.AmountMinor,
			Currency:            req.Currency,
			CreatedAt:           time.Now(),
		}
		if approval.valid() {
			a.ApprovedBy = approval.ApprovedBy
			a.ApprovedAt = approval.ApprovedAt
		}
		if executed {
			a.PayoutReference = payoutReferenceOrDefault(req)
			a.Executed = true
		}
		// Best-effort: a failure to persist the audit row must not be
		// reported as though the payment guard itself failed, and must
		// never retroactively "un-execute" a payment that already
		// happened. Errors are swallowed here deliberately — this is
		// audit logging, not the guard.
		_ = audit.PutCompensatingPaymentAudit(ctx, a)
	}

	// Guard 1: the check must be about the SAME rail we are about to
	// charge. A verdict formed against one rail says nothing about another
	// (see destination.rs's rail_id doc comment) -- trusting it across
	// rails would be exactly the class of substitution bug this function
	// exists to prevent.
	if check.RailId != rail.Id() {
		record(false)
		return patala.Receipt{}, fmt.Errorf("%w: check formed against %q, executing against %q",
			ErrRefundRailMismatch, check.RailId, rail.Id())
	}

	// Guard 2: patala's own refusal, or cackle's own sender-address guard.
	// Neither is overridable by anything below this line.
	if check.Refused() {
		record(false)
		reason := check.Reason
		if check.SameAsSenderAddress {
			reason = "destination is the same address the original payment came from -- " + reason
		}
		return patala.Receipt{}, fmt.Errorf("%w: %s", ErrRefundDestinationRefused, reason)
	}

	// Guard 3: defensive, not trusting. See the function doc comment.
	if !check.HumanMustConfirm {
		record(false)
		return patala.Receipt{}, errors.New("payments: compensating: check.HumanMustConfirm is false -- refusing to proceed without confirmation (this should be structurally impossible; see destination.rs)")
	}

	// Guard 4: an explicit, populated human approval. No default value of
	// HumanApproval satisfies this.
	if !approval.valid() {
		record(false)
		return patala.Receipt{}, ErrRefundNotApproved
	}

	if strings.TrimSpace(req.OriginalReference) == "" {
		record(false)
		return patala.Receipt{}, fmt.Errorf("%w: OriginalReference is required", ErrRefundBadRequest)
	}
	if req.AmountMinor <= 0 {
		record(false)
		return patala.Receipt{}, fmt.Errorf("%w: AmountMinor must be positive", ErrRefundBadRequest)
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		record(false)
		return patala.Receipt{}, fmt.Errorf("%w: Currency is required", ErrRefundBadRequest)
	}
	if strings.TrimSpace(check.Destination) == "" {
		record(false)
		return patala.Receipt{}, fmt.Errorf("%w: check.Destination is empty", ErrRefundBadRequest)
	}

	payoutRef := payoutReferenceOrDefault(req)
	if payoutRef == strings.TrimSpace(req.OriginalReference) {
		record(false)
		return patala.Receipt{}, fmt.Errorf("%w: PayoutReference must not equal OriginalReference -- a compensating payment needs its own idempotency key", ErrRefundBadRequest)
	}

	receipt, err := rail.Charge(patala.PayRequest{
		AmountMinor: uint64(req.AmountMinor),
		Currency:    currency,
		Destination: check.Destination,
		Reference:   payoutRef,
	})
	if err != nil {
		record(false)
		return patala.Receipt{}, fmt.Errorf("payments: compensating: charge: %w", err)
	}

	record(true)
	return receipt, nil
}

// payoutReferenceOrDefault applies CompensatingPaymentRequest.PayoutReference's
// documented default (OriginalReference + "-payout") — one place, so the
// audit row and the actual Charge reference can never disagree.
func payoutReferenceOrDefault(req CompensatingPaymentRequest) string {
	if ref := strings.TrimSpace(req.PayoutReference); ref != "" {
		return ref
	}
	return strings.TrimSpace(req.OriginalReference) + "-payout"
}
