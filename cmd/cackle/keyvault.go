package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/store"
	"github.com/vul-os/cackle/internal/store/keyvault"
)

// openKeyVault is the boot-time gate on Cackle's most dangerous secret: the
// per-event Ed25519 issuer private keys. It unlocks the vault from
// operator-supplied material, then completes the data half of migration 0004
// (sealing any key still stored as pre-0004 plaintext).
//
// It FAILS CLOSED, in this order:
//
//  1. No key material configured, and the database already holds keys (or is
//     about to need one) → refuse to start, naming the variable to set. There
//     is no plaintext mode to fall back to; that mode is what this change
//     removed.
//  2. No key material configured, and the database still holds pre-0004
//     PLAINTEXT keys → refuse to start, and say explicitly that the upgrade
//     cannot proceed without it. A half-migrated database — some keys sealed,
//     some not — is never produced.
//  3. Material configured but wrong for this database → refuse to start.
//
// The one case that needs no material is --demo, which seals its throwaway
// keys with a PUBLIC constant and says so in the log. See keyvault.DemoSource.
func openKeyVault(ctx context.Context, st *store.Store, cfg *config.Config, logger *slog.Logger) error {
	status, err := st.KeyVaultStatus(ctx)
	if err != nil {
		return fmt.Errorf("read key vault status: %w", err)
	}

	src, origin, err := chooseKeySource(cfg, status)
	if err != nil {
		return err
	}

	if err := st.UnlockKeyVault(ctx, src); err != nil {
		return fmt.Errorf("unlock event key vault (%s): %w", origin, err)
	}
	src.Zero()

	sealed, err := st.SealLegacyEventKeys(ctx)
	if err != nil {
		return fmt.Errorf("seal pre-0004 plaintext event keys: %w", err)
	}
	if sealed > 0 {
		logger.Warn("migrated plaintext event signing keys to encrypted storage",
			"keys_sealed", sealed,
			"reminder", "every backup or snapshot taken before this upgrade still contains PLAINTEXT signing keys; treat them as key material and rotate if any may have leaked")
	}

	// Re-read: proves the refusal path cannot be reached by a partially
	// completed migration, and gives the boot log an unambiguous statement of
	// where this database stands.
	status, err = st.KeyVaultStatus(ctx)
	if err != nil {
		return fmt.Errorf("re-read key vault status: %w", err)
	}
	if status.LegacyPlaintextKeys > 0 {
		return fmt.Errorf("refusing to serve: %d event signing key(s) are still stored as plaintext after the migration ran; this is a bug, do not expose this instance", status.LegacyPlaintextKeys)
	}

	logger.Info("event key vault unlocked",
		"source", status.SourceKind,
		"from", origin,
		"kdf", status.KDF,
		"keys_sealed_this_boot", sealed)

	if status.SourceKind == keyvault.KindDemo {
		logger.Warn("DEMO KEY VAULT: this database's event signing keys are sealed with a PUBLIC constant from the Cackle source, which protects nothing",
			"consequence", "anyone with this file can mint valid tickets for its events",
			"action", "never use this database for a real event; a non-demo boot will refuse to open it")
	}

	return nil
}

// chooseKeySource decides what material to unlock with, and produces the
// operator-facing refusal when there is none.
func chooseKeySource(cfg *config.Config, status store.KeyVaultStatus) (keyvault.Source, string, error) {
	if cfg.HasKeySource() {
		return cfg.KeySource, cfg.KeySourceOrigin, nil
	}

	// Demo mode is the only path with no operator material, and only when the
	// operator has supplied none — an explicit passphrase always wins, even
	// under --demo, so a real database can be demoed against safely.
	if cfg.Demo {
		return keyvault.DemoSource(), "--demo (public demo key)", nil
	}

	return keyvault.Source{}, "", errors.New(noKeyMaterialMessage(status))
}

// noKeyMaterialMessage is the message an operator sees when they start Cackle
// with no key material. It is long on purpose: this is the one error where
// being terse would push someone toward looking for the plaintext mode that
// used to exist.
func noKeyMaterialMessage(status store.KeyVaultStatus) string {
	const how = `configure ONE of:

  CACKLE_KEY_PASSPHRASE=...            an operator passphrase (at least 12 characters)
  CACKLE_KEY_PASSPHRASE_FILE=/path     the same, read from a file (docker/systemd secrets)
  CACKLE_KEY_FILE=/path                32+ bytes of random key material
                                       (head -c 32 /dev/urandom > /etc/cackle/keyfile)

Keep it somewhere you will still have it after the server is gone: without it
the event signing keys in %s cannot be decrypted, and no NEW ticket can be
issued for any existing event. Already-issued tickets keep working — gates
verify with public keys only — and so does every gate, online or offline.

There is no plaintext mode. Storing these keys unencrypted is what this
release removed; see docs/SELF-HOSTING.md.`

	switch {
	case status.LegacyPlaintextKeys > 0:
		// Be exact about what has and has not happened. The SCHEMA migration
		// has already run by the time this message is produced (store.Open
		// applies migrations), so claiming "nothing has been changed" would be
		// false and would leave an operator wondering whether to restore. What
		// is true — and what they need to know — is that no key was touched.
		return fmt.Sprintf(`refusing to start: %d event signing key(s) in this database are stored as PLAINTEXT and must be encrypted before this version will serve them.

No key has been touched. The schema migration has been applied, every key is
still exactly as it was, and this will not half-migrate them: either all of
them get encrypted or none do. To finish the upgrade, back up the database,
then `+how, status.LegacyPlaintextKeys, "CACKLE_DB")

	case status.Initialised:
		return `refusing to start: this database's event signing keys are encrypted at rest and no key material was supplied to unlock them.

Nothing can be signed without it. To unlock, ` + fmt.Sprintf(how, "CACKLE_DB")

	default:
		return `refusing to start: no key material configured for event signing keys.

Every event Cackle creates gets its own Ed25519 signing key, and those keys are
encrypted at rest — so creating an event (and therefore selling a ticket) needs
key material before the first one exists. To provide it, ` + fmt.Sprintf(how, "CACKLE_DB")
	}
}
