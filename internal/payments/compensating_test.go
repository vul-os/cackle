//go:build patala

package payments

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	patala "github.com/vul-os/patala/patala-go/bindings/patala"
)

// newCompensatingMockRail builds the offline stand-in used throughout this
// file: patala's own MockRail, whose synthetic "<network>:<kind>:<label>"
// address grammar (documented in patala-go/bindingtest/destination_test.go)
// lets every DestinationStatus be produced with no real chain reachable.
// NOT a real chain address format -- a real rail decodes its own alphabet,
// length and checksum instead.
func newCompensatingMockRail(t *testing.T) *patala.PatalaRail {
	t.Helper()
	return patala.PatalaRailNewMock("mock", patala.RailClassNonCustodialFinal, []string{"USDC"}, 0, false)
}

// newCompensatingOpaqueRail builds the offline stand-in for a rail that
// cannot check a destination at all -- the shape of every fiat rail, whose
// destination is an opaque processor-side token. It is the only way to
// reach DestinationStatusUnknown without a feature-gated real rail.
func newCompensatingOpaqueRail(t *testing.T) *patala.PatalaRail {
	t.Helper()
	return patala.PatalaRailNewMockWithoutDestinationChecks("opaque", patala.RailClassCustodialReversible, []string{"USD"}, 0, false)
}

func approvedNow(by string) HumanApproval {
	return HumanApproval{ApprovedBy: by, ApprovedAt: time.Now()}
}

// fakeAuditStore is an in-memory CompensatingPaymentAuditStore for tests --
// the same shape as fakeRecordStore in recordstore_test.go.
type fakeAuditStore struct {
	rows []CompensatingPaymentAudit
}

func (f *fakeAuditStore) PutCompensatingPaymentAudit(ctx context.Context, a CompensatingPaymentAudit) error {
	if a.ID == "" {
		a.ID = time.Now().Format("20060102T150405.000000000")
	}
	f.rows = append(f.rows, a)
	return nil
}

func (f *fakeAuditStore) ListCompensatingPaymentAudits(ctx context.Context, originalReference string) ([]CompensatingPaymentAudit, error) {
	var out []CompensatingPaymentAudit
	for _, r := range f.rows {
		if r.OriginalReference == originalReference {
			out = append(out, r)
		}
	}
	return out, nil
}

// ── ValidateRefundDestination: coverage of every DestinationStatus ─────────

// TestValidateRefundDestination_AllFiveStatusesReachable proves
// ValidateRefundDestination is a thin, non-lossy wrapper: every one of
// patala's five DestinationStatus variants must still be individually
// producible through it, and the exact same discipline
// patala-go/bindingtest/destination_test.go itself uses (an asserted
// coverage count, not "some of these look right") is applied here too, one
// layer up.
func TestValidateRefundDestination_AllFiveStatusesReachable(t *testing.T) {
	mock := newCompensatingMockRail(t)
	opaque := newCompensatingOpaqueRail(t)

	cases := []struct {
		name        string
		rail        *patala.PatalaRail
		destination string
		want        patala.DestinationStatus
		wantRefusal bool
	}{
		{"plain wallet", mock, "mock:wallet:alice", patala.DestinationStatusStructurallyValid, false},
		{"program account", mock, "mock:program:vault", patala.DestinationStatusNotAWallet, true},
		{"different network", mock, "stellar:wallet:alice", patala.DestinationStatusWrongNetwork, true},
		{"junk", mock, "definitely-not-an-address", patala.DestinationStatusMalformed, true},
		{"empty string", mock, "", patala.DestinationStatusMalformed, true},
		{"opaque rail cannot check", opaque, "cus_opaque_token", patala.DestinationStatusUnknown, false},
	}

	seen := map[patala.DestinationStatus]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No sender address supplied -- isolates patala's own check from
			// this file's additive guard for this table.
			check, err := ValidateRefundDestination(tc.rail, "", tc.destination)
			if err != nil {
				t.Fatalf("ValidateRefundDestination: %v", err)
			}
			if check.Status != tc.want {
				t.Fatalf("Status = %v, want %v", check.Status, tc.want)
			}
			if check.IsRefusal != tc.wantRefusal {
				t.Errorf("IsRefusal = %v, want %v", check.IsRefusal, tc.wantRefusal)
			}
			if check.Refused() != tc.wantRefusal {
				t.Errorf("Refused() = %v, want %v (no sender address supplied, so Refused must equal IsRefusal here)", check.Refused(), tc.wantRefusal)
			}
			if !check.SenderAddressUnknown {
				t.Error("SenderAddressUnknown = false, want true: no sender address was supplied")
			}
			if check.SameAsSenderAddress {
				t.Error("SameAsSenderAddress = true with no sender address supplied at all")
			}
			seen[check.Status] = true
		})
	}

	all := []patala.DestinationStatus{
		patala.DestinationStatusMalformed,
		patala.DestinationStatusWrongNetwork,
		patala.DestinationStatusNotAWallet,
		patala.DestinationStatusStructurallyValid,
		patala.DestinationStatusUnknown,
	}
	for _, s := range all {
		if !seen[s] {
			t.Errorf("DestinationStatus %v was never produced -- unreachable through ValidateRefundDestination", s)
		}
	}
	if len(seen) != len(all) {
		t.Fatalf("saw %d distinct statuses through ValidateRefundDestination, want %d -- variants are collapsing somewhere in this wrapper", len(seen), len(all))
	}
}

