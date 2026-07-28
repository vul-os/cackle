//go:build patala

package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	patala "github.com/vul-os/patala/patala-go/bindings/patala"
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
		t.Fatal("Webhooks = true for the manual rail, want false: it leaves verify_webhook at PaymentRail's Err(Unsupported) default (there is no processor behind it to push anything) -- see ErrPatalaNoWebhook")
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

// TestPatalaFiatProvider_WebhookOnManualRailIsUnsupported pins the ONE rail
// that still has no push surface. patala-fiat's `manual` leaves
// verify_webhook at PaymentRail's Err(Unsupported) trait default, and this
// asserts that reaches Cackle as ErrPatalaNoWebhook (poll Verify instead)
// rather than as a generic rejection -- through the real cdylib, so it is
// patala's actual answer being checked, not a Go-side name comparison.
func TestPatalaFiatProvider_WebhookOnManualRailIsUnsupported(t *testing.T) {
	rs := newFakeRecordStore()
	p, err := NewPatalaFiat("manual", rs)
	if err != nil {
		t.Fatalf("NewPatalaFiat: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/webhook/manual", strings.NewReader(`{"id":"evt_1"}`))
	result, err := p.Webhook(context.Background(), req)
	if !errors.Is(err, ErrPatalaNoWebhook) {
		t.Fatalf("Webhook error = %v, want one wrapping ErrPatalaNoWebhook", err)
	}
	if result.Status == StatusPaid {
		t.Fatal("an unsupported webhook returned StatusPaid")
	}
}

// --- the wired webhook path, end to end through the real Rust verifier ------
//
// These drive patala-fiat's Stripe rail, whose verify_webhook is entirely
// local (HMAC-SHA256 over "{t}.{raw_body}", per Stripe's documented signing
// scheme) and makes NO network call -- so a genuinely-signed delivery can be
// verified offline, by the actual compiled Rust code, with no credentials
// and no processor. That is what makes this a real test of the wiring rather
// than a mock of it.

const patalaTestStripeWebhookSecret = "whsec_test_secret_for_cackle_tests"

// stripeSignedDelivery builds a request carrying a VALID Stripe signature
// for body, computed exactly as patala-fiat's stripe::webhook expects it.
func stripeSignedDelivery(t *testing.T, body string, ts time.Time) *http.Request {
	t.Helper()
	signed := fmt.Sprintf("%d.%s", ts.Unix(), body)
	mac := hmac.New(sha256.New, []byte(patalaTestStripeWebhookSecret))
	mac.Write([]byte(signed))
	req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/stripe", strings.NewReader(body))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac.Sum(nil))))
	return req
}

func stripeCheckoutSessionEvent(eventID, reference, paymentStatus string, amountTotal int64, currency string) string {
	return fmt.Sprintf(`{"id":%q,"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","payment_status":%q,"amount_total":%d,"currency":%q,"client_reference_id":%q}}}`,
		eventID, paymentStatus, amountTotal, currency, reference)
}

// newStripeProviderForWebhookTest builds the Stripe rail with the webhook
// secret the helpers above sign with. Construction only ever reads config;
// nothing here dials Stripe.
func newStripeProviderForWebhookTest(t *testing.T) (*PatalaFiatProvider, *fakeRecordStore) {
	t.Helper()
	t.Setenv("CACKLE_STRIPE_SECRET_KEY", "sk_test_x")
	t.Setenv("CACKLE_STRIPE_WEBHOOK_SECRET", patalaTestStripeWebhookSecret)
	rs := newFakeRecordStore()
	p, err := NewPatalaFiat("stripe", rs)
	if err != nil {
		t.Fatalf("NewPatalaFiat(stripe): %v (is this build's cdylib compiled with fiat-stripe / fiat-all?)", err)
	}
	return p, rs
}

// TestPatalaFiatProvider_StripeCapabilitiesAdvertiseWebhooks is the other
// half of TestPatalaFiatProvider_CapabilitiesShape: a real processor rail
// DOES have a push surface, and Capabilities must now say so. Before patala
// exported verify_webhook this was false for every rail.
func TestPatalaFiatProvider_StripeCapabilitiesAdvertiseWebhooks(t *testing.T) {
	p, _ := newStripeProviderForWebhookTest(t)
	if !p.Capabilities().Webhooks {
		t.Fatal("Webhooks = false for the stripe rail, want true (patala_core::PaymentRail::verify_webhook is reachable through the binding and Webhook is wired to it)")
	}
}

// TestPatalaFiatProvider_WebhookSettlesAGenuineStripeDelivery is the proof
// that (and how) the webhook capability is actually wired: a correctly
// signed, paid Checkout Session must produce a paid Result carrying the
// RAIL's reported amount/currency and Stripe's own event id, and must move
// the stored record to paid.
func TestPatalaFiatProvider_WebhookSettlesAGenuineStripeDelivery(t *testing.T) {
	ctx := context.Background()
	p, rs := newStripeProviderForWebhookTest(t)

	const reference = "ord-patala-webhook-1"
	now := time.Now()
	if err := rs.PutPaymentRecord(ctx, PaymentRecord{
		Provider:    "stripe",
		Reference:   reference,
		AmountMinor: 5000,
		Currency:    "USD",
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seeding the record Begin would have written: %v", err)
	}

	body := stripeCheckoutSessionEvent("evt_paid_1", reference, "paid", 5000, "usd")
	result, err := p.Webhook(ctx, stripeSignedDelivery(t, body, now))
	if err != nil {
		t.Fatalf("Webhook on a genuinely signed delivery: %v", err)
	}
	if result.Status != StatusPaid {
		t.Fatalf("Status = %q, want %q", result.Status, StatusPaid)
	}
	if result.Reference != reference {
		t.Fatalf("Reference = %q, want %q", result.Reference, reference)
	}
	if result.EventID != "evt_paid_1" {
		t.Fatalf("EventID = %q, want Stripe's own event id %q (replay protection keys on it)", result.EventID, "evt_paid_1")
	}
	// The rail's reported figures, NOT the stored expectation -- substituting
	// our own would make payments.Reconcile compare a number against itself.
	if result.AmountMinor != 5000 || result.Currency != "USD" {
		t.Fatalf("AmountMinor/Currency = %d %s, want 5000 USD as the RAIL reported them", result.AmountMinor, result.Currency)
	}
	if result.PaidAt.IsZero() {
		t.Fatal("PaidAt is zero on a settled result")
	}

	// And Reconcile -- the check that actually protects the money -- must
	// accept it against the order it is really for, and reject a bigger
	// claim.
	if err := Reconcile(result, OrderRef{ID: reference, AmountMinor: 5000, Currency: "USD"}); err != nil {
		t.Fatalf("Reconcile against the true order: %v", err)
	}
	if err := Reconcile(result, OrderRef{ID: reference, AmountMinor: 100000, Currency: "USD"}); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("Reconcile against a 100000 order = %v, want ErrAmountMismatch", err)
	}

	rec, ok, err := rs.GetPaymentRecord(ctx, "stripe", reference)
	if err != nil || !ok {
		t.Fatalf("GetPaymentRecord after webhook: ok=%v err=%v", ok, err)
	}
	if rec.Status != StatusPaid {
		t.Fatalf("stored record status = %q after a settled webhook, want %q", rec.Status, StatusPaid)
	}
}

// TestPatalaFiatProvider_WebhookNeverPaysAnUnsettledDelivery is the
// correctness constraint this whole pass exists for, exercised end to end:
// an AUTHENTIC delivery that is not a settlement must never come back paid.
// A Stripe session with payment_status "unpaid" is patala's NotSettled;
// signature-only rails (BTCPay, Coinbase Commerce, OpenNode, LNbits,
// Mollie) produce Unconfirmed, which patala_webhook_status_test.go pins to
// StatusPending in the default build. Both land in the same arm here.
func TestPatalaFiatProvider_WebhookNeverPaysAnUnsettledDelivery(t *testing.T) {
	ctx := context.Background()
	p, rs := newStripeProviderForWebhookTest(t)

	const reference = "ord-patala-webhook-2"
	now := time.Now()
	if err := rs.PutPaymentRecord(ctx, PaymentRecord{
		Provider: "stripe", Reference: reference, AmountMinor: 5000, Currency: "USD",
		Status: StatusPending, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seeding record: %v", err)
	}

	body := stripeCheckoutSessionEvent("evt_unpaid_1", reference, "unpaid", 5000, "usd")
	result, err := p.Webhook(ctx, stripeSignedDelivery(t, body, now))
	if result.Status == StatusPaid {
		t.Fatal("an authentic but UNSETTLED delivery produced StatusPaid -- this issues tickets for an unpaid order")
	}
	if !errors.Is(err, ErrUnhandledEvent) {
		t.Fatalf("Webhook error = %v, want one wrapping ErrUnhandledEvent (ack the delivery, settle nothing)", err)
	}

	rec, ok, err := rs.GetPaymentRecord(ctx, "stripe", reference)
	if err != nil || !ok {
		t.Fatalf("GetPaymentRecord: ok=%v err=%v", ok, err)
	}
	if rec.Status != StatusPending {
		t.Fatalf("stored record status = %q after an unsettled webhook, want it left at %q", rec.Status, StatusPending)
	}
}

// TestPatalaFiatProvider_WebhookFailsClosedOnBadSignature covers the whole
// fail-closed family: no signature, a wrong signature, and one that is
// valid but stale beyond Stripe's replay window. None may produce a Result,
// and none may be mistaken for "this rail has no webhook surface".
func TestPatalaFiatProvider_WebhookFailsClosedOnBadSignature(t *testing.T) {
	ctx := context.Background()
	p, rs := newStripeProviderForWebhookTest(t)

	const reference = "ord-patala-webhook-3"
	now := time.Now()
	if err := rs.PutPaymentRecord(ctx, PaymentRecord{
		Provider: "stripe", Reference: reference, AmountMinor: 5000, Currency: "USD",
		Status: StatusPending, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seeding record: %v", err)
	}
	body := stripeCheckoutSessionEvent("evt_paid_2", reference, "paid", 5000, "usd")

	unsigned := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/stripe", strings.NewReader(body))

	forged := stripeSignedDelivery(t, body, now)
	forged.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", now.Unix(), strings.Repeat("00", sha256.Size)))

	// Correctly signed, but an hour old: Stripe's tolerance is 5 minutes.
	stale := stripeSignedDelivery(t, body, now.Add(-time.Hour))

	for _, tc := range []struct {
		name string
		req  *http.Request
	}{
		{"no signature header", unsigned},
		{"wrong signature", forged},
		{"stale signature", stale},
	} {
		result, err := p.Webhook(ctx, tc.req)
		if err == nil {
			t.Fatalf("%s: Webhook returned no error (status %q) -- it must fail closed", tc.name, result.Status)
		}
		if result.Status == StatusPaid {
			t.Fatalf("%s: rejected webhook still returned StatusPaid", tc.name)
		}
		if errors.Is(err, ErrPatalaNoWebhook) {
			t.Fatalf("%s: rejection reported as ErrPatalaNoWebhook -- a rejected delivery must not look like an unsupported rail", tc.name)
		}
		if errors.Is(err, ErrUnhandledEvent) {
			t.Fatalf("%s: rejection reported as ErrUnhandledEvent -- that would make the HTTP route ack a forged delivery with 200", tc.name)
		}
	}

	rec, _, _ := rs.GetPaymentRecord(ctx, "stripe", reference)
	if rec.Status != StatusPending {
		t.Fatalf("stored record status = %q after only rejected webhooks, want %q", rec.Status, StatusPending)
	}
}

// TestPatalaFiatProvider_WebhookRejectsUnknownReference: a perfectly signed
// settlement for an order this deployment never began is anomalous, and is
// refused rather than settled -- the same posture Verify takes.
func TestPatalaFiatProvider_WebhookRejectsUnknownReference(t *testing.T) {
	ctx := context.Background()
	p, _ := newStripeProviderForWebhookTest(t)

	body := stripeCheckoutSessionEvent("evt_paid_3", "never-began", "paid", 5000, "usd")
	result, err := p.Webhook(ctx, stripeSignedDelivery(t, body, time.Now()))
	if err == nil {
		t.Fatalf("Webhook for a reference nobody began returned no error (status %q)", result.Status)
	}
	if result.Status == StatusPaid {
		t.Fatal("Webhook for an unknown reference returned StatusPaid")
	}
}

// --- the mapping's tie to the generated bindings ----------------------------

// TestPatalaWebhookStatusConstantsMatchGeneratedBindings closes the loop on
// patala_webhook_status.go: that file hardcodes patala's WebhookStatus
// discriminants so the mapping can be tested in the DEFAULT (untagged)
// build, which means something has to guarantee the copy stays true.
//
// patala.go already asserts the numeric values at COMPILE time, so a
// renumbering cannot even build. The one thing a compile-time check cannot
// see is a variant that was ADDED -- so this reads the generated bindings
// source and asserts it declares no WebhookStatus variant the mapping does
// not know about. It fails closed: an unreadable or unrecognisable bindings
// file, or a suspiciously small variant count, is a failure, never a pass.
func TestPatalaWebhookStatusConstantsMatchGeneratedBindings(t *testing.T) {
	// Numeric agreement, restated at run time so a reader of the test
	// output can see what was compared.
	for _, tc := range []struct {
		name    string
		binding uint
		ours    patalaWebhookStatus
	}{
		{"Settled", uint(patala.WebhookStatusSettled), patalaWebhookSettled},
		{"NotSettled", uint(patala.WebhookStatusNotSettled), patalaWebhookNotSettled},
		{"Unconfirmed", uint(patala.WebhookStatusUnconfirmed), patalaWebhookUnconfirmed},
	} {
		if tc.binding != uint(tc.ours) {
			t.Fatalf("WebhookStatus%s is %d in the generated bindings but %d in patala_webhook_status.go", tc.name, tc.binding, uint(tc.ours))
		}
	}

	src := readGeneratedBindingsSource(t)
	// e.g. `WebhookStatusSettled     WebhookStatus = 1`
	re := regexp.MustCompile(`(?m)^\s*WebhookStatus([A-Za-z0-9]+)\s+WebhookStatus\s*=\s*(\d+)\s*$`)
	matches := re.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		t.Fatal("found no `WebhookStatus<Name> WebhookStatus = <n>` constant declarations in the generated bindings -- this check cannot verify anything, so it fails rather than passes silently")
	}

	declared := make(map[patalaWebhookStatus]string, len(matches))
	for _, m := range matches {
		value, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("WebhookStatus%s has unparseable discriminant %q", m[1], m[2])
		}
		declared[patalaWebhookStatus(value)] = m[1]
	}

	if len(declared) != len(patalaWebhookStatusNames) {
		t.Fatalf("the generated bindings declare %d WebhookStatus variants %v, but patala_webhook_status.go maps %d %v -- update the mapping AND its pinning test before this path is trusted again",
			len(declared), declared, len(patalaWebhookStatusNames), patalaWebhookStatusNames)
	}
	for value, name := range declared {
		ours, ok := patalaWebhookStatusNames[value]
		if !ok {
			t.Fatalf("the bindings declare WebhookStatus%s = %d, which patala_webhook_status.go does not map", name, uint(value))
		}
		if ours != name {
			t.Fatalf("discriminant %d is WebhookStatus%s in the bindings but %q in patala_webhook_status.go", uint(value), name, ours)
		}
		// Every declared variant must round-trip through the mapping
		// rather than hit its fail-closed default.
		if _, err := patalaWebhookStatusToCackle(value); err != nil {
			t.Fatalf("WebhookStatus%s = %d is declared by the bindings but falls into the mapping's unknown arm: %v", name, uint(value), err)
		}
	}
	t.Logf("verified %d WebhookStatus variants against the generated bindings: %v", len(declared), declared)
}

// readGeneratedBindingsSource returns the generated patala-go source this
// build actually links against, located via `go list` so it works wherever
// the sibling patala checkout lives (Makefile's PATALA_DIR is overridable).
// Every failure path is fatal: this test's whole value is in reading the
// real file.
func readGeneratedBindingsSource(t *testing.T) string {
	t.Helper()
	const pkg = "github.com/vul-os/patala/patala-go/bindings/patala"
	out, err := exec.Command("go", "list", "-f", "{{.Dir}}", pkg).Output()
	if err != nil {
		t.Fatalf("go list %s: %v (this test needs the generated bindings this build links against; run `make patala-generate`)", pkg, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Fatalf("go list %s returned an empty directory", pkg)
	}
	path := filepath.Join(dir, "patala_py.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated bindings %s: %v", path, err)
	}
	return string(src)
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
