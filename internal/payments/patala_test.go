//go:build patala

package payments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// These tests exercise the patala path entirely against patala-fiat's
// "manual" rail (built into the cdylib whenever the `fiat` feature is on
// at all — see patala.go's own doc comment) so they need zero real
// processor credentials and make zero network calls, while still proving
// the actual cgo round trip: PatalaConfigFromEnv -> patala.PatalaRailNewFiat
// -> Charge -> (store) -> Verify, through the real compiled Rust cdylib,
// not a mock of it.

func TestNewPatalaFiat_RequiresNonNilStore(t *testing.T) {
	if _, err := NewPatalaFiat("manual", nil); err == nil {
		t.Fatal("NewPatalaFiat with a nil store: want error, got nil")
	}
}

func TestNewPatalaFiat_UnknownProviderFailsClosed(t *testing.T) {
	rs := newFakeRecordStore()
	if _, err := NewPatalaFiat("not-a-real-processor", rs); err == nil {
		t.Fatal("NewPatalaFiat with an unknown provider name: want error, got nil")
	}
}

func TestPatalaFiatProvider_CapabilitiesShape(t *testing.T) {
	rs := newFakeRecordStore()
	p, err := NewPatalaFiat("manual", rs)
	if err != nil {
		t.Fatalf("NewPatalaFiat: %v", err)
	}
	if p.Name() != "manual" {
		t.Fatalf("Name() = %q, want manual", p.Name())
	}
	caps := p.Capabilities()
	if caps.Flow != FlowRedirect {
		t.Fatalf("Flow = %q, want %q (see Capabilities doc comment on the approximation)", caps.Flow, FlowRedirect)
	}
	if caps.Webhooks {
		t.Fatal("Webhooks = true, want false (patala has no webhook surface -- see ErrPatalaNoWebhook)")
	}
	if !caps.ZeroDecimalOK {
		t.Fatal("ZeroDecimalOK = false, want true (patala-fiat always routes through its own currency table)")
	}
}

func TestPatalaFiatProvider_BeginPersistsAndVerifyIsHonestlyPending(t *testing.T) {
	ctx := context.Background()
	rs := newFakeRecordStore()
	p, err := NewPatalaFiat("manual", rs)
	if err != nil {
		t.Fatalf("NewPatalaFiat: %v", err)
	}

	order := Order{
		Reference:   "ord-patala-1",
		AmountMinor: 1000,
		Currency:    "JPY", // zero-decimal -- money must render as 1000 JPY, never 10.00
		CallbackURL: "https://example.test/return",
	}
	charge, err := p.Begin(ctx, order)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if charge.Reference != order.Reference {
		t.Fatalf("Charge.Reference = %q, want %q", charge.Reference, order.Reference)
	}

	rec, ok, err := rs.GetPaymentRecord(ctx, "manual", order.Reference)
	if err != nil {
		t.Fatalf("GetPaymentRecord: %v", err)
	}
	if !ok {
		t.Fatal("Begin did not persist a PaymentRecord via the RecordStore seam")
	}
	if rec.AmountMinor != order.AmountMinor || rec.Currency != order.Currency {
		t.Fatalf("persisted record = %d %s, want %d %s (the ORDER's real total, not the 0 charge() itself returns)",
			rec.AmountMinor, rec.Currency, order.AmountMinor, order.Currency)
	}

	// patala's ManualRail can only ever be marked paid via a direct Rust
	// call to mark_paid, which is NOT reachable through the generic
	// PatalaRailNewFiat/PatalaRail FFI surface this adapter uses (see
	// patala.go's module doc and patala-go/README.md's own "what a cackle
	// consumer needs to know"). So Verify here must stay honestly pending
	// forever -- never fabricate a "paid" this seam cannot actually
	// observe.
	result, err := p.Verify(ctx, order.Reference)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Status != StatusPending {
		t.Fatalf("Status = %q, want pending (patala manual can never actually settle through this FFI surface)", result.Status)
	}
}

func TestPatalaFiatProvider_VerifyUnknownReferenceFailsClosed(t *testing.T) {
	ctx := context.Background()
	rs := newFakeRecordStore()
	p, err := NewPatalaFiat("manual", rs)
	if err != nil {
		t.Fatalf("NewPatalaFiat: %v", err)
	}
	if _, err := p.Verify(ctx, "never-began"); err == nil {
		t.Fatal("Verify for a reference nobody ever began: want error, got nil")
	}
}

