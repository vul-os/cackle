package payments

import (
	"context"
	"errors"
	"testing"
)

func TestNewStub_RequiresOptIn(t *testing.T) {
	_, err := NewStub(false)
	if !errors.Is(err, ErrStubNotOptedIn) {
		t.Fatalf("NewStub(false) = %v, want ErrStubNotOptedIn", err)
	}
}

func TestNewStub_RefusesWhenRealSecretPresent(t *testing.T) {
	t.Setenv(EnvPaystackSecretKey, "sk_test_definitelyreal")
	_, err := NewStub(true)
	if !errors.Is(err, ErrStubRefusesRealSecret) {
		t.Fatalf("NewStub(true) with a real secret configured = %v, want ErrStubRefusesRealSecret", err)
	}
}

func TestNewStub_SucceedsOptedInNoSecret(t *testing.T) {
	t.Setenv(EnvPaystackSecretKey, "")
	stub, err := NewStub(true)
	if err != nil {
		t.Fatalf("NewStub(true) = %v, want nil", err)
	}
	if stub.Name() != ProviderNameStub {
		t.Fatalf("Name() = %q, want %q", stub.Name(), ProviderNameStub)
	}
}

func TestStub_BeginThenVerify(t *testing.T) {
	stub, err := NewStub(true)
	if err != nil {
		t.Fatalf("NewStub() = %v", err)
	}
	order := Order{ID: "ord_1", TotalCents: 12345, Currency: "ZAR", BuyerEmail: "a@b.com"}

	charge, err := stub.Begin(context.Background(), order)
	if err != nil {
		t.Fatalf("Begin() = %v", err)
	}
	if charge.Reference != order.ID {
		t.Fatalf("charge.Reference = %q, want %q", charge.Reference, order.ID)
	}

	result, err := stub.Verify(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("result.Status = %q, want paid", result.Status)
	}
	if result.AmountCents != order.TotalCents {
		t.Fatalf("result.AmountCents = %d, want %d", result.AmountCents, order.TotalCents)
	}
	if result.Currency != "ZAR" {
		t.Fatalf("result.Currency = %q, want ZAR", result.Currency)
	}
	if result.EventID == "" {
		t.Fatal("result.EventID is empty, want a non-empty idempotency id")
	}
}

func TestStub_VerifyUnknownReferenceFailsClosed(t *testing.T) {
	stub, _ := NewStub(true)
	_, err := stub.Verify(context.Background(), "never-began")
	if err == nil {
		t.Fatal("Verify() of an unknown reference = nil error, want failure")
	}
}

func TestStub_BeginRejectsMissingID(t *testing.T) {
	stub, _ := NewStub(true)
	_, err := stub.Begin(context.Background(), Order{TotalCents: 100})
	if err == nil {
		t.Fatal("Begin() with empty order id = nil error, want rejection")
	}
}

func TestStub_BeginRejectsNonPositiveAmount(t *testing.T) {
	stub, _ := NewStub(true)
	_, err := stub.Begin(context.Background(), Order{ID: "ord_1", TotalCents: 0})
	if err == nil {
		t.Fatal("Begin() with zero total_cents = nil error, want rejection")
	}
}

func TestStub_WebhookAlwaysFails(t *testing.T) {
	stub, _ := NewStub(true)
	_, err := stub.Webhook(context.Background(), nil)
	if err == nil {
		t.Fatal("Webhook() = nil error, want failure (stub has no webhook mechanism)")
	}
}
