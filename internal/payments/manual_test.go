package payments

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestManualCapabilities(t *testing.T) {
	m := NewManual(nil)
	caps := m.Capabilities()
	if caps.Flow != FlowManual {
		t.Fatalf("Capabilities().Flow = %q, want FlowManual", caps.Flow)
	}
	if caps.Webhooks {
		t.Fatal("Capabilities().Webhooks = true, want false")
	}
	if !caps.ZeroDecimalOK {
		t.Fatal("Capabilities().ZeroDecimalOK = false, want true")
	}
	if len(caps.Currencies) != 0 {
		t.Fatalf("Capabilities().Currencies = %v, want empty (unrestricted)", caps.Currencies)
	}
	if !caps.SupportsCurrency("JPY") || !caps.SupportsCurrency("KWD") {
		t.Fatal("manual provider should support every currency")
	}
}

func TestManualBegin_ReturnsInstructions(t *testing.T) {
	m := NewManual(nil)
	order := Order{Reference: "ord_1", AmountMinor: 12345, Currency: "USD", BuyerEmail: "a@b.com"}
	charge, err := m.Begin(context.Background(), order)
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.Provider != ProviderNameManual {
		t.Fatalf("charge.Provider = %q, want %q", charge.Provider, ProviderNameManual)
	}
	if charge.Reference != "ord_1" {
		t.Fatalf("charge.Reference = %q, want ord_1", charge.Reference)
	}
	if charge.RedirectURL != "" {
		t.Fatal("manual provider must never return a RedirectURL")
	}
	if charge.Instructions == "" {
		t.Fatal("charge.Instructions is empty")
	}
}

func TestManualBegin_IdempotentOnRetry(t *testing.T) {
	m := NewManual(nil)
	order := Order{Reference: "ord_1", AmountMinor: 500, Currency: "USD"}
	first, err := m.Begin(context.Background(), order)
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	// A different (buggy) amount on retry must not change the recorded
	// instructions/amount — Begin should return the ALREADY recorded
	// charge, not re-derive one.
	second, err := m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 999999, Currency: "EUR"})
	if err != nil {
		t.Fatalf("second Begin() = %v", err)
	}
	if second.Instructions != first.Instructions {
		t.Fatalf("second Begin() returned different instructions: %q vs %q", second.Instructions, first.Instructions)
	}
}

func TestManualBegin_RejectsMissingReference(t *testing.T) {
	m := NewManual(nil)
	_, err := m.Begin(context.Background(), Order{AmountMinor: 100, Currency: "USD"})
	if err == nil {
		t.Fatal("Begin() with empty reference = nil error, want rejection")
	}
}

func TestManualBegin_RejectsNonPositiveAmount(t *testing.T) {
	m := NewManual(nil)
	_, err := m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 0, Currency: "USD"})
	if err == nil {
		t.Fatal("Begin() with zero amount = nil error, want rejection")
	}
}

func TestManualBegin_RejectsInvalidCurrency(t *testing.T) {
	m := NewManual(nil)
	_, err := m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "ZZZ"})
	if err == nil {
		t.Fatal("Begin() with unknown currency = nil error, want rejection")
	}
}

func TestManualBegin_ZeroDecimalCurrencyFormatsCorrectly(t *testing.T) {
	m := NewManual(nil)
	charge, err := m.Begin(context.Background(), Order{Reference: "ord_jpy", AmountMinor: 1000, Currency: "JPY"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if !strings.Contains(charge.Instructions, "1000 JPY") {
		t.Fatalf("Instructions = %q, want it to contain '1000 JPY' (not '10.00')", charge.Instructions)
	}
}

func TestManualBegin_CustomInstructions(t *testing.T) {
	called := false
	m := NewManual(func(o Order) (string, error) {
		called = true
		return "Bank transfer to Acme Org, ref " + o.Reference, nil
	})
	charge, err := m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if !called {
		t.Fatal("custom ManualInstructions function was not called")
	}
	if charge.Instructions != "Bank transfer to Acme Org, ref ord_1" {
		t.Fatalf("Instructions = %q", charge.Instructions)
	}
}

func TestManualBegin_CustomInstructionsErrorPropagates(t *testing.T) {
	m := NewManual(func(o Order) (string, error) {
		return "", errors.New("boom")
	})
	_, err := m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})
	if err == nil {
		t.Fatal("Begin() = nil error, want propagated instructions error")
	}
}

func TestManualVerify_PendingBeforeMarkPaid(t *testing.T) {
	m := NewManual(nil)
	m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})
	result, err := m.Verify(context.Background(), "ord_1")
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPending {
		t.Fatalf("result.Status = %q, want pending", result.Status)
	}
}