func TestPatalaFiatProvider_WebhookAlwaysFails(t *testing.T) {
	rs := newFakeRecordStore()
	p, err := NewPatalaFiat("manual", rs)
	if err != nil {
		t.Fatalf("NewPatalaFiat: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook/manual", nil)
	if _, err := p.Webhook(context.Background(), req); err != ErrPatalaNoWebhook {
		t.Fatalf("Webhook error = %v, want ErrPatalaNoWebhook", err)
	}
}

func TestPatalaConfigFromEnv(t *testing.T) {
	os.Setenv("CACKLE_STRIPE_SECRET_KEY", "sk_test_x")
	os.Setenv("CACKLE_STRIPE_WEBHOOK_SECRET", "whsec_x")
	os.Setenv("CACKLE_STRIPE_UNRELATED", "") // empty values must be dropped, not passed through
	defer func() {
		os.Unsetenv("CACKLE_STRIPE_SECRET_KEY")
		os.Unsetenv("CACKLE_STRIPE_WEBHOOK_SECRET")
		os.Unsetenv("CACKLE_STRIPE_UNRELATED")
	}()

	cfg := PatalaConfigFromEnv("stripe")
	if cfg["secret_key"] != "sk_test_x" {
		t.Fatalf(`cfg["secret_key"] = %q, want "sk_test_x"`, cfg["secret_key"])
	}
	if cfg["webhook_secret"] != "whsec_x" {
		t.Fatalf(`cfg["webhook_secret"] = %q, want "whsec_x"`, cfg["webhook_secret"])
	}
	if _, ok := cfg["unrelated"]; ok {
		t.Fatal("an empty-valued env var must not appear in the config map")
	}

	if got := PatalaConfigFromEnv("nobody-configured-this"); len(got) != 0 {
		t.Fatalf("PatalaConfigFromEnv for an unconfigured provider = %v, want empty", got)
	}
}

func TestPatalaConfigFromEnv_KeyOverrides(t *testing.T) {
	os.Setenv("CACKLE_ADYEN_HMAC_KEY", "deadbeef")
	defer os.Unsetenv("CACKLE_ADYEN_HMAC_KEY")

	cfg := PatalaConfigFromEnv("adyen")
	if cfg["hmac_key_hex"] != "deadbeef" {
		t.Fatalf(`cfg["hmac_key_hex"] = %q, want "deadbeef" (CACKLE_ADYEN_HMAC_KEY must map onto patala-fiat's own "hmac_key_hex" key, not a literal "hmac_key")`, cfg["hmac_key_hex"])
	}
	if _, ok := cfg["hmac_key"]; ok {
		t.Fatal("the un-overridden literal-lowercase key must not ALSO be present")
	}
}

// TestPatalaFiatProvider_StripeConstructsOfflineFromEnv proves the config
// mapping (CACKLE_STRIPE_* -> patala-fiat's "secret_key"/"webhook_secret"
// keys) actually works for a REAL, feature-gated processor adapter, not
// just "manual" -- construction only, exactly like patala-go's own
// examples/fiatroundtrip does for the identical reason (never dial a real
// processor from an automated test).
func TestPatalaFiatProvider_StripeConstructsOfflineFromEnv(t *testing.T) {
	os.Setenv("CACKLE_STRIPE_SECRET_KEY", "sk_test_x")
	os.Setenv("CACKLE_STRIPE_WEBHOOK_SECRET", "whsec_x")
	defer func() {
		os.Unsetenv("CACKLE_STRIPE_SECRET_KEY")
		os.Unsetenv("CACKLE_STRIPE_WEBHOOK_SECRET")
	}()

	rs := newFakeRecordStore()
	p, err := NewPatalaFiat("stripe", rs)
	if err != nil {
		t.Fatalf("NewPatalaFiat(stripe): %v (is this build's cdylib compiled with fiat-stripe / fiat-all?)", err)
	}
	if p.Name() != "stripe" {
		t.Fatalf("Name() = %q, want stripe", p.Name())
	}
	// Construction-only: never Begin/Verify here, which would dial the
	// real Stripe API with a fake key.
}

func TestPatalaFiatProviderNames_IncludesManual(t *testing.T) {
	names := PatalaFiatProviderNames()
	found := false
	for _, n := range names {
		if n == "manual" {
			found = true
		}
	}
	if !found {
		t.Fatalf("PatalaFiatProviderNames() = %v, want it to include \"manual\"", names)
	}
}
