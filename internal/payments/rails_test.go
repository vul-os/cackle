package payments

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// This file is the cross-rail matrix. Every other _test.go in this package
// tests ONE adapter in depth; this one asserts the properties that must
// hold for EVERY rail a default (untagged, pure-Go) build can construct, so
// a new adapter cannot be added without either satisfying them or visibly
// editing this table.
//
// The property under test is the one that costs real money when it breaks:
// a rail that has NOT established settlement must never produce a Result
// whose Status is StatusPaid. "Has not established settlement" covers all
// of: still waiting on the buyer (manual), abandoned at the processor
// (paystack), on-chain but not yet confirmed to the operator's own
// threshold (stablecoin), and an authentic-but-contentless webhook
// notification (the patala path — pinned separately and untagged in
// patala_webhook_status_test.go, because that rail cannot be constructed
// without cgo; see patalaWebhookStatusToCackle).
//
// Note what is deliberately NOT unified here: the inbound signature checks
// themselves. Paystack is hex HMAC-SHA512, Adyen base64, iyzico SHA-1,
// payfast MD5, Mollie has none at all. Those live per-provider on purpose
// — a shared "generic verifier" would be one switch statement over a dozen
// third parties' unrelated quirks, and every implementor would inherit
// every other implementor's mistakes. This table only asserts the OUTCOME
// each rail must reach, never how it gets there.

// railCase is one rail in the matrix. new builds the provider (rails with
// no push surface still implement Webhook — they must error, never
// fabricate a Result), and unsettled drives it to its most dangerous
// not-yet-paid state.
type railCase struct {
	name string
	// new constructs the rail. Rails needing env/HTTP fakes set them up
	// here against t, so each case is hermetic.
	new func(t *testing.T) Provider
	// unsettledVerify returns the reference to poll for a charge that has
	// NOT settled. nil means this rail has no meaningful "began but
	// unsettled" Verify state to poll (the stub settles inside Begin).
	// Nothing is SKIPPED on account of this being nil — `go test` without
	// -v discards a passing package's output, so a skip is invisible in
	// the run that matters. Cases with no unsettled state are counted and
	// asserted instead; see railsWithUnsettledState.
	unsettledVerify func(t *testing.T, p Provider) string
}

func railCases() []railCase {
	return []railCase{
		{
			name: "manual",
			new: func(t *testing.T) Provider {
				t.Helper()
				return NewManual(nil)
			},
			unsettledVerify: func(t *testing.T, p Provider) string {
				t.Helper()
				// Begun, but no organiser has marked it paid. This is the
				// default rail's entire normal state for a live order.
				if _, err := p.Begin(context.Background(), Order{
					Reference: "ord_matrix_manual", AmountMinor: 100, Currency: "USD",
				}); err != nil {
					t.Fatalf("manual Begin: %v", err)
				}
				return "ord_matrix_manual"
			},
		},
		{
			name: "stub",
			new: func(t *testing.T) Provider {
				t.Helper()
				s, err := NewStub(true)
				if err != nil {
					t.Fatalf("NewStub: %v", err)
				}
				return s
			},
			// The stub settles inside Begin by design, so it has no
			// unsettled Verify state. Its Webhook must still fail closed,
			// which the shared assertions below cover.
			unsettledVerify: nil,
		},
		{
			name: "paystack",
			new: func(t *testing.T) Provider {
				t.Helper()
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// A transaction the buyer walked away from. The
					// processor answers happily; the money never moved.
					_, _ = w.Write([]byte(`{"status":true,"message":"ok","data":{"id":1,"status":"abandoned","reference":"ord_matrix_paystack","amount":5000,"currency":"ZAR"}}`))
				}))
				t.Cleanup(ts.Close)
				return newTestPaystackProvider(t, ts)
			},
			unsettledVerify: func(t *testing.T, p Provider) string {
				t.Helper()
				return "ord_matrix_paystack"
			},
		},
		{
			name: "stablecoin",
			new: func(t *testing.T) Provider {
				t.Helper()
				allocator := newFakeStablecoinAllocator("0xMatrixAddr")
				now := time.Now()
				allocator.byAddr["0xmatrixaddr"] = StablecoinAllocation{
					Address: "0xMatrixAddr", AmountMinor: 1234, Currency: "USD",
					AllocatedAt: now.Add(-time.Minute),
				}
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// The full amount IS on-chain — but with 1
					// confirmation against a configured threshold of 12.
					// This is the crypto-specific shape of "not yet
					// final": a card rail has no equivalent, and
					// flattening it to paid would issue tickets against a
					// transfer that can still be reorged away.
					_, _ = w.Write([]byte(`{"status":"1","message":"OK","result":[` +
						tokenTxJSON("0xMatrixAddr", "12340000", 6, 1, now.Unix()) + `]}`))
				}))
				t.Cleanup(ts.Close)
				return newTestStablecoin(t, ts, allocator)
			},
			unsettledVerify: func(t *testing.T, p Provider) string {
				t.Helper()
				return "0xMatrixAddr"
			},
		},
	}
}