func TestManualVerify_UnknownReferenceFailsClosed(t *testing.T) {
	m := NewManual(nil)
	_, err := m.Verify(context.Background(), "never-began")
	if err == nil {
		t.Fatal("Verify() of an unknown reference = nil error, want failure")
	}
}

func TestManualMarkPaid_RecordsWhoAndWhen(t *testing.T) {
	m := NewManual(nil)
	m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})

	result, err := m.MarkPaid(context.Background(), "ord_1", "organiser@example.com")
	if err != nil {
		t.Fatalf("MarkPaid() = %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("result.Status = %q, want paid", result.Status)
	}
	if result.EventID == "" {
		t.Fatal("result.EventID is empty, want a non-empty idempotency id")
	}
	if result.PaidAt.IsZero() {
		t.Fatal("result.PaidAt is zero, want the mark time")
	}

	rec, ok := m.Record("ord_1")
	if !ok {
		t.Fatal("Record() = not found")
	}
	if rec.MarkedBy != "organiser@example.com" {
		t.Fatalf("rec.MarkedBy = %q, want organiser@example.com", rec.MarkedBy)
	}
	if rec.MarkedAt.IsZero() {
		t.Fatal("rec.MarkedAt is zero, want the mark time")
	}
}

func TestManualMarkPaid_RequiresMarkedBy(t *testing.T) {
	m := NewManual(nil)
	m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})
	_, err := m.MarkPaid(context.Background(), "ord_1", "")
	if err == nil {
		t.Fatal("MarkPaid() with empty markedBy = nil error, want rejection (no actor to audit)")
	}
}

func TestManualMarkPaid_UnknownReferenceFailsClosed(t *testing.T) {
	m := NewManual(nil)
	_, err := m.MarkPaid(context.Background(), "never-began", "organiser@example.com")
	if err == nil {
		t.Fatal("MarkPaid() of an unknown reference = nil error, want failure")
	}
}

func TestManualMarkPaid_IdempotentKeepsOriginalAuditTrail(t *testing.T) {
	m := NewManual(nil)
	m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})

	first, err := m.MarkPaid(context.Background(), "ord_1", "alice@example.com")
	if err != nil {
		t.Fatalf("first MarkPaid() = %v", err)
	}
	second, err := m.MarkPaid(context.Background(), "ord_1", "bob@example.com")
	if err != nil {
		t.Fatalf("second MarkPaid() = %v", err)
	}
	if second.EventID != first.EventID {
		t.Fatalf("second MarkPaid() produced a different EventID: %q vs %q", second.EventID, first.EventID)
	}
	rec, _ := m.Record("ord_1")
	if rec.MarkedBy != "alice@example.com" {
		t.Fatalf("rec.MarkedBy = %q, want the FIRST marker (alice@example.com)", rec.MarkedBy)
	}
}

func TestManualMarkFailed_RecordsAudit(t *testing.T) {
	m := NewManual(nil)
	m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})
	result, err := m.MarkFailed(context.Background(), "ord_1", "organiser@example.com")
	if err != nil {
		t.Fatalf("MarkFailed() = %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("result.Status = %q, want failed", result.Status)
	}
	rec, _ := m.Record("ord_1")
	if rec.MarkedBy != "organiser@example.com" {
		t.Fatalf("rec.MarkedBy = %q, want organiser@example.com", rec.MarkedBy)
	}
}

func TestManualMarkPaid_ReconcilesViaReconcile(t *testing.T) {
	m := NewManual(nil)
	m.Begin(context.Background(), Order{Reference: "ord_1", AmountMinor: 100, Currency: "USD"})
	result, err := m.MarkPaid(context.Background(), "ord_1", "organiser@example.com")
	if err != nil {
		t.Fatalf("MarkPaid() = %v", err)
	}
	want := OrderRef{ID: "ord_1", AmountMinor: 100, Currency: "USD"}
	if err := Reconcile(result, want); err != nil {
		t.Fatalf("Reconcile() = %v, want nil", err)
	}
}

func TestManualWebhook_AlwaysFails(t *testing.T) {
	m := NewManual(nil)
	_, err := m.Webhook(context.Background(), nil)
	if !errors.Is(err, ErrManualNoWebhook) {
		t.Fatalf("Webhook() = %v, want ErrManualNoWebhook", err)
	}
}

func TestManualProvider_ImplementsProviderInterface(t *testing.T) {
	var _ Provider = NewManual(nil)
}