// TestValidateRefundDestination_HumanMustConfirmIsAlwaysTrue is the single
// most important property carried over from patala: no status -- not even
// StructurallyValid -- may waive the human confirmation step.
func TestValidateRefundDestination_HumanMustConfirmIsAlwaysTrue(t *testing.T) {
	mock := newCompensatingMockRail(t)
	opaque := newCompensatingOpaqueRail(t)
	for _, dest := range []struct {
		rail *patala.PatalaRail
		addr string
	}{
		{mock, "mock:wallet:alice"},
		{mock, "mock:program:vault"},
		{mock, "stellar:wallet:alice"},
		{mock, "junk"},
		{mock, ""},
		{opaque, "cus_opaque_token"},
	} {
		check, err := ValidateRefundDestination(dest.rail, "", dest.addr)
		if err != nil {
			t.Fatalf("ValidateRefundDestination(%q): %v", dest.addr, err)
		}
		if !check.HumanMustConfirm {
			t.Errorf("HumanMustConfirm = false for status %v; no status may waive it", check.Status)
		}
		if check.ExchangeDepositCaveat == "" || !strings.Contains(check.ExchangeDepositCaveat, "exchange") {
			t.Errorf("ExchangeDepositCaveat missing or does not mention exchanges for status %v", check.Status)
		}
	}
}

// ── The sender-address guard: cackle's own, not patala's ──────────────────

// TestValidateRefundDestination_RefusesTheSenderAddress is the central
// negative-space test this whole file exists for: a destination that is
// PERFECTLY structurally valid on patala's own terms (mock:wallet:alice
// parses as a plain wallet -- StructurallyValid, IsRefusal == false) must
// still be refused when it is the same address the original payment came
// from. This test would FAIL if the sender-address guard were removed or
// disabled: check.Refused() would read false, and IsRefusal alone (patala's
// own signal) already asserts false here by construction, so nothing except
// SameAsSenderAddress can make Refused() true in this case.
func TestValidateRefundDestination_RefusesTheSenderAddress(t *testing.T) {
	mock := newCompensatingMockRail(t)
	sender := "mock:wallet:alice"

	check, err := ValidateRefundDestination(mock, sender, sender)
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if check.IsRefusal {
		t.Fatal("test invariant broken: mock:wallet:alice must be patala-side StructurallyValid, not a refusal -- otherwise this test cannot isolate the sender-address guard")
	}
	if !check.SameAsSenderAddress {
		t.Fatal("SameAsSenderAddress = false for destination == sender address; the guard did not fire")
	}
	if !check.Refused() {
		t.Fatal("Refused() = false when destination equals the original payment's sender address -- this is exactly the refund-to-sender hazard the flow exists to prevent")
	}

	// And ExecuteCompensatingPayment must refuse to move money here, with
	// no override available -- even with a fully-populated human approval.
	_, err = ExecuteCompensatingPayment(context.Background(), mock,
		CompensatingPaymentRequest{OriginalReference: "ord-1", AmountMinor: 500, Currency: "USDC"},
		check, approvedNow("alice-the-operator"), nil)
	if !errors.Is(err, ErrRefundDestinationRefused) {
		t.Fatalf("ExecuteCompensatingPayment error = %v, want ErrRefundDestinationRefused", err)
	}
}

