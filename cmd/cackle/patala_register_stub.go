//go:build !patala

package main

import "github.com/vul-os/cackle/internal/payments"

// registerPatalaProviders is a no-op in Cackle's DEFAULT build: pure Go,
// CGO_ENABLED=0, `manual` (and, while it's still native, `paystack`) as the
// only payment providers — see internal/payments/patala.go and
// docs/PAYMENTS.md "The patala path" for what building with `-tags patala`
// adds instead (every processor patala-fiat ships: Stripe, Adyen, BTCPay,
// lnbits, and 17 more, via patala-go's cgo binding).
//
// This stub exists so main.go can call registerPatalaProviders
// unconditionally, in both builds, without main.go itself needing a build
// tag or importing anything cgo-shaped — see patala_register.go (the
// `-tags patala` counterpart, same signature, real implementation) for the
// other half of this pair. This is also why the offline gate / scanner
// (internal/tickets, internal/scan) and the default `make build`/`make
// test` targets are entirely unaffected by patala existing at all: nothing
// in the default build graph ever imports patala-go.
func registerPatalaProviders(_ *payments.Registry, _ payments.RecordStore) error {
	return nil
}
