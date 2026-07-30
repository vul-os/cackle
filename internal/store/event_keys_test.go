package store

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vul-os/cackle/internal/store/keyvault"
	"github.com/vul-os/cackle/internal/tickets"
)

// --- fixtures --------------------------------------------------------------

func mustEventWithKey(t *testing.T, st *Store) (*Event, *EventKey) {
	t.Helper()
	ctx := context.Background()

	org := &Org{Name: "Keys Ltd", Slug: "keys-" + NewID()}
	if err := st.CreateOrg(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	ev := &Event{
		OrgID: org.ID, Slug: "ev-" + NewID(), Title: "Sealed Fest", Status: "published",
		StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour),
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	k := &EventKey{PublicKey: pub, PrivateKey: priv}
	if err := st.CreateEventWithKey(ctx, ev, k); err != nil {
		t.Fatalf("create event with key: %v", err)
	}
	return ev, k
}

// rawEventKeyRow reads the columns straight out of SQLite, bypassing every
// accessor, so the tests can assert what is ACTUALLY on disk rather than what
// the Go API chooses to show them.
func rawEventKeyRow(t *testing.T, st *Store, keyID string) (sealed, nonce, legacy []byte) {
	t.Helper()
	var s, n, l []byte
	err := st.db.QueryRow(
		`SELECT sealed_private_key, sealed_nonce, legacy_private_key FROM event_keys WHERE id = ?`,
		keyID).Scan(&s, &n, &l)
	if err != nil {
		t.Fatalf("read raw event_keys row: %v", err)
	}
	return s, n, l
}

// --- (a) the key is encrypted at rest -------------------------------------

// TestEventKeyIsNotPlaintextOnDisk is the headline assertion: after creating
// an event, the private key must not appear anywhere in the database file.
// Not in the column, not in a freelist page, not in the WAL.
func TestEventKeyIsNotPlaintextOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	unlockTestVault(t, st)

	_, key := mustEventWithKey(t, st)

	sealed, nonce, legacy := rawEventKeyRow(t, st, key.ID)
	if len(sealed) == 0 || len(nonce) == 0 {
		t.Fatalf("sealed_private_key/sealed_nonce empty (%d/%d bytes)", len(sealed), len(nonce))
	}
	if legacy != nil {
		t.Fatalf("legacy_private_key is populated on a freshly created key (%d bytes)", len(legacy))
	}
	if bytes.Contains(sealed, key.PrivateKey) {
		t.Fatal("the ciphertext contains the private key verbatim")
	}

	// Flush WAL into the main file, then read every byte of both and look for
	// the key. This is the assertion that would have failed loudly before this
	// change, and the one that catches a future "convenience" write path.
	if _, err := st.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	for _, p := range []string{path, path + "-wal"} {
		blob, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if bytes.Contains(blob, key.PrivateKey) {
			t.Fatalf("%s contains the raw Ed25519 private key", p)
		}
		// The seed half alone is enough to reconstruct the key, so check it
		// separately rather than trusting that the 64-byte form is the only
		// shape it could take.
		if bytes.Contains(blob, key.PrivateKey.Seed()) {
			t.Fatalf("%s contains the Ed25519 private key seed", p)
		}
	}

	// The PUBLIC key, by contrast, is expected to be readable in the clear —
	// it is what gates pin, and pretending otherwise would be confusing.
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	if !bytes.Contains(blob, key.PublicKey) {
		t.Fatal("public key is not readable in the database file; the gate bundle path depends on it being there")
	}
}

func TestSealedKeyRoundTripsAndSigns(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	ev, key := mustEventWithKey(t, st)

	got, err := st.LatestActiveEventKey(ctx, ev.ID)
	if err != nil {
		t.Fatalf("LatestActiveEventKey: %v", err)
	}
	if !bytes.Equal(got.PrivateKey, key.PrivateKey) {
		t.Fatal("decrypted private key differs from the one stored")
	}
	if !bytes.Equal(got.PublicKey, key.PublicKey) {
		t.Fatal("public key differs from the one stored")
	}

	// And it is a working signing key, verified against the stored public half.
	msg := []byte("admit one")
	sig := ed25519.Sign(got.PrivateKey, msg)
	if !ed25519.Verify(got.PublicKey, msg, sig) {
		t.Fatal("signature made with the decrypted key does not verify")
	}
}

