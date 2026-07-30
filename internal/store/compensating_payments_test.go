package store

import (
	"context"
	"testing"
	"time"
)

func TestCompensatingPaymentAudit_PutAssignsIDAndListReturnsIt(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	a := &CompensatingPaymentAudit{
		OriginalReference: "ord-1",
		RailID:            "mock",
		Destination:       "mock:wallet:customer",
		Status:            "StructurallyValid",
		Reason:            "looks fine",
		AmountMinor:       500,
		Currency:          "USDC",
	}
	if err := st.PutCompensatingPaymentAudit(ctx, a); err != nil {
		t.Fatalf("PutCompensatingPaymentAudit: %v", err)
	}
	if a.ID == "" {
		t.Fatal("PutCompensatingPaymentAudit did not assign an ID")
	}

	rows, err := st.ListCompensatingPaymentAudits(ctx, "ord-1")
	if err != nil {
		t.Fatalf("ListCompensatingPaymentAudits: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	got := rows[0]
	if got.ID != a.ID || got.RailID != "mock" || got.Destination != "mock:wallet:customer" ||
		got.Status != "StructurallyValid" || got.AmountMinor != 500 || got.Currency != "USDC" {
		t.Fatalf("round-tripped row = %+v, want it to match what was put", got)
	}
	if got.Refused || got.SameAsSenderAddress || got.SenderAddressKnown || got.Executed {
		t.Fatalf("boolean fields defaulted to true unexpectedly: %+v", got)
	}
	if got.ApprovedAt != nil {
		t.Fatalf("ApprovedAt = %v, want nil (never approved)", got.ApprovedAt)
	}
}

// TestCompensatingPaymentAudit_RefusalAndApprovalBothPersist proves a
// refused row and a later approved-and-executed row for the SAME order are
// both kept — there is no update-in-place that would erase the refusal once
// a later attempt succeeds. This is the durable half of "customer tried
// address X, refused because Y" not being lost.
func TestCompensatingPaymentAudit_RefusalAndApprovalBothPersist(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	refused := &CompensatingPaymentAudit{
		OriginalReference:   "ord-2",
		RailID:              "mock",
		Destination:         "mock:wallet:sender",
		SenderAddressKnown:  true,
		SenderAddress:       "mock:wallet:sender",
		Status:              "StructurallyValid",
		Reason:              "looks fine",
		SameAsSenderAddress: true,
		Refused:             true,
		AmountMinor:         500,
		Currency:            "USDC",
	}
	if err := st.PutCompensatingPaymentAudit(ctx, refused); err != nil {
		t.Fatalf("PutCompensatingPaymentAudit (refused): %v", err)
	}

	approvedAt := time.Now()
	executed := &CompensatingPaymentAudit{
		OriginalReference:  "ord-2",
		PayoutReference:    "ord-2-payout",
		RailID:             "mock",
		Destination:        "mock:wallet:customer",
		SenderAddressKnown: true,
		SenderAddress:      "mock:wallet:sender",
		Status:             "StructurallyValid",
		Reason:             "looks fine",
		Refused:            false,
		ApprovedBy:         "alice-the-operator",
		ApprovedAt:         &approvedAt,
		Executed:           true,
		AmountMinor:        500,
		Currency:           "USDC",
	}
	if err := st.PutCompensatingPaymentAudit(ctx, executed); err != nil {
		t.Fatalf("PutCompensatingPaymentAudit (executed): %v", err)
	}
	if refused.ID == executed.ID {
		t.Fatal("both rows were assigned the same ID")
	}

	rows, err := st.ListCompensatingPaymentAudits(ctx, "ord-2")
	if err != nil {
		t.Fatalf("ListCompensatingPaymentAudits: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 -- the refusal must not have been overwritten by the later approval", len(rows))
	}

	var sawRefused, sawExecuted bool
	for _, r := range rows {
		switch r.ID {
		case refused.ID:
			sawRefused = true
			if !r.Refused || !r.SameAsSenderAddress || r.Executed || r.ApprovedBy != "" {
				t.Errorf("refused row = %+v, want Refused=true SameAsSenderAddress=true Executed=false ApprovedBy=empty", r)
			}
		case executed.ID:
			sawExecuted = true
			if r.Refused || r.ApprovedBy != "alice-the-operator" || !r.Executed || r.PayoutReference != "ord-2-payout" {
				t.Errorf("executed row = %+v, want Refused=false ApprovedBy=alice-the-operator Executed=true PayoutReference=ord-2-payout", r)
			}
			if r.ApprovedAt == nil || r.ApprovedAt.IsZero() {
				t.Error("executed row ApprovedAt is nil/zero")
			}
		}
	}
	if !sawRefused || !sawExecuted {
		t.Fatalf("expected both a refused and an executed row, sawRefused=%v sawExecuted=%v", sawRefused, sawExecuted)
	}
}

func TestCompensatingPaymentAudit_ListScopedToOriginalReference(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	for _, ref := range []string{"ord-a", "ord-a", "ord-b"} {
		if err := st.PutCompensatingPaymentAudit(ctx, &CompensatingPaymentAudit{
			OriginalReference: ref, RailID: "mock", Destination: "mock:wallet:x",
			Status: "StructurallyValid", AmountMinor: 1, Currency: "USDC",
		}); err != nil {
			t.Fatalf("PutCompensatingPaymentAudit: %v", err)
		}
	}

	rowsA, err := st.ListCompensatingPaymentAudits(ctx, "ord-a")
	if err != nil || len(rowsA) != 2 {
		t.Fatalf("ord-a: got %d rows, err=%v, want 2 rows", len(rowsA), err)
	}
	rowsB, err := st.ListCompensatingPaymentAudits(ctx, "ord-b")
	if err != nil || len(rowsB) != 1 {
		t.Fatalf("ord-b: got %d rows, err=%v, want 1 row", len(rowsB), err)
	}
	rowsNone, err := st.ListCompensatingPaymentAudits(ctx, "ord-does-not-exist")
	if err != nil || len(rowsNone) != 0 {
		t.Fatalf("unknown reference: got %d rows, err=%v, want 0 rows", len(rowsNone), err)
	}
}
