package payments

import "testing"

// These tests are deliberately UNTAGGED (no `//go:build patala`), so they
// run in the ordinary `go test ./internal/...` suite — see
// patala_webhook_status.go's file comment for why the mapping lives outside
// the cgo-only build in the first place.
//
// What they exist to stop: a future regeneration of patala-go's bindings
// (or a well-meaning edit) quietly re-pointing WebhookStatus.Unconfirmed —
// an authenticated delivery that asserts NOTHING about money — at
// StatusPaid, which would issue tickets for unpaid orders on every
// signature-only rail patala-fiat ships (BTCPay, Coinbase Commerce,
// OpenNode, LNbits, Mollie).

// TestPatalaWebhookStatusToCackle_PinsEveryVariant is the mapping contract,
// written out one variant at a time rather than derived from the code under
// test, so changing patalaWebhookStatusToCackle can never change what this
// test expects.
func TestPatalaWebhookStatusToCackle_PinsEveryVariant(t *testing.T) {
	cases := []struct {
		name string
		in   patalaWebhookStatus
		want Status
	}{
		// The rail established settlement. The ONLY paid mapping there is.
		{"Settled", 1, StatusPaid},
		// Authentic, and the rail says it has not settled. patala conflates
		// pending/failed/cancelled/expired here, so this must NOT become
		// Cackle's terminal StatusFailed.
		{"NotSettled", 2, StatusPending},
		// Authentic, asserts nothing about money. NEVER StatusPaid.
		{"Unconfirmed", 3, StatusPending},
	}

	// Coverage assertion: this table must cover every variant this build
	// knows about. A binding that grew a variant, or a constant that was
	// renumbered, fails here instead of slipping through untested.
	if len(cases) != len(patalaWebhookStatusNames) {
		t.Fatalf("this table pins %d variants but the build knows %d (%v) — every WebhookStatus variant must be pinned here",
			len(cases), len(patalaWebhookStatusNames), patalaWebhookStatusNames)
	}
	seen := make(map[patalaWebhookStatus]bool, len(cases))
	for _, tc := range cases {
		if name, ok := patalaWebhookStatusNames[tc.in]; !ok {
			t.Fatalf("discriminant %d (%q here) is not a variant this build declares", uint(tc.in), tc.name)
		} else if name != tc.name {
			t.Fatalf("discriminant %d is %q in this build, but this table calls it %q — the constants were renumbered", uint(tc.in), name, tc.name)
		}
		if seen[tc.in] {
			t.Fatalf("discriminant %d appears twice in the table", uint(tc.in))
		}
		seen[tc.in] = true
	}
	for ws, name := range patalaWebhookStatusNames {
		if !seen[ws] {
			t.Fatalf("variant %s (%d) is declared by this build but not pinned by this table", name, uint(ws))
		}
	}

	for _, tc := range cases {
		got, err := patalaWebhookStatusToCackle(tc.in)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s (%d) maps to %q, want %q", tc.name, uint(tc.in), got, tc.want)
		}
	}
}

// TestPatalaWebhookStatusToCackle_OnlySettledIsEverPaid states the one
// invariant in its own right, independent of the table above: exactly one
// discriminant may produce StatusPaid, and it is Settled. The sweep covers
// far more values than the enum declares so an added-but-unmapped variant
// (which lands in the fail-closed default) cannot come out paid either.
func TestPatalaWebhookStatusToCackle_OnlySettledIsEverPaid(t *testing.T) {
	const sweep = 256
	paid := make([]uint, 0, 1)
	for i := uint(0); i < sweep; i++ {
		status, err := patalaWebhookStatusToCackle(patalaWebhookStatus(i))
		if err != nil {
			// Unknown discriminant: must fail closed, and must not have
			// handed back a usable settlement status anyway.
			if status == StatusPaid {
				t.Fatalf("discriminant %d returned an error AND StatusPaid — the fail-closed path must never return paid", i)
			}
			continue
		}
		if status == StatusPaid {
			paid = append(paid, i)
		}
	}
	if len(paid) != 1 {
		t.Fatalf("%d discriminants in [0,%d) map to StatusPaid (%v), want exactly 1", len(paid), sweep, paid)
	}
	if patalaWebhookStatus(paid[0]) != patalaWebhookSettled {
		t.Fatalf("discriminant %d maps to StatusPaid, but Settled is %d", paid[0], uint(patalaWebhookSettled))
	}
}

// TestPatalaWebhookStatusToCackle_UnconfirmedIsNeverPaid is the single
// assertion this whole file exists for, spelled out so a reader (or a
// future regeneration) cannot miss it. Unconfirmed means "authentic, says
// nothing about money" — treating it as settlement marks unpaid orders
// paid.
func TestPatalaWebhookStatusToCackle_UnconfirmedIsNeverPaid(t *testing.T) {
	got, err := patalaWebhookStatusToCackle(patalaWebhookUnconfirmed)
	if err != nil {
		t.Fatalf("Unconfirmed: unexpected error: %v", err)
	}
	if got == StatusPaid {
		t.Fatal("WebhookStatus.Unconfirmed maps to StatusPaid — this issues tickets for orders nobody paid for; it MUST map to StatusPending")
	}
	if got != StatusPending {
		t.Fatalf("WebhookStatus.Unconfirmed maps to %q, want %q", got, StatusPending)
	}
}

// TestPatalaWebhookStatusToCackle_UnknownFailsClosed pins the default arm:
// a discriminant this build does not know is an error, not a guess.
func TestPatalaWebhookStatusToCackle_UnknownFailsClosed(t *testing.T) {
	// One past the highest declared variant — the shape a bindings
	// regeneration that added a variant would actually take.
	next := patalaWebhookStatus(len(patalaWebhookStatusNames) + 1)
	if _, ok := patalaWebhookStatusNames[next]; ok {
		t.Fatalf("discriminant %d is declared after all; this test needs an undeclared one", uint(next))
	}
	status, err := patalaWebhookStatusToCackle(next)
	if err == nil {
		t.Fatalf("unknown discriminant %d returned status %q and no error, want a fail-closed error", uint(next), status)
	}
	if status != "" {
		t.Fatalf("unknown discriminant %d returned status %q, want the zero Status", uint(next), status)
	}
}

func TestPatalaWebhookStatusName(t *testing.T) {
	if got := patalaWebhookStatusName(patalaWebhookUnconfirmed); got != "Unconfirmed" {
		t.Fatalf("name of Unconfirmed = %q, want %q", got, "Unconfirmed")
	}
	if got := patalaWebhookStatusName(patalaWebhookStatus(99)); got != "unknown(99)" {
		t.Fatalf("name of an undeclared discriminant = %q, want %q", got, "unknown(99)")
	}
}