// TestSealedKeyIsBoundToItsRow proves the AAD binding at the storage layer: a
// ciphertext lifted from one event's row cannot be decrypted in another's,
// so an attacker with write access to the file cannot transplant a key.
func TestSealedKeyIsBoundToItsRow(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	evA, keyA := mustEventWithKey(t, st)
	evB, _ := mustEventWithKey(t, st)

	sealedA, nonceA, _ := rawEventKeyRow(t, st, keyA.ID)

	// Overwrite event B's key with event A's ciphertext.
	var keyBID string
	if err := st.db.QueryRow(`SELECT id FROM event_keys WHERE event_id = ?`, evB.ID).Scan(&keyBID); err != nil {
		t.Fatalf("find key B: %v", err)
	}
	if _, err := st.db.Exec(
		`UPDATE event_keys SET sealed_private_key = ?, sealed_nonce = ? WHERE id = ?`,
		sealedA, nonceA, keyBID); err != nil {
		t.Fatalf("transplant ciphertext: %v", err)
	}

	if _, err := st.LatestActiveEventKey(ctx, evB.ID); !errors.Is(err, keyvault.ErrWrongKey) {
		t.Fatalf("reading a transplanted key = %v, want keyvault.ErrWrongKey", err)
	}
	// Event A is untouched and still works.
	if _, err := st.LatestActiveEventKey(ctx, evA.ID); err != nil {
		t.Fatalf("event A key after transplant: %v", err)
	}
}

// --- (c) fail closed on missing key material ------------------------------

