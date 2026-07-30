package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vul-os/cackle/internal/store/keyvault"
)

// ErrKeyVaultLocked is returned by every operation that would need an event's
// PRIVATE signing key while no key material has been supplied. It is the
// fail-closed path: there is deliberately no mode in which these operations
// succeed by writing or reading a plaintext private key instead.
//
// Operations that only need PUBLIC key material — building a scan bundle,
// building a KeyRing, verifying a ticket — never return this, because a
// process that only serves gates never needs the vault open. See
// ActiveEventKeys.
var ErrKeyVaultLocked = errors.New("store: event key vault is locked: set CACKLE_KEY_PASSPHRASE (or CACKLE_KEY_FILE) — event signing keys are encrypted at rest and cannot be created, read or used without it")

// ErrKeyVaultUninitialised means the database has no key_vault row: key
// material has never been configured for it. Unlock initialises one, so this
// is only returned by read-only status calls.
var ErrKeyVaultUninitialised = errors.New("store: event key vault has not been initialised for this database")

// dekRowID is the single key_vault row id.
const dekRowID = "dek"

// dekAAD binds the wrapped DEK to its purpose, so a wrapped DEK cannot be
// replayed as some other ciphertext.
var dekAAD = []byte("cackle.keyvault.dek.v1")

// keyVault is the Store's unlocked-vault state. Nil vault means locked.
type keyVaultState struct {
	mu    sync.RWMutex
	vault *keyvault.Vault
}

func (k *keyVaultState) get() *keyvault.Vault {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.vault
}

func (k *keyVaultState) set(v *keyvault.Vault) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.vault = v
}

// KeyVaultStatus is the non-secret view of a database's vault: enough for an
// operator (or a boot log line) to know what material this database expects
// and whether anything is still unencrypted, and nothing more. It contains no
// key material, no ciphertext and no salt.
//
// This mirrors SlipScan's rule that a vault exposes metadata only: secrets
// can be set, rotated, revoked and USED, never viewed. Cackle has no
// "show me the private key" path either — the only way private key material
// leaves internal/store is as a signature over a ticket.
type KeyVaultStatus struct {
	// Initialised reports whether a key_vault row exists at all.
	Initialised bool
	// SourceKind is what material this database was sealed with:
	// "passphrase", "keyfile", or "demo".
	SourceKind string
	// KDF is the derivation used, e.g. "argon2id".
	KDF string
	// Unlocked reports whether THIS process currently holds the DEK.
	Unlocked bool
	// LegacyPlaintextKeys counts event_keys rows whose private half is
	// still the pre-0004 plaintext. Non-zero means the data half of the
	// migration has not run (or could not).
	LegacyPlaintextKeys int
	CreatedAt           time.Time
	RotatedAt           *time.Time
}

// KeyVaultStatus reports the vault's metadata. Safe to log in full.
func (s *Store) KeyVaultStatus(ctx context.Context) (KeyVaultStatus, error) {
	st := KeyVaultStatus{Unlocked: s.KeyVaultUnlocked()}

	legacy, err := s.LegacyPlaintextKeyCount(ctx)
	if err != nil {
		return st, err
	}
	st.LegacyPlaintextKeys = legacy

	row, err := s.loadDEKRow(ctx, s.db)
	if errors.Is(err, ErrKeyVaultUninitialised) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	st.Initialised = true
	st.SourceKind = row.sourceKind
	st.KDF = row.kdf
	st.CreatedAt = row.createdAt
	st.RotatedAt = row.rotatedAt
	return st, nil
}

// KeyVaultUnlocked reports whether this process holds the unwrapped DEK.
func (s *Store) KeyVaultUnlocked() bool { return s.keys.get() != nil }

