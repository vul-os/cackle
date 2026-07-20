package payments

import (
	"errors"
	"io"
)

// This file is small shared HTTP-safety infrastructure used by the
// regional-processor adapters in this package (flutterwave.go, xendit.go,
// midtrans.go, razorpay.go, payu.go, mercadopago.go, dlocal.go, mpesa.go,
// yoco.go, payfast.go, iyzico.go). It intentionally does NOT reuse
// paystack.go's private paystackReadLimited (kept scoped to Paystack so
// that adapter's existing tests/sentinel error are untouched) nor
// provider.go's readLimited (owned by a sibling agent building the v2
// Provider seam, whose readLimitedReader dependency may still be
// in-flight) — this is an independent, self-contained copy so this file's
// adapters do not take on a build-order dependency on either.

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