// railsWithUnsettledState returns the subset of rails that have a
// began-but-unsettled Verify state, and fails if that subset ever shrinks
// below what this repo actually ships. Counting them is what lets the
// tests below run with NO t.Skip anywhere: a rail that quietly stopped
// being exercised turns into a failure here rather than into output nobody
// sees, per this repo's fail-closed guard rule.
func railsWithUnsettledState(t *testing.T) []railCase {
	t.Helper()
	const wantAtLeast = 3 // manual, paystack, stablecoin
	var out []railCase
	for _, rc := range railCases() {
		if rc.unsettledVerify != nil {
			out = append(out, rc)
		}
	}
	if len(out) < wantAtLeast {
		t.Fatalf("only %d rail(s) expose a began-but-unsettled Verify state, want at least %d — a rail lost its coverage silently", len(out), wantAtLeast)
	}
	return out
}

// TestEveryRail_UnsettledNeverBecomesPaid is the money test. For each rail
// in the default build, drive it to a state where the buyer has not (yet)
// actually paid, and assert the rail reports anything except StatusPaid.
//
// An order marked paid on an unconfirmed transaction is the worst bug
// available in this package: it issues a real, signed, offline-verifiable
// ticket for money that never arrived, and the gate will happily admit it
// with the venue network down.
func TestEveryRail_UnsettledNeverBecomesPaid(t *testing.T) {
	// Every rail, including ones that settle inside Begin: a reference
	// this rail never began must never verify as paid.
	for _, rc := range railCases() {
		t.Run(rc.name+"/never_began", func(t *testing.T) {
			p := rc.new(t)
			result, err := p.Verify(context.Background(), "ord_matrix_never_began")
			if err == nil && result.Status == StatusPaid {
				t.Fatalf("%s: a reference this rail never began verified as StatusPaid", rc.name)
			}
			if result.Status == StatusPaid {
				t.Fatalf("%s: Verify errored (%v) but still returned Status=paid — a caller ignoring the error would issue tickets", rc.name, err)
			}
		})
	}

	for _, rc := range railsWithUnsettledState(t) {
		t.Run(rc.name+"/began_unsettled", func(t *testing.T) {
			p := rc.new(t)
			ref := rc.unsettledVerify(t, p)

			result, err := p.Verify(context.Background(), ref)
			if err != nil {
				// Fail-closed by erroring is fine — it cannot be mistaken
				// for a settlement by any caller. What is NOT fine is
				// returning a paid Result alongside an error, since a
				// caller that logs-and-continues would then act on it.
				if result.Status == StatusPaid {
					t.Fatalf("%s: Verify errored (%v) but still returned Status=paid", rc.name, err)
				}
				return
			}
			if result.Status == StatusPaid {
				t.Fatalf("%s: an unsettled charge verified as StatusPaid — this issues tickets for money that never arrived", rc.name)
			}
		})
	}
}

// TestEveryRail_ReconcileRefusesEveryUnsettledResult closes the loop: even
// if a rail's Verify were to hand back a non-paid Result, the shared
// Reconcile gate that every settlement path runs through must refuse it.
// This is the belt to the previous test's braces — it means a future rail
// that gets its own status mapping wrong still cannot settle an order
// unless it explicitly says StatusPaid.
func TestEveryRail_ReconcileRefusesEveryUnsettledResult(t *testing.T) {
	for _, rc := range railsWithUnsettledState(t) {
		t.Run(rc.name, func(t *testing.T) {
			p := rc.new(t)
			ref := rc.unsettledVerify(t, p)

			result, err := p.Verify(context.Background(), ref)
			if err != nil {
				return // already fail-closed upstream of Reconcile
			}
			// Reconcile against an order that agrees on everything EXCEPT
			// settlement, so the only thing that can let it through is the
			// status check itself.
			want := OrderRef{ID: result.Reference, AmountMinor: result.AmountMinor, Currency: result.Currency}
			if err := Reconcile(result, want); err == nil {
				t.Fatalf("%s: Reconcile accepted an unsettled result (status=%q) whose amount and currency happen to match", rc.name, result.Status)
			}
		})
	}
}

