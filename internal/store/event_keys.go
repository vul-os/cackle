package store

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vul-os/cackle/internal/store/keyvault"
)

// EventKey is one event's Ed25519 issuer keypair, as persisted in
// event_keys. Every event gets its OWN key, generated at creation time —
// see CreateEventWithKey in events.go. There is never a global signing key.
//
// PrivateKey is populated on exactly one read path: LatestActiveEventKey,
// which requires an unlocked vault. The list methods (ListEventKeys,
// ActiveEventKeys) leave it nil ON PURPOSE — see their doc comments. If you
// are holding an EventKey and PrivateKey is nil, that is not a bug, it is the
// list path declining to decrypt something it was not asked to decrypt.
type EventKey struct {
	ID         string
	EventID    string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// eventKeyAAD binds a sealed private key to the event_keys row it belongs to.
// Moving a ciphertext to another row (or another event) makes it fail to
// authenticate rather than silently decrypt under the wrong identity.
func eventKeyAAD(keyID, eventID string) []byte {
	return []byte("cackle.event_key.v1|" + keyID + "|" + eventID)
}

// sealPrivateKey encrypts priv for storage under the row identified by
// keyID/eventID. It refuses when the vault is locked: this is where "no
// passphrase configured" becomes "no key gets created", with no branch that
// stores plaintext instead.
func (s *Store) sealPrivateKey(keyID, eventID string, priv ed25519.PrivateKey) (sealed, nonce []byte, err error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, nil, fmt.Errorf("store: seal event key: private key is %d bytes, want %d", len(priv), ed25519.PrivateKeySize)
	}
	vault := s.keys.get()
	if vault == nil {
		return nil, nil, ErrKeyVaultLocked
	}
	sealed, nonce, err = vault.Seal(eventKeyAAD(keyID, eventID), priv)
	if err != nil {
		return nil, nil, fmt.Errorf("store: seal event key: %w", err)
	}
	return sealed, nonce, nil
}