// TestLockedVaultRefusesRatherThanFallingBack is the test the brief asks for:
// with no key material configured, every path that would touch a private key
// REFUSES. It must not succeed, and it must not succeed-by-writing-plaintext.
func TestLockedVaultRefusesRatherThanFallingBack(t *testing.T) {
	st := openLockedTestStore(t)
	ctx := context.Background()

	if st.KeyVaultUnlocked() {
		t.Fatal("a freshly opened store reports its key vault unlocked")
	}

	org := &Org{Name: "No Passphrase Ltd", Slug: "nopass-" + NewID()}
	if err := st.CreateOrg(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	ev := &Event{
		OrgID: org.ID, Slug: "locked-ev", Title: "Locked", Status: "draft",
		StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour),
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// 1. Event creation refuses.
	err = st.CreateEventWithKey(ctx, ev, &EventKey{PublicKey: pub, PrivateKey: priv})
	if !errors.Is(err, ErrKeyVaultLocked) {
		t.Fatalf("CreateEventWithKey with a locked vault = %v, want ErrKeyVaultLocked", err)
	}

	// 2. And it wrote NOTHING — not the event, not a key. A half-created event
	//    with no signing key would be worse than a refusal.
	var events, keys int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM event_keys`).Scan(&keys); err != nil {
		t.Fatalf("count event_keys: %v", err)
	}
	if events != 0 || keys != 0 {
		t.Fatalf("refused creation still wrote rows: %d events, %d keys", events, keys)
	}

	// 3. Rotation refuses.
	err = st.CreateEventKey(ctx, &EventKey{EventID: "whatever", PublicKey: pub, PrivateKey: priv})
	if !errors.Is(err, ErrKeyVaultLocked) {
		t.Fatalf("CreateEventKey with a locked vault = %v, want ErrKeyVaultLocked", err)
	}

	// 4. The data migration refuses (rather than migrating to plaintext, or
	//    reporting success having done nothing).
	if _, err := st.SealLegacyEventKeys(ctx); !errors.Is(err, ErrKeyVaultLocked) {
		t.Fatalf("SealLegacyEventKeys with a locked vault = %v, want ErrKeyVaultLocked", err)
	}

	// 5. Unlocking with no material refuses, and leaves the vault locked.
	if err := st.UnlockKeyVault(ctx, keyvault.Source{}); !errors.Is(err, keyvault.ErrNoSource) {
		t.Fatalf("UnlockKeyVault(zero Source) = %v, want keyvault.ErrNoSource", err)
	}
	if st.KeyVaultUnlocked() {
		t.Fatal("vault reports unlocked after a refused unlock")
	}
}

// TestSigningRefusesAfterRelock covers the "server restarted without its
// passphrase" case: the sealed keys are all still there, and every one of
// them is unreadable.
func TestSigningRefusesAfterRelock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	unlockTestVault(t, st)
	ev, _ := mustEventWithKey(t, st)
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen with NO key material — exactly what happens when an operator
	// forgets the environment variable after a redeploy.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	if _, err := reopened.LatestActiveEventKey(context.Background(), ev.ID); !errors.Is(err, ErrKeyVaultLocked) {
		t.Fatalf("LatestActiveEventKey after relock = %v, want ErrKeyVaultLocked", err)
	}
}

func TestWrongKeyMaterialRefuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	unlockTestVault(t, st)
	ev, _ := mustEventWithKey(t, st)
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	wrong, err := keyvault.Keyfile([]byte("a-completely-different-keyfile!!!"))
	if err != nil {
		t.Fatalf("keyvault.Keyfile: %v", err)
	}
	err = reopened.UnlockKeyVault(context.Background(), wrong)
	if !errors.Is(err, keyvault.ErrWrongKey) {
		t.Fatalf("unlock with wrong material = %v, want keyvault.ErrWrongKey", err)
	}
	if reopened.KeyVaultUnlocked() {
		t.Fatal("vault unlocked despite wrong key material")
	}
	if _, err := reopened.LatestActiveEventKey(context.Background(), ev.ID); !errors.Is(err, ErrKeyVaultLocked) {
		t.Fatalf("signing key readable after a failed unlock: %v", err)
	}

	// The right material still works — a failed attempt does not damage
	// anything.
	unlockTestVault(t, reopened)
	if _, err := reopened.LatestActiveEventKey(context.Background(), ev.ID); err != nil {
		t.Fatalf("correct material after a failed attempt: %v", err)
	}
}

// TestKindMismatchRefusesIncludingDemo covers the promotion accident: a demo
// database must not be openable as a real one, and a real database must not be
// openable by --demo's public key.
func TestKindMismatchRefusesIncludingDemo(t *testing.T) {
	ctx := context.Background()

	t.Run("demo database refused by a real passphrase", func(t *testing.T) {
		dir := t.TempDir()
		st, err := Open(filepath.Join(dir, "cackle.db"))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer st.Close()
		if err := st.UnlockKeyVault(ctx, keyvault.DemoSource()); err != nil {
			t.Fatalf("unlock with demo source: %v", err)
		}
		mustEventWithKey(t, st)

		real, err := keyvault.Passphrase("a-real-operator-passphrase")
		if err != nil {
			t.Fatalf("Passphrase: %v", err)
		}
		if err := st.UnlockKeyVault(ctx, real); err == nil {
			t.Fatal("a real passphrase opened a demo database")
		} else if !bytes.Contains([]byte(err.Error()), []byte("PUBLIC --demo")) {
			t.Fatalf("unhelpful demo-promotion error: %v", err)
		}
	})

	t.Run("real database refused by --demo", func(t *testing.T) {
		dir := t.TempDir()
		st, err := Open(filepath.Join(dir, "cackle.db"))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer st.Close()
		real, err := keyvault.Passphrase("a-real-operator-passphrase")
		if err != nil {
			t.Fatalf("Passphrase: %v", err)
		}
		if err := st.UnlockKeyVault(ctx, real); err != nil {
			t.Fatalf("unlock with passphrase: %v", err)
		}
		if err := st.UnlockKeyVault(ctx, keyvault.DemoSource()); err == nil {
			t.Fatal("--demo opened a real database")
		}
	})

	t.Run("keyfile database refused by a passphrase", func(t *testing.T) {
		dir := t.TempDir()
		st, err := Open(filepath.Join(dir, "cackle.db"))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer st.Close()
		unlockTestVault(t, st)

		pass, err := keyvault.Passphrase("a-real-operator-passphrase")
		if err != nil {
			t.Fatalf("Passphrase: %v", err)
		}
		if err := st.UnlockKeyVault(ctx, pass); err == nil {
			t.Fatal("a passphrase opened a keyfile-sealed database")
		}
	})
}

// --- (b) the gate path never needs key material ---------------------------

// TestGatePathNeedsNoKeyMaterial is the verification the brief asks for: an
// offline gate holds public keys only, so nothing in this change may put a
// passphrase anywhere near one. Concretely — with the vault LOCKED, a process
// can still build the KeyRing a gate pins and verify a real ticket with it.
func TestGatePathNeedsNoKeyMaterial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	unlockTestVault(t, st)
	ev, key := mustEventWithKey(t, st)

	// Issue a ticket while the issuing server is unlocked (the only thing
	// that ever needs the private half).
	kid := tickets.KeyID(key.PublicKey)
	payload := tickets.Payload{
		TID: NewID(), EID: ev.ID, TT: NewID(), KID: kid,
		Sub: NewID(), Name: "Ada Lovelace", IAT: time.Now().Unix(),
	}
	token, err := tickets.Issue(payload, key.PrivateKey)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Now a process with NO key material at all — the state a bundle-building
	// or gate-serving process is in when no passphrase is configured.
	locked, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer locked.Close()
	if locked.KeyVaultUnlocked() {
		t.Fatal("expected a locked vault")
	}

	active, err := locked.ActiveEventKeys(context.Background(), ev.ID)
	if err != nil {
		t.Fatalf("ActiveEventKeys with a locked vault: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("got %d active keys, want 1", len(active))
	}
	if active[0].PrivateKey != nil {
		t.Fatal("the public-only list path returned private key material")
	}
	if !bytes.Equal(active[0].PublicKey, key.PublicKey) {
		t.Fatal("public key from the locked read path differs")
	}

	all, err := locked.ListEventKeys(context.Background(), ev.ID)
	if err != nil {
		t.Fatalf("ListEventKeys with a locked vault: %v", err)
	}
	for _, k := range all {
		if k.PrivateKey != nil {
			t.Fatal("ListEventKeys returned private key material")
		}
	}

	// Build the gate's ring from those public halves and verify the ticket
	// with no vault, no passphrase, and no network.
	ring := tickets.NewKeyRing(ev.ID)
	for _, k := range active {
		ring.Add(tickets.KeyID(k.PublicKey), k.PublicKey)
	}
	if _, err := tickets.VerifyWithRing(token, ring, time.Now()); err != nil {
		t.Fatalf("offline verification with a locked vault: %v", err)
	}

	// And the ring a gate would cache to disk contains no private material.
	ringPath := filepath.Join(dir, "keyring.json")
	if err := ring.SaveToFile(ringPath); err != nil {
		t.Fatalf("save keyring: %v", err)
	}
	blob, err := os.ReadFile(ringPath)
	if err != nil {
		t.Fatalf("read keyring: %v", err)
	}
	if bytes.Contains(blob, key.PrivateKey) || bytes.Contains(blob, key.PrivateKey.Seed()) {
		t.Fatal("the cached gate keyring contains private key material")
	}
	if !bytes.Contains(blob, []byte(kid)) {
		t.Fatalf("keyring does not mention the kid it should pin: %s", blob)
	}
}

// --- (e) rotation ---------------------------------------------------------

// TestRotationDoesNotVoidIssuedTicketsButRevocationDoes pins the precise
// rotation semantics, because a rotation story that silently voids sold
// tickets is not a rotation story:
//
//   - ADDING a key leaves every previously-issued ticket verifiable, because
//     the ring carries every non-revoked key and the ticket names its kid.
//   - REVOKING the key a ticket was signed with makes that ticket fail at the
//     gate with ErrUnknownKID once the gate re-pulls its bundle.
//
// Both halves are asserted so nobody can later "simplify" revocation into the
// rotation path without a test going red.
func TestRotationDoesNotVoidIssuedTicketsButRevocationDoes(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	ev, key1 := mustEventWithKey(t, st)

	kid1 := tickets.KeyID(key1.PublicKey)
	oldTicket, err := tickets.Issue(tickets.Payload{
		TID: NewID(), EID: ev.ID, TT: NewID(), KID: kid1,
		Sub: NewID(), Name: "Early Buyer", IAT: time.Now().Unix(),
	}, key1.PrivateKey)
	if err != nil {
		t.Fatalf("issue old ticket: %v", err)
	}

	// Rotate: add a second key. The first is NOT revoked.
	pub2, priv2, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	key2 := &EventKey{EventID: ev.ID, PublicKey: pub2, PrivateKey: priv2, CreatedAt: time.Now().Add(time.Second)}
	if err := st.CreateEventKey(ctx, key2); err != nil {
		t.Fatalf("CreateEventKey: %v", err)
	}

	active, err := st.ActiveEventKeys(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ActiveEventKeys: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("after rotation there are %d active keys, want 2 (the keyring must hold both or old tickets die)", len(active))
	}

	ringAfterRotation := tickets.NewKeyRing(ev.ID)
	for _, k := range active {
		ringAfterRotation.Add(tickets.KeyID(k.PublicKey), k.PublicKey)
	}

	// The already-issued ticket, already in an attendee's inbox, still works.
	if _, err := tickets.VerifyWithRing(oldTicket, ringAfterRotation, time.Now()); err != nil {
		t.Fatalf("rotation invalidated an already-issued ticket: %v", err)
	}

	// New tickets are signed with the NEW key.
	latest, err := st.LatestActiveEventKey(ctx, ev.ID)
	if err != nil {
		t.Fatalf("LatestActiveEventKey: %v", err)
	}
	if !bytes.Equal(latest.PublicKey, pub2) {
		t.Fatal("signing key after rotation is not the newest key")
	}
	newTicket, err := tickets.Issue(tickets.Payload{
		TID: NewID(), EID: ev.ID, TT: NewID(), KID: tickets.KeyID(latest.PublicKey),
		Sub: NewID(), Name: "Late Buyer", IAT: time.Now().Unix(),
	}, latest.PrivateKey)
	if err != nil {
		t.Fatalf("issue new ticket: %v", err)
	}
	if _, err := tickets.VerifyWithRing(newTicket, ringAfterRotation, time.Now()); err != nil {
		t.Fatalf("newly issued ticket does not verify: %v", err)
	}

	// Now REVOKE key1 — the compromise response — and rebuild the ring the way
	// a gate would on its next pull.
	var key1RowID string
	if err := st.db.QueryRow(
		`SELECT id FROM event_keys WHERE event_id = ? AND public_key = ?`, ev.ID, []byte(key1.PublicKey),
	).Scan(&key1RowID); err != nil {
		t.Fatalf("find key1 row: %v", err)
	}
	if err := st.RevokeEventKey(ctx, key1RowID, time.Now()); err != nil {
		t.Fatalf("RevokeEventKey: %v", err)
	}

	activeAfterRevoke, err := st.ActiveEventKeys(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ActiveEventKeys after revoke: %v", err)
	}
	if len(activeAfterRevoke) != 1 {
		t.Fatalf("after revocation there are %d active keys, want 1", len(activeAfterRevoke))
	}
	ringAfterRevoke := tickets.NewKeyRing(ev.ID)
	for _, k := range activeAfterRevoke {
		ringAfterRevoke.Add(tickets.KeyID(k.PublicKey), k.PublicKey)
	}

	// This is the sharp edge, asserted rather than described: the OLD ticket
	// now fails at the gate. Revocation is not a no-cost operation.
	if _, err := tickets.VerifyWithRing(oldTicket, ringAfterRevoke, time.Now()); !errors.Is(err, tickets.ErrUnknownKID) {
		t.Fatalf("ticket signed by a revoked key = %v, want tickets.ErrUnknownKID", err)
	}
	// The signature itself is still perfectly valid — revocation is a ring
	// policy, not a cryptographic event. A gate that has NOT re-pulled still
	// admits it, which is why the docs say rotation completes only when every
	// gate has refreshed.
	if _, err := tickets.VerifyWithRing(oldTicket, ringAfterRotation, time.Now()); err != nil {
		t.Fatalf("a stale ring should still verify the old ticket, got: %v", err)
	}
	// And the new ticket is unaffected.
	if _, err := tickets.VerifyWithRing(newTicket, ringAfterRevoke, time.Now()); err != nil {
		t.Fatalf("new ticket after revoking the old key: %v", err)
	}
	// Revoked keys are still listed (the row is not deleted), so an operator
	// can audit what was ever trusted.
	all, err := st.ListEventKeys(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ListEventKeys: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListEventKeys returned %d keys, want 2 including the revoked one", len(all))
	}
}

// TestRewrapKeyVaultRotatesMaterialWithoutTouchingKeys covers passphrase
// rotation: the operator's secret changes, the DEK does not, and therefore not
// one issued ticket is affected.
func TestRewrapKeyVaultRotatesMaterialWithoutTouchingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")
	ctx := context.Background()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	unlockTestVault(t, st)
	ev, key := mustEventWithKey(t, st)
	sealedBefore, nonceBefore, _ := rawEventKeyRow(t, st, mustKeyID(t, st, ev.ID))

	newSrc, err := keyvault.Passphrase("the-replacement-passphrase")
	if err != nil {
		t.Fatalf("Passphrase: %v", err)
	}
	oldSrc, err := keyvault.Keyfile(testKeyfile)
	if err != nil {
		t.Fatalf("Keyfile: %v", err)
	}
	if err := st.RewrapKeyVault(ctx, oldSrc, newSrc); err != nil {
		t.Fatalf("RewrapKeyVault: %v", err)
	}

	// The sealed event key bytes are untouched: rewrapping is one row.
	sealedAfter, nonceAfter, _ := rawEventKeyRow(t, st, mustKeyID(t, st, ev.ID))
	if !bytes.Equal(sealedBefore, sealedAfter) || !bytes.Equal(nonceBefore, nonceAfter) {
		t.Fatal("rewrapping the vault rewrote the sealed event key; it should only rewrap the DEK")
	}

	status, err := st.KeyVaultStatus(ctx)
	if err != nil {
		t.Fatalf("KeyVaultStatus: %v", err)
	}
	if status.SourceKind != keyvault.KindPassphrase {
		t.Fatalf("source kind after rewrap = %q, want %q", status.SourceKind, keyvault.KindPassphrase)
	}
	if status.RotatedAt == nil {
		t.Fatal("rotated_at not stamped after a rewrap")
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// The NEW material opens it and the key is unchanged.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if err := reopened.UnlockKeyVault(ctx, newSrc); err != nil {
		t.Fatalf("unlock with the new passphrase: %v", err)
	}
	got, err := reopened.LatestActiveEventKey(ctx, ev.ID)
	if err != nil {
		t.Fatalf("LatestActiveEventKey after rewrap: %v", err)
	}
	if !bytes.Equal(got.PrivateKey, key.PrivateKey) {
		t.Fatal("the event signing key changed across a passphrase rotation")
	}

	// The OLD material no longer opens it.
	if err := reopened.UnlockKeyVault(ctx, oldSrc); err == nil {
		t.Fatal("the superseded key material still opens the vault")
	}
}

func mustKeyID(t *testing.T, st *Store, eventID string) string {
	t.Helper()
	var id string
	if err := st.db.QueryRow(`SELECT id FROM event_keys WHERE event_id = ?`, eventID).Scan(&id); err != nil {
		t.Fatalf("find key id: %v", err)
	}
	return id
}

// --- (d) the data migration, against a real pre-migration database --------

// preMigrationDB builds a database at exactly the pre-0004 schema — migrations
// 0001 and 0002 applied and nothing else — containing an event whose signing
// key is a PLAINTEXT BLOB in event_keys.private_key, which is what every
// existing deployment looks like today.
func preMigrationDB(t *testing.T, path string) (db *sql.DB, eventID, keyID string, priv ed25519.PrivateKey, pub ed25519.PublicKey) {
	t.Helper()

	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	// Only the two migrations that existed before this change. Nothing here
	// renames or renumbers them; they are applied exactly as shipped.
	for _, m := range []struct {
		name    string
		version int
	}{
		{"0001_init.sql", 1},
		{"0002_admission_reported_result.sql", 2},
	} {
		if err := applyMigration(db, m.name, m.version); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
	}

	// Sanity: this really is the old schema, with a NOT NULL plaintext column.
	var plaintextCol int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('event_keys') WHERE name = 'private_key'`).Scan(&plaintextCol); err != nil {
		t.Fatalf("inspect event_keys: %v", err)
	}
	if plaintextCol != 1 {
		t.Fatal("pre-migration fixture does not have the plaintext private_key column")
	}

	pub, priv, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	orgID := NewID()
	eventID, keyID = NewID(), NewID()
	now := timeToText(time.Now())

	if _, err := db.Exec(
		`INSERT INTO orgs (id, name, slug, created_at) VALUES (?, ?, ?, ?)`,
		orgID, "Legacy Promoters", "legacy-"+orgID, now); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO events (id, org_id, slug, title, starts_at, ends_at, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'published', ?, ?)`,
		eventID, orgID, "legacy-fest-"+eventID, "Legacy Fest", now, now, now, now); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO event_keys (id, event_id, public_key, private_key, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		keyID, eventID, []byte(pub), []byte(priv), now); err != nil {
		t.Fatalf("insert plaintext event key: %v", err)
	}
	return db, eventID, keyID, priv, pub
}

func TestMigrationSealsRealPreMigrationDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")
	ctx := context.Background()

	db, eventID, keyID, priv, pub := preMigrationDB(t, path)

	// The plaintext key really is in the file to begin with. If this assertion
	// ever fails the rest of the test proves nothing.
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	if !bytes.Contains(before, priv) {
		t.Fatal("fixture is wrong: the pre-migration database does not contain the plaintext key")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	// Upgrade: Open applies every pending migration, including 0004.
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open (migrating): %v", err)
	}
	defer st.Close()

	// The schema half ran; the data half has not, so the key is parked as
	// legacy plaintext and the boot check can see it.
	n, err := st.LegacyPlaintextKeyCount(ctx)
	if err != nil {
		t.Fatalf("LegacyPlaintextKeyCount: %v", err)
	}
	if n != 1 {
		t.Fatalf("legacy plaintext keys after schema migration = %d, want 1", n)
	}
	status, err := st.KeyVaultStatus(ctx)
	if err != nil {
		t.Fatalf("KeyVaultStatus: %v", err)
	}
	if status.Initialised {
		t.Fatal("a migrated-but-unconfigured database reports an initialised vault")
	}
	if status.LegacyPlaintextKeys != 1 {
		t.Fatalf("status.LegacyPlaintextKeys = %d, want 1", status.LegacyPlaintextKeys)
	}

	// A legacy key must NOT be readable, or "encrypted at rest" would be a
	// claim the code does not keep for existing deployments.
	unlockTestVault(t, st)
	if _, err := st.LatestActiveEventKey(ctx, eventID); !errors.Is(err, ErrEventKeyNotSealed) {
		t.Fatalf("reading an unmigrated key = %v, want ErrEventKeyNotSealed", err)
	}

	// Run the data half.
	sealed, err := st.SealLegacyEventKeys(ctx)
	if err != nil {
		t.Fatalf("SealLegacyEventKeys: %v", err)
	}
	if sealed != 1 {
		t.Fatalf("sealed %d keys, want 1", sealed)
	}

	// The plaintext column is empty, the sealed columns are populated...
	sealedBlob, nonce, legacy := rawEventKeyRow(t, st, keyID)
	if legacy != nil {
		t.Fatalf("legacy_private_key still populated after migration (%d bytes)", len(legacy))
	}
	if len(sealedBlob) == 0 || len(nonce) == 0 {
		t.Fatal("sealed columns empty after migration")
	}

	// ...the key survived intact (this is a MIGRATION, not a re-keying: the
	// public key in every already-issued ticket's kid must still match)...
	got, err := st.LatestActiveEventKey(ctx, eventID)
	if err != nil {
		t.Fatalf("LatestActiveEventKey after migration: %v", err)
	}
	if !bytes.Equal(got.PrivateKey, priv) {
		t.Fatal("migration changed the private key")
	}
	if !bytes.Equal(got.PublicKey, pub) {
		t.Fatal("migration changed the public key")
	}
	msg := []byte("still the same issuer")
	if !ed25519.Verify(pub, msg, ed25519.Sign(got.PrivateKey, msg)) {
		t.Fatal("the migrated key no longer signs verifiably under its own public key")
	}

	// ...and the plaintext is gone from the FILE, not merely from the column.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db after migration: %v", err)
	}
	if bytes.Contains(after, priv) {
		t.Fatal("the database file still contains the plaintext private key after migration")
	}
	if bytes.Contains(after, priv.Seed()) {
		t.Fatal("the database file still contains the private key seed after migration")
	}
	if walBlob, err := os.ReadFile(path + "-wal"); err == nil {
		if bytes.Contains(walBlob, priv) || bytes.Contains(walBlob, priv.Seed()) {
			t.Fatal("the WAL still contains the plaintext private key after migration")
		}
	}

	// Idempotent: a second run seals nothing and changes nothing.
	again, err := st.SealLegacyEventKeys(ctx)
	if err != nil {
		t.Fatalf("second SealLegacyEventKeys: %v", err)
	}
	if again != 0 {
		t.Fatalf("second run sealed %d keys, want 0", again)
	}
	sealedBlob2, nonce2, _ := rawEventKeyRow(t, st, keyID)
	if !bytes.Equal(sealedBlob, sealedBlob2) || !bytes.Equal(nonce, nonce2) {
		t.Fatal("a second migration run re-sealed an already-sealed key")
	}
	if n, err := st.LegacyPlaintextKeyCount(ctx); err != nil || n != 0 {
		t.Fatalf("LegacyPlaintextKeyCount after migration = %d (err %v), want 0", n, err)
	}
}

// TestMigrationRefusesWithoutKeyMaterial is the other half of (d): if the
// operator has no passphrase yet, the upgrade must refuse rather than
// half-migrate.
func TestMigrationRefusesWithoutKeyMaterial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")
	ctx := context.Background()

	db, eventID, keyID, priv, _ := preMigrationDB(t, path)
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	if _, err := st.SealLegacyEventKeys(ctx); !errors.Is(err, ErrKeyVaultLocked) {
		t.Fatalf("SealLegacyEventKeys with no key material = %v, want ErrKeyVaultLocked", err)
	}

	// Nothing moved: the row is exactly as it was, still legacy, still
	// unreadable. Half-migrated is not a state this can produce.
	sealed, nonce, legacy := rawEventKeyRow(t, st, keyID)
	if len(sealed) != 0 || len(nonce) != 0 {
		t.Fatal("a refused migration wrote sealed columns anyway")
	}
	if !bytes.Equal(legacy, priv) {
		t.Fatal("a refused migration modified the legacy plaintext")
	}
	if n, err := st.LegacyPlaintextKeyCount(ctx); err != nil || n != 1 {
		t.Fatalf("LegacyPlaintextKeyCount = %d (err %v), want 1", n, err)
	}
	if _, err := st.LatestActiveEventKey(ctx, eventID); err == nil {
		t.Fatal("an unmigrated, unconfigured database served a signing key")
	}
}

// TestCheckConstraintForbidsSealedAndPlaintextTogether proves the schema
// itself, not just the Go code, refuses the "encrypted it and forgot to remove
// the original" state — the exact failure this whole change is about.
func TestCheckConstraintForbidsSealedAndPlaintextTogether(t *testing.T) {
	st := openTestStore(t)
	ev, _ := mustEventWithKey(t, st)
	keyID := mustKeyID(t, st, ev.ID)

	if _, err := st.db.Exec(
		`UPDATE event_keys SET legacy_private_key = ? WHERE id = ?`,
		[]byte("plaintext sneaking back in"), keyID); err == nil {
		t.Fatal("the schema allowed a row to hold both a sealed and a plaintext private key")
	}

	// Neither may a row hold nothing at all — an event with a vanished key.
	if _, err := st.db.Exec(
		`UPDATE event_keys SET sealed_private_key = NULL, sealed_nonce = NULL WHERE id = ?`, keyID); err == nil {
		t.Fatal("the schema allowed an event key row with no private key at all")
	}
}

func TestKeyVaultStatusLeaksNothing(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	_, key := mustEventWithKey(t, st)

	status, err := st.KeyVaultStatus(ctx)
	if err != nil {
		t.Fatalf("KeyVaultStatus: %v", err)
	}
	if !status.Initialised || !status.Unlocked {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.SourceKind != keyvault.KindKeyfile {
		t.Fatalf("SourceKind = %q, want %q", status.SourceKind, keyvault.KindKeyfile)
	}
	if status.KDF != keyvault.KDFHKDF {
		t.Fatalf("KDF = %q, want %q", status.KDF, keyvault.KDFHKDF)
	}

	// The status struct is what gets logged at boot. Rendering it must not
	// produce any part of the private key or the key material.
	rendered := []byte(fmt.Sprintf("%+v", status))
	if bytes.Contains(rendered, key.PrivateKey) || bytes.Contains(rendered, key.PrivateKey.Seed()) {
		t.Fatal("KeyVaultStatus renders private key material")
	}
	if bytes.Contains(rendered, testKeyfile) {
		t.Fatal("KeyVaultStatus renders the operator's key material")
	}

	// A fingerprint identifies the key without disclosing it.
	fp, err := st.EventKeyFingerprint(ctx, key.EventID)
	if err != nil {
		t.Fatalf("EventKeyFingerprint: %v", err)
	}
	if len(fp) != 8 {
		t.Fatalf("fingerprint %q has length %d, want 8", fp, len(fp))
	}
}