// UnlockKeyVault derives this database's key-encryption key from src and
// unwraps the data-encryption key, after which event signing keys can be
// created and used. On a database that has never had a vault (a fresh
// install, or one that predates migration 0004), it initialises one: a fresh
// random DEK wrapped under src.
//
// It fails, rather than falling back to anything, when:
//   - src holds no material (keyvault.ErrNoSource) — an unset or blank
//     passphrase is refused at the Source constructor, not here, so there is
//     no path through this function that proceeds without material;
//   - the database was sealed with a different KIND of material than src;
//   - src does not match the material the database was sealed with
//     (keyvault.ErrWrongKey).
//
// Calling it twice is fine; the second call re-derives and replaces the
// in-memory vault.
func (s *Store) UnlockKeyVault(ctx context.Context, src keyvault.Source) error {
	if !src.Valid() {
		return fmt.Errorf("store: unlock key vault: %w", keyvault.ErrNoSource)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: unlock key vault: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	row, err := s.loadDEKRow(ctx, tx)
	switch {
	case errors.Is(err, ErrKeyVaultUninitialised):
		vault, err := initDEK(ctx, tx, src)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: unlock key vault: commit: %w", err)
		}
		s.keys.set(vault)
		return nil
	case err != nil:
		return err
	}

	if row.sourceKind != src.Kind() {
		return fmt.Errorf("store: unlock key vault: %w", mismatchedKindError(row.sourceKind, src.Kind()))
	}

	kek, err := src.DeriveKEK(row.params())
	if err != nil {
		return fmt.Errorf("store: unlock key vault: derive: %w", err)
	}
	dek, err := keyvault.Open(kek, dekAAD, row.nonce, row.wrapped)
	if err != nil {
		if errors.Is(err, keyvault.ErrWrongKey) {
			return fmt.Errorf("store: unlock key vault: %w (this database was sealed with a different %s)", keyvault.ErrWrongKey, row.sourceKind)
		}
		return fmt.Errorf("store: unlock key vault: unwrap: %w", err)
	}
	vault, err := keyvault.NewVault(dek, row.sourceKind)
	if err != nil {
		return fmt.Errorf("store: unlock key vault: %w", err)
	}
	s.keys.set(vault)
	return nil
}