// TestValidateRefundDestination_SenderAddressGuardIsCaseInsensitive proves
// the guard does not use a naive exact-byte comparison that a differently-
// cased (but identical) address could slip past.
func TestValidateRefundDestination_SenderAddressGuardIsCaseInsensitive(t *testing.T) {
	mock := newCompensatingMockRail(t)
	check, err := ValidateRefundDestination(mock, "mock:wallet:ALICE", "MOCK:WALLET:alice")
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if !check.SameAsSenderAddress {
		t.Fatal("SameAsSenderAddress = false for a case-differing match; the guard is too strict and would miss a real match")
	}
}

// TestValidateRefundDestination_DifferentAddressIsNotFlagged is the mirror
// of the refusal test: a genuinely different destination must NOT be
// flagged as the sender address, or the guard would be so broad it refuses
// every legitimate payout too.
func TestValidateRefundDestination_DifferentAddressIsNotFlagged(t *testing.T) {
	mock := newCompensatingMockRail(t)
	check, err := ValidateRefundDestination(mock, "mock:wallet:alice", "mock:wallet:bob")
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if check.SameAsSenderAddress {
		t.Fatal("SameAsSenderAddress = true for two genuinely different addresses")
	}
	if check.Refused() {
		t.Fatalf("Refused() = true for a StructurallyValid destination that differs from the sender: %+v", check)
	}
}

// TestValidateRefundDestination_UnknownSenderIsNeverTreatedAsSafe: an empty
// sender address must surface as SenderAddressUnknown, and must NOT read as
// "confirmed different, therefore fine".
func TestValidateRefundDestination_UnknownSenderIsNeverTreatedAsSafe(t *testing.T) {
	mock := newCompensatingMockRail(t)
	check, err := ValidateRefundDestination(mock, "", "mock:wallet:alice")
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if !check.SenderAddressUnknown {
		t.Fatal("SenderAddressUnknown = false with an empty sender address supplied")
	}
	if check.SameAsSenderAddress {
		t.Fatal("SameAsSenderAddress must never be true when the sender address is unknown")
	}
}

// ── ExecuteCompensatingPayment: every refusal path, and the guards fail closed ──

func validStructurallyValidCheck(t *testing.T, rail *patala.PatalaRail) RefundDestinationCheck {
	t.Helper()
	check, err := ValidateRefundDestination(rail, "mock:wallet:sender-only-known-here", "mock:wallet:customer")
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if check.Refused() {
		t.Fatalf("test invariant broken: expected an unrefused check, got %+v", check)
	}
	return check
}

// TestExecuteCompensatingPayment_RefusesAMalformedDestination proves a
// patala-side refusal (not cackle's own sender-address guard) also stops
// the flow before Charge.
func TestExecuteCompensatingPayment_RefusesAMalformedDestination(t *testing.T) {
	mock := newCompensatingMockRail(t)
	check, err := ValidateRefundDestination(mock, "", "not-an-address-at-all")
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if !check.IsRefusal {
		t.Fatal("test invariant broken: expected patala to refuse this destination")
	}

	_, err = ExecuteCompensatingPayment(context.Background(), mock,
		CompensatingPaymentRequest{OriginalReference: "ord-2", AmountMinor: 100, Currency: "USDC"},
		check, approvedNow("op"), nil)
	if !errors.Is(err, ErrRefundDestinationRefused) {
		t.Fatalf("error = %v, want ErrRefundDestinationRefused", err)
	}
}

// TestExecuteCompensatingPayment_RefusesWithoutApproval is the second
// central negative-space test: a destination check that patala and cackle
// both agree is clean must STILL refuse to pay out with a zero-value
// HumanApproval. This test would FAIL if the approval gate were ever made
// optional, defaulted to true, or bypassable by any "trusted caller" path.
func TestExecuteCompensatingPayment_RefusesWithoutApproval(t *testing.T) {
	mock := newCompensatingMockRail(t)
	check := validStructurallyValidCheck(t, mock)

	_, err := ExecuteCompensatingPayment(context.Background(), mock,
		CompensatingPaymentRequest{OriginalReference: "ord-3", AmountMinor: 100, Currency: "USDC"},
		check, HumanApproval{}, nil)
	if !errors.Is(err, ErrRefundNotApproved) {
		t.Fatalf("error = %v, want ErrRefundNotApproved", err)
	}
}

