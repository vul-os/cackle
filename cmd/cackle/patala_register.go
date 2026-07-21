//go:build patala

package main

import (
	"fmt"

	"github.com/vul-os/cackle/internal/payments"
)

// registerPatalaProviders wires up every patala-fiat processor this binary
// was compiled against (see internal/payments/patala.go's build comment for
// the full recipe: `-tags patala`, CGO_ENABLED=1, a C toolchain, and the
// sibling patala repo's Go bindings generated with
// `make FEATURES=fiat-all generate`) that this deployment has actually
// configured, by name, via CACKLE_<PROVIDER>_* environment variables — the
// exact same variable names every removed native adapter used to read (see
// docs/PAYMENTS.md's adapter table), so an operator moving onto this path
// does not need to rename anything.
//
// payments.ProviderNameManual is deliberately excluded here: manual is
// Cackle's own native, always-registered provider (see the unconditional
// payments.NewManualWithStore call in run(), above) and never goes through
// patala at all — see docs/PAYMENTS.md "manual stays native" and
// patala.go's own doc comment on why (patala's generic FFI surface can't
// drive manual's MarkPaid/MarkFailed operator actions, and manual needs no
// network/cgo/patala in the first place).
//
// A provider with NO CACKLE_<NAME>_* variables set at all is treated as
// "the operator hasn't configured this one" and silently skipped, mirroring
// this file's own former cfg.PaystackSecretKey != "" gate in main.go. Once
// at least one such variable IS set, any error from payments.NewPatalaFiat
// (a missing REQUIRED key, a malformed numeric field, an unsupported
// provider name, a config error patala-fiat itself rejects, ...) is a real
// misconfiguration and fails startup loudly — the same convention every
// other provider registered in run() already follows.
func registerPatalaProviders(registry *payments.Registry, recordStore payments.RecordStore) error {
	for _, name := range payments.PatalaFiatProviderNames() {
		if name == payments.ProviderNameManual {
			continue
		}
		if len(payments.PatalaConfigFromEnv(name)) == 0 {
			continue // not configured -- see doc comment above
		}
		p, err := payments.NewPatalaFiat(name, recordStore)
		if err != nil {
			return fmt.Errorf("configure patala %s: %w", name, err)
		}
		if err := registry.Register(p); err != nil {
			return fmt.Errorf("configure patala %s: %w", name, err)
		}
	}
	return nil
}