// RewrapKeyVault rotates the operator's key material: it unwraps the DEK with
// old and re-wraps it under new, with fresh KDF parameters and a fresh salt.
//
// Every sealed event key is untouched, because the DEK does not change — so
// this cannot invalidate a single issued ticket, and it never decrypts an
// event key at all. That is the reason for the envelope: passphrase rotation
// is one row, not a re-encryption pass over the crown jewels.
//
// The old wrapping is overwritten in place; there is no history and no way
// back to the previous passphrase. Same discipline as SlipScan's
// Vault::replace.
func (s *Store) RewrapKeyVault(ctx context.Context, current, replacement keyvault.Source) error {
	if !current.Valid() || !replacement.Valid() {
		return fmt.Errorf("store: rewrap key vault: %w", keyvault.ErrNoSource)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: rewrap key vault: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	row, err := s.loadDEKRow(ctx, tx)
	if err != nil {
		return err
	}
	if row.sourceKind != current.Kind() {
		return fmt.Errorf("store: rewrap key vault: %w", mismatchedKindError(row.sourceKind, current.Kind()))
	}

	currentKEK, err := current.DeriveKEK(row.params())
	if err != nil {
		return fmt.Errorf("store: rewrap key vault: derive current: %w", err)
	}
	dek, err := keyvault.Open(currentKEK, dekAAD, row.nonce, row.wrapped)
	if err != nil {
		return fmt.Errorf("store: rewrap key vault: current material does not match: %w", err)
	}

	params, err := keyvault.NewKDFParams(replacement.Kind())
	if err != nil {
		return fmt.Errorf("store: rewrap key vault: %w", err)
	}
	replacementKEK, err := replacement.DeriveKEK(params)
	if err != nil {
		return fmt.Errorf("store: rewrap key vault: derive replacement: %w", err)
	}
	wrapped, nonce, err := keyvault.Seal(replacementKEK, dekAAD, dek)
	if err != nil {
		return fmt.Errorf("store: rewrap key vault: wrap: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE key_vault
		SET wrapped_key = ?, nonce = ?, kdf = ?, salt = ?,
		    argon_time = ?, argon_memory = ?, argon_lanes = ?,
		    source_kind = ?, rotated_at = ?
		WHERE id = ?`,
		wrapped, nonce, params.Name, params.Salt,
		params.Time, params.Memory, params.Lanes,
		replacement.Kind(), timeToText(time.Now()), dekRowID,
	); err != nil {
		return fmt.Errorf("store: rewrap key vault: update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: rewrap key vault: commit: %w", err)
	}

	vault, err := keyvault.NewVault(dek, replacement.Kind())
	if err != nil {
		return fmt.Errorf("store: rewrap key vault: %w", err)
	}
	s.keys.set(vault)
	return nil
}

// initDEK mints and wraps a fresh DEK for a database that has none.
func initDEK(ctx context.Context, tx *sql.Tx, src keyvault.Source) (*keyvault.Vault, error) {
	params, err := keyvault.NewKDFParams(src.Kind())
	if err != nil {
		return nil, fmt.Errorf("store: init key vault: %w", err)
	}
	kek, err := src.DeriveKEK(params)
	if err != nil {
		return nil, fmt.Errorf("store: init key vault: derive: %w", err)
	}
	dek, err := keyvault.GenerateDEK()
	if err != nil {
		return nil, fmt.Errorf("store: init key vault: %w", err)
	}
	wrapped, nonce, err := keyvault.Seal(kek, dekAAD, dek)
	if err != nil {
		return nil, fmt.Errorf("store: init key vault: wrap: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO key_vault
			(id, wrapped_key, nonce, kdf, salt, argon_time, argon_memory, argon_lanes, source_kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dekRowID, wrapped, nonce, params.Name, params.Salt,
		params.Time, params.Memory, params.Lanes, src.Kind(), timeToText(time.Now()),
	); err != nil {
		return nil, fmt.Errorf("store: init key vault: insert: %w", err)
	}
	vault, err := keyvault.NewVault(dek, src.Kind())
	if err != nil {
		return nil, fmt.Errorf("store: init key vault: %w", err)
	}
	return vault, nil
}

// dekRow is the stored wrapping of the DEK.
type dekRow struct {
	wrapped     []byte
	nonce       []byte
	kdf         string
	salt        []byte
	argonTime   uint32
	argonMemory uint32
	argonLanes  uint8
	sourceKind  string
	createdAt   time.Time
	rotatedAt   *time.Time
}

func (r dekRow) params() keyvault.KDFParams {
	return keyvault.KDFParams{
		Name:   r.kdf,
		Salt:   r.salt,
		Time:   r.argonTime,
		Memory: r.argonMemory,
		Lanes:  r.argonLanes,
	}
}

// querier is satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) loadDEKRow(ctx context.Context, q querier) (dekRow, error) {
	var r dekRow
	var createdAt string
	var rotatedAt sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT wrapped_key, nonce, kdf, salt, argon_time, argon_memory, argon_lanes,
		       source_kind, created_at, rotated_at
		FROM key_vault WHERE id = ?`, dekRowID,
	).Scan(&r.wrapped, &r.nonce, &r.kdf, &r.salt, &r.argonTime, &r.argonMemory, &r.argonLanes,
		&r.sourceKind, &createdAt, &rotatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return dekRow{}, ErrKeyVaultUninitialised
	}
	if err != nil {
		return dekRow{}, fmt.Errorf("store: load key vault: %w", err)
	}
	if r.createdAt, err = textToTime(createdAt); err != nil {
		return dekRow{}, fmt.Errorf("store: parse key vault created_at: %w", err)
	}
	if r.rotatedAt, err = textToNullTime(rotatedAt); err != nil {
		return dekRow{}, fmt.Errorf("store: parse key vault rotated_at: %w", err)
	}
	return r, nil
}

// mismatchedKindError explains a kind mismatch in terms an operator can act
// on. The demo cases matter most: they are the ones where "it worked
// yesterday" has a security-relevant explanation.
func mismatchedKindError(stored, given string) error {
	switch {
	case stored == keyvault.KindDemo:
		return fmt.Errorf("this database's event keys were sealed with the PUBLIC --demo vault key, which is a constant in the Cackle source and protects nothing; it must not be promoted to a real deployment. Create a fresh database and re-issue, or run it with --demo only")
	case given == keyvault.KindDemo:
		return fmt.Errorf("this database expects an operator %s, so --demo cannot unseal it; point --demo at a different CACKLE_DB", stored)
	default:
		return fmt.Errorf("this database was sealed with a %s but a %s was supplied; supply the %s it was sealed with (or rotate deliberately)", stored, given, stored)
	}
}