// openPrivateKey is the inverse of sealPrivateKey.
func (s *Store) openPrivateKey(keyID, eventID string, sealed, nonce []byte) (ed25519.PrivateKey, error) {
	vault := s.keys.get()
	if vault == nil {
		return nil, ErrKeyVaultLocked
	}
	priv, err := vault.Open(eventKeyAAD(keyID, eventID), nonce, sealed)
	if err != nil {
		return nil, fmt.Errorf("store: open event key %s: %w", keyID, err)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("store: open event key %s: decrypted %d bytes, want %d", keyID, len(priv), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(priv), nil
}

// CreateEventKey inserts an additional issuer key for an event (key
// rotation). Event creation itself always goes through
// CreateEventWithKey, so this is only needed for rotating/adding keys to
// an existing event.
//
// Adding a key does NOT invalidate anything already issued: tickets carry the
// `kid` they were signed with, and every non-revoked key stays in the KeyRing
// gates pin (see ActiveEventKeys and events.Service.IssuerPublicKeys). What
// voids already-issued tickets is REVOKING the key they were signed with —
// see RevokeEventKey.
//
// Fails with ErrKeyVaultLocked if no key material is configured.
func (s *Store) CreateEventKey(ctx context.Context, k *EventKey) error {
	if k.ID == "" {
		k.ID = NewID()
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now()
	}
	sealed, nonce, err := s.sealPrivateKey(k.ID, k.EventID, k.PrivateKey)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO event_keys (id, event_id, public_key, sealed_private_key, sealed_nonce, created_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.EventID, []byte(k.PublicKey), sealed, nonce, timeToText(k.CreatedAt), nullTimeToText(k.RevokedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create event key: %w", err)
	}
	return nil
}

// ListEventKeys returns every issuer key ever generated for an event,
// including revoked ones, oldest first.
//
// PUBLIC HALVES ONLY: every returned EventKey has a nil PrivateKey, and this
// call works fine with the vault locked. Nothing that builds a scan bundle or
// a KeyRing needs private material, so nothing that builds one needs a
// passphrase.
func (s *Store) ListEventKeys(ctx context.Context, eventID string) ([]EventKey, error) {
	return s.queryEventKeys(ctx, `
		SELECT id, event_id, public_key, created_at, revoked_at
		FROM event_keys WHERE event_id = ? ORDER BY created_at ASC`, eventID)
}

// ActiveEventKeys returns every non-revoked issuer key for an event, oldest
// first. In the common case (no rotation) this is exactly one key — this
// is what a scanner's pinned KeyRing should be built from.
//
// PUBLIC HALVES ONLY, same as ListEventKeys: PrivateKey is nil on every
// returned key and the vault may be locked. This is the property that keeps
// the offline gate story unchanged by encryption at rest — the bundle a gate
// downloads is built from these rows, and a process that can build one still
// holds nothing that can sign a ticket.
func (s *Store) ActiveEventKeys(ctx context.Context, eventID string) ([]EventKey, error) {
	return s.queryEventKeys(ctx, `
		SELECT id, event_id, public_key, created_at, revoked_at
		FROM event_keys WHERE event_id = ? AND revoked_at IS NULL ORDER BY created_at ASC`, eventID)
}

// LatestActiveEventKey returns the most recently created non-revoked key
// for an event — the key new tickets should be signed with. Returns
// ErrNotFound if the event has no active key (should not happen for any
// event created via CreateEventWithKey and never revoked into oblivion).
//
// This is the ONLY read path that returns private key material, and it
// therefore requires an unlocked vault: with no key material configured it
// returns ErrKeyVaultLocked and no ticket can be signed. A row still holding
// pre-0004 plaintext returns ErrEventKeyNotSealed rather than serving the
// plaintext — the boot-time migration is not optional.
//
// `, id DESC` IS LOAD-BEARING, not tidiness. created_at is written by
// timeToText (store.go:207) in RFC3339, which has whole-second resolution, so
// a key rotated in the same second as the one it replaces stores a
// BYTE-IDENTICAL created_at and `ORDER BY created_at DESC` alone has nothing
// left to choose with. What it returned then was an artifact of the query
// plan — today's temp-b-tree sort hands back the OLDER key, and adding an
// index SQLite can walk in order flips it to the newer one.
//
// Ordering by id recovers the real answer because ids come from NewID() =
// ulid.Make(), whose entropy source is a LockedMonotonicReader: a
// millisecond-resolution timestamp, plus entropy that strictly increases
// within a millisecond. Two keys made by one process therefore always sort by
// id in true creation order, however close together. Two keys made by
// DIFFERENT processes inside the same millisecond sort by entropy rather than
// by time — still a stable total order every plan agrees on, which is what
// LIMIT 1 needs. See TestLatestActiveEventKeyBreaksSameSecondTies.
func (s *Store) LatestActiveEventKey(ctx context.Context, eventID string) (*EventKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, event_id, public_key, sealed_private_key, sealed_nonce, created_at, revoked_at
		FROM event_keys WHERE event_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC, id DESC LIMIT 1`, eventID)
	return s.scanEventKeyWithPrivate(row)
}

// ErrEventKeyNotSealed means an event_keys row still holds its pre-0004
// plaintext private key. Reading it is refused: serving from the plaintext
// column would make the encryption cosmetic and would let a deployment run
// indefinitely without ever completing the migration.
var ErrEventKeyNotSealed = errors.New("store: event key is still stored as pre-0004 plaintext: restart with key material configured so the boot migration can seal it")

// RevokeEventKey marks a key revoked at t. It does not delete the row —
// old tickets signed with a revoked key remain cryptographically
// verifiable; revocation only affects whether future KeyRings include it.
//
// Read that sentence precisely, because it is the sharp edge of rotation: the
// SIGNATURE stays valid forever, but a gate that re-pulls its bundle no longer
// carries the key to check it against, and answers ErrUnknownKID — a refusal
// at the door. Revoking the key an in-circulation ticket was signed with
// voids that ticket. See docs/TICKET-FORMAT.md.
func (s *Store) RevokeEventKey(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE event_keys SET revoked_at = ? WHERE id = ?`, timeToText(at), id)
	if err != nil {
		return fmt.Errorf("store: revoke event key: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// scanEventKeyWithPrivate scans a row that includes the sealed columns and
// decrypts the private half. A NULL sealed_private_key can only mean the row
// is still legacy plaintext (the 0004 CHECK constraint permits no other
// reading of it), which is refused.
func (s *Store) scanEventKeyWithPrivate(row *sql.Row) (*EventKey, error) {
	var k EventKey
	var pub, sealed, nonce []byte
	var createdAt string
	var revokedAt sql.NullString

	err := row.Scan(&k.ID, &k.EventID, &pub, &sealed, &nonce, &createdAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan event key: %w", err)
	}
	k.PublicKey = ed25519.PublicKey(pub)
	if k.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse event key created_at: %w", err)
	}
	if k.RevokedAt, err = textToNullTime(revokedAt); err != nil {
		return nil, fmt.Errorf("store: parse event key revoked_at: %w", err)
	}
	if len(sealed) == 0 {
		return nil, fmt.Errorf("%w (key %s)", ErrEventKeyNotSealed, k.ID)
	}
	if k.PrivateKey, err = s.openPrivateKey(k.ID, k.EventID, sealed, nonce); err != nil {
		return nil, err
	}
	return &k, nil
}

// queryEventKeys scans public-only rows. The SELECT list it is given must not
// include the sealed or legacy columns; there is no decryption here.
func (s *Store) queryEventKeys(ctx context.Context, query string, args ...any) ([]EventKey, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list event keys: %w", err)
	}
	defer rows.Close()

	var out []EventKey
	for rows.Next() {
		var k EventKey
		var pub []byte
		var createdAt string
		var revokedAt sql.NullString
		if err := rows.Scan(&k.ID, &k.EventID, &pub, &createdAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("store: scan event key row: %w", err)
		}
		k.PublicKey = ed25519.PublicKey(pub)
		if k.CreatedAt, err = textToTime(createdAt); err != nil {
			return nil, fmt.Errorf("store: parse event key created_at: %w", err)
		}
		if k.RevokedAt, err = textToNullTime(revokedAt); err != nil {
			return nil, fmt.Errorf("store: parse event key revoked_at: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// LegacyPlaintextKeyCount reports how many event_keys rows still hold their
// pre-0004 plaintext private key. Zero on any database that has completed the
// data half of migration 0004, and on every database created after it.
//
// cmd/cackle calls this at boot to decide whether it may start.
func (s *Store) LegacyPlaintextKeyCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM event_keys WHERE legacy_private_key IS NOT NULL`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count legacy event keys: %w", err)
	}
	return n, nil
}

// SealLegacyEventKeys is the DATA half of migration 0004: it encrypts every
// event_keys row still holding pre-0004 plaintext and clears that plaintext.
// It returns the number of rows sealed.
//
// Properties, all of them load-bearing:
//
//   - IDEMPOTENT. It selects only rows with a non-NULL legacy_private_key, so
//     running it on an already-migrated database seals nothing and returns 0.
//     It is safe to call on every boot, which is exactly what cmd/cackle does.
//   - FAIL CLOSED. With the vault locked it returns ErrKeyVaultLocked without
//     touching a single row — it will not half-migrate, and it will not
//     "temporarily" leave plaintext in place while pretending to have run.
//   - ALL OR NOTHING. Every row is sealed in ONE transaction. A crash leaves
//     either the whole batch sealed or the whole batch as it was.
//   - NO PLAINTEXT LEFT BEHIND. The same UPDATE that writes the ciphertext
//     NULLs legacy_private_key; the CHECK constraint from 0004 would reject
//     any row that tried to keep both. After committing, the WAL is
//     checkpointed and the database VACUUMed so the freed pages holding the
//     old plaintext are not merely unlinked but gone from the file.
//
// What it cannot do — and docs/SELF-HOSTING.md says this to operators
// directly — is reach plaintext that has already left this file. Every backup
// and snapshot taken before the upgrade still contains unencrypted signing
// keys and stays exactly as dangerous as it ever was.
func (s *Store) SealLegacyEventKeys(ctx context.Context) (int, error) {
	if !s.KeyVaultUnlocked() {
		return 0, ErrKeyVaultLocked
	}

	n, err := s.LegacyPlaintextKeyCount(ctx)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}

	sealedCount, err := s.sealLegacyBatch(ctx)
	if err != nil {
		return 0, err
	}

	// Purge the freed pages the plaintext lived on. Both statements must run
	// outside a transaction, and THE ORDER MATTERS: under WAL journaling (which
	// is what buildDSN configures for every file-backed database) VACUUM's
	// rebuilt pages are written to the write-ahead log, NOT to the database
	// file. Checkpointing before the VACUUM and stopping there leaves the
	// original plaintext page sitting in the main file — verified by
	// TestMigrationSealsRealPreMigrationDatabase, which scans the raw bytes and
	// fails on exactly that mistake. So: rebuild first, then force the rebuilt
	// pages down into the file and truncate the log.
	//
	// A failure here is reported rather than swallowed: the keys ARE sealed at
	// this point, but an operator told "migrated" deserves to know the file was
	// not rebuilt, because that residue is the difference between "encrypted at
	// rest" and "encrypted, sitting next to a copy of the original".
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return sealedCount, fmt.Errorf("store: seal legacy event keys: vacuum (keys are sealed; freed pages may still hold plaintext): %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return sealedCount, fmt.Errorf("store: seal legacy event keys: checkpoint wal (keys are sealed; freed pages may still hold plaintext): %w", err)
	}
	return sealedCount, nil
}

func (s *Store) sealLegacyBatch(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: seal legacy event keys: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	type legacyRow struct {
		id      string
		eventID string
		priv    []byte
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, event_id, legacy_private_key
		FROM event_keys WHERE legacy_private_key IS NOT NULL ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("store: seal legacy event keys: select: %w", err)
	}
	var pending []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.id, &r.eventID, &r.priv); err != nil {
			rows.Close()
			return 0, fmt.Errorf("store: seal legacy event keys: scan: %w", err)
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("store: seal legacy event keys: rows: %w", err)
	}
	rows.Close()

	for _, r := range pending {
		// A legacy row whose blob is not a well-formed Ed25519 private key
		// aborts the whole migration. Skipping it would silently leave
		// plaintext behind, and "encrypted except for the rows we could not
		// parse" is not a state anyone should discover later.
		if len(r.priv) != ed25519.PrivateKeySize {
			return 0, fmt.Errorf("store: seal legacy event keys: key %s has a %d-byte private key, want %d; refusing to migrate this database", r.id, len(r.priv), ed25519.PrivateKeySize)
		}
		sealed, nonce, err := s.sealPrivateKey(r.id, r.eventID, ed25519.PrivateKey(r.priv))
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE event_keys
			SET sealed_private_key = ?, sealed_nonce = ?, legacy_private_key = NULL
			WHERE id = ?`, sealed, nonce, r.id); err != nil {
			return 0, fmt.Errorf("store: seal legacy event keys: update %s: %w", r.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: seal legacy event keys: commit: %w", err)
	}
	return len(pending), nil
}

// EventKeyFingerprint returns a short non-reversible label for an event's
// current signing key, so an operator can confirm a key changed without ever
// being shown the key. Requires an unlocked vault.
func (s *Store) EventKeyFingerprint(ctx context.Context, eventID string) (string, error) {
	k, err := s.LatestActiveEventKey(ctx, eventID)
	if err != nil {
		return "", err
	}
	return keyvault.Fingerprint(k.ID, k.PrivateKey), nil
}