// TestEveryRail_UnsignedWebhookIsAlwaysRejected pins both halves of
// Capabilities().Webhooks, with no skip on either branch:
//
//   - Webhooks == false: the rail must ERROR from Webhook rather than
//     return an empty Result. An empty Result has Status "" which is not
//     StatusPaid, so it would fail closed by accident — but a caller could
//     still read Reference/AmountMinor off it and believe the delivery was
//     understood.
//   - Webhooks == true: the rail has a real signature scheme, and a body
//     carrying NO signature at all must be rejected. This deliberately does
//     not construct a valid signature for any rail — each scheme is
//     different (paystack hex HMAC-SHA512, adyen base64, iyzico SHA-1,
//     payfast MD5, mollie none) and belongs in that rail's own file. All
//     this asserts is the outcome every scheme must share.
func TestEveryRail_UnsignedWebhookIsAlwaysRejected(t *testing.T) {
	for _, rc := range railCases() {
		t.Run(rc.name, func(t *testing.T) {
			p := rc.new(t)
			req := httptest.NewRequest(http.MethodPost, "/api/payments/webhook/"+p.Name(),
				bytes.NewReader([]byte(`{"event":"charge.success","data":{"status":"success","reference":"ord_matrix","amount":5000,"currency":"ZAR"}}`)))
			result, err := p.Webhook(context.Background(), req)
			if err == nil {
				t.Fatalf("%s: an unsigned webhook body was accepted (status=%q, webhooks_capability=%v) — a forged delivery would settle an order",
					rc.name, result.Status, p.Capabilities().Webhooks)
			}
			if result.Status == StatusPaid {
				t.Fatalf("%s: Webhook errored but still returned Status=paid", rc.name)
			}
		})
	}
}

// TestHandleWebhook_DoubleDeliveryPassesOnce is the seam-level half of the
// idempotency guarantee: the same signed delivery arriving twice must be
// admitted exactly once, so only one Settle call is ever attempted.
//
// The AUTHORITATIVE guarantee against double-issuing tickets is
// orders.Service.Settle's compare-and-set on the order row (see
// TestSettle_IssuesTicketsAndIsIdempotent and
// TestSettle_ConcurrentDoubleDeliveryIssuesOnce in internal/orders) —
// replay protection here is the cheap first line, not the last one. Both
// exist because a SeenStore that is per-process, or that is lost on
// restart, still must not be able to double-issue.
func TestHandleWebhook_DoubleDeliveryPassesOnce(t *testing.T) {
	paid := Result{
		Provider: "fake", Reference: "ord_dup", EventID: "evt_dup",
		Status: StatusPaid, AmountMinor: 2500, Currency: "USD",
	}
	p := &fakeProvider{name: "fake", webhookResult: paid}
	seen := newFakeSeenStore()
	lookup := staticOrderLookup{ref: OrderRef{ID: "ord_dup", AmountMinor: 2500, Currency: "USD"}}

	req := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, "/api/payments/webhook/fake", bytes.NewReader([]byte(`{}`)))
	}

	first, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if first.Status != StatusPaid {
		t.Fatalf("first delivery status = %q, want paid", first.Status)
	}

	second, err := HandleWebhook(context.Background(), p, req(), seen, lookup)
	if err == nil {
		t.Fatal("second delivery of the same event was admitted — it would reach Settle a second time")
	}
	if second.Status == StatusPaid {
		t.Fatalf("second delivery was rejected (%v) but still returned Status=paid", err)
	}
}

// staticOrderLookup is an OrderLookup that always returns one stored
// order, standing in for the caller's own database read.
type staticOrderLookup struct{ ref OrderRef }

func (s staticOrderLookup) Lookup(_ context.Context, _ string) (OrderRef, error) {
	return s.ref, nil
}