// TestExecuteCompensatingPayment_RefusesAPartialApproval: an approval
// missing EITHER field is not an approval at all.
func TestExecuteCompensatingPayment_RefusesAPartialApproval(t *testing.T) {
	mock := newCompensatingMockRail(t)

	cases := []struct {
		name     string
		approval HumanApproval
	}{
		{"no ApprovedBy", HumanApproval{ApprovedAt: time.Now()}},
		{"no ApprovedAt", HumanApproval{ApprovedBy: "op"}},
		{"whitespace ApprovedBy", HumanApproval{ApprovedBy: "   ", ApprovedAt: time.Now()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := validStructurallyValidCheck(t, mock)
			_, err := ExecuteCompensatingPayment(context.Background(), mock,
				CompensatingPaymentRequest{OriginalReference: "ord-partial-" + tc.name, AmountMinor: 100, Currency: "USDC"},
				check, tc.approval, nil)
			if !errors.Is(err, ErrRefundNotApproved) {
				t.Errorf("error = %v, want ErrRefundNotApproved", err)
			}
		})
	}
}

// TestExecuteCompensatingPayment_RefusesAnUnknownVerdictWithoutApproval:
// Unknown (a fiat/opaque rail that cannot check at all) must go through
// EXACTLY the same approval gate as every other status -- it must never be
// auto-approved because "there was nothing to refuse".
func TestExecuteCompensatingPayment_RefusesAnUnknownVerdictWithoutApproval(t *testing.T) {
	opaque := newCompensatingOpaqueRail(t)
	check, err := ValidateRefundDestination(opaque, "", "cus_opaque_token")
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if check.Status != patala.DestinationStatusUnknown {
		t.Fatalf("test invariant broken: want Unknown, got %v", check.Status)
	}
	if check.Refused() {
		t.Fatal("test invariant broken: Unknown must not be a refusal")
	}

	_, err = ExecuteCompensatingPayment(context.Background(), opaque,
		CompensatingPaymentRequest{OriginalReference: "ord-unknown", AmountMinor: 100, Currency: "USD"},
		check, HumanApproval{}, nil)
	if !errors.Is(err, ErrRefundNotApproved) {
		t.Fatalf("error = %v, want ErrRefundNotApproved -- an Unknown verdict must still require a human, not pass through", err)
	}

	// With a real approval it succeeds -- Unknown is not a refusal, it is
	// "nothing could be established", which the human is trusted to judge.
	approvedCheck := check
	receipt, err := ExecuteCompensatingPayment(context.Background(), opaque,
		CompensatingPaymentRequest{OriginalReference: "ord-unknown", AmountMinor: 100, Currency: "USD"},
		approvedCheck, approvedNow("op"), nil)
	if err != nil {
		t.Fatalf("ExecuteCompensatingPayment with a real approval on an Unknown (non-refused) verdict: %v", err)
	}
	if receipt.Reference != "ord-unknown-payout" {
		t.Errorf("Receipt.Reference = %q, want %q", receipt.Reference, "ord-unknown-payout")
	}
}

// TestExecuteCompensatingPayment_RefusesARailMismatch: a check formed
// against one rail must not authorise a Charge on a different rail.
func TestExecuteCompensatingPayment_RefusesARailMismatch(t *testing.T) {
	mockA := patala.PatalaRailNewMock("rail-a", patala.RailClassNonCustodialFinal, []string{"USDC"}, 0, false)
	mockB := patala.PatalaRailNewMock("rail-b", patala.RailClassNonCustodialFinal, []string{"USDC"}, 0, false)

	check, err := ValidateRefundDestination(mockA, "rail-a:wallet:sender", "rail-a:wallet:customer")
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}
	if check.Refused() {
		t.Fatalf("test invariant broken: expected an unrefused check against rail-a, got %+v", check)
	}
	_, err = ExecuteCompensatingPayment(context.Background(), mockB,
		CompensatingPaymentRequest{OriginalReference: "ord-mismatch", AmountMinor: 100, Currency: "USDC"},
		check, approvedNow("op"), nil)
	if !errors.Is(err, ErrRefundRailMismatch) {
		t.Fatalf("error = %v, want ErrRefundRailMismatch", err)
	}
}

