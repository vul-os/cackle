package payments

import (
	"errors"
	"io"
	"time"
)

// This file is small shared HTTP-safety infrastructure. It used to back a
// whole family of regional-processor adapters (flutterwave.go, xendit.go,
// midtrans.go, razorpay.go, payu.go, mercadopago.go, yoco.go, payfast.go,
// iyzico.go, and the crypto adapters btcpay.go/lnbits.go/opennode.go/
// coinbasecommerce.go) that have since moved out to the patala substrate
// (see docs/PAYMENTS.md "The patala path" and internal/payments/patala.go)
// — stablecoin.go (never ported to patala; see its own doc comment) and
// paystack.go (stays native for its orgs.BankingProvider payout methods —
// see paystack.go's doc comment) are this file's only remaining consumers.
// It intentionally does NOT reuse paystack.go's private
// paystackReadLimited (kept scoped to Paystack so that adapter's existing
// tests/sentinel error are untouched) nor provider.go's own read-limiting
// — this is an independent, self-contained copy so callers of this file
// don't take on a build-order dependency on either.

// cryptoDefaultHTTPTimeout bounds every outbound call the crypto adapter
// still in this package (stablecoin.go) makes, regardless of the caller's
// own context deadline. Moved here from btcpay.go (removed — see above)
// since stablecoin.go is now its only user.
const cryptoDefaultHTTPTimeout = 20 * time.Second

// cryptoMaxBodyBytes caps every HTTP body (API responses and incoming
// webhook bodies alike) stablecoin.go reads, via boundedRead below. Moved
// here from btcpay.go for the same reason as cryptoDefaultHTTPTimeout.
const cryptoMaxBodyBytes int64 = 1 << 20 // 1 MiB

// errBoundedReadTooLarge is returned by boundedRead when the input
// exceeds the given limit. Every adapter that calls boundedRead should
// map this (via errors.Is) onto its own provider-specific
// "response too large" / "webhook body too large" sentinel error, so
// error messages stay consistent with that adapter's own naming.
var errBoundedReadTooLarge = errors.New("payments: body exceeds size limit")

// boundedRead reads at most limit bytes from r, failing closed with
// errBoundedReadTooLarge if there was more rather than silently
// truncating. Every adapter in this package must route provider API
// response bodies AND incoming webhook request bodies through this (or an
// equivalent) before any json.Unmarshal or signature check, so a
// malicious or misbehaving endpoint can never force an unbounded read.
func boundedRead(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errBoundedReadTooLarge
	}
	return b, nil
}
