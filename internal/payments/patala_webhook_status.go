package payments

import "fmt"

// This file deliberately carries NO `//go:build patala` tag, unlike
// patala.go which it serves.
//
// The one thing in the patala webhook path that can mark an order paid is
// the mapping from patala's `WebhookStatus` onto Cackle's own Status, and
// getting it wrong marks unpaid orders as paid. Keeping that mapping (and
// the discriminants it switches on) in an untagged file means the default
// `go test ./internal/...` run — the one CI, contributors, and every
// pre-commit check actually execute — pins it, instead of it only being
// checked in the opt-in, cgo-only `-tags patala` build almost nobody runs.
// Nothing here imports patala-go; it is plain Go over a uint.
//
// patala.go (tagged) is the only caller. It asserts AT COMPILE TIME that
// each constant below still equals the generated binding's own
// patala.WebhookStatus* value, so a binding regeneration that renumbered
// the enum breaks the build rather than silently re-pointing this mapping
// — and patala_test.go (tagged) additionally asserts the generated
// bindings declare no variant this file does not know about.

// patalaWebhookStatus mirrors `patala_core::WebhookStatus` as the UniFFI Go
// binding represents it (`type WebhookStatus uint`, discriminants numbered
// from 1 in declaration order). Three states, never a bool — see the core
// type's own docs for why "the rail says this did not settle" and "the rail
// cannot say" must not collapse into one.
type patalaWebhookStatus uint

const (
	// patalaWebhookSettled: the rail established that the payment named by
	// this delivery has settled. The ONLY variant that may ever become
	// StatusPaid.
	patalaWebhookSettled patalaWebhookStatus = 1
	// patalaWebhookNotSettled: the delivery is authentic and the rail
	// established the payment has NOT settled — which in patala's own
	// wording covers "still pending, failed, cancelled, expired"
	// indiscriminately.
	patalaWebhookNotSettled patalaWebhookStatus = 2
	// patalaWebhookUnconfirmed: the delivery is authentic but carries no
	// settlement claim at all (BTCPay, Coinbase Commerce, OpenNode, LNbits
	// and Mollie all sign a notification that names an object and nothing
	// else). patala_core's docs are explicit: **never treat this as
	// settlement.**
	patalaWebhookUnconfirmed patalaWebhookStatus = 3
)

// patalaWebhookStatusNames is the human-readable name of each variant, used
// in error text. Its key set is also the authoritative "every variant this
// build knows about" list that patalaWebhookStatusToCackle must cover and
// that both test suites count against.
var patalaWebhookStatusNames = map[patalaWebhookStatus]string{
	patalaWebhookSettled:     "Settled",
	patalaWebhookNotSettled:  "NotSettled",
	patalaWebhookUnconfirmed: "Unconfirmed",
}

// patalaWebhookStatusToCackle maps one patala WebhookStatus onto Cackle's
// settlement Status, failing closed on anything it does not recognise.
//
// The mapping, and why each one is what it is:
//
//   - Settled -> StatusPaid. The one variant patala_core says a caller may
//     gate entitlement on (`WebhookEvent::is_settled` is `status ==
//     Settled` and nothing else) — and even then only after reconciling the
//     reported amount/currency against Cackle's own stored order, which
//     payments.Reconcile does for every provider.
//
//   - NotSettled -> StatusPending, NOT StatusFailed. patala flattens "still
//     pending", "failed", "cancelled" and "expired" into this one variant,
//     so it does not carry enough information to move an order to Cackle's
//     TERMINAL StatusFailed; doing that would kill orders whose buyer is
//     still mid-checkout. Pending is the honest, non-destructive reading:
//     "not settled, and this seam cannot say whether it ever will be" —
//     keep polling Verify.
//
//   - Unconfirmed -> StatusPending. **This is the load-bearing one.** An
//     Unconfirmed delivery is authentic, which is exactly what makes it
//     tempting to treat as good news; patala_core's own docs say it
//     asserts nothing whatsoever about money and must never be treated as
//     settlement. Mapping it to StatusPaid would issue tickets for orders
//     nobody has paid for, on every signature-only rail (BTCPay, Coinbase
//     Commerce, OpenNode, LNbits, Mollie). See
//     TestPatalaWebhookStatusToCackle_PinsEveryVariant.
//
// An unrecognised discriminant is an error, never a Status — a binding that
// grew a fourth variant must stop this path dead rather than have it guess.
// The returned Status is the zero value in that case, which is not
// StatusPaid either, so even a caller that ignored the error fails closed.
func patalaWebhookStatusToCackle(ws patalaWebhookStatus) (Status, error) {
	switch ws {
	case patalaWebhookSettled:
		return StatusPaid, nil
	case patalaWebhookNotSettled:
		return StatusPending, nil
	case patalaWebhookUnconfirmed:
		return StatusPending, nil
	default:
		return "", fmt.Errorf("payments: patala: unknown WebhookStatus discriminant %d — this build's mapping covers %d variants (%v); regenerate the bindings and update patala_webhook_status.go before trusting this path",
			uint(ws), len(patalaWebhookStatusNames), patalaWebhookStatusNames)
	}
}

// patalaWebhookStatusName renders ws for error/log text, never panicking on
// an unknown discriminant.
func patalaWebhookStatusName(ws patalaWebhookStatus) string {
	if name, ok := patalaWebhookStatusNames[ws]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", uint(ws))
}