// TestExecuteCompensatingPayment_RefusesAPayoutReferenceEqualToOriginal: a
// compensating payment must carry its OWN idempotency key.
func TestExecuteCompensatingPayment_RefusesAPayoutReferenceEqualToOriginal(t *testing.T) {
	mock := newCompensatingMockRail(t)
	check := validStructurallyValidCheck(t, mock)

	_, err := ExecuteCompensatingPayment(context.Background(), mock,
		CompensatingPaymentRequest{
			OriginalReference: "ord-same-ref",
			PayoutReference:   "ord-same-ref", // deliberately equal
			AmountMinor:       100, Currency: "USDC",
		},
		check, approvedNow("op"), nil)
	if !errors.Is(err, ErrRefundBadRequest) {
		t.Fatalf("error = %v, want ErrRefundBadRequest", err)
	}
}

// TestExecuteCompensatingPayment_RefusesBadRequestFields covers the
// remaining structural refusals as one table, with an asserted count so a
// silently-dropped case is itself a test failure.
func TestExecuteCompensatingPayment_RefusesBadRequestFields(t *testing.T) {
	mock := newCompensatingMockRail(t)

	const expectedCases = 3
	cases := []struct {
		name string
		req  CompensatingPaymentRequest
	}{
		{"empty original reference", CompensatingPaymentRequest{AmountMinor: 100, Currency: "USDC"}},
		{"zero amount", CompensatingPaymentRequest{OriginalReference: "ord-zero", Currency: "USDC"}},
		{"negative amount", CompensatingPaymentRequest{OriginalReference: "ord-neg", AmountMinor: -1, Currency: "USDC"}},
	}
	if len(cases) != expectedCases {
		t.Fatalf("test table has %d cases, want %d -- update expectedCases if this is deliberate", len(cases), expectedCases)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := validStructurallyValidCheck(t, mock)
			_, err := ExecuteCompensatingPayment(context.Background(), mock, tc.req, check, approvedNow("op"), nil)
			if !errors.Is(err, ErrRefundBadRequest) {
				t.Errorf("error = %v, want ErrRefundBadRequest", err)
			}
		})
	}
}

// TestExecuteCompensatingPayment_HappyPathProducesItsOwnReceipt is the
// positive control: with a clean check, a real approval, and well-formed
// fields, the payout executes, gets its OWN reference (never the original's),
// and verifies on its own -- while the original payment (simulated by a
// separate Charge on the same mock rail) stays untouched.
func TestExecuteCompensatingPayment_HappyPathProducesItsOwnReceipt(t *testing.T) {
	mock := newCompensatingMockRail(t)

	original, err := mock.Charge(patala.PayRequest{
		AmountMinor: 2500, Currency: "USDC", Destination: "mock:wallet:merchant", Reference: "ord-happy",
	})
	if err != nil {
		t.Fatalf("original Charge: %v", err)
	}

	check := validStructurallyValidCheck(t, mock)
	audit := &fakeAuditStore{}
	payout, err := ExecuteCompensatingPayment(context.Background(), mock,
		CompensatingPaymentRequest{OriginalReference: "ord-happy", AmountMinor: int64(original.AmountMinor), Currency: original.Currency},
		check, approvedNow("alice-the-operator"), audit)
	if err != nil {
		t.Fatalf("ExecuteCompensatingPayment: %v", err)
	}
	if payout.Reference == original.Reference {
		t.Fatal("payout reused the original reference; it must have its own idempotency key")
	}
	if payout.Reference != "ord-happy-payout" {
		t.Errorf("Reference = %q, want %q", payout.Reference, "ord-happy-payout")
	}

	valid, err := mock.Verify(payout)
	if err != nil || !valid {
		t.Fatalf("Verify(payout) = %v, %v; want true, nil", valid, err)
	}
	stillValid, err := mock.Verify(original)
	if err != nil || !stillValid {
		t.Fatalf("the original stopped verifying after the payout: %v, %v", stillValid, err)
	}

	if len(audit.rows) != 1 {
		t.Fatalf("audit store has %d rows, want 1", len(audit.rows))
	}
	row := audit.rows[0]
	if !row.Executed || row.PayoutReference != "ord-happy-payout" || row.ApprovedBy != "alice-the-operator" {
		t.Errorf("audit row = %+v, want Executed=true PayoutReference=ord-happy-payout ApprovedBy=alice-the-operator", row)
	}
	if row.Refused {
		t.Errorf("audit row Refused = true for a successful payout")
	}
}

// TestExecuteCompensatingPayment_AuditRecordsARefusalToo proves the audit
// trail captures a refused attempt as loudly as a successful one -- "customer
// tried address X, refused because Y" must be a durable fact, not lost the
// moment the function returns an error.
func TestExecuteCompensatingPayment_AuditRecordsARefusalToo(t *testing.T) {
	mock := newCompensatingMockRail(t)
	sender := "mock:wallet:alice"
	check, err := ValidateRefundDestination(mock, sender, sender)
	if err != nil {
		t.Fatalf("ValidateRefundDestination: %v", err)
	}

	audit := &fakeAuditStore{}
	_, err = ExecuteCompensatingPayment(context.Background(), mock,
		CompensatingPaymentRequest{OriginalReference: "ord-refused-audit", AmountMinor: 100, Currency: "USDC"},
		check, approvedNow("op"), audit)
	if !errors.Is(err, ErrRefundDestinationRefused) {
		t.Fatalf("error = %v, want ErrRefundDestinationRefused", err)
	}

	rows, err := audit.ListCompensatingPaymentAudits(context.Background(), "ord-refused-audit")
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListCompensatingPaymentAudits = %v, %v; want 1 row", rows, err)
	}
	if !rows[0].Refused || !rows[0].SameAsSenderAddress || rows[0].Executed {
		t.Errorf("audit row = %+v, want Refused=true SameAsSenderAddress=true Executed=false", rows[0])
	}
}

// ── destinationStatusName: exhaustiveness ──────────────────────────────────

func TestDestinationStatusName_AllFiveAreDistinctAndNonEmpty(t *testing.T) {
	all := []patala.DestinationStatus{
		patala.DestinationStatusMalformed,
		patala.DestinationStatusWrongNetwork,
		patala.DestinationStatusNotAWallet,
		patala.DestinationStatusStructurallyValid,
		patala.DestinationStatusUnknown,
	}
	seen := map[string]bool{}
	for _, s := range all {
		name := destinationStatusName(s)
		if name == "" {
			t.Errorf("destinationStatusName(%v) is empty", s)
		}
		if seen[name] {
			t.Errorf("destinationStatusName(%v) = %q, collides with another status's name", s, name)
		}
		seen[name] = true
	}
	if len(seen) != len(all) {
		t.Fatalf("saw %d distinct names, want %d", len(seen), len(all))
	}

	// An out-of-range discriminant (a binding regression, or a status this
	// file hasn't been updated for) must render loudly, never silently as
	// one of the five real names or an empty string.
	unrecognised := destinationStatusName(patala.DestinationStatus(99))
	if !strings.Contains(unrecognised, "Unrecognised") {
		t.Errorf("destinationStatusName(99) = %q, want it to say Unrecognised", unrecognised)
	}
}

// TestExecuteCompensatingPayment_RefusesIfHumanMustConfirmIsEverFalse is the
// defensive assert this file's own doc comment promises: patala sets
// HumanMustConfirm true unconditionally today, so the only way to exercise
// this guard is to construct a check by hand rather than through
// ValidateRefundDestination -- exactly what a future binding regression
// would look like from this package's point of view. This proves
// ExecuteCompensatingPayment does not simply trust the upstream contract at
// the one call site that is about to move money.
func TestExecuteCompensatingPayment_RefusesIfHumanMustConfirmIsEverFalse(t *testing.T) {
	mock := newCompensatingMockRail(t)
	check := RefundDestinationCheck{
		DestinationVerdict: patala.DestinationVerdict{
			RailId:                mock.Id(),
			Status:                patala.DestinationStatusStructurallyValid,
			Reason:                "looks fine",
			HumanMustConfirm:      false, // the regression this guard exists for
			ExchangeDepositCaveat: "caveat",
			IsRefusal:             false,
		},
		Destination: "mock:wallet:customer",
	}
	_, err := ExecuteCompensatingPayment(context.Background(), mock,
		CompensatingPaymentRequest{OriginalReference: "ord-hmc", AmountMinor: 100, Currency: "USDC"},
		check, approvedNow("op"), nil)
	if err == nil {
		t.Fatal("ExecuteCompensatingPayment succeeded with HumanMustConfirm == false; want a refusal")
	}
}

// ── HumanApproval.valid ─────────────────────────────────────────────────────

func TestHumanApproval_ZeroValueIsNeverValid(t *testing.T) {
	if (HumanApproval{}).valid() {
		t.Fatal("the zero-value HumanApproval must never be valid -- there is no default approval")
	}
}
